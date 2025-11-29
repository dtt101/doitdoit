package model

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dtt101/doitdoit/styles"
)

func (m Model) View() string {
	var columns []string

	// Calculate dynamic width
	// App margins: 4 (2 left + 2 right)
	// Column margins: 2 per column (1 left + 1 right)
	// Column borders: 2 per column
	// Column padding: 2 per column
	// Total extra per column = 6

	availableWidth := m.width - 4
	if availableWidth < 0 {
		availableWidth = 0
	}

	colWidth := (availableWidth / m.VisibleDays) - 6
	if colWidth < 10 {
		colWidth = 10 // Minimum width
	}

	// Pre-calculate column contents to determine max height
	var colContents []string
	maxContentHeight := 0

	// Minimum height based on window size
	// App margins: 2 (1 top + 1 bottom)
	// Footer overhead: ~7 lines
	// Column overhead: 4 lines (2 border + 2 padding)
	minTotalHeight := m.height - 9
	if minTotalHeight < 10 {
		minTotalHeight = 10
	}
	minContentHeight := minTotalHeight - 4 // Subtract border+padding

	// If showing future, we just have one column
	keys := m.dateKeys
	if m.ShowFuture {
		keys = []string{"Future"}
		colWidth = availableWidth - 6
	}

	for i, dateStr := range keys {
		isFocused := m.State != Adding && (m.ShowFuture || m.ColIdx == i)

		// Header
		header := ""
		if m.ShowFuture {
			header = "Future"
		} else {
			displayDate, _ := time.Parse("2006-01-02", dateStr)
			header = displayDate.Format("Mon, Jan 02")
			if dateStr == time.Now().Format("2006-01-02") {
				header = "Today"
			}
		}

		title := styles.TitleStyle.Render(header)

		// Tasks
		var taskViews []string
		tasks := m.Data[dateStr]

		for j, task := range tasks {
			var style lipgloss.Style
			if task.Completed {
				style = styles.CompletedTaskStyle
			} else {
				style = styles.TaskStyle
			}

			title := task.Title
			if m.ShowFuture && task.DueDate != "" {
				title += fmt.Sprintf(" (%s)", task.DueDate)
			}

			if isFocused && m.RowIdx == j {
				if m.State == Moving {
					// Use special moving style with highlight background
					style = styles.MovingTaskStyle
				} else {
					// Normal selection highlight
					style = style.Copy().Foreground(styles.Highlight).Bold(true)
				}
			}

			// Calculate title width to ensure proper wrapping
			titleWidth := colWidth
			if titleWidth < 1 {
				titleWidth = 1
			}

			taskViews = append(taskViews, style.Width(titleWidth).Render(title))

			// Add a blank line between tasks
			if j < len(tasks)-1 {
				taskViews = append(taskViews, "")
			}
		}

		// Input field if adding to this column
		if (m.State == Adding || m.State == SettingDate) && (m.ShowFuture || m.ColIdx == i) {
			// Add spacing before input if there are tasks
			if len(tasks) > 0 {
				taskViews = append(taskViews, "")
			}

			// Match TaskStyle padding
			inputStyle := lipgloss.NewStyle()
			prefix := ""
			if m.State == SettingDate {
				prefix = "Due Date: "
			}
			taskViews = append(taskViews, inputStyle.Render(prefix+m.TextInput.View()))
		} else if len(tasks) == 0 && !((m.State == Adding || m.State == SettingDate) && (m.ShowFuture || m.ColIdx == i)) {
			taskViews = append(taskViews, lipgloss.NewStyle().Foreground(styles.Subtle).Render("No tasks"))
		}

		// Assemble content
		content := lipgloss.JoinVertical(lipgloss.Left, title, lipgloss.JoinVertical(lipgloss.Left, taskViews...))
		colContents = append(colContents, content)

		h := lipgloss.Height(content)
		if h > maxContentHeight {
			maxContentHeight = h
		}
	}

	// Ensure we meet the minimum window height
	if maxContentHeight < minContentHeight {
		maxContentHeight = minContentHeight
	}

	// Render columns with unified height
	for i, content := range colContents {
		isFocused := m.State != Adding && m.State != SettingDate && (m.ShowFuture || m.ColIdx == i)

		style := styles.ColumnStyle.Copy().Width(colWidth).Height(maxContentHeight)
		if isFocused {
			style = styles.FocusedColumnStyle.Copy().Width(colWidth).Height(maxContentHeight)
		}

		columns = append(columns, style.Render(content))
	}

	footer := m.helpView()
	if errView := m.errorView(); errView != "" {
		footer = errView + "\n" + footer
	}

	return styles.AppStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, columns...) + "\n" + footer)
}

func (m Model) helpView() string {
	var items []string

	key := func(k string) string {
		return styles.KeyStyle.Render(k)
	}
	desc := func(d string) string {
		return lipgloss.NewStyle().Foreground(styles.Subtle).Render(d)
	}
	group := func(k, d string) string {
		return key(k) + " " + desc(d)
	}

	switch m.State {
	case Browsing:
		items = append(items, group("a", "add"))
		items = append(items, group("d", "delete"))
		items = append(items, group("space", "toggle"))
		items = append(items, group("m", "move"))
		items = append(items, group("f", "future"))
		if m.ShowFuture {
			items = append(items, group("t", "date"))
		}
		items = append(items, group("arrows/hjkl", "nav"))
		items = append(items, group("q", "quit"))
	case Adding:
		items = append(items, group("enter", "save"))
		items = append(items, group("esc", "cancel"))
	case Moving:
		if !m.ShowFuture {
			items = append(items, group("←/→/h/l", "move day"))
		}
		items = append(items, group("↑/↓/k/j", "move up/down"))
		items = append(items, group("m/esc", "done"))
	case SettingDate:
		items = append(items, group("enter", "save date"))
		items = append(items, group("esc", "cancel"))
	}

	var helpStr string
	for i, item := range items {
		if i > 0 {
			helpStr += "   "
		}
		helpStr += item
	}

	return styles.HelpStyle.Render(helpStr)
}

func (m Model) errorView() string {
	if m.Err == nil {
		return ""
	}
	return lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("Error: %v", m.Err))
}
