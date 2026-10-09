# doitdoit

**A fast, keyboard-first task manager for your terminal — with your tasks kept in a file you own.**

`doitdoit` gives you a calm, multi-day view of what is in front of you, gets out of the way when you are working, and makes sure unfinished tasks do not disappear into yesterday. There is no account, database, or hosted backend: the entire backend is one readable JSON file.

Supported desktop platforms are **macOS and Linux**, with **Omarchy as a first-class Linux platform**. Windows is no longer supported. Release archives cover Intel/AMD 64-bit (`amd64`) and ARM64 (`arm64`) on both supported operating systems.

Put that file in Dropbox, iCloud Drive, or Google Drive and your task list can travel with you.

<img width="1467" height="799" alt="Screenshot 2026-08-20 at 16 50 28" src="https://github.com/user-attachments/assets/e374314c-424e-460c-ae8e-fa3ee19d9dd5" />

## Why doitdoit?

- **See the days ahead.** Work from a clean, scrolling multi-day view instead of a long, undifferentiated list. Every day has its own column.
- **Keep your data yours.** Everything lives in a single portable, human-readable JSON file. Back it up, inspect it, script against it, or sync it with the service you already use.
- **Never lose an unfinished task.** Anything incomplete automatically rolls forward to Today. You choose whether completed history is kept forever or pruned after a positive number of days.
- **Plan quickly from the keyboard.** Add, edit, complete, delete, copy, reorder, schedule, repeat a move, and undo without leaving the terminal.
- **Capture now, decide later.** Drop ideas into Future, then schedule them for tomorrow, the next seven days, or an exact date when you are ready.
- **Looks at home on Omarchy.** `doitdoit` follows your active Omarchy theme and includes every stock Omarchy 4 (Quattro) palette.
- **Take it to your phone.** The optional static [web companion](./web) can read and write the same Dropbox-hosted task file.

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

On first launch, choose where `doitdoit.json` should live and how long completed history should be retained. The retention prompt defaults to keeping history forever, including when input reaches EOF. Accept a normal local path, or point it straight at a synced folder:

```text
~/Dropbox/doitdoit/doitdoit.json
```

On Omarchy, setup also offers live theme updates when following the system
theme. Press Enter to leave them disabled, or answer `yes` to install the
managed hook. Existing custom hooks are preserved.

Start adding tasks with `a`, move with `m`, complete with `Space`, and open Future with `f`.

