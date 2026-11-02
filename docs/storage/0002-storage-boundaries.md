# JSON storage boundaries

The default desktop backend is a user-owned JSON file. The optional service is a
separate backend; switching is explicit and does not synchronize the two. The web
companion uses the service. See the [service guide](../../service/README.md).

## JSON contract

`taskstore` owns task data and lifecycle operations independent of Bubble Tea.
`JSONStore` implements the existing schema and `Store` interface. Preserve field
names, task IDs, local-calendar `YYYY-MM-DD` buckets and the undated `Future`
bucket. On load, valid dated tasks in `Future` are normalized before incomplete
past tasks roll to Today and explicitly configured retention is applied. Invalid
dates remain available for recovery. Retention defaults to forever.

Writes use atomic replacement and owner-only permissions, with a `.bak` of the
previous valid contents. Loads pair data with its revision; conditional saves
check that revision. Independent bucket changes may merge, but competing edits
within a bucket remain visible conflicts. Identical results may converge. Storage
moves refuse existing destinations. Preserve these guarantees and the existing
recovery behavior when changing the implementation.

JSON conflict recovery discovers adjacent Dropbox conflict copies and requires
explicit review of nonredundant unfinished differences. It preserves recovery
material before cleanup. Failed saves retain a private draft and baseline; drafts
are cleared only after resolution or explicit discard. Ordinary quit is blocked
if a pending draft cannot be persisted, with export or explicit discard available.
Completed differences are not automatically restored. See the [README](../../README.md#recover-tasks-from-dropbox-copies)
for the user workflow.

Revision checks do not eliminate the check-to-replacement race with generic folder
sync. Conflict review cannot recover a version the app never observed or guarantee
a provider uploaded a save. Local recovery copies are not a second synced source
of truth. No immutable-record sidecars or automatic backend migration are used.

## Boundaries and verification

`LoadStore` applies lifecycle maintenance above the adapter. The TUI owns input,
presentation and conflict review; CLI capture uses `taskstore` directly. Service
clients use task operations and revision-guarded commands instead of whole-file
`Store.Load/Save`; service reads do not roll over or prune tasks.

Persistence and recovery tests use temporary files and isolated HOME, never real
tasks or credentials. Exercise revision conflicts, atomic replacement, backup,
permissions, safe moves and recovery failure paths. Follow the repository test,
vet and race requirements when making changes.
