package model

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/styles"
)

const minColumnContentWidth = 24

func (m Model) terminalTooSmall() bool {
	return (m.width > 0 && m.width < 24) || (m.height > 0 && m.height < 10)
}

func (m Model) compactLayout() bool {
	return (m.width > 0 && m.width < 60) || (m.height > 0 && m.height < 20)
}

func (m Model) appStyle() lipgloss.Style {
	if m.compactLayout() {
		return styles.AppStyle.Margin(0)
	}
	return styles.AppStyle
}

func (m Model) columnStyle(focused bool) lipgloss.Style {
	style := styles.ColumnStyle
	if focused {
		style = styles.FocusedColumnStyle
	}
	if m.compactLayout() {
		style = style.Padding(0, 1).Margin(0)
	}
	return style
}

func (m Model) footerStyle() lipgloss.Style {
	if m.compactLayout() {
		return styles.HelpStyle.Margin(0)
	}
	return styles.HelpStyle
}

// Keep the logical date window intact: resizing must not redistribute tasks or
// change how far ahead scheduled tasks are stored in dated buckets.
func (m Model) visibleColumnCount() int {
	if m.ShowFuture || m.FocusToday {
		return 1
	}
	count := max(1, len(m.dateKeys))
	if m.width > 0 {
		available := m.width - m.appStyle().GetHorizontalFrameSize()
		minimum := minColumnContentWidth + m.columnStyle(false).GetHorizontalFrameSize()
		count = min(count, max(1, available/minimum))
	}
	return count
}

func (m Model) visibleColumnStart() int {
	count := m.visibleColumnCount()
	start := min(max(0, m.columnOffset), max(0, len(m.dateKeys)-count))
	if m.ColIdx < start {
		start = m.ColIdx
	} else if m.ColIdx >= start+count {
		start = m.ColIdx - count + 1
	}
	return max(0, start)
}

func (m Model) visibleKeys() []string {
	if m.ShowFuture {
		return []string{"Future"}
	}
	if len(m.dateKeys) == 0 {
		return nil
	}
	start := m.visibleColumnStart()
	return m.dateKeys[start:min(len(m.dateKeys), start+m.visibleColumnCount())]
}

type columnGeometry struct {
	width, height               int
	contentWidth, contentHeight int
}

func (m Model) columnGeometry(count int) columnGeometry {
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	style := m.columnStyle(false)
	blockWidth := (width-m.appStyle().GetHorizontalFrameSize())/max(1, count) - style.GetHorizontalMargins()
	blockHeight := height - m.appStyle().GetVerticalFrameSize() - lipgloss.Height(m.footerView())
	return columnGeometry{
		width: blockWidth, height: blockHeight,
		contentWidth:  max(1, blockWidth-style.GetHorizontalBorderSize()-style.GetHorizontalPadding()),
		contentHeight: max(1, blockHeight-style.GetVerticalBorderSize()-style.GetVerticalPadding()),
	}
}

func (m Model) inputPrefix() string {
	switch m.State {
	case Editing:
		return "Edit: "
	case SettingMoveDate:
		return "Move to: "
	default:
		return ""
	}
}

func (m *Model) resizeTextInput() {
	geometry := m.columnGeometry(m.visibleColumnCount())
	m.TextInput = m.sizedTextInput(geometry.contentWidth)
}

func (m Model) sizedTextInput(contentWidth int) textinput.Model {
	// Textinput renders an additional cell for its virtual cursor. Recalculate
	// overflow after a resize without changing the value or cursor position.
	input := m.TextInput
	width := max(1, contentWidth-lipgloss.Width(m.inputPrefix())-1)
	if input.Width() != width {
		position := input.Position()
		// SetWidth alone leaves the old horizontal viewport in place. Reset
		// its bounds before restoring the cursor in the newly sized viewport.
		input.SetWidth(0)
		input.SetCursor(position)
		input.SetWidth(width)
		input.CursorEnd()
		input.SetCursor(position)
	}
	return input
}

func (m *Model) syncViewport() {
	m.columnOffset = m.visibleColumnStart()
	m.resizeTextInput()
	if len(m.dateKeys) == 0 && !m.ShowFuture {
		return
	}
	geometry := m.columnGeometry(m.visibleColumnCount())
	key := m.getCurrentKey()
	doc := m.dayContent(key, m.ColIdx, geometry.contentWidth)
	rows := doc.viewportRows(geometry.contentHeight)
	if m.scrollOffsets == nil {
		m.scrollOffsets = make(map[string]int)
	}
	m.scrollOffsets[key] = doc.visibleOffset(m.scrollOffsets[key], rows, true)
	if m.ShowHelp {
		m.helpOffset = min(max(0, m.helpOffset), m.helpMaxOffset())
	}
}

func (m *Model) returnToToday() {
	wasToday := !m.ShowFuture && len(m.dateKeys) > m.ColIdx && m.dateKeys[m.ColIdx] == time.Now().Format(dateLayout)
	m.ShowFuture = false
	m.updateDateKeys()
	m.ColIdx, m.columnOffset = 0, 0
	if !wasToday {
		m.RowIdx = 0
	}
	m.clampRow()
}

type taskSpan struct {
	start, end int
}

type dayContent struct {
	header               string
	lines                []string
	tasks                []taskSpan
	focusStart, focusEnd int
}

func (d dayContent) viewportRows(height int) int {
	rows := max(1, height-lipgloss.Height(d.header))
	if len(d.lines) > rows && rows > 1 {
		rows-- // reserve an overflow indicator below the task viewport
	}
	return rows
}

func (d dayContent) visibleOffset(offset, rows int, focused bool) int {
	offset = min(max(0, offset), max(0, len(d.lines)-rows))
	if focused && d.focusStart >= 0 {
		if d.focusEnd-d.focusStart <= rows {
			if d.focusStart < offset {
				offset = d.focusStart
			} else if d.focusEnd > offset+rows {
				offset = d.focusEnd - rows
			}
		} else {
			// Oversized tasks can be paged through while remaining selected.
			if d.focusStart >= offset+rows || d.focusEnd <= offset {
				offset = d.focusStart
			}
			offset = max(offset, d.focusStart)
		}
	}
	return min(max(0, offset), max(0, len(d.lines)-rows))
}

func (m *Model) scrollPage(direction int) {
	geometry := m.columnGeometry(m.visibleColumnCount())
	key := m.getCurrentKey()
	doc := m.dayContent(key, m.ColIdx, geometry.contentWidth)
	rows := doc.viewportRows(geometry.contentHeight)
	offset := doc.visibleOffset(m.scrollOffsets[key], rows, true)
	offset = min(max(0, offset+direction*max(1, rows-1)), max(0, len(doc.lines)-rows))
	for i, span := range doc.tasks {
		if span.end > offset && span.start < offset+rows {
			m.RowIdx = i
			if direction < 0 {
				break
			}
		}
	}
	if m.scrollOffsets == nil {
		m.scrollOffsets = make(map[string]int)
	}
	m.scrollOffsets[key] = offset
}

// Bound unusually long footer messages; the final ellipsis makes truncation
// explicit. Essential state-specific controls are rendered separately.
func fitLines(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(lipgloss.Wrap(content, width, ""), "\n")
	if len(lines) > height {
		lines = lines[:height]
		lines[height-1] = ansi.Truncate(lines[height-1], max(0, width-1), "") + "…"
	}
	return strings.Join(lines, "\n")
}
