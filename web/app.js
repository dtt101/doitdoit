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
  const PRUNE_AFTER_DAYS = CFG.pruneAfterDays || 5;
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

  const tmplBoard = $("tmpl-board").innerHTML;
  Mustache.parse(tmplBoard);

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
  };

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
      logout();
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

  function logout() {
    LS.del("doitdoit:tokens");
    state.accessToken = null;
    state.refreshToken = null;
    state.data = null;
    state.rev = null;
    showConnect();
  }

  async function ensureToken() {
    if (!state.accessToken) throw new Error("not authenticated");
    if (Date.now() > state.tokenExp - 5000 && state.refreshToken) {
      await refreshAccessToken();
    }
  }

  // ── Dropbox file ops ──────────────────────────────────────────────
  // Dropbox-API-Arg must be ASCII-safe JSON.
  function asciiJson(obj) {
    return JSON.stringify(obj).replace(/[-￿]/g, (c) =>
      "\\u" + ("0000" + c.charCodeAt(0).toString(16)).slice(-4)
    );
  }

  async function dbxDownload() {
    await ensureToken();
    const r = await fetch("https://content.dropboxapi.com/2/files/download", {
      method: "POST",
      headers: {
        Authorization: "Bearer " + state.accessToken,
        "Dropbox-API-Arg": asciiJson({ path: FILE_PATH }),
      },
    });
    if (r.status === 409) {
      // path/not_found — file doesn't exist yet. Start empty.
      return { data: {}, rev: null };
    }
    if (r.status === 401) {
      await refreshAccessToken();
      return dbxDownload();
    }
    if (!r.ok) {
      const txt = await r.text().catch(() => "");
      throw new Error("download " + r.status + " " + txt.slice(0, 120));
    }
    const meta = JSON.parse(r.headers.get("Dropbox-API-Result") || "{}");
    const text = await r.text();
    let data = {};
    if (text.trim()) {
      try { data = JSON.parse(text); }
      catch { throw new Error("dropbox file is not valid JSON"); }
    }
    return { data, rev: meta.rev || null };
  }

  async function dbxUpload(data, rev) {
    await ensureToken();
    const body = JSON.stringify(data, null, 2);
    const args = rev
      ? { path: FILE_PATH, mode: { ".tag": "update", update: rev }, mute: true, autorename: false }
      : { path: FILE_PATH, mode: "overwrite", mute: true, autorename: false };
    const r = await fetch("https://content.dropboxapi.com/2/files/upload", {
      method: "POST",
      headers: {
        Authorization: "Bearer " + state.accessToken,
        "Dropbox-API-Arg": asciiJson(args),
        "Content-Type": "application/octet-stream",
      },
      body,
    });
    if (r.status === 401) {
      await refreshAccessToken();
      return dbxUpload(data, rev);
    }
    if (r.status === 409) {
      // conflict — likely a stale rev. Surface specifically.
      const err = await r.json().catch(() => null);
      throw Object.assign(new Error("conflict"), { conflict: true, body: err });
    }
    if (!r.ok) {
      const txt = await r.text().catch(() => "");
      throw new Error("upload " + r.status + " " + txt.slice(0, 120));
    }
    const meta = await r.json();
    return meta.rev;
  }

  // ── Domain logic — ported from model/task.go ──────────────────────
  function todayStr(d = new Date()) {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
  }
  function parseDay(s) {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s || "");
    if (!m) return null;
    return new Date(+m[1], +m[2] - 1, +m[3]);
  }
  function addDays(d, n) {
    const x = new Date(d);
    x.setDate(x.getDate() + n);
    return x;
  }
  function startOfDay(d) {
    return new Date(d.getFullYear(), d.getMonth(), d.getDate());
  }

  // model/task.go:132 — rollOverIncompleteTasks
  function rollOverIncompleteTasks(data) {
    const today = todayStr();
    const todayDate = startOfDay(new Date());
    const toRoll = [];
    const datesToRemove = [];
    let changed = false;

    for (const dateStr of Object.keys(data)) {
      if (dateStr === "Future") continue;
      const parsed = parseDay(dateStr);
      if (!parsed) continue;
      if (parsed < todayDate) {
        const remaining = [];
        for (const t of data[dateStr]) {
          if (!t.completed) {
            t.due_date = today;
            toRoll.push(t);
          } else {
            remaining.push(t);
          }
        }
        if (remaining.length) data[dateStr] = remaining;
        else datesToRemove.push(dateStr);
      }
    }

    if (toRoll.length) {
      data[today] = (data[today] || []).concat(toRoll);
      changed = true;
    }
    for (const d of datesToRemove) { delete data[d]; changed = true; }
    if (data[today] && data[today].length === 0) delete data[today];
    return changed;
  }

  // model/task.go:241 — pruneOldTasks
  function pruneOldTasks(data) {
    const cutoff = todayStr(addDays(new Date(), -PRUNE_AFTER_DAYS));
    let changed = false;
    for (const k of Object.keys(data)) {
      if (k === "Future") {
        const tasks = data[k] || [];
        const active = tasks.filter((t) => !t.completed);
        if (active.length !== tasks.length) { data[k] = active; changed = true; }
        continue;
      }
      if (k < cutoff) { delete data[k]; changed = true; }
    }
    return changed;
  }

  // model/task.go:272 — DistributeFutureTasks (in-memory only, for view)
  function distributeFutureTasks(data, visibleDays) {
    const future = data["Future"] || [];
    if (!future.length) return;
    const today = startOfDay(new Date());
    const lastVisible = addDays(today, visibleDays - 1);
    const todayKey = todayStr(today);
    const remain = [];
    for (const t of future) {
      const due = parseDay(t.due_date);
      if (!due) { remain.push(t); continue; }
      if (due <= lastVisible) {
        const target = due < today ? todayKey : t.due_date;
        if (!data[target]) data[target] = [];
        data[target].push(t);
      } else {
        remain.push(t);
      }
    }
    data["Future"] = remain;
  }

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

  function render() {
    if (!state.data) return;
    // re-distribute on each render so future-dated tasks flow into visible days
    distributeFutureTasks(state.data, VISIBLE_DAYS);
    const view = buildView(state.data);
    board.innerHTML = Mustache.render(tmplBoard, view);

    // Empty state if literally no tasks anywhere
    const totalTasks = view.days.reduce((n, d) => n + d.tasks.length, 0);
    emptyState.hidden = totalTasks > 0;
  }

  // ── Mutations ─────────────────────────────────────────────────────
  function genId() {
    return Date.now() + "-" + Math.floor(Math.random() * 1e7);
  }

  function parseAddInput(raw) {
    let title = raw.trim();
    let key = todayStr();
    let due = "";

    // !target prefix: !future or !YYYY-MM-DD
    const m = /^!(\S+)\s+(.+)$/.exec(title);
    if (m) {
      const target = m[1].toLowerCase();
      title = m[2].trim();
      if (target === "future") {
        key = "Future";
      } else if (/^\d{4}-\d{2}-\d{2}$/.test(target)) {
        key = target;
        due = target;
      } else {
        return { error: "unknown target — use !future or !YYYY-MM-DD" };
      }
    }
    if (!title) return { error: "task title cannot be empty" };
    return { title, key, due };
  }

  function addTask(rawInput) {
    const parsed = parseAddInput(rawInput);
    if (parsed.error) { toast(parsed.error, "err"); return; }
    const t = {
      id: genId(),
      title: parsed.title,
      completed: false,
      created_at: new Date().toISOString(),
    };
    if (parsed.due) t.due_date = parsed.due;
    if (!state.data[parsed.key]) state.data[parsed.key] = [];
    state.data[parsed.key].push(t);
    render();
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
    render();
    queueSave();
  }

  function deleteTask(dayKey, id) {
    const f = findTask(dayKey, id);
    if (!f) return;
    f.list.splice(f.idx, 1);
    if (f.list.length === 0 && dayKey !== "Future") delete state.data[dayKey];
    render();
    queueSave();
  }

  // ── Save (debounced + conflict-aware) ─────────────────────────────
  let saveTimer = null;
  function queueSave() {
    state.dirty = true;
    setSync("dirty");
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(doSave, 600);
  }

  async function doSave() {
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
        toast("remote changed — reloading", "err");
        await reload({ silent: true });
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
    setSync("syncing");
    try {
      const { data, rev } = await dbxDownload();
      state.data = data;
      state.rev = rev;
      const r1 = rollOverIncompleteTasks(state.data);
      const r2 = pruneOldTasks(state.data);
      if (r1 || r2) {
        // persist rollover/prune so the CLI sees a consistent file too
        state.rev = await dbxUpload(state.data, state.rev);
      }
      render();
      setSync("idle");
      if (!opts.silent) {
        // subtle confirm only on explicit reloads
        if (opts.confirm) toast("reloaded", "ok");
      }
    } catch (err) {
      console.error(err);
      toast("load failed: " + err.message, "err");
      setSync("error");
    }
  }

  // ── UI wiring ─────────────────────────────────────────────────────
  function showBoard() {
    connectEl.hidden = true;
    board.hidden = false;
    promptBar.hidden = false;
    menuBtn.hidden = false;
    requestAnimationFrame(() => addInput && addInput.focus({ preventScroll: true }));
  }
  function showConnect() {
    connectEl.hidden = false;
    board.hidden = true;
    promptBar.hidden = true;
    menuBtn.hidden = true;
    emptyState.hidden = true;
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
    else if (action === "delete") {
      li.classList.add("task--exit");
      // wait for exit animation, then mutate
      setTimeout(() => deleteTask(dayKey, id), 200);
    }
  });

  addForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const v = addInput.value;
    if (!v.trim()) return;
    addTask(v);
    addInput.value = "";
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
    else if (act === "reload") { menuDialog.close(); reload({ confirm: true }); }
    else if (act === "copy-path") {
      navigator.clipboard?.writeText("/Apps/<your-app>" + FILE_PATH).then(
        () => toast("path copied", "ok"),
        () => toast("copy failed", "err")
      );
      menuDialog.close();
    }
    else if (act === "logout") {
      if (confirm("disconnect dropbox? your tasks stay safe in dropbox.")) {
        logout();
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
    if (e.key === "/" && document.activeElement !== addInput) {
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
  window.doitdoit = { reload, logout, state };

  boot();
})();
