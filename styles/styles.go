package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

var (
	// Colors
	Subtle    color.Color
	Highlight color.Color
	KeyColor  color.Color
	Text      color.Color
	Special   color.Color
	Warning   color.Color

	// Column Styles
	ColumnStyle        lipgloss.Style
	FocusedColumnStyle lipgloss.Style

	// Task Styles
	TaskStyle          lipgloss.Style
	SelectedTaskStyle  lipgloss.Style
	CompletedTaskStyle lipgloss.Style
	MovingTaskStyle    lipgloss.Style

	TitleStyle lipgloss.Style

	HelpStyle lipgloss.Style
	KeyStyle  lipgloss.Style
	AppStyle  lipgloss.Style
)

func init() {
	Apply(DefaultTheme())
}

// Apply rebuilds every style from the given theme. Call it before the model
// is created; styles are read at render time so the whole UI follows.
func Apply(t Theme) {
	Subtle = t.Subtle
	Highlight = t.Highlight
	KeyColor = t.Key
	Text = t.Text
	Special = t.Special
	Warning = t.Warning

	ColumnStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(1, 1).
		Margin(0, 1).
		Width(30)

	FocusedColumnStyle = ColumnStyle.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Highlight)

	TaskStyle = lipgloss.NewStyle().
		Foreground(Text)

	SelectedTaskStyle = TaskStyle.
		Foreground(Highlight).
		Bold(true)

	CompletedTaskStyle = TaskStyle.
		Foreground(Subtle).
		Strikethrough(true)

	MovingTaskStyle = lipgloss.NewStyle().
		Foreground(t.MovingFg).
		Background(t.MovingBg).
		Bold(true).
		Padding(0, 1)

	TitleStyle = lipgloss.NewStyle().
		Foreground(Special).
		Bold(true).
		PaddingBottom(1)

	HelpStyle = lipgloss.NewStyle().
		Foreground(Subtle).
		MarginTop(2).
		MarginLeft(1)

	KeyStyle = lipgloss.NewStyle().
		Foreground(KeyColor).
		Bold(true)

	AppStyle = lipgloss.NewStyle().
		Margin(1, 2)
}
