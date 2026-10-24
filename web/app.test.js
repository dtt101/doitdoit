"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const vm = require("node:vm");
const fs = require("node:fs");
const Sync = require("./sync.js");
const Domain = require("./domain.js");
const Auth = require("./auth.js");
const { webcrypto } = require("node:crypto");

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
    DOITDOIT_CONFIG: {}, DoitdoitSync: Sync, DoitdoitDomain: Domain, DoitdoitAuth: Auth,
    location: { origin: "https://example.test", pathname: "/", search: "" },
  });
  const localStorage = {
    getItem: key => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key),
  };
  const context = vm.createContext({
    window, document, localStorage, sessionStorage: localStorage, URLSearchParams, Blob, crypto: webcrypto,
    URL: { createObjectURL: blob => { downloads.push(blob); return "blob:test"; }, revokeObjectURL() {} },
    console: { error() {}, warn() {} }, confirm: () => true, navigator: { locks: { request: async (key, fn) => fn() } }, AbortSignal, TextEncoder, btoa,
    setTimeout: (fn, delay) => { const id = ++timerID; timers.set(id, { fn, delay }); return id; },
    clearTimeout: id => timers.delete(id),
    setInterval: (fn, delay) => { const id = ++timerID; intervals.set(id, { fn, delay }); return id; },
    clearInterval: id => intervals.delete(id), requestAnimationFrame() {},
    fetch: (url, options) => { const d = deferred(); requests.push({ url, options, ...d }); return d.promise; },
  });
  vm.runInContext(fs.readFileSync(require.resolve("./app.js"), "utf8"), context);
  const { state, reload, logout } = window.doitdoit;
  Object.assign(state, { data: snapshot("base"), accessToken: true, online: true });
  storage.set("doitdoit:service:https://doitdoit-service.davidt-91d.workers.dev:tokens", JSON.stringify({ access_token: "test", refresh_token: "refresh", client_id: "client", expires: Date.now() + 3600000 }));
  function edit(title) {
    get("add-input").value = "!future " + title;
    return get("add-form").emit("submit");
  }
  function reply(index, data, rev = "bbbbbbbbb", status = 200) {
    requests[index].resolve({ status, ok: status >= 200 && status < 300,
      headers: { get: () => JSON.stringify({ rev }) },
      text: async () => JSON.stringify(data), json: async () => data,
    });
  }
  return { state, reload, logout, edit, reply, requests, messages, storage, localStorage, get, window, intervals, downloads };
}
const tick = () => new Promise(resolve => setImmediate(resolve));

test("notes icon opens large plain-text notes without expanding the task list or saving", async () => {
  const a = app();
  const notes = "  café\n<img src=x onerror=alert(1)>\n" + "Long notes\n".repeat(10000);
  a.state.data = {
    Future: [{ id: "idea", title: "Idea", notes }, { id: "empty", title: "Empty", notes: "" }, { id: "legacy", title: "Legacy" }],
    "2099-01-01": [{ id: "done", title: "Done", completed: true, notes: "Different notes" }],
  };
  await a.get("menu-dialog").emit("click", { target: { closest: () => ({ dataset: { act: "history" } }) } });
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

test("service capture sends a retained command, then refreshes only after acknowledgement", async () => {
  const a = app();
  const saving = a.edit("new task"); await tick();
  const command = JSON.parse(a.requests[0].options.body);
  assert.equal(a.requests[0].options.method, "POST");
  assert.equal(command.title, "new task");
  assert.equal(command.scheduled_date, null);
  assert.match(command.operation_id, /^[0-9a-f-]{36}$/);
  assert.ok([...a.storage.keys()].some(key => key.includes(":pending:")));
  assert.equal(a.state.data.Future.length, 1, "no optimistic task before acknowledgement");
  a.reply(0, { task: { id: "new", revision: 1 } }); await tick();
  a.reply(1, { tasks: [{ id: "new", title: "new task", revision: 1, status: "open", scheduled_date: null, position: "a" }], snapshot_cursor: 1, timezone: "Europe/London", next_cursor: null });
  await saving;
  assert.equal(a.state.data.Future[0].id, "new");
  assert.equal(a.get("add-input").value, "");
  assert.equal(a.state.dirty, false);
});

test("an uncertain save blocks further edits and survives refresh", async () => {
  const a = app();
  const saving = a.edit("retained"); await tick();
  a.requests[0].reject(new Error("lost response")); await tick();
  a.reply(1, { tasks: [], snapshot_cursor: 0, timezone: "Europe/London", next_cursor: null }); await saving;
  assert.equal(a.state.dirty, true);
  assert.match(a.get("sync-indicator").textContent, /retained/);
  await a.edit("must wait");
  assert.equal(a.requests.length, 2);
  assert.equal(a.get("add-input").value, "!future must wait");
});

test("reads keep overdue dates and never send maintenance writes", async () => {
  const a = app(); const loading = a.reload(); await tick();
  a.reply(0, { tasks: [{ id: "old", title: "Still open", status: "open", revision: 1, scheduled_date: "2000-01-01", position: "a" }], snapshot_cursor: 1, timezone: "Pacific/Auckland", next_cursor: null });
  await loading;
  assert.equal(a.requests.length, 1);
  assert.equal(a.state.data["2000-01-01"][0].scheduled_date, "2000-01-01");
  const section = a.get("board").children[0].children.find(section => section.dataset.key === "2000-01-01");
  assert.equal(section.children[0].children[0].textContent, "Overdue");
  assert.equal(a.state.timezone, "Pacific/Auckland");
});

test("background reads cannot replace an active editor or a newer save", async () => {
  const a = app(); const loading = a.reload(); await tick();
  a.state.interactionActive = true;
  a.reply(0, { tasks: [], snapshot_cursor: 0, timezone: "Europe/London", next_cursor: null }); await loading;
  assert.equal(a.state.data.Future[0].title, "base");
});

test("invalid capture keeps input and sends nothing", async () => {
  const a = app();
  a.get("add-input").value = "!unknown task";
  await a.get("add-form").emit("submit");
  assert.equal(a.requests.length, 0);
  assert.equal(a.get("add-input").value, "!unknown task");
});

test("completion sends the reviewed revision and leaves notes/date untouched", async () => {
  const a = app();
  a.state.data = { "2000-01-01": [{ id: "a", title: "Overdue", completed: false, revision: 7, notes: "Keep", due_date: "2000-01-01" }] };
  const row = { dataset: { key: "2000-01-01", id: "a" } };
  const button = { dataset: { action: "toggle" }, closest: () => row };
  await a.get("board").emit("click", { target: { closest: () => button } }); await tick();
  const body = JSON.parse(a.requests[0].options.body);
  assert.equal(body.expected_revision, 7);
  assert.deepEqual(body.changes, { status: "completed" });
  a.reply(0, { task: { id: "a", revision: 8 } }); await tick();
  a.reply(1, { tasks: [], snapshot_cursor: 1, timezone: "Europe/London", next_cursor: null }); await tick();
});
