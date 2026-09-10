"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const P = require("./record-protocol.js");
const M = require("./record-migration.js");
const { replay } = require("./record-replay.js");
const cases = JSON.parse(fs.readFileSync(path.join(__dirname, "../docs/storage/fixtures/validation.json"), "utf8")).legacy;

test("migration preparation validates before producing an activation", async () => {
  for (const c of cases) {
    if (!c.valid) { await assert.rejects(M.prepare(c.raw)); continue; }
    const plan = await M.prepare(c.raw);
    assert.equal(plan.raw, c.raw); assert.equal(plan.backup, await P.hash(c.raw));
    assert.deepEqual(JSON.parse(plan.import.body).snapshot, c.snapshot);
    const view = await replay([plan.import, plan.activation], { [plan.backup]: plan.raw });
    assert.deepEqual(view.issues, []); assert.deepEqual(view.pending, []); assert.deepEqual(view.snapshot, c.snapshot); assert.equal(view.completion_events, 0);
    for (const r of plan.repairs) { assert.equal(r.import, plan.import.id); assert.equal(r.id, c.snapshot[r.bucket][r.index].id); assert.notEqual(r.original_id, r.id); }
    assert.deepEqual(await M.prepare(c.raw), plan);
  }
});

test("equivalent formatting shares import identity but retains exact backup identities", async () => {
  const raw = cases.find(c => c.valid && c.snapshot.Future).raw;
  const formatted = JSON.stringify(JSON.parse(raw), null, 2);
  const a = await M.prepare(raw), b = await M.prepare(formatted);
  assert.equal(a.import.id, b.import.id); assert.notEqual(a.backup, b.backup); assert.notEqual(a.activation.id, b.activation.id);
});
