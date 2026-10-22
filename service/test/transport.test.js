import { env } from "cloudflare:workers";
import { describe, expect, it } from "vitest";
import { request, signIn, startLogin, origin } from "./auth-helper.js";

const auth = (token) => ({
  Authorization: "Bearer " + token,
  "Content-Type": "application/json",
});
async function rpc(token, method, params = {}) {
  return request("/mcp", {
    method: "POST",
    headers: { ...auth(token), Accept: "application/json, text/event-stream" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }),
  });
}
describe("authenticated API and MCP", () => {
  it("challenges unauthenticated and invalid tokens without exposing data", async () => {
    for (const path of ["/v1/tasks", "/mcp"]) {
      expect((await request(path)).status).toBe(401);
      expect((await request(path, { headers: auth("invalid") })).status).toBe(
        401,
      );
    }
    const metadata = await request("/.well-known/oauth-protected-resource");
    expect(metadata.status).toBe(200);
    expect(await metadata.json()).toMatchObject({ resource: origin });
    const as = await request("/.well-known/oauth-authorization-server");
    expect(await as.json()).toMatchObject({
      code_challenge_methods_supported: ["S256"],
    });
  });

  it("requires browser-bound consent and safely renders client-supplied names", async () => {
    const login = await startLogin(undefined, '<script>alert("x")</script>');
    expect(login.html).not.toContain("<script>");
    expect(login.html).toContain("&#60;script&#62;");
    expect(login.consent.headers.get("X-Frame-Options")).toBe("DENY");
    const forged = await request("/authorize", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ handle: login.handle, decision: "approve" }),
    });
    expect(forged.status).toBe(400);
  });

  it("rejects sign-in by an account outside the owner allowlist", async () => {
    const signed = await signIn(undefined, 999);
    expect(signed.callback.status).toBe(403);
    expect(signed.token).toBe("");
    expect(
      await env.DB.prepare("SELECT count(*) AS n FROM owners").first("n"),
    ).toBe(0);
  });

  it("logs in, creates through MCP, edits through HTTP and returns a structured stale-date conflict", async () => {
    const signed = await signIn();
    expect(signed.token).not.toBe("");
    const initialized = await rpc(signed.token, "initialize", {
      protocolVersion: "2025-11-25",
      capabilities: {},
      clientInfo: { name: "test", version: "1" },
    });
    expect(initialized.status).toBe(200);
    const operationID = crypto.randomUUID();
    const createdResponse = await rpc(signed.token, "tools/call", {
      name: "create_task",
      arguments: {
        operation_id: operationID,
        title: "☕ Coffee",
        scheduled_date: "2020-01-01",
      },
    });
    expect(createdResponse.status).toBe(200);
    const created = await createdResponse.json();
    const task = created.result.structuredContent.task;
    expect(task).toMatchObject({ scheduled_date: "2020-01-01", revision: 1 });
    const updated = await request("/v1/tasks/" + task.id, {
      method: "PATCH",
      headers: auth(signed.token),
      body: JSON.stringify({
        operation_id: crypto.randomUUID(),
        expected_revision: 1,
        changes: { scheduled_date: "2030-01-01" },
      }),
    });
    expect(updated.status).toBe(200);
    const conflict = await rpc(signed.token, "tools/call", {
      name: "update_task",
      arguments: {
        id: task.id,
        operation_id: crypto.randomUUID(),
        expected_revision: 1,
        changes: { scheduled_date: null },
      },
    });
    const body = await conflict.json();
    expect(body.result.isError).toBe(true);
    expect(body.result.structuredContent.error).toMatchObject({
      code: "revision_conflict",
      current: { scheduled_date: "2030-01-01", revision: 2 },
    });
    // A timeout/retry across transports returns the original acknowledgement.
    const retried = await request("/v1/tasks", {
      method: "POST",
      headers: auth(signed.token),
      body: JSON.stringify({
        operation_id: operationID,
        scheduled_date: "2020-01-01",
        title: "☕ Coffee",
      }),
    });
    expect((await retried.json()).task).toEqual(task);
    const list = await rpc(signed.token, "tools/call", {
      name: "list_tasks",
      arguments: {},
    });
    expect((await list.json()).result.structuredContent.tasks).toHaveLength(1);
  });

  it("denies writes to read-only grants through both transports", async () => {
    const signed = await signIn("tasks:read");
    expect(
      (await request("/v1/tasks", { headers: auth(signed.token) })).status,
    ).toBe(200);
    const body = { operation_id: crypto.randomUUID(), title: "No" };
    expect(
      (
        await request("/v1/tasks", {
          method: "POST",
          headers: auth(signed.token),
          body: JSON.stringify(body),
        })
      ).status,
    ).toBe(403);
    const mcp = await rpc(signed.token, "tools/call", {
      name: "create_task",
      arguments: body,
    });
    expect(mcp.status).toBe(403);
    expect(mcp.headers.get("WWW-Authenticate")).toContain("tasks:write");
  });

  it("refreshes tokens and rejects invalid redirect/PKCE and cross-origin requests", async () => {
    const signed = await signIn();
    const refreshed = await request("/oauth/token", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "refresh_token",
        refresh_token: signed.refreshToken,
        client_id: signed.login.client.client_id,
        resource: origin,
      }),
    });
    expect(refreshed.status).toBe(200);
    const badRedirect = new URLSearchParams(signed.login.params);
    badRedirect.set("redirect_uri", "https://attacker.example/callback");
    const denied = await request("/authorize?" + badRedirect);
    expect(denied.status).toBe(400);
    expect(denied.headers.has("Location")).toBe(false);
    const noPKCE = new URLSearchParams(signed.login.params);
    noPKCE.delete("code_challenge");
    const missingChallenge = await request("/authorize?" + noPKCE);
    expect(missingChallenge.status).toBe(302);
    expect(
      new URL(missingChallenge.headers.get("Location")).searchParams.get(
        "error",
      ),
    ).toBe("invalid_request");
    expect(
      (
        await request("/v1/tasks", {
          headers: {
            ...auth(signed.token),
            Origin: "https://attacker.example",
          },
        })
      ).status,
    ).toBe(403);
  });

  it("requires read scope even when a token grants write scope", async () => {
    const signed = await signIn("tasks:write");
    const denied = await request("/v1/tasks", { headers: auth(signed.token) });
    expect(denied.status).toBe(403);
    expect(denied.headers.get("WWW-Authenticate")).toContain("tasks:read");
  });

  it("rejects expired and revoked access tokens", async () => {
    const signed = await signIn();
    await env.DB.prepare(
      "UPDATE oauth_credentials SET expires=0 WHERE kind='access'",
    ).run();
    expect(
      (await request("/v1/tasks", { headers: auth(signed.token) })).status,
    ).toBe(401);
    const active = await signIn();
    const revoked = await request("/oauth/revoke", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        token: active.token,
        client_id: active.login.client.client_id,
      }),
    });
    expect(revoked.status).toBe(200);
    expect(
      (await request("/v1/tasks", { headers: auth(active.token) })).status,
    ).toBe(401);
  });

  it("validates HTTP bodies before any task mutation", async () => {
    const signed = await signIn();
    for (const body of [
      '{"title":',
      JSON.stringify({
        operation_id: crypto.randomUUID(),
        title: "Task",
        scheduled_date: "2026-02-30",
      }),
    ]) {
      expect(
        (
          await request("/v1/tasks", {
            method: "POST",
            headers: auth(signed.token),
            body,
          })
        ).status,
      ).toBe(400);
    }
    expect(
      (
        await request("/v1/tasks", {
          method: "POST",
          headers: auth(signed.token),
          body: " ".repeat(65_537),
        })
      ).status,
    ).toBe(413);
    expect(
      await env.DB.prepare("SELECT count(*) AS n FROM tasks").first("n"),
    ).toBe(0);
  });
});

