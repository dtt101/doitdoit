# Project instructions

## Overview

`doitdoit` is a Go 1.27 terminal task manager built with Bubble Tea. It uses the
Cloudflare Worker + D1 task service backend across the Go TUI, non-interactive
CLI, MCP clients, and the static web companion in `web/`.

Supported desktop platforms are macOS and Linux only (`amd64` and `arm64`).
Omarchy is a first-class Linux platform: preserve automatic theme detection,
opt-in live theme updates, managed hook safety, and their isolated tests.
Windows builds and compatibility fallbacks are out of scope.

These instructions apply to the whole repository.

## Toolchain and common commands

- Run `mise install` to install the Go version pinned in `mise.toml`.
- Run the Go test suite with `go test -count=1 ./...`.
- Run web tests with `node --test web/*.test.js`; the web app has no package
  manager, dependency installation, or build step.
- For the Cloudflare service, use Rust 1.93+ and Node 24+.
  Run one-time `make setup` in `service/`, then `make check` and `make bundle`
  (a deployment dry-run). `make check-rust` runs Cargo checks alone. All service
  application code, including OAuth and MCP, is Rust. Node is used only for
  Cloudflare development tooling and local workerd/D1 integration tests with
  mocked GitHub, never real tasks or accounts.
- Before handing off a non-trivial Go change, also run `go vet ./...`. Run
  `go test -race ./...` for persistence, reload, concurrency, or release-related
  changes.
- Format changed Go files with `gofmt`. Run `go mod tidy` only when Go imports or
  dependencies change, and include the resulting `go.mod`/`go.sum` changes.
- Do not edit or commit the root `doitdoit` binary, `.cache/`, or `dist/`; they are
  local/generated artifacts ignored by Git.

## Repository map

- `main.go`: command dispatch, startup, and service TUI launch.
- `service_tui.go`: service TUI startup and lock/credential wiring.
- `cli/`: non-interactive commands (`doitdoit add`, `doitdoit service ...`).
- `config/`: configuration (`service_origin`, `theme`), and the optional Omarchy
  theme hook.
- `model/`: Bubble Tea state/update logic, rendering, notes, and service command
  orchestration (`model/service.go`).
- `styles/`: embedded themes and Omarchy theme resolution.
- `web/`: independent static companion; `domain.js` holds task behavior,
  `sync.js` holds snapshot reads and retained revision-guarded commands, and
  `auth.js` owns browser OAuth.
- `service/`: Rust/WASM Cloudflare task service and D1 migrations, including
  Rust OAuth/MCP endpoints and local integration tests.
- `api/`: service contract and shared Go/Rust fixtures.
- `taskclient/`: HTTP task operations, desktop OAuth, private credentials and
  retained commands.
- `tasktransfer/`: strict explicit portable-export import planning and field
  verification.
- `devtools/service-smoke/`: browser-login and HTTP/MCP smoke test client
  for the Rust service; invoked by `make smoke` in `service/`.
- `releasecheck/`: release licence and third-party inventory checks.
- `.github/workflows/`: the authoritative CI and release gates.

## Change guidelines

- The task service uses a Cloudflare Worker task API/MCP service, D1, and the
  Go TUI and static web app as clients. Follow `service/README.md` and
  `api/README.md` for its architecture and contract.
- Add or update tests beside the package or JavaScript module being changed.
  Prefer behavior-focused tests and temporary directories.
- Tests must never read or modify the developer's real credentials or
  `~/.doitdoit_config.json`. Use `t.TempDir()` and isolate `HOME` and
  `XDG_CONFIG_HOME` with `t.Setenv()` when configuration or credentials are
  involved.
- Data safety is core behavior. Retain private commands with `0600` permissions
  and `operation_id` before sending mutations, enforce `expected_revision`
  checks, and keep conflicting edits visible for explicit review.
- Task lifecycle behavior in the TUI, CLI, and web companion must agree:
  no automatic rollover or pruning, original overdue dates, and service ordering.
- Shared Go code may rely on Unix behavior available on both Linux and macOS.
  Keep OS-specific behavior behind build-tagged files; do not add Windows
  compatibility shims. CI and release gates must cover both supported systems.
- The web companion must remain static and self-contained. Do not add remote
  scripts/fonts or commit credentials. Browser OAuth uses public-client PKCE;
  tokens and retained commands remain browser-local.
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
