# doitdoit · web companion

A small static web app that reads and writes the same Dropbox JSON file the
CLI uses. Designed for adding and ticking off tasks from a phone, but works
equally well on desktop. No backend, no build step, no framework — just
HTML + CSS + a single vanilla JS file + [mustache.js] for templates.

## Architecture

- **Auth**: Dropbox OAuth 2.0 with [PKCE]. The app key is public and committed
  to `config.js`; no client secret is required, no server is involved. Tokens
  live in `localStorage` only.
- **Data**: a single JSON file at the path you configure (default
  `/Apps/doitdoit/doitdoit.json`). Reads via `/2/files/download`, writes via
  `/2/files/upload` with `mode: { update: <rev> }` so concurrent CLI writes
  surface as a 409 and the page reloads instead of clobbering.
- **Domain logic**: `rollOverIncompleteTasks` and `pruneOldTasks` are ported
  from `model/task.go`. Keep them in sync if the CLI's rules change.

### Why no htmx?

The earlier plan reached for htmx. In practice:

- htmx's strength is HTML-over-the-wire from a server.
- We have no server. Dropbox returns JSON, not HTML.
- Mutations need PKCE auth, optimistic UI, conflict handling — all JS-driven.

So htmx would be a wrapper over a JS app, not the app itself. mustache.js +
vanilla event delegation does the same job in fewer moving parts. Same
spirit (no React, no build, server-rendered feel) without forcing a fit.

## One-time setup

### 1. Register a Dropbox app

Go to https://www.dropbox.com/developers/apps → **Create app**:

| Field          | Value                                          |
| -------------- | ---------------------------------------------- |
| API            | Scoped access                                  |
| Access type    | **App folder** (recommended) or Full Dropbox   |
| App name       | anything unique, e.g. `doitdoit-yourname`      |

On the app's settings page:

- **Permissions** tab → enable `files.content.read` and `files.content.write`,
  then **Submit**.
- **Settings** tab → **OAuth 2 / Redirect URIs** → add the URL where this page
  will be served (e.g. `https://you.github.io/doitdoit/`). Add
  `http://localhost:8000/` too if you want to test locally.
- Copy the **App key** (a public client ID, not the secret) and paste it into
  [`config.js`](./config.js) as `dropboxAppKey`.

### 2. Move (or create) the data file

If you chose **App folder** access, your file must live under
`/Apps/<your-app-name>/`. Either:

- Move your existing `doitdoit.json` there, then update the CLI's
  `~/.doitdoit_config.json` `storage_path` to match, **or**
- Pick a new file name in `config.js` (`dropboxFilePath`) and let the web app
  create it on first save.

If you chose **Full Dropbox** access, the file path can be anywhere; just set
`dropboxFilePath` accordingly.

### 3. Deploy on GitHub Pages

In the repo: **Settings → Pages → Source: Deploy from a branch → Branch
`main`, Folder `/web` → Save.** Wait ~30s; the URL appears at the top of the
Pages settings page.

That's it. Push to `main` to redeploy.

## Local development

The app is fully static. Any local file server works:

```bash
cd web
python3 -m http.server 8000
# or
npx http-server -p 8000
```

Then open http://localhost:8000/. Add `http://localhost:8000/` to your
Dropbox app's redirect URIs to test the OAuth round-trip locally.

## Add-task syntax

The bottom prompt accepts plain text plus an optional `!target` prefix:

| Input                          | Result                                |
| ------------------------------ | ------------------------------------- |
| `buy bread`                    | adds to today                         |
| `!future write a postcard`     | adds to the Future bucket             |
| `!2026-06-01 dentist`          | adds to that specific date            |

## Mobile install

Open the deployed URL in Android Chrome, then **menu → Add to Home screen**.
It launches fullscreen with the dark theme color in the status bar. iOS Safari
works similarly via the share sheet.

## Files

```
web/
├── index.html      # shell, mustache template, font imports
├── style.css       # the entire visual identity (CRT amber on warm black)
├── app.js          # OAuth, Dropbox API, rollover/prune, mutations, render
├── config.js       # public Dropbox app key + file path
└── .nojekyll       # tell GitHub Pages not to run Jekyll
```

## Troubleshooting

- **"no dropbox app key set"** — edit `config.js` and paste your app key.
- **OAuth redirect mismatch** — the URL in your address bar must match a
  redirect URI registered in your Dropbox app settings exactly (trailing
  slash and protocol matter).
- **"dropbox file is not valid JSON"** — usually a half-finished hand-edit
  of the file. Open it in Dropbox's web UI and fix the syntax.
- **Tasks added on web don't show in CLI (or vice versa)** — both sides only
  read from the file on startup/refresh. Restart the CLI; on web, the page
  reloads on focus and every 60 seconds.
- **Logout** — header's `···` menu → **disconnect dropbox**. Wipes the token
  from localStorage; your tasks stay safe in Dropbox.

[mustache.js]: https://github.com/janl/mustache.js
[PKCE]: https://datatracker.ietf.org/doc/html/rfc7636