it("serves the generated Rust entrypoint through workerd dispatch", async () => {
  const { SELF } = await import("cloudflare:test");
  expect((await SELF.fetch(origin + "/health")).status).toBe(200);
  const denied = await SELF.fetch(origin + "/v1/tasks");
  expect(denied.status).toBe(401);
  expect(denied.headers.get("WWW-Authenticate")).toContain("resource_metadata");
});

it("rotates refresh credentials atomically and revokes a replayed grant", async () => {
  const signed = await signIn();
  const refresh = () =>
    request("/oauth/token", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "refresh_token",
        refresh_token: signed.refreshToken,
        client_id: signed.login.client.client_id,
        resource: origin,
      }),
    });
  const results = await Promise.all([refresh(), refresh()]);
  expect(results.map((r) => r.status).sort()).toEqual([200, 400]);
  expect(
    await env.DB.prepare(
      "SELECT count(*) AS n FROM oauth_credentials WHERE kind='refresh'",
    ).first("n"),
  ).toBe(1);
  const next = await results.find((r) => r.status === 200).json();
  expect(next.refresh_token).not.toBe(signed.refreshToken);
  // Replay detection invalidates the whole grant, including successor tokens.
  expect((await refresh()).status).toBe(400);
  expect(
    (await request("/v1/tasks", { headers: auth(next.access_token) })).status,
  ).toBe(401);
});

