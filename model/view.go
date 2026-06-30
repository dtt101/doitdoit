package model

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dtt101/doitdoit/styles"
)

// Layout overhead used to size columns against the terminal dimensions.
const (
	// appHorizontalMargin is the app's left + right margin (2 each).
	appHorizontalMargin = 4
	// columnChromeWidth is the non-content width each column adds:
	// 2 margin + 2 border + 2 padding.
	columnChromeWidth = 6
	// minColumnWidth is the smallest a column may shrink to.
	minColumnWidth = 10

	// appVerticalOverhead is the app's top + bottom margin (2) plus the
	// footer (~7 lines).
	appVerticalOverhead = 9
	// minTotalColumnHeight is the smallest total column height to target.
	minTotalColumnHeight = 10
	// columnChromeHeight is the non-content height each column adds:
	// 2 border + 2 padding.
	columnChromeHeight = 4
)

func (m Model) View() string {
	availableWidth := m.width - appHorizontalMargin
	if availableWidth < 0 {
		availableWidth = 0
	}

	// If showing future, we just have one column.
	keys := m.dateKeys
	if m.ShowFuture {
		keys = []string{"Future"}
	}

	// Group the visible days into columns. Normally each day is its own
	// column, but Saturday and Sunday are stacked into a single column when
	// more than one day is on screen.
	groups := m.columnGroups(keys)

	numCols := len(groups)
	if numCols < 1 {
		numCols = 1
	}
	colWidth := (availableWidth / numCols) - columnChromeWidth
	if colWidth < minColumnWidth {
		colWidth = minColumnWidth
	}

	minTotalHeight := m.height - appVerticalOverhead
	if minTotalHeight < minTotalColumnHeight {
		minTotalHeight = minTotalColumnHeight
	}
	minContentHeight := minTotalHeight - columnChromeHeight

	// Pre-calculate column contents to determine max height.
	var colContents []string
	maxContentHeight := 0

	for _, group := range groups {
		var sections []string
		for _, dayIdx := range group {
			// Separate stacked days within a column with a blank line.
			if len(sections) > 0 {
				sections = append(sections, "")
			}
			sections = append(sections, m.renderDaySection(keys[dayIdx], dayIdx, colWidth))
		}

		content := lipgloss.JoinVertical(lipgloss.Left, sections...)
		colContents = append(colContents, content)

		h := lipgloss.Height(content)
		if h > maxContentHeight {
			maxContentHeight = h
		}
	}

	// Ensure we meet the minimum window height.
	if maxContentHeight < minContentHeight {
		maxContentHeight = minContentHeight
	}

	// Render columns with unified height.
	var columns []string
	for i, content := range colContents {
		isFocused := m.State != Adding && m.State != SettingDate && m.groupFocused(groups[i])

		style := styles.ColumnStyle.Width(colWidth).Height(maxContentHeight)
		if isFocused {
			style = styles.FocusedColumnStyle.Width(colWidth).Height(maxContentHeight)
		}

		columns = append(columns, style.Render(content))
	}

	footer := m.helpView()
	if errView := m.errorView(); errView != "" {
		footer = errView + "\n" + footer
	}

	return styles.AppStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, columns...) + "\n" + footer)
}

// columnGroups maps the visible day keys to columns, each column being the day
// indices stacked within it. Saturday and Sunday share a column once more than
// one day is shown; the single-day and Future views keep one day per column.
func (m Model) columnGroups(keys []string) [][]int {
	if m.ShowFuture || m.VisibleDays <= 1 {
		groups := make([][]int, len(keys))
		for i := range keys {
			groups[i] = []int{i}
		}
		return groups
	}

	var groups [][]int
	for i := 0; i < len(keys); {
		if !isWeekend(keys[i]) {
			groups = append(groups, []int{i})
			i++
			continue
		}

		// Gather the run of consecutive weekend days (Sat, then Sun).
		var group []int
		for i < len(keys) && isWeekend(keys[i]) {
			group = append(group, i)
			i++
		}
		groups = append(groups, group)
	}
	return groups
}

// groupFocused reports whether the focused day falls within the given column.
func (m Model) groupFocused(group []int) bool {
	if m.ShowFuture {
		return true
	}
	for _, idx := range group {
		if m.ColIdx == idx {
			return true
		}
	}
	return false
}

// renderDaySection builds the header and task list for a single day, with the
// selection highlight applied only when that day is the focused one.
func (m Model) renderDaySection(dateStr string, dayIdx, colWidth int) string {
	isFocused := m.State != Adding && (m.ShowFuture || m.ColIdx == dayIdx)

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

	titleStyle := styles.TitleStyle
	if isFocused {
		titleStyle = styles.FocusedTitleStyle
	}
	title := titleStyle.Render(header)

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
			if m.copyFlash {
				style = style.Foreground(styles.Special).Bold(true)
			} else if m.State == Moving {
				// Use special moving style with highlight background
				style = styles.MovingTaskStyle
			} else {
				// Normal selection highlight
				style = style.Foreground(styles.Highlight).Bold(true)
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

	// Input field if adding to this day
	if (m.State == Adding || m.State == SettingDate) && (m.ShowFuture || m.ColIdx == dayIdx) {
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
	} else if len(tasks) == 0 {
		taskViews = append(taskViews, lipgloss.NewStyle().Foreground(styles.Subtle).Render("No tasks"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, lipgloss.JoinVertical(lipgloss.Left, taskViews...))
}

// isWeekend reports whether the YYYY-MM-DD date string falls on a weekend.
func isWeekend(dateStr string) bool {
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return false
	}
	wd := d.Weekday()
	return wd == time.Saturday || wd == time.Sunday
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
		items = append(items, group("y", "copy"))
		items = append(items, group("space", "toggle"))
		items = append(items, group("m", "move"))
		items = append(items, group("f", "future"))
		if m.ShowFuture {
			items = append(items, group("t", "date"))
			items = append(items, group("T", "to today"))
			items = append(items, group("↑/↓/k/j", "nav"))
		} else {
			items = append(items, group(">", "postpone"))
			items = append(items, group("arrows/hjkl", "nav"))
		}
		items = append(items, group("q", "quit"))
	case Adding:
		items = append(items, group("enter", "save"))
		items = append(items, group("esc", "cancel"))
	case Moving:
		if !m.ShowFuture {
			items = append(items, group("←/→/h/l", "move day"))
		}
		items = append(items, group("↑/↓/k/j", "move up/down"))
		if !m.ShowFuture {
			items = append(items, group("f", "to future"))
		}
		items = append(items, group("y", "copy"))
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
