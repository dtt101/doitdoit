(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.DoitdoitRecordProtocol = api;
})(globalThis, function () {
  "use strict";
  const MAX_BYTES = 16 << 20;
  const requireValue = (value, message = "invalid protocol value") => { if (!value) throw new Error(message); };
  const object = value => value !== null && typeof value === "object" && !Array.isArray(value);
  const ordered = list => list.every((v, i) => i === 0 || list[i - 1] < v);
  const id = s => typeof s === "string" && /^[a-f0-9]{64}$/.test(s) && s.length === 64;
  const hex32 = s => typeof s === "string" && /^[a-f0-9]{32}$/.test(s) && s.length === 32;
  function scalar(s) {
    for (let i = 0; i < s.length; i++) {
      const c = s.charCodeAt(i);
      if (c >= 0xd800 && c <= 0xdbff) { const low = s.charCodeAt(++i); requireValue(low >= 0xdc00 && low <= 0xdfff); }
      else requireValue(c < 0xdc00 || c > 0xdfff);
    }
  }
  function encode(value) {
    if (typeof value === "string") {
      scalar(value);
      return '"' + value.replace(/["\\\u0000-\u001f\u007f-\uffff]/g, c => c === '"' || c === "\\" ? "\\" + c : "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0")) + '"';
    }
    if (Array.isArray(value)) return "[" + value.map(encode).join(",") + "]";
    if (object(value)) return "{" + Object.keys(value).sort().map(k => encode(k) + ":" + encode(value[k])).join(",") + "}";
    requireValue(value === null || typeof value === "boolean" || Number.isSafeInteger(value));
    return JSON.stringify(value);
  }
  // JSON.parse cannot detect duplicate keys or fractions rounded to integers.
  // A small token reader checks the original text before canonical encoding.
  function canonical(raw) {
    requireValue(typeof raw === "string"); scalar(raw);
    requireValue(new TextEncoder().encode(raw).length <= MAX_BYTES);
    let pos = 0;
    const space = () => { while (/[ \t\r\n]/.test(raw[pos] || "x")) pos++; };
    function string() {
      const start = pos++;
      while (pos < raw.length) {
        if (raw[pos] === '"') { pos++; const s = JSON.parse(raw.slice(start, pos)); scalar(s); return s; }
        if (raw[pos++] === "\\") pos++;
      }
      throw new Error("unterminated string");
    }
    function value(depth = 0) {
      requireValue(depth < 256, "JSON nesting limit"); space(); const c = raw[pos];
      if (c === '"') return string();
      if (c === "{" || c === "[") {
        const obj = c === "{"; const result = obj ? Object.create(null) : []; const end = obj ? "}" : "]"; pos++; space();
        if (raw[pos] === end) { pos++; return result; }
        for (;;) {
          space(); let key;
          if (obj) { requireValue(raw[pos] === '"'); key = string(); requireValue(!Object.hasOwn(result, key), "duplicate key"); space(); requireValue(raw[pos++] === ":"); }
          const child = value(depth + 1); if (obj) result[key] = child; else result.push(child);
          space(); const next = raw[pos++]; if (next === end) return result; requireValue(next === ",");
        }
      }
      for (const [literal, v] of [["true", true], ["false", false], ["null", null]]) if (raw.startsWith(literal, pos)) { pos += literal.length; return v; }
      const match = /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/.exec(raw.slice(pos));
      requireValue(match); const token = match[0]; pos += token.length; requireValue(token.length <= 128);
      const [mantissa, exponent = "0"] = token.toLowerCase().split("e"); const exp = Number(exponent); requireValue(Math.abs(exp) <= 1000);
      const decimals = (mantissa.split(".")[1] || "").length; let n = BigInt(mantissa.replace(".", "")); const power = exp - decimals;
      if (power >= 0) n *= 10n ** BigInt(power);
      else { const divisor = 10n ** BigInt(-power); requireValue(n % divisor === 0n); n /= divisor; }
      requireValue(n >= -9007199254740991n && n <= 9007199254740991n);
      return Number(n);
    }
    const result = encode(value()); space(); requireValue(pos === raw.length); requireValue(result.length <= MAX_BYTES); return result;
  }
  async function hash(raw) { const bytes = await globalThis.crypto.subtle.digest("SHA-256", new TextEncoder().encode(raw)); return Array.from(new Uint8Array(bytes), b => b.toString(16).padStart(2, "0")).join(""); }
  function date(s) {
    if (typeof s !== "string" || !/^[0-9]{4}-[0-9]{2}-[0-9]{2}$/.test(s) || s.length !== 10) return false;
    const [year, month, day] = s.split("-").map(Number); const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
    return month >= 1 && month <= 12 && day >= 1 && day <= [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1];
  }
  const bucket = s => s === "Future" || date(s);
  function creationTimestamp(s) {
    if (typeof s !== "string") return false;
    const m = /^[0-9]{4}-[0-9]{2}-[0-9]{2}T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](\.[0-9]+)?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$/.exec(s);
    return !!m && m[0] === s && date(s.slice(0, 10));
  }
  function fields(v, required, optional = "") {
    requireValue(object(v)); const names = required.split(" ").filter(Boolean); const allowed = [...names, ...optional.split(" ")];
    requireValue(names.every(k => Object.hasOwn(v, k)) && Object.keys(v).every(k => allowed.includes(k)));
  }
  function list(v, min, valid) { requireValue(Array.isArray(v) && v.length >= min && ordered(v) && v.every(x => typeof x === "string" && valid(x))); }
  function tasks(v, seen, nonempty) {
    requireValue(Array.isArray(v) && (!nonempty || v.length > 0));
    for (const t of v) {
      fields(t, "id title completed created_at due_date");
      requireValue([t.id, t.title, t.created_at, t.due_date].every(s => typeof s === "string"));
      requireValue(t.id !== "" && !seen.has(t.id)); seen.add(t.id);
      requireValue(typeof t.completed === "boolean" && creationTimestamp(t.created_at) && (t.due_date === "" || date(t.due_date)));
    }
  }
  function validate(v) {
    requireValue(object(v) && v.schema === 1);
    if (v.kind === "import") {
      fields(v, "schema kind snapshot"); requireValue(object(v.snapshot)); const seen = new Set();
      for (const [k, ts] of Object.entries(v.snapshot)) { requireValue(bucket(k)); tasks(ts, seen, true); }
    } else if (v.kind === "activate") { fields(v, "schema kind import backup"); requireValue(id(v.import) && id(v.backup)); }
    else {
      requireValue(v.kind === "change"); fields(v, "schema kind parents origin action task_ids buckets", "resolves undo_of");
      list(v.parents, 1, id); list(v.task_ids, 0, s => s !== "");
      requireValue(["create", "edit", "complete", "reopen", "move", "reorder", "delete", "maintenance", "undo", "resolve"].includes(v.action));
      requireValue(Object.hasOwn(v, "resolves") === (v.action === "resolve") && Object.hasOwn(v, "undo_of") === (v.action === "undo"));
      if (v.action === "resolve") list(v.resolves, 2, id); if (v.action === "undo") requireValue(id(v.undo_of));
      const o = v.origin; fields(o, "device nonce at day offset_minutes"); requireValue(hex32(o.device) && hex32(o.nonce));
      requireValue(typeof o.at === "string" && o.at.length === 24 && /^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}Z$/.test(o.at) && creationTimestamp(o.at));
      requireValue(date(o.day) && Number.isSafeInteger(o.offset_minutes) && Math.abs(o.offset_minutes) <= 840);
      requireValue(new Date(Date.parse(o.at) + o.offset_minutes * 60000).toISOString().slice(0, 10) === o.day);
      requireValue(Array.isArray(v.buckets) && v.buckets.length > 0); const before = new Set(), after = new Set();
      for (const b of v.buckets) { fields(b, v.action === "resolve" ? "bucket after" : "bucket before after"); requireValue(bucket(b.bucket)); tasks(b.after, after, false); if (v.action !== "resolve") tasks(b.before, before, false); }
      requireValue(ordered(v.buckets.map(b => b.bucket)));
    }
  }
  async function parse(raw) { const body = canonical(raw); validate(JSON.parse(body)); return { id: await hash(body), body }; }
  function validSnapshot(s) { const seen = new Set(); for (const ts of Object.values(s)) { let done = false; for (const t of ts) { requireValue(!seen.has(t.id) && (!done || t.completed)); seen.add(t.id); done ||= t.completed; } } }
  async function normalizeLegacy(raw) {
    const body = canonical(raw), original = JSON.parse(body); requireValue(object(original)); const counts = new Map();
    for (const [k, ts] of Object.entries(original)) { requireValue(bucket(k) && Array.isArray(ts)); for (const t of ts) { fields(t, "title completed created_at", "id due_date"); if (Object.hasOwn(t, "id")) { requireValue(typeof t.id === "string"); counts.set(t.id, (counts.get(t.id) || 0) + 1); } } }
    const digest = await hash(body), result = Object.create(null);
    for (const k of Object.keys(original).sort()) {
      const ts = original[k].map((t, i) => ({ ...t, id: !t.id || counts.get(t.id) > 1 ? `repair:${digest}:${k}:${i}` : t.id, due_date: Object.hasOwn(t, "due_date") ? t.due_date : "" }));
      tasks(ts, new Set(), false); if (ts.length) result[k] = ts;
    }
    validSnapshot(result); return result;
  }
  return { MAX_BYTES, canonical, encode, hash, parse, creationTimestamp, normalizeLegacy, validSnapshot };
});