it("binds refresh to its client, audience and scopes without consuming valid credentials", async () => {
  const signed = await signIn();
  const exchange = (changes) =>
    request("/oauth/token", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "refresh_token",
        refresh_token: signed.refreshToken,
        client_id: signed.login.client.client_id,
        resource: origin,
        ...changes,
      }),
    });
  for (const change of [
    { client_id: "other" },
    { resource: "https://attacker.example" },
    { scope: "admin" },
  ]) {
    expect((await exchange(change)).status).toBe(400);
  }
  expect((await exchange({})).status).toBe(200);
  const stored = JSON.stringify(
    (await env.DB.prepare("SELECT * FROM oauth_credentials").all()).results,
  );
  expect(stored).not.toContain(signed.token);
  expect(stored).not.toContain(signed.refreshToken);
});

it("responds to MCP protocol errors and notifications without mutating tasks", async () => {
  const signed = await signIn();
  const headers = {
    ...auth(signed.token),
    Accept: "application/json, text/event-stream",
  };
  const send = (body) =>
    request("/mcp", {
      method: "POST",
      headers,
      body: typeof body === "string" ? body : JSON.stringify(body),
    });
  const malformed = await send("{");
  expect(malformed.status).toBe(400);
  expect((await malformed.json()).error.code).toBe(-32700);
  const batch = await send([]);
  expect((await batch.json()).error.code).toBe(-32600);
  const unknown = await rpc(signed.token, "nonexistent");
  expect((await unknown.json()).error.code).toBe(-32601);
  const initialized = await rpc(signed.token, "initialize", {
    protocolVersion: "2025-03-26",
    capabilities: {},
    clientInfo: { name: "test", version: "1" },
  });
  expect((await initialized.json()).result.protocolVersion).toBe("2025-03-26");
  const tools = await rpc(signed.token, "tools/list");
  expect((await tools.json()).result.tools).toHaveLength(11);
  expect(
    (await send({ jsonrpc: "2.0", method: "notifications/initialized" }))
      .status,
  ).toBe(202);
  expect(
    (
      await send({
        jsonrpc: "2.0",
        method: "tools/call",
        params: {
          name: "create_task",
          arguments: { operation_id: crypto.randomUUID(), title: "No" },
        },
      })
    ).status,
  ).toBe(202);
  expect(
    await env.DB.prepare("SELECT count(*) AS n FROM tasks").first("n"),
  ).toBe(0);
  expect((await request("/mcp", { headers: auth(signed.token) })).status).toBe(
    405,
  );
  const unsupported = await request("/mcp", {
    method: "POST",
    headers: { ...headers, "MCP-Protocol-Version": "bad" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "ping" }),
  });
  expect(unsupported.status).toBe(400);
});

