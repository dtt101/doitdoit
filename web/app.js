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
  const SPIN = ["[|]", "[/]", "[-]", "[\\]"];
  let spinIdx = 0;
  let spinTimer = null;
  function setSync(stateName, label) {
    syncEl.dataset.state = stateName;
    if (stateName === "syncing") {
      if (!spinTimer) {
        spinTimer = setInterval(() => {
          spinIdx = (spinIdx + 1) % SPIN.length;
          syncEl.textContent = SPIN[spinIdx];
        }, 100);
      }
      return;
    }
    if (spinTimer) { clearInterval(spinTimer); spinTimer = null; }
    syncEl.textContent = label || (
      stateName === "idle" ? "[ok]" :
      stateName === "dirty" ? "[~~]" :
      stateName === "error" ? "[!!]" : "[??]"
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
      await logout();
      throw new Error("refresh failed; reconnect required");
    }
    saveTokens(await r.json());
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
    const token = state.accessToken;
    try {
      if (token) {
        await fetch("https://api.dropboxapi.com/2/auth/token/revoke", {
          method: "POST",
          headers: { Authorization: "Bearer " + token },
        });
      }
    } catch (err) {
      console.warn("Dropbox token revocation failed; clearing this device", err);
    } finally {
      LS.del("doitdoit:tokens");
      state.accessToken = null;
      state.refreshToken = null;
      state.data = null;
      state.rev = null;
      state.dirty = false;
      state.conflict = false;
      showConnect();
    }
  }

  async function ensureToken() {
    if (!state.accessToken) throw new Error("not authenticated");
    if (Date.now() > state.tokenExp - 5000 && state.refreshToken) {
      await refreshAccessToken();
    }
  }

  // ── Dropbox file ops ──────────────────────────────────────────────
  const Sync = window.DoitdoitSync;

  async function dbxDownload() {
    await ensureToken();
    const result = await Sync.downloadOnce(fetch, state.accessToken, FILE_PATH);
    if (result.unauthorized) {
      await refreshAccessToken();
      return dbxDownload();
    }
    return result;
  }

  async function dbxUpload(data, rev) {
    await ensureToken();
    const result = await Sync.uploadOnce(fetch, state.accessToken, FILE_PATH, data, rev);
    if (result.unauthorized) {
      await refreshAccessToken();
      return dbxUpload(data, rev);
    }
    return result.rev;
  }

  // ── Shared domain logic (also exercised by web/domain.test.js) ─────
  const Domain = window.DoitdoitDomain;
  const { todayStr, parseDay, addDays, rollOverIncompleteTasks,
    distributeFutureTasks, insertBeforeCompleted } = Domain;
  const storageTarget = (target) => Domain.storageTarget(target, VISIBLE_DAYS);
  const targetForTask = (dayKey, task) => Domain.targetForTask(dayKey, task);
  const pruneOldTasks = (data) => Domain.pruneOldTasks(data, RETENTION_DAYS);
  const parseAddInput = (raw, target) => Domain.parseAddInput(raw, target, VISIBLE_DAYS);

  // ── View model + render ───────────────────────────────────────────
  const DOW = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"];

  function buildView(data) {
    const today = new Date();
    const todayKey = todayStr(today);
    const days = [];

    for (let i = 0; i < VISIBLE_DAYS; i++) {
      const d = addDays(today, i);
      const key = todayStr(d);
      const tasks = (data[key] || []).map(toTaskView.bind(null, key));
      const label = i === 0
        ? `${DOW[d.getDay()]} ${key} · today`
        : i === 1
          ? `${DOW[d.getDay()]} ${key} · tomorrow`
          : `${DOW[d.getDay()]} ${key}`;
      days.push({
        key,
        label,
        tasks,
        hasTasks: tasks.length > 0,
        count: tasks.length || "",
        cls: i === 0 ? "today" : "future",
      });
    }

    const futureTasks = (data["Future"] || []).map(toTaskView.bind(null, "Future"));
    days.push({
      key: "Future",
      label: "future",
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
      mark: t.completed ? "x" : " ",
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
      const rule = document.createElement("span");
      rule.className = "day__rule";
      const label = document.createElement("span");
      label.className = "day__label";
      label.textContent = day.label;
      const count = document.createElement("span");
      count.className = "day__count";
      count.textContent = day.count;
      const fillRule = document.createElement("span");
      fillRule.className = "day__rule day__rule--fill";
      heading.append(rule, label, count, fillRule);
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
        const leftBracket = document.createElement("span");
        leftBracket.className = "bracket";
        leftBracket.textContent = "[";
        const mark = document.createElement("span");
        mark.className = "task__mark";
        mark.textContent = task.mark;
        const rightBracket = document.createElement("span");
        rightBracket.className = "bracket";
        rightBracket.textContent = "]";
        toggle.append(leftBracket, mark, rightBracket);
        const title = document.createElement("button");
        title.className = "task__title";
        title.dataset.action = "edit";
        title.setAttribute("aria-label", `edit ${task.title}`);
        title.textContent = task.title;
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
      empty.textContent = "— nothing here —";
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
    if (parsed.error) { toast(parsed.error, "err"); return; }
    const t = {
      id: genId(),
      title: parsed.title,
      completed: false,
      created_at: new Date().toISOString(),
    };
    if (parsed.due) t.due_date = parsed.due;
    if (!state.data[parsed.key]) state.data[parsed.key] = [];
    insertBeforeCompleted(state.data[parsed.key], t);
    render({ preserveScroll: true });
    queueSave();
  }

  function findTask(dayKey, id) {
    const list = state.data[dayKey];
    if (!list) return null;
    const idx = list.findIndex((x) => String(x.id) === String(id));
    return idx >= 0 ? { list, idx, task: list[idx] } : null;
  }

  function toggleTask(dayKey, id) {
    const f = findTask(dayKey, id);
    if (!f) return;
    f.task.completed = !f.task.completed;
    // Reorder to match the CLI: completed tasks sink to the bottom of the
    // day, uncompleted tasks move back above the completed block.
    f.list.splice(f.idx, 1);
    if (f.task.completed) f.list.push(f.task);
    else insertBeforeCompleted(f.list, f.task);
    render({ preserveScroll: true });
    queueSave();
  }

  function deleteTask(dayKey, id) {
    const f = findTask(dayKey, id);
    if (!f) return;
    f.list.splice(f.idx, 1);
    if (f.list.length === 0 && dayKey !== "Future") delete state.data[dayKey];
    const row = Array.from(board.querySelectorAll(".task")).find(
      (el) => el.dataset.key === dayKey && el.dataset.id === String(id)
    );
    row?.remove();
    updateDayChrome(dayKey);
    queueSave();
  }

  function moveTask(dayKey, id, destinationKey, destinationIndex) {
    const found = findTask(dayKey, id);
    if (!found) return false;
    const task = found.task;
    found.list.splice(found.idx, 1);
    if (found.list.length === 0 && dayKey !== "Future") delete state.data[dayKey];

    if (destinationKey === "Future") delete task.due_date;
    else task.due_date = destinationKey;
    const targetList = state.data[destinationKey] || (state.data[destinationKey] = []);
    const index = Math.max(0, Math.min(destinationIndex, targetList.length));
    targetList.splice(index, 0, task);
    Domain.groupTasksByCompletion(state.data);
    return true;
  }

  // ── Save (debounced + conflict-aware) ─────────────────────────────
  let saveTimer = null;
  function queueSave() {
    state.dirty = true;
    state.conflict = false;
    setSync("dirty");
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(doSave, 600);
  }

  async function doSave() {
    if (state.interactionActive) { saveTimer = setTimeout(doSave, 400); return; }
    if (state.saving) { saveTimer = setTimeout(doSave, 400); return; }
    state.saving = true;
    setSync("syncing");
    try {
      const newRev = await dbxUpload(state.data, state.rev);
      state.rev = newRev;
      state.dirty = false;
      setSync("idle");
    } catch (err) {
      if (err.conflict) {
        state.conflict = true;
        LS.set("doitdoit:recovery", Sync.recoverySnapshot(state.data, FILE_PATH));
        toast("remote changed — local edits kept; use menu to recover or reload", "err");
        setSync("error");
      } else {
        console.error(err);
        toast("save failed: " + err.message, "err");
        setSync("error");
      }
    } finally {
      state.saving = false;
    }
  }

  async function reload(opts = {}) {
    if (state.interactionActive) return;
    setSync("syncing");
    try {
      const { data, rev } = await dbxDownload();
      const before = state.data ? JSON.stringify(state.data) : null;
      state.data = data;
      state.rev = rev;
      state.dirty = false;
      state.conflict = false;
      const r1 = rollOverIncompleteTasks(state.data);
      const r2 = pruneOldTasks(state.data);
      if (r1 || r2) {
        // persist rollover/prune so the CLI sees a consistent file too
        state.rev = await dbxUpload(state.data, state.rev);
      }
      // Normalize the in-memory view before comparing so an unchanged focus
      // reload does not rebuild the board just because dated Future tasks moved.
      distributeFutureTasks(state.data, VISIBLE_DAYS);
      if (before !== JSON.stringify(state.data) || !state.rendered) {
        render({ animate: !state.rendered, preserveScroll: state.rendered });
      }
      setSync("idle");
      if (!opts.silent) {
        // subtle confirm only on explicit reloads
        if (opts.confirm) toast("reloaded", "ok");
      }
    } catch (err) {
      console.error(err);
      if (err.conflict) {
        state.dirty = true;
        state.conflict = true;
        LS.set("doitdoit:recovery", Sync.recoverySnapshot(state.data, FILE_PATH));
        toast("remote changed during maintenance — recovery copy kept", "err");
      } else {
        toast("load failed: " + err.message, "err");
      }
      setSync("error");
    }
  }

  // ── UI wiring ─────────────────────────────────────────────────────
  function shortDateLabel(value) {
    const date = parseDay(value);
    if (!date) return "date…";
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
    dateLabel.textContent = target.kind === "custom" && target.date ? shortDateLabel(target.date) : "date…";
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
    const found = findTask(dayKey, id);
    if (!found) { closeEditor(); return; }
    const task = found.task;
    task.title = title;
    if (destination.due) task.due_date = destination.due;
    else delete task.due_date;

    if (destination.key !== dayKey) {
      found.list.splice(found.idx, 1);
      if (found.list.length === 0 && dayKey !== "Future") delete state.data[dayKey];
      const targetList = state.data[destination.key] || (state.data[destination.key] = []);
      insertBeforeCompleted(targetList, task);
    }
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
    connectEl.hidden = false;
    board.hidden = true;
    promptBar.hidden = true;
    menuBtn.hidden = true;
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
    addTask(v, addTarget);
    addInput.value = "";
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
        reload({ confirm: true });
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
      if (confirm("disconnect dropbox? your tasks stay safe in dropbox.")) {
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
