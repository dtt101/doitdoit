"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { createServiceStore, toData, applyEvents } = require("./sync.js");
const origin = "https://service.example";
const task = (extra = {}) => ({ id: "a", title: "Task", status: "open", scheduled_date: "2000-01-01", revision: 1, position: "a", notes: "café\nDetails", ...extra });
function fixture(request, storage = new Map()) {
  const adapter = { getItem: k => storage.get(k) ?? null, setItem: (k,v) => storage.set(k,v), removeItem: k => storage.delete(k) };
  return { store: createServiceStore({ origin, request, storage: adapter, locks: null, uuid: () => "operation-1" }), storage, adapter };
}
test("snapshot pagination fixes its cursor, preserves past dates and excludes deleted rows", async () => {
  const urls = [];
  const a = fixture(async url => {
    urls.push(url);
    return { tasks: urls.length === 1 ? [task()] : [task({ id: "b", scheduled_date: null }), task({ id: "c", deleted_at: "2026-01-01" })], timezone: "Europe/London", snapshot_cursor: 7, next_cursor: urls.length === 1 ? "a" : null };
  });
  const result = await a.store.load();
  assert.equal(result.data["2000-01-01"][0].notes, "café\nDetails");
  assert.equal(result.data.Future[0].id, "b");
  assert.match(urls[1], /snapshot_cursor=7/);
  assert.equal(Object.values(result.data).flat().length, 2);
});
test("sorts only by service status, opaque position and ID", () => {
  assert.deepEqual(toData([task({ id: "done", status: "completed", position: "0" }), task({ id: "b", position: "b" }), task()])["2000-01-01"].map(t => t.id), ["a", "b", "done"]);
});
test("lost acknowledgements survive recreation and retry identical commands", async () => {
  const calls = [];
  const a = fixture(async (path, options) => { calls.push([path, options]); throw new Error("offline"); });
  await assert.rejects(a.store.mutate("PATCH", "/v1/tasks/a", { expected_revision: 1, changes: { status: "completed" } }), /offline/);
  assert.ok(a.store.pending());
  await assert.rejects(a.store.mutate("POST", "/v1/tasks", { title: "duplicate" }), /retained/);
  assert.throws(() => a.store.reviewRejected(), /may have succeeded/);
  const b = fixture(async (path, options) => { calls.push([path, options]); return { task: task() }; }, a.storage);
  await b.store.retry();
  assert.deepEqual(calls[1], calls[0]);
  assert.equal(b.store.pending(), null);
});
test("a rejected revision requires explicit review and keeps the original command", async () => {
  const a = fixture(async () => { throw Object.assign(new Error("Changed remotely"), { status: 409, code: "revision_conflict", current: task({ revision: 2 }) }); });
  await assert.rejects(a.store.mutate("DELETE", "/v1/tasks/a", { expected_revision: 1 }), /remotely/);
  await assert.rejects(a.store.retry(), /rejected/);
  a.store.reviewRejected();
  assert.equal(a.store.pending(), null);
  assert.equal(a.store.recovery().rejected[0].error.current.revision, 2);
});
test("storage failure prevents sending and failed recovery blocks discard", async () => {
  let calls = 0;
  const a = fixture(async () => { calls++; });
  a.adapter.setItem = () => { throw new Error("quota"); };
  await assert.rejects(a.store.mutate("POST", "/v1/tasks", { title: "Safe" }), /quota/);
  assert.equal(calls, 0);
});
test("malformed acknowledgement stays retained", async () => {
  const a = fixture(async () => ({}));
  await assert.rejects(a.store.mutate("POST", "/v1/tasks", { title: "Safe" }), /acknowledgement/);
  assert.ok(a.store.pending());
});

test("recovery write failure cannot discard a rejected save", async () => {
  const a = fixture(async () => { throw Object.assign(new Error("conflict"), { status: 409 }); });
  await assert.rejects(a.store.mutate("PATCH", "/v1/tasks/a", { expected_revision: 1, changes: { title: "Draft" } }));
  a.adapter.setItem = () => { throw new Error("quota"); };
  assert.throws(() => a.store.reviewRejected(), /quota/);
  assert.equal(a.store.pending().body.changes.title, "Draft");
});

test("a changed snapshot cursor is rejected instead of mixing pages", async () => {
  let calls = 0;
  const a = fixture(async () => ({ tasks: [task()], snapshot_cursor: ++calls, timezone: "Europe/London", next_cursor: calls === 1 ? "a" : null }));
  await assert.rejects(a.store.load(), /snapshot/);
});

test("recovery exports raw data when retained command JSON is corrupted", () => {
  const a = fixture(async () => ({}));
  a.storage.set("doitdoit:service:pending:" + origin, "{not valid json");
  const recovery = a.store.recovery();
  assert.ok(recovery);
  assert.equal(recovery.corruptedPending.raw, "{not valid json");
  assert.ok(recovery.corruptedPending.error);
});

test("changes pages incrementally and applyEvents updates tasks by revision", async () => {
  const urls = [];
  const a = fixture(async url => {
    urls.push(url);
    if (urls.length === 1) {
      return { events: [{ cursor: 8, after: task({ revision: 2, title: "Updated" }) }], cursor: 8, next_cursor: 8 };
    }
    return { events: [{ cursor: 9, after: task({ id: "b", title: "Added" }) }], cursor: 9, next_cursor: null };
  });
  const res = await a.store.changes(7);
  assert.equal(res.cursor, 9);
  assert.equal(res.events.length, 2);
  assert.match(urls[0], /\/v1\/changes\?after=7/);
  assert.match(urls[1], /\/v1\/changes\?after=8/);
  const updated = applyEvents([task({ revision: 1 })], res.events);
  assert.equal(updated.find(t => t.id === "a").title, "Updated");
  assert.equal(updated.find(t => t.id === "b").title, "Added");
});
