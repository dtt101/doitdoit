# Cloudflare task service

A Rust/WASM Cloudflare Worker with D1 storage, HTTP task endpoints, authenticated
MCP, and GitHub owner login. All application code is in `rust/`. There is no
TypeScript layer or runtime npm dependency. Cloudflare's Rust SDK generates the
JavaScript/WASM entrypoint. Node is used for the official `cf` CLI, its Wrangler bundler, and the local workerd test
harness, which invokes the compiled Rust service.

Implementation follows the [accepted plan](../plans/cloudflare-task-service.md).
The real local GitHub login and `make smoke` flow have passed. Deployment is
directly to production, per the user's decision; there is no staging environment.
The Go TUI defaults to JSON and offers explicit service backend selection.
The Worker is deployed as of 2026-10-20. Health, discovery and unauthenticated
access checks pass. Production owner login, Codex MCP reads/writes, and a
separate refresh/revocation test have passed. User-observed ChatGPT reads,
creation, completion and edits in both directions with the TUI passed on
2026-10-24, followed by a successful idle-session read. Fresh ChatGPT login and
revocation remain to be checked. Gemini sign-in, listing and creation visible in
the TUI passed by user report after the consent fix. Completion in the TUI was
read back correctly by Gemini with date and note preserved. Gemini completion
writes, idle refresh and Claude remain unverified.

Implemented locally: create/get/list/update, soft delete/restore, history,
filter/search, neighbor-based reorder with compaction, immutable paged boards,
change feeds, portable export and staged atomic import. HTTP and MCP share task
operations, per-task revisions and atomic history/receipts. The Go TUI/CLI include
desktop OAuth, private rotating credentials, retained retry commands and explicit
backend selection. Hosted validation and recovery evidence are tracked in the plan.

## Local development

Use Rust 1.93+ with rustup, Make, and Node 24+. From `service/`:

```sh
make setup          # WASM target, pinned worker-build, Cloudflare dev tools
make check          # Cargo checks, WASM build, local Worker/D1 tests
make bundle         # cf deployment dry-run only
make migrate-local  # empty local D1 schema
make dev            # local Worker
```

For Rust checks alone, run `make check-rust` (cargo fmt, Clippy with warnings
as errors, and cargo test). `make build` compiles WASM; `make test` builds and
runs integration tests. Cargo.lock pins Rust dependencies. package-lock.json
contains only development tools. No npm check script wraps the Rust toolchain.
The package manifest approves the pinned esbuild and workerd install scripts,
which prepare and check the platform binaries used by Cloudflare's tools.
Review these approvals when upgrading those dependencies.

`make setup` installs worker-build 0.8.7 under ignored `.tools/`, without changing
your default toolchain. It is safe to rerun: it reuses the pinned helper version
and replaces a different version if needed. The first build downloads Wasm binding/optimization
helpers and may take a few minutes. CI uses Rust 1.93.1. Generated `build/`,
`target/`, and `.tools/` are ignored; do not edit generated JavaScript.
The Rust SDK's supported `--no-panic-recovery` mode avoids experimental catch
wrappers incompatible with this toolchain.

`make dev` listens on 127.0.0.1. Use http://localhost:8787/health to match the
configured service origin. D1 uses a placeholder ID and local `.wrangler/`
storage. There is no KV binding. These commands provision no cloud resources.
Development mode disables workers.dev; production enables its stable URL. Preview URLs are disabled in both modes.

Tests use ephemeral workerd/D1 data and mocked GitHub responses, with no real
tokens, Cloudflare account, or developer tasks. They require loopback access.
The static Dropbox web companion retains its existing no-build setup.

## Authentication

Rust implements the public-client OAuth endpoints, with GitHub providing the
upstream identity. Only the configured numeric GitHub user ID may authorize a
grant. Removing/changing the owner ID blocks previously issued task credentials.

The browser must approve the displayed client name, exact redirect, and scopes.
Client names are escaped and explicitly unverified. Consent and upstream state
are short-lived, single-use and bound to an HttpOnly, SameSite browser cookie.
Only S256 PKCE and exactly registered HTTPS/loopback redirects are supported.
The consent form submits only to this service. Approval returns an HTML
continuation page before navigating to GitHub, so the form's security policy
does not constrain GitHub's or the client's later redirects. A Continue link
is available if automatic navigation is blocked. Denial uses the same document
boundary to return to the exactly registered callback. Neither path permits
arbitrary callback addresses or requires scripts or wildcard form permissions.
GitHub's access token is used to fetch identity and is never stored or forwarded.

