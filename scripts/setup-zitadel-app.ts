/**
 * Zitadel OIDC Application Setup Script
 *
 * Creates a "User Agent" (PKCE, no client secret) application in Zitadel
 * for the lurus-identity frontend SPA at identity.lurus.cn.
 *
 * Usage: npx tsx scripts/setup-zitadel-app.ts
 */

import { chromium } from "playwright";
import { writeFileSync } from "fs";
import { resolve } from "path";
import { fileURLToPath } from "url";

// Cross-runtime compatible __dirname
const __dirname = typeof import.meta.dir !== "undefined"
  ? import.meta.dir
  : resolve(fileURLToPath(import.meta.url), "..");

const ZITADEL_URL    = "https://auth.lurus.cn";
const ADMIN_EMAIL    = "zitadel-admin@zitadel.auth.lurus.cn";
const ADMIN_PASSWORD = "Lurus@ops";
const APP_NAME       = "lurus-identity";
const PROJECT_NAME   = "lurus-api"; // existing project

const REDIRECT_URIS = [
  "http://localhost:5173/callback",
  "https://identity.lurus.cn/callback",
];
const POST_LOGOUT_URIS = [
  "http://localhost:5173/",
  "https://identity.lurus.cn/",
];

async function run() {
  let browser;
  const candidates = [
    { channel: "msedge",    headless: true },
    { channel: "chrome",    headless: true },
    { headless: true },
  ];
  for (const opts of candidates) {
    try {
      console.log(`Trying: channel=${(opts as any).channel ?? "chromium"} headless=${opts.headless}`);
      browser = await chromium.launch({ ...opts, timeout: 30_000 } as any);
      break;
    } catch { /* try next */ }
  }
  if (!browser) throw new Error("No browser available. Install Chrome or Edge.");

  const ctx  = await browser.newContext({ ignoreHTTPSErrors: false, viewport: { width: 1280, height: 900 } });
  const page = await ctx.newPage();

  const screenshot = (name: string) =>
    page.screenshot({ path: resolve(__dirname, "..", "web", name), fullPage: true }).catch(() => {});

  try {
    console.log("[1/6] Opening Zitadel console (triggers auth redirect)...");
    await page.goto(`${ZITADEL_URL}/ui/console`, { waitUntil: "networkidle", timeout: 30000 });

    console.log("[2/6] Logging in...");
    // Step 1: login name — use click + pressSequentially to trigger Angular change detection
    await page.waitForSelector('input[type="text"], input[name="loginName"]', { timeout: 15000 });
    await screenshot(".zitadel-debug-1-loginpage.png");
    const loginInput = page.locator('input[type="text"], input[name="loginName"]').first();
    await loginInput.click();
    await loginInput.pressSequentially(ADMIN_EMAIL, { delay: 50 });
    await screenshot(".zitadel-debug-2-loginfilled.png");
    await page.keyboard.press("Enter");
    await page.waitForTimeout(1000);
    await screenshot(".zitadel-debug-3-afterenter.png");

    // Step 2: password
    await page.waitForSelector('input[type="password"]', { timeout: 15000 });
    const pwInput = page.locator('input[type="password"]').first();
    await pwInput.click();
    await pwInput.pressSequentially(ADMIN_PASSWORD, { delay: 50 });
    await page.keyboard.press("Enter");

    await page.waitForURL(/\/ui\/console.*/, { timeout: 20000 });
    console.log("  ✓ Logged in");

    console.log(`[3/6] Opening project: ${PROJECT_NAME}...`);
    await page.goto(`${ZITADEL_URL}/ui/console/projects`, { waitUntil: "networkidle", timeout: 20000 });
    await page.waitForTimeout(1500);

    // Click on project
    const projectLink = page.locator(`text="${PROJECT_NAME}"`).first();
    if (await projectLink.count() > 0) {
      await projectLink.click();
    } else {
      await page.click(`[data-testid="${PROJECT_NAME}"], .project-card:has-text("${PROJECT_NAME}")`);
    }
    await page.waitForURL(/\/ui\/console\/projects\/\d+/, { timeout: 10000 });
    console.log("  ✓ Project opened");

    console.log("[4/6] Creating new application...");
    // Close any accidentally-opened modal (e.g. Add Manager dialog)
    await page.keyboard.press("Escape");
    await page.waitForTimeout(500);

    // Navigate directly to the new-application URL (project ID is in current URL)
    const projectUrl = page.url();
    const projectId = projectUrl.match(/\/projects\/(\d+)/)?.[1];
    if (!projectId) throw new Error(`Could not extract project ID from URL: ${projectUrl}`);
    await page.goto(`${ZITADEL_URL}/ui/console/projects/${projectId}/apps/create`, { waitUntil: "networkidle", timeout: 20000 });
    await page.waitForTimeout(2000);
    await screenshot(".zitadel-debug-4-newapp.png");
    console.log(`  Current URL: ${page.url()}`);

    // Step 1: Name and Type
    // Use getByRole to find the visible name textbox robustly
    const nameInput = page.getByRole('textbox').first();
    await nameInput.waitFor({ state: 'visible', timeout: 15000 });
    await nameInput.click();
    await nameInput.pressSequentially(APP_NAME, { delay: 50 });
    await page.waitForTimeout(300);

    // Select "User Agent" card (SPA/PKCE). Use getByText with partial match.
    await page.getByText(/User Agent/i).first().click();
    await page.waitForTimeout(400);
    await screenshot(".zitadel-debug-5-nameandtype.png");

    // Click Step 1 Continue
    await page.locator('[data-e2e="continue-button-nameandtype"]').click({ force: true });
    await page.waitForTimeout(1000);
    await screenshot(".zitadel-debug-6-step2.png");

    // Step 2: Authentication Method — select PKCE
    await page.getByText(/PKCE/i).first().click().catch(() => {});
    await page.waitForTimeout(400);
    // Try data-e2e first, fallback to any Continue button
    await page.locator('[data-e2e="continue-button-authmethod"]').click({ force: true })
      .catch(() => page.locator('button').filter({ hasText: /Continue|Next/i }).last().click({ force: true }));
    await page.waitForTimeout(1000);
    await screenshot(".zitadel-debug-7-step3.png");

    // Step 3: Redirect URIs
    for (const uri of REDIRECT_URIS) {
      const uriInput = page.getByRole('textbox').first();
      if (await uriInput.count() > 0) {
        await uriInput.click();
        await uriInput.pressSequentially(uri, { delay: 30 });
        await page.keyboard.press("Enter");
        await page.waitForTimeout(400);
      }
    }

    // Step 3: click Continue → arrives at Step 4 Overview
    await page.locator('[data-e2e="continue-button-redirecturis"]').click({ force: true })
      .catch(() => page.locator('button').filter({ hasText: /Continue/i }).last().click({ force: true }));
    await page.waitForTimeout(1000);
    await screenshot(".zitadel-debug-8-overview.png");

    // Step 4: Overview — click "Create" to actually create the app
    await page.getByRole('button', { name: /Create/i }).click();
    await page.waitForTimeout(2000);
    await screenshot(".zitadel-debug-9-created.png");

    console.log("[5/6] Extracting Client ID...");
    await page.waitForTimeout(1500);
    const pageContent = await page.content();

    // Extract client_id (format: digits@project-name)
    const clientIdMatch = pageContent.match(/(\d{15,20}@[\w-]+)/);
    let clientId = "";
    if (clientIdMatch) {
      clientId = clientIdMatch[1]!;
      console.log(`  ✓ Client ID: ${clientId}`);
    } else {
      const idEl = await page.locator('[data-testid="client-id"], .client-id').first().textContent().catch(() => "");
      if (idEl) clientId = idEl.trim();
      console.log(`  Client ID (manual check needed): ${idEl || "not found automatically"}`);
      console.log("  → Check the Zitadel app page for the Client ID");
    }

    console.log("[6/6] Saving result...");
    const result = {
      clientId,
      redirectUris:     REDIRECT_URIS,
      postLogoutUris:   POST_LOGOUT_URIS,
      issuer:           ZITADEL_URL,
      note:             `Add VITE_ZITADEL_CLIENT_ID=${clientId} to web/.env.local and rebuild.`,
    };

    const outPath = resolve(__dirname, "..", "web", ".zitadel-app.json");
    writeFileSync(outPath, JSON.stringify(result, null, 2));
    console.log(`\n✅ Done! Saved to ${outPath}`);
    console.log(`\nNext steps:`);
    console.log(`  1. echo 'VITE_ZITADEL_CLIENT_ID=${clientId}' > web/.env.local`);
    console.log(`  2. cd web && bun run build`);
    console.log(`  3. git add -A && git commit -m "feat(identity): OIDC PKCE auth"`);
    console.log(`  4. git push → CI builds → ArgoCD deploys`);

    await screenshot(".zitadel-setup-done.png");

  } catch (err) {
    console.error("Setup failed:", err);
    await screenshot(".zitadel-setup-error.png");
    throw err;
  } finally {
    await browser.close();
  }
}

run().catch(e => { console.error(e); process.exit(1); });
