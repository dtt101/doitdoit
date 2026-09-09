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
  const fetchImpl = async (_url, opts) => { options = opts; return response(200, { rev: "bbbbbbbbb" }); };
  const result = await Sync.uploadOnce(fetchImpl, "token", "/tasks.json", { Future: [] }, "aaaaaaaaa");
  assert.equal(result.rev, "bbbbbbbbb");
  assert.match(options.headers["Dropbox-API-Arg"], /update/);
  assert.match(options.headers["Dropbox-API-Arg"], /aaaaaaaaa/);
});

test("upload conflicts are surfaced without replacing local data", async () => {
  const local = { Future: [{ id: "1", title: "local" }] };
  const fetchImpl = async () => response(409, { error_summary: "conflict" });
  await assert.rejects(
    Sync.uploadOnce(fetchImpl, "token", "/tasks.json", local, "aaaaaaaaa"),
    (error) => error.conflict === true,
  );
  const recovery = Sync.recoverySnapshot(local, "/tasks.json", new Date("2026-08-26T12:00:00Z"));
  local.Future[0].title = "changed later";
  assert.equal(recovery.data.Future[0].title, "local");
});

test("download handles missing files and invalid JSON", async () => {
  assert.deepEqual(await Sync.downloadOnce(async () => response(409, { error: { ".tag": "path", path: { ".tag": "not_found" } } }), "t", "/x"), { data: {}, rev: null });
  await assert.rejects(
    Sync.downloadOnce(async () => response(200, "{broken", { "Dropbox-API-Result": '{"rev":"111111111"}' }), "t", "/x"),
    /not valid JSON/,
  );
});

test("JSON store refreshes authentication while preserving path and revision", async () => {
  let token = "aaaaaaaaa";
  let refreshes = 0;
  const calls = [];
  const store = Sync.createJSONStore({
    path: "/tasks.json", getToken: () => token, ensureToken: async () => {},
    refreshAccessToken: async () => { token = "bbbbbbbbb"; refreshes++; },
    fetchImpl: async (url, options) => {
      calls.push({ url, options });
      if (options.headers.Authorization === "Bearer aaaaaaaaa") return response(401, "");
      if (url.endsWith("download")) return response(200, { Future: [] }, { "Dropbox-API-Result": '{"rev":"111111111"}' });
      return response(200, { rev: "222222222" });
    },
  });
  const snapshot = await store.load();
  assert.deepEqual(snapshot, { data: { Future: [] }, rev: "111111111" });
  token = "aaaaaaaaa";
  assert.equal(await store.save(snapshot.data, snapshot.rev), "222222222");
  assert.equal(refreshes, 2);
  for (const { url, options } of calls) {
    const args = JSON.parse(options.headers["Dropbox-API-Arg"]);
    assert.equal(args.path, "/tasks.json");
    if (url.endsWith("upload")) assert.deepEqual(args.mode, { ".tag": "update", update: "111111111" });
  }
});

test("JSON store propagates conflicts and creates a detached recovery snapshot", async () => {
  const store = Sync.createJSONStore({
    path: "/tasks.json", getToken: () => "token", ensureToken: async () => {},
    refreshAccessToken: async () => assert.fail("unexpected refresh"),
    fetchImpl: async () => response(409, { error_summary: "conflict" }),
  });
  const data = { Future: [{ id: "a", title: "local" }] };
  await assert.rejects(store.save(data, "aaaaaaaaa"), error => error.conflict);
  const snapshot = store.recovery(data, new Date("2026-09-07T12:00:00Z"));
  data.Future[0].title = "changed";
  assert.equal(snapshot.data.Future[0].title, "local");
  assert.equal(snapshot.filePath, "/tasks.json");
});

for (const meta of [{}, null, { rev: null }, { rev: 123 }, { rev: "" }, { rev: "bad" }, { rev: "12345678g" }]) {
  test(`invalid revision metadata is rejected on both reads and writes: ${JSON.stringify(meta)}`, async () => {
    await assert.rejects(Sync.downloadOnce(async () => response(200, {}, {
      "Dropbox-API-Result": JSON.stringify(meta),
    }), "t", "/x"), /revision metadata/);
    await assert.rejects(Sync.uploadOnce(async () => response(200, meta), "t", "/x", {}, "aaaaaaaaa"), /revision metadata/);
  });
}

