import type { Page } from "@playwright/test";

interface OIDCUser {
  username: string;
  password: string;
}

export async function loginWithOIDC(page: Page, user: OIDCUser) {
  await page.goto("/login");
  await page.getByRole("button", { name: /^Continue with / }).click();
  await page.locator('input[name="login"]').fill(user.username);
  await page.locator('input[name="password"]').fill(user.password);
  await page.getByRole("button", { name: "Login" }).click();
  await page.waitForURL((url) => url.pathname === "/");
}
