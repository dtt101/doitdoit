package model

import (
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/styles"
)

func TestTaskMarkersAndCountsWithoutColour(t *testing.T) {
	m := newFeedbackTestModel(t)
	key := m.getCurrentKey()
	m.Data[key][1].Completed = true
	m.persist()
	view := ansi.Strip(m.renderDaySection(key, 0, 60))
	for _, want := range []string{"Today · 1 remaining", "[ ] First task", "[x] Second task", "Completed 1 · c hide"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in:\n%s", want, view)
		}
	}
	m = pressSpace(m)
	if !strings.Contains(ansi.Strip(m.View().Content), "0 remaining") {
		t.Fatal("completion did not update remaining count")
	}
	m = pressRune(m, 'u')
	if !strings.Contains(ansi.Strip(m.View().Content), "1 remaining") {
		t.Fatal("undo did not restore remaining count")
	}
	wrapped := ansi.Strip(m.taskView(Task{Title: strings.Repeat("long title ", 10)}, true, 20))
	for i, line := range strings.Split(wrapped, "\n") {
		prefix := "    "
		if i == 0 {
			prefix = "[ ] "
		}
		if !strings.HasPrefix(line, prefix) || lipgloss.Width(line) > 20 {
			t.Fatalf("wrapped task lost its checkbox alignment or exceeded width: %q", line)
		}
	}
	for _, selected := range []bool{false, true} {
		view := ansi.Strip(m.taskView(Task{Title: "1234567890123456"}, selected, 20))
		if view != "[ ] 1234567890123456" {
			t.Fatalf("task should use the space freed by the arrow: %q", view)
		}
	}
}

func TestCollapseIsPresentationOnlyAndSkipsHiddenTasks(t *testing.T) {
	m := newFeedbackTestModel(t)
	key := m.getCurrentKey()
	// Deliberately interleaved: display order must never reorder the JSON.
	m.Data[key] = []Task{
		{ID: "a", Title: "Active A"},
		{ID: "done", Title: "Finished", Completed: true},
		{ID: "b", Title: "Active B"},
	}
	m.persist()
	before, _ := os.ReadFile(m.FilePath)
	m.RowIdx = 1
	m = pressRune(m, 'c')
	if !m.HideCompleted || m.RowIdx != 2 || strings.Contains(m.View().Content, "Finished") {
		t.Fatal("collapsing did not select a visible task")
	}
	m = pressRune(m, 'k')
	if m.RowIdx != 0 {
		t.Fatal("navigation did not skip the completed task")
	}
	m = pressRune(m, 'j')
	if m.RowIdx != 2 {
		t.Fatal("navigation did not follow visible row indices")
	}
	m = pressRune(m, 'c')
	if !strings.Contains(ansi.Strip(m.View().Content), "Finished") {
		t.Fatalf("completed tasks did not expand (hidden=%v):\n%s", m.HideCompleted, ansi.Strip(m.View().Content))
	}
	after, _ := os.ReadFile(m.FilePath)
	if string(before) != string(after) || m.Data[key][1].ID != "done" || m.moveUndo != nil {
		t.Fatal("collapse changed storage, order, or undo history")
	}
}

func TestAllCompletedCollapsedTasksCannotBeMutated(t *testing.T) {
	m := newFeedbackTestModel(t)
	key := m.getCurrentKey()
	for i := range m.Data[key] {
		m.Data[key][i].Completed = true
	}
	m.persist()
	m = pressRune(m, 'c')
	before := cloneTodoData(m.Data)
	for _, key := range []rune{'d', ' ', 'e', 'm', 'J', 'K', '.', 'y'} {
		m = pressRune(m, key)
		if m.State != Browsing || !sameJSON(before, m.Data) || m.moveUndo != nil {
			t.Fatalf("%q acted on a hidden task", key)
		}
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Completed 2 · c show") {
		t.Fatal("collapsed section does not explain how to show tasks")
	}
	m = pressRune(m, 'c')
	m = pressSpace(m)
	if m.Data[key][0].Completed {
		t.Fatal("expanded task could not be reopened")
	}
}

func TestCollapseCompletionUndoAndReloadKeepVisibleSelection(t *testing.T) {
	m := newFeedbackTestModel(t)
	key := m.getCurrentKey()
	m = pressRune(m, 'c')
	m = pressSpace(m)
	if !m.hasSelectedTask() || m.Data[key][m.RowIdx].ID != "2" {
		t.Fatal("completing a task did not select the next visible task")
	}
	m = pressRune(m, 'u')
	if !m.HideCompleted || m.Data[key][m.RowIdx].ID != "1" {
		t.Fatal("undo did not restore the task and collapsed view")
	}
	updated, _ := m.Update(dataFileCheckedMsg{
		data:    TodoData{key: {{ID: "done", Title: "Hidden", Completed: true}, {ID: "active", Title: "Visible"}}},
		modTime: m.dataModTime.Add(time.Second),
	})
	m = updated.(Model)
	if !m.hasSelectedTask() || m.Data[key][m.RowIdx].ID != "active" {
		t.Fatal("reload selected a hidden task")
	}
}

func TestInputAndTaskMarkersFollowLightDarkAndCustomThemes(t *testing.T) {
	t.Cleanup(func() { styles.Apply(styles.DefaultTheme()) })
	custom, err := styles.ThemeFromPalette(map[string]string{
		"foreground": "#242424", "dark_foreground": "#565656", "muted": "#cccccc",
		"accent": "#174099", "magenta": "#703070", "green": "#236523", "red": "#992222", "background": "#ffffff",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nord", "catppuccin-latte", "custom"} {
		t.Run(name, func(t *testing.T) {
			theme := custom
			if name != "custom" {
				var err error
				theme, err = styles.BuiltinTheme(name)
				if err != nil {
					t.Fatal(err)
				}
			}
			m := newFeedbackTestModel(t)
			m = pressRune(m, 'a')
			m.TextInput.SetValue("Keep this input")
			m.TextInput.SetCursor(4)
			updated, _ := m.Update(ThemeReloadMsg{Theme: theme})
			m = updated.(Model)
			inputStyles := m.TextInput.Styles()
			if inputStyles.Focused.Placeholder.GetForeground() != theme.Subtle || inputStyles.Blurred.Placeholder.GetForeground() != theme.Subtle || inputStyles.Focused.Text.GetForeground() != theme.Text {
				t.Fatal("input did not follow the theme roles")
			}
			if m.TextInput.Value() != "Keep this input" || m.TextInput.Position() != 4 {
				t.Fatal("theme reload disturbed input")
			}
			for _, width := range []int{24, 48, 80} {
				m = resizeModel(m, width, 24)
				assertFitsTerminal(t, m)
				view := ansi.Strip(m.taskView(Task{Title: "Done", Completed: true}, true, 20))
				if !strings.HasPrefix(view, "[x] Done") {
					t.Fatal("task state depends on colour")
				}
			}
		})
	}
}