it("validates code PKCE and redirect before one-time consumption, then detects replay", async () => {
  const { digest } = await import("./client.js");
  const signed = await signIn();
  const grant = await env.DB.prepare(
    "SELECT grant_id FROM oauth_credentials WHERE digest=?",
  )
    .bind(await digest(signed.token))
    .first("grant_id");
  const code = crypto.randomUUID();
  const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk";
  await env.DB.prepare(
    "INSERT INTO oauth_credentials(digest,kind,grant_id,expires,redirect,challenge) VALUES(?,'code',?,?,?,?)",
  )
    .bind(
      await digest(code),
      grant,
      Date.now() / 1000 + 300,
      "http://localhost:4242/callback",
      "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
    )
    .run();
  const exchange = (changes) =>
    request("/oauth/token", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "authorization_code",
        code,
        client_id: signed.login.client.client_id,
        resource: origin,
        redirect_uri: "http://localhost:4242/callback",
        code_verifier: verifier,
        ...changes,
      }),
    });
  for (const change of [
    { redirect_uri: "https://attacker.example/callback" },
    { code_verifier: "x".repeat(43) },
  ]) {
    expect((await exchange(change)).status).toBe(400);
    expect(
      await env.DB.prepare("SELECT kind FROM oauth_credentials WHERE digest=?")
        .bind(await digest(code))
        .first("kind"),
    ).toBe("code");
  }
  expect((await exchange({})).status).toBe(200);
  const replay=await exchange({});
  expect(replay.status).toBe(400);
  expect((await replay.json()).error).toBe('invalid_grant');
  expect(
    (await request("/v1/tasks", { headers: auth(signed.token) })).status,
  ).toBe(401);
});

it("binds upstream callbacks to the consenting browser and consumes state once", async () => {
  const login = await startLogin();
  const init = {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Cookie: login.cookie,
    },
    body: new URLSearchParams({ handle: login.handle, decision: "approve" }),
  };
  const approved = await request("/authorize", init);
  const state = new URL(approved.headers.get("Location")).searchParams.get(
    "state",
  );
  const callback =
    "/callback?" + new URLSearchParams({ state, error: "access_denied" });
  expect((await request(callback)).status).toBe(400);
  const denied = await request(callback, { headers: { Cookie: login.cookie } });
  expect(denied.status).toBe(302);
  expect(
    new URL(denied.headers.get("Location")).searchParams.get("error"),
  ).toBe("access_denied");
  expect(
    (await request(callback, { headers: { Cookie: login.cookie } })).status,
  ).toBe(400);
  expect((await request("/authorize", init)).status).toBe(400);
});

it("keeps browser consent same-origin and permits the full registered callback redirect chain", async () => {
  const login = await startLogin();
  expect(login.consent.headers.get("Referrer-Policy")).toBe("same-origin");
  expect(login.consent.headers.get("Content-Security-Policy")).toBe(
    "default-src 'none'; form-action 'self' https://github.com http://localhost:4242; frame-ancestors 'none'; base-uri 'none'",
  );
  const consent = (requestOrigin) => request("/authorize", {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Origin: requestOrigin,
      Cookie: login.cookie,
    },
    body: new URLSearchParams({ handle: login.handle, decision: "approve" }),
  });
  // A valid cookie/handle cannot bypass the origin check, including opaque origins.
  for (const requestOrigin of ["null", "https://attacker.example"]) {
    expect((await consent(requestOrigin)).status).toBe(403);
    for (const path of ["/v1/tasks", "/mcp"]) {
      expect((await request(path, {headers: {Origin: requestOrigin}})).status).toBe(403);
    }
  }
  const approved = await consent(origin);
  expect(approved.status).toBe(302);
  expect(new URL(approved.headers.get("Location")).origin).toBe("https://github.com");
  expect(approved.headers.get("Referrer-Policy")).toBe("no-referrer");
  expect((await consent(origin)).status).toBe(400);
});


it("restricts consent form redirects to the selected registered callback origin", async () => {
  for (const redirect of [
    "http://127.0.0.1:38295/callback/test",
    "https://client.example/callback?next=https://untrusted.example",
  ]) {
    const login = await startLogin(undefined, "Callback policy test", redirect);
    const policy = login.consent.headers.get("Content-Security-Policy");
    expect(policy).toBe(`default-src 'none'; form-action 'self' https://github.com ${new URL(redirect).origin}; frame-ancestors 'none'; base-uri 'none'`);
    const altered = new URLSearchParams(login.params);
    altered.set("redirect_uri", "https://unregistered.example/callback");
    const rejected = await request("/authorize?" + altered);
    expect(rejected.status).toBe(400);
    expect(rejected.headers.get("Content-Security-Policy")).toBe(
      "default-src 'none'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'",
    );
  }
});
