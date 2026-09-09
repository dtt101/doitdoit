"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const Outbox = require("./outbox.js");

class Storage {
  values = new Map();
  get length() { return this.values.size; }
  key(i) { return [...this.values.keys()][i] ?? null; }
  getItem(key) { return this.values.get(key) ?? null; }
  setItem(key, value) { this.values.set(key, value); }
  removeItem(key) { this.values.delete(key); }
}
const record = n => {
  const body = JSON.stringify({ kind: "test-fixture", nonce: n });
  return { body, id: crypto.createHash("sha256").update(body).digest("hex") };
};
// A minimal validator isolates outbox durability from PR 4's protocol replay.
const validate = async r => assert.equal(crypto.createHash("sha256").update(r.body).digest("hex"), r.id);
const open = (storage, account = "account-a", path = "/tasks.json") => Outbox.create({ storage, account, path, validate });

test("queued edits survive reopening and duplicate tab delivery", async () => {
  const disk = new Storage(), tabA = open(disk), tabB = open(disk);
  await Promise.all([tabA.enqueue(record(1)), tabB.enqueue(record(2)), tabB.enqueue(record(1))]);
  assert.equal((await open(disk).list()).records.length, 2);
  const uploaded = new Map();
  const publish = async r => {
    if (uploaded.has(r.id)) assert.equal(uploaded.get(r.id), r.body);
    uploaded.set(r.id, r.body);
  };
  await Promise.all([tabA.flush(publish), tabB.flush(publish)]);
  assert.equal(uploaded.size, 2);
  assert.equal((await open(disk).list()).records.length, 0);
});

test("failure and restart after upload before ack retry the same ID", async () => {
  const disk = new Storage(), box = open(disk), edit = record(1);
  await box.enqueue(edit);
  await assert.rejects(box.flush(async () => { throw new Error("offline"); }), /offline/);
  const remove = disk.removeItem.bind(disk);
  disk.removeItem = () => { throw new Error("interrupted acknowledgement"); };
  const uploaded = [];
  await assert.rejects(box.flush(async r => uploaded.push(r.id)), /acknowledgement/);
  disk.removeItem = remove;
  await open(disk).flush(async r => uploaded.push(r.id));
  assert.deepEqual(uploaded, [edit.id, edit.id]);
});

test("quota, permissions, corrupt entries, and invalid records are not acknowledged", async () => {
  const disk = new Storage();
  const write = disk.setItem.bind(disk);
  for (const name of ["QuotaExceededError", "SecurityError"]) {
    disk.setItem = () => { throw Object.assign(new Error(name), { name }); };
    await assert.rejects(open(disk).enqueue(record(1)), e => e.name === name);
    assert.equal(disk.length, 0);
  }
  disk.setItem = write;
  await open(disk).enqueue(record(1));
  const key = disk.key(0);
  disk.setItem(key, "{broken");
  assert.equal((await open(disk).list()).issues.length, 1);
  await assert.rejects(open(disk).flush(() => assert.fail("must not publish")), /recovery/);
  assert.equal(disk.getItem(key), "{broken");
  await assert.rejects(open(disk).enqueue({ ...record(2), body: "changed" }));
});

test("accounts and task paths cannot replay each other's queues", async () => {
  const disk = new Storage();
  await open(disk).enqueue(record(1));
  assert.equal((await open(disk, "account-b").list()).records.length, 0);
  assert.equal((await open(disk, "account-a", "/other.json").list()).records.length, 0);
  const edit = record(2);
  const pending = open(disk).enqueue(edit);
  edit.body = "mutated after submission";
  await pending;
  assert.equal((await open(disk).list()).records.length, 2);
});

test("record envelopes are stable across property order and immutable during validation", async () => {
  const disk = new Storage(), edit = record(1);
  const box = Outbox.create({ storage: disk, account: "a", path: "/tasks.json", validate: async copy => {
    assert.ok(Object.isFrozen(copy));
    assert.throws(() => { copy.body = "changed by validator"; }, TypeError);
    await validate(copy);
  } });
  await box.enqueue(edit);
  await box.enqueue({ body: edit.body, id: edit.id });
  assert.equal(disk.length, 1);
  assert.deepEqual((await box.list()).records, [edit]);
  await box.flush(async copy => { copy.body = "changed by publisher"; });
  assert.equal(disk.length, 0);
});

test("invalid envelopes and validator rejection never write pending storage", async () => {
  const disk = new Storage();
  for (const edit of [null, [], {}, { ...record(1), extra: true }, { ...record(1), body: null }, { ...record(1), id: 1 }]) {
    await assert.rejects(open(disk).enqueue(edit));
  }
  const rejected = Outbox.create({ storage: disk, account: "a", path: "/tasks.json", validate: () => false });
  await assert.rejects(rejected.enqueue(record(1)), /invalid pending record/);
  assert.equal(disk.length, 0);
  for (const account of [null, {}, 42, ""]) {
    assert.throws(() => open(disk, account), /requires account/);
  }
});

test("silently failed writes and unavailable reads are not acknowledged", async () => {
  const disk = new Storage();
  disk.setItem = () => {};
  await assert.rejects(open(disk).enqueue(record(1)), /not retained/);
  disk.getItem = () => { throw new Error("storage unavailable"); };
  await assert.rejects(open(disk).enqueue(record(1)), /storage unavailable/);
});

test("publication failure keeps every unacknowledged operation for retry", async () => {
  const disk = new Storage(), box = open(disk);
  for (let n = 0; n < 3; n++) await box.enqueue(record(n));
  const ordered = (await box.list()).records;
  const uploaded = [];
  await assert.rejects(box.flush(async r => {
    if (uploaded.length === 1) throw new Error("quota or network failure");
    uploaded.push(r.id);
  }), /failure/);
  assert.deepEqual(uploaded, [ordered[0].id]);
  assert.deepEqual((await open(disk).list()).records, ordered.slice(1));
  await open(disk).flush(async r => uploaded.push(r.id));
  assert.deepEqual(uploaded, ordered.map(r => r.id));
  assert.equal(disk.length, 0);
});

test("changed pending evidence during upload is retained for recovery", async () => {
  const disk = new Storage(), box = open(disk);
  await box.enqueue(record(1));
  const key = disk.key(0);
  await assert.rejects(box.flush(async () => disk.setItem(key, "corrupt replacement")), /changed before acknowledgement/);
  assert.equal(disk.getItem(key), "corrupt replacement");
});

test("clearing site storage removes unpublished edits", async () => {
  const disk = new Storage();
  await open(disk).enqueue(record(1));
  disk.values.clear();
  assert.deepEqual(await open(disk).list(), { records: [], issues: [] });
});
