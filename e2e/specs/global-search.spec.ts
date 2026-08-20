import { expect, test } from "@playwright/test";

import { kindClusterName } from "../env";

test("admin can open global search and jump to the cluster settings tab", async ({
  page,
}) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();

  await page.getByRole("button", { name: /Search resources/i }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();

  await dialog.getByPlaceholder("Search resources...").fill("settings cluster");

  const clusterResult = dialog.getByText("Cluster", { exact: true });
  await expect(clusterResult).toBeVisible();

  await clusterResult.click();

  await page.waitForURL(
    (url) =>
      url.pathname === "/settings" &&
      url.searchParams.get("tab") === "clusters",
  );

  await expect(page.getByRole("button", { name: "Add Cluster" })).toBeVisible();
});

test("global search finds a real Kubernetes resource and opens its detail page", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Search resources/i }).click();

  const dialog = page.getByRole("dialog");
  const nodeName = `${kindClusterName}-control-plane`;
  await dialog.getByPlaceholder("Search resources...").fill(`node ${nodeName}`);
  const result = dialog.getByText(nodeName, { exact: true });
  await expect(result).toBeVisible();
  await result.click();

  await page.waitForURL(`**/nodes/${nodeName}`);
  await expect(page.getByText(nodeName, { exact: true }).first()).toBeVisible();
});
