# Task service API: v1 prototype

This contract is experimental and is not used by the released JSON-backed TUI.
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
MCP and OAuth requests. Responses contain private data and are not cacheable.
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
- position is an opaque stable string. The prototype initializes it to the
  generated task ID and sorts by position, then ID. Creation order and user
  reordering are not yet implemented; clients must not interpret this key.

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

MCP get/update additionally take id. create defaults to empty notes, null date,
and open status. changes accepts title, notes, status, and scheduled_date;
at least one must be supplied. Omission retains a field; explicit null clears
only scheduled_date. Unknown request fields are rejected. Completion is an
explicit desired status, never a toggle.

List is deliberately bounded to 500 active tasks and less than 1 MiB of task
JSON. An oversized result returns result_limit rather than a partial list.
Pagination, search, history reads, deletion/restoration, reorder, export, and the
consistent board/change-feed contract belong to the next service milestone.

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
