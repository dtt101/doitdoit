"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const Sync = require("./sync.js");

function response(status, body, headers = {}) {
  return {
    status,
    ok: status >= 200 && status < 300,
    headers: { get: (name) => headers[name] || null },
    text: async () => typeof body === "string" ? body : JSON.stringify(body),
    json: async () => typeof body === "string" ? JSON.parse(body) : body,
  };
}

test("revision upload sends update mode and returns the new revision", async () => {
  let options;
  const fetchImpl = async (_url, opts) => { options = opts; return response(200, { rev: "new" }); };
  const result = await Sync.uploadOnce(fetchImpl, "token", "/tasks.json", { Future: [] }, "old");
  assert.equal(result.rev, "new");
  assert.match(options.headers["Dropbox-API-Arg"], /update/);
  assert.match(options.headers["Dropbox-API-Arg"], /old/);
});

test("upload conflicts are surfaced without replacing local data", async () => {
  const local = { Future: [{ id: "1", title: "local" }] };
  const fetchImpl = async () => response(409, { error_summary: "conflict" });
  await assert.rejects(
    Sync.uploadOnce(fetchImpl, "token", "/tasks.json", local, "stale"),
    (error) => error.conflict === true,
  );
  const recovery = Sync.recoverySnapshot(local, "/tasks.json", new Date("2026-08-26T12:00:00Z"));
  local.Future[0].title = "changed later";
  assert.equal(recovery.data.Future[0].title, "local");
});

test("download handles missing files and invalid JSON", async () => {
  assert.deepEqual(await Sync.downloadOnce(async () => response(409, {}), "t", "/x"), { data: {}, rev: null });
  await assert.rejects(
    Sync.downloadOnce(async () => response(200, "{broken", { "Dropbox-API-Result": "{}" }), "t", "/x"),
    /not valid JSON/,
  );
});
