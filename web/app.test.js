"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const vm = require("node:vm");
const fs = require("node:fs");
const Sync = require("./sync.js");
const Domain = require("./domain.js");

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
const snapshot = title => ({ Future: [{ id: "a", title, completed: false }] });
const copy = value => JSON.parse(JSON.stringify(value));

// Execute the real app, its event handlers, and transport with isolated storage.
// The DOM adapter only implements presentation operations used by these scenarios.
function app() {
  const elements = new Map(), timers = new Map(), intervals = new Map(), storage = new Map();
  const requests = [], messages = [], downloads = [];
  let timerID = 0;
  function element() {
    const listeners = new Map();
    return {
      attributes: {}, dataset: {}, style: { setProperty() {} }, value: "", hidden: false, children: [],
      classList: { add() {}, remove() {}, toggle() {} },
      addEventListener(name, fn) { const list = listeners.get(name) || []; list.push(fn); listeners.set(name, list); },
      emit(name, event = {}) { return Promise.all((listeners.get(name) || []).map(fn => fn({ preventDefault() {}, ...event }))); },
      append(...items) { this.children.push(...items); },
      appendChild(item) { this.children.push(item); if (item.textContent) messages.push(item.textContent); },
      replaceChildren(...items) { this.children = items; },
      querySelectorAll() { return []; }, querySelector() { return element(); }, closest() { return element(); },
      setAttribute(name, value) { this.attributes[name] = value; },
      removeAttribute(name) { delete this.attributes[name]; }, remove() {}, focus() {}, showModal() { this.open = true; }, close() { this.open = false; },
      getBoundingClientRect() { return { left: 0, right: 100, top: 0, bottom: 100, height: 10 }; },
      click() { downloads.push(this); },
    };
  }
  const get = id => { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); };
  const document = Object.assign(element(), {
    getElementById: get, createElement: element, createDocumentFragment: element,
    createElementNS: element, documentElement: element(), visibilityState: "visible",
  });
  const window = Object.assign(element(), {
    DOITDOIT_CONFIG: {}, DoitdoitSync: Sync, DoitdoitDomain: Domain,
    location: { origin: "https://example.test", pathname: "/", search: "" },
  });
  const localStorage = {
    getItem: key => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key),
  };
  const context = vm.createContext({
    window, document, localStorage, URLSearchParams, Blob,
    URL: { createObjectURL: blob => { downloads.push(blob); return "blob:test"; }, revokeObjectURL() {} },
    console: { error() {}, warn() {} }, confirm: () => true, navigator: {},
    setTimeout: (fn, delay) => { const id = ++timerID; timers.set(id, { fn, delay }); return id; },
    clearTimeout: id => timers.delete(id),
    setInterval: (fn, delay) => { const id = ++timerID; intervals.set(id, { fn, delay }); return id; },
    clearInterval: id => intervals.delete(id), requestAnimationFrame() {},
    fetch: (url, options) => { const d = deferred(); requests.push({ url, options, ...d }); return d.promise; },
  });
  vm.runInContext(fs.readFileSync(require.resolve("./app.js"), "utf8"), context);
  const { state, reload, logout } = window.doitdoit;
  Object.assign(state, { data: snapshot("base"), rev: "aaaaaaaaa", accessToken: "test", tokenExp: Date.now() + 3600000 });
  function edit(title) {
    get("add-input").value = "!future " + title;
    return get("add-form").emit("submit");
  }
  function save() {
    const entry = [...timers].find(([, t]) => t.delay === 600 || t.delay === 400);
    assert.ok(entry, "save scheduled");
    timers.delete(entry[0]);
    return entry[1].fn();
  }
  function reply(index, data, rev = "bbbbbbbbb", status = 200) {
    requests[index].resolve({ status, ok: status >= 200 && status < 300,
      headers: { get: () => JSON.stringify({ rev }) },
      text: async () => JSON.stringify(data), json: async () => data,
    });
  }
  return { state, reload, logout, edit, save, reply, requests, messages, storage, localStorage, get, window, intervals, downloads };
}
const tick = () => new Promise(resolve => setImmediate(resolve));

test("reload begun clean cannot replace an edit or its revision", async () => {
  const a = app();
  const loading = a.reload(); await tick();
  await a.edit("local"); const expected = copy(a.state.data);
  a.reply(0, snapshot("remote")); await loading;
  assert.deepEqual(copy(a.state.data), expected);
  assert.equal(a.state.rev, "aaaaaaaaa"); assert.equal(a.state.dirty, true);
});

