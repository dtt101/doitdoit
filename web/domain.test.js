"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const Domain = require("./domain.js");

const now = new Date(2026, 7, 26, 12);

test("task operations preserve completion ordering and scheduling without UI state", () => {
  const data = { Future: [{ id: "done", completed: true }] };
  Domain.insertTask(data, "Future", { id: "a", title: "Original", completed: false });
  assert.deepEqual(data.Future.map(t => t.id), ["a", "done"]);
  assert.equal(Domain.toggleTask(data, "Future", "a"), true);
  assert.deepEqual(data.Future.map(t => t.id), ["done", "a"]);
  Domain.toggleTask(data, "Future", "a");
  assert.deepEqual(data.Future.map(t => t.id), ["a", "done"]);
  assert.equal(Domain.editTask(data, "Future", "a", "Edited", { key: "2026-09-08", due: "2026-09-08" }), true);
  assert.equal(data["2026-09-08"][0].title, "Edited");
  assert.equal(data["2026-09-08"][0].due_date, "2026-09-08");
  assert.equal(Domain.moveTask(data, "2026-09-08", "a", "Future", 99), true);
  assert.equal(data["2026-09-08"], undefined);
  assert.deepEqual(data.Future.map(t => t.id), ["a", "done"]);
  assert.equal(Object.hasOwn(data.Future[0], "due_date"), false);
  Domain.deleteTask(data, "Future", "a");
  Domain.deleteTask(data, "Future", "done");
  assert.deepEqual(data.Future, []);
  assert.equal(Domain.moveTask(data, "Future", "missing", "2026-09-08", 0), false);
});

test("service capture preserves explicit past dates and undated tasks", () => {
  assert.deepEqual(Domain.storageTarget({ kind: "custom", date: "2026-08-25" }, now), { key: "2026-08-25", due: "2026-08-25" });
  assert.deepEqual(Domain.parseAddInput("!future write postcard", { kind: "today" }, now), { title: "write postcard", key: "Future", due: "" });
  assert.deepEqual(Domain.storageTarget({ kind: "tomorrow" }, now), { key: "2026-08-27", due: "2026-08-27" });
});

test("completion grouping is stable and reports whether it repaired data", () => {
  const data = { Future: [
    { id: "open-1", completed: false },
    { id: "done-1", completed: true },
    { id: "open-2", completed: false },
    { id: "done-2", completed: true },
  ] };
  assert.equal(Domain.groupTasksByCompletion(data), true);
  assert.deepEqual(data.Future.map((task) => task.id), ["open-1", "open-2", "done-1", "done-2"]);
  assert.equal(Domain.groupTasksByCompletion(data), false);
});

test("invalid dates and titles are rejected", () => {
  assert.ok(Domain.parseAddInput("", { kind: "today" }, now).error);
  assert.ok(Domain.storageTarget({ kind: "custom", date: "2026-02-30" }, now).error);
});

test("notes and original dates survive drag previews", () => {
  const notes = "Words café\nhttps://example.com";
  const data = { "2026-08-25": [{ id: "notes", title: "Original", notes, completed: false }] };
  Domain.toggleTask(data, "2026-08-25", "notes");
  assert.equal(data["2026-08-25"][0].due_date, undefined);
  Domain.moveTask(data, "2026-08-25", "notes", "Future", 0);
  assert.equal(data.Future[0].notes, notes);
});
