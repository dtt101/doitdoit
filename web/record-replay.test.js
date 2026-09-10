"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const P = require("./record-protocol.js");
const { replay } = require("./record-replay.js");
const fixture = JSON.parse(fs.readFileSync(path.join(__dirname, "../docs/storage/fixtures/scenarios.json"), "utf8"));
async function expandFixtures() {
  const records = {};
  async function walk(v) {
    if (typeof v === "string" && v.startsWith("$raw:")) return P.hash(fixture.raw[v.slice(5)]);
    if (typeof v === "string" && v.startsWith("$")) return (await expand(v.slice(1))).id;
    if (Array.isArray(v)) return Promise.all(v.map(walk));
    if (v && typeof v === "object") return Object.fromEntries(await Promise.all(Object.entries(v).map(async ([k, x]) => [k, await walk(x)])));
    return v;
  }
  async function expand(name) { if (records[name]) return records[name]; const body = await walk(fixture.records[name]); for (const k of ["parents", "resolves"]) if (body[k]) body[k].sort(); return records[name] = await P.parse(JSON.stringify(body)); }
  for (const name of Object.keys(fixture.records)) await expand(name);
  return records;
}
function check(actual, expected, records, key = "") {
  if (["snapshot", "common"].includes(key)) return assert.deepEqual(actual, expected, key);
  if (Array.isArray(expected)) {
    assert.ok(Array.isArray(actual), key); assert.equal(actual.length, expected.length, key);
    if (["pending", "alternatives", "heads"].includes(key)) return assert.deepEqual(actual, expected.map(name => records[name].id).sort());
    if (key === "candidates") expected = [...expected].sort((a, b) => records[a.tip.slice(1)].id < records[b.tip.slice(1)].id ? -1 : 1);
    expected.forEach((v, i) => check(actual[i], v, records, key));
  } else if (expected !== null && typeof expected === "object") {
    assert.ok(actual !== null && typeof actual === "object", key); for (const [k, v] of Object.entries(expected)) check(actual[k], v, records, k);
  } else assert.deepEqual(actual, typeof expected === "string" && expected.startsWith("$") ? records[expected.slice(1)].id : expected, key);
}
for (const scenario of fixture.scenarios) test(scenario.name, async () => {
  const records = await expandFixtures(), backups = {};
  for (const [name, raw] of Object.entries(fixture.raw)) if (!(scenario.withhold_raw || []).includes(name)) backups[await P.hash(raw)] = raw;
  const delivered = scenario.delivery.map(name => records[name]);
  async function verify(expected) {
    const base = await replay(delivered, backups); check(base, expected, records); if (!Object.hasOwn(expected, "issues")) assert.deepEqual(base.issues, []);
    let seed = 42; const random = () => { seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0; return seed / 4294967296; };
    for (let i = 0; i < 12; i++) { const shuffled = [...delivered, ...delivered]; for (let j = shuffled.length - 1; j > 0; j--) { const k = Math.floor(random() * (j + 1)); [shuffled[j], shuffled[k]] = [shuffled[k], shuffled[j]]; } assert.deepEqual(await replay(shuffled, backups), base); }
  }
  await verify(scenario.expected);
  if (scenario.then) { delivered.push(...scenario.then.delivery.map(name => records[name])); for (const name of scenario.then.raw || []) backups[await P.hash(fixture.raw[name])] = fixture.raw[name]; await verify(scenario.then.expected); }
});

test("production timestamp validator consumes shared vectors", () => {
  for (const { value, valid } of JSON.parse(fs.readFileSync(path.join(__dirname, "../docs/storage/fixtures/creation-timestamps.json"), "utf8"))) assert.equal(P.creationTimestamp(value), valid, value);
});

test("canonical JSON rejects ambiguous values before hashing", async () => {
  for (const raw of ['{"a":1,"a":2}', '{"a":1,"\\u0061":2}', '"\\ud800"', '"\\udc00"', '"\\ud800\\u0041"', '1.1', '9007199254740992', '9007199254740991.1', '1e1000000000', 'null true', '\ufeff{}', '"\ud800"']) assert.throws(() => P.canonical(raw), raw);
  for (const [raw, expected] of [['-0', '0'], ['1e2', '100'], ['"\\ud83d\\ude00"', '"\\ud83d\\ude00"']]) assert.equal(P.canonical(raw), expected);
  for (const vector of fixture.canonical_vectors) assert.equal(P.canonical(JSON.stringify(vector.value)), vector.ascii);
});

