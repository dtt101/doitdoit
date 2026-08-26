# doitdoit · web companion (experimental)

> This Dropbox companion is experimental, is not included in the
> doitdoit CLI release, and is outside that release's security and
> privacy assurance. Review the residual risks in
> [`../docs/web-companion-follow-up.md`](../docs/web-companion-follow-up.md)
> before using it with sensitive or irreplaceable task data.

A small static web app that reads and writes the same Dropbox JSON file the
CLI uses. Designed for adding and ticking off tasks from a phone, but works
equally well on desktop. No backend, build step, framework, remotely executed
script, or remote font is required.

## Architecture

- **Auth**: Dropbox OAuth 2.0 with [PKCE]. The app key is public and committed
  to `config.js`; no client secret is required, no server is involved. Tokens
  live in `localStorage` only.
- **Data**: a single JSON file at the path you configure (`/config.json` in
  the app's Dropbox scope by default). Reads via `/2/files/download`, writes
  via `/2/files/upload` with `mode: { update: <rev> }` so concurrent changes
  surface as a recoverable conflict instead of being clobbered.
- **Domain logic**: rollover, distribution, ordering, and explicit retention
  behavior live in `domain.js` and are tested with the same lifecycle rules as
  the CLI. Retention defaults to forever; set `retentionDays` to a positive
  value only after making an equivalent explicit choice in the CLI.
- **Conflicts**: revision conflicts leave local edits intact and store a
  recovery snapshot in browser storage. Use the menu to download it before
  deliberately reloading the Dropbox version.
- **Browser security**: the page uses a restrictive Content Security Policy
  and local assets only. OAuth tokens must remain available to JavaScript
  because this is a serverless client, so a compromised browser profile or
  malicious extension could still access them.

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

1. **Configure Pages**: in the repository, open **Settings → Pages** and set
   **Source** to **GitHub Actions**.
2. **Push your changes**: the included workflow uploads `web/` as a Pages
   artifact and deploys it. The deployment URL appears in the workflow and
   Pages settings.

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

## Managing tasks

Use the controls under the bottom prompt to add a task to Today, Tomorrow,
Future, or a date from the device's date picker. Dates beyond the visible
five-day window stay in Future until they come into range.

- Tap a task title to edit its title or schedule.
- Drag the `≡` handle to reorder a task or move it between visible days and
  Future. With a keyboard, focus the handle, press Space or Enter to pick up,
  use the arrow keys to move, then press Space or Enter again to save.
- Tap `[ ]` to toggle completion. Delete is available inside the task editor.

The original optional `!target` prefixes remain available as shortcuts and
override the selected date control:

| Input                          | Result                                |
| ------------------------------ | ------------------------------------- |
| `buy bread`                    | adds to today                         |
| `!future write a postcard`     | adds to the Future bucket             |
| `!2027-06-01 dentist`          | schedules for that specific date      |

## Mobile install

Open the deployed URL in Android Chrome, then **menu → Add to Home screen**.
It launches fullscreen with the dark theme color in the status bar. iOS Safari
works similarly via the share sheet.

## Files

```
web/
├── icons/          # Android, favicon, and Apple home-screen icons
├── index.html      # local-only shell and Content Security Policy
├── manifest.webmanifest # install metadata and maskable icon declarations
├── style.css       # the entire visual identity (CRT amber on warm black)
├── app.js          # OAuth, mutations, accessible DOM rendering
├── domain.js       # shared/testable task lifecycle rules
├── sync.js         # shared/testable Dropbox revision operations
├── *.test.js       # Node built-in unit tests
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
- **Tasks added on web don't show in CLI (or vice versa)** — the CLI polls
  every few seconds; the web page reloads on focus and every 60 seconds.
- **Conflict warning** — download the recovery copy from the menu, then force
  reload and reapply the desired change. The app never silently discards the
  recovery snapshot.
- **Logout** — header's `···` menu → **disconnect dropbox**. The app asks
  Dropbox to revoke the token, then wipes it from localStorage; tasks remain
  in Dropbox.

## Privacy and recovery

Task content is sent only to Dropbox's API and remains in the selected Dropbox
file. OAuth credentials and conflict-recovery snapshots are stored in this
browser's localStorage. The app contains no analytics or advertising. GitHub
Pages and Dropbox may retain ordinary request metadata under their own terms.
Disconnect when using a shared device, and remove this site's browser data if
you also want to erase local recovery snapshots.

Back up the Dropbox file before migrations. The CLI additionally maintains a
local `.bak` copy on each successful replacement. A web conflict recovery can
be downloaded as JSON and restored by replacing the Dropbox file only after
reviewing both versions.

[PKCE]: https://datatracker.ietf.org/doc/html/rfc7636
