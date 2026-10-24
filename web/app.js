(() => {
  "use strict";

  // ── Config ─────────────────────────────────────────────────────────
  const CFG = window.DOITDOIT_CONFIG || {};
  const SERVICE_ORIGIN = CFG.serviceOrigin || "https://doitdoit-service.davidt-91d.workers.dev";
  const VISIBLE_DAYS = Math.max(1, CFG.visibleDays || 5);
  const REDIRECT_URI = window.location.origin + window.location.pathname;
  const auth = window.DoitdoitAuth.createAuth({ origin: SERVICE_ORIGIN, redirect: REDIRECT_URI,
    storage: localStorage, session: sessionStorage, fetchImpl: fetch, locks: navigator.locks, cryptoImpl: crypto });
  const taskStore = window.DoitdoitSync.createServiceStore({ origin: SERVICE_ORIGIN,
    request: auth.request, storage: localStorage, locks: navigator.locks });

  // ── DOM refs ───────────────────────────────────────────────────────
  const $ = (id) => document.getElementById(id);
  const board = $("board");
  const connectEl = $("connect");
  const promptBar = $("prompt-bar");
  const addForm = $("add-form");
  const addInput = $("add-input");
  const syncEl = $("sync-indicator");
  const toasts = $("toasts");
  const emptyState = $("empty-state");
  const metaPath = $("meta-path");
  const menuBtn = $("btn-menu");
  const menuDialog = $("menu-dialog");
  const addSchedule = $("add-schedule");
  const addDate = $("add-date");
  const addDateLabel = $("add-date-label");
  const editDialog = $("edit-dialog");
  const editForm = $("edit-form");
  const editTitle = $("edit-title");
  const editSchedule = $("edit-schedule");
  const editDate = $("edit-date");
  const editDateLabel = $("edit-date-label");
  const dragStatus = $("drag-status");

  metaPath.textContent = "Shared tasks";

  // ── State ──────────────────────────────────────────────────────────
  const state = {
    data: null,         // TodoData = { "YYYY-MM-DD" | "Future": Task[] }
    accessToken: auth.connected(),
    timezone: "Europe/London",
    showHistory: false,
    online: false,
    dirty: false,
    saving: false,
    rendered: false,
    interactionActive: false,
    editing: null,
    conflict: false,
  };

  let addTarget = { kind: "today", date: "" };
  let editTarget = { kind: "today", date: "" };

  // ── Sync indicator ─────────────────────────────────────────────────
  function setSync(stateName, label) {
    syncEl.dataset.state = stateName;
    syncEl.textContent = label || (
      stateName === "idle" ? "Synced" :
      stateName === "syncing" ? "Syncing…" :
      stateName === "dirty" ? "Unsaved" :
      stateName === "error" ? (state.conflict ? "Conflict" : "Sync failed") : "Not connected"
    );
  }

  function toast(msg, kind) {
    const el = document.createElement("div");
    el.className = "toast" + (kind ? " toast--" + kind : "");
    el.textContent = msg;
    toasts.appendChild(el);
    setTimeout(() => {
      el.classList.add("toast--leaving");
      setTimeout(() => el.remove(), 220);
    }, 3200);
  }

  // ── localStorage helpers ───────────────────────────────────────────
  const LS = {
    get(k) { try { return JSON.parse(localStorage.getItem(k)); } catch { return null; } },
  };

  async function startOAuth() {
    try { window.location.assign(await auth.start()); }
    catch (err) { toast(err.message, "err"); }
  }
  async function logout() {
    if (state.saving || state.interactionActive) { toast("Finish the current action first.", "err"); return; }
    try {
      await auth.logout();
      sessionGeneration++;
      reloadGeneration++;
      state.accessToken = false;
      state.data = null;
      showConnect();
    } catch (err) { toast("Disconnect failed: " + err.message, "err"); }
  }

  const Domain = window.DoitdoitDomain;
  const { todayStr, parseDay, addDays } = Domain;
  function ownerToday() {
    const parts = new Intl.DateTimeFormat("en-CA", { timeZone: state.timezone, year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(new Date());
    const part = type => parts.find(p => p.type === type).value;
    return parseDay(`${part("year")}-${part("month")}-${part("day")}`);
  }
  const storageTarget = target => Domain.storageTarget(target, ownerToday());
  const targetForTask = (dayKey, task) => Domain.targetForTask(dayKey, task, ownerToday());
  const parseAddInput = (raw, target) => Domain.parseAddInput(raw, target, ownerToday());

  // ── View model + render ───────────────────────────────────────────
  const weekdayFormat = new Intl.DateTimeFormat(undefined, { weekday: "long" });
  const dayDateFormat = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });

  function buildView(data) {
    const today = ownerToday();
    const todayKey = todayStr(today);
    const days = [];

    const keys = new Set(Array.from({ length: VISIBLE_DAYS }, (_, i) => todayStr(addDays(today, i))));
    // Keep distant scheduled tasks reachable without filling every intervening day.
    for (const key of Object.keys(data)) {
      if (parseDay(key) && data[key].some(t => key >= todayKey || !t.completed || state.showHistory)) keys.add(key);
    }
    for (const key of [...keys].sort()) {
      const d = parseDay(key);
      const tasks = (data[key] || []).filter(t => key >= todayKey || !t.completed || state.showHistory).map(toTaskView.bind(null, key));
      const label = key < todayKey ? (tasks.some(t => !t.completed) ? "Overdue" : "Earlier") : key === todayKey ? "Today" : key === todayStr(addDays(today, 1)) ? "Tomorrow" : weekdayFormat.format(d);
      days.push({
        key,
        label,
        date: key < todayKey ? new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short", year: "numeric" }).format(d) : dayDateFormat.format(d),
        tasks,
        hasTasks: tasks.length > 0,
        count: tasks.length || "",
        cls: key === todayKey ? "today" : "future",
      });
    }

    const futureTasks = (data["Future"] || []).map(toTaskView.bind(null, "Future"));
    days.push({
      key: "Future",
      label: "Future",
      date: "",
      tasks: futureTasks,
      hasTasks: futureTasks.length > 0,
      count: futureTasks.length || "",
      cls: "future-bucket",
    });

    return { days, todayKey };
  }

  function toTaskView(dayKey, t) {
    return {
      id: String(t.id),
      title: t.title,
      hasNotes: typeof t.notes === "string" && t.notes.length > 0,
      completed: !!t.completed,
      mark: t.completed ? "✓" : "",
      due: dayKey === "Future" && parseDay(t.due_date) ? t.due_date : null,
      dayKey,
    };
  }

  function renderBoard(view) {
    const fragment = document.createDocumentFragment();
    for (const day of view.days) {
      const section = document.createElement("section");
      section.className = `day day--${day.cls}`;
      section.dataset.key = day.key;

      const heading = document.createElement("h2");
      heading.className = "day__head";
      const label = document.createElement("span");
      label.className = "day__label";
      label.textContent = day.label;
      const count = document.createElement("span");
      count.className = "day__count";
      count.textContent = day.count;
      const date = document.createElement("span");
      date.className = "day__date";
      date.textContent = day.date;
      heading.append(label, count, date);
      section.append(heading);

      const list = document.createElement("ul");
      list.className = "tasks";
      for (const task of day.tasks) {
        const row = document.createElement("li");
        row.className = "task" + (task.completed ? " task--done" : "") + (task.hasNotes ? " task--has-notes" : "");
        row.dataset.id = task.id;
        row.dataset.key = task.dayKey;
        const toggle = document.createElement("button");
        toggle.className = "task__check";
        toggle.dataset.action = "toggle";
        toggle.setAttribute("aria-label", `toggle complete: ${task.title}`);
        toggle.setAttribute("aria-pressed", String(task.completed));
        const mark = document.createElement("span");
        mark.className = "task__mark";
        mark.setAttribute("aria-hidden", "true");
        mark.textContent = task.mark;
        toggle.append(mark);
        const title = document.createElement("button");
        title.className = "task__title";
        title.dataset.action = "edit";
        title.setAttribute("aria-label", `edit ${task.title}`);
        title.textContent = task.title;
        if (task.due) {
          const due = document.createElement("span");
          due.className = "task__due";
          const dateText = new Intl.DateTimeFormat(undefined, {
            day: "numeric", month: "short", year: "numeric",
          }).format(parseDay(task.due));
          due.textContent = dateText;
          title.append(due);
          title.setAttribute("aria-label", `edit ${task.title}, scheduled for ${dateText}`);
        }
        const drag = document.createElement("button");
        drag.className = "task__drag";
        drag.dataset.action = "drag";
        drag.setAttribute("aria-label", `reorder ${task.title}`);
        drag.setAttribute("aria-pressed", "false");
        drag.textContent = "≡";
        row.append(toggle, title);
        if (task.hasNotes) {
          const notes = document.createElement("button");
          notes.className = "task__notes-button";
          notes.type = "button";
          notes.dataset.action = "notes";
          notes.setAttribute("aria-label", `view notes for ${task.title}`);
          notes.setAttribute("aria-haspopup", "dialog");
          notes.title = "View notes";
          const icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
          icon.setAttribute("viewBox", "0 0 24 24");
          icon.setAttribute("width", "19");
          icon.setAttribute("height", "19");
          icon.setAttribute("fill", "none");
          icon.setAttribute("stroke", "currentColor");
          icon.setAttribute("stroke-width", "1.6");
          icon.setAttribute("stroke-linecap", "round");
          icon.setAttribute("stroke-linejoin", "round");
          icon.setAttribute("aria-hidden", "true");
          const lines = document.createElementNS("http://www.w3.org/2000/svg", "path");
          lines.setAttribute("d", "M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9l-6-6Z M14 3v6h6 M8 13h8 M8 17h5");
          icon.append(lines);
          notes.append(icon);
          row.append(notes);
        }
        row.append(drag);
        list.append(row);
      }
      section.append(list);
      const empty = document.createElement("div");
      empty.className = "day__empty" + (day.hasTasks ? " is-hidden" : "");
      empty.textContent = day.key === "Future" ? "Room for what’s next." : "Nothing planned.";
      section.append(empty);
      fragment.append(section);
    }
    board.replaceChildren(fragment);
  }

  function render(opts = {}) {
    if (!state.data) return;
    const preserveScroll = opts.preserveScroll !== false && state.rendered;
    const scrollY = preserveScroll ? window.scrollY : 0;
    const view = buildView(state.data);
    board.classList.toggle("board--animate", !!opts.animate);
    renderBoard(view);
    state.rendered = true;

    // Empty state if literally no tasks anywhere
    const totalTasks = view.days.reduce((n, d) => n + d.tasks.length, 0);
    emptyState.hidden = totalTasks > 0;
    if (preserveScroll) requestAnimationFrame(() => window.scrollTo(0, scrollY));
  }

  function daySection(dayKey) {
    return Array.from(board.querySelectorAll(".day")).find((el) => el.dataset.key === dayKey) || null;
  }

  function updateDayChrome(dayKey) {
    const section = daySection(dayKey);
    if (!section) return;
    const count = (state.data[dayKey] || []).length;
    section.querySelector(".day__count").textContent = count || "";
    section.querySelector(".day__empty").classList.toggle("is-hidden", count > 0);
    const total = buildView(state.data).days.reduce((n, d) => n + d.tasks.length, 0);
    emptyState.hidden = total > 0;
  }

  // ── Mutations ─────────────────────────────────────────────────────
  let reloadGeneration = 0;
  let sessionGeneration = 0;
  let lastMove = null;

  function canEdit() {
    if (!state.accessToken || !state.data || !state.online) {
      toast("Refresh the service connection before editing.", "err");
      return false;
    }
    if (state.saving || taskStore.pending()) {
      toast("Finish or retry the retained save from the menu before editing.", "err");
      return false;
    }
    if (navigator.onLine === false) { toast("Connect to the internet before editing.", "err"); return false; }
    return true;
  }
  function pendingStatus() {
    const pending = taskStore.pending();
    state.dirty = !!pending;
    state.conflict = !!pending?.rejected;
    setSync(pending ? "error" : "idle", pending ? (pending.rejected ? "Save rejected · menu to review" : "Save retained · menu to retry") : undefined);
  }
  async function saveCommand(method, path, body) {
    if (!canEdit()) return false;
    state.saving = true;
    state.online = false;
    reloadGeneration++;
    setSync("syncing", "Saving…");
    try {
      await taskStore.mutate(method, path, body);
      return true;
    } catch (err) { toast(err.message + " — open the menu to resolve the save.", "err"); return false; }
    finally { state.saving = false; pendingStatus(); await reload({ silent: true }); }
  }
  async function addTask(rawInput, selectedTarget) {
    const parsed = parseAddInput(rawInput, selectedTarget);
    if (parsed.error) { toast(parsed.error, "err"); return false; }
    return saveCommand("POST", "/v1/tasks", { title: parsed.title, scheduled_date: parsed.key === "Future" ? null : parsed.key });
  }
  function findTask(dayKey, id) { return Domain.findTask(state.data, dayKey, id); }
  function toggleTask(dayKey, id) {
    const found = findTask(dayKey, id);
    if (found) return saveCommand("PATCH", "/v1/tasks/" + id, { expected_revision: found.task.revision, changes: { status: found.task.completed ? "open" : "completed" } });
  }
  function deleteTask(dayKey, id) {
    const found = findTask(dayKey, id);
    if (found) return saveCommand("DELETE", "/v1/tasks/" + id, { expected_revision: found.task.revision });
  }
  // Dragging is a preview. Only the final reviewed task and neighbors are sent.
  function moveTask(dayKey, id, destinationKey, destinationIndex) {
    if (!findTask(dayKey, id)) return false;
    lastMove = { sourceKey: lastMove?.sourceKey || dayKey, id, destinationKey };
    return Domain.moveTask(state.data, dayKey, id, destinationKey, destinationIndex);
  }
  async function queueSave() {
    const move = lastMove;
    lastMove = null;
    if (!move) return;
    const found = findTask(move.destinationKey, move.id);
    if (!found) return;
    const task = found.task;
    if (move.sourceKey !== move.destinationKey) {
      await saveCommand("PATCH", "/v1/tasks/" + move.id, { expected_revision: task.revision, changes: { scheduled_date: move.destinationKey === "Future" ? null : move.destinationKey } });
    } else {
      const group = found.list.filter(t => t.completed === task.completed);
      const index = group.findIndex(t => t.id === move.id);
      await saveCommand("POST", "/v1/tasks/" + move.id + "/reorder", { expected_revision: task.revision, after_id: group[index - 1]?.id || null, before_id: group[index + 1]?.id || null });
    }
  }
  async function retrySave() {
    if (state.saving || state.interactionActive) return;
    state.saving = true;
    state.online = false;
    reloadGeneration++;
    setSync("syncing", "Retrying save…");
    try {
      const pending = taskStore.pending();
      await taskStore.retry();
      if (pending?.method === "POST" && pending.path === "/v1/tasks") {
        const input = parseAddInput(addInput.value, addTarget);
        if (input.title === pending.body.title && (input.key === "Future" ? null : input.key) === pending.body.scheduled_date) addInput.value = "";
      }
    }
    catch (err) { toast(err.message, "err"); }
    finally { state.saving = false; pendingStatus(); await reload({ silent: true }); }
  }
  async function reload(opts = {}) {
    if (state.interactionActive || !state.accessToken || state.saving) return;
    const request = ++reloadGeneration;
    const session = sessionGeneration;
    setSync("syncing");
    try {
      const { data, timezone } = await taskStore.load();
      if (request !== reloadGeneration || session !== sessionGeneration || state.saving || state.interactionActive) return;
      state.data = data;
      state.online = true;
      state.timezone = timezone;
      render({ animate: !state.rendered, preserveScroll: state.rendered });
      pendingStatus();
      if (opts.confirm) toast("Refreshed", "ok");
    } catch (err) {
      if (request !== reloadGeneration || session !== sessionGeneration) return;
      state.online = false;
      toast("Load failed: " + err.message, "err");
      setSync("error", "Disconnected · edits paused");
    }
  }

  // ── UI wiring ─────────────────────────────────────────────────────
  function shortDateLabel(value) {
    const date = parseDay(value);
    if (!date) return "Date…";
    return new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" }).format(date);
  }

  function paintSchedule(root, attribute, target, dateInput, dateLabel) {
    root.querySelectorAll(`[${attribute}]`).forEach((button) => {
      const selected = button.getAttribute(attribute) === target.kind;
      button.classList.toggle("is-selected", selected);
      button.setAttribute("aria-pressed", String(selected));
    });
    const dateChip = dateInput.closest(".schedule__date");
    dateChip.classList.toggle("is-selected", target.kind === "custom");
    dateInput.value = target.date || "";
    dateLabel.textContent = target.kind === "custom" && target.date ? shortDateLabel(target.date) : "Date…";
  }

  function chooseSchedule(kind, scope) {
    if (scope === "add") {
      addTarget = { kind, date: kind === "custom" ? addDate.value : "" };
      paintSchedule(addSchedule, "data-add-schedule", addTarget, addDate, addDateLabel);
    } else {
      editTarget = { kind, date: kind === "custom" ? editDate.value : "" };
      paintSchedule(editSchedule, "data-edit-schedule", editTarget, editDate, editDateLabel);
    }
  }

  function openEditor(dayKey, id) {
    if (!canEdit()) return;
    const found = findTask(dayKey, id);
    if (!found) return;
    state.editing = { dayKey, id };
    state.interactionActive = true;
    editTitle.value = found.task.title;
    editTarget = targetForTask(dayKey, found.task);
    paintSchedule(editSchedule, "data-edit-schedule", editTarget, editDate, editDateLabel);
    if (typeof editDialog.showModal === "function") editDialog.showModal();
    else editDialog.setAttribute("open", "");
    requestAnimationFrame(() => editTitle.focus({ preventScroll: true }));
  }

  function focusTaskTitle(dayKey, id) {
    requestAnimationFrame(() => {
      const row = Array.from(board.querySelectorAll(".task")).find(
        (candidate) => candidate.dataset.key === dayKey && candidate.dataset.id === String(id)
      );
      row?.querySelector(".task__title")?.focus({ preventScroll: true });
    });
  }

  function closeEditor(returnTarget = state.editing) {
    state.editing = null;
    state.interactionActive = false;
    if (editDialog.open && typeof editDialog.close === "function") editDialog.close();
    else editDialog.removeAttribute("open");
    if (returnTarget) focusTaskTitle(returnTarget.dayKey, returnTarget.id);
  }

  function saveEditor() {
    if (!state.editing || !canEdit()) return;
    const title = editTitle.value.trim();
    if (!title) { toast("task title cannot be empty", "err"); editTitle.focus(); return; }
    const destination = storageTarget(editTarget);
    if (destination.error) { toast(destination.error, "err"); return; }
    const { dayKey, id } = state.editing;
    const found = findTask(dayKey, id);
    if (!found) return;
    const revision = found.task.revision;
    closeEditor();
    void saveCommand("PATCH", "/v1/tasks/" + id, { expected_revision: revision, changes: { title, scheduled_date: destination.key === "Future" ? null : destination.key } });
  }

  let pointerDrag = null;
  let keyboardDrag = null;
  let dragScrollFrame = null;

  function announceDrag(message) {
    dragStatus.textContent = "";
    requestAnimationFrame(() => { dragStatus.textContent = message; });
  }

  function taskLabel(dayKey, id) {
    return findTask(dayKey, id)?.task.title || "task";
  }

  function beginDrag(handle, clientX, clientY, input) {
    if (!canEdit()) return;
    lastMove = null;
    const row = handle.closest(".task");
    if (!row || pointerDrag || keyboardDrag) return;
    const rect = row.getBoundingClientRect();
    const ghost = row.cloneNode(true);
    ghost.classList.add("task-drag-ghost");
    ghost.setAttribute("aria-hidden", "true");
    ghost.style.width = rect.width + "px";
    ghost.style.left = rect.left + "px";
    ghost.style.top = rect.top + "px";
    const placeholder = document.createElement("li");
    placeholder.className = "task task--placeholder";
    placeholder.style.height = rect.height + "px";
    row.parentNode.insertBefore(placeholder, row);
    row.classList.add("task--dragging");
    document.body.appendChild(ghost);
    board.classList.add("is-dragging");
    state.interactionActive = true;
    pointerDrag = {
      id: row.dataset.id,
      sourceKey: row.dataset.key,
      row,
      ghost,
      placeholder,
      offsetX: clientX - rect.left,
      offsetY: clientY - rect.top,
      lastX: clientX,
      lastY: clientY,
      input,
    };
    dragScrollFrame = requestAnimationFrame(scrollWhileDragging);
    announceDrag(`Picked up ${taskLabel(row.dataset.key, row.dataset.id)}`);
  }

  function beginPointerDrag(e, handle) {
    if (e.pointerType === "touch" || (e.pointerType === "mouse" && e.button !== 0)) return;
    e.preventDefault();
    beginDrag(handle, e.clientX, e.clientY, "pointer");
    handle.setPointerCapture?.(e.pointerId);
  }

  function placeDrag(clientX, clientY) {
    if (!pointerDrag) return;
    const drag = pointerDrag;
    drag.lastX = clientX;
    drag.lastY = clientY;
    drag.ghost.style.left = (clientX - drag.offsetX) + "px";
    drag.ghost.style.top = (clientY - drag.offsetY) + "px";

    const contentTop = document.querySelector(".hdr").getBoundingClientRect().bottom + 8;
    const contentBottom = (promptBar.hidden ? window.innerHeight : promptBar.getBoundingClientRect().top) - 8;
    const hitY = Math.max(contentTop, Math.min(clientY, contentBottom));
    const under = document.elementFromPoint(clientX, hitY);
    const section = under?.closest?.(".day");
    if (!section) return;
    const list = section.querySelector(".tasks");
    const rows = Array.from(list.children).filter(
      (candidate) => candidate !== drag.row && candidate !== drag.placeholder
    );
    const before = rows.find((candidate) => hitY < candidate.getBoundingClientRect().top + candidate.offsetHeight / 2);
    if (before) list.insertBefore(drag.placeholder, before);
    else list.appendChild(drag.placeholder);
  }

  function scrollWhileDragging() {
    if (!pointerDrag) return;
    const edge = 88;
    const contentTop = document.querySelector(".hdr").getBoundingClientRect().bottom;
    const contentBottom = promptBar.hidden ? window.innerHeight : promptBar.getBoundingClientRect().top;
    let delta = 0;
    if (pointerDrag.lastY < contentTop + edge) delta = -12;
    else if (pointerDrag.lastY > contentBottom - edge) delta = 12;
    if (delta) {
      window.scrollBy(0, delta);
      placeDrag(pointerDrag.lastX, pointerDrag.lastY);
    }
    dragScrollFrame = requestAnimationFrame(scrollWhileDragging);
  }

  function movePointerDrag(e) {
    if (!pointerDrag || pointerDrag.input !== "pointer") return;
    e.preventDefault();
    placeDrag(e.clientX, e.clientY);
  }

  function beginTouchDrag(e, handle) {
    if (e.touches.length !== 1) return;
    e.preventDefault();
    const touch = e.touches[0];
    beginDrag(handle, touch.clientX, touch.clientY, "touch");
  }

  function moveTouchDrag(e) {
    if (!pointerDrag || pointerDrag.input !== "touch" || !e.touches.length) return;
    e.preventDefault();
    const touch = e.touches[0];
    placeDrag(touch.clientX, touch.clientY);
  }

  function finishPointerDrag(cancelled) {
    if (!pointerDrag) return;
    const drag = pointerDrag;
    let destinationKey = drag.sourceKey;
    let destinationIndex = 0;
    if (!cancelled) {
      const section = drag.placeholder.closest(".day");
      destinationKey = section?.dataset.key || drag.sourceKey;
      destinationIndex = Array.from(drag.placeholder.parentNode.children)
        .filter((candidate) => candidate !== drag.row && candidate !== drag.placeholder)
        .filter((candidate) => candidate.compareDocumentPosition(drag.placeholder) & Node.DOCUMENT_POSITION_FOLLOWING)
        .length;
    }
    drag.ghost.remove();
    drag.placeholder.remove();
    drag.row.classList.remove("task--dragging");
    board.classList.remove("is-dragging");
    if (dragScrollFrame) cancelAnimationFrame(dragScrollFrame);
    dragScrollFrame = null;
    pointerDrag = null;
    state.interactionActive = false;
    if (!cancelled && moveTask(drag.sourceKey, drag.id, destinationKey, destinationIndex)) {
      render({ preserveScroll: true });
      queueSave();
      announceDrag(`Moved ${taskLabel(destinationKey, drag.id)} to ${destinationKey === "Future" ? "Future" : destinationKey}`);
    } else {
      announceDrag("Move cancelled");
    }
  }

  function focusDragHandle(dayKey, id, grabbed) {
    requestAnimationFrame(() => {
      const row = Array.from(board.querySelectorAll(".task")).find(
        (candidate) => candidate.dataset.key === dayKey && candidate.dataset.id === String(id)
      );
      const handle = row?.querySelector(".task__drag");
      if (handle) {
        handle.setAttribute("aria-pressed", String(!!grabbed));
        handle.focus({ preventScroll: true });
      }
    });
  }

  function beginKeyboardDrag(row) {
    if (!canEdit()) return;
    lastMove = null;
    keyboardDrag = {
      id: row.dataset.id,
      dayKey: row.dataset.key,
      sourceKey: row.dataset.key,
      snapshot: JSON.stringify(state.data),
    };
    state.interactionActive = true;
    row.querySelector(".task__drag").setAttribute("aria-pressed", "true");
    announceDrag(`Picked up ${taskLabel(keyboardDrag.dayKey, keyboardDrag.id)}. Use arrow keys to move.`);
  }

  function keyboardMove(key) {
    const drag = keyboardDrag;
    const found = findTask(drag.dayKey, drag.id);
    if (!found) return;
    let destinationKey = drag.dayKey;
    let destinationIndex = found.idx;
    if (key === "ArrowUp") destinationIndex--;
    else if (key === "ArrowDown") destinationIndex++;
    else {
      const keys = Array.from(board.querySelectorAll(".day")).map((section) => section.dataset.key);
      const sectionIndex = keys.indexOf(drag.dayKey) + (key === "ArrowLeft" ? -1 : 1);
      if (sectionIndex < 0 || sectionIndex >= keys.length) return;
      destinationKey = keys[sectionIndex];
      destinationIndex = Math.min(found.idx, (state.data[destinationKey] || []).length);
    }
    if (destinationKey === drag.dayKey && (destinationIndex < 0 || destinationIndex >= found.list.length)) return;
    if (!moveTask(drag.dayKey, drag.id, destinationKey, destinationIndex)) return;
    drag.dayKey = destinationKey;
    render({ preserveScroll: true });
    focusDragHandle(drag.dayKey, drag.id, true);
    announceDrag(`Moved to ${destinationKey === "Future" ? "Future" : destinationKey}, position ${destinationIndex + 1}`);
  }

  function finishKeyboardDrag(cancelled) {
    if (!keyboardDrag) return;
    const drag = keyboardDrag;
    if (cancelled) {
      lastMove = null;
      state.data = JSON.parse(drag.snapshot);
      render({ preserveScroll: true });
      announceDrag("Move cancelled");
    } else {
      state.interactionActive = false;
      queueSave();
      announceDrag("Saving task position");
    }
    keyboardDrag = null;
    state.interactionActive = false;
    focusDragHandle(cancelled ? drag.sourceKey : drag.dayKey, drag.id, false);
  }

  function syncPromptHeight() {
    if (promptBar.hidden) return;
    const height = Math.ceil(promptBar.getBoundingClientRect().height);
    document.documentElement.style.setProperty("--prompt-h", height + "px");
  }

  function showBoard() {
    connectEl.hidden = true;
    board.hidden = false;
    promptBar.hidden = false;
    menuBtn.hidden = false;
    requestAnimationFrame(syncPromptHeight);
  }
  function showConnect() {
    setSync("disconnected");
    connectEl.hidden = false;
    board.hidden = true;
    promptBar.hidden = true;
    menuBtn.hidden = !LS.get("doitdoit:recovery") && !taskStore.recovery();
    emptyState.hidden = true;
  }

  function downloadRecovery() {
    const service = taskStore.recovery();
    const legacy = LS.get("doitdoit:recovery");
    const recovery = service || legacy ? { service, legacy } : null;
    if (!recovery) {
      toast("no unsaved recovery copy", "err");
      return;
    }
    const blob = new Blob([JSON.stringify(recovery, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "doitdoit-recovery.json";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
    toast("recovery copy downloaded", "ok");
  }

  // delegated click handler for tasks
  const notesDialog = $("notes-dialog");
  const notesContent = $("notes-content");
  let notesTrigger = null;
  function openNotes(dayKey, id, trigger) {
    const found = Domain.findTask(state.data, dayKey, id);
    if (!found || typeof found.task.notes !== "string" || !found.task.notes) return;
    $("notes-task-title").textContent = found.task.title;
    notesContent.textContent = found.task.notes;
    notesTrigger = trigger;
    notesDialog.showModal();
    notesContent.scrollTop = 0;
  }
  $("notes-close").addEventListener("click", () => notesDialog.close());
  notesDialog.addEventListener("close", () => {
    notesContent.textContent = "";
    $("notes-task-title").textContent = "";
    if (notesTrigger?.isConnected) notesTrigger.focus({ preventScroll: true });
    notesTrigger = null;
  });

  board.addEventListener("click", (e) => {
    const btn = e.target.closest("button[data-action]");
    if (!btn) return;
    const li = btn.closest(".task");
    if (!li) return;
    const id = li.dataset.id;
    const dayKey = li.dataset.key;
    const action = btn.dataset.action;
    if (action === "toggle") toggleTask(dayKey, id);
    else if (action === "edit") openEditor(dayKey, id);
    else if (action === "notes") openNotes(dayKey, id, btn);
    else if (action === "delete") {
      li.classList.add("task--exit");
      // wait for exit animation, then mutate
      setTimeout(() => deleteTask(dayKey, id), 200);
    }
  });

  board.addEventListener("pointerdown", (e) => {
    const handle = e.target.closest(".task__drag");
    if (handle) beginPointerDrag(e, handle);
  });
  board.addEventListener("touchstart", (e) => {
    const handle = e.target.closest(".task__drag");
    if (handle) beginTouchDrag(e, handle);
  }, { passive: false });
  window.addEventListener("pointermove", movePointerDrag, { passive: false });
  window.addEventListener("pointerup", () => {
    if (pointerDrag?.input === "pointer") finishPointerDrag(false);
  });
  window.addEventListener("pointercancel", () => {
    if (pointerDrag?.input === "pointer") finishPointerDrag(true);
  });
  window.addEventListener("touchmove", moveTouchDrag, { passive: false });
  window.addEventListener("touchend", () => {
    if (pointerDrag?.input === "touch") finishPointerDrag(false);
  });
  window.addEventListener("touchcancel", () => {
    if (pointerDrag?.input === "touch") finishPointerDrag(true);
  });

  board.addEventListener("keydown", (e) => {
    const handle = e.target.closest(".task__drag");
    if (!handle) return;
    const row = handle.closest(".task");
    if ((e.key === " " || e.key === "Enter") && !keyboardDrag) {
      e.preventDefault();
      beginKeyboardDrag(row);
    } else if ((e.key === " " || e.key === "Enter") && keyboardDrag) {
      e.preventDefault();
      finishKeyboardDrag(false);
    } else if (e.key === "Escape" && keyboardDrag) {
      e.preventDefault();
      finishKeyboardDrag(true);
    } else if (keyboardDrag && ["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"].includes(e.key)) {
      e.preventDefault();
      keyboardMove(e.key);
    }
  });

  addForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const v = addInput.value;
    if (!v.trim()) return;
    if (await addTask(v, addTarget) && addInput.value === v) addInput.value = "";
  });

  addSchedule.addEventListener("click", (e) => {
    const button = e.target.closest("[data-add-schedule]");
    if (button) chooseSchedule(button.dataset.addSchedule, "add");
  });
  addDate.addEventListener("change", () => {
    if (!addDate.value) return;
    addTarget = { kind: "custom", date: addDate.value };
    paintSchedule(addSchedule, "data-add-schedule", addTarget, addDate, addDateLabel);
  });

  editSchedule.addEventListener("click", (e) => {
    const button = e.target.closest("[data-edit-schedule]");
    if (button) chooseSchedule(button.dataset.editSchedule, "edit");
  });
  editDate.addEventListener("change", () => {
    if (!editDate.value) return;
    editTarget = { kind: "custom", date: editDate.value };
    paintSchedule(editSchedule, "data-edit-schedule", editTarget, editDate, editDateLabel);
  });
  editForm.addEventListener("submit", (e) => {
    e.preventDefault();
    saveEditor();
  });
  editDialog.addEventListener("click", (e) => {
    const action = e.target.closest("[data-edit-action]")?.dataset.editAction;
    if (action === "cancel") closeEditor();
    else if (action === "delete" && state.editing) {
      const { dayKey, id } = state.editing;
      closeEditor();
      deleteTask(dayKey, id);
    }
    else if (e.target === editDialog) closeEditor();
  });
  editDialog.addEventListener("cancel", (e) => {
    e.preventDefault();
    closeEditor();
  });

  $("btn-connect").addEventListener("click", startOAuth);

  // Menu
  menuBtn.addEventListener("click", () => {
    if (typeof menuDialog.showModal === "function") menuDialog.showModal();
    else menuDialog.setAttribute("open", "");
  });
  menuDialog.addEventListener("click", async (e) => {
    const item = e.target.closest("[data-act]");
    if (!item) return;
    const act = item.dataset.act;
    if (act === "close") menuDialog.close();
    else if (act === "reload") {
      menuDialog.close();
      reload({ confirm: true });
    }
    else if (act === "retry") { menuDialog.close(); void retrySave(); }
    else if (act === "review") {
      menuDialog.close();
      try {
        await taskStore.reviewRejected();
        state.online = false;
        pendingStatus();
        void reload({ confirm: true });
        toast("Rejected save kept in recovery. Review the current task before editing again.");
      } catch (err) { toast(err.message, "err"); }
    }
    else if (act === "history") { state.showHistory = !state.showHistory; menuDialog.close(); render(); }
    else if (act === "recovery") { menuDialog.close(); downloadRecovery(); }
    else if (act === "signin") { menuDialog.close(); if (!state.saving && !state.interactionActive) void startOAuth(); }
    else if (act === "logout") {
      if (confirm("Disconnect this browser? Retained saves and recovery copies stay on this device.")) {
        void logout();
      }
      menuDialog.close();
    }
  });
  // close on backdrop click
  menuDialog.addEventListener("click", (e) => {
    const rect = menuDialog.getBoundingClientRect();
    if (e.clientX < rect.left || e.clientX > rect.right ||
        e.clientY < rect.top  || e.clientY > rect.bottom) {
      menuDialog.close();
    }
  });

  // Background sync — pick up CLI changes
  window.addEventListener("focus", () => {
    if (state.accessToken && !state.dirty && document.visibilityState === "visible") {
      reload({ silent: true });
    }
  });
  setInterval(() => {
    if (state.accessToken && !state.dirty && document.visibilityState === "visible") {
      reload({ silent: true });
    }
  }, 15_000);

  // Keyboard shortcut: `/` focuses input (when not already typing)
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && pointerDrag) {
      e.preventDefault();
      finishPointerDrag(true);
      return;
    }
    if (e.key === "/" && document.activeElement !== addInput) {
      if (state.interactionActive) return;
      const tag = document.activeElement?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;
      e.preventDefault();
      addInput.focus();
    }
  });

  // ── Boot ──────────────────────────────────────────────────────────
  async function boot() {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    const oauthErr = params.get("error");

    if (code || oauthErr) {
      window.history.replaceState({}, "", REDIRECT_URI);
      try { await auth.callback(params); }
      catch (err) { toast(err.message, "err"); showConnect(); return; }
    }
    state.accessToken = auth.connected();
    if (!state.accessToken) { showConnect(); return; }
    showBoard();
    await reload({ silent: true });
  }

  // expose minimal debug surface
  window.doitdoit = { reload, logout, state, storageTarget, parseAddInput, retrySave };

  paintSchedule(addSchedule, "data-add-schedule", addTarget, addDate, addDateLabel);
  if ("ResizeObserver" in window) new ResizeObserver(syncPromptHeight).observe(promptBar);
  window.addEventListener("resize", syncPromptHeight);
  boot();
})();
