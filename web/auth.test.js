"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { webcrypto } = require("node:crypto");
const { createAuth } = require("./auth.js");
const origin = "https://service.example", redirect = "https://web.example/app/";
const tokenKey = "doitdoit:service:" + origin + ":tokens";
function fixture(fetchImpl) {
  const values = new Map(), login = new Map();
  const storageFor = map => ({ getItem: key => map.get(key) ?? null, setItem: (key, value) => map.set(key, value), removeItem: key => map.delete(key) });
  let tail = Promise.resolve();
  const locks = { request(key, fn) { const next = tail.then(fn); tail = next.catch(() => {}); return next; } };
  const auth = createAuth({ origin, redirect, storage: storageFor(values), session: storageFor(login), fetchImpl, locks, cryptoImpl: webcrypto });
  return { auth, values, login };
}
const response = (body, status = 200) => ({ ok: status < 300, status, json: async () => body });
test("uses S256, resource binding, session state and exact redirect", async () => {
  const requests = [];
  const a = fixture(async (url, options) => { requests.push([url, options]); return response(url.endsWith("register") ? { client_id: "client" } : { access_token: "access", refresh_token: "refresh", expires_in: 900 }); });
  const url = new URL(await a.auth.start());
  assert.equal(url.searchParams.get("code_challenge_method"), "S256");
  assert.equal(url.searchParams.get("redirect_uri"), redirect);
  assert.equal(url.searchParams.get("resource"), origin);
  assert.equal(url.searchParams.get("state").length, 43);
  await a.auth.callback(new URLSearchParams({ code: "code", state: url.searchParams.get("state") }));
  assert.equal(a.auth.connected(), true);
  assert.equal(requests[1][1].body.get("code_verifier").length, 43);
  assert.equal(requests[1][1].body.get("resource"), origin);
  assert.equal(requests[1][1].credentials, "omit");
});
test("rejects forged state before sending a code", async () => {
  let calls = 0;
  const a = fixture(async () => { calls++; return response({ client_id: "client" }); });
  await a.auth.start();
  await assert.rejects(a.auth.callback(new URLSearchParams({ code: "code", state: "wrong" })), /state mismatch/);
  assert.equal(calls, 1);
  assert.equal(a.auth.connected(), false);
});
test("concurrent requests rotate refresh only once and reread the stored credential", async () => {
  let refreshes = 0;
  const a = fixture(async url => {
    if (url.endsWith("token")) { refreshes++; return response({ access_token: "new", refresh_token: "next", expires_in: 900 }); }
    return response({ tasks: [] });
  });
  a.values.set(tokenKey, JSON.stringify({ access_token: "old", refresh_token: "refresh", client_id: "client", expires: 0 }));
  await Promise.all([a.auth.request("/v1/board"), a.auth.request("/v1/board")]);
  assert.equal(refreshes, 1);
  assert.equal(JSON.parse(a.values.get(tokenKey)).refresh_token, "next");
});
test("lost rotating refresh response requires reauthentication instead of replay", async () => {
  let calls = 0;
  const a = fixture(async () => { calls++; throw new Error("lost response"); });
  a.values.set(tokenKey, JSON.stringify({ access_token: "old", refresh_token: "refresh", client_id: "client", expires: 0 }));
  await assert.rejects(a.auth.request("/v1/board"), /lost response/);
  await assert.rejects(a.auth.request("/v1/board"), /interrupted/);
  assert.equal(calls, 1);
});
test("401 retries a task command with the identical body after refresh", async () => {
  const calls = [];
  const a = fixture(async (url, options) => {
    calls.push([url, options]);
    if (url.endsWith("token")) return response({ access_token: "new", refresh_token: "next", expires_in: 900 });
    return response({}, calls.length === 1 ? 401 : 200);
  });
  a.values.set(tokenKey, JSON.stringify({ access_token: "old", refresh_token: "refresh", client_id: "client", expires: Date.now() + 3600000 }));
  await a.auth.request("/v1/tasks", { method: "POST", body: '{"operation_id":"same","title":"Task"}' });
  assert.equal(calls[0][1].body, calls[2][1].body);
  assert.equal(calls[2][1].headers.Authorization, "Bearer new");
});
test("logout revokes before removing local credentials", async () => {
  const a = fixture(async () => response({}));
  a.values.set(tokenKey, JSON.stringify({ access_token: "a", refresh_token: "r", client_id: "client" }));
  await a.auth.logout();
  assert.equal(a.auth.connected(), false);
});
