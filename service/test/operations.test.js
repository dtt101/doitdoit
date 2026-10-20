import { env } from "cloudflare:workers";
import { createExecutionContext, waitOnExecutionContext } from "cloudflare:test";
import { expect, it } from "vitest";
import { scheduled } from "../build/worker/shim.mjs";
import { request } from "./client.js";

it("limits public registration before writing clients and isolates callers", async () => {
  const ip = crypto.randomUUID();
  for (let n = 0; n < 10; n++) {
    const response = await request("/oauth/register", { method: "POST", headers: {
      "CF-Connecting-IP": ip, "Content-Type": "application/json",
    }, body: "{}" });
    expect(response.status).toBe(400);
  }
  const denied = await request("/oauth/register", { method: "POST", headers: { "CF-Connecting-IP": ip } });
  expect(denied.status).toBe(429);
  expect((await denied.json()).error).toBe("rate_limited");
  expect((await request("/oauth/register", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" })).status).toBe(400);
  expect(await env.DB.prepare("SELECT count(*) AS n FROM oauth_clients").first("n")).toBe(0);
});

it("hourly cleanup expires login state but preserves live grants and replay evidence", async () => {
  const now = Date.now() / 1000;
  for (const [id, expiry] of [["expired", now - 1], ["live", now + 600]]) {
    await env.DB.prepare("INSERT INTO oauth_pending VALUES(?,'consent','browser','{}',?)").bind(id, expiry).run();
    await env.DB.prepare("INSERT INTO oauth_clients VALUES(?,'{}',?)").bind(id, expiry).run();
    await env.DB.prepare("INSERT INTO oauth_grants VALUES(?,'owner','client','tasks:read','http://localhost:8787',0)").bind(id).run();
    await env.DB.prepare("INSERT INTO oauth_credentials(digest,kind,grant_id,expires) VALUES(?,'access',?,?)").bind(id, id, expiry).run();
    await env.DB.prepare("INSERT INTO oauth_credentials(digest,kind,grant_id,expires) VALUES(?,'used_refresh',?,?)").bind(id + '-spent', id, now - 60).run();
  }
  const ctx = createExecutionContext();
  await scheduled({ cron: "17 * * * *", scheduledTime: Date.now(), type: "scheduled" }, env, ctx);
  await waitOnExecutionContext(ctx);
  for (const table of ["oauth_pending", "oauth_clients", "oauth_grants"]) {
    expect(await env.DB.prepare(`SELECT count(*) AS n FROM ${table}`).first("n")).toBe(1);
  }
  expect((await env.DB.prepare("SELECT digest FROM oauth_credentials ORDER BY digest").all()).results).toEqual([{digest: "live"}, {digest: "live-spent"}]);
});
