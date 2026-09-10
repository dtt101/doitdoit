(function (root, factory) {
  if (typeof module === "object" && module.exports) module.exports = factory(require("./record-protocol.js"));
  else root.DoitdoitRecordReplay = factory(root.DoitdoitRecordProtocol);
})(globalThis, function (P) {
  "use strict";
  const MAX_RECORDS = 512, MAX_VIEWS = 4096;
  class ReplayLimit extends Error {}
  const keys = o => Object.keys(o).sort();
  const equal = (a, b) => P.encode(a) === P.encode(b);
  const union = (a, b) => [...new Set([...a, ...b])].sort();
  const intersects = (a, b) => a.some(x => b.includes(x));
  const copy = s => structuredClone(s);
  const setBucket = (s, k, ts) => { if (ts?.length) s[k] = copy(ts); else delete s[k]; };
  const select = (s, ks) => { if (s === null) return null; const out = {}; for (const k of ks) setBucket(out, k, s[k]); return out; };
  const writes = n => n.kind === "import" ? n.snapshot : Object.fromEntries(n.buckets.map(b => [b.bucket, b.after]));
  const subset = (nodes, ids) => Object.fromEntries([...ids].filter(id => nodes[id]).map(id => [id, nodes[id]]));
  const activated = nodes => Object.values(nodes).some(n => n.kind === "activate");
  const maximal = (ids, nodes) => ids.filter(id => !ids.some(other => nodes[other].ancestors.has(id))).sort();
  const empty = () => ({ snapshot: null, common: {}, conflicts: [], pending: [], issues: [], heads: [], unique_imports: 0, completion_events: 0 });
  function locations(s) { const out = new Map(); for (const [bucket, ts] of Object.entries(s)) for (const task of ts) out.set(task.id, { task, bucket }); return out; }
  function affected(before, after) {
    const a = locations(before), b = locations(after), changed = new Set(), order = new Set();
    for (const [id, x] of a) if (!b.has(id) || !equal(x, b.get(id))) changed.add(id);
    for (const id of b.keys()) if (!a.has(id)) changed.add(id);
    for (const [k, ts] of Object.entries(before)) {
      const pos = new Map((after[k] || []).map((t, i) => [t.id, i]));
      ts.forEach((x, i) => { if (!changed.has(x.id)) for (const y of ts.slice(i + 1)) if (!changed.has(y.id) && pos.get(x.id) > pos.get(y.id)) { order.add(x.id); order.add(y.id); } });
    }
    return union([...changed], [...order]);
  }
  function validateChange(n, v, nodes) {
    const requireValue = v => { if (!v) throw new Error("action does not match causal bucket replacements"); };
    const before = copy(v.common), after = copy(before), touched = n.buckets.map(b => b.bucket);
    for (const b of n.buckets) setBucket(after, b.bucket, b.after);
    P.validSnapshot(after);
    if (n.action === "resolve") {
      const selected = v.conflicts.filter(c => intersects(c.buckets, touched)); let wantBuckets = [], wantTips = [], changed = [];
      for (const c of selected) { wantBuckets = union(wantBuckets, c.buckets); wantTips = union(wantTips, c.alternatives); for (const candidate of c.candidates) changed = union(changed, affected(candidate.snapshot, select(after, c.buckets))); }
      requireValue(selected.length && equal(touched, wantBuckets) && equal(n.resolves, wantTips) && equal(changed, n.task_ids)); return;
    }
    requireValue(!v.conflicts.some(c => intersects(c.buckets, touched)));
    for (const b of n.buckets) requireValue(equal(b.before, before[b.bucket] || []) && !equal(b.before, b.after));
    const changed = affected(before, after); requireValue(changed.length && equal(changed, n.task_ids));
    if (n.action === "undo") {
      const original = nodes[n.undo_of]; requireValue(original && n.ancestors.has(n.undo_of) && original.kind === "change" && original.action !== "resolve" && original.buckets.length === n.buckets.length);
      n.buckets.forEach((b, i) => { const o = original.buckets[i]; requireValue(b.bucket === o.bucket && equal(b.before, o.after) && equal(b.after, o.before)); }); return;
    }
    const a = locations(before), b = locations(after);
    for (const id of union([...a.keys()], [...b.keys()])) {
      const x = copy(a.get(id)), y = b.get(id); if (x && y && equal(x, y)) continue;
      switch (n.action) {
        case "create": requireValue(!x && y && /^task:[a-f0-9]{32}$/.test(id) && id.length === 37); break;
        case "delete": requireValue(x && !y); break;
        case "edit": requireValue(x && y && x.bucket === y.bucket && x.task.title !== y.task.title); x.task.title = y.task.title; requireValue(equal(x, y)); break;
        case "complete": case "reopen": requireValue(x && y && x.bucket === y.bucket && x.task.completed !== y.task.completed && y.task.completed === (n.action === "complete")); x.task.completed = y.task.completed; requireValue(equal(x, y)); break;
        case "move": requireValue(x && y && (x.bucket !== y.bucket || x.task.due_date !== y.task.due_date)); x.bucket = y.bucket; x.task.due_date = y.task.due_date; requireValue(equal(x, y)); break;
        case "maintenance": requireValue(x); if (y) { x.bucket = y.bucket; x.task.due_date = y.task.due_date; requireValue(equal(x, y)); } break;
        default: requireValue(false);
      }
    }
    if (!["reorder", "maintenance"].includes(n.action)) for (const id of changed) requireValue(!a.has(id) || !b.has(id) || !equal(a.get(id), b.get(id)));
  }
  function conflictBuckets(a, b) {
    const x = writes(a), y = writes(b), ks = new Set();
    for (const k of keys(x)) if (Object.hasOwn(y, k) && !equal(x[k], y[k])) ks.add(k);
    for (const k of keys(x)) for (const l of keys(y)) if (k !== l) for (const t of x[k]) if (y[l].some(u => u.id === t.id)) { ks.add(k); ks.add(l); }
    if (a.kind === "change" && b.kind === "change" && intersects(keys(x), keys(y)) && !equal(a.buckets, b.buckets)) for (const k of union(keys(x), keys(y))) ks.add(k);
    if (ks.size) { if (a.kind === "change") for (const k of keys(x)) ks.add(k); if (b.kind === "change") for (const k of keys(y)) ks.add(k); }
    return [...ks].sort();
  }
  function projector() {
    const cache = new Map(); let visits = 0;
    function project(nodes) {
      const cacheKey = keys(nodes).join(","); if (cache.has(cacheKey)) return cache.get(cacheKey);
      if (++visits > MAX_VIEWS) throw new ReplayLimit();
      const v = empty(), active = new Set(Object.values(nodes).filter(n => n.kind === "activate").map(n => n.import));
      const events = Object.fromEntries(Object.entries(nodes).filter(([id, n]) => n.kind === "change" || n.kind === "import" && active.has(id))), ids = keys(events), comps = [];
      ids.forEach((a, i) => { for (const b of ids.slice(i + 1)) {
        const x = events[a], y = events[b]; if (x.ancestors.has(b) || y.ancestors.has(a)) continue;
        const buckets = conflictBuckets(x, y); if (!buckets.length) continue;
        if (!Object.values(events).some(r => r.action === "resolve" && r.ancestors.has(a) && r.ancestors.has(b) && buckets.every(k => Object.hasOwn(writes(r), k)))) comps.push({ buckets, ids: [a, b] });
      } });
      for (let changed = true; changed;) {
        changed = false;
        for (const c of comps) for (const id of ids) {
          const n = events[id]; if (n.kind !== "change" || !intersects(keys(writes(n)), c.buckets) || !c.ids.some(tip => n.ancestors.has(tip))) continue;
          const buckets = union(c.buckets, keys(writes(n))), members = union(c.ids, [id]);
          if (buckets.length !== c.buckets.length || members.length !== c.ids.length) { c.buckets = buckets; c.ids = members; changed = true; }
        }
        for (let i = 0; i < comps.length; i++) for (let j = i + 1; j < comps.length;) {
          if (intersects(comps[i].buckets, comps[j].buckets)) { comps[i].buckets = union(comps[i].buckets, comps[j].buckets); comps[i].ids = union(comps[i].ids, comps[j].ids); comps.splice(j, 1); changed = true; } else j++;
        }
      }
      const allBuckets = [...new Set(Object.values(events).flatMap(n => keys(writes(n))))].sort();
      for (const k of allBuckets) { const tips = maximal(ids.filter(id => Object.hasOwn(writes(events[id]), k)), nodes); if (tips.length) setBucket(v.common, k, writes(events[tips[0]])[k]); }
      comps.sort((a, b) => a.buckets[0] < b.buckets[0] ? -1 : 1);
      for (const c of comps) {
        const tips = maximal(c.ids, nodes), commonIDs = new Set([...nodes[tips[0]].ancestors].filter(id => tips.every(t => nodes[t].ancestors.has(id)))), commonNodes = subset(nodes, commonIDs);
        for (const [id, n] of Object.entries(nodes)) if (n.kind === "activate" && commonIDs.has(n.import)) commonNodes[id] = n;
        const common = project(commonNodes), baseline = activated(commonNodes) ? select(common.common, c.buckets) : null;
        const conflict = { buckets: c.buckets, alternatives: tips, common: baseline, candidates: [] };
        for (const id of tips) {
          const closure = subset(nodes, nodes[id].ancestors); closure[id] = nodes[id];
          if (nodes[id].kind === "import") for (const [aid, n] of Object.entries(nodes)) if (n.kind === "activate" && n.import === id) closure[aid] = n;
          conflict.candidates.push({ tip: id, snapshot: select(project(closure).common, c.buckets) });
        }
        for (const k of c.buckets) setBucket(v.common, k, baseline?.[k]); v.conflicts.push(conflict);
      }
      if (activated(nodes) && !v.conflicts.length) v.snapshot = copy(v.common);
      cache.set(cacheKey, v); return v;
    }
    return project;
  }
  // Pure rebuild. Callers retain durable records/outbox and their last verified
  // view; this module never writes storage, starts migration, or loads in app.js.
  async function rebuild(records, backups) {
    const nodes = Object.create(null), bad = Object.create(null), accepted = Object.create(null), pending = new Set(), project = projector();
    // Detach before the first await so asynchronous hashing cannot change input.
    records = copy(records); backups = copy(backups);
    for (const envelope of records) {
      try { const r = await P.parse(envelope.body); if (r.id !== envelope.id || r.body !== envelope.body) throw new Error("invalid envelope"); nodes[r.id] = { ...JSON.parse(r.body), ancestors: new Set() }; }
      catch { bad[envelope.id] = "invalid-record"; }
    }
    for (const id of keys(bad)) delete nodes[id]; for (const id of keys(nodes)) pending.add(id);
    for (;;) {
      let progress = false;
      for (const id of [...pending].sort()) {
        const n = nodes[id], deps = n.kind === "activate" ? [n.import] : n.parents || [];
        if (deps.some(dep => !accepted[dep])) continue;
        n.ancestors = new Set(deps.flatMap(dep => [dep, ...accepted[dep].ancestors])); let code = "";
        if (n.kind === "import") { try { P.validSnapshot(n.snapshot); } catch { code = "invalid-order"; } }
        else if (n.kind === "activate") {
          if (accepted[n.import].kind !== "import") code = "invalid-activation";
          else {
            if (!Object.hasOwn(backups, n.backup)) continue;
            try { const raw = backups[n.backup]; if (await P.hash(raw) !== n.backup || !equal(await P.normalizeLegacy(raw), accepted[n.import].snapshot)) code = "invalid-backup"; } catch { code = "invalid-backup"; }
          }
        } else {
          const closure = subset(accepted, n.ancestors), v = project(closure);
          if (!activated(closure) || !equal(maximal(n.parents, accepted), n.parents)) code = "invalid-parents";
          else { try { validateChange(n, v, accepted); } catch { code = "invalid-intent"; } }
        }
        pending.delete(id); progress = true; if (code) bad[id] = code; else accepted[id] = n;
      }
      if (!progress) break;
    }
    const v = copy(project(accepted)); v.pending = [...pending].sort(); v.issues = keys(bad).map(id => ({ id, code: bad[id] })); v.heads = maximal(keys(accepted), accepted);
    v.unique_imports = Object.values(accepted).filter(n => n.kind === "import").length; v.completion_events = Object.values(accepted).filter(n => n.action === "complete").length;
    return v;
  }
  async function replay(records, backups = {}) {
    records = copy(records); backups = copy(backups);
    const ids = [...new Set(records.map(r => r.id))].sort();
    try { if (ids.length > MAX_RECORDS) throw new ReplayLimit(); return await rebuild(records, backups); }
    catch (error) { if (!(error instanceof ReplayLimit)) throw error; return { ...empty(), pending: ids, issues: [{ id: "", code: "replay-limit" }] }; }
  }
  return { replay, MAX_RECORDS, MAX_VIEWS };
});
