import { expect, test } from "@playwright/test";

import { kindClusterName, viewerUser } from "../env";
import { loginWithOIDC } from "../helpers/oidc";

test.describe("Kubernetes-native authorization", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("viewer permissions come from Kubernetes RBAC, not Lightkite roles", async ({
    page,
  }) => {
    await loginWithOIDC(page, viewerUser);

    const identity = await page.request.get("/api/auth/user");
    expect(identity.status()).toBe(200);
    expect(await identity.json()).toMatchObject({
      user: {
        sub: viewerUser.subject,
        username: viewerUser.username,
      },
      platformAdmin: false,
    });

    expect((await page.request.get("/api/v1/clusters")).status()).toBe(200);
    expect((await page.request.get("/api/v1/admin/clusters/")).status()).toBe(
      403,
    );
    expect((await page.request.get("/api/v1/admin/audit-logs")).status()).toBe(
      403,
    );

    const clusterPath = `/api/v1/_clusters/${encodeURIComponent(kindClusterName)}`;
    expect(
      (
        await page.request.get(
          `${clusterPath}/kubernetes/api/v1/namespaces/default/configmaps`,
        )
      ).status(),
    ).toBe(200);
    expect(
      (
        await page.request.get(
          `${clusterPath}/kubernetes/api/v1/namespaces/kube-system/configmaps`,
        )
      ).status(),
    ).toBe(403);
    const allowedHistory = await page.request.get(
      `${clusterPath}/configmaps/default/kube-root-ca.crt/history`,
    );
    expect(allowedHistory.status(), await allowedHistory.text()).toBe(200);
    const deniedHistory = await page.request.get(
      `${clusterPath}/configmaps/kube-system/kube-root-ca.crt/history`,
    );
    expect(deniedHistory.status(), await deniedHistory.text()).toBe(403);
    const deniedNodes = await page.request.get(
      `${clusterPath}/kubernetes/api/v1/nodes`,
    );
    expect(deniedNodes.status(), await deniedNodes.text()).toBe(403);
    const deniedWrite = await page.request.post(
      `${clusterPath}/kubernetes/api/v1/namespaces/default/configmaps`,
      {
        data: {
          apiVersion: "v1",
          kind: "ConfigMap",
          metadata: { name: "viewer-must-not-create" },
          data: { denied: "true" },
        },
      },
    );
    expect(deniedWrite.status(), await deniedWrite.text()).toBe(403);
  });
});