test("shared validation and legacy normalization", async () => {
  const f = JSON.parse(fs.readFileSync(path.join(__dirname, "../docs/storage/fixtures/validation.json"), "utf8"));
  for (const tc of f.legacy) {
    if (!tc.valid) await assert.rejects(P.normalizeLegacy(tc.raw));
    else assert.deepEqual(JSON.parse(JSON.stringify(await P.normalizeLegacy(tc.raw))), tc.snapshot);
  }
  for (const tc of f.canonical) { if (tc.valid) assert.equal(P.canonical(tc.raw), tc.canonical); else assert.throws(() => P.canonical(tc.raw)); }
  for (const tc of f.records) { if (tc.valid) await P.parse(tc.raw); else await assert.rejects(P.parse(tc.raw)); }
});

test("corruption blocks its descendants and cannot be hidden by a valid duplicate", async () => {
  const r = await expandFixtures(); const corrupt = { ...r.base, body: r.base.body + "\n" };
  const view = await replay([r.base, corrupt, r.active, r.complete]);
  assert.equal(view.snapshot, null); assert.deepEqual(view.issues, [{ id: r.base.id, code: "invalid-record" }]); assert.deepEqual(view.pending, [r.active.id, r.complete.id].sort());
});

test("rebuild limits are explicit and duplicate deliveries do not consume the record budget", async () => {
  const { MAX_RECORDS } = require("./record-replay.js");
  const records = Array.from({ length: MAX_RECORDS + 1 }, (_, i) => ({ id: i.toString(16).padStart(64, "0"), body: "{}" }));
  const limited = await replay(records); assert.equal(limited.snapshot, null); assert.equal(limited.pending.length, records.length); assert.deepEqual(limited.issues, [{ id: "", code: "replay-limit" }]);
  const r = await expandFixtures(); const duplicated = await replay(records.map(() => r.base)); assert.deepEqual(duplicated.issues, []); assert.equal(duplicated.unique_imports, 1);
  assert.throws(() => P.canonical("[".repeat(256) + "0" + "]".repeat(256)));
});

test("input and returned views are detached across asynchronous hashing and rebuild", async () => {
  const r = await expandFixtures(), records = [r.base, r.active], backups = { [await P.hash(fixture.raw.original)]: fixture.raw.original };
  const rebuilding = replay(records, backups); records.length = 0; delete backups[Object.keys(backups)[0]];
  const view = await rebuilding; assert.deepEqual(view.snapshot, fixture.records.base.snapshot); view.snapshot.Future[0].title = "mutated";
  assert.deepEqual(view.common, fixture.records.base.snapshot);
  const rebuilt = await replay([r.base, r.active], { [await P.hash(fixture.raw.original)]: fixture.raw.original }); assert.deepEqual(rebuilt.snapshot, fixture.records.base.snapshot);
});

test("standalone change validation rejects malformed fields and origin metadata", async () => {
  const records = await expandFixtures(), valid = JSON.parse(records.complete.body);
  const mutations = [
    v => { v.parents = []; }, v => { v.parents = [v.parents[0], v.parents[0]]; },
    v => { v.task_ids = [null]; }, v => { v.task_ids = ["b", "a"]; },
    v => { v.action = "rename"; }, v => { v.resolves = v.parents; },
    v => { v.action = "undo"; }, v => { v.action = "resolve"; },
    v => { v.origin.day = "2026-09-08"; }, v => { v.origin.offset_minutes = 841; },
    v => { v.origin.offset_minutes = null; }, v => { v.origin.extra = true; },
    v => { v.origin.at = "2026-09-07T1:00:00.000Z"; },
    v => { v.buckets.push(v.buckets[0]); }, v => { delete v.buckets[0].before; },
    v => { v.buckets[0].after = null; },
    v => { v.buckets[0].after[0].due_date = "2026-02-30"; },
    v => { v.buckets[0].after[0].title = null; },
    v => { v.buckets[0].after.push(v.buckets[0].after[0]); },
  ];
  for (const mutate of mutations) { const body = structuredClone(valid); mutate(body); await assert.rejects(P.parse(JSON.stringify(body))); }
});

test("activation verifies raw backup hash as well as normalized content", async () => {
  const r = await expandFixtures(); const view = await replay([r.base, r.active], { [await P.hash(fixture.raw.original)]: fixture.raw.reformatted });
  assert.equal(view.snapshot, null); assert.deepEqual(view.issues, [{ id: r.active.id, code: "invalid-backup" }]);
});
