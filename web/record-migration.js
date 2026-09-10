(function (root, factory) {
  if (typeof module === "object" && module.exports) module.exports = factory(require("./record-protocol.js"));
  else root.DoitdoitRecordMigration = factory(root.DoitdoitRecordProtocol);
})(globalThis, function (P) {
  "use strict";
  // Pure preparation shared with desktop migration. Browser durability, discovery,
  // outbox publication, and Dropbox integration remain stage 7; this is inactive.
  async function prepare(raw) {
    const snapshot = await P.normalizeLegacy(raw);
    const imported = await P.parse(JSON.stringify({ schema: 1, kind: "import", snapshot }));
    const backup = await P.hash(raw);
    const activation = await P.parse(JSON.stringify({ schema: 1, kind: "activate", import: imported.id, backup }));
    const original = JSON.parse(raw), repairs = [];
    for (const bucket of Object.keys(snapshot).sort()) snapshot[bucket].forEach((task, index) => {
      const old = original[bucket][index].id || "";
      if (old !== task.id) repairs.push({ import: imported.id, bucket, index, original_id: old, id: task.id });
    });
    return { raw, backup, import: imported, activation, repairs };
  }
  return { prepare };
});
