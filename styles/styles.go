package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	Subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#787878"}
	Highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#AD8EE6"}
	Text      = lipgloss.AdaptiveColor{Light: "#191919", Dark: "#ECEFF4"}
	Special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	Warning   = lipgloss.AdaptiveColor{Light: "#F25D94", Dark: "#FF7AA8"}

	// Column Styles
	ColumnStyle = lipgloss.NewStyle().
			Border(lipgloss.HiddenBorder()).
			BorderForeground(Subtle).
			Padding(1, 1).
			Margin(0, 1).
			Width(30)

	FocusedColumnStyle = ColumnStyle.
				Border(lipgloss.RoundedBorder()).
				BorderForeground(Highlight)

	// Task Styles
	TaskStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			Foreground(Text)

	SelectedTaskStyle = TaskStyle.
				Foreground(Highlight).
				Bold(true)

	TitleStyle = lipgloss.NewStyle().
			Foreground(Special).
			Bold(true).
			PaddingBottom(1)

	HelpStyle = lipgloss.NewStyle().
			Foreground(Subtle).
			MarginTop(2)
)
