(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.DoitdoitDomain = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function todayStr(d = new Date()) {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
  }
  function parseDay(s) {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s || "");
    if (!m) return null;
    const d = new Date(+m[1], +m[2] - 1, +m[3]);
    return todayStr(d) === s ? d : null;
  }
  function addDays(d, n) {
    const x = new Date(d);
    x.setDate(x.getDate() + n);
    return x;
  }
  function startOfDay(d) { return new Date(d.getFullYear(), d.getMonth(), d.getDate()); }

  function groupTasksByCompletion(data) {
    let changed = false;
    for (const key of Object.keys(data)) {
      const tasks = data[key] || [];
      let seenCompleted = false;
      let needsGrouping = false;
      for (const task of tasks) {
        if (task.completed) seenCompleted = true;
        else if (seenCompleted) { needsGrouping = true; break; }
      }
      if (needsGrouping) {
        data[key] = tasks.filter((task) => !task.completed)
          .concat(tasks.filter((task) => task.completed));
        changed = true;
      }
    }
    return changed;
  }

  function storageTarget(target, visibleDays, now = new Date()) {
    if (target.kind === "future") return { key: "Future", due: "" };
    let date = target.kind === "tomorrow"
      ? addDays(now, 1)
      : target.kind === "custom" ? parseDay(target.date) : now;
    if (!date) return { error: "choose a valid date" };
    date = startOfDay(date);
    const today = startOfDay(now);
    if (date < today) date = today;
    const due = todayStr(date);
    return { key: date > addDays(today, visibleDays - 1) ? "Future" : due, due };
  }

  function targetForTask(dayKey, task, now = new Date()) {
    if (dayKey === "Future" && !task.due_date) return { kind: "future", date: "" };
    const due = task.due_date || dayKey;
    if (due === todayStr(now)) return { kind: "today", date: due };
    if (due === todayStr(addDays(now, 1))) return { kind: "tomorrow", date: due };
    return { kind: "custom", date: due };
  }

  function rollOverIncompleteTasks(data, now = new Date()) {
    const today = todayStr(now);
    const todayDate = startOfDay(now);
    const toRoll = [];
    const datesToRemove = [];
    let changed = false;
    for (const dateStr of Object.keys(data)) {
      if (dateStr === "Future") continue;
      const parsed = parseDay(dateStr);
      if (!parsed || parsed >= todayDate) continue;
      const remaining = [];
      for (const task of data[dateStr]) {
        if (!task.completed) { task.due_date = today; toRoll.push(task); }
        else remaining.push(task);
      }
      if (remaining.length) data[dateStr] = remaining;
      else datesToRemove.push(dateStr);
    }
    if (toRoll.length) { data[today] = (data[today] || []).concat(toRoll); changed = true; }
    for (const date of datesToRemove) { delete data[date]; changed = true; }
    if (data[today] && data[today].length === 0) delete data[today];
    if (groupTasksByCompletion(data)) changed = true;
    return changed;
  }

  function pruneOldTasks(data, retentionDays, now = new Date()) {
    if (!Number.isInteger(retentionDays) || retentionDays <= 0) return false;
    const cutoff = todayStr(addDays(now, -retentionDays));
    let changed = false;
    for (const key of Object.keys(data)) {
      if (key === "Future") {
        const tasks = data[key] || [];
        const active = tasks.filter((task) => !task.completed);
        if (active.length !== tasks.length) { data[key] = active; changed = true; }
      } else if (key < cutoff) { delete data[key]; changed = true; }
    }
    return changed;
  }

  function distributeFutureTasks(data, visibleDays, now = new Date()) {
    const future = data.Future || [];
    if (!future.length) return false;
    const today = startOfDay(now);
    const lastVisible = addDays(today, visibleDays - 1);
    const remain = [];
    let changed = false;
    for (const task of future) {
      const due = parseDay(task.due_date);
      if (!due || due > lastVisible) { remain.push(task); continue; }
      const target = due < today ? todayStr(today) : task.due_date;
      (data[target] || (data[target] = [])).push(task);
      changed = true;
    }
    data.Future = remain;
    if (groupTasksByCompletion(data)) changed = true;
    return changed;
  }

  function parseAddInput(raw, selectedTarget, visibleDays, now = new Date()) {
    let title = raw.trim();
    let target = selectedTarget;
    const match = /^!(\S+)\s+(.+)$/.exec(title);
    if (match) {
      const prefix = match[1].toLowerCase();
      title = match[2].trim();
      if (prefix === "future") target = { kind: "future", date: "" };
      else if (/^\d{4}-\d{2}-\d{2}$/.test(prefix)) target = { kind: "custom", date: prefix };
      else return { error: "unknown target — use !future or !YYYY-MM-DD" };
    }
    if (!title) return { error: "task title cannot be empty" };
    const destination = storageTarget(target, visibleDays, now);
    if (destination.error) return destination;
    return { title, key: destination.key, due: destination.due };
  }

  function insertBeforeCompleted(list, task) {
    const index = list.findIndex((candidate) => candidate.completed);
    list.splice(index < 0 ? list.length : index, 0, task);
  }

  return { todayStr, parseDay, addDays, startOfDay, storageTarget, targetForTask,
    rollOverIncompleteTasks, pruneOldTasks, distributeFutureTasks, parseAddInput,
    insertBeforeCompleted, groupTasksByCompletion };
});