test("only the latest overlapping reload may apply", async () => {
  const a = app();
  const first = a.reload(); const second = a.reload(); await tick();
  a.reply(1, snapshot("latest"), "ccccccccc"); await second;
  a.reply(0, snapshot("old")); await first;
  assert.equal(a.state.data.Future[0].title, "latest"); assert.equal(a.state.rev, "ccccccccc");
});

test("upload A acknowledges only A and uploads B with A's revision", async () => {
  const a = app(); await a.edit("A");
  const saving = a.save(); await tick();
  await a.edit("B"); const expected = copy(a.state.data);
  assert.equal(JSON.parse(a.requests[0].options.body).Future.some(t => t.title === "B"), false);
  a.reply(0, { rev: "bbbbbbbbb" }); await saving;
  assert.equal(a.state.dirty, true);
  await a.window.emit("focus");
  for (const { fn, delay } of a.intervals.values()) if (delay === 60000) fn();
  assert.equal(a.requests.length, 1);
  const second = a.save(); await tick();
  assert.deepEqual(JSON.parse(a.requests[1].options.body), expected);
  assert.equal(JSON.parse(a.requests[1].options.headers["Dropbox-API-Arg"]).mode.update, "bbbbbbbbb");
  a.reply(1, { rev: "ccccccccc" }); await second;
  assert.equal(a.state.dirty, false);
});

test("failed upload retains newer edits and revision; forced reload saves exact recovery", async () => {
  const a = app(); await a.edit("A"); const saving = a.save(); await tick();
  await a.edit("B"); const expected = copy(a.state.data);
  a.requests[0].reject(new Error("offline")); await saving;
  assert.equal(a.state.dirty, true); assert.equal(a.state.rev, "aaaaaaaaa");
  const loading = a.reload({ force: true }); await tick();
  a.reply(1, snapshot("remote")); await loading;
  assert.deepEqual(JSON.parse(a.storage.get("doitdoit:recovery")).data, expected);
  assert.equal(a.state.dirty, false);
  await a.get("menu-dialog").emit("click", { target: { closest: () => ({ dataset: { act: "recovery" } }) } });
  assert.deepEqual(JSON.parse(await a.downloads[0].text()), expected);
});

for (const failure of ["quota", "readback"]) test(`recovery ${failure} failure blocks forced replacement and disconnect`, async () => {
  const a = app(); await a.edit("unsaved"); const expected = copy(a.state.data);
  if (failure === "quota") a.localStorage.setItem = () => { throw new Error("quota"); };
  else a.localStorage.setItem = () => {};
  const loading = a.reload({ force: true }); await tick();
  a.reply(0, snapshot("remote")); await loading;
  assert.deepEqual(copy(a.state.data), expected); assert.equal(a.state.dirty, true);
  assert.equal(a.state.rev, "aaaaaaaaa"); assert.ok(a.messages.some(m => m.startsWith("load failed")));
  await a.logout();
  assert.deepEqual(copy(a.state.data), expected); assert.equal(a.state.accessToken, "test");
  assert.ok(a.messages.some(m => m.startsWith("disconnect blocked")));
});

test("an edit during forced reload remains dirty without being discarded", async () => {
  const a = app(); await a.edit("A"); const loading = a.reload({ force: true }); await tick();
  await a.edit("B"); const expected = copy(a.state.data);
  a.reply(0, snapshot("remote")); await loading;
  assert.deepEqual(copy(a.state.data), expected); assert.equal(a.state.dirty, true);
});

test("maintenance upload and a new edit share save serialization", async () => {
  const a = app(); const loading = a.reload(); await tick();
  a.reply(0, { "2000-01-01": [{ id: "old", title: "rollover", completed: false }] }); await tick();
  assert.equal(a.requests.length, 2); assert.equal(a.state.saving, true);
  await a.edit("new");
  await a.reload({ force: true }); assert.equal(a.requests.length, 2);
  a.reply(1, { rev: "ccccccccc" }); await loading;
  assert.equal(a.state.dirty, true);
  const saving = a.save(); await tick();
  assert.ok(JSON.parse(a.requests[2].options.body).Future.some(t => t.title === "new"));
  a.reply(2, { rev: "ddddddddd" }); await saving;
  assert.equal(a.state.dirty, false);
});

test("failed maintenance stays dirty and blocks background reload", async () => {
  const a = app(); const loading = a.reload(); await tick();
  a.reply(0, { "2000-01-01": [{ id: "old", title: "rollover", completed: false }] }); await tick();
  a.requests[1].reject(new Error("offline")); await loading;
  assert.equal(a.state.dirty, true); await a.window.emit("focus"); assert.equal(a.requests.length, 2);
});

