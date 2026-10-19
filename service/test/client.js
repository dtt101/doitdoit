// Test-only HTTP client. Identity is seeded as an opaque D1 credential, never
// passed through a trusted binding or request header bypass.
import { env } from "cloudflare:workers";
import { fetch as workerFetch } from "../build/worker/shim.mjs";
import {
  createExecutionContext,
  waitOnExecutionContext,
} from "cloudflare:test";
export async function request(path, init) {
  const ctx = createExecutionContext();
  const response = await workerFetch(
    new Request("http://localhost:8787" + path, init),
    env,
    ctx,
  );
  await waitOnExecutionContext(ctx);
  return response;
}
export async function digest(value) {
  return [
    ...new Uint8Array(
      await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value)),
    ),
  ]
    .map((n) => n.toString(16).padStart(2, "0"))
    .join("");
}
export class RustTasks {
  constructor(bindings, principal) {
    this.principal = principal;
  }
  async call(path, method = "GET", input) {
    const token = crypto.randomUUID();
    const grant = crypto.randomUUID();
    await env.DB.prepare(
      "INSERT INTO oauth_grants(id,owner,client,scope,resource) VALUES(?,?,?,?,?)",
    )
      .bind(
        grant,
        this.principal.ownerID,
        this.principal.clientID,
        this.principal.scopes.join(" "),
        "http://localhost:8787",
      )
      .run();
    await env.DB.prepare(
      "INSERT INTO oauth_credentials(digest,kind,grant_id,expires) VALUES(?,'access',?,?)",
    )
      .bind(await digest(token), grant, Date.now() / 1000 + 900)
      .run();
    const response = await request(path, {
      method,
      headers: {
        Authorization: "Bearer " + token,
        "Content-Type": "application/json",
      },
      ...(input === undefined ? {} : { body: JSON.stringify(input) }),
    });
    const result = await response.json();
    if (!response.ok) throw { status: response.status, ...result.error };
    return result;
  }
  async list() {
    return (await this.call("/v1/tasks")).tasks;
  }
  async get(id) {
    return (await this.call("/v1/tasks/" + id)).task;
  }
  async create(input) {
    return (await this.call("/v1/tasks", "POST", input)).task;
  }
  async update(id, input) {
    return (await this.call("/v1/tasks/" + id, "PATCH", input)).task;
  }
}
