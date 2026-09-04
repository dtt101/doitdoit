"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const Domain = require("./domain.js");

const now = new Date(2026, 7, 26, 12);

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

test("future distribution and add targeting match the visible window", () => {
  const data = {
    "2026-08-27": [{ id: "done", completed: true }],
    Future: [
      { id: "soon", due_date: "2026-08-27" },
      { id: "later", due_date: "2026-09-20" },
    ],
  };
  Domain.distributeFutureTasks(data, 3, now);
  assert.equal(data["2026-08-27"][0].id, "soon");
  assert.equal(data["2026-08-27"][1].id, "done");
  assert.equal(data.Future[0].id, "later");
  assert.deepEqual(
    Domain.parseAddInput("!future write postcard", { kind: "today" }, 3, now),
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
  assert.ok(Domain.parseAddInput("", { kind: "today" }, 3, now).error);
  assert.ok(Domain.storageTarget({ kind: "custom", date: "2026-02-30" }, 3, now).error);
});
