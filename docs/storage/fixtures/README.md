# Shared protocol examples

`scenarios.json` is language-neutral test input for the future Go and JavaScript
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

These checks run under the existing web CI command and introduce no dependencies.
They do not implement replay or prove convergence. PR 4 must consume these expected
results in both languages and add shuffled/duplicate delivery, batch conflicts,
resolution, identity repair, invalid calendar values, and intent-validation cases.

`creation-timestamps.json` contains `{value, valid}` vectors for standalone task
creation timestamps. Go `Parse` and `Queue` consume them; the JavaScript fixture
oracle checks calendar and format rules without normalizing the original string.
Stage 4 must also consume these vectors in the production JavaScript validator.
Valid legacy spelling, offsets (including negative zero), and fractional precision
remain unchanged in hashed records. Invalid calendar dates, zone components,
one-digit hours, comma fractions, and trailing input are rejected.
