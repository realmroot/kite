import { execFileSync } from "node:child_process";
import { expect, test } from "@playwright/test";

import { adminUser, authFile, ensureAuthDir, kindClusterName } from "../env";
import { loginWithOIDC } from "../helpers/oidc";

interface KubeconfigJSON {
  "current-context": string;
  contexts: Array<{
    name: string;
    context: { cluster: string };
  }>;
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

function bindOIDCUsers(kubeconfigPath: string) {
  const resources = {
    apiVersion: "v1",
    kind: "List",
    items: [
      {
        apiVersion: "rbac.authorization.k8s.io/v1",
        kind: "ClusterRoleBinding",
        metadata: { name: "kite-e2e-admin" },
        subjects: [
          {
            kind: "User",
            name: adminUser.username,
            apiGroup: "rbac.authorization.k8s.io",
          },
        ],
        roleRef: {
          kind: "ClusterRole",
          name: "cluster-admin",
          apiGroup: "rbac.authorization.k8s.io",
        },
      },
      {
        apiVersion: "rbac.authorization.k8s.io/v1",
        kind: "RoleBinding",
        metadata: { name: "kite-e2e-viewer", namespace: "default" },
        subjects: [
          {
            kind: "User",
            name: "viewer@kite.test",
            apiGroup: "rbac.authorization.k8s.io",
          },
        ],
        roleRef: {
          kind: "ClusterRole",
          name: "view",
          apiGroup: "rbac.authorization.k8s.io",
        },
      },
    ],
  };
  execFileSync(
    "kubectl",
    ["--kubeconfig", kubeconfigPath, "apply", "-f", "-"],
    { input: JSON.stringify(resources), stdio: ["pipe", "pipe", "pipe"] },
  );
}

test("create a reusable OIDC admin session and credential-free catalog", async ({
  page,
}) => {
  const kubeconfigPath = process.env.KUBECONFIG;
  if (!kubeconfigPath) throw new Error("KUBECONFIG is required for e2e tests");

  bindOIDCUsers(kubeconfigPath);
  const cluster = loadClusterMetadata(kubeconfigPath);

  await loginWithOIDC(page, adminUser);

  const identityResponse = await page.request.get("/api/auth/user");
  const identityBody = await identityResponse.text();
  expect(identityResponse.status(), identityBody).toBe(200);
  expect(
    (JSON.parse(identityBody) as { platformAdmin?: boolean }).platformAdmin,
    identityBody,
  ).toBe(true);

  const response = await page.request.post("/api/v1/admin/clusters/", {
    data: {
      name: kindClusterName,
      description: "OIDC-enabled local kind cluster",
      apiServerUrl: cluster.server,
      caBundle: cluster["certificate-authority-data"] || "",
      tlsServerName: cluster["tls-server-name"] || "",
      isDefault: true,
    },
  });
  const responseBody = await response.text();
  expect(response.status(), responseBody).toBe(201);

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();

  ensureAuthDir();
  await page.context().storageState({ path: authFile });
});
