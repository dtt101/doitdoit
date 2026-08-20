package model

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dtt101/doitdoit/styles"
)

// Layout overhead used to size columns against the terminal dimensions.
const (
	// minColumnContentWidth is the smallest usable width inside a column's
	// border and padding.
	minColumnContentWidth = 10

	// appVerticalOverhead is the app's top + bottom margin (2) plus the
	// footer (~7 lines).
	appVerticalOverhead = 9
	// minTotalColumnHeight is the smallest total column height to target.
	minTotalColumnHeight = 10
)

func (m Model) View() tea.View {
	// If showing future, we just have one column.
	keys := m.dateKeys
	if m.ShowFuture {
		keys = []string{"Future"}
	}

	// Group the visible days into columns. Normally each day is its own
	// column, but Saturday and Sunday are stacked into a single column when
	// more than one day is on screen.
	groups := m.columnGroups(keys)
	columns := m.renderColumns(keys, groups)

	footer := m.helpView()
	if errView := m.errorView(); errView != "" {
		footer = errView + "\n" + footer
	}

	content := styles.AppStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, columns...) + "\n" + footer)
	if m.ShowHelp {
		content = m.renderHelpOverlay(content)
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// renderColumns sizes and renders the visible columns. Lip Gloss v2 treats a
// style's Width and Height as the complete block size including border and
// padding (but excluding margins), so the inner content dimensions must be
// derived from the style's frame rather than subtracted from the block twice.
func (m Model) renderColumns(keys []string, groups [][]int) []string {
	availableWidth := m.width - styles.AppStyle.GetHorizontalFrameSize()
	if availableWidth < 0 {
		availableWidth = 0
	}

	numCols := len(groups)
	if numCols < 1 {
		numCols = 1
	}

	columnHorizontalMargins := styles.ColumnStyle.GetHorizontalMargins()
	columnHorizontalFrame := styles.ColumnStyle.GetHorizontalBorderSize() + styles.ColumnStyle.GetHorizontalPadding()
	columnBlockWidth := (availableWidth / numCols) - columnHorizontalMargins
	minColumnBlockWidth := minColumnContentWidth + columnHorizontalFrame
	if columnBlockWidth < minColumnBlockWidth {
		columnBlockWidth = minColumnBlockWidth
	}
	contentWidth := columnBlockWidth - columnHorizontalFrame

	minTotalHeight := m.height - appVerticalOverhead
	if minTotalHeight < minTotalColumnHeight {
		minTotalHeight = minTotalColumnHeight
	}
	columnVerticalFrame := styles.ColumnStyle.GetVerticalBorderSize() + styles.ColumnStyle.GetVerticalPadding()
	minContentHeight := minTotalHeight - columnVerticalFrame
	if minContentHeight < 1 {
		minContentHeight = 1
	}

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
			sections = append(sections, m.renderDaySection(keys[dayIdx], dayIdx, contentWidth))
		}

		content := lipgloss.JoinVertical(lipgloss.Left, sections...)
		content = lipgloss.Wrap(content, contentWidth, "")
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
	columnBlockHeight := maxContentHeight + columnVerticalFrame

	// Render columns with unified height.
	var columns []string
	for i, content := range colContents {
		isFocused := m.State != Adding && m.State != SettingMoveDate && m.groupFocused(groups[i])

		style := styles.ColumnStyle.Width(columnBlockWidth).Height(columnBlockHeight)
		if isFocused {
			style = styles.FocusedColumnStyle.Width(columnBlockWidth).Height(columnBlockHeight)
		}

		columns = append(columns, style.Render(content))
	}

	return columns
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
			} else if m.State == ChoosingMoveDestination {
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
	if (m.State == Adding || m.State == SettingMoveDate) && (m.ShowFuture || m.ColIdx == dayIdx) {
		// Add spacing before input if there are tasks
		if len(tasks) > 0 {
			taskViews = append(taskViews, "")
		}

		// Match TaskStyle padding
		inputStyle := lipgloss.NewStyle()
		prefix := ""
		if m.State == SettingMoveDate {
			prefix = "Move to: "
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
	key := func(k string) string {
		return styles.KeyStyle.Render(k)
	}
	desc := func(d string) string {
		return lipgloss.NewStyle().Foreground(styles.Subtle).Render(d)
	}
	group := func(k, d string) string {
		return key(k) + " " + desc(d)
	}

	brand := m.brandView()
	if m.State == Browsing {
		return styles.HelpStyle.Render(brand + desc(". Press ") + key("?") + desc(" for help"))
	}

	helpItems := m.footerHelpItems()
	items := make([]string, len(helpItems))
	for i, item := range helpItems {
		items[i] = group(item.key, item.description)
	}
	prefix := brand + desc(". ")
	if m.State == ChoosingMoveDestination {
		prefix += desc("Move to: ")
	}
	return styles.HelpStyle.Render(wrapFooterItems(prefix, items, m.footerContentWidth()))
}

// wrapFooterItems keeps each key/description pair together and moves whole
// destinations onto the next line when the footer is narrower than the list.
func wrapFooterItems(prefix string, items []string, width int) string {
	if width <= 0 {
		return prefix + strings.Join(items, "   ")
	}

	var lines []string
	current := prefix
	itemsOnLine := 0
	for _, item := range items {
		separator := ""
		if itemsOnLine > 0 {
			separator = "   "
		}
		if lipgloss.Width(current)+lipgloss.Width(separator)+lipgloss.Width(item) > width && itemsOnLine > 0 {
			lines = append(lines, current)
			current = item
			itemsOnLine = 1
			continue
		}
		if lipgloss.Width(current)+lipgloss.Width(item) > width && itemsOnLine == 0 && current != "" {
			lines = append(lines, current)
			current = item
			itemsOnLine = 1
			continue
		}
		current += separator + item
		itemsOnLine++
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) footerHelpItems() []helpItem {
	switch m.State {
	case Browsing:
		return []helpItem{{"?", "help"}}
	case Adding:
		return []helpItem{{"enter", "save"}, {"esc", "cancel"}}
	case ChoosingMoveDestination:
		return m.moveDestinationHelpItems()
	case SettingMoveDate:
		return []helpItem{{"enter", "move"}, {"esc", "back"}}
	default:
		return nil
	}
}

func (m Model) footerContentWidth() int {
	if m.width <= 0 {
		return 0
	}
	return max(0, m.width-styles.AppStyle.GetHorizontalFrameSize()-styles.HelpStyle.GetHorizontalFrameSize())
}

func (m Model) brandView() string {
	const brand = "doitdoit"
	if m.brandFrame == 0 {
		return lipgloss.NewStyle().Foreground(styles.Special).Render(brand)
	}

	const matrixGlyphs = "01#$%&*+"
	resolved := max(0, m.brandFrame-4)
	var rendered strings.Builder
	for i, letter := range brand {
		glyph := letter
		if i >= resolved {
			glyph = rune(matrixGlyphs[(i*3+m.brandFrame*5)%len(matrixGlyphs)])
		}
		rendered.WriteString(lipgloss.NewStyle().
			Foreground(styles.Special).
			Bold(i < resolved).
			Render(string(glyph)))
	}
	return rendered.String()
}

// brandBounds returns the terminal cells occupied by the footer wordmark.
func (m Model) brandBounds() (x, y, width int, ok bool) {
	contentWidth := m.footerContentWidth()
	const brandWidth = len("doitdoit")
	if contentWidth < brandWidth || m.width <= 0 || m.height <= 0 {
		return 0, 0, 0, false
	}

	keys := m.dateKeys
	if m.ShowFuture {
		keys = []string{"Future"}
	}
	columns := lipgloss.JoinHorizontal(lipgloss.Top, m.renderColumns(keys, m.columnGroups(keys))...)

	x = styles.AppStyle.GetMarginLeft() + styles.HelpStyle.GetMarginLeft()
	y = styles.AppStyle.GetMarginTop() + lipgloss.Height(columns) + styles.HelpStyle.GetMarginTop()
	if errView := m.errorView(); errView != "" {
		y += lipgloss.Height(errView)
	}
	if x < 0 || x+brandWidth > m.width || y < 0 || y >= m.height {
		return 0, 0, 0, false
	}
	return x, y, brandWidth, true
}

type helpItem struct {
	key         string
	description string
}

func (m Model) moveDestinationHelpItems() []helpItem {
	items := []helpItem{{"t", "today"}}
	base := m.moveBaseDate()
	for days := 1; days <= 7; days++ {
		date := base.AddDate(0, 0, days)
		items = append(items, helpItem{fmt.Sprintf("%d", days), date.Format("Mon 02")})
	}
	return append(items,
		helpItem{"f", "future"},
		helpItem{"d", "other date"},
		helpItem{"esc", "cancel"},
	)
}

func (m Model) helpItems() []helpItem {
	navigation := "arrows / hjkl"
	viewToggle := "Future view"
	if m.ShowFuture {
		navigation = "↑/↓ / k/j"
		viewToggle = "main view"
	}
	return []helpItem{
		{navigation, "navigate"},
		{"a", "add task"},
		{"space / enter", "toggle task"},
		{"d", "delete task"},
		{"y", "copy task"},
		{"m", "move task"},
		{"J / K", "reorder task"},
		{".", "repeat move"},
		{"u", "undo move"},
		{"f", viewToggle},
		{"q / ctrl+c", "quit"},
	}
}

func (m Model) helpModalView() string {
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Highlight).
		Padding(1, 2)

	modalWidth := 64
	if m.width > 0 && modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	minimumModalWidth := modalStyle.GetHorizontalFrameSize() + 1
	if modalWidth < minimumModalWidth {
		modalWidth = minimumModalWidth
	}
	innerWidth := modalWidth - modalStyle.GetHorizontalFrameSize()

	renderItem := func(item helpItem, width int) string {
		itemGap := 2
		keyWidth := min(14, max(1, width/2))
		if keyWidth+itemGap >= width {
			itemGap = min(1, max(0, width-2))
			keyWidth = max(1, width-itemGap-1)
		}
		descriptionWidth := max(1, width-keyWidth-itemGap)
		return lipgloss.JoinHorizontal(lipgloss.Top,
			styles.KeyStyle.Width(keyWidth).Render(item.key),
			strings.Repeat(" ", itemGap),
			lipgloss.NewStyle().Foreground(styles.Text).Width(descriptionWidth).Render(item.description),
		)
	}

	items := m.helpItems()
	var shortcuts string
	if innerWidth >= 48 {
		gap := 5
		leftPaneWidth := (innerWidth - gap) / 2
		rightPaneWidth := innerWidth - gap - leftPaneWidth
		midpoint := (len(items) + 1) / 2
		left, right := items[:midpoint], items[midpoint:]
		rows := make([]string, midpoint)
		for i := range midpoint {
			leftItem := renderItem(left[i], leftPaneWidth)
			rightItem := strings.Repeat(" ", rightPaneWidth)
			if i < len(right) {
				rightItem = renderItem(right[i], rightPaneWidth)
			}
			rows[i] = lipgloss.JoinHorizontal(lipgloss.Top, leftItem, strings.Repeat(" ", gap), rightItem)
		}
		shortcuts = lipgloss.JoinVertical(lipgloss.Left, rows...)
	} else {
		rows := make([]string, len(items))
		for i, item := range items {
			rows[i] = renderItem(item, innerWidth)
		}
		shortcuts = lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	closeHint := lipgloss.NewStyle().Foreground(styles.Subtle).Render("Press Esc to close")
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.FocusedTitleStyle.Render("Keyboard shortcuts"),
		shortcuts,
		"",
		closeHint,
	)
	return modalStyle.Width(modalWidth).Render(body)
}

func (m Model) renderHelpOverlay(background string) string {
	modal := m.helpModalView()
	if m.width <= 0 || m.height <= 0 {
		return modal
	}
	x := max(0, (m.width-lipgloss.Width(modal))/2)
	y := max(0, (m.height-lipgloss.Height(modal))/2)
	compositor := lipgloss.NewCompositor(
		lipgloss.NewLayer(background),
		lipgloss.NewLayer(modal).X(x).Y(y).Z(1),
	)
	return lipgloss.NewCanvas(m.width, m.height).
		Compose(compositor).
		Render()
}

func (m Model) errorView() string {
	if m.Err == nil {
		return ""
	}
	return lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("Error: %v", m.Err))
}
