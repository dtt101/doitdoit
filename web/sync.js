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

  async function downloadOnce(fetchImpl, token, path) {
    const response = await fetchImpl("https://content.dropboxapi.com/2/files/download", {
      method: "POST",
      headers: { Authorization: "Bearer " + token, "Dropbox-API-Arg": asciiJson({ path }) },
    });
    if (response.status === 401) return { unauthorized: true };
    if (response.status === 409) return { data: {}, rev: null };
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      throw new Error("download " + response.status + " " + text.slice(0, 120));
    }
    const meta = JSON.parse(response.headers.get("Dropbox-API-Result") || "{}");
    const text = await response.text();
    let data = {};
    if (text.trim()) {
      try { data = JSON.parse(text); }
      catch { throw new Error("dropbox file is not valid JSON"); }
    }
    return { data, rev: meta.rev || null };
  }

  async function uploadOnce(fetchImpl, token, path, data, rev) {
    const args = rev
      ? { path, mode: { ".tag": "update", update: rev }, mute: true, autorename: false }
      : { path, mode: "overwrite", mute: true, autorename: false };
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
    return { rev: meta.rev };
  }

  function recoverySnapshot(data, filePath, now = new Date()) {
    return { savedAt: now.toISOString(), filePath, data: JSON.parse(JSON.stringify(data)) };
  }

  return { asciiJson, downloadOnce, uploadOnce, recoverySnapshot };
});
