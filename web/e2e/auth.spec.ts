import { test, expect } from "@playwright/test";

const USER = "e2euser";
const PASS = "e2epassword123";

// One shared backend + DB, so the single-user constraint forces an order:
// onboard creates the account, then login exercises it from a clean client.
test.describe.serial("auth", () => {
  test("onboarding creates the first account", async ({ page }) => {
    await page.goto("/");
    // Fresh DB has no users, so /login bounces to onboarding.
    await expect(page).toHaveURL(/onboard/);

    await page.getByLabel("Username").fill(USER);
    await page.getByLabel("Password", { exact: true }).fill(PASS);
    await page.getByLabel("Confirm Password").fill(PASS);
    await page.getByRole("button", { name: /create account/i }).click();

    // Onboard awaits its login before navigating, so the session cookie is set
    // and the app loads: catch-all lands on /settings without bouncing to login.
    await expect(page).toHaveURL(/settings/);
  });

  test("login sets a working, non-Secure session cookie over http", async ({
    page,
    context,
  }) => {
    // Clean client: drop the cookie and localStorage so this is a real login.
    await context.clearCookies();
    await page.goto("/login");
    await page.evaluate(() => localStorage.clear());

    await page.goto("/login");
    await page.getByLabel("Username").fill(USER);
    await page.getByLabel("Password", { exact: true }).fill(PASS);
    await page.getByRole("button", { name: /^login$/i }).click();

    await expect(page).toHaveURL(/settings/);

    // Browser-level guard for the fix: gorilla/sessions 1.4.0 defaulted the
    // cookie to Secure, which browsers drop on plain http, breaking IP:PORT.
    const session = (await context.cookies()).find(
      (c) => c.name === "user_session"
    );
    expect(session, "user_session cookie must be set").toBeTruthy();
    expect(session!.secure, "cookie must not be Secure over http").toBe(false);
    expect(session!.httpOnly, "cookie must be HttpOnly").toBe(true);

    // Staying on /settings proves the API accepted the cookie: a rejected
    // session would 401 and the client redirects to /login.
    await expect(page).toHaveURL(/settings/);
    await expect(page.getByRole("tab", { name: "Application" })).toBeVisible();
  });
});
