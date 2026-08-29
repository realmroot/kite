import http from "node:http";
import {
  expect,
  request as playwrightRequest,
  test,
  type APIRequestContext,
  type APIResponse,
} from "@playwright/test";

import { authFile, kindClusterName, viewerUser } from "../env";
import { loginWithOIDC } from "../helpers/oidc";

interface RawResponse {
  status: number;
  body: string;
}

async function expectStatus(response: APIResponse, status: number) {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
}

async function sendRawHTTP(
  baseURL: string,
  path: string,
  cookieHeader: string,
): Promise<RawResponse> {
  const url = new URL(baseURL);
  return new Promise<RawResponse>((resolve, reject) => {
    const request = http.request(
      {
        hostname: url.hostname,
        port: url.port || (url.protocol === "https:" ? "443" : "80"),
        method: "GET",
        path,
        headers: { Cookie: cookieHeader },
      },
      (response) => {
        let body = "";
        response.setEncoding("utf8");
        response.on("data", (chunk: string) => {
          body += chunk;
        });
        response.on("end", () =>
          resolve({ status: response.statusCode ?? 0, body }),
        );
      },
    );
    request.on("error", reject);
    request.end();
  });
}

test.use({ storageState: { cookies: [], origins: [] } });

test("encoded proxy paths cannot escape Kubernetes RBAC", async ({
  page,
}, testInfo) => {
  const baseURL = testInfo.project.use.baseURL;
  if (typeof baseURL !== "string") throw new Error("baseURL is required");

  const suffix = Date.now().toString(36);
  const podName = `e2e-proxy-pod-${suffix}`;
  const secretName = `e2e-proxy-secret-${suffix}`;
  const secretProofValue = Buffer.from(`proxy-escape-proof-${suffix}`).toString(
    "base64",
  );
  const clusterPath = `/api/v1/_clusters/${encodeURIComponent(kindClusterName)}`;
  const podPath = `${clusterPath}/kubernetes/api/v1/namespaces/default/pods/${podName}`;
  const secretPath = `${clusterPath}/kubernetes/api/v1/namespaces/kube-system/secrets/${secretName}`;
  let podCreated = false;
  let secretCreated = false;
  let adminRequest: APIRequestContext | undefined;

  try {
    adminRequest = await playwrightRequest.newContext({
      baseURL,
      storageState: authFile,
    });
    await expectStatus(
      await adminRequest.post(
        `${clusterPath}/kubernetes/api/v1/namespaces/default/pods`,
        {
          data: {
            apiVersion: "v1",
            kind: "Pod",
            metadata: { name: podName, labels: { "e2e.kite.io/test": suffix } },
            spec: {
              containers: [
                { name: "pause", image: "registry.k8s.io/pause:3.10.1" },
              ],
            },
          },
        },
      ),
      201,
    );
    podCreated = true;
    await expectStatus(
      await adminRequest.post(
        `${clusterPath}/kubernetes/api/v1/namespaces/kube-system/secrets`,
        {
          data: {
            apiVersion: "v1",
            kind: "Secret",
            metadata: { name: secretName, namespace: "kube-system" },
            type: "Opaque",
            data: { proof: secretProofValue },
          },
        },
      ),
      201,
    );
    secretCreated = true;

    await loginWithOIDC(page, viewerUser);
    await expectStatus(await page.request.get(podPath), 200);
    await expectStatus(
      await page.request.get(
        `${clusterPath}/kubernetes/api/v1/namespaces/kube-system/secrets`,
      ),
      403,
    );
    await expectStatus(await page.request.get(secretPath), 403);

    const storage = await page.context().storageState();
    const cookieHeader = storage.cookies
      .map((cookie) => `${cookie.name}=${cookie.value}`)
      .join("; ");
    expect(cookieHeader).toContain("kite_session=");

    const dotdot = "%2e%2e";
    const escapePath = `${clusterPath}/kubernetes/api/v1/namespaces/default/pods/${encodeURIComponent(
      podName,
    )}/proxy/${Array(4).fill(dotdot).join("/")}/kube-system/secrets`;
    const escaped = await sendRawHTTP(baseURL, escapePath, cookieHeader);
    expect(escaped.status, escaped.body).toBeGreaterThanOrEqual(400);
    expect(escaped.body).not.toContain(secretName);
    expect(escaped.body).not.toContain(secretProofValue);

    const clusterEscapePath = `${clusterPath}/kubernetes/api/v1/namespaces/default/pods/${encodeURIComponent(
      podName,
    )}/proxy/${Array(5).fill(dotdot).join("/")}/secrets`;
    const clusterEscaped = await sendRawHTTP(
      baseURL,
      clusterEscapePath,
      cookieHeader,
    );
    expect(clusterEscaped.status, clusterEscaped.body).toBeGreaterThanOrEqual(
      400,
    );
    expect(clusterEscaped.body).not.toContain(secretName);
    expect(clusterEscaped.body).not.toContain(secretProofValue);

    const escapedName = [
      "%2e%2e",
      "%2e%2e",
      "%2e%2e",
      "namespaces",
      "kube-system",
      "services",
      "http%3akube-dns%3ametrics",
    ].join("%2f");
    const nameEscapePath = `${clusterPath}/kubernetes/api/v1/namespaces/default/pods/${escapedName}/proxy/metrics`;
    const nameEscaped = await sendRawHTTP(
      baseURL,
      nameEscapePath,
      cookieHeader,
    );
    expect(nameEscaped.status, nameEscaped.body).toBeGreaterThanOrEqual(400);
    expect(nameEscaped.body).not.toContain("# HELP");

    const legitimatePath = `${clusterPath}/kubernetes/api/v1/namespaces/default/pods/${encodeURIComponent(
      podName,
    )}/proxy/`;
    const legitimate = await sendRawHTTP(baseURL, legitimatePath, cookieHeader);
    expect(legitimate.body).not.toContain("invalid proxy path");
    if (legitimate.status === 403) {
      expect(legitimate.body).toContain("pods/proxy");
      expect(legitimate.body).toContain(viewerUser.username);
    }
  } finally {
    if (podCreated) {
      await adminRequest?.delete(podPath).catch(() => undefined);
    }
    if (secretCreated) {
      await adminRequest?.delete(secretPath).catch(() => undefined);
    }
    await adminRequest?.dispose();
  }
});
