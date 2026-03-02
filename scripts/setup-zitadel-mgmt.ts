/**
 * Zitadel Management API Setup Script
 *
 * Idempotent: safe to run multiple times.
 * Replaces the Playwright-based setup-zitadel-app.ts.
 *
 * What it does:
 *  1. Verify/create project roles: admin, ops
 *  2. Register OIDC Apps: lurus-newapi, lurus-webmail (skip if already exists)
 *  3. Print all App clientIds for K8s secret updates
 *
 * Prerequisites (manual console steps):
 *  - Create Machine User "lurus-ops-bot" in Zitadel Settings → Service Users
 *  - Issue a PAT for lurus-ops-bot and copy it
 *  - Grant lurus-ops-bot "Org Owner" permission
 *  - Copy project numeric ID from lurus-api project URL
 *
 * Usage:
 *   ZITADEL_PAT=<pat> ZITADEL_ORG_ID=<org_id> ZITADEL_PROJECT_ID=<project_id> \
 *     bun run scripts/setup-zitadel-mgmt.ts
 */

const ZITADEL_URL = process.env.ZITADEL_URL || "https://auth.lurus.cn";
const PAT = process.env.ZITADEL_PAT;
const ORG_ID = process.env.ZITADEL_ORG_ID;
const PROJECT_ID = process.env.ZITADEL_PROJECT_ID;

