# doitdoit

A simple, efficient terminal-based task manager written in Go. `doitdoit` helps you manage your daily tasks with a focus on what's ahead, keeping your workflow smooth and keyboard-driven.

![Screenshot Placeholder](path/to/screenshot.png)

## Features

*   **Clean TUI:** A multi-column terminal interface displaying tasks for today and the next few days.
*   **Automatic Rollover:** Incomplete tasks from previous days are automatically moved to "Today" when you start the app. No task is left behind.
*   **Future Planning:** "Future" view (`f` key) for scheduling.
*   **Keyboard Driven:** Fully navigable and operable using standard Vim-like keys (`h`, `j`, `k`, `l`) or arrow keys.
*   **Bulk Import:** Easily import a list of tasks from a text file.
*   **Data Pruning:** Automatically cleans up tasks older than 5 days to keep your data file lightweight.
*   **Cloud Sync Friendly:** All data is stored in a single JSON file, making it easy to sync across devices using your preferred file sync service.

## Installation & Running

### Prerequisites
*   [Go](https://go.dev/dl/) (1.19 or later recommended)

### Running from Source
```bash
git clone https://github.com/dtt101/doitdoit.git
cd doitdoit
go run main.go
```

### Building
To build a standalone binary:
```bash
go build -o doitdoit
./doitdoit
```

## Running Tests

Ensure you have Go 1.24+ available, then run all tests with:
```bash
go test ./...
```
For a fresh run that skips cache, use `go test ./... -count=1`, and add `-cover` if you want a quick coverage summary.

## Usage

### First Run
On the first run, `doitdoit` will ask where you want to store your data file (`doitdoit.json`). You can choose the default location or specify a custom path.

**Pro Tip:** To sync your tasks across devices, specify a path inside a cloud-synced folder (e.g., `~/Dropbox/doitdoit/` or `~/Google Drive/Tasks/`).

### Keybindings

#### Navigation
*   **Arrow Keys** or **`h` `j` `k` `l`**: Navigate between days (columns) and tasks (rows).
*   **`f`**: Toggle the "Future" view to see tasks without a due date.

#### Task Management
*   **`a`**: Add a new task to the currently selected day/column.
*   **`d`**: Delete the selected task.
*   **`Space`** or **`Enter`**: Toggle task completion status.
*   **`m`**: Enter **Move Mode**.
    *   Use **Left/Right** to move the task to a different day.
    *   Use **Up/Down** to reorder the task within the list.
    *   Press **`m`** or **`Esc`** to exit Move Mode.
*   **`t`**: Set a due date for a task (Only available in Future view).

#### Global
*   **`q`** or **`Ctrl+c`**: Quit the application.

### CLI Commands

The `doitdoit` binary supports several command-line flags and subcommands:

*   `doitdoit`: Launch the main application.
*   `doitdoit -days <number>`: Launch the app displaying a specific number of days (default is 3).
*   `doitdoit -file <path>`: specific a path to the data file for this session.
*   `doitdoit config show`: Display the current path of your data file.
*   `doitdoit config move <new_path>`: Move your data file to a new location and update the configuration.

## Bulk Import

To import multiple tasks at once:
1.  Create a file named `import.txt` in the same directory as your `doitdoit.json` data file.
2.  Add one task per line in the text file.
3.  Run `doitdoit`.
4.  The tasks will be automatically imported into your "Future" list, and the `import.txt` file will be deleted.
