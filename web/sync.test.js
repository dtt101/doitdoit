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

test("JSON store refreshes authentication while preserving path and revision", async () => {
  let token = "old";
  let refreshes = 0;
  const calls = [];
  const store = Sync.createJSONStore({
    path: "/tasks.json", getToken: () => token, ensureToken: async () => {},
    refreshAccessToken: async () => { token = "new"; refreshes++; },
    fetchImpl: async (url, options) => {
      calls.push({ url, options });
      if (options.headers.Authorization === "Bearer old") return response(401, "");
      if (url.endsWith("download")) return response(200, { Future: [] }, { "Dropbox-API-Result": '{"rev":"r1"}' });
      return response(200, { rev: "r2" });
    },
  });
  const snapshot = await store.load();
  assert.deepEqual(snapshot, { data: { Future: [] }, rev: "r1" });
  token = "old";
  assert.equal(await store.save(snapshot.data, snapshot.rev), "r2");
  assert.equal(refreshes, 2);
  for (const { url, options } of calls) {
    const args = JSON.parse(options.headers["Dropbox-API-Arg"]);
    assert.equal(args.path, "/tasks.json");
    if (url.endsWith("upload")) assert.deepEqual(args.mode, { ".tag": "update", update: "r1" });
  }
});

test("JSON store propagates conflicts and creates a detached recovery snapshot", async () => {
  const store = Sync.createJSONStore({
    path: "/tasks.json", getToken: () => "token", ensureToken: async () => {},
    refreshAccessToken: async () => assert.fail("unexpected refresh"),
    fetchImpl: async () => response(409, { error_summary: "conflict" }),
  });
  const data = { Future: [{ id: "a", title: "local" }] };
  await assert.rejects(store.save(data, "stale"), error => error.conflict);
  const snapshot = store.recovery(data, new Date("2026-09-07T12:00:00Z"));
  data.Future[0].title = "changed";
  assert.equal(snapshot.data.Future[0].title, "local");
  assert.equal(snapshot.filePath, "/tasks.json");
});