Clients use dynamic registration at `/oauth/register`, authorization at
`/authorize`, code/refresh exchange at `/oauth/token`, and revocation at
`/oauth/revoke`. Discovery documents advertise these endpoints. Client ID
Metadata Documents and confidential clients are not implemented or advertised.
The canonical resource/audience is SERVICE_ORIGIN for both HTTP and MCP.
Scopes are tasks:read, tasks:write, and optionally offline_access.

Access tokens expire after 15 minutes. Refresh tokens rotate and expire after
30 days. D1 stores only SHA-256 credential digests. Code/refresh consumption and
successor issuance happen in one SQL statement, guarded by a trigger against
concurrent replay. Reusing a consumed credential revokes its grant. Revocation
invalidates all access and refresh tokens belonging to that grant. No signing
keys or custom cryptographic algorithms are used.

To exercise real local login, register a GitHub OAuth app with callback
http://localhost:8787/callback and create a private `.dev.vars`:

```text
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
GITHUB_OWNER_ID=...
```

Never commit credentials, OAuth secrets, or local databases. OWNER_TIMEZONE is
explicit in cloudflare.config.ts. There is no authentication bypass. The Go prototype
client uses browser PKCE login and private credentials. See the root README for
`service login`, session selection, export and reviewed import commands.

With `.dev.vars` configured, migrations applied, and `make dev` running, open a
second terminal in `service/` and run:

```sh
make smoke
```

This development command uses the existing Go client (and the repository's Go
toolchain) to test the Rust service at http://localhost:8787. It opens browser
consent and GitHub login, receives the callback on a temporary loopback port,
and checks a task's past date, date update, identical retry, stale-write conflict,
and MCP read-back. It leaves one clearly labelled **completed test task** in the
local database. Tokens stay in memory and the temporary login is revoked on
completion or failure. Login times out after five minutes; Ctrl-C cancels it.
If the browser does not open automatically, use the link printed in the terminal.
This tests real GitHub login and both service transports; hosted assistant
interoperability still requires the reachable HTTPS production endpoint.

This is a narrow OAuth implementation, not a general identity platform.
The public auth routes enforce per-IP edge limits: 10 registrations/minute and
60 other auth requests/minute per Cloudflare location. These are abuse controls,
not globally precise quotas. Hourly cleanup removes expired login state and
credentials for dead grants; spent credentials remain while successors are live
so replay detection continues working. Task history/receipts are untouched.
Track actual assistant discovery, login, tools, refresh and revocation checks
in the plan. Use disposable tasks before real-data cutover.

## MCP

Rust serves stateless Streamable HTTP in JSON response mode at `/mcp`, targeting
MCP 2025-11-25 with compatible 2025-03-26/2025-06-18 request headers. It exposes
initialize, ping, tools/list, and 11 tools for list/get/create/update,
delete/restore/history, board/changes/export and reorder. Import stays HTTP-only.
It does
not offer SSE subscriptions, resumability, sessions, sampling, or notifications
from the server. Notifications receive HTTP 202 without performing mutations.
Both transports invoke the same Rust Tasks methods and enforce identical scopes.

### Connect an assistant to doitdoit

Use the hosted server URL, including `/mcp`:

```text
https://doitdoit-service.davidt-91d.workers.dev/mcp
```

Name the connection `doitdoit`. The service uses OAuth with automatic client
registration and GitHub owner login; no manually supplied client secret or
Cloudflare credential is needed. Complete sign-in when prompted. A client may
already have authorization, so absence of a new sign-in prompt alone does not
prove anonymous access. Anonymous MCP requests return HTTP 401.

These clients access D1 service tasks. Connecting an assistant does not import
the Dropbox JSON file or select the service as the TUI's default backend.

#### ChatGPT

1. Open Plugins and choose **+ → Add custom MCP server** (labels can vary by UI).
2. Enter `doitdoit` and the URL above. The interface tested here asked only for
   name and URL; a description or OAuth selector is not required in that form.
3. Finish adding the server and complete authentication if prompted.
4. Start a conversation, select `doitdoit` through the **@** menu, and ask:
   “Use doitdoit to list my tasks. Do not create or change anything.”