test("disconnect preserves edits and ignores an in-flight save acknowledgement", async () => {
  const a = app(); await a.edit("A"); const saving = a.save(); await tick();
  await a.edit("B"); const expected = copy(a.state.data);
  const logout = a.logout(); await tick();
  assert.deepEqual(JSON.parse(a.storage.get("doitdoit:recovery")).data, expected);
  assert.equal(a.state.data, null); assert.equal(a.get("btn-menu").hidden, false);
  a.reply(0, { rev: "bbbbbbbbb" }); await saving;
  assert.equal(a.state.rev, null); assert.equal(a.state.dirty, false);
  a.reply(1, {}); await logout;
});

test("disconnect invalidates an in-flight reload", async () => {
  const a = app(); const loading = a.reload(); await tick();
  const logout = a.logout(); await tick();
  a.reply(0, snapshot("late")); await loading;
  assert.equal(a.state.data, null); a.reply(1, {}); await logout;
});

test("authentication failure retains unsaved edits and keeps recovery available", async () => {
  const a = app(); a.state.refreshToken = "refresh"; await a.edit("local");
  const expected = copy(a.state.data); const saving = a.save(); await tick();
  a.reply(0, {}, undefined, 401); await tick();
  a.reply(1, {}, undefined, 400); await saving;
  assert.deepEqual(copy(a.state.data), expected); assert.equal(a.state.dirty, true);
  assert.equal(a.state.rev, "aaaaaaaaa"); assert.ok(a.messages.some(m => m.includes("local edits retained")));
});

for (const kind of ["download", "upload"]) test(`malformed ${kind} revision cannot clear local state`, async () => {
  const a = app();
  if (kind === "upload") await a.edit("local");
  const expected = copy(a.state.data);
  const pending = kind === "download" ? a.reload() : a.save(); await tick();
  a.reply(0, kind === "download" ? snapshot("remote") : {}, "invalid"); await pending;
  assert.deepEqual(copy(a.state.data), expected); assert.equal(a.state.rev, "aaaaaaaaa");
  assert.equal(a.state.dirty, kind === "upload");
});

test("reload response cannot overwrite a save that has already completed", async () => {
  const a = app(); const loading = a.reload(); await tick();
  await a.edit("new"); const saving = a.save(); await tick();
  a.reply(1, { rev: "ccccccccc" }); await saving;
  a.reply(0, snapshot("old")); await loading;
  assert.ok(a.state.data.Future.some(t => t.title === "new"));
  assert.equal(a.state.rev, "ccccccccc"); assert.equal(a.state.dirty, false);
});

test("conflict recovery failure is visible and further edits cannot bypass the conflict", async () => {
  const a = app(); await a.edit("A"); const saving = a.save(); await tick();
  a.localStorage.setItem = () => { throw new Error("quota"); };
  a.reply(0, {}, undefined, 409); await saving;
  assert.equal(a.state.dirty, true); assert.equal(a.state.conflict, true);
  assert.ok(a.messages.some(m => m.includes("recovery failed: quota")));
  await a.edit("B"); await a.save(); assert.equal(a.requests.length, 1);
  assert.equal(a.state.conflict, true);
});

test("a late token refresh cannot reconnect a disconnected session", async () => {
  const a = app(); a.state.refreshToken = "refresh"; await a.edit("local");
  const saving = a.save(); await tick(); a.reply(0, {}, undefined, 401); await tick();
  const logout = a.logout(); await tick();
  a.reply(1, { access_token: "late", refresh_token: "late" }); await saving;
  assert.equal(a.state.accessToken, null); assert.equal(a.storage.has("doitdoit:tokens"), false);
  a.reply(2, {}); await logout;
});

test("reload arriving during an active interaction leaves the board and revision intact", async () => {
  const a = app(); const loading = a.reload(); await tick();
  a.state.interactionActive = true;
  a.reply(0, snapshot("remote")); await loading;
  assert.equal(a.state.data.Future[0].title, "base"); assert.equal(a.state.rev, "aaaaaaaaa");
  assert.equal(a.get("sync-indicator").dataset.state, "idle");
});


test("capture keeps invalid input for correction and clears a successful capture", async () => {
  const a = app();
  const original = copy(a.state.data);
  a.get("add-input").value = "!not-a-date book the dentist";
  await a.get("add-form").emit("submit");
  assert.equal(a.get("add-input").value, "!not-a-date book the dentist");
  assert.deepEqual(copy(a.state.data), original);
  assert.equal(a.state.dirty, false);
  assert.ok(a.messages.some(message => message.includes("unknown target")));
  await a.edit("book the dentist");
  assert.equal(a.get("add-input").value, "");
  assert.ok(a.state.data.Future.some(task => task.title === "book the dentist"));
});

