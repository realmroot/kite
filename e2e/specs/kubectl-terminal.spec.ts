import { expect, test } from "@playwright/test";

import { adminUser } from "../env";

test("kubectl terminal uses the current OIDC user and cleans up its session", async ({
  page,
  request,
}) => {
  const clusterPath = "/api/v1/pods/kube-system";
  const sessionSelector = encodeURIComponent(
    "kite.io/component=kubectl-terminal",
  );
  const listSessionNames = async () => {
    const response = await request.get(
      `${clusterPath}?labelSelector=${sessionSelector}`,
    );
    expect(response.status()).toBe(200);
    const body = (await response.json()) as {
      items: Array<{ metadata: { name: string } }>;
    };
    return body.items.map((item) => item.metadata.name);
  };
  const sessionsBefore = new Set(await listSessionNames());

  await page.goto("/");
  await page.getByRole("button", { name: "Toggle Kubectl Terminal" }).click();
  await expect(
    page.getByText("Kubectl Terminal", { exact: true }),
  ).toBeVisible();
  await expect(page.locator(".xterm-rows")).toContainText(
    "kubectl session ready",
    { timeout: 90_000 },
  );
  const createdSession = (await listSessionNames()).find(
    (name) => !sessionsBefore.has(name),
  );
  expect(createdSession).toBeTruthy();

  await page.locator(".xterm-helper-textarea").focus();
  await page.keyboard.type("kubectl auth whoami");
  await page.keyboard.press("Enter");
  await expect(page.locator(".xterm-rows")).toContainText(adminUser.username, {
    timeout: 30_000,
  });

  await page.getByRole("button", { name: "Close terminal" }).click();
  await expect(page.getByText("Kubectl Terminal", { exact: true })).toHaveCount(
    0,
  );

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
