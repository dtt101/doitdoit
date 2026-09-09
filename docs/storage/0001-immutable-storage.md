# ADR 0001: immutable storage protocol

Status: accepted design for implementation, not an enabled storage format.
Protocol: 1. Activation: next major application release, after plan stages 2–9.
Scope: [stage 1](../../plans/major-version-storage.md). No current writer changes.

## Decision

Use immutable, content-addressed JSON records in `<configured-json-path>.store/`.
Keep the configured JSON file as a legacy input, never a continuously rewritten
projection. Reconstruct current tasks from imports and atomic changes. Keep the
existing conservative same-bucket conflict rule until a separately reviewed policy
change. This deliberately favors visible conflicts over silently choosing winners.

The alternative of a single shared JSON/JSONL/SQLite file still has competing
writers. One mutable file per task reduces collisions but loses history and needs
delete/order conflict handling. A hosted service changes the distribution model.
An immutable protocol retains user-owned storage at the cost of replay, dependency,
and migration complexity. It cannot eliminate provider delays or recover writes
that no upgraded client ever observed.

## Discovery and files

Given `/Tasks/doitdoit.json`, append `.store` to that exact path (do not replace its
extension). Files in the sibling directory are:

```
records/<sha256>.json     canonical protocol record bodies
legacy/<sha256>.json      exact original legacy bytes, including whitespace
```

Hashes are lowercase, 64-character SHA-256 hex. Record hashes cover canonical
body bytes; legacy hashes cover raw bytes. There is no mutable manifest, head,
counter, shared device configuration, or single elected migrating device.
Temporary files are outside these namespaces and never replayed. Unexpected
directory contents must not be overwritten or treated as a fresh store.

Folder clients scan both namespaces, including replacement of previously seen
paths. Dropbox clients use these same relative names under the existing configured
API path, list all pages, and rescan on cursor reset. Only a provider's explicit
not-found result means absence; permission, network, and other errors never do.
New Dropbox uploads are create-only with autorename disabled. On an existing name,
download and verify exact content; never overwrite. Local publication likewise
must be exclusive and atomic on the supported Linux and macOS desktops,
including Omarchy. Sync file data and containing directories before acknowledging
local durability; a Windows no-op durability fallback is outside the support scope.
Preserve local `0600` files, restrictive directories, atomic replacement/backups
for mutable local state, and existing legacy-save safeguards while that adapter lives.

Cache/outbox/device identity is local and scoped to provider account plus resolved
anchor path (local paths use the existing platform path rules). A moved store must
rebind that scope explicitly; account switching must not reuse an outbox. Caches
are disposable. Pending unpublished edits are durable user data, not caches, and
cannot be deleted during rebuild. A copied complete directory is a fork until a
user deliberately reconnects it; there is no global account service.

## Canonical bytes and identity

`record.schema.json` defines the structural body. These additional rules are
normative and must be tested in both languages:

1. Input JSON must be UTF-8 with no BOM, duplicate object keys, unpaired surrogates,
   non-finite numbers, or integers outside the JavaScript safe-integer range.
2. Object keys sort by unsigned UTF-16 code units. Arrays retain their order.
   Emit no whitespace. Emit integers in decimal without exponent or leading zeros;
   negative zero becomes zero. Emit booleans and null as JSON literals.
3. Emit string quotes and backslashes as `\"` and `\\`. Emit every other code unit
   outside U+0020–U+007E as lowercase `\uXXXX` (including newline and surrogate
   pairs); printable ASCII is literal. Do not normalize Unicode. Encode the result
   as ASCII/UTF-8. This small profile avoids Go/JavaScript escaping differences.
4. Record IDs are SHA-256 of those bytes. A record references IDs, never filenames,
   clocks, or fixture aliases. An ID with different content is corruption: preserve
   evidence, report it, and stop dependent replay/publication.

Imports normalize legacy data before hashing: preserve bucket arrays and all valid
task field values; absent `due_date` becomes `""`; absent/empty buckets are omitted.
Bucket names are valid local-calendar `YYYY-MM-DD` or exactly `Future`. A valid
`created_at` is retained as its original string, including offset and precision;
equivalence does not collapse different timestamp spellings. Never parse dates
through UTC to choose buckets. Reject unknown legacy fields into recovery instead
of silently dropping future-client data. Missing required title/completion/time,
wrong types, and invalid dates are errors, not an empty import.

