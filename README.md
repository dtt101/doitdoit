# doitdoit

**A fast, keyboard-first task manager for your terminal — synced across your terminal, phone, and MCP assistants.**

`doitdoit` gives you a calm, multi-day view of what is in front of you, gets out of the way when you are working, and keeps unfinished tasks visible in Overdue with their original dates. Tasks live in the [Cloudflare task service](#cloudflare-task-service) shared by the TUI, CLI, static web companion, and MCP clients.

Supported desktop platforms are **macOS and Linux**, with **Omarchy as a first-class Linux platform**. Windows is no longer supported. Release archives cover Intel/AMD 64-bit (`amd64`) and ARM64 (`arm64`) on both supported operating systems.

<img width="1467" height="799" alt="Screenshot 2026-08-20 at 16 50 28" src="https://github.com/user-attachments/assets/e374314c-424e-460c-ae8e-fa3ee19d9dd5" />

## Why doitdoit?

- **See the days ahead.** Work from a clean, scrolling multi-day view instead of a long, undifferentiated list. Every day has its own column.
- **One shared task service.** Terminal, mobile web companion, and MCP assistants all work against the same revision-guarded task API, with explicit portable export and import.
- **Never lose an unfinished task.** Unfinished past tasks keep their original dates and stay visible in Overdue, with full history retained and recoverable soft deletes.
- **Plan quickly from the keyboard.** Add, edit, complete, delete, copy, reorder, schedule, repeat a move, and undo without leaving the terminal.
- **Capture now, decide later.** Drop ideas into Future, then schedule them for tomorrow, the next seven days, or an exact date when you are ready.
- **Looks at home on Omarchy.** `doitdoit` follows your active Omarchy theme and includes every stock Omarchy 4 (Quattro) palette.
- **Take it to your phone.** The static [web companion](./web) connects to the same service backend.

## Omarchy

`doitdoit` is built to feel like part of an [Omarchy](https://omarchy.org) desktop, not a generic TUI dropped into it.

With no extra configuration, it reads the currently active Omarchy 4 (Quattro) theme — including user-installed themes — from `~/.local/state/omarchy/current/theme/colors.toml`. It also bundles all 22 stock Quattro palettes, so they are available on macOS and other Linux distributions too.

### Install on Omarchy

Omarchy 4 (Quattro) already includes mise. Install and activate `doitdoit` directly from its GitHub releases with:

```bash
mise use -g github:dtt101/doitdoit
doitdoit
```

The `github:` prefix selects mise's GitHub backend, which chooses the appropriate prebuilt release for the current platform. Because the tool is in your global mise configuration, `omarchy update` updates it with the rest of your mise-managed tools. You can also update it directly with `mise up github:dtt101/doitdoit`.

Theme-change integration is explicitly opt-in. Install the managed hook and any running `doitdoit` instances will retint as soon as you switch your Omarchy theme:

```bash
doitdoit config omarchy-hook install
doitdoit config omarchy-hook status
```

The managed hook contains:

```bash
#!/bin/sh
pkill -SIGUSR2 -x doitdoit
```

Remove the integration with `doitdoit config omarchy-hook remove`. Installation refuses to overwrite an existing modified file, directory, or symlink, and removal deletes only the exact managed hook. Without the hook, `doitdoit` simply picks up the new theme on its next launch.

To remove the mise-managed installation while preserving your tasks and configuration:

```bash
doitdoit config omarchy-hook remove
mise unuse -g github:dtt101/doitdoit
```

## Install

### macOS

Install mise with Homebrew if you do not already have it:

```bash
brew install mise
```

Add mise to your default zsh shell by placing this line in `~/.zshrc`, then open a new terminal:

```bash
eval "$(mise activate zsh)"
```

Install and run `doitdoit`:

```bash
mise use -g github:dtt101/doitdoit
doitdoit
```

mise automatically selects the Apple Silicon or Intel macOS archive for your Mac.

### Other Linux distributions

Once [mise is installed and activated](https://mise.jdx.dev/getting-started.html), use the same command on Linux:

```bash
mise use -g github:dtt101/doitdoit
doitdoit
```

Sign in once, then launch `doitdoit`:

```bash
doitdoit service login
doitdoit
```

Start adding tasks with `a`, move with `m`, complete with `Space`, open Future with `f`, Overdue with `o`, and Deleted with `x`.

You can also download a prebuilt archive from [GitHub Releases](https://github.com/dtt101/doitdoit/releases), install with Go 1.27.3 or later, or build from source. The repository's `mise.toml` pins the development toolchain:

```bash
go install github.com/dtt101/doitdoit@latest

git clone https://github.com/dtt101/doitdoit.git
cd doitdoit
mise install
go build -o doitdoit
./doitdoit
```

## Daily workflow

The default view shows up to three days, with one day per column. Narrow windows
show fewer columns, keeping the selected day on screen. Move right to see later
days and left to return toward Today.

Press `t` to jump back to Today, including from Future.

Long lists scroll inside their columns, keeping the selected task, day header,
and footer visible. Use `j`/`k` or Up/Down to select tasks, and Page Up/Page Down
to scroll through a list or a task title taller than the window. Task and date
inputs fit their column and preserve typed text and cursor position on resize.
Windows smaller than 24 columns by 10 rows show a resize prompt and pause task
shortcuts until there is room to display them.

Day headers show how many tasks remain. A `>` marks the selected task, `[ ]`
marks an incomplete task, and `[x]` marks a completed task. Wrapped lines keep
the selection marker visible. Completed tasks appear in a separate section;
press `c` to collapse or expand it across the current session. Collapsing only
changes the display and navigation, preserving the task file and stored order.

Press `f` for the separate Future list, `o` for Overdue, and `x` for Deleted. Completed tasks appear in a **Completed**
section. `J`/`K` reorder within the displayed section.
To set or change a date, select a task, press `m` then `d`, enter `YYYY-MM-DD`
or `MM-DD`, and press Enter. `MM-DD` uses the current year. Chosen dates are preserved as entered.

Setting a date immediately moves a task out of Future into that date's calendar
bucket, however far away. Moving a task to Future clears its date. Navigation
and resizing only change the display; they never move or save tasks.

### Keybindings

The browsing footer shows the everyday shortcuts: `a` Add, `Space` Complete,
`m` Move (Move/date in Future), `f` Future (or Days when viewing Future), and
`?` Help. Empty columns explain how to add a task to the selected day or Future.

Press `?` while browsing days or Future to open the full keyboard-shortcuts
modal; press `?` again or `Esc` to close it. In short windows, scroll the help panel with Up/Down
or Page Up/Page Down. After pressing `m`, the available destinations appear
directly in the footer. Saved moves and deletions show a confirmation with `u`
to undo. The confirmation stays available while navigating and clears when
another task change replaces the undo history or an external reload invalidates
it. Save errors appear instead of a success confirmation.

| Key | Action |
| --- | --- |
| `h` `j` `k` `l` or arrows | Move between days and tasks |
| `Page Up` / `Page Down` | Scroll a task list, including long wrapped titles |
| `t` | Return to Today in the current layout |
| `a` | Add a task to the selected day or Future |
| `e` | Edit the selected task title |
| `n` | Open plain-text notes for the selected task |
| `Space` or `Enter` | Toggle completion |
| `m` | Move or schedule the selected task |
| `m` then `d` | Set or change the selected task’s date |
| `J` / `K` | Reorder the selected task |
| `.` | Repeat the last move destination |
| `u` | Undo the most recent task change |
| `c` | Collapse or expand completed tasks (session only) |
| `y` | Copy the task text to the clipboard |
| `d` | Move the selected task to Deleted |
| `f` | Toggle the Future view |
| `o` | Toggle the Overdue view |
| `x` | Toggle the Deleted view (`Enter` restores) |
| `?` | Open keyboard shortcuts |
| `q` or `Ctrl+c` | Quit |
| `r` | Refresh / retry retained save |

Press `n` on a highlighted task to edit its notes. Enter inserts a new line;
`Esc` closes the editor and automatically saves your changes. If saving fails,
the editor stays open with your notes intact. Press `Esc` to retry; after closing,
`u` undoes the notes change. Tasks with notes show a
small `▤` symbol beside the title. `Tab` switches between editing and a link view, where
HTTP/HTTPS URLs are clickable using your terminal's usual link gesture (often
Ctrl-click or Cmd-click). Link activation requires a terminal with OSC 8 support.
Scroll the link view with arrows or Page Up/Page Down.

Inside notes, use your terminal's usual text selection, copy, and paste shortcuts.
Pasted text is inserted at the cursor, preserving line breaks. Notes are plain
text, without Markdown. They are stored on the task in the service and follow
it through moves and date changes. The web companion opens notes in a scrollable viewer via the note icon and preserves them
when editing or syncing tasks. Notes editing is available in the TUI.

After pressing `m`, choose a destination:

| Key | Destination |
| --- | --- |
| `t` | Today |
| `f` | Future, without a date |
| `1`–`7` | That many calendar days from the task's current date; from Today for Future tasks |
| `d` | An exact date entered as `YYYY-MM-DD` or `MM-DD` |
| `Esc` | Cancel |

Focus stays in the source list after a move, so clearing and scheduling a batch of tasks stays fast.

## Themes

On Omarchy, the default `system` setting follows the active stock or custom theme. Everywhere else, `system` uses an adaptive built-in palette, or you can select any bundled Quattro theme explicitly:

```bash
doitdoit config theme
doitdoit config theme tokyo-night
doitdoit config theme system
```

Bundled themes:

`catppuccin`, `catppuccin-latte`, `ethereal`, `everforest`, `flexoki-light`, `gruvbox`, `hackerman`, `kanagawa`, `last-horizon`, `lumon`, `lupine`, `matte-black`, `miasma`, `nord`, `osaka-jade`, `retro-82`, `ristretto`, `rose-pine`, `solitude`, `tokyo-night`, `vantablack`, `white`.

`doitdoit` renders on your terminal's background rather than painting over it. On Omarchy, the terminal and app change together. On other systems, choose a palette that fits your terminal; the light options are `catppuccin-latte`, `flexoki-light`, `lupine`, `rose-pine`, and `white`.

## Command line

```text
doitdoit                         Launch the TUI
doitdoit --service <origin>      Use a specific task service for this session
doitdoit add <title>             Add a task to Today without opening the TUI
doitdoit add --when <target> <title>
                                 Target today, tomorrow, future, or YYYY-MM-DD
doitdoit add --service <origin> <title>
                                 Capture into a specific task service
doitdoit config show             Show the service origin and theme
doitdoit config theme            Show the current and available themes
doitdoit config theme <name>     Select a theme
doitdoit config omarchy-hook install|status|remove
doitdoit service login|status|logout [--url <origin>]
doitdoit service export --out <file> [--url <origin>]
doitdoit service import --file <file> [--dry-run | --apply]
doitdoit service retry --file <retained-command>
doitdoit service history --id <id>
doitdoit service restore --id <id> --revision <reviewed-revision>
```

For example, `doitdoit add --when tomorrow "send the invoice"` is suitable for shell aliases, launchers, and scripts. Titles can be quoted or supplied as multiple arguments. Put flags before the title.

Use `--notes` to include plain-text notes when creating a task:

```bash
doitdoit add --when tomorrow --notes "Include September expenses" "send the invoice"
doitdoit add --notes $'First line\nSecond line' "task with multiline notes"
```

Notes preserve whitespace and line breaks and use the same plain-text `notes`
field as the TUI. The multiline example uses Bash's `$'…'` quoting.

## Mobile companion

The [`web/`](./web) directory contains an installable static web app for adding,
editing, scheduling, reordering and completing shared service tasks from a phone.
Sign in with GitHub; it uses the same backend as the TUI and MCP.

See [web/README.md](./web/README.md) for setup, recovery and deployment.

## Development

```bash
mise install
go test ./...
go run .
```

Use `go test -count=1 ./...` to bypass the test cache or `go test -cover ./...` for a coverage summary. Run `go vet ./...` for non-trivial Go changes and `go test -race ./...` for persistence, reload, or concurrency changes. Test the static web companion with `node --test web/*.test.js`; it needs no build step or dependency installation.

CI and release gates run on Linux and macOS. The Go suite includes isolated Omarchy theme detection, first-run setup, managed hook installation/removal, and theme rendering tests. These fixtures protect Omarchy integration without changing the runner's desktop configuration; they do not replace a live Omarchy theme-switch smoke test before release.

### Publish a release

mise installs from published GitHub Releases; it does not build or install the current `main` branch. There is no separate mise package to publish, and there is currently no application version file to edit—the Git tag is the release version.

After the changes for a release are merged and CI is green, choose the next [semantic version](https://semver.org/) and create an annotated tag from the exact commit you want to ship. Replace `vX.Y.Z` below with the new, unused version:

```bash
git switch main
git pull --ff-only
git tag -a vX.Y.Z -m "doitdoit vX.Y.Z"
git push origin vX.Y.Z
```

Pushing a `v*` tag starts [the release workflow](./.github/workflows/release.yml). It reruns the Go and web tests, race detector, vet, and vulnerability scan on Linux and macOS; GoReleaser then builds Linux and macOS `.tar.gz` archives for `amd64` and `arm64`, the checksum file, and the GitHub Release. Windows archives are no longer produced. Confirm both the workflow and the new entry on [GitHub Releases](https://github.com/dtt101/doitdoit/releases) succeeded before announcing the version.

New installations using `mise use -g github:dtt101/doitdoit` resolve the latest published release. Existing installations stay on their installed version until the user runs:

```bash
mise up github:dtt101/doitdoit
```

On Omarchy, `omarchy update` also updates globally configured mise tools. Merely pushing commits to `main` does not update either new or existing mise users.

## Licence

This software is licensed under the [MIT License](./LICENSE). Third-party attributions and licence terms are in [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).

Uninstalling intentionally leaves `~/.doitdoit_config.json` and your local service credentials in place; remove them manually if you no longer need them.

### Cloudflare task service

The TUI, web companion, and assistant MCP clients share tasks through a Rust/WASM Cloudflare Worker backed by D1.

From this checkout:

```sh
go run . service login
go run .
```

See the [service guide](service/README.md) for TUI controls, ChatGPT/Gemini setup
and reviewed import/export; [Operations](service/OPERATIONS.md) for encrypted
backups, deployment and recovery; and the [API contract](api/README.md) for protocol
details.
