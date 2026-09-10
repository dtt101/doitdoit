"use strict";

// Design-fixture checks only. Production Go/JS replay consumers arrive in PR 4.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { createHash } = require("node:crypto");
const root = path.join(__dirname, "../docs/storage");
const schema = JSON.parse(fs.readFileSync(path.join(root, "record.schema.json"), "utf8"));
const fixture = JSON.parse(fs.readFileSync(path.join(root, "fixtures/scenarios.json"), "utf8"));

function canonical(value) {
  if (typeof value === "string") {
    return '"' + value.replace(/["\\\u0000-\u001f\u007f-\uffff]/g, (c) => {
      if (c === '"' || c === "\\") return "\\" + c;
      return "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0");
    }) + '"';
  }
  if (Array.isArray(value)) return "[" + value.map(canonical).join(",") + "]";
  if (value !== null && typeof value === "object") {
    return "{" + Object.keys(value).sort().map(k => canonical(k) + ":" + canonical(value[k])).join(",") + "}";
  }
  assert.ok(value === null || typeof value === "boolean" || Number.isSafeInteger(value));
  return JSON.stringify(value);
}
const hash = value => createHash("sha256").update(value, "utf8").digest("hex");

// Validate precisely the structural keyword subset used by our checked-in schema.
// Fail on a new keyword so schema growth cannot silently skip validation.
function validate(value, rule) {
  const supported = new Set(["$schema", "title", "description", "$defs", "$ref", "anyOf",
    "type", "const", "enum", "pattern", "minLength", "minimum", "maximum", "minItems",
    "uniqueItems", "items", "required", "properties", "patternProperties", "additionalProperties"]);
  for (const key of Object.keys(rule)) assert.ok(supported.has(key), "unsupported keyword " + key);
  if (rule.$ref) {
    assert.ok(rule.$ref.startsWith("#/$defs/"));
    return validate(value, schema.$defs[rule.$ref.slice(8)]);
  }
  if (rule.anyOf) {
    const results = rule.anyOf.map(candidate => {
      try { validate(value, candidate); return true; } catch { return false; }
    });
    assert.ok(results.some(Boolean), "no schema variant matched");
  }
  if ("const" in rule) assert.deepEqual(value, rule.const);
  if (rule.enum) assert.ok(rule.enum.includes(value));
  if (rule.type === "object") assert.ok(value !== null && typeof value === "object" && !Array.isArray(value));
  else if (rule.type === "array") assert.ok(Array.isArray(value));
  else if (rule.type === "integer") assert.ok(Number.isSafeInteger(value));
  else if (rule.type) assert.equal(typeof value, rule.type);
  if (rule.pattern) assert.match(value, new RegExp(rule.pattern));
  if (rule.minLength !== undefined) assert.ok(value.length >= rule.minLength);
  if (rule.minimum !== undefined) assert.ok(value >= rule.minimum);
  if (rule.maximum !== undefined) assert.ok(value <= rule.maximum);
  if (rule.minItems !== undefined) assert.ok(value.length >= rule.minItems);
  if (rule.uniqueItems) assert.equal(new Set(value.map(canonical)).size, value.length);
  if (rule.items) for (const item of value) validate(item, rule.items);
  if (rule.type === "object") {
    for (const key of rule.required || []) assert.ok(Object.hasOwn(value, key), "missing " + key);
    for (const [key, child] of Object.entries(value)) {
      let matched = false;
      if (Object.hasOwn(rule.properties || {}, key)) {
        validate(child, rule.properties[key]); matched = true;
      }
      for (const [pattern, childRule] of Object.entries(rule.patternProperties || {})) {
        if (new RegExp(pattern).test(key)) { validate(child, childRule); matched = true; }
      }
      if (rule.additionalProperties === false) assert.ok(matched, "unknown property " + key);
    }
  }
}