Valid globally unique nonempty task IDs are retained. For absent/empty/duplicate
IDs, compute `repair:<snapshot-hash>:<bucket>:<index>` where snapshot-hash covers
the canonical parsed legacy object before repair, index is zero-based decimal,
and every occurrence of a duplicate ID is repaired. Preserve raw originals and
flag identity ambiguity for reconciliation across different snapshots. Never match
two tasks by title alone. New tasks use `task:<random-128-bit-lowercase-hex>`; writers
must retry locally detected collisions. Imported completions have unknown times.

## Records and causal state

`import` contains one normalized full legacy snapshot and no observation time or
device ID. Equivalent normalized imports have identical IDs. It establishes an
initial candidate state, not a stream of new-task/completion events.

`activate` references an import ID and a raw legacy backup hash. Verify the backup
parses/normalizes to that import before activation. Multiple activations are valid;
different raw formatting may produce different activations for the same import.
Fresh installs activate an empty import with an exact `{}` backup. No separate
empty-store shortcut bypasses this protocol.

`change` contains sorted unique causal `parents`, locally generated device ID and
128-bit random nonce, UTC RFC3339 time with milliseconds, local calendar day and
UTC offset in minutes, action metadata, and sorted bucket replacements. Parents
are the maximal record IDs in the complete causal state seen by its author, not
just the last edit to this task. Activation is a parent of the first change.
Parent references must form an acyclic graph. Timestamps never order changes.
An activation has an implicit causal edge to its import; backups are required
availability dependencies, not causal records. Imports have no parents. Do not
emit an ordinary change before an activation exists in its ancestor closure.

Bucket replacements carry `before` and `after` task arrays; an empty array means
the bucket is absent. This is the normative state delta in protocol 1. Action
metadata describes intent/history and must agree with that delta. Enumerated
actions are create, edit, complete, reopen, move, reorder, delete, maintenance,
undo, and resolve. `task_ids` lists exactly the affected tasks (including order
changes). Move replaces both buckets atomically; reorder records complete order;
delete removes a task from the projection but leaves history. A maintenance batch
may affect multiple tasks. All other actions except resolve/undo must implement
their named intent only; do not disguise arbitrary edits as completion events.

`before` must exactly match the projection at the parents. Validate date fields,
global task ID uniqueness, stable active-before-completed ordering, and all touched
buckets as a unit. Invalid batches apply nowhere. Undo is a new validated inverse
batch referring to the original change; it cannot erase a remote action. A later
conflict may prevent the inverse and require resolution. PR 4 must add exact intent
validation fixtures before shipping replay; this PR provides the wire contract.
For ordinary changes every replacement requires `before`, and `resolves` is
forbidden. `undo_of` is required exactly for undo and must reference an ancestor
change. For resolve, every replacement omits `before`, `resolves` is required,
and `undo_of` is forbidden. Sort bucket replacements by canonical bucket key and
forbid repeated buckets; sort `task_ids` and `resolves` uniquely by UTF-16 code
units. Schema regexes are structural only: validate actual calendar dates,
timestamps, Unicode scalar strings, and these cross-field rules before replay.

## Replay and conflicts

Deduplicate IDs. Replay only complete dependency closures; a missing ancestor or
activation backup leaves the record pending. Arrivals can be duplicated or shuffled.
Keep the last verified view while dependencies are missing, with a pending/error
indicator; do not present incomplete replay as up-to-date or import legacy again
to fill a hole. Pending edits remain durable. Independent complete branches may
be displayed; publication waits if any observed record is unsupported or invalid.

Causally ordered batches apply in ancestry order. Concurrent batches commute only
when their touched bucket sets are disjoint, or their full replacement sets and
before/after values are identical. Identical concurrent state changes coalesce in
the projection but retain separate action records (statistics must define repeated
intent counting). Concurrent differing replacements of any common bucket are a
conflict, even for different tasks. A multi-bucket batch conflicts as a whole.

Represent a conflict as the sorted maximal competing record IDs plus all connected
touched buckets. Preserve each branch's projected candidate and their common
ancestor state. Do not flatten candidates by record-ID order. Independent disjoint
batches still apply. A descendant editing a conflicted component stays on its
branch and extends that alternative; it does not select a winner. Readers show
the common state plus explicit alternatives until resolution. Where there is no
common imported state, show alternatives without inventing a common winner.

