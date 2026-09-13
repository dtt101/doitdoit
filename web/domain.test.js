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

test("rollover keeps completed history and moves incomplete tasks", () => {
  const data = {
    "2026-08-25": [
      { id: "open", title: "Open", completed: false },
      { id: "done", title: "Done", completed: true },
    ],
    "2026-08-26": [
      { id: "today-done", title: "Done today", completed: true },
    ],
  };
  assert.equal(Domain.rollOverIncompleteTasks(data, now), true);
  assert.equal(data["2026-08-26"][0].id, "open");
  assert.equal(data["2026-08-26"][1].id, "today-done");
  assert.equal(data["2026-08-25"][0].id, "done");
});

test("retention defaults to forever and prunes only after an explicit choice", () => {
  const forever = { "2026-01-01": [{ id: "done", completed: true }] };
  assert.equal(Domain.pruneOldTasks(forever, 0, now), false);
  assert.ok(forever["2026-01-01"]);
  const pruned = { "2026-08-01": [{ id: "done", completed: true }], Future: [{ id: "f", completed: true }] };
  assert.equal(Domain.pruneOldTasks(pruned, 5, now), true);
  assert.equal(pruned["2026-08-01"], undefined);
  assert.deepEqual(pruned.Future, []);
});

test("legacy Future migration and add targeting are independent of the visible window", () => {
  const data = {
    "2026-08-27": [{ id: "done", completed: true }],
    Future: [
      { id: "soon", due_date: "2026-08-27" },
      { id: "later", due_date: "2026-09-20" },
    ],
  };
  Domain.migrateDatedFutureTasks(data);
  assert.equal(data["2026-08-27"][0].id, "soon");
  assert.equal(data["2026-08-27"][1].id, "done");
  assert.deepEqual(data.Future, []);
  assert.equal(data["2026-09-20"][0].id, "later");
  assert.deepEqual(
    Domain.parseAddInput("!future write postcard", { kind: "today" }, now),
    { title: "write postcard", key: "Future", due: "" },
  );
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

test("notes survive web edits, moves, rollover and JSON round trips", () => {
  const notes = "Words café\nhttps://example.com";
  const data = { Future: [{ id: "notes", title: "Original", notes, completed: false }] };
  Domain.editTask(data, "Future", "notes", "Edited", { key: "2026-08-25", due: "2026-08-25" });
  Domain.rollOverIncompleteTasks(data, now);
  Domain.toggleTask(data, "2026-08-26", "notes");
  Domain.moveTask(data, "2026-08-26", "notes", "Future", 0);
  assert.equal(JSON.parse(JSON.stringify(data)).Future[0].notes, notes);
});


test("migration preserves fields and ordering before normal lifecycle, and is idempotent", () => {
  const legacy = { id: "far", due_date: "2099-01-01", notes: "café\nDetails", completed: false, created_at: "2026-01-01T12:00:00Z" };
  const data = { "2099-01-01": [{ id: "existing" }, { id: "done", completed: true }], Future: [
    { id: "idea" }, legacy, { id: "bad", due_date: "invalid" },
    { id: "past-open", due_date: "2026-08-25" },
    { id: "past-done", due_date: "2026-08-25", completed: true },
  ] };
  assert.equal(Domain.migrateDatedFutureTasks(data), true);
  assert.deepEqual(data["2099-01-01"].map(t => t.id), ["existing", "far", "done"]);
  assert.deepEqual(data["2099-01-01"][1], legacy);
  assert.deepEqual(data.Future.map(t => t.id), ["idea", "bad"]);
  assert.equal(Domain.migrateDatedFutureTasks(data), false);
  Domain.rollOverIncompleteTasks(data, now);
  assert.deepEqual(data["2026-08-25"].map(t => t.id), ["past-done"]);
  assert.equal(data["2026-08-26"][0].due_date, "2026-08-26");
  assert.deepEqual(Domain.storageTarget({ kind: "custom", date: "2099-01-01" }, now), { key: "2099-01-01", due: "2099-01-01" });
});
