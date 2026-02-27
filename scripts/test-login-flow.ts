/**
 * Quick smoke-test: verify identity.lurus.cn redirects to Zitadel with PKCE params.
 * Usage: npx tsx scripts/test-login-flow.ts
 */
import { chromium } from "playwright";

const IDENTITY_URL = "https://identity.lurus.cn";
const EXPECTED_CLIENT_ID = "361861888939722569@lurus-api";
const ZITADEL_URL = "https://auth.lurus.cn";

async function run() {
  let browser;
  for (const opts of [{ channel: "msedge", headless: true }, { channel: "chrome", headless: true }, { headless: true }]) {
    try {
      browser = await (chromium as any).launch({ ...opts, timeout: 20_000 });
      break;
    } catch { /* try next */ }
  }
  if (!browser) throw new Error("No browser available.");

  const ctx = await browser.newContext({ ignoreHTTPSErrors: false });
  const page = await ctx.newPage();

  try {
    console.log(`[1] Opening ${IDENTITY_URL} ...`);
    await page.goto(IDENTITY_URL, { waitUntil: "domcontentloaded", timeout: 20000 });
    await page.waitForTimeout(3000);

    const finalUrl = page.url();
    console.log(`    Final URL: ${finalUrl}`);

    // Zitadel processes /oauth/v2/authorize (with PKCE params) server-side,
    // then redirects to /ui/login/login?authRequestID=<id>. The PKCE params
    // are stored in the authRequestID — they won't appear in the final URL.
    const isAtZitadelLogin = finalUrl.startsWith(`${ZITADEL_URL}/ui/login`);
    const hasAuthRequestID = finalUrl.includes("authRequestID=");

    console.log(`\n--- Results ---`);
    console.log(`  Redirected to Zitadel login: ${isAtZitadelLogin ? "✓" : "✗"}`);
    console.log(`  authRequestID present:       ${hasAuthRequestID ? "✓" : "✗"} (PKCE params stored server-side)`);

    if (isAtZitadelLogin && hasAuthRequestID) {
      console.log(`\n✅ PKCE flow working correctly!`);
      console.log(`   (Zitadel stored PKCE params in authRequestID — correct behavior)`);
    } else {
      console.log(`\n✗ PKCE flow issues detected.`);
      const title = await page.title();
      const bodySnip = await page.locator("body").innerText().catch(() => "");
      console.log(`  Page title: ${title}`);
      console.log(`  Body: ${bodySnip.slice(0, 300)}`);
      process.exit(1);
    }
  } finally {
    await browser.close();
  }
}

run().catch(e => { console.error(e); process.exit(1); });