You can also download a prebuilt archive from [GitHub Releases](https://github.com/dtt101/doitdoit/releases), install with Go 1.27 or later, or build from source. The repository's `mise.toml` pins the development toolchain:

```bash
go install github.com/dtt101/doitdoit@latest

git clone https://github.com/dtt101/doitdoit.git
cd doitdoit
mise install
go build -o doitdoit
./doitdoit
```

## One file is the backend

There is no server to maintain and no proprietary data store. Task data is saved atomically to one `doitdoit.json` file with owner-only permissions, while machine-specific settings such as the file location and theme stay separately in `~/.doitdoit_config.json`.

The terminal application has no ads or telemetry and sends no task data to the maintainer. It accesses the network only indirectly when you deliberately place its JSON file in a third-party synced folder; that provider's terms then apply.

This makes a few useful workflows almost effortless:

- Put the data file in a Dropbox, iCloud Drive, or Google Drive folder to sync it between machines.
- Keep different machine-specific themes while sharing exactly the same tasks.
- Back up or version the file using ordinary file tools.
- Create tasks from another script or tool using a simple, open JSON format.
- Use `-file` to open a different task list for a project or a one-off session.

The TUI checks for external changes every few seconds and reloads them without disturbing you while you are typing. Before every save it verifies that the file still matches the version it loaded. Independent date-list changes can merge; competing changes within a list require review. Each replacement keeps the previous valid contents in the task file's `.bak` sibling.

### Recover tasks from Dropbox copies

On startup, the terminal checks for Dropbox “conflicted copy” files beside your configured task file, including conflicted `.bak` files. It leaves ordinary `.bak` files, historical backups, and subdirectories alone. Copies whose tasks already match the current file are silently deleted; additional tasks in the current file are kept. Copies with missing or duplicate task IDs are never automatically deleted.

Saved copies produce an optional recovery notice; they do not open a review on startup or block normal rollover and retention. Press **`r`** to recover tasks or rescan. The compact panel shows one unfinished task at a time and defaults to keeping your current file. Old copies can contain tasks you deliberately deleted or changed later: absence alone cannot distinguish a deletion from an unsynced addition. Completed tasks on either side are excluded, including old unfinished versions of tasks now marked done. Copies containing only excluded differences are preserved without prompting. Recover from one copy at a time:

1. Use **arrows / `j` / `k`** to select a task. The screen offers task actions: **Delete / Restore**, **Keep as is / Update task**, or **Keep task / Delete task**. Press **`1`** or **`2`** to choose. Enter toggles the choice. Restoring a task is always an explicit choice.
2. Choose ordering separately when lists differ. Moves are shown as one task decision. Ambiguous IDs require choosing connected whole lists. List and ordering choices containing completed work are excluded from saved-copy recovery. The app will not guess task identity from titles or introduce duplicate IDs.
3. **`K`** explicitly keeps the current version for every difference. **`p`** summarizes what will be added, updated, or deleted, including any rollover or retention effects. Press **`v`** to see unfinished tasks after saving. **Page Up / Page Down** scroll long notes and lists.
4. **Enter — Save & finish** applies the choices to your main task list and removes the resolved copies in the background. **Esc** returns to choices, then defers review without changing anything. If a file changes during review, the app requires fresh choices.

Task timestamps record creation, not the last edit. They never automatically decide which status is newer. Selecting a changed task chooses its complete version, including notes, completion, and date.

Before removing a reviewed, nonredundant copy, the app exports both alternatives to a local, owner-only recovery directory. Press **`?`** for additional controls, including **`x`** to export explicitly; the screen shows the export path. Recovery lives under `os.UserConfigDir()/doitdoit/recovery/`, in a directory scoped to the resolved task-file path (normally `~/.config` on Linux and `~/Library/Application Support` on macOS). Exports remain there until you remove them; regular synced backups remain separate.

### Recover an unsaved terminal edit

A failed save pauses editing and automatic writes and displays **“Your changes need review before they can be saved.”** Press **`r`** to review the suggested task changes; **`b`** shows the original values. Noncompeting changes are proposed in the preview, but conflicting decisions require your choice. This does not change the conservative automatic merge policy.

The draft and its baseline are saved in local recovery storage and reopened on the next launch. **`q`** safely quits when recovery succeeds. If recovery storage fails, normal quit is blocked: use **`x`** to export/retry recovery, or **`D`**, then **`y`**, to explicitly discard the draft. Cancelling review preserves pending edits. Recovery is cleared only after successful resolution or explicit discard.

This review is available in the terminal; the experimental web app retains its download-recovery/reload workflow, and noninteractive CLI commands do not open a resolver. Folder sync still has a narrow race between verification and replacement or cleanup. Review can recover versions it has observed; it cannot guarantee Dropbox has uploaded a local save or recover an unseen offline version. Let Dropbox finish syncing when switching machines.

Back up both the task JSON file and `~/.doitdoit_config.json` before migrations or major upgrades, and test that the backup can be read. Need to relocate an existing task file later? `doitdoit config move <new_path>` moves it and updates your configuration, but refuses any existing file, directory, or symlink at the destination so it cannot overwrite data.

Completed history is preserved forever by default. Choose a positive pruning period during first-run setup or change it later with `doitdoit config retention <days>`. Pruning never occurs until that choice has been saved. `doitdoit config retention forever` returns to non-pruning mode.

## Daily workflow

The default view shows up to three days, with one day per column. Narrow windows
show fewer columns, keeping the selected day on screen. Move right to see later
days and left to return toward Today.

Press `t` to jump back to Today. Press `T` to focus on Today alone; press it again
to restore the responsive multi-day layout at Today. Focus mode stays on Today
across midnight. Both shortcuts work from Future too.

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

When unfinished tasks carry forward, a session notice reports how many moved
to Today. It appears in the footer when space allows; `?` also explains rollover.
The count reflects rollover observed during this session and is never stored.

Press `f` for the separate Future list. Completed tasks appear in a **Completed**
section. `J`/`K` reorder within the displayed section.
To set or change a date, select a task, press `m` then `d`, enter `YYYY-MM-DD`
or `MM-DD`, and press Enter. `MM-DD` uses the current year; past dates move
the task to Today.

Setting a date immediately moves a task out of Future into that date's calendar
bucket, however far away. Moving a task to Future clears its date. Navigation,
resizing, and Today focus only change the display; they never move or save tasks.

On load, legacy tasks in Future with valid dates move to their recorded date
buckets before normal rollover and retention run. Overdue incomplete tasks roll
to Today; completed tasks keep their historical dates, subject to retention.
The migration uses the normal revision-checked save and backup. Malformed dates
stay untouched in Future under **Invalid dates** so you can correct them.

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
| `T` | Toggle Today-only focus; return to Today |
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
| `d` | Delete the selected task |
| `f` | Toggle the Future view |
| `?` | Open keyboard shortcuts |
| `q` or `Ctrl+c` | Quit |
| `r` | Review conflicts / rescan Dropbox copies |

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
text, without Markdown. They are saved in the optional JSON `notes` field and
follow the task through moves and rollover. The web companion opens notes in a scrollable viewer via the note icon and preserves them
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
doitdoit -file <path>            Use a different data file for this session
doitdoit add <title>             Add a task to Today without opening the TUI
doitdoit add --when <target> <title>
                                 Target today, tomorrow, future, or YYYY-MM-DD
doitdoit add --file <path> <title>
                                 Capture into a one-off task file
doitdoit config show             Show the data file, theme, and retention
doitdoit config move <new_path>  Move the data file and update the config
doitdoit config theme            Show the current and available themes
doitdoit config theme <name>     Select a theme
doitdoit config retention        Show the completed-history retention
doitdoit config retention forever
doitdoit config retention <days> Set a positive retention period
doitdoit config omarchy-hook install|status|remove
```

For example, `doitdoit add --when tomorrow "send the invoice"` is suitable for shell aliases, launchers, and scripts. Titles can be quoted or supplied as multiple arguments. Put flags before the title.

Use `--notes` to include plain-text notes when creating a task:

```bash
doitdoit add --when tomorrow --notes "Include September expenses" "send the invoice"
doitdoit add --notes $'First line\nSecond line' "task with multiline notes"
```

Notes preserve whitespace and line breaks and use the same optional JSON `notes`
field as the TUI. The multiline example uses Bash's `$'…'` quoting.

## Mobile companion

The [`web/`](./web) directory contains an experimental installable web app for adding, editing, scheduling, reordering, and completing tasks from a phone. It connects directly to the same JSON file through Dropbox. It is outside the CLI release and its security/privacy assurance; review the browser security and conflict-handling notes in its README before using it with real data.

See [web/README.md](./web/README.md) for Dropbox setup and deployment instructions.

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

Uninstalling intentionally leaves your task JSON file and `~/.doitdoit_config.json` in place; back them up and remove them manually only if you no longer want the data.
