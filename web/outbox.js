(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.DoitdoitOutbox = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  // Inactive foundation: integration supplies the protocol validator/replayer.
  // Each record has its own key, so tabs never read/replace one shared queue.
  function create({ storage, account, path, validate }) {
    if (typeof account !== "string" || !account || typeof path !== "string" || !path || !storage || typeof validate !== "function") throw new Error("outbox requires account, path, and record validator");
    const prefix = "doitdoit:outbox:v1:" + JSON.stringify([account, path]) + ":";
    const keyFor = id => {
      if (typeof id !== "string" || !/^[a-f0-9]{64}$/.test(id)) throw new Error("invalid record ID");
      return prefix + id;
    };
    async function checked(record) {
      // Detach before awaiting validation: later UI mutations cannot change what
      // was validated, acknowledged, or passed to an upload callback.
      if (!record || typeof record !== "object" || Array.isArray(record) ||
          !Object.hasOwn(record, "id") || !Object.hasOwn(record, "body") ||
          Object.keys(record).some(key => key !== "id" && key !== "body")) {
        throw new Error("pending record must contain only id and body");
      }
      const copy = Object.freeze({ id: record.id, body: record.body });
      keyFor(copy.id);
      if (typeof copy.body !== "string") throw new Error("record body must be canonical JSON text");
      if (await validate(copy) === false) throw new Error("invalid pending record");
      return copy;
    }
    async function enqueue(record) {
      const copy = await checked(record);
      const key = keyFor(copy.id);
      const bytes = JSON.stringify(copy);
      const previous = storage.getItem(key);
      if (previous !== null && previous !== bytes) throw new Error("pending record collision");
      // QuotaExceededError/SecurityError propagate. Callers must not announce a
      // saved edit or clear their in-memory mutation when this fails.
      storage.setItem(key, bytes);
      if (storage.getItem(key) !== bytes) throw new Error("pending record was not retained");
      return copy.id;
    }
    async function list() {
      const keys = [];
      for (let i = 0; i < storage.length; i++) {
        const key = storage.key(i);
        if (key && key.startsWith(prefix)) keys.push(key);
      }
      const records = [], issues = [];
      for (const key of keys.sort()) {
        const raw = storage.getItem(key);
        if (raw === null) continue; // another tab acknowledged this entry
        try {
          const record = await checked(JSON.parse(raw));
          if (keyFor(record.id) !== key) throw new Error("pending key/ID mismatch");
          records.push(record);
        } catch (error) { issues.push({ key, error }); }
      }
      return { records, issues };
    }
    async function flush(publish) {
      const { records, issues } = await list();
      if (issues.length) throw new Error("pending records require recovery", { cause: issues[0].error });
      for (const record of records) {
        // Publisher must confirm create-only upload and exact content on retry.
        // Failure leaves this and later entries queued; earlier acks are safe.
        await publish({ ...record });
        const key = keyFor(record.id);
        const current = storage.getItem(key);
        if (current !== null) {
          if (current !== JSON.stringify(record)) throw new Error("pending record changed before acknowledgement");
          storage.removeItem(key);
        }
      }
    }
    return { enqueue, list, flush };
  }

  return { create };
});
