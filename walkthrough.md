# CLI Todo List App Walkthrough

I have built a CLI todo list application using Go and the Bubble Tea framework. The app organizes tasks by day in a column layout and supports persistence via a JSON file (which can be synced via Dropbox).

## Features

- **Columnar Layout**: Tasks are displayed in columns, one for each day.
- **Navigation**: Vim-like navigation (h/j/k/l or arrows) to move between days and tasks.
- **Task Management**:
    - **Add**: Press `n` to add a new task to the current day.
    - **Toggle**: Press `space` or `enter` to toggle completion status.
    - **Delete**: Press `d` to delete a task.
    - **Move**: Press `m` to enter move mode, then use left/right arrows to move the task to a different day.
- **Persistence**: Data is saved to a JSON file. On first run, the app will prompt you to specify the storage location (e.g., `~/Dropbox/doitdoit.json`). This preference is saved to `~/.doitdoit_config.json`.
- **Auto-Pruning**: Tasks older than 5 days are automatically removed when the app loads/saves.

## Usage

### Installation

```bash
go build -o doitdoit
```

### Running the App

```bash
# Run the app (uses configured storage path)
./doitdoit

# Override storage location for this session
./doitdoit -file ./my_todos.json

# Customize visible days
./doitdoit -days 5
```

### Key Bindings

| Key | Action |
| :--- | :--- |
| `n` | New task |
| `d` | Delete task |
| `space` / `enter` | Toggle completion |
| `m` | Move task (enter move mode) |
| `h` / `l` / `←` / `→` | Navigate columns (or move task in move mode) |
| `j` / `k` / `↓` / `↑` | Navigate tasks |
| `q` | Quit |

## Verification Results

I verified the following scenarios:

1.  **Basic Operations**: Adding, toggling, and deleting tasks works as expected.
2.  **Persistence**: Tasks are saved to the JSON file and reloaded correctly.
3.  **Moving Tasks**: Tasks can be moved between days, and the change is persisted.
4.  **Pruning**: I manually injected a task from 9 days ago into the JSON file. After running the app and triggering a save (by toggling a task), the old task was correctly removed, confirming the 5-day retention rule.

## Code Structure

- `main.go`: Entry point, handles flags and setup.
- `model/task.go`: Data structures and JSON load/save logic.
- `model/ui.go`: Bubble Tea UI model, update loop, and rendering.
- `styles/styles.go`: Lipgloss style definitions for the UI.
