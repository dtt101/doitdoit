# Project instructions

## Overview

`doitdoit` is a Go 1.27 terminal task manager built with Bubble Tea. It stores
tasks in a user-owned JSON file. The repository also contains an experimental,
fully static Dropbox web companion in `web/`.

These instructions apply to the whole repository.

## Toolchain and common commands

- Run `mise install` to install the Go version pinned in `mise.toml`.
- Run the Go test suite with `go test -count=1 ./...`.
- Run web tests with `node --test web/*.test.js`; the web app has no package
  manager, dependency installation, or build step.
- Before handing off a non-trivial Go change, also run `go vet ./...`. Run
  `go test -race ./...` for persistence, reload, concurrency, or release-related
  changes.
- Format changed Go files with `gofmt`. Run `go mod tidy` only when Go imports or
  dependencies change, and include the resulting `go.mod`/`go.sum` changes.
- Do not edit or commit the root `doitdoit` binary, `.cache/`, or `dist/`; they are
  local/generated artifacts ignored by Git.

## Repository map

- `main.go`: command dispatch, startup, and TUI construction.
- `cli/`: non-interactive commands such as `doitdoit add`.
- `config/`: configuration, first-run setup, storage moves, retention, and the
  optional Omarchy theme hook.
- `model/`: task lifecycle, persistence, reload/conflict handling, Bubble Tea
  state/update logic, and rendering.
- `styles/`: embedded themes and Omarchy theme resolution.
- `web/`: independent static companion; `domain.js` holds task behavior and
  `sync.js` holds Dropbox revision-aware I/O.
- `releasecheck/`: release licence and third-party inventory checks.
- `.github/workflows/`: the authoritative CI and release gates.

## Change guidelines

- Add or update tests beside the package or JavaScript module being changed.
  Prefer behavior-focused tests and temporary directories.
- Tests must never read or modify the developer's real task file or
  `~/.doitdoit_config.json`. Use `t.TempDir()` and isolate `HOME` with
  `t.Setenv()` when configuration is involved.
- Treat the task JSON format as a compatibility boundary. Date buckets use
  local-calendar `YYYY-MM-DD` keys; the undated bucket is exactly `Future`.
  Preserve existing JSON field names unless a migration and compatibility tests
  are part of the change.
- Data safety is core behavior. Do not weaken atomic replacement, `0600` file
  permissions, `.bak` creation, external-change detection, or the rule that a
  same-bucket concurrent edit remains a visible conflict.
- Task lifecycle behavior shared by the CLI and web companion—rollover,
  retention, distribution, and ordering—should stay aligned. Update both
  implementations and their tests when changing shared semantics, or document
  an intentional difference.
- Keep platform-specific behavior behind the existing build-tagged files and
  avoid introducing Unix-only assumptions into shared Go code.
- The web companion must remain static and self-contained. Do not add remote
  scripts/fonts or commit Dropbox secrets. Its app key is a public OAuth client
  ID; OAuth tokens remain browser-local.
- If dependencies, embedded themes, or distributed files change, update
  `THIRD_PARTY_NOTICES.md` as needed and run `go test -count=1 ./releasecheck`.

## Releases and installation

- Distribution is through GoReleaser archives on GitHub Releases. mise installs
  those archives via `github:dtt101/doitdoit`.
- There is no AUR package, `packaging/aur` tree, separate mise package, or
  application version file to maintain. Do not recreate AUR packaging unless
  the distribution strategy is explicitly changed.
- Keep the Go versions in `go.mod` and `mise.toml` aligned.
- A pushed annotated `v*` Git tag is the release version and triggers
  `.github/workflows/release.yml`. Do not create or push tags unless explicitly
  requested.
- When changing release behavior, keep `.goreleaser.yaml`, the workflows, and
  the release section of `README.md` consistent.
