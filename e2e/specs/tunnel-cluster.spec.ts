import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { once } from "node:events";
import { expect, test } from "@playwright/test";

interface KubeconfigJSON {
  "current-context": string;
  contexts: Array<{ name: string; context: { cluster: string } }>;
  clusters: Array<{
    name: string;
    cluster: {
      server: string;
      "certificate-authority-data"?: string;
      "tls-server-name"?: string;
    };
  }>;
}

function loadClusterMetadata(kubeconfigPath: string) {
  const value = execFileSync(
    "kubectl",
    ["--kubeconfig", kubeconfigPath, "config", "view", "--raw", "-o", "json"],
    { encoding: "utf8" },
  );
  const config = JSON.parse(value) as KubeconfigJSON;
  const context = config.contexts.find(
    (candidate) => candidate.name === config["current-context"],
  );
  const cluster = config.clusters.find(
    (candidate) => candidate.name === context?.context.cluster,
  )?.cluster;
  if (!cluster?.server)
    throw new Error("kind kubeconfig has no current server");
  return cluster;
}

async function stopProcess(process: ChildProcess) {
  if (process.exitCode !== null) return;
  process.kill("SIGTERM");
  await Promise.race([
    once(process, "exit"),
    new Promise((_, reject) =>
      setTimeout(() => reject(new Error("cluster agent did not stop")), 10_000),
    ),
  ]);
}

test("tunnel cluster carries the current OIDC identity to Kubernetes", async ({
  request,
}) => {
  const kubeconfigPath = process.env.KUBECONFIG;
  if (!kubeconfigPath) throw new Error("KUBECONFIG is required for tunnel E2E");

  const cluster = loadClusterMetadata(kubeconfigPath);
  const suffix = Date.now().toString(36);
  const name = `kite-tunnel-${suffix}`;
  const temporaryDirectory = mkdtempSync(resolve(tmpdir(), "kite-tunnel-e2e-"));
  const caFile = resolve(temporaryDirectory, "ca.crt");
  const caData = cluster["certificate-authority-data"];
  if (!caData)
    throw new Error("kind kubeconfig has no certificate authority data");
  writeFileSync(caFile, Buffer.from(caData, "base64"));

  let clusterId: number | undefined;
  let agent: ChildProcess | undefined;
  try {
    const response = await request.post("/api/v1/admin/clusters/", {
      data: { name, connectionMode: "tunnel" },
    });
    const responseText = await response.text();
    expect(response.status(), responseText).toBe(201);
    const created = JSON.parse(responseText) as {
      id: number;
      clusterAgentServer: string;
      clusterAgentToken: string;
      clusterAgentPublicKey: string;
    };
    clusterId = created.id;

    const e2eDirectory = dirname(dirname(fileURLToPath(import.meta.url)));
    const kiteBinary = resolve(e2eDirectory, "..", "kite");
    agent = spawn(
      kiteBinary,
      [
        "cluster-agent",
        `--server=${created.clusterAgentServer}`,
        `--token=${created.clusterAgentToken}`,
        `--public-key=${created.clusterAgentPublicKey}`,
        `--api-server=${cluster.server}`,
        `--ca-file=${caFile}`,
        ...(cluster["tls-server-name"]
          ? [`--tls-server-name=${cluster["tls-server-name"]}`]
          : []),
      ],
      { stdio: ["ignore", "pipe", "pipe"] },
    );

    let processOutput = "";
    agent.stdout?.on("data", (chunk) => {
      processOutput += chunk.toString();
    });
    agent.stderr?.on("data", (chunk) => {
      processOutput += chunk.toString();
    });

    await expect
      .poll(
        async () => {
          if (agent?.exitCode !== null) {
            throw new Error(`cluster agent exited early:\n${processOutput}`);
          }
          const catalogResponse = await request.get("/api/v1/admin/clusters/");
          if (!catalogResponse.ok()) return false;
          const catalog = (await catalogResponse.json()) as Array<{
            id: number;
            connected: boolean;
          }>;
          return (
            catalog.find((item) => item.id === clusterId)?.connected ?? false
          );
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    const namespaces = await request.get(
      `/api/v1/_clusters/${encodeURIComponent(name)}/namespaces`,
    );
    const namespacesText = await namespaces.text();
    expect(namespaces.status(), namespacesText).toBe(200);
    const body = JSON.parse(namespacesText) as {
      items: Array<{ metadata: { name: string } }>;
    };
    expect(body.items.some((item) => item.metadata.name === "default")).toBe(
      true,
    );
  } finally {
    if (agent) await stopProcess(agent);
    if (clusterId) {
      await request.delete(`/api/v1/admin/clusters/${clusterId}`);
    }
    rmSync(temporaryDirectory, { recursive: true, force: true });
  }
});