A resolve batch must causally include every currently known competing tip and
name them in `resolves`. It replaces the whole connected conflict component. For
resolve only, `before` is omitted because it has multiple candidate states. A
concurrent, previously unseen branch can reopen the conflict, as can conflicting
concurrent resolutions. Ordinary changes to unresolved components are rejected by
the UI; disjoint edits remain possible. All clients with the same complete records
must produce the same candidates, pending set, and resolved projection.

## Migration and the legacy bridge

Migration has no normal prompts or configuration changes. Preserve exact bytes
before rollover/pruning; durably publish backup, import, then activation; read back
and validate before enabling edits. Every step is retryable. Activation can arrive
first through sync; it must wait for its dependencies. Continue observing the
legacy anchor after activation, but never write a generated projection back to it.
Configuration/theme/retention values stay local and unchanged.

Equal offline imports coalesce by ID. Unequal imports have no reliable common
ancestor: compare candidates by task ID, retain identical buckets, and union disjoint
buckets only when global task IDs remain unique. Differing common buckets, or a
task present in different buckets, form an import conflict. Do not infer deletion
from absence. A resolved import conflict uses the same resolve protocol and keeps
every original snapshot. This can require genuine conflict resolution, although
ordinary equal-snapshot upgrades are invisible.

Every later observed legacy snapshot is backed up and imported. A transport can
prove a baseline only if it supplies a revision chain tying that write to the
earlier snapshot; locally remembering the last hash does NOT prove that a stale
device edited that version. Protocol 1 conservatively treats every unequal legacy
observation as another import candidate, not an automatically inferred deletion or
edit delta. Identical observed snapshots deduplicate. This settles the baseline
ambiguity safely without pretending folder sync provides causality.

Do not automatically read nearby files solely because their names resemble provider
conflict copies. A provider-confirmed association or explicit user selection is
required; selected copies enter the same snapshot-import path. Unknown unrelated
files remain untouched. Keep discovery of possible copies separate from importing.

The old binary cannot be fenced or made to read protocol records. Old clients see
only the legacy snapshot and can overwrite each other before observation. This is
NOT indefinite bidirectional mixed-version compatibility. Release notes require
upgrading devices for shared editing. New clients preserve observed legacy writes;
they cannot guarantee recovery of unobserved writes or identify a missing sidecar
as a fresh store on a device that has never seen it. Previously activated local
clients retain an activation witness and refuse silent fallback if the sidecar
disappears. A first-seen offline device can create another import; reconcile when
records arrive. Never claim it knows global migration or upload completion.

## Compatibility matrix

| Client | Legacy JSON only | Complete protocol 1 store | Partial/unknown protocol |
| --- | --- | --- | --- |
| Current TUI/CLI | Existing read/write behavior | Still edits legacy file; misses new changes | Unaware of sidecar |
| Current web | Existing revision-aware saves | Still edits legacy file; misses new changes | Unaware of sidecar |
| Next-major TUI/CLI | Automatic import, backup, activation | Replay, publish, observe legacy | Preserve edits; pending/read-only as appropriate |
| Compatible static web | Same migration via Dropbox, no new setup | Same protocol with durable browser outbox | Same safety rules; prompt login only if auth expires |

Unknown record schemas/kinds/fields cannot be ignored as harmless extensions.
Show the last verified view read-only, preserve pending operations, and request a
compatible application update. Invalid records likewise never cause destructive
fallback. Independent protocol versioning requires an explicit future migration.
The web deployment must not activate storage before the matching major CLI release;
cached old web clients have the same mixed-version limits as old binaries.

## Recovery, retention, and delivery boundary

Recover from immutable records and raw backups; export the latest resolved state
to an explicitly chosen legacy JSON destination for downgrade. Block a purported
complete export while unresolved conflicts remain, or explicitly export recovery
alternatives separately. Restoring only the migration backup loses later changes.

History records real actions from activation onward. Import timestamps cannot
supply completion history. No automatic physical compaction/purge is defined in
protocol 1. Existing retention continues to govern projected task visibility;
physical history retention is a release gate for PR 8, not a claim that hiding a
task erases its content. A physical purge requires a separately specified epoch/
stale-device protocol before implementation or release promises.

This PR ships documents, a structural schema, and language-neutral scenario
fixtures. The fixture checker validates structure and references, not a production
replay implementation. Go and JavaScript replay consumers in PR 4 must assert the
expected states under delivery permutations. No runtime, storage, release, or
conflict-policy behavior changes in this PR.
