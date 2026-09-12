/* doitdoit web companion — vanilla JS app.
 *
 * Talks to Dropbox HTTP API directly from the browser via OAuth 2.0 PKCE.
 * Reads/writes a single JSON file. Ports the rollover + prune logic from
 * model/task.go so the web app and CLI agree on the data lifecycle.
 *
 * No bundler, no framework. Loaded as a regular script (not a module) so
 * that config.js can expose `window.DOITDOIT_CONFIG` synchronously.
 */
(() => {
  "use strict";

  // ── Config ─────────────────────────────────────────────────────────
  const CFG = window.DOITDOIT_CONFIG || {};
  const APP_KEY = CFG.dropboxAppKey || "";
  const FILE_PATH = CFG.dropboxFilePath || "/doitdoit.json";
  const VISIBLE_DAYS = Math.max(1, CFG.visibleDays || 5);
  const RETENTION_DAYS = Number.isInteger(CFG.retentionDays) && CFG.retentionDays > 0
    ? CFG.retentionDays : 0;
  const REDIRECT_URI = window.location.origin + window.location.pathname;

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

  metaPath.textContent = "/Apps/…" + FILE_PATH;

  // ── State ──────────────────────────────────────────────────────────
  const state = {
    data: null,         // TodoData = { "YYYY-MM-DD" | "Future": Task[] }
    rev: null,          // dropbox file revision (for conflict detection)
    accessToken: null,
    refreshToken: null,
    tokenExp: 0,
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
    set(k, v) { localStorage.setItem(k, JSON.stringify(v)); },
    del(k) { localStorage.removeItem(k); },
  };

  // ── PKCE + OAuth ──────────────────────────────────────────────────
  // https://www.dropbox.com/developers/reference/oauth-guide
  function b64url(bytes) {
    let s = "";
    for (const b of bytes) s += String.fromCharCode(b);
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }
  async function pkceChallenge() {
    const verifier = b64url(crypto.getRandomValues(new Uint8Array(64)));
    const hash = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
    return { verifier, challenge: b64url(new Uint8Array(hash)) };
  }

  async function startOAuth() {
    if (!APP_KEY) {
      toast("no dropbox app key set — edit web/config.js", "err");
      return;
    }
    const { verifier, challenge } = await pkceChallenge();
    LS.set("doitdoit:pkce_verifier", verifier);
    const url = new URL("https://www.dropbox.com/oauth2/authorize");
    url.searchParams.set("client_id", APP_KEY);
    url.searchParams.set("response_type", "code");
    url.searchParams.set("code_challenge", challenge);
    url.searchParams.set("code_challenge_method", "S256");
    url.searchParams.set("redirect_uri", REDIRECT_URI);
    url.searchParams.set("token_access_type", "offline");
    window.location.assign(url.toString());
  }

  async function exchangeCode(code) {
    const verifier = LS.get("doitdoit:pkce_verifier");
    if (!verifier) throw new Error("missing PKCE verifier (did you reload mid-flow?)");
    const body = new URLSearchParams({
      code,
      grant_type: "authorization_code",
      client_id: APP_KEY,
      code_verifier: verifier,
      redirect_uri: REDIRECT_URI,
    });
    const r = await fetch("https://api.dropboxapi.com/oauth2/token", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
    });
    if (!r.ok) throw new Error("token exchange failed (" + r.status + ")");
    LS.del("doitdoit:pkce_verifier");
    saveTokens(await r.json());
  }

  async function refreshAccessToken() {
    const session = sessionGeneration;
    if (!state.refreshToken) throw new Error("no refresh token; please reconnect");
    const body = new URLSearchParams({
      grant_type: "refresh_token",
      refresh_token: state.refreshToken,
      client_id: APP_KEY,
    });
    const r = await fetch("https://api.dropboxapi.com/oauth2/token", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
    });
    if (!r.ok) {
      throw new Error("refresh failed; disconnect and reconnect required (local edits retained)");
    }
    const tokens = await r.json();
    if (session !== sessionGeneration) throw new Error("session disconnected");
    saveTokens(tokens);
  }

  function saveTokens(tok) {
    state.accessToken = tok.access_token;
    if (tok.refresh_token) state.refreshToken = tok.refresh_token;
    state.tokenExp = Date.now() + (tok.expires_in || 14400) * 1000 - 60_000;
    LS.set("doitdoit:tokens", {
      access_token: state.accessToken,
      refresh_token: state.refreshToken,
      exp: state.tokenExp,
    });
  }

  function loadTokens() {
    const tok = LS.get("doitdoit:tokens");
    if (!tok || !tok.access_token) return false;
    state.accessToken = tok.access_token;
    state.refreshToken = tok.refresh_token || null;
    state.tokenExp = tok.exp || 0;
    return true;
  }

  async function logout() {
    // Finish provisional drag state before capturing unsaved work.
    finishPointerDrag(true);
    finishKeyboardDrag(true);
    try {
      if (state.dirty) preserveRecovery();
      LS.del("doitdoit:tokens");
    } catch (err) {
      toast("disconnect blocked: " + err.message, "err");
      setSync("error");
      return;
    }
    const token = state.accessToken;
    sessionGeneration++;
    reloadGeneration++;
    if (saveTimer) clearTimeout(saveTimer);
    state.accessToken = null;
    state.refreshToken = null;
    state.data = null;
    state.rev = null;
    state.dirty = false;
    state.conflict = false;
    showConnect();
    try {
      if (token) {
        await fetch("https://api.dropboxapi.com/2/auth/token/revoke", {
          method: "POST",
          headers: { Authorization: "Bearer " + token },
        });
      }
    } catch (err) {
      console.warn("Dropbox token revocation failed; cleared this device", err);
    }
  }

  async function ensureToken() {
    if (!state.accessToken) throw new Error("not authenticated");
    if (Date.now() > state.tokenExp - 5000 && state.refreshToken) {
      await refreshAccessToken();
    }
  }

  // ── Task storage boundary ─────────────────────────────────────────
  const Sync = window.DoitdoitSync;
  const taskStore = Sync.createJSONStore({
    path: FILE_PATH, fetchImpl: fetch, ensureToken, refreshAccessToken,
    getToken: () => state.accessToken,
  });

  // ── Shared domain logic (also exercised by web/domain.test.js) ─────
  const Domain = window.DoitdoitDomain;
  const { todayStr, parseDay, addDays, rollOverIncompleteTasks,
    distributeFutureTasks } = Domain;
  const storageTarget = (target) => Domain.storageTarget(target, VISIBLE_DAYS);
  const targetForTask = (dayKey, task) => Domain.targetForTask(dayKey, task);
  const pruneOldTasks = (data) => Domain.pruneOldTasks(data, RETENTION_DAYS);
  const parseAddInput = (raw, target) => Domain.parseAddInput(raw, target, VISIBLE_DAYS);

  // ── View model + render ───────────────────────────────────────────
  const weekdayFormat = new Intl.DateTimeFormat(undefined, { weekday: "long" });
  const dayDateFormat = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });

  function buildView(data) {
    const today = new Date();
    const todayKey = todayStr(today);
    const days = [];

    for (let i = 0; i < VISIBLE_DAYS; i++) {
      const d = addDays(today, i);
      const key = todayStr(d);
      const tasks = (data[key] || []).map(toTaskView.bind(null, key));
      const label = i === 0 ? "Today" : i === 1 ? "Tomorrow" : weekdayFormat.format(d);
      days.push({
        key,
        label,
        date: dayDateFormat.format(d),
        tasks,
        hasTasks: tasks.length > 0,
        count: tasks.length || "",
        cls: i === 0 ? "today" : "future",
      });
    }

    const futureTasks = (data["Future"] || []).map(toTaskView.bind(null, "Future"));
    days.push({
      key: "Future",
      label: "Future",
      date: "Someday & scheduled",
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
        row.className = "task" + (task.completed ? " task--done" : "");
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
        row.append(toggle, title, drag);
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
    // re-distribute on each render so future-dated tasks flow into visible days
    distributeFutureTasks(state.data, VISIBLE_DAYS);
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
  function genId() {
    return Date.now() + "-" + Math.floor(Math.random() * 1e7);
  }

  function addTask(rawInput, selectedTarget) {
    const parsed = parseAddInput(rawInput, selectedTarget);
    if (parsed.error) { toast(parsed.error, "err"); return false; }
    const t = {
      id: genId(),
      title: parsed.title,
      completed: false,
      created_at: new Date().toISOString(),
    };
    if (parsed.due) t.due_date = parsed.due;
    Domain.insertTask(state.data, parsed.key, t);
    render({ preserveScroll: true });
    queueSave();
    return true;
  }

  function findTask(dayKey, id) { return Domain.findTask(state.data, dayKey, id); }

  function toggleTask(dayKey, id) {
    if (!Domain.toggleTask(state.data, dayKey, id)) return;
    render({ preserveScroll: true });
    queueSave();
  }

  function deleteTask(dayKey, id) {
    if (!Domain.deleteTask(state.data, dayKey, id)) return;
    const row = Array.from(board.querySelectorAll(".task")).find(
      (el) => el.dataset.key === dayKey && el.dataset.id === String(id)
    );
    row?.remove();
    updateDayChrome(dayKey);
    queueSave();
  }

  function moveTask(dayKey, id, destinationKey, destinationIndex) {
    return Domain.moveTask(state.data, dayKey, id, destinationKey, destinationIndex);
  }

  // ── Save (debounced + conflict-aware) ─────────────────────────────
  let saveTimer = null;
  let mutationGeneration = 0;
  let reloadGeneration = 0;
  let sessionGeneration = 0;

  function preserveRecovery() {
    const bytes = JSON.stringify(taskStore.recovery(state.data));
    localStorage.setItem("doitdoit:recovery", bytes);
    if (localStorage.getItem("doitdoit:recovery") !== bytes) {
      throw new Error("recovery copy could not be verified");
    }
  }

  function scheduleSave(delay = 600) {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(doSave, delay);
  }

  function queueSave() {
    mutationGeneration++;
    state.dirty = true;
    setSync(state.conflict ? "error" : "dirty");
    scheduleSave();
  }

  async function doSave() {
    saveTimer = null;
    if (!state.dirty || !state.data || !state.accessToken || state.conflict) return;
    if (state.interactionActive || state.saving) { scheduleSave(400); return; }
    const generation = mutationGeneration;
    const session = sessionGeneration;
    const snapshot = JSON.parse(JSON.stringify(state.data));
    reloadGeneration++; // Any earlier download has an obsolete revision context.
    state.saving = true;
    setSync("syncing");
    try {
      const newRev = await taskStore.save(snapshot, state.rev);
      if (session !== sessionGeneration) return;
      state.rev = newRev;
      state.dirty = generation !== mutationGeneration;
      setSync(state.dirty ? "dirty" : "idle");
      if (state.dirty) scheduleSave();
    } catch (err) {
      if (session !== sessionGeneration) return;
      state.dirty = true;
      if (saveTimer) clearTimeout(saveTimer);
      saveTimer = null;
      if (err.conflict) {
        state.conflict = true;
        try {
          preserveRecovery();
          toast("remote changed — recovery copy kept; use menu to recover or reload", "err");
        } catch (recoveryError) {
          toast("local edits retained; recovery failed: " + recoveryError.message, "err");
        }
      } else {
        console.error(err);
        toast("save failed: " + err.message, "err");
      }
      setSync("error");
    } finally {
      state.saving = false;
    }
  }

  async function reload(opts = {}) {
    if (state.interactionActive || !state.accessToken) return;
    if (state.saving) {
      if (!opts.silent) toast("save in progress — reload after it finishes", "err");
      return;
    }
    if (state.dirty && !opts.force) return;
    const request = ++reloadGeneration;
    const generation = mutationGeneration;
    const session = sessionGeneration;
    const before = JSON.stringify(state.data);
    const stale = () => request !== reloadGeneration || generation !== mutationGeneration ||
      session !== sessionGeneration || state.saving || state.interactionActive ||
      before !== JSON.stringify(state.data);
    setSync("syncing");
    try {
      const { data, rev } = await taskStore.load();
      if (stale()) {
        if (request === reloadGeneration && session === sessionGeneration && !state.saving) {
          setSync(state.conflict ? "error" : state.dirty ? "dirty" : "idle");
        }
        return;
      }
      // Check recovery immediately before replacement, with no intervening await.
      if (state.dirty) preserveRecovery();
      if (saveTimer) clearTimeout(saveTimer);
      saveTimer = null;
      state.data = data;
      state.rev = rev;
      state.dirty = false;
      state.conflict = false;
      const r1 = rollOverIncompleteTasks(state.data);
      const r2 = pruneOldTasks(state.data);
      distributeFutureTasks(state.data, VISIBLE_DAYS);
      if (before !== JSON.stringify(state.data) || !state.rendered) {
        render({ animate: !state.rendered, preserveScroll: state.rendered });
      }
      if (r1 || r2) {
        queueSave();
        // Maintenance uses the same snapshot acknowledgement and serialization.
        if (saveTimer) clearTimeout(saveTimer);
        await doSave();
      } else {
        setSync("idle");
      }
      if (session === sessionGeneration && opts.confirm) toast("reloaded", "ok");
    } catch (err) {
      if (stale()) return;
      console.error(err);
      toast("load failed: " + err.message, "err");
      setSync("error");
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
    if (!state.editing) return;
    const title = editTitle.value.trim();
    if (!title) { toast("task title cannot be empty", "err"); editTitle.focus(); return; }
    const destination = storageTarget(editTarget);
    if (destination.error) { toast(destination.error, "err"); return; }
    const { dayKey, id } = state.editing;
    if (!Domain.editTask(state.data, dayKey, id, title, destination)) { closeEditor(); return; }
    closeEditor({ dayKey: destination.key, id });
    render({ preserveScroll: true });
    queueSave();
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
      state.data = JSON.parse(drag.snapshot);
      render({ preserveScroll: true });
      announceDrag("Move cancelled");
    } else {
      queueSave();
      announceDrag("Task position saved");
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
    menuBtn.hidden = !LS.get("doitdoit:recovery");
    emptyState.hidden = true;
  }

  function downloadRecovery() {
    const recovery = LS.get("doitdoit:recovery");
    if (!recovery?.data) {
      toast("no unsaved recovery copy", "err");
      return;
    }
    const blob = new Blob([JSON.stringify(recovery.data, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "doitdoit-recovery.json";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
    toast("recovery copy downloaded", "ok");
  }

  // delegated click handler for tasks
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

  addForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const v = addInput.value;
    if (!v.trim()) return;
    if (addTask(v, addTarget)) addInput.value = "";
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
  menuDialog.addEventListener("click", (e) => {
    const item = e.target.closest("[data-act]");
    if (!item) return;
    const act = item.dataset.act;
    if (act === "close") menuDialog.close();
    else if (act === "reload") {
      menuDialog.close();
      if (!state.dirty || confirm("discard unsaved local changes and load Dropbox? a recovery copy will remain available.")) {
        reload({ confirm: true, force: true });
      }
    }
    else if (act === "recovery") { menuDialog.close(); downloadRecovery(); }
    else if (act === "copy-path") {
      navigator.clipboard?.writeText("/Apps/<your-app>" + FILE_PATH).then(
        () => toast("path copied", "ok"),
        () => toast("copy failed", "err")
      );
      menuDialog.close();
    }
    else if (act === "logout") {
      if (confirm("disconnect dropbox? unsaved edits will be kept in a local recovery copy.")) {
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
  }, 60_000);

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

    if (oauthErr) {
      window.history.replaceState({}, "", REDIRECT_URI);
      toast("oauth: " + oauthErr, "err");
      showConnect();
      return;
    }
    if (code) {
      window.history.replaceState({}, "", REDIRECT_URI);
      try {
        await exchangeCode(code);
      } catch (err) {
        console.error(err);
        toast("oauth failed: " + err.message, "err");
        showConnect();
        return;
      }
    }

    if (!loadTokens()) { showConnect(); return; }
    showBoard();
    await reload({ silent: true });
  }

  // expose minimal debug surface
  window.doitdoit = { reload, logout, state, storageTarget, parseAddInput };

  addDate.min = todayStr();
  editDate.min = todayStr();
  paintSchedule(addSchedule, "data-add-schedule", addTarget, addDate, addDateLabel);
  if ("ResizeObserver" in window) new ResizeObserver(syncPromptHeight).observe(promptBar);
  window.addEventListener("resize", syncPromptHeight);
  boot();
})();