const expanded = new Map();
function expand(name, visiting = new Set()) {
  if (expanded.has(name)) return expanded.get(name);
  assert.ok(Object.hasOwn(fixture.records, name), "unknown record " + name);
  assert.ok(!visiting.has(name), "causal cycle " + name);
  const next = new Set([...visiting, name]);
  function walk(value) {
    if (typeof value === "string" && value.startsWith("$raw:")) {
      const raw = value.slice(5);
      assert.ok(Object.hasOwn(fixture.raw, raw));
      return hash(fixture.raw[raw]);
    }
    if (typeof value === "string" && value.startsWith("$")) return expand(value.slice(1), next).id;
    if (Array.isArray(value)) return value.map(walk);
    if (value !== null && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, walk(v)]));
    return value;
  }
  const body = walk(fixture.records[name]);
  // Symbolic alias order is not hash order; canonical set fields sort after expansion.
  for (const field of ["parents", "resolves"]) if (body[field]) body[field].sort();
  const result = { body, id: hash(canonical(body)) };
  expanded.set(name, result);
  return result;
}

test("protocol canonical byte vectors", () => {
  for (const vector of fixture.canonical_vectors) assert.equal(canonical(vector.value), vector.ascii);
});

test("all shared record examples satisfy the structural schema and reference known records", () => {
  for (const name of Object.keys(fixture.records)) validate(expand(name).body, schema);
  assert.equal(expand("base").id, expand("same").id);
  assert.notEqual(expand("base").id, expand("other").id);
  assert.notEqual(hash(fixture.raw.original), hash(fixture.raw.reformatted));
  assert.deepEqual(JSON.parse(fixture.raw.original), JSON.parse(fixture.raw.reformatted));
  for (const { body } of expanded.values()) {
    if (body.kind === "change") {
      for (const bucket of body.buckets) assert.equal(Object.hasOwn(bucket, "before"), body.action !== "resolve");
      assert.equal(Object.hasOwn(body, "resolves"), body.action === "resolve");
      assert.equal(Object.hasOwn(body, "undo_of"), body.action === "undo");
    }
    if (body.kind !== "activate") continue;
    const raw = Object.values(fixture.raw).find(bytes => hash(bytes) === body.backup);
    const imported = [...expanded.values()].find(record => record.id === body.import);
    assert.deepEqual(JSON.parse(raw), imported.body.snapshot);
  }
});

test("schema rejects unknown versions, fields, kinds, and malformed dependency IDs", () => {
  for (const body of [
    { ...expand("base").body, schema: 2 },
    { ...expand("base").body, extra: true },
    { ...expand("base").body, kind: "unknown" },
    { ...expand("complete").body, parents: ["not-a-hash"] },
  ]) assert.throws(() => validate(body, schema));
});

test("scenario delivery and expected alternatives name existing records", () => {
  assert.equal(new Set(fixture.scenarios.map(s => s.name)).size, fixture.scenarios.length);
  for (const scenario of fixture.scenarios) {
    for (const stage of [scenario, scenario.then].filter(Boolean)) {
      for (const name of [...stage.delivery, ...stage.expected.pending]) expand(name);
      for (const conflict of stage.expected.conflicts) {
        assert.ok(conflict.alternatives.length >= 2);
        for (const name of conflict.alternatives) expand(name);
      }
      if (stage.expected.snapshot !== null) validate(stage.expected.snapshot, schema.anyOf[0].properties.snapshot);
      for (const raw of [...(stage.raw || []), ...(stage.withhold_raw || [])]) assert.ok(Object.hasOwn(fixture.raw, raw));
    }
  }
});

// Fixture oracle only; stage 4 must reuse these vectors in its production
// validator. Do not use Date.parse alone: it can normalize invalid calendar days.
function validCreationTimestamp(value) {
  const match = /^([0-9]{4})-([0-9]{2})-([0-9]{2})T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](\.[0-9]+)?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$/.exec(value);
  if (!match || match[0] !== value) return false;
  const [year, month, day] = match.slice(1, 4).map(Number);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return month >= 1 && month <= 12 && day >= 1 && day <= days[month - 1];
}

test("shared creation timestamps preserve legacy precision and reject malformed dates", () => {
  const cases = JSON.parse(fs.readFileSync(path.join(root, "fixtures/creation-timestamps.json"), "utf8"));
  for (const { value, valid } of cases) assert.equal(validCreationTimestamp(value), valid, JSON.stringify(value));
});
