import { test, expect, type Page } from "@playwright/test";

const USER = "e2euser";
const PASS = "e2epassword123";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Username").fill(USER);
  await page.getByLabel("Password", { exact: true }).fill(PASS);
  await page.getByRole("button", { name: /^login$/i }).click();
  await expect(page).toHaveURL(/settings/);
}

// One shared backend + DB, so the single-user constraint forces an order:
// onboarding (test 1) creates the account, the rest log in as that user. Each
// test gets a fresh browser context, so no cookie/localStorage leaks between them.
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
    await login(page);

    // Browser-level guard for the fix: gorilla/sessions 1.4.0 defaulted the
    // cookie to Secure, which browsers drop on plain http, breaking IP:PORT.
    const session = (await context.cookies()).find(
      (c) => c.name === "user_session"
    );
    expect(session, "user_session cookie must be set").toBeTruthy();
    expect(session!.secure, "cookie must not be Secure over http").toBe(false);
    expect(session!.httpOnly, "cookie must be HttpOnly").toBe(true);

    // A visible authed tab proves the API accepted the cookie: a rejected
    // session would 401 and the client redirects to /login.
    await expect(page.getByRole("tab", { name: "Application" })).toBeVisible();
  });

  test("wrong credentials are rejected", async ({ page }) => {
    await page.goto("/login");
    await page.getByLabel("Username").fill(USER);
    await page.getByLabel("Password", { exact: true }).fill("wrong-password");
    await page.getByRole("button", { name: /^login$/i }).click();

    await expect(page.getByText(/login failed/i)).toBeVisible();
    await expect(page).toHaveURL(/login/);
  });

  test("unauthenticated access to a protected route redirects to login", async ({
    page,
  }) => {
    await page.goto("/settings");
    await expect(page).toHaveURL(/login/);
  });

  test("logout ends the session", async ({ page }) => {
    await login(page);

    await page.getByRole("button", { name: /logout/i }).click();
    await expect(page).toHaveURL(/login/);
  });
});
