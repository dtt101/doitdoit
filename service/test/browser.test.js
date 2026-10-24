import { it, expect } from "vitest";
import { request } from "./client.js";
import { signIn } from "./auth-helper.js";
const browser = "http://localhost:8000";

it("permits only the configured browser origin and bounded preflights", async () => {
  for (const origin of [browser, "https://attacker.example", "null", browser + ".evil"]) {
    const response = await request("/v1/tasks", { method: "OPTIONS", headers: { Origin: origin, "Access-Control-Request-Method": "PATCH", "Access-Control-Request-Headers": "authorization,content-type" } });
    expect(response.status).toBe(origin === browser ? 204 : 403);
    expect(response.headers.get("Access-Control-Allow-Origin")).toBe(origin === browser ? browser : null);
    expect(response.headers.has("Access-Control-Allow-Credentials")).toBe(false);
  }
  for (const path of ["/authorize", "/callback", "/mcp", "/v1/imports"]) {
    expect((await request(path, { headers: { Origin: browser } })).status).toBe(403);
  }
  const denied = await request("/v1/board", { headers: { Origin: browser } });
  expect(denied.status).toBe(401);
  expect(denied.headers.get("Access-Control-Allow-Origin")).toBe(browser);
  for (const extra of [{ "Access-Control-Request-Method": "PUT" }, { "Access-Control-Request-Headers": "cookie" }]) {
    expect((await request("/v1/tasks", { method: "OPTIONS", headers: { Origin: browser, "Access-Control-Request-Method": "POST", ...extra } })).status).toBe(400);
  }
});

it("allows authenticated browser reads and commands without weakening revisions", async () => {
  const signed = await signIn();
  const headers = { Origin: browser, Authorization: "Bearer " + signed.token, "Content-Type": "application/json" };
  const created = await request("/v1/tasks", { method: "POST", headers, body: JSON.stringify({ operation_id: crypto.randomUUID(), title: "Browser task", scheduled_date: "2000-01-01" }) });
  expect(created.status).toBe(201);
  expect(created.headers.get("Access-Control-Allow-Origin")).toBe(browser);
  const task = (await created.json()).task;
  expect((await request("/v1/board?limit=100", { headers })).status).toBe(200);
  const conflict = await request("/v1/tasks/" + task.id, { method: "PATCH", headers, body: JSON.stringify({ operation_id: crypto.randomUUID(), expected_revision: 99, changes: { title: "Stale" } }) });
  expect(conflict.status).toBe(409);
  expect(conflict.headers.get("Access-Control-Allow-Origin")).toBe(browser);
});

it("allows browser OAuth registration, refresh and revocation without cookies", async () => {
  const registered = await request("/oauth/register", { method: "POST", headers: { Origin: browser, "Content-Type": "application/json" }, body: JSON.stringify({ client_name: "Web test", redirect_uris: [browser + "/"], token_endpoint_auth_method: "none" }) });
  expect(registered.status).toBe(201);
  expect(registered.headers.get("Access-Control-Allow-Origin")).toBe(browser);
  const signed = await signIn();
  const headers = { Origin: browser, "Content-Type": "application/x-www-form-urlencoded" };
  const refreshed = await request("/oauth/token", { method: "POST", headers, body: new URLSearchParams({ grant_type: "refresh_token", client_id: signed.login.client.client_id, refresh_token: signed.refreshToken, resource: "http://localhost:8787" }) });
  expect(refreshed.status).toBe(200);
  expect(refreshed.headers.get("Access-Control-Allow-Origin")).toBe(browser);
  const next = await refreshed.json();
  const revoked = await request("/oauth/revoke", { method: "POST", headers, body: new URLSearchParams({ token: next.refresh_token, client_id: signed.login.client.client_id }) });
  expect(revoked.status).toBe(200);
  expect(revoked.headers.has("Access-Control-Allow-Credentials")).toBe(false);
  expect((await request("/v1/board", { headers: { Origin: browser, Authorization: "Bearer " + next.access_token } })).status).toBe(401);
});
