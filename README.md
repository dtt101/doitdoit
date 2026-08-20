# doitdoit

**A fast, keyboard-first task manager for your terminal — with your tasks kept in a file you own.**

`doitdoit` gives you a calm, multi-day view of what is in front of you, gets out of the way when you are working, and makes sure unfinished tasks do not disappear into yesterday. There is no account, database, or hosted backend: the entire backend is one readable JSON file.

Put that file in Dropbox, iCloud Drive, or Google Drive and your task list can travel with you.

<img width="1400" height="770" alt="Screenshot 2026-08-20 at 09 20 20" src="https://github.com/user-attachments/assets/fa6758d4-92a9-4853-824c-b4e0cd873095" />


## Why doitdoit?

- **See the days ahead.** Work from a clean, scrolling multi-day view instead of a long, undifferentiated list. Weekends are intelligently stacked to save space.
- **Keep your data yours.** Everything lives in a single portable, human-readable JSON file. Back it up, inspect it, script against it, or sync it with the service you already use.
- **Never lose an unfinished task.** Anything incomplete automatically rolls forward to Today, while old history is pruned to keep the file lean.
- **Plan quickly from the keyboard.** Add, complete, delete, copy, reorder, schedule, repeat a move, and undo without leaving the terminal.
- **Capture now, decide later.** Drop ideas into Future, then schedule them for tomorrow, the next seven days, or an exact date when you are ready.
- **Looks at home on Omarchy.** `doitdoit` follows your active Omarchy theme and includes every stock Omarchy 4 (Quattro) palette.
- **Take it to your phone.** The optional static [web companion](./web) can read and write the same Dropbox-hosted task file.

## Especially nice on Omarchy

`doitdoit` is built to feel like part of an [Omarchy](https://omarchy.org) desktop, not a generic TUI dropped into it.

With no extra configuration, it reads the currently active Omarchy 4 (Quattro) theme — including user-installed themes — from `~/.local/state/omarchy/current/theme/colors.toml`. It also bundles all 22 stock Quattro palettes, so they are available on macOS, other Linux distributions, and Windows too.

> **AUR package coming soon** for an even easier install on Omarchy and Arch Linux.

Add the optional theme hook below and any running `doitdoit` instance will retint as soon as you switch your Omarchy theme:

```bash
mkdir -p ~/.config/omarchy/hooks
$EDITOR ~/.config/omarchy/hooks/theme-set
```

Add:

```bash
#!/bin/bash
pkill -SIGUSR2 doitdoit
```

Without the hook, `doitdoit` simply picks up the new theme on its next launch.

## Try it

Requires [Go](https://go.dev/dl/) 1.24 or later.

```bash
go install github.com/dtt101/doitdoit@latest
doitdoit
```

On first launch, choose where `doitdoit.json` should live. Accept a normal local path, or point it straight at a synced folder:

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

This makes a few useful workflows almost effortless:

- Put the data file in a Dropbox, iCloud Drive, or Google Drive folder to sync it between machines.
- Keep different machine-specific themes while sharing exactly the same tasks.
- Back up or version the file using ordinary file tools.
- Create tasks from another script or tool using a simple, open JSON format.
- Use `-file` to open a different task list for a project or a one-off session.

The TUI checks for external changes every few seconds and reloads them without disturbing you while you are typing. Like most file-sync workflows, simultaneous edits are last-write-wins, so avoid changing the same file from two devices at exactly the same moment.

Need to relocate an existing task file later? `doitdoit config move <new_path>` moves it and updates your configuration safely.

## Daily workflow

The default view shows Today and the days immediately ahead. Move right beyond the final column and the calendar keeps scrolling forward; move left to return toward Today. Saturday and Sunday share a compact weekend column whenever multiple days are visible.

Press `f` for the separate Future list. Tasks with a specific future date remain there until that date enters the visible window, while undated ideas wait until you decide what to do with them.

### Keybindings

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
doitdoit config show             Show the configured data file and theme
doitdoit config move <new_path>  Move the data file and update the config
doitdoit config theme            Show the current and available themes
doitdoit config theme <name>     Select a theme
```

## Bulk import

To turn a plain list into tasks, create `import.txt` beside your `doitdoit.json`, with one task per line, and launch `doitdoit`. The tasks are added to Future and the import file is removed after a successful import.

```text
book dentist
renew passport
plan weekend trip
```

## Mobile companion

The [`web/`](./web) directory contains a small installable web app for adding, editing, scheduling, reordering, and completing tasks from a phone. It connects directly to the same JSON file through Dropbox: no application server, framework, or build step required.

See [web/README.md](./web/README.md) for Dropbox setup and deployment instructions.

## Development

```bash
go test ./...
go run .
```

Use `go test -count=1 ./...` to bypass the test cache or `go test -cover ./...` for a coverage summary.

## Licence

This software is licensed under the terms in [LICENCE.md](./LICENCE.md), inspired by the O'SaaSy License from [37signals](https://www.fizzy.do/license).
