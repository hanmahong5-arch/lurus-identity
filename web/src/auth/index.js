// Authentication module for lurus-identity SPA.
// Supports two token types:
//   1. Zitadel OIDC PKCE tokens  → stored in localStorage['token']
//   2. Lurus session tokens (WeChat login) → stored in localStorage['lurus_token']
//
// No external library — uses Web Crypto API (supported in all modern browsers).

const ISSUER      = 'https://auth.lurus.cn'
const CLIENT_ID   = import.meta.env.VITE_ZITADEL_CLIENT_ID || ''
const REDIRECT_URI = window.location.origin + '/callback'
const SCOPES      = 'openid profile email'

// ── Utilities ─────────────────────────────────────────────────────────────────

function randomString(len = 64) {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~'
  const arr = new Uint8Array(len)
  crypto.getRandomValues(arr)
  return Array.from(arr, x => chars[x % chars.length]).join('')
}

function base64url(buf) {
  return btoa(String.fromCharCode(...new Uint8Array(buf)))
    .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

async function sha256(str) {
  const data = new TextEncoder().encode(str)
  return crypto.subtle.digest('SHA-256', data)
}

// ── PKCE ──────────────────────────────────────────────────────────────────────

async function generatePKCE() {
  const verifier  = randomString(64)
  const challenge = base64url(await sha256(verifier))
  return { verifier, challenge }
}

// ── Token management ──────────────────────────────────────────────────────────

/**
 * Returns the current active token for API calls.
 * Prefers lurus session token (WeChat login) over Zitadel token.
 */
export function getToken() {
  return localStorage.getItem('lurus_token') || localStorage.getItem('token') || ''
}

/**
 * Stores a lurus-issued session token (received from WeChat OAuth callback).
 */
export function storeLurusToken(token) {
  localStorage.setItem('lurus_token', token)
}

// ── Public API ────────────────────────────────────────────────────────────────

/** Redirect user to Zitadel authorization endpoint (PKCE flow). */
export async function login() {
  if (!CLIENT_ID) {
    console.error('VITE_ZITADEL_CLIENT_ID is not set. Run scripts/setup-zitadel-app.ts first.')
    return
  }
  const { verifier, challenge } = await generatePKCE()
  sessionStorage.setItem('pkce_verifier', verifier)
  // Remember where to send the user after login (default: /wallet).
  // Exclude auth-flow pages to prevent redirect loops.
  const AUTH_PAGES = ['/login', '/callback', '/zlogin']
  const currentPath = window.location.pathname
  sessionStorage.setItem('login_return', AUTH_PAGES.includes(currentPath) ? '/wallet' : currentPath)

  const params = new URLSearchParams({
    client_id:             CLIENT_ID,
    redirect_uri:          REDIRECT_URI,
    response_type:         'code',
    scope:                 SCOPES,
    code_challenge:        challenge,
    code_challenge_method: 'S256',
  })
  window.location.href = `${ISSUER}/oauth/v2/authorize?${params}`
}

/**
 * Exchange authorization code for tokens (called from /callback page).
 * Returns the access_token string on success.
 */
export async function handleCallback(code) {
  const verifier = sessionStorage.getItem('pkce_verifier')
  if (!verifier) throw new Error('PKCE verifier missing — please restart the login flow.')

  const res = await fetch(`${ISSUER}/oauth/v2/token`, {
    method:  'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      grant_type:    'authorization_code',
      client_id:     CLIENT_ID,
      redirect_uri:  REDIRECT_URI,
      code,
      code_verifier: verifier,
    }),
  })

  const data = await res.json()
  if (!res.ok) throw new Error(data.error_description || data.error || 'Token exchange failed')

  localStorage.setItem('token', data.access_token)
  sessionStorage.removeItem('pkce_verifier')

  return data.access_token
}

/** Clear session and redirect to Zitadel end_session endpoint. */
export function logout() {
  localStorage.removeItem('token')
  localStorage.removeItem('lurus_token')
  sessionStorage.clear()
  const params = new URLSearchParams({
    client_id:               CLIENT_ID,
    post_logout_redirect_uri: window.location.origin,
  })
  window.location.href = `${ISSUER}/oidc/v1/end_session?${params}`
}

/** Returns true if any token exists in localStorage (not validated server-side). */
export function isLoggedIn() {
  return !!(localStorage.getItem('token') || localStorage.getItem('lurus_token'))
}
