import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import {
  expect,
  request as playwrightRequest,
  test,
  type APIResponse,
} from "@playwright/test";

import { adminUser, kindClusterName, oidcIssuer } from "../env";

async function expectJSON<T>(response: APIResponse, status: number) {
  const text = await response.text();
  expect(response.status(), text).toBe(status);
  return JSON.parse(text) as T;
}

async function expectStatus(response: APIResponse, status: number) {
  const text = await response.text();
  expect(response.status(), text).toBe(status);
}

test("backend contracts follow OIDC and Kubernetes-native authorization", async ({
  request,
}, testInfo) => {
  const baseURL = testInfo.project.use.baseURL;
  if (typeof baseURL !== "string") throw new Error("baseURL is required");

  const suffix = Date.now().toString(36);
  const clusterPath = `/api/v1/_clusters/${encodeURIComponent(kindClusterName)}`;
  const configMapName = `kite-e2e-${suffix}`;
  const templateName = `Kite E2E ${suffix}`;
  const repositoryName = `kite-e2e-${suffix}`;
  let dummyClusterId: number | undefined;
  let templateId: number | undefined;
  let repositoryId: number | undefined;
  let configMapCreated = false;

  const repositoryServer = createServer((incoming, response) => {
    if (incoming.url === "/index.yaml") {
      response.writeHead(200, { "content-type": "application/yaml" });
      response.end(
        "apiVersion: v1\nentries: {}\ngenerated: 2026-08-20T00:00:00Z\n",
      );
      return;
    }
    response.writeHead(404);
    response.end();
  });
  await new Promise<void>((resolve) =>
    repositoryServer.listen(0, "127.0.0.1", resolve),
  );
  const repositoryPort = (repositoryServer.address() as AddressInfo).port;

  try {
    const anonymous = await playwrightRequest.newContext({
      baseURL,
      storageState: { cookies: [], origins: [] },
    });
    try {
      await expectStatus(await anonymous.get("/healthz"), 200);
      await expectStatus(await anonymous.get("/metrics"), 200);
      await expectStatus(await anonymous.get("/api/v1/version"), 200);
      await expectStatus(await anonymous.get("/api/v1/clusters"), 401);
      await expectStatus(
        await anonymous.get(
          `${clusterPath}/kubernetes/api/v1/namespaces/default/configmaps`,
        ),
        401,
      );
      await expectStatus(await anonymous.get("/api/v1/admin/clusters/"), 401);
    } finally {
      await anonymous.dispose();
    }

    const identity = await expectJSON<{
      user: {
        issuer: string;
        sub: string;
        username: string;
        roles?: unknown;
      };
      platformAdmin: boolean;
    }>(await request.get("/api/auth/user"), 200);
    expect(identity).toMatchObject({
      user: {
        issuer: oidcIssuer,
        sub: adminUser.subject,
        username: adminUser.username,
      },
      platformAdmin: true,
    });
    expect(identity.user.roles).toBeUndefined();

    const catalog = await expectJSON<
      Array<
        Record<string, unknown> & {
          id: number;
          name: string;
          description: string;
          apiServerUrl: string;
          caBundle: string;
          tlsServerName: string;
          enabled: boolean;
          isDefault: boolean;
        }
      >
    >(await request.get("/api/v1/admin/clusters/"), 200);
    const kindCluster = catalog.find(
      (cluster) => cluster.name === kindClusterName,
    );
    expect(kindCluster).toBeDefined();
    expect(kindCluster).not.toHaveProperty("config");

    await expectStatus(
      await request.post("/api/v1/admin/clusters/", {
        data: {
          name: `legacy-${suffix}`,
          apiServerUrl: "https://127.0.0.1:6443",
          config: "credential-bearing kubeconfig must be rejected",
        },
      }),
      400,
    );

    const dummyCluster = await expectJSON<{ id: number }>(
      await request.post("/api/v1/admin/clusters/", {
        data: {
          name: `metadata-${suffix}`,
          description: "credential-free catalog contract",
          apiServerUrl: "https://127.0.0.1:1",
        },
      }),
      201,
    );
    dummyClusterId = dummyCluster.id;
    await expectStatus(
      await request.put(`/api/v1/admin/clusters/${dummyClusterId}`, {
        data: {
          description: "updated credential-free metadata",
          apiServerUrl: "https://127.0.0.1:1",
          enabled: false,
        },
      }),
      200,
    );

    const template = await expectJSON<{ id: number; name: string }>(
      await request.post("/api/v1/admin/templates/", {
        data: {
          name: templateName,
          description: "OIDC architecture E2E template",
          yaml: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: example\n",
        },
      }),
      201,
    );
    templateId = template.id;
    const templates = await expectJSON<Array<{ id: number }>>(
      await request.get(`${clusterPath}/templates`),
      200,
    );
    expect(templates.some((item) => item.id === templateId)).toBe(true);

    const repository = await expectJSON<{ id: number; hasAuth: boolean }>(
      await request.post("/api/v1/admin/charts/repositories", {
        data: {
          name: repositoryName,
          url: `http://127.0.0.1:${repositoryPort}`,
        },
      }),
      201,
    );
    repositoryId = repository.id;
    expect(repository.hasAuth).toBe(false);
    const repositories = await expectJSON<
      Array<{ id: number; password?: string }>
    >(await request.get(`${clusterPath}/charts/repositories`), 200);
    expect(
      repositories.find((item) => item.id === repositoryId),
    ).not.toHaveProperty("password");

    const configMap = {
      apiVersion: "v1",
      kind: "ConfigMap",
      metadata: { name: configMapName },
      data: { phase: "created" },
    };
    await expectStatus(
      await request.post(
        `${clusterPath}/kubernetes/api/v1/namespaces/default/configmaps`,
        {
          data: configMap,
        },
      ),
      201,
    );
    configMapCreated = true;
    const fetched = await expectJSON<{ data: { phase: string } }>(
      await request.get(
        `${clusterPath}/kubernetes/api/v1/namespaces/default/configmaps/${configMapName}`,
      ),
      200,
    );
    expect(fetched.data.phase).toBe("created");

    const search = await expectJSON<{
      results: Array<{ name: string; resourceType: string }>;
    }>(
      await request.get(
        `${clusterPath}/search?q=${encodeURIComponent(configMapName)}`,
      ),
      200,
    );
    expect(search.results).toContainEqual(
      expect.objectContaining({
        name: configMapName,
        resourceType: "configmaps",
      }),
    );

    await expectStatus(
      await request.get(`${clusterPath}/prometheus/resource-usage-history`),
      503,
    );

    const metricsServerPods = await expectJSON<{
      items: Array<{ metadata: { name: string } }>;
    }>(
      await request.get(
        `${clusterPath}/kubernetes/api/v1/namespaces/kube-system/pods?labelSelector=${encodeURIComponent(
          "k8s-app=metrics-server",
        )}`,
      ),
      200,
    );
    const metricsPodName = metricsServerPods.items[0]?.metadata.name;
    expect(metricsPodName).toBeTruthy();
    const metricsPath = `${clusterPath}/prometheus/pods/kube-system/${encodeURIComponent(
      metricsPodName,
    )}/metrics?duration=30m`;
    await expect
      .poll(async () => (await request.get(metricsPath)).status(), {
        timeout: 60_000,
      })
      .toBe(200);
    const podMetrics = await expectJSON<{
      cpu: Array<{ value: number }>;
      memory: Array<{ value: number }>;
      fallback: boolean;
    }>(await request.get(metricsPath), 200);
    expect(podMetrics.fallback).toBe(true);
    expect(podMetrics.cpu.length).toBeGreaterThan(0);
    expect(podMetrics.memory.length).toBeGreaterThan(0);

    await expectStatus(
      await request.put(`/api/v1/admin/clusters/${kindCluster!.id}`, {
        data: {
          name: kindCluster!.name,
          description: kindCluster!.description,
          apiServerUrl: kindCluster!.apiServerUrl,
          caBundle: kindCluster!.caBundle,
          tlsServerName: kindCluster!.tlsServerName,
          prometheusURL: "http://kite-prometheus-api.monitoring.svc:9090",
          isDefault: kindCluster!.isDefault,
          enabled: kindCluster!.enabled,
        },
      }),
      200,
    );
    const prometheusHistory = await expectJSON<{
      cpu: Array<{ value: number }>;
      memory: Array<{ value: number }>;
      networkIn: Array<{ value: number }>;
      networkOut: Array<{ value: number }>;
    }>(
      await request.get(
        `${clusterPath}/prometheus/resource-usage-history?duration=30m`,
      ),
      200,
    );
    expect(prometheusHistory.cpu[0]?.value).toBe(1.5);
    expect(prometheusHistory.memory[0]?.value).toBe(1.5);
    expect(prometheusHistory.networkIn[0]?.value).toBe(1.5);
    expect(prometheusHistory.networkOut[0]?.value).toBe(1.5);

    const prometheusPodMetrics = await expectJSON<{ fallback: boolean }>(
      await request.get(metricsPath),
      200,
    );
    expect(prometheusPodMetrics.fallback).toBe(false);

    await expectStatus(
      await request.delete(
        `${clusterPath}/kubernetes/api/v1/namespaces/default/configmaps/${configMapName}`,
      ),
      200,
    );
    configMapCreated = false;

    const audit = await expectJSON<{
      data: Array<{
        resourceName: string;
        operator: { issuer: string; sub: string; username: string };
      }>;
    }>(
      await request.get(
        `/api/v1/admin/audit-logs?resourceName=${encodeURIComponent(configMapName)}`,
      ),
      200,
    );
    expect(audit.data.length).toBeGreaterThanOrEqual(2);
    expect(audit.data[0].operator).toMatchObject({
      issuer: oidcIssuer,
      sub: adminUser.subject,
      username: adminUser.username,
    });
  } finally {
    if (configMapCreated) {
      await request.delete(
        `${clusterPath}/kubernetes/api/v1/namespaces/default/configmaps/${configMapName}`,
      );
    }
    if (repositoryId) {
      await request.delete(`/api/v1/admin/charts/repositories/${repositoryId}`);
    }
    if (templateId) {
      await request.delete(`/api/v1/admin/templates/${templateId}`);
    }
    if (dummyClusterId) {
      await request.delete(`/api/v1/admin/clusters/${dummyClusterId}`);
    }
    await new Promise<void>((resolve, reject) =>
      repositoryServer.close((error) => (error ? reject(error) : resolve())),
    );
  }
});
