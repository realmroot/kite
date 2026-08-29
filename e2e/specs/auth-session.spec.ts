import { expect, test } from "@playwright/test";

import { adminUser, oidcIssuer } from "../env";
import { loginWithOIDC } from "../helpers/oidc";

test.describe("OIDC session", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("logout revokes the server session and returns to login", async ({
    page,
  }) => {
    await loginWithOIDC(page, adminUser);
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();

    await page.locator("header").getByRole("button").last().click();
    await page.getByRole("menuitem", { name: "Log out" }).click();

    await page.waitForURL("**/login");
    await expect(
      page.getByRole("button", {
        name: "Continue with Lightkite E2E Identity",
      }),
    ).toBeVisible();
    const user = await page.request.get("/api/auth/user");
    expect(user.status()).toBe(401);
  });
});

test.describe("OIDC login", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("returns to the dashboard with a provider-issued identity", async ({
    page,
  }) => {
    await loginWithOIDC(page, adminUser);
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();

    const response = await page.request.get("/api/auth/user");
    expect(response.status()).toBe(200);
    const body = (await response.json()) as {
      user: { issuer: string; sub: string; username: string };
      platformAdmin: boolean;
    };
    expect(body.user).toMatchObject({
      issuer: oidcIssuer,
      sub: adminUser.subject,
      username: adminUser.username,
    });
    expect(body.platformAdmin).toBe(true);
  });
});
