# Shared protocol examples

`scenarios.json` is language-neutral test input consumed by both Go and JavaScript
replayers. It is not an on-disk store. `$name` is a fixture record reference and
`$raw:name` references UTF-8 bytes in `raw`. Expand dependencies recursively,
canonicalize each body per ADR 0001, and use its SHA-256 as the reference. Production
records must contain actual hashes. Alias substitution is limited to fixture
references; application task text must never undergo substitution.
Sort `parents` and `resolves` after alias expansion, before hashing the containing
record, because fixture alias order need not equal hash order.

Each scenario starts with an empty local store. All raw backups are available
unless `withhold_raw` says otherwise. Deliver the listed records in order, then
assert `expected`. When present, `then` delivers additional records/backups and
asserts its own result. Imports without a complete activation are not active task
state. `snapshot: null` means there is no single complete, conflict-free snapshot;
it does not mean tasks were deleted. `conflicts.alternatives` lists the maximal
state-bearing tips, omitting activation wrappers. Lists of pending IDs and
alternatives are compared as sets after alias expansion/deduplication.

`completion_events` counts explicit complete actions, never imports. It does not
define future statistics for concurrent redundant actions. `unique_imports` counts
content-distinct import records, not device observations.

Run structural/reference/canonical-byte checks with:

```
node --test web/storage-protocol.test.js
```

These structural checks run under the existing web CI command without dependencies.
`recordstore/replay_test.go` and `web/record-replay.test.js` additionally assert the
expected states under shuffled/duplicate delivery. The Go suite invokes a Node
bridge (when Node is available) to compare complete views across languages,
including candidate snapshots, common state, heads, issues, and activity counts.

Expected objects assert selected fields; `snapshot` and `common` are exact maps.
`pending`, `heads`, and `alternatives` use fixture names compared as hash-sorted
sets. Candidate `tip` and issue `id` use `$name` references. Candidate snapshots
contain only their component's buckets; combine them with the independent buckets
in the view when presenting alternatives. Cases with `issues` intentionally carry
structurally valid but semantically invalid records (for example mismatched backups
or a completion action that also renames a task).

`validation.json` supplies shared canonicalization, standalone record, and pure
legacy-normalization vectors, including deterministic missing/duplicate ID repair.
It does not perform migration, discovery, or file writes.

`creation-timestamps.json` contains `{value, valid}` vectors for standalone task
creation timestamps. Go `Parse` and `Queue` consume them; the JavaScript fixture
oracle checks calendar and format rules without normalizing the original string.
The production JavaScript validator also consumes these vectors in its tests.
Valid legacy spelling, offsets (including negative zero), and fractional precision
remain unchanged in hashed records. Invalid calendar dates, zone components,
one-digit hours, comma fractions, and trailing input are rejected.