test("only structured path/not_found is absence, all other 409 responses fail", async () => {
  for (const body of [{}, { error_summary: "path/not_found/..." },
    { error: { ".tag": "path", path: { ".tag": "not_file" } } },
    { error: { ".tag": "path", path: { ".tag": "restricted_content" } } }, "invalid JSON"]) {
    await assert.rejects(Sync.downloadOnce(async () => response(409, body), "t", "/x"), /download 409/);
  }
  for (const metadata of ["{broken", "[]", "{}"]) {
    await assert.rejects(Sync.downloadOnce(async () => response(200, {}, {
      "Dropbox-API-Result": metadata,
    }), "t", "/x"));
  }
});

test("confirmed missing file uses strict create-only mode and a concurrent creator conflicts", async () => {
  const calls = [];
  const store = Sync.createJSONStore({
    path: "/x", ensureToken: async () => {}, refreshAccessToken: async () => {}, getToken: () => "t",
    fetchImpl: async (url, options) => {
      calls.push(options);
      if (url.endsWith("download")) return response(409, { error: { ".tag": "path", path: { ".tag": "not_found" } } });
      const args = JSON.parse(options.headers["Dropbox-API-Arg"]);
      assert.equal(args.mode, "add"); assert.equal(args.autorename, false); assert.equal(args.strict_conflict, true);
      return response(409, { error: { ".tag": "path", path: { ".tag": "conflict" } } });
    },
  });
  await assert.rejects(store.save({}, null), /before creating/); assert.equal(calls.length, 0);
  const loaded = await store.load(); assert.equal(loaded.rev, null);
  await assert.rejects(store.save({ Future: [] }, loaded.rev), err => err.conflict);
});

test("revision updates are strict and invalid write expectations never reach the network", async () => {
  const fetchImpl = async (_url, options) => {
    const args = JSON.parse(options.headers["Dropbox-API-Arg"]);
    assert.deepEqual(args.mode, { ".tag": "update", update: "aaaaaaaaa" });
    assert.equal(args.strict_conflict, true); assert.equal(args.autorename, false);
    return response(200, { rev: "bbbbbbbbb" });
  };
  await Sync.uploadOnce(fetchImpl, "t", "/x", {}, "aaaaaaaaa");
  for (const rev of [undefined, "", 12, "malformed"]) {
    await assert.rejects(Sync.uploadOnce(() => assert.fail("unexpected upload"), "t", "/x", {}, rev), /revision/);
  }
});

test("authentication retries are bounded and keep the original detached upload", async () => {
  let release;
  const wait = new Promise(resolve => { release = resolve; });
  let calls = 0, refreshes = 0;
  const data = { Future: [{ title: "A" }] };
  const store = Sync.createJSONStore({
    path: "/x", ensureToken: () => wait, getToken: () => "t",
    refreshAccessToken: async () => { refreshes++; },
    fetchImpl: async (_url, options) => {
      calls++;
      assert.equal(JSON.parse(options.body).Future[0].title, "A");
      return response(401, {});
    },
  });
  const saving = store.save(data, "aaaaaaaaa"); data.Future[0].title = "B"; release();
  await assert.rejects(saving, /authentication failed/);
  assert.equal(calls, 2); assert.equal(refreshes, 1);
});

test("stale overlapping download cannot revoke confirmed creation context", async () => {
  const pending = [];
  const store = Sync.createJSONStore({
    path: "/x", ensureToken: async () => {}, getToken: () => "t", refreshAccessToken: async () => {},
    fetchImpl: (url, options) => {
      if (url.endsWith("upload")) {
        assert.equal(JSON.parse(options.headers["Dropbox-API-Arg"]).mode, "add");
        return Promise.resolve(response(200, { rev: "ccccccccc" }));
      }
      return new Promise(resolve => pending.push(resolve));
    },
  });
  const first = store.load(), latest = store.load();
  await new Promise(resolve => setImmediate(resolve));
  pending[1](response(409, { error: { ".tag": "path", path: { ".tag": "not_found" } } }));
  const loaded = await latest;
  pending[0](response(200, {}, { "Dropbox-API-Result": '{"rev":"aaaaaaaaa"}' }));
  await first;
  assert.equal(await store.save(loaded.data, loaded.rev), "ccccccccc");
});
