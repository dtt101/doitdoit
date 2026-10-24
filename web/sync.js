(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.DoitdoitSync = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";
  const clone = value => JSON.parse(JSON.stringify(value));

  function toData(tasks) {
    const data = {};
    for (const task of [...tasks].sort((a, b) =>
      (a.status === b.status ? 0 : a.status === "open" ? -1 : 1) ||
      (a.position < b.position ? -1 : a.position > b.position ? 1 : a.id.localeCompare(b.id)))) {
      if (task.deleted_at) continue;
      const key = task.scheduled_date || "Future";
      (data[key] ||= []).push({ ...task, completed: task.status === "completed", due_date: task.scheduled_date || "" });
    }
    return data;
  }

  function createServiceStore({ origin, request, storage, locks = globalThis.navigator?.locks, uuid = () => crypto.randomUUID() }) {
    const key = "doitdoit:service:pending:" + origin;
    function read() {
      const raw = storage.getItem(key);
      if (!raw) return null;
      const value = JSON.parse(raw);
      if (value.origin !== origin || !value.body?.operation_id || !/^\/v1\/tasks(?:\/[0-9a-f-]+(?:\/(?:reorder|restore))?)?$/.test(value.path) ||
          !["POST", "PATCH", "DELETE"].includes(value.method)) throw new Error("invalid retained command; download recovery before proceeding");
      return value;
    }
    function retain(value) {
      const raw = JSON.stringify(value);
      storage.setItem(key, raw);
      if (storage.getItem(key) !== raw) throw new Error("retained save could not be verified; nothing was sent");
    }
    async function load() {
      const tasks = [];
      let after, cursor, timezone;
      const seen = new Set();
      do {
        const query = new URLSearchParams({ limit: "100" });
        if (after) { query.set("after", after); query.set("snapshot_cursor", String(cursor)); }
        const page = await request("/v1/board?" + query);
        if (!Array.isArray(page.tasks) || !Number.isSafeInteger(page.snapshot_cursor) || typeof page.timezone !== "string" ||
            (cursor !== undefined && (cursor !== page.snapshot_cursor || timezone !== page.timezone))) throw new Error("invalid service snapshot");
        cursor = page.snapshot_cursor; timezone = page.timezone;
        tasks.push(...page.tasks);
        after = page.next_cursor;
        if (after && seen.has(after)) throw new Error("repeated service snapshot page");
        seen.add(after);
      } while (after);
      return { data: toData(tasks), timezone };
    }
    async function retryCommand() {
      const command = read();
      if (!command) return null;
      if (command.rejected) throw new Error("This save was rejected. Download recovery, then review the latest tasks.");
      let result;
      try {
        result = await request(command.path, { method: command.method, body: JSON.stringify(command.body) });
      } catch (err) {
        // These responses establish that no command was accepted. A lost response,
        // timeout, 401 or server error remains uncertain and retains identical bytes.
        if ([400, 403, 404, 409, 413, 415, 422].includes(err.status)) {
          command.rejected = true;
          command.error = { code: err.code, message: err.message, current: err.current };
          retain(command);
        }
        throw err;
      }
      if (!result?.task?.id || !Number.isSafeInteger(result.task.revision)) throw new Error("invalid save acknowledgement; retry retained save");
      storage.removeItem(key);
      if (storage.getItem(key) !== null) throw new Error("save acknowledged; retained command could not be cleared");
      return result.task;
    }
    async function mutate(method, path, body) {
      if (read()) throw new Error("Resolve the retained save from the menu first.");
      retain({ origin, method, path, body: { ...clone(body), operation_id: uuid() }, savedAt: new Date().toISOString() });
      return retryCommand();
    }
    function reviewRejected() {
      const command = read();
      if (!command?.rejected) throw new Error("The save may have succeeded. Retry it before making further changes.");
      const archive = key + ":recovery";
      const previous = JSON.parse(storage.getItem(archive) || "[]");
      const bytes = JSON.stringify([...previous, command]);
      storage.setItem(archive, bytes);
      if (storage.getItem(archive) !== bytes) throw new Error("recovery copy could not be verified");
      storage.removeItem(key);
    }
    const locked = fn => locks?.request ? locks.request(key, fn) : fn();
    return { load, mutate: (...args) => locked(() => mutate(...args)), retry: () => locked(retryCommand), pending: read,
      reviewRejected: () => locked(reviewRejected),
      recovery: () => {
        const pending = read();
        const rejected = JSON.parse(storage.getItem(key + ":recovery") || "[]");
        return pending || rejected.length ? { pending, rejected } : null;
      } };
  }
  return { toData, createServiceStore };
});
