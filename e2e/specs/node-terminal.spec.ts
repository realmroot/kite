import { expect, test } from "@playwright/test";

import { kindClusterName } from "../env";

const nodeName = `${kindClusterName}-control-plane`;

test("node terminal is Kubernetes-authorized and cleans up its privileged session", async ({
  page,
  request,
}) => {
  const sessionSelector = encodeURIComponent("kite.io/component=node-terminal");
  const listSessionNames = async () => {
    const response = await request.get(
      `/api/v1/pods/kube-system?labelSelector=${sessionSelector}`,
    );
    expect(response.status()).toBe(200);
    const body = (await response.json()) as {
      items: Array<{ metadata: { name: string } }>;
    };
    return body.items.map((item) => item.metadata.name);
  };
  const sessionsBefore = new Set(await listSessionNames());

  await page.goto(`/nodes/${nodeName}`);
  await page.getByRole("tab", { name: "Terminal" }).click();
  await expect(page.locator(".xterm-rows")).toContainText(
    "node terminal ready",
    {
      timeout: 90_000,
    },
  );
  const createdSession = (await listSessionNames()).find(
    (name) => !sessionsBefore.has(name),
  );
  expect(createdSession).toBeTruthy();

  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("printf NODE_TERMINAL_OK");
  await page.keyboard.press("Enter");
  await expect(page.locator(".xterm-rows")).toContainText("NODE_TERMINAL_OK");

  await page.goto("/");
  await expect
    .poll(
      async () => {
        const sessions = await listSessionNames();
        return sessions.includes(createdSession!);
      },
      { timeout: 30_000 },
    )
    .toBe(false);
});
