
// src/auth.js

export const ADMIN_USERNAME = 'admin'; // UI label only

let unauthorizedHandler = null;

export function setUnauthorizedHandler(fn) {
  unauthorizedHandler = typeof fn === 'function' ? fn : null;
}

// Generic API helper that always sends cookies and JSON
async function api(url, opts = {}) {
  const res = await fetch(url, {
    credentials: 'include',                           // <-- send/receive cookie
    headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
    ...opts,
  });

  if (res.status === 401 && unauthorizedHandler) {
    try { unauthorizedHandler(); } catch {}
  }

  let data = null;
  try { data = await res.json(); } catch {}
  return { ok: res.ok, status: res.status, data };
}

/**
 * Login: server validates credentials & sets HttpOnly + Secure cookie
 */
export async function login(username, password) {
  const { ok, data } = await api('/api/v1/GetToken', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  return ok
    ? { ok: true, defaultCredentials: !!data?.defaultCredentials }
    : { ok: false, message: data?.error || 'Invalid credentials' };
}

/**
 * Logout: clears cookie on the server
 */
export async function logout() {
  const { ok, data } = await api('/api/v1/logout', { method: 'POST' });
  return ok ? { ok: true } : { ok: false, message: data?.error || 'Logout failed' };
}

/**
 * isAuthenticated: call a protected route to check if the JWT cookie is valid
 */
export async function isAuthenticated() {
  const { ok } = await api('/api/v1/me', { method: 'GET' });
  return ok;
}

/**
 * Change password: updates DB + bumps jwt_version;
 * server issues a fresh cookie for current session.
 */
export async function changePassword(oldPassword, newPassword) {
  const { ok, data } = await api('/api/v1/ChangePassword', {
    method: 'POST',
    body: JSON.stringify({ oldPassword, newPassword }),
  });
  return ok ? { ok: true } : { ok: false, message: data?.error || 'Unable to change password' };
}

/**
 * Reset admin/admin: bumps jwt_version; server clears cookie (forces re-login)
 */
export async function resetToDefault() {
  const { ok, data } = await api('/api/v1/ResetToDefault', { method: 'POST' });
  return ok ? { ok: true } : { ok: false, message: data?.error || 'Reset failed' };
}

/**
 * Helper: wrapper for your protected fetch() calls
 */
export async function authFetch(url, opts = {}) {
  const res = await fetch(url, { credentials: 'include', ...opts }); // cookie gets attached automatically
  if (res.status === 401 && unauthorizedHandler) {
    try { unauthorizedHandler(); } catch {}
  }
  return res;
}
