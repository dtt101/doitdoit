# doitdoit

**A fast, keyboard-first task manager for your terminal — with your tasks kept in a file you own.**

`doitdoit` gives you a calm, multi-day view of what is in front of you, gets out of the way when you are working, and makes sure unfinished tasks do not disappear into yesterday. There is no account, database, or hosted backend: the entire backend is one readable JSON file.

Put that file in Dropbox, iCloud Drive, or Google Drive and your task list can travel with you.

<img width="1467" height="799" alt="Screenshot 2026-08-20 at 16 50 28" src="https://github.com/user-attachments/assets/e374314c-424e-460c-ae8e-fa3ee19d9dd5" />

## Why doitdoit?

- **See the days ahead.** Work from a clean, scrolling multi-day view instead of a long, undifferentiated list. Weekends are intelligently stacked to save space.
- **Keep your data yours.** Everything lives in a single portable, human-readable JSON file. Back it up, inspect it, script against it, or sync it with the service you already use.
- **Never lose an unfinished task.** Anything incomplete automatically rolls forward to Today. You choose whether completed history is kept forever or pruned after a positive number of days.
- **Plan quickly from the keyboard.** Add, complete, delete, copy, reorder, schedule, repeat a move, and undo without leaving the terminal.
- **Capture now, decide later.** Drop ideas into Future, then schedule them for tomorrow, the next seven days, or an exact date when you are ready.
- **Looks at home on Omarchy.** `doitdoit` follows your active Omarchy theme and includes every stock Omarchy 4 (Quattro) palette.
- **Take it to your phone.** The optional static [web companion](./web) can read and write the same Dropbox-hosted task file.

## Especially nice on Omarchy

`doitdoit` is built to feel like part of an [Omarchy](https://omarchy.org) desktop, not a generic TUI dropped into it.

With no extra configuration, it reads the currently active Omarchy 4 (Quattro) theme — including user-installed themes — from `~/.local/state/omarchy/current/theme/colors.toml`. It also bundles all 22 stock Quattro palettes, so they are available on macOS, other Linux distributions, and Windows too.

After the v0.2.0 package is published, install the prebuilt static binary from the AUR on Omarchy with:

```bash
omarchy pkg aur add doitdoit-bin
```

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

`doitdoit` is an independent project and is not affiliated with or endorsed by Omarchy, 37signals, or their creators.

## Try it

Requires [Go](https://go.dev/dl/) 1.24 or later.

```bash
go install github.com/dtt101/doitdoit@latest
doitdoit
```

On first launch, choose where `doitdoit.json` should live and how long completed history should be retained. The retention prompt defaults to keeping history forever, including when input reaches EOF. Accept a normal local path, or point it straight at a synced folder:

```text
~/Dropbox/doitdoit/doitdoit.json
```

That is the whole setup. Start adding tasks with `a`, move with `m`, complete with `Space`, and open Future with `f`.

> `go install` places the binary in `$GOPATH/bin` (usually `~/go/bin`). Make sure that directory is on your `PATH`.

You can also download a prebuilt archive from [GitHub Releases](https://github.com/dtt101/doitdoit/releases), or build from source:

```bash
git clone https://github.com/dtt101/doitdoit.git
cd doitdoit
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

The TUI checks for external changes every few seconds and reloads them without disturbing you while you are typing. Like most file-sync workflows, simultaneous edits are last-write-wins, so avoid changing the same file from two devices at exactly the same moment.

Back up both the task JSON file and `~/.doitdoit_config.json` before migrations or major upgrades, and test that the backup can be read. Need to relocate an existing task file later? `doitdoit config move <new_path>` moves it and updates your configuration, but refuses any existing file, directory, or symlink at the destination so it cannot overwrite data.

Completed history is preserved forever by default. Choose a positive pruning period during first-run setup or change it later with `doitdoit config retention <days>`. Pruning never occurs until that choice has been saved. `doitdoit config retention forever` returns to non-pruning mode.

## Daily workflow

The default view shows Today and the days immediately ahead. Move right beyond the final column and the calendar keeps scrolling forward; move left to return toward Today. Saturday and Sunday share a compact weekend column whenever multiple days are visible.

Press `f` for the separate Future list. Tasks with a specific future date remain there until that date enters the visible window, while undated ideas wait until you decide what to do with them.

### Keybindings

Press `?` in the main view to open the keyboard-shortcuts modal; press `?` again or `Esc` to close it. After pressing `m`, the available destinations appear directly in the footer.

| Key | Action |
| --- | --- |
| `h` `j` `k` `l` or arrows | Move between days and tasks |
| `a` | Add a task to the selected day or Future |
| `Space` or `Enter` | Toggle completion |
| `m` | Move or schedule the selected task |
| `J` / `K` | Reorder the selected task |
| `.` | Repeat the last move destination |
| `u` | Undo the most recent move or reorder |
| `y` | Copy the task text to the clipboard |
| `d` | Delete the selected task |
| `f` | Toggle the Future view |
| `?` | Open keyboard shortcuts |
| `q` or `Ctrl+c` | Quit |

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
doitdoit -days <number>          Set the number of visible days (default: 3)
doitdoit -file <path>            Use a different data file for this session
doitdoit config show             Show the data file, theme, and retention
doitdoit config move <new_path>  Move the data file and update the config
doitdoit config theme            Show the current and available themes
doitdoit config theme <name>     Select a theme
doitdoit config retention        Show the completed-history retention
doitdoit config retention forever
doitdoit config retention <days> Set a positive retention period
doitdoit config omarchy-hook install|status|remove
```

## Mobile companion

The [`web/`](./web) directory contains an experimental installable web app for adding, editing, scheduling, reordering, and completing tasks from a phone. It connects directly to the same JSON file through Dropbox. It is outside the `doitdoit-bin` AUR release and its security/privacy assurance; review its separate documentation and threat model before using it with real data.

See [web/README.md](./web/README.md) for Dropbox setup and deployment instructions.

## Development

```bash
go test ./...
go run .
```

Use `go test -count=1 ./...` to bypass the test cache or `go test -cover ./...` for a coverage summary.

## Licence

This software is licensed under the [MIT License](./LICENSE). Third-party attributions and licence terms are in [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).

To uninstall on Omarchy, first run `doitdoit config omarchy-hook remove`, then `omarchy pkg drop doitdoit-bin`. Package removal intentionally leaves your task JSON file and `~/.doitdoit_config.json` in place; back them up and remove them manually only if you no longer want the data.