if (!PAT || !ORG_ID || !PROJECT_ID) {
  console.error(
    "ERROR: Required environment variables missing.\n" +
    "Set: ZITADEL_PAT, ZITADEL_ORG_ID, ZITADEL_PROJECT_ID\n\n" +
    "Example:\n" +
    "  ZITADEL_PAT=xxx ZITADEL_ORG_ID=123 ZITADEL_PROJECT_ID=456 \\\n" +
    "    bun run scripts/setup-zitadel-mgmt.ts"
  );
  process.exit(1);
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

async function mgmtGet(path: string): Promise<unknown> {
  const url = `${ZITADEL_URL}/management/v1${path}`;
  const res = await fetch(url, {
    headers: {
      Authorization: `Bearer ${PAT}`,
      "x-zitadel-orgid": ORG_ID!,
      "Content-Type": "application/json",
    },
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`GET ${path} → ${res.status}: ${body}`);
  }
  return res.json();
}

async function mgmtPost(path: string, body: unknown): Promise<unknown> {
  const url = `${ZITADEL_URL}/management/v1${path}`;
  const res = await fetch(url, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${PAT}`,
      "x-zitadel-orgid": ORG_ID!,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`POST ${path} → ${res.status}: ${text}`);
  }
  return res.json();
}

// ---------------------------------------------------------------------------
// Role management
// ---------------------------------------------------------------------------

interface RoleResult {
  key: string;
}
interface RoleSearchResult {
  result?: RoleResult[];
}

async function getExistingRoles(): Promise<Set<string>> {
  const data = (await mgmtPost(
    `/projects/${PROJECT_ID}/roles/_search`,
    { limit: 100 }
  )) as RoleSearchResult;
  const keys = new Set<string>();
  for (const role of data.result ?? []) {
    keys.add(role.key);
  }
  return keys;
}

async function ensureRole(key: string, displayName: string, existingKeys: Set<string>): Promise<void> {
  if (existingKeys.has(key)) {
    console.log(`  role "${key}": already exists ✓`);
    return;
  }
  await mgmtPost(`/projects/${PROJECT_ID}/roles`, { key, displayName });
  console.log(`  role "${key}": created ✓`);
}

// ---------------------------------------------------------------------------
// App management
// ---------------------------------------------------------------------------

interface AppResult {
  name: string;
  id: string;
  oidcConfig?: { clientId: string };
}
interface AppSearchResult {
  result?: AppResult[];
}

async function getExistingApps(): Promise<Map<string, { id: string; clientId: string }>> {
  const data = (await mgmtPost(
    `/projects/${PROJECT_ID}/apps/_search`,
    { limit: 100 }
  )) as AppSearchResult;
  const apps = new Map<string, { id: string; clientId: string }>();
  for (const app of data.result ?? []) {
    apps.set(app.name, {
      id: app.id,
      clientId: app.oidcConfig?.clientId ?? "",
    });
  }
  return apps;
}

interface OIDCAppConfig {
  name: string;
  redirectUris: string[];
  postLogoutRedirectUris: string[];
}

interface CreateOIDCResponse {
  appId: string;
  clientId: string;
}

async function ensureOIDCApp(
  cfg: OIDCAppConfig,
  existingApps: Map<string, { id: string; clientId: string }>
): Promise<{ clientId: string; created: boolean }> {
  const existing = existingApps.get(cfg.name);
  if (existing) {
    console.log(`  app "${cfg.name}": already exists (clientId: ${existing.clientId}) ✓`);
    return { clientId: existing.clientId, created: false };
  }

  const data = (await mgmtPost(`/projects/${PROJECT_ID}/apps/oidc`, {
    name: cfg.name,
    // User Agent = SPA (PKCE, no client secret)
    appType: "OIDC_APP_TYPE_USER_AGENT",
    authMethodType: "OIDC_AUTH_METHOD_TYPE_NONE",
    redirectUris: cfg.redirectUris,
    responseTypes: ["OIDC_RESPONSE_TYPE_CODE"],
    grantTypes: ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"],
    postLogoutRedirectUris: cfg.postLogoutRedirectUris,
    version: "OIDC_VERSION_1_0",
    devMode: false,
    accessTokenType: "OIDC_TOKEN_TYPE_BEARER",
    accessTokenRoleAssertion: true,
    idTokenRoleAssertion: true,
    idTokenUserinfoAssertion: true,
  })) as CreateOIDCResponse;

  console.log(`  app "${cfg.name}": created (clientId: ${data.clientId}) ✓`);
  return { clientId: data.clientId, created: true };
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

const APPS_TO_REGISTER: OIDCAppConfig[] = [
  {
    name: "lurus-newapi",
    redirectUris: ["https://newapi.lurus.cn/callback"],
    postLogoutRedirectUris: ["https://newapi.lurus.cn/"],
  },
  {
    name: "lurus-webmail",
    redirectUris: ["https://mail.lurus.cn/auth/callback"],
    postLogoutRedirectUris: ["https://mail.lurus.cn/"],
  },
];

const ROLES_TO_ENSURE: Array<{ key: string; displayName: string }> = [
  { key: "admin", displayName: "Platform Admin" },
  { key: "ops", displayName: "Operations Staff" },
];

async function main() {
  console.log(`Zitadel Management Setup`);
  console.log(`  ZITADEL_URL:    ${ZITADEL_URL}`);
  console.log(`  PROJECT_ID:     ${PROJECT_ID}`);
  console.log(`  ORG_ID:         ${ORG_ID}`);
  console.log();

  // --- Step 1: Roles ---
  console.log("[1/2] Ensuring project roles...");
  const existingRoles = await getExistingRoles();
  for (const role of ROLES_TO_ENSURE) {
    await ensureRole(role.key, role.displayName, existingRoles);
  }
  console.log();

  // --- Step 2: OIDC Apps ---
  console.log("[2/2] Ensuring OIDC Apps...");
  const existingApps = await getExistingApps();
  const results: Array<{ name: string; clientId: string; created: boolean }> = [];

  for (const app of APPS_TO_REGISTER) {
    const result = await ensureOIDCApp(app, existingApps);
    results.push({ name: app.name, clientId: result.clientId, created: result.created });
  }
  console.log();

  // --- Summary ---
  const newApps = results.filter((r) => r.created);
  if (newApps.length === 0) {
    console.log("All apps already exist. No changes made.");
  } else {
    console.log("New apps registered. Update K8s secrets:");
    console.log();
    for (const app of newApps) {
      const secretName = app.name === "lurus-newapi" ? "lurus-newapi-secrets" : "lurus-webmail-secrets";
      const ns = app.name === "lurus-newapi" ? "lurus-system" : "lurus-webmail";
      console.log(`  kubectl patch secret ${secretName} -n ${ns} --type=merge \\`);
      console.log(`    -p '{"stringData":{"ZITADEL_CLIENT_ID":"${app.clientId}"}}'`);
      console.log();
    }
  }

  console.log("Done.");
}

main().catch((err) => {
  console.error("Fatal:", err instanceof Error ? err.message : err);
  process.exit(1);
});
