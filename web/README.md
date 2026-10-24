# Web companion

Static mobile client for the same service used by the TUI and MCP. No build step,
framework or Dropbox connection.

## Use

Open https://dtt101.github.io/doitdoit/ and choose **Sign in with GitHub**. Only
the service's configured owner can sign in. On your phone, use **Add to Home
screen** in the browser menu (Safari's share menu on iOS).

- Add tasks to Today, Tomorrow, Future (undated), or an explicit date.
- Tap a title to edit its title/date; use the note icon to read notes.
- Tap a checkbox to complete or reopen without changing the scheduled date.
- Drag ≡ to reorder within a date/status group. Moving to another date appends
  to that group. Keyboard: Space/Enter picks up, arrows move, Space/Enter saves,
  Escape cancels.
- Overdue tasks appear before Today with their original dates. Nothing rolls
  forward or expires. The menu can show earlier completed tasks.
- Delete is recoverable through the TUI's Deleted view (`x`, then Enter).

Dates use the service owner's timezone. The visible page refreshes every 15
seconds and on focus; active editors and drag previews are left intact.

## Failed saves

Editing requires an internet connection. Each command is retained in browser
storage before it is sent, with its original operation ID and reviewed revision.
Further edits pause until it is resolved.

- **Save retained:** menu → **retry retained save**. This safely retries the
  identical command, including after reloading or signing in again.
- **Save rejected:** download the recovery copy, then choose **Review latest
  after rejected save**. Review the current task and apply the desired edit again.
  The rejected command remains in local recovery; it is never silently merged.
- **Sign-in refresh interrupted:** menu → **Sign in again**, then retry any
  retained save. A rotating refresh credential is never replayed after an
  uncertain response.

Refreshing tasks does not discard retained commands. Disconnect revokes this
browser's credentials; retained saves/recovery stay on the device. Clearing site
data removes them. Download recovery first if anything is unresolved. Existing
Dropbox recovery copies remain downloadable, but are never imported automatically.
The old Dropbox JSON is no longer updated by this app.

## Development and deployment

```sh
node --test web/*.test.js
python3 -m http.server 8000 --directory web
```

For local service use, set `serviceOrigin` in `config.js` to
`http://localhost:8787`, and change `connect-src` in `index.html` to that origin.
The local service allows `http://localhost:8000`; use that exact URL, not 127.0.0.1.

Production is served by the existing GitHub Pages workflow when `web/` changes
on main. Deploy the service first when changing browser access. Its production
`BROWSER_ORIGIN` is exactly `https://dtt101.github.io`; consent/callback, MCP and
import endpoints are excluded from cross-origin access. No Cloudflare credentials
belong in browser configuration.

`auth.js` owns public-client OAuth (S256 PKCE and checked state), rotating tokens
and cross-tab refresh locks. `sync.js` owns snapshot pagination and retained
revision-guarded commands. `domain.js` provides calendar/input and drag-preview
helpers; rendering never writes tasks.

The browser needs HTTPS (or localhost), Web Crypto and Web Locks, supported by
current Chrome and Safari. Credentials and recovery commands are browser-local
and accessible to JavaScript. Use a trusted browser/profile; there are no remote
scripts, fonts, analytics or task caches. Requests go to the configured service;
sign-in uses GitHub. Hosting providers may retain ordinary request metadata.
