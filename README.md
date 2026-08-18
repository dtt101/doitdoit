# doitdoit

A simple, efficient terminal-based task manager written in Go. `doitdoit` helps you manage your daily tasks with a focus on what's ahead, keeping your workflow smooth and keyboard-driven.

This software is licensed under the terms described in [LICENCE.md](./LICENCE.md).  
Inspired by the O'SaaSy License from [37signals](https://www.fizzy.do/license).

![Screenshot](https://github.com/user-attachments/assets/f91436e2-55e5-4c3f-8eee-742b2057265a)

## Features

*   **Clean TUI:** A multi-column terminal interface displaying tasks for today and the next few days. When more than one day is shown, Saturday and Sunday are stacked together in a single weekend column.
*   **Automatic Rollover:** Incomplete tasks from previous days are automatically moved to "Today" when you start the app. No task is left behind.
*   **Future Planning:** "Future" view (`f` key) for scheduling.
*   **Keyboard Driven:** Fully navigable and operable using standard Vim-like keys (`h`, `j`, `k`, `l`) or arrow keys.
*   **Bulk Import:** Easily import a list of tasks from a text file.
*   **Data Pruning:** Automatically cleans up tasks older than 5 days to keep your data file lightweight.
*   **Cloud Sync Friendly:** All data is stored in a single JSON file, making it easy to sync across devices using your preferred file sync service.
*   **Omarchy Theming:** Automatically follows the current [Omarchy](https://omarchy.org) theme, and ships the Omarchy stock palettes as built-in themes for other platforms (see [Theming](#theming)).
*   **Mobile Companion:** A static web app under [`web/`](./web) reads and writes the same Dropbox JSON file from a phone. See [web/README.md](./web/README.md).

## Installation & Running

### Prerequisites
*   [Go](https://go.dev/dl/) 1.24 or later

### Install via `go install`
```bash
go install github.com/dtt101/doitdoit@latest
```
This builds and places the binary in `$GOPATH/bin` (usually `~/go/bin`); ensure that directory is on your `PATH`.

### Running from Source
```bash
git clone https://github.com/dtt101/doitdoit.git
cd doitdoit
go run .
```

### Building
To build a standalone binary:
```bash
go build -o doitdoit
./doitdoit
```

## Running Tests

```bash
go test ./...
```
Add `-count=1` to skip the cache, or `-cover` for a coverage summary.

## Usage

### First Run
On the first run, `doitdoit` will ask where you want to store your data file (`doitdoit.json`). You can choose the default location or specify a custom path.

**Pro Tip:** To sync your tasks across devices, specify a path inside a cloud-synced folder (e.g., `~/Dropbox/doitdoit/` or `~/Google Drive/Tasks/`).

### Keybindings

#### Navigation
*   In the main view, use **Arrow Keys** or **`h` `j` `k` `l`** to navigate between dated columns and tasks. Moving right past the last visible day scrolls the date window forward indefinitely; moving left scrolls back as far as Today.
*   In the Future view, use **Up/Down** or **`j` `k`** to navigate its task list.
*   **`f`**: Toggle between the main and Future views.

#### Task Management
*   **`a`**: Add a new task to the currently selected day/column.
*   **`d`**: Delete the selected task.
*   **`Space`** or **`Enter`**: Toggle task completion status.
*   **`y`**: Copy the selected task's text to the clipboard.
*   **`m`**: Choose where to move the selected task.
    *   Press **`t`** for Today or **`f`** for the undated Future list.
    *   Press **`1`**–**`7`** to move that many calendar days from the task's current date in the main view. Future tasks count from today.
    *   Press **`d`** to enter a date as `YYYY-MM-DD` or `MM-DD`.
    *   Press **`Esc`** to cancel.
*   **`J`** / **`K`**: Move the selected task down or up within its current list.
*   **`.`**: Move the selected task to the last destination again.
*   **`u`**: Undo the most recent task move or reorder.

After scheduling a task, focus stays in its source list so the next task can be moved immediately. These task-management keys work in both the main and Future views.

#### Global
*   **`q`** or **`Ctrl+c`**: Quit the application.

### CLI Commands

The `doitdoit` binary supports several command-line flags and subcommands:

*   `doitdoit`: Launch the main application.
*   `doitdoit -days <number>`: Set the number of days in the scrolling viewport (default is 3).
*   `doitdoit -file <path>`: Specify a path to the data file for this session.
*   `doitdoit config show`: Display the current path of your data file and the configured theme.
*   `doitdoit config move <new_path>`: Move your data file to a new location and update the configuration.
*   `doitdoit config theme`: Show the current theme and list all available themes.
*   `doitdoit config theme <name>`: Set the theme.

## Theming

`doitdoit` supports [Omarchy](https://omarchy.org) theming (Omarchy 4 "Quattro" and later) and ships the Omarchy stock palettes as built-in themes for every other platform.

### On Omarchy

With no theme configured, `doitdoit` follows the currently applied Omarchy theme — stock or user-installed — by reading `~/.local/state/omarchy/current/theme/colors.toml`. To retint running instances the moment you switch themes, add a hook at `~/.config/omarchy/hooks/theme-set`:

```bash
#!/bin/bash
pkill -SIGUSR2 doitdoit
```

`SIGUSR2` triggers a theme reload, the same convention Omarchy uses for btop and Helix. Without the hook, the new theme applies next time `doitdoit` starts.

### Everywhere else (e.g. macOS)

Pick any of the embedded Omarchy stock palettes by name:

```bash
doitdoit config theme            # show current theme + list available ones
doitdoit config theme tokyo-night
```

Available themes: `catppuccin`, `catppuccin-latte`, `ethereal`, `everforest`, `flexoki-light`, `gruvbox`, `hackerman`, `kanagawa`, `last-horizon`, `lumon`, `lupine`, `matte-black`, `miasma`, `nord`, `osaka-jade`, `retro-82`, `ristretto`, `rose-pine`, `solitude`, `tokyo-night`, `vantablack`, `white`.

`doitdoit` doesn't paint its own background — it renders on your terminal's. On Omarchy the terminal is themed in lockstep so everything matches; elsewhere, pick a palette that suits your terminal background (`catppuccin-latte`, `flexoki-light`, `lupine`, `rose-pine`, and `white` are the light ones).

### Colour mapping

Each palette's `foreground` becomes task text, `accent` the selection and focused column, `magenta` the key hints, `green` the day titles, and `red` errors. Dim text (completed tasks, help) uses `dark_foreground`, the palette's comment colour, while unfocused column borders use the background-tier `muted`.

### Configuration

*   `doitdoit config theme system` returns to the automatic behaviour: follow the live Omarchy theme when present, otherwise use the built-in adaptive palette.
*   The theme is stored in `~/.doitdoit_config.json` alongside the storage path, so each machine can have its own theme while sharing the same task data.

## Bulk Import

To import multiple tasks at once:
1.  Create a file named `import.txt` in the same directory as your `doitdoit.json` data file.
2.  Add one task per line in the text file.
3.  Run `doitdoit`.
4.  The tasks will be automatically imported into your "Future" list, and the `import.txt` file will be deleted.