test("sync status explains unsaved, syncing, saved and conflict states", async () => {
  const a = app();
  const indicator = a.get("sync-indicator");
  assert.equal(indicator.textContent, "Not connected");
  await a.edit("first");
  assert.equal(indicator.textContent, "Unsaved");
  const saving = a.save(); await tick();
  assert.equal(indicator.textContent, "Syncing…");
  a.reply(0, { rev: "bbbbbbbbb" }); await saving;
  assert.equal(indicator.textContent, "Synced");
  await a.edit("second");
  const conflict = a.save(); await tick();
  a.reply(1, {}, undefined, 409); await conflict;
  assert.equal(indicator.textContent, "Conflict");
  await a.edit("third");
  assert.equal(indicator.textContent, "Conflict");
});

test("load migrates dated Future tasks with a conditional save and renders distant dates", async () => {
  const a = app();
  const legacy = { Future: [
    { id: "scheduled", title: "Book a trip", due_date: "2099-06-21", notes: "Details\nMore", completed: false },
    { id: "idea", title: "Learn pottery", completed: true },
  ] };
  const loading = a.reload(); await tick();
  a.reply(0, legacy, "bbbbbbbbb"); await tick();
  assert.equal(a.requests.length, 2);
  const saved = JSON.parse(a.requests[1].options.body);
  assert.deepEqual(saved.Future, [legacy.Future[1]]);
  assert.deepEqual(saved["2099-06-21"], [legacy.Future[0]]);
  const arg = JSON.parse(a.requests[1].options.headers["Dropbox-API-Arg"]);
  assert.equal(arg.mode.update, "bbbbbbbbb");
  a.reply(1, { rev: "ccccccccc" }); await loading;
  assert.equal(a.state.dirty, false);
  const sections = a.get("board").children[0].children;
  const future = sections.find(section => section.dataset.key === "Future");
  assert.equal(future.children[1].children.length, 1);
  const distant = sections.find(section => section.dataset.key === "2099-06-21");
  assert.equal(distant.children[1].children[0].dataset.id, "scheduled");
  const again = a.reload(); await tick();
  a.reply(2, saved, "ccccccccc"); await again;
  assert.equal(a.requests.length, 3, "repeat load must not upload");
});

test("migration upload conflicts retain the migrated draft for recovery", async () => {
  const a = app();
  const loading = a.reload(); await tick();
  a.reply(0, { Future: [{ id: "legacy", due_date: "2099-01-01" }] }); await tick();
  a.reply(1, {}, undefined, 409); await loading;
  assert.equal(a.state.conflict, true);
  assert.equal(a.state.dirty, true);
  assert.equal(a.state.data["2099-01-01"][0].id, "legacy");
});


test("notes icon opens large plain-text notes without expanding the task list or saving", async () => {
  const a = app();
  const notes = "  café\n<img src=x onerror=alert(1)>\n" + "Long notes\n".repeat(10000);
  a.state.data = {
    Future: [{ id: "idea", title: "Idea", notes }, { id: "empty", title: "Empty", notes: "" }, { id: "legacy", title: "Legacy" }],
    "2099-01-01": [{ id: "done", title: "Done", completed: true, notes: "Different notes" }],
  };
  await a.edit("another idea");
  const before = copy(a.state.data);
  const dirty = a.state.dirty;
  const sections = a.get("board").children[0].children;
  const rows = sections.flatMap(section => section.children[1].children);
  for (const id of ["idea", "done"]) {
    const row = rows.find(row => row.dataset.id === id);
    const button = row.children.find(child => child.className === "task__notes-button");
    assert.equal(button.textContent, undefined, "board must not contain note text");
    assert.equal(button.children[0].attributes["aria-hidden"], "true");
    assert.equal(button.attributes["aria-haspopup"], "dialog");
    assert.match(button.attributes["aria-label"], /view notes for/);
    button.closest = () => row;
    await a.get("board").emit("click", { target: { closest: () => button } });
    assert.equal(a.get("notes-dialog").open, true);
    assert.equal(a.get("notes-content").textContent, id === "idea" ? notes : "Different notes");
    assert.equal(a.get("notes-content").children.length, 0);
    assert.equal(a.get("notes-task-title").textContent, id === "idea" ? "Idea" : "Done");
    await a.get("notes-close").emit("click");
    assert.equal(a.get("notes-dialog").open, false);
    await a.get("notes-dialog").emit("close");
    assert.equal(a.get("notes-content").textContent, "");
  }
  for (const id of ["empty", "legacy"]) {
    assert.equal(rows.find(row => row.dataset.id === id).children.some(child => child.className === "task__notes-button"), false);
  }
  assert.deepEqual(copy(a.state.data), before);
  assert.equal(a.state.dirty, dirty);
  assert.equal(a.requests.length, 0);
});
