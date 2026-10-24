(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.DoitdoitAuth = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";
  function createAuth({ origin, redirect, storage, session, fetchImpl = fetch, locks = navigator.locks, cryptoImpl = crypto }) {
    const prefix = "doitdoit:service:" + origin;
    const tokenKey = prefix + ":tokens";
    const read = key => JSON.parse(storage.getItem(key) || "null");
    function write(key, value) {
      const bytes = JSON.stringify(value);
      storage.setItem(key, bytes);
      if (storage.getItem(key) !== bytes) throw new Error("browser storage unavailable");
    }
    const b64 = bytes => btoa(String.fromCharCode(...bytes)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    async function json(path, options = {}) {
      const response = await fetchImpl(origin + path, { ...options, credentials: "omit", cache: "no-store", redirect: "error", signal: AbortSignal.timeout(30000) });
      const value = await response.json().catch(() => null);
      if (!response.ok) {
        const error = value?.error;
        throw Object.assign(new Error(error?.message || value?.error_description || "Service request failed (" + response.status + ")"),
          { status: response.status, code: error?.code || error, current: error?.current });
      }
      return value;
    }
    function requireLocks() {
      if (!locks?.request) throw new Error("This browser needs Web Locks support for secure sign-in. Use a current browser.");
    }
    async function start() {
      requireLocks();
      const client = await json("/oauth/register", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({
        client_name: "doitdoit web", redirect_uris: [redirect], token_endpoint_auth_method: "none",
        grant_types: ["authorization_code", "refresh_token"], response_types: ["code"],
      }) });
      const verifier = b64(cryptoImpl.getRandomValues(new Uint8Array(32)));
      const state = b64(cryptoImpl.getRandomValues(new Uint8Array(32)));
      const challenge = b64(new Uint8Array(await cryptoImpl.subtle.digest("SHA-256", new TextEncoder().encode(verifier))));
      const login = JSON.stringify({ verifier, state, client: client.client_id, redirect, created: Date.now() });
      session.setItem(prefix + ":login", login);
      if (session.getItem(prefix + ":login") !== login) throw new Error("browser sign-in storage unavailable");
      const params = new URLSearchParams({ client_id: client.client_id, redirect_uri: redirect, response_type: "code", scope: "tasks:read tasks:write offline_access", resource: origin, state, code_challenge: challenge, code_challenge_method: "S256" });
      return origin + "/authorize?" + params;
    }
    async function callback(params) {
      const login = JSON.parse(session.getItem(prefix + ":login") || "null");
      session.removeItem(prefix + ":login");
      if (!login || login.state !== params.get("state") || login.redirect !== redirect || Date.now() - login.created > 600000) throw new Error("Sign-in expired or state mismatch. Please sign in again.");
      if (params.has("error")) throw new Error("Sign-in denied: " + params.get("error"));
      if (!params.get("code")) throw new Error("Missing sign-in code");
      requireLocks();
      await locks.request(prefix + ":auth", async () => {
        const tokens = await json("/oauth/token", { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: new URLSearchParams({ grant_type: "authorization_code", code: params.get("code"), client_id: login.client, redirect_uri: redirect, code_verifier: login.verifier, resource: origin }) });
        write(tokenKey, { ...tokens, client_id: login.client, expires: Date.now() + tokens.expires_in * 1000 });
      });
    }
    async function token(force = false, rejected) {
      requireLocks();
      return locks.request(prefix + ":auth", async () => {
        let tokens = read(tokenKey);
        if (!tokens?.access_token) throw new Error("Sign in to the service first.");
        if (tokens.refreshing) throw new Error("Sign-in refresh was interrupted. Sign in again; retained saves are safe.");
        if ((force && tokens.access_token === rejected) || Date.now() >= tokens.expires - 60000) {
          // Never replay a rotating refresh credential after an uncertain response.
          write(tokenKey, { ...tokens, refreshing: true });
          const next = await json("/oauth/token", { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: new URLSearchParams({ grant_type: "refresh_token", refresh_token: tokens.refresh_token, client_id: tokens.client_id, resource: origin }) });
          tokens = { ...next, client_id: tokens.client_id, expires: Date.now() + next.expires_in * 1000 };
          write(tokenKey, tokens);
        }
        return tokens.access_token;
      });
    }
    async function request(path, options = {}) {
      const access = await token();
      const send = bearer => json(path, { ...options, headers: { "Content-Type": "application/json", ...options.headers, Authorization: "Bearer " + bearer } });
      try { return await send(access); }
      catch (err) { if (err.status !== 401) throw err; return send(await token(true, access)); }
    }
    async function logout() {
      requireLocks();
      await locks.request(prefix + ":auth", async () => {
        const tokens = read(tokenKey);
        // Clear only after revocation is acknowledged; failures can be retried.
        if (tokens) await json("/oauth/revoke", { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: new URLSearchParams({ token: tokens.refresh_token || tokens.access_token, client_id: tokens.client_id }) });
        storage.removeItem(tokenKey);
      });
    }
    return { start, callback, request, logout, connected: () => !!read(tokenKey)?.access_token };
  }
  return { createAuth };
});
