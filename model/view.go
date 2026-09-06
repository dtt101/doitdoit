package model

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dtt101/doitdoit/styles"
)

func (m Model) View() tea.View {
	content := ""
	if m.terminalTooSmall() {
		content = fitLines("Window too small.\nResize to at least 24 x 10.\nq quit", m.width, m.height)
	} else {
		columns := m.renderColumns(m.visibleKeys())
		content = m.appStyle().Render(lipgloss.JoinHorizontal(lipgloss.Top, columns...) + "\n" + m.footerView())
		if m.ShowHelp {
			content = m.renderHelpOverlay(content)
		}
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// Every column has a fixed viewport. Task count and wrapped titles cannot push
// the footer below the terminal, and headers stay visible while paging.
func (m Model) renderColumns(keys []string) []string {
	geometry := m.columnGeometry(len(keys))
	columns := make([]string, 0, len(keys))
	for i, key := range keys {
		dayIdx := m.visibleColumnStart() + i
		if m.ShowFuture {
			dayIdx = m.ColIdx
		}
		focused := m.ShowFuture || m.ColIdx == dayIdx
		doc := m.dayContent(key, dayIdx, geometry.contentWidth)
		rows := doc.viewportRows(geometry.contentHeight)
		offset := doc.visibleOffset(m.scrollOffsets[key], rows, focused)
		end := min(len(doc.lines), offset+rows)
		body := strings.Join(doc.lines[offset:end], "\n")
		body = lipgloss.NewStyle().Height(rows).Render(body)
		content := doc.header + "\n" + body
		if len(doc.lines) > rows && geometry.contentHeight > lipgloss.Height(doc.header)+rows {
			indicator := "more"
			if m.State == Browsing {
				indicator = "pgup/pgdn"
			}
			if offset > 0 {
				indicator = "↑ " + indicator
			}
			if end < len(doc.lines) {
				indicator += " ↓"
			}
			content += "\n" + lipgloss.NewStyle().Foreground(styles.Subtle).Render(indicator)
		}
		style := m.columnStyle(focused).Width(geometry.width).Height(geometry.height)
		columns = append(columns, style.Render(content))
	}
	return columns
}

// Kept as the complete section renderer for tests and previews; the board uses
// dayContent to slice task lines independently of the fixed header.
func (m Model) renderDaySection(dateStr string, dayIdx, colWidth int) string {
	doc := m.dayContent(dateStr, dayIdx, colWidth)
	return doc.header + "\n" + strings.Join(doc.lines, "\n")
}

func (m Model) dayContent(dateStr string, dayIdx, colWidth int) dayContent {
	focused := m.ShowFuture || m.ColIdx == dayIdx
	header := "Future"
	if !m.ShowFuture {
		displayDate, _ := time.Parse(dateLayout, dateStr)
		header = displayDate.Format("Mon, Jan 02")
		if dateStr == time.Now().Format(dateLayout) {
			header = "Today"
			if m.FocusToday {
				header += " (focus)"
			}
		}
	}
	doc := dayContent{header: styles.TitleStyle.Render(header), focusStart: -1}
	appendBlock := func(content string) {
		doc.lines = append(doc.lines, strings.Split(lipgloss.Wrap(content, colWidth, ""), "\n")...)
	}
	appendInput := func() {
		// Render a resized copy as well, so View is correct before the next
		// update (and does not mutate the input's value or cursor).
		input := m.sizedTextInput(colWidth)
		doc.focusStart = len(doc.lines)
		appendBlock(m.inputPrefix() + input.View())
		doc.focusEnd = len(doc.lines)
	}

	tasks := m.Data[dateStr]
	for j, task := range tasks {
		if j > 0 {
			doc.lines = append(doc.lines, "")
		}
		start := len(doc.lines)
		selected := focused && m.RowIdx == j
		if selected && m.State == Editing {
			appendInput()
		} else {
			style := styles.TaskStyle
			if task.Completed {
				style = styles.CompletedTaskStyle
			}
			if selected && m.State != Adding {
				switch {
				case m.copyFlash:
					style = style.Foreground(styles.Special).Bold(true)
				case m.State == ChoosingMoveDestination:
					style = styles.MovingTaskStyle
				default:
					style = style.Foreground(styles.Highlight).Bold(true)
				}
			}
			title := task.Title
			if m.ShowFuture && task.DueDate != "" {
				title += fmt.Sprintf(" (%s)", task.DueDate)
			}
			appendBlock(style.Width(colWidth).Render(title))
			if selected && m.State != Adding {
				doc.focusStart, doc.focusEnd = start, len(doc.lines)
			}
		}
		doc.tasks = append(doc.tasks, taskSpan{start: start, end: len(doc.lines)})
		if selected && m.State == SettingMoveDate {
			appendInput()
		}
	}
	if focused && m.State == Adding {
		if len(tasks) > 0 {
			doc.lines = append(doc.lines, "")
		}
		appendInput()
	} else if len(tasks) == 0 {
		message := "Nothing planned yet.\nSelect this day to add a task."
		if focused {
			switch {
			case m.ShowFuture:
				message = "Capture an idea for later."
			case dateStr == time.Now().Format(dateLayout):
				message = "What needs doing today?"
			default:
				message = "Plan something for this day."
			}
			message += "\nPress a to add a task."
		}
		appendBlock(lipgloss.NewStyle().Foreground(styles.Subtle).Render(message))
	}
	return doc
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
	helpItems := m.footerHelpItems()
	items := make([]string, len(helpItems))
	for i, item := range helpItems {
		items[i] = group(item.key, item.description)
	}
	prefix := brand + desc(". ")
	compact := (m.height > 0 && m.height < 16) || (m.width > 0 && m.width < 36)
	if compact {
		prefix = ""
	}
	if m.State == ChoosingMoveDestination {
		prefix += desc("Move to: ")
		if compact {
			prefix = ""
			items = []string{group("t", "today"), group("1–7", "+days"), group("f", "future"), group("d", "date"), group("esc", "cancel")}
		}
	}
	footer := wrapFooterItems(prefix, items, m.footerContentWidth())
	if m.State == Browsing && m.Err == nil && m.feedback != "" {
		feedbackItems := []string{lipgloss.NewStyle().Foreground(styles.Text).Render(m.feedback)}
		if m.moveUndo != nil {
			feedbackItems = append(feedbackItems, group("u", "undo"))
		}
		footer += "\n" + wrapFooterItems("", feedbackItems, m.footerContentWidth())
	}
	return m.footerStyle().Render(footer)
}

func (m Model) footerView() string {
	footer := m.helpView()
	if errView := m.errorView(); errView != "" {
		footer = errView + "\n" + footer
	}
	return footer
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
		viewToggle := "future"
		if m.ShowFuture {
			viewToggle = "days"
		}
		items := []helpItem{{"a", "add"}, {"space", "complete"}, {"m", "move"}, {"f", viewToggle}, {"?", "help"}}
		if (m.height == 0 || m.height >= 16) && (m.width == 0 || m.width >= 36) {
			focusToggle := "focus today"
			if m.FocusToday {
				focusToggle = "all days"
			}
			items = append(items, helpItem{"t", "today"}, helpItem{"T", focusToggle})
		}
		return items
	case Adding:
		return []helpItem{{"enter", "save"}, {"esc", "cancel"}}
	case Editing:
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
	return max(0, m.width-m.appStyle().GetHorizontalFrameSize()-m.footerStyle().GetHorizontalFrameSize())
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
	if contentWidth < brandWidth || m.width < 36 || m.height < 16 {
		return 0, 0, 0, false
	}

	columns := lipgloss.JoinHorizontal(lipgloss.Top, m.renderColumns(m.visibleKeys())...)

	x = m.appStyle().GetMarginLeft() + m.footerStyle().GetMarginLeft()
	y = m.appStyle().GetMarginTop() + lipgloss.Height(columns) + m.footerStyle().GetMarginTop()
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
	if m.FocusToday && !m.ShowFuture {
		navigation = "↑/↓ / k/j"
	}
	return []helpItem{
		{navigation, "navigate"},
		{"a", "add task"},
		{"e", "edit task"},
		{"space / enter", "toggle task"},
		{"d", "delete task"},
		{"y", "copy task"},
		{"m", "move task"},
		{"J / K", "reorder task"},
		{".", "repeat move"},
		{"u", "undo last change"},
		{"f", viewToggle},
		{"t", "return to Today"},
		{"T", "toggle Today focus"},
		{"pgup / pgdown", "scroll list"},
		{"q / ctrl+c", "quit"},
	}
}

func (m Model) helpModalView() string {
	view, _ := m.renderHelpModal(m.helpOffset)
	return view
}

func (m Model) helpMaxOffset() int {
	_, maximum := m.renderHelpModal(0)
	return maximum
}

func (m Model) renderHelpModal(offset int) (string, int) {
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Highlight).
		Padding(1, 2)
	if m.compactLayout() {
		modalStyle = modalStyle.Padding(0, 1)
	}

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

	heading := styles.TitleStyle.Width(innerWidth).Render("Keyboard shortcuts")
	closeHint := lipgloss.NewStyle().Foreground(styles.Subtle).Width(innerWidth).Render("Press Esc to close")
	lines := strings.Split(shortcuts, "\n")
	rows := len(lines)
	if m.height > 0 {
		rows = min(rows, max(1, m.height-2-modalStyle.GetVerticalFrameSize()-lipgloss.Height(heading)-lipgloss.Height(closeHint)-1))
	}
	maximum := max(0, len(lines)-rows)
	offset = min(max(0, offset), maximum)
	shortcuts = strings.Join(lines[offset:offset+rows], "\n")
	scrollHint := ""
	if maximum > 0 {
		scrollHint = lipgloss.NewStyle().Foreground(styles.Subtle).Render("↑/↓ scroll")
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		heading,
		shortcuts,
		scrollHint,
		closeHint,
	)
	return modalStyle.Width(modalWidth).Render(body), maximum
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
	content := lipgloss.NewStyle().Foreground(styles.Warning).Render(fmt.Sprintf("Error: %v", m.Err))
	if m.width <= 0 || m.height <= 0 {
		return content
	}
	minimumColumn := m.columnStyle(false).GetVerticalFrameSize() + 3
	rows := min(3, max(1, m.height-m.appStyle().GetVerticalFrameSize()-lipgloss.Height(m.helpView())-minimumColumn))
	return fitLines(content, m.width-m.appStyle().GetHorizontalFrameSize(), rows)
}
