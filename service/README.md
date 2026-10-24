# Cloudflare task service

The Go TUI and assistant MCP clients share tasks in Cloudflare D1 through a
Rust/WASM Worker. JSON remains the default until you explicitly select the service;
there is no automatic import or synchronization with the JSON file.

## Use the TUI

From the repository root:

```sh
go run . service login
go run . --service https://doitdoit-service.davidt-91d.workers.dev
```

Login uses GitHub. Credentials and retained commands are stored privately under
your OS configuration directory in `doitdoit/services/`.

| Action | Command / key |
| --- | --- |
| Use the service on future launches | `go run . service use` |
| Return to JSON | `go run . service use-json` |
| Open an explicit JSON file | `go run . --file PATH` |
| Show overdue tasks | `o` |
| Show deleted tasks / restore selected | `x` / `enter` |
| Refresh or retry a retained write | `r` |
| Undo using the task revision | `u` |
| Review / archive a retained failed command | `d` / a second `d` |

Overdue tasks retain their dates. Conflicts require review. Archiving an uncertain
command does not cancel a possible server write: check its outcome before
recreating it. CLI captures can be resumed with
`go run . service retry --file RETAINED_COMMAND`.

After service edits, the old JSON file is stale. Export current service data
before switching back. The [mobile web companion](../web/README.md) uses the same
service as the TUI and MCP; sign in with GitHub.

## Connect an assistant

Use the name `doitdoit` and this MCP URL:

```text
https://doitdoit-service.davidt-91d.workers.dev/mcp
```

The service uses automatic OAuth client registration and GitHub owner login.
No Cloudflare credential or manually supplied client secret is needed. Complete
sign-in if prompted; clients may reuse existing authorization.

### ChatGPT

1. Open **Plugins → + → Add custom MCP server**; labels may vary.
2. Enter the name and URL, then finish adding the connection.
3. In a new conversation, select `doitdoit` from **@** and ask it to list tasks.

The tested form needs only name and URL.
[OpenAI connection guide](https://developers.openai.com/plugins/deploy/connect-chatgpt).

### Gemini

1. Open Gemini on the web: **Settings → Connected Apps → Custom apps**.
2. Add the MCP URL, select **Next**, and complete sign-in.
3. Select the app through **@** and ask it to list tasks.

If sign-in stalls, start a fresh connection attempt. On the continuation page,
use **Continue** if automatic navigation pauses. If Custom apps is unavailable,
check [Google's account and region requirements](https://support.google.com/gemini/answer/17209137).
Gemini CLI is a separate client.

### Check shared tasks

Create a labelled test task with an explicit date and note through the assistant.
Find it in the TUI and press `r`. Edit it in the TUI, then ask the assistant to
fetch it again. Confirm the status, date and note. After 16 minutes idle, fetch
again without reconnecting to check session continuity.

ChatGPT and Gemini shared-task checks have passed. Claude is deferred; remaining
checks are listed in the [plan](../plans/cloudflare-task-service.md#remaining-work).

## Import and export

From the repository root:

```sh
go run . service export --out /path/to/new-private-export.json
go run . service import --file /path/to/tasks.json --dry-run
```

Review the frozen source, mappings, issues and recovery material before applying:

```sh
go run . service import --file /path/to/tasks.json --apply --reviewed-recovery
```

Export refuses to overwrite files. Import preserves source bytes and task fields,
resumes repeated sources, publishes atomically, and verifies the result against
an export. Import does not select the TUI backend. Real-task import and backend
cutover require an explicit decision; see the [cutover steps](OPERATIONS.md#cutover).

## Develop locally

Use Go from `mise.toml`, Rust 1.93+ with rustup, Node 24+, Python 3, GnuPG and Make.
From `service/`:

```sh
make setup          # install pinned development tools and WASM target
make check          # Rust formatting, Clippy, unit and Worker/D1 tests
make test-backup    # isolated SQL validation and encryption tests
make bundle         # development deployment dry-run
make migrate-local  # apply local migrations
make dev            # http://localhost:8787
```

For real local login, create a GitHub OAuth app with callback
`http://localhost:8787/callback`. Put these values in ignored `.dev.vars` and
set its permissions to `0600`:

```text
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
GITHUB_OWNER_ID=...
```

With the local Worker running, `make smoke` opens GitHub login and tests HTTP/MCP
using a labelled task. Tests otherwise use disposable local databases and mocked
GitHub, require loopback access, and never use real task files or accounts.

All application code is Rust in `rust/`. Node is Cloudflare tooling only.
`cloudflare.config.ts` owns environments and bindings; `wrangler.config.ts`
provides Rust build settings; `wrangler.jsonc` configures isolated tests.
Use `cf` through Make. Wrangler remains necessary for local migrations with the
existing placeholder ID and full SQL export. Do not edit generated `build/`,
`target/` or `.tools/` contents.

## Operations

Encrypted backups run daily at **03:43 UTC** on GitHub. Daily copies last seven
days; Sunday copies last 35 days. The first hosted export and downloaded-artifact
recovery passed. The private recovery key stays outside GitHub.

See [Operations](OPERATIONS.md) for deployment, backup controls, recovery and
cost limits; the [API contract](../api/README.md) for protocol details; and the
[plan](../plans/cloudflare-task-service.md) for architecture and remaining work.
