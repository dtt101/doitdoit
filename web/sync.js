(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.DoitdoitSync = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function asciiJson(value) {
    return JSON.stringify(value).replace(/[\u0080-\uffff]/g, (char) =>
      "\\u" + ("0000" + char.charCodeAt(0).toString(16)).slice(-4));
  }

  function revision(meta) {
    if (!meta || typeof meta.rev !== "string" || !/^[0-9a-f]{9,}$/.test(meta.rev)) {
      throw new Error("missing or invalid Dropbox revision metadata");
    }
    return meta.rev;
  }

  async function downloadOnce(fetchImpl, token, path) {
    const response = await fetchImpl("https://content.dropboxapi.com/2/files/download", {
      method: "POST",
      headers: { Authorization: "Bearer " + token, "Dropbox-API-Arg": asciiJson({ path }) },
    });
    if (response.status === 401) return { unauthorized: true };
    if (response.status === 409) {
      const body = await response.json().catch(() => null);
      if (body?.error?.[".tag"] === "path" && body.error.path?.[".tag"] === "not_found") {
        return { data: {}, rev: null };
      }
      throw new Error("download 409: Dropbox path could not be read");
    }
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      throw new Error("download " + response.status + " " + text.slice(0, 120));
    }
    const meta = JSON.parse(response.headers.get("Dropbox-API-Result") || "{}");
    const rev = revision(meta);
    const text = await response.text();
    let data = {};
    if (text.trim()) {
      try { data = JSON.parse(text); }
      catch { throw new Error("dropbox file is not valid JSON"); }
    }
    return { data, rev };
  }

  async function uploadOnce(fetchImpl, token, path, data, rev) {
    if (rev !== null) revision({ rev });
    const args = rev !== null
      ? { path, mode: { ".tag": "update", update: rev }, mute: true, autorename: false, strict_conflict: true }
      : { path, mode: "add", mute: true, autorename: false, strict_conflict: true };
    const response = await fetchImpl("https://content.dropboxapi.com/2/files/upload", {
      method: "POST",
      headers: {
        Authorization: "Bearer " + token,
        "Dropbox-API-Arg": asciiJson(args),
        "Content-Type": "application/octet-stream",
      },
      body: JSON.stringify(data, null, 2),
    });
    if (response.status === 401) return { unauthorized: true };
    if (response.status === 409) {
      const body = await response.json().catch(() => null);
      throw Object.assign(new Error("conflict"), { conflict: true, body });
    }
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      throw new Error("upload " + response.status + " " + text.slice(0, 120));
    }
    const meta = await response.json();
    return { rev: revision(meta) };
  }

  function recoverySnapshot(data, filePath, now = new Date()) {
    return { savedAt: now.toISOString(), filePath, data: JSON.parse(JSON.stringify(data)) };
  }

  // The app owns authentication state; the store owns task-file transport.
  function createJSONStore({ path, fetchImpl, ensureToken, refreshAccessToken, getToken }) {
    // Null is a create-only expectation, allowed only after confirmed absence.
    let missing = false;
    let requestGeneration = 0;
    async function authenticated(request) {
      await ensureToken();
      let result = await request();
      if (result.unauthorized) {
        await refreshAccessToken();
        result = await request();
        if (result.unauthorized) throw new Error("authentication failed; reconnect required");
      }
      return result;
    }
    async function load() {
      const request = ++requestGeneration;
      const result = await authenticated(() => downloadOnce(fetchImpl, getToken(), path));
      if (request === requestGeneration) missing = result.rev === null;
      return result;
    }
    async function save(data, rev) {
      if (rev === null && !missing) throw new Error("load the Dropbox file before creating it");
      requestGeneration++; // Older downloads cannot change the creation expectation.
      // Detach before authentication yields, including any refresh/retry.
      const snapshot = JSON.parse(JSON.stringify(data));
      const result = await authenticated(() => uploadOnce(fetchImpl, getToken(), path, snapshot, rev));
      missing = false;
      return result.rev;
    }
    return { load, save, recovery: (data, now) => recoverySnapshot(data, path, now) };
  }

  return { asciiJson, downloadOnce, uploadOnce, recoverySnapshot, createJSONStore };
});
