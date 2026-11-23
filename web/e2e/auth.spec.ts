import { test, expect, type Page } from "@playwright/test";

const USER = "e2euser";
const PASS = "e2epassword123";
const NEW_PASS = "e2e-new-password-456";

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Username").fill(USER);
  await page.getByLabel("Password", { exact: true }).fill(PASS);
  await page.getByRole("button", { name: /^login$/i }).click();
  await expect(page).toHaveURL(/settings/);
}

async function openSecurityTab(page: Page) {
  await page.getByRole("tab", { name: "Security" }).click();
}

async function fillPasswordForm(page: Page, current: string, next: string) {
  await page
    .getByRole("textbox", { name: "Current password", exact: true })
    .fill(current);
  await page
    .getByRole("textbox", { name: "New password", exact: true })
    .fill(next);
  await page
    .getByRole("textbox", { name: "Confirm new password", exact: true })
    .fill(next);
  await page.getByRole("button", { name: /change password/i }).click();
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
      (c) => c.name === "user_session",
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

  test("a wrong current password is rejected and keeps the session", async ({
    page,
  }) => {
    await login(page);
    await openSecurityTab(page);

    await fillPasswordForm(page, "not-the-password", NEW_PASS);

    await expect(page.getByText("Current password is incorrect")).toBeVisible();
    await expect(page).toHaveURL(/settings/);
  });

  test("changing the password signs out other sessions and swaps the working password", async ({
    page,
    browser,
  }) => {
    test.slow();
    await login(page);
    const otherContext = await browser.newContext();
    const otherPage = await otherContext.newPage();
    await login(otherPage);

    await openSecurityTab(page);
    await fillPasswordForm(page, PASS, NEW_PASS);
    await expect(page.getByText(/password changed/i)).toBeVisible();

    // The caller stays signed in; every other session is gone.
    await page.reload();
    await expect(page).toHaveURL(/settings/);
    await otherPage.reload();
    await expect(otherPage).toHaveURL(/login/);
    await otherContext.close();

    await page.getByRole("button", { name: /logout/i }).click();
    await expect(page).toHaveURL(/login/);

    await page.getByLabel("Username").fill(USER);
    await page.getByLabel("Password", { exact: true }).fill(PASS);
    await page.getByRole("button", { name: /^login$/i }).click();
    await expect(page.getByText(/login failed/i)).toBeVisible();

    await page.getByLabel("Password", { exact: true }).fill(NEW_PASS);
    await page.getByRole("button", { name: /^login$/i }).click();
    await expect(page).toHaveURL(/settings/);
  });
});
