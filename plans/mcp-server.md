# MCP server

## Goal

Let Claude (and other MCP clients) read and change doitdoit tasks through a
small set of tools. A desktop client should work directly against the local
task file, and a hosted endpoint should let the Claude mobile app reach the
Dropbox-hosted file.

The JSON file stays authoritative, as `plans/major-version-storage.md`
requires. The MCP server is another client of the existing `taskstore.Store`
boundary, not a new backend.

## Non-goals

- No new storage format, database or sync service.
- No Dropbox conflicted-copy recovery or merge review over MCP. That stays in
  the TUI, where a person can see both sides.
- No `config move`, theme or retention changes over MCP.
- No pruning of completed history from the MCP server (see Lifecycle).

## Shape

Add a `doitdoit mcp` subcommand built on the official Go MCP SDK
(`github.com/modelcontextprotocol/go-sdk`). It runs in two modes.

**stdio (phase 1).** This mode uses the existing JSON file store at the
configured path, or at `-file`. Atomic replacement, `0600` permissions, `.bak`
creation and revision checks all come for free. It covers Claude Desktop and
Claude Code, and needs no network code.

**Streamable HTTP (phase 2).** This mode uses a new `DropboxStore` that
implements `taskstore.Store`:

- `Load` downloads the file. It records the content hash in `Revision`, as
  the file store does, and keeps the Dropbox `rev` for that hash internally.
- `Save` checks that the expected `Revision` matches the last download, then
  uploads in Dropbox update mode pinned to that `rev`. A Dropbox conflict
  returns `ErrDataConflict`.
- `.bak` must not be weakened. Before replacing the main file, write the
  previous contents to the `.bak` sibling. If that write fails, abort the
  save.
- `Move` returns an unsupported error.
- Mirror the revision handling in `web/sync.js` so the hosted server and the
  web companion behave alike against the same file.

The tools layer must not know which store it is talking to.

## Lifecycle

This must stay aligned with the CLI and web companion.

- **Reads** run migration, rollover and completion grouping in memory, so
  results match what the TUI shows. They never save.
- **Writes** call `MaintainSnapshot` with `retentionDays = 0`, apply the
  operation, and save once.
- **Pruning never runs from the MCP server.** Retention lives in the local
  `~/.doitdoit_config.json`, which the hosted server cannot see. Passing 0
  keeps stdio and HTTP behaviour identical and leaves pruning to the TUI.

## Tools

Every tool is a single intent addressed by task ID. List results always
include IDs.

| Tool | Inputs | Notes |
| --- | --- | --- |
| `list_tasks` | `range`: `today`, `date`, `next_days`, `future`; optional `date`, `days`, `include_completed` | Returns tasks grouped by bucket. |
| `add_task` | `title`, optional `when`, `notes` | `when` accepts the same targets as `doitdoit add --when`. |
| `complete_task` | `id`, optional `completed` (default true) | Restores completion grouping. |
| `update_task` | `id`, optional `title`, `notes` | Changes only the fields provided. |
| `move_task` | `id`, `to`: `today`, `future`, `+N` days, or a date | Follows TUI move semantics; a past date moves the task to Today. |
| `delete_task` | `id` | |

Task behaviour belongs in `taskstore` (`operations.go`, `capture.go`,
`move.go`). Where an operation the tools need is missing, add it there with
tests, so the MCP layer stays a thin adapter and the TUI can use it too.

## Conflicts

On `ErrDataConflict`, reload and reapply the same operation once. Operations
are addressed by ID, so reapplying is safe. If the task no longer exists
after the reload, return a clear error.

A second conflict, or any same-bucket ambiguity, returns an error to the
client rather than guessing. This preserves the rule that a competing edit
stays visible.

## Auth (phase 2)

- The HTTP endpoint implements OAuth per the MCP authorization spec, so it
  can be added as a Claude custom connector. An authless endpoint, or one
  protected only by an unguessable URL, is not acceptable for a full task
  list.
- The server is single-user. It holds a Dropbox refresh token as a deployment
  secret and requests only file read and write scopes.
- This is a different trust model from the web companion, whose OAuth tokens
  stay browser-local. Document that clearly, and keep the server outside the
  CLI release's security and privacy assurance, as the web companion is.
- No secrets are committed. Configuration comes from environment variables.

## Phases

1. **stdio.** Add `doitdoit mcp`, the tools and any missing `taskstore`
   operations. Document the Claude Desktop and Claude Code setup in
   `README.md`.
2. **DropboxStore.** Add the adapter and its tests against a fake Dropbox API
   (`httptest`), with no network calls in tests.
3. **HTTP and auth.** Add the Streamable HTTP transport, OAuth, deployment
   notes and connector setup.

## Testing

- Follow `AGENTS.md`. Use `t.TempDir()` and isolate `HOME` with
  `t.Setenv()`, and never touch the real task file or config.
- Write tool-level tests for each tool against an in-memory or temporary
  file store.
- Write conflict tests: an external change between load and save leads to
  one successful retry, and a second change returns an error.
- Write lifecycle tests: reads never save, writes never prune, and rollover
  matches `MaintainSnapshot`.
- Run `go test -race ./...`, since this touches persistence and concurrency.
- Update `THIRD_PARTY_NOTICES.md` for the SDK and run
  `go test -count=1 ./releasecheck`.

## Open questions

- Should HTTP mode be a subcommand flag (`doitdoit mcp --http`) or a separate
  binary, so the released CLI does not carry server and OAuth dependencies?
- Where should it be hosted? It needs a small always-on or scale-to-zero
  service with secret storage.
- Should the hosted server reuse the web companion's Dropbox app
  registration, or register a separate app with server-side credentials?
