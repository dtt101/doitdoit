# Cloudflare task service

A Rust/WASM Cloudflare Worker with D1 storage, HTTP task endpoints, authenticated
MCP, and GitHub owner login. All application code is in `rust/`. There is no
TypeScript layer or runtime npm dependency. Cloudflare's Rust SDK generates the
JavaScript/WASM entrypoint. Node is used for the official `cf` CLI, its Wrangler bundler, and the local workerd test
harness, which invokes the compiled Rust service.

This is the first part of milestone 1 in the [accepted plan](../plans/cloudflare-task-service.md).
The real local GitHub login and `make smoke` flow have passed. The initial deployment
is directly to production with an empty D1 database, per the user's decision;
there is no separate staging environment. The existing Go TUI still uses JSON.
The Worker is deployed as of 2026-10-20. Health, discovery and unauthenticated
access checks pass. Production owner login, Codex MCP reads/writes, and a
separate refresh/revocation test have passed. ChatGPT/Claude interoperability
and automatic client refresh remain to be checked.

Implemented: create/get/list/update, explicit scheduling/status, per-task
revisions, atomic history and idempotent acknowledgements, shared HTTP/MCP
operations, GitHub owner login, and a separate Go HTTP client proof of concept.
Pending: real assistant interoperability, ordering/reorder, board/change-feed consistency, remaining task
operations, TUI integration, migration, and deployment/restore validation.

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
The consent form policy allows GitHub and the selected callback origin, so an
existing GitHub session can redirect straight back to the client. It does not
allow arbitrary callback origins or wildcards.
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
client accepts a token source; persistent desktop login is a later step.

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
Actual ChatGPT/
Claude discovery, login, tools, refresh and revocation will be verified on the
initial production endpoint with disposable tasks before real-data cutover.

## MCP

Rust serves stateless Streamable HTTP in JSON response mode at `/mcp`, targeting
MCP 2025-11-25 with compatible 2025-03-26/2025-06-18 request headers. It exposes
initialize, ping, tools/list, and tools/call for list/get/create/update. It does
not offer SSE subscriptions, resumability, sessions, sampling, or notifications
from the server. Notifications receive HTTP 202 without performing mutations.
Both transports invoke the same Rust Tasks methods and enforce identical scopes.

## Atomic task writes

The application inserts one operation_receipts command. SQL triggers reject a
mismatched operation ID or stale task revision, record before-state, mutate the
task, record after-state, and save the acknowledgement in the same statement.
Any failure rolls back all effects. Identical retries return the original
acknowledgement, even after later edits. Canonical sorted JSON normalizes key
order and defaults before hashing. Tests race writes/retries, inject a failure
at receipt storage, and check exact event/receipt counts.

Dates never change during reads or midnight. History and receipts are retained
indefinitely. Delete, restore, history reads, ordering and export are pending.
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
make migrate-production # applies additive SQL to the named production D1
make deploy             # uploads Rust Worker and production secrets
```

`make deploy` intentionally requires the private secrets file. Deploy does not
import tasks or switch the TUI. Validate health, discovery, owner login and actual
ChatGPT/Claude MCP operations after deploy. D1 recovery, independent export and a
restore drill remain required before real-data cutover.

The user confirmed Workers Free; no subscription upgrade is automated.
Free daily quotas can interrupt service until reset. If the account already has
Workers Paid, its billing rules apply. Monitor usage in the Cloudflare dashboard.
Logs omit invocation URLs and traces redact query strings to protect OAuth codes.