User-observed reads, creation, completion and TUI interoperability passed on
2026-10-24. A later idle-session read also succeeded; the underlying refresh
exchange was not independently traced. The initial connection showed no login
prompt, so fresh-login behavior remains a separate check.
See [OpenAI's connection guide](https://developers.openai.com/plugins/deploy/connect-chatgpt).

#### Gemini web app

1. Open Gemini on the web, then **Settings → Connected Apps**.
2. Under **Custom apps**, choose the field or action to add a custom app.
3. Paste the MCP URL above, select **Next**, and follow the sign-in instructions.
   Automatic client registration is supported; manual credentials should not
   be needed.
4. Select the app with **@** and request a read-only task list first.

As checked on 2026-10-24, Google's guide limits this feature to adults in the US
using personal Google accounts, in English, with Keep Activity enabled. If
Custom apps is absent, account or regional availability may prevent this route.
User confirmed successful Gemini connection, task listing and creation on
2026-10-24 after the consent fix. The created task's date and note were confirmed
in the TUI. User then completed it in the TUI; Gemini fetched it as completed
with the original date and note preserved. Gemini completion writes and idle
refresh remain to be checked.
If a previous attempt stopped on the consent button with a `form-action` console
error, restart the connection from Gemini. The continuation-page fix was deployed
on 2026-10-24; an old consent page still has the previous policy and a used handle
cannot be submitted again.
See [Google's custom apps guide](https://support.google.com/gemini/answer/17209137).
Gemini CLI is a separate client with
[remote MCP support](https://geminicli.com/docs/tools/mcp-server/); a CLI test
would not establish compatibility with the regular Gemini app.

#### Verify shared tasks with the TUI

From the repository root:

```sh
go run . service login
go run . --service https://doitdoit-service.davidt-91d.workers.dev
```

Create a clearly labelled test task through the assistant with an explicit date
and note. Find it on that date in the TUI. Complete it through the assistant,
read it back, then press `r` in the TUI and confirm completion. Reopen it in the
TUI and ask the assistant to fetch its current status. Check that date and note
survive both changes. Finally, leave the assistant connection idle for at least
16 minutes and fetch again without reconnecting, to exercise session continuity
beyond the 15-minute access-token lifetime. Record observed results separately
from any independently traced token refresh.

Claude testing is deferred. Remaining client and cutover gates are tracked in
the [plan](../plans/cloudflare-task-service.md).

## Atomic task writes

The application inserts one operation_receipts command. SQL triggers reject a
mismatched operation ID or stale task revision, record before-state, mutate the
task, record after-state, and save the acknowledgement in the same statement.
Any failure rolls back all effects. Identical retries return the original
acknowledgement, even after later edits. Canonical sorted JSON normalizes key
order and defaults before hashing. Tests race writes/retries, inject a failure
at receipt storage, and check exact event/receipt counts.

Dates never change during reads or midnight. History and receipts are retained
indefinitely. Migration 0004 preserves existing task/history/receipt rows while
adding group ordering, recoverable deletion and staged imports. An integration
test applies it to a populated 0001–0003 database and checks exact preservation.
See the [API contract](../api/README.md) for input/result bounds and errors.

## Production deployment

Use `cf` via Make from this directory. Local migrations retain Wrangler because
`cf` beta rejects the existing nil local database ID; this preserves the local
database used by the smoke test. Production migrations use `cf`. `cloudflare.config.ts` is authoritative
for development/production; `wrangler.config.ts` supplies only Rust build settings.
`wrangler.jsonc` remains the isolated Vitest configuration. The `.ts` files are
Cloudflare tooling configuration, not application code.

Target account: `91d8cf0b8c805dac586f9ab55d6d6212`.
Worker: `doitdoit-service`, origin `https://doitdoit-service.davidt-91d.workers.dev`.
D1: `doitdoit-prod` / `43659423-7b12-42ac-8f1e-fc97f3ed0fea`, Western Europe,
read replication disabled. No separate staging resources.

Create a production GitHub OAuth app with the service origin as homepage and
`https://doitdoit-service.davidt-91d.workers.dev/callback` as callback.
Store its `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, and the numeric
`GITHUB_OWNER_ID` in ignored `.env.production`, with file permissions `0600`.
Keep `.dev.vars` for the separate localhost OAuth app. Never paste secrets in chat.

```sh
make check
make bundle-production # builds and previews; uploads nothing
make migrate-production # applies migrations to the named production D1
make deploy             # uploads Rust Worker and production secrets
```

`make deploy` intentionally requires the private secrets file. Deploy does not
import tasks or switch the TUI. Validate health, discovery, owner login and actual
ChatGPT/Claude MCP operations after deploy. D1 recovery, independent export and a
restore drill remain required before real-data cutover. See [recovery procedures](OPERATIONS.md).

For the extended hosted smoke (creates and completes two labelled test tasks,
rotates a temporary desktop login and revokes it):

```sh
go run ../devtools/service-smoke --url https://doitdoit-service.davidt-91d.workers.dev --extended
```

This needs one browser approval and tests reorder, delete/restore, history,
changes, all 11 MCP tools being discoverable, portable export and revoked access.
It never imports source data or selects the TUI backend.

The user confirmed Workers Free; no subscription upgrade is automated.
Free daily quotas can interrupt service until reset. If the account already has
Workers Paid, its billing rules apply. Monitor usage in the Cloudflare dashboard.
Logs omit invocation URLs and traces redact query strings to protect OAuth codes.
