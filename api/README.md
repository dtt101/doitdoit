# Task service API: v1

This contract is experimental. The Go TUI uses it only with explicit service
backend selection; JSON remains the default.
The canonical task types and input validators are in
[service/rust/contracts.rs](../service/rust/contracts.rs). Rust MCP tool schemas describe the same wire inputs. The shared
[task fixture](fixtures/task.json) is checked by Rust and Go tests.

## Identity and transport

All task endpoints require a service-issued OAuth bearer token, bound to the
service origin as its resource/audience. Both tasks:read and tasks:write are
needed for writes; a read-only grant can list/get tasks. OAuth login permits
only the configured GitHub numeric owner ID. No Cloudflare token reaches a client.

HTTP API paths start with /v1. MCP uses /mcp with stateless Streamable HTTP
and JSON responses. Read/write input bodies are limited to 64 KiB, including
MCP and OAuth requests. Import requests allow 256 KiB. Responses contain private data and are not cacheable.
API and MCP clients use the same service-issued credentials and task operations.

## Task representation

See the fixture for exact field names and null handling.

- id is a stable server-generated UUID.
- title is nonempty after trimming, with at most 500 characters.
- notes are preserved plain text, with at most 32,000 JavaScript string units;
  the request byte limit still applies.
- status is open or completed.
- scheduled_date is a valid YYYY-MM-DD calendar date or null. Explicit past
  dates are accepted. There is no rollover or date clamping.
- revision is a positive safe integer, incremented by every accepted update.
- created_at/updated_at are UTC timestamps; completed_at/deleted_at are nullable.
- position is an opaque stable string, sorted lexically then by ID within
  scheduled-date/status groups. New tasks append to their group. Reorder takes
  reviewed neighbors; clients never calculate positions.

There is no separate overdue field: clients derive it from scheduled_date,
status, and the owner's configured timezone. These initial endpoints accept
absolute dates only; relative-date filters and the owner-settings API are
pending. Completion/reopening preserves scheduling.

## Endpoints and tools

| HTTP | MCP tool | Input / response |
| --- | --- | --- |
| GET /v1/tasks | list_tasks | Returns {"tasks": [...]} |
| GET /v1/tasks/{id} | get_task | Returns {"task": {...}} |
| POST /v1/tasks | create_task | operation_id, title, optional notes/scheduled_date; returns {"task": {...}} |
| PATCH /v1/tasks/{id} | update_task | operation_id, expected_revision, changes; returns {"task": {...}} |
| DELETE /v1/tasks/{id} | delete_task | operation_id, expected_revision; soft delete |
| POST /v1/tasks/{id}/restore | restore_task | operation_id, expected_revision; restore with existing fields |
| POST /v1/tasks/{id}/reorder | reorder_task | operation_id, expected_revision, after_id, before_id |
| GET /v1/tasks/{id}/history | task_history | after event cursor, limit; events and next_cursor |
| GET /v1/board | task_board | snapshot_cursor, after ID, limit; tasks, timezone and cursor |
| GET /v1/changes | task_changes | after event cursor, limit; events, cursor and next_cursor |
| GET /v1/export | export_tasks | snapshot_cursor, after ID, limit; portable task export including deleted |

MCP task-specific operations additionally take id. create defaults to empty notes, null date,
and open status. changes accepts title, notes, status, and scheduled_date;
at least one must be supplied. Omission retains a field; explicit null clears
only scheduled_date. Unknown request fields are rejected. Completion is an
explicit desired status, never a toggle.

List is deliberately bounded to 500 active tasks and less than 1 MiB of task
JSON. An oversized result returns result_limit rather than a partial list.
Supply query parameters to use paged listing: status, scheduled_date, undated,
search, include_deleted, after (task ID), and limit (1–100, default 50). Date and
undated filters are mutually exclusive. Search is a substring of title/notes,
with SQLite's ASCII case folding, bounded to 200 UTF-16 units. Paged listing is a
live view; use board snapshots when consistency across pages matters. Empty MCP
list arguments retain the original bounded unpaged behavior.

## Ordering and consistent reads

Reorder is limited to active tasks with the same scheduled_date and status.
after_id is the preceding neighbor; before_id is the following neighbor. Null
means the corresponding group edge. After excluding the moved task, the
neighbors must be adjacent. Stale adjacency returns ordering_conflict (409).
Concurrent insertions into the same gap cannot both use the old neighbor pair.

The server uses bounded base-36 fractional strings and integer arithmetic,
never floating-point ranks. Legacy/equal/exhausted keys trigger atomic group
compaction. Compaction advances affected revisions and records history; a task
whose reviewed revision changed must be reviewed again. Ordinary title/notes
edits do not renumber a group. Restoring or changing group appends the task.

Board/export capture an owner event high-water cursor and reconstruct the last
task state at or before it. Pages are in task-ID order. Keep snapshot_cursor
fixed and pass next_cursor as after on every subsequent page. Board excludes
deleted tasks; export includes them and adds schema_version: 1. Both include
the owner timezone. The portable export contains task state, not OAuth grants,
history or retry receipts; a full D1 backup is separate.

After a snapshot, read changes starting at its snapshot_cursor. Each page
returns ordered events, a safe resume cursor, and a non-null next_cursor when
more events were present. Continue from cursor. Apply replayed events by task
revision, including deletion and compaction events. History/events/receipts are
retained indefinitely; there is currently no expired-cursor window. A future
retention policy must introduce explicit stale-cursor recovery before pruning.

## Explicit import (HTTP only)

POST /v1/imports takes operation_id, exact source_sha256 and expected_count
(maximum 10,000). The returned run ID is stable for the owner's source
fingerprint. GET /v1/imports/{run} reports progress. POST its /tasks with a tasks
array of 1–25 canonical tasks; identical batches are safe to resend, differing
values for a staged ID conflict. POST /commit with {} publishes the complete
validated import atomically with history and a receipt. ID collisions or an
incomplete run fail without publishing a partial board. Staged data is private
and invisible to ordinary readers. These endpoints require both task scopes
and are intentionally absent from MCP.

## Revisions, retries, and conflicts

Clients generate a UUID operation_id before sending a write and retain it
until the outcome is known. A timeout does not establish failure. Retry only the
identical operation ID and command. There is no automatic retry with a new
revision.

Operation IDs are unique within an owner, across clients and transports.
The normalized command (including target task, action, and expected revision)
is hashed. Equivalent input key order/defaults do not change the hash. Repeated
identical commands return the original task acknowledgement even if the task has
changed again. Reuse with a different payload returns operation_id_reused.
Receipts are kept indefinitely in this prototype; there is no expiry/cleanup.

A stale task revision returns HTTP 409 with:

```json
{
  "error": {
    "code": "revision_conflict",
    "message": "This task changed. Review the current version before retrying.",
    "current": { "...": "full canonical current task" }
  }
}
```

MCP returns an isError tool result with the same error under structuredContent.
The losing write creates neither history nor a receipt. Resolve explicitly
against the current revision using a new operation ID.

Other stable errors include invalid_request (400), not_found (404),
operation_id_reused (409), request_too_large (413), unsupported_media_type
(415), result_limit (422), and internal_error (500). Authentication failures use
OAuth challenges (401); insufficient scope uses a scope challenge (403).
Unexpected internal SQL/identity error details are not sent to clients.
