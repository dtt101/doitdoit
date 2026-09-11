package model

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/styles"
)

func (m Model) openNotes() (tea.Model, tea.Cmd) {
	if !m.hasSelectedTask() {
		return m, nil
	}
	m.notesKey, m.notesRow = m.getCurrentKey(), m.RowIdx
	m.notesInput = textarea.New()
	// Clipboard shortcuts belong to the terminal; accept its paste events.
	m.notesInput.KeyMap.Paste.SetEnabled(false)
	m.notesInput.Prompt = ""
	m.notesInput.ShowLineNumbers = false
	m.notesInput.Placeholder = "Words and links…"
	m.notesInput.MaxHeight, m.notesInput.MaxWidth = 0, 0
	m.notesInput.SetValue(m.Data[m.notesKey][m.notesRow].Notes)
	m.notesPreview, m.notesOffset = false, 0
	m.State = EditingNotes
	m.styleNotes()
	m.resizeNotes()
	return m, m.notesInput.Focus()
}

func (m *Model) resizeNotes() {
	m.notesInput.SetWidth(max(1, m.notesWidth()))
	m.notesInput.SetHeight(m.notesHeight())
}

func (m Model) notesPanelStyle() lipgloss.Style {
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(styles.Highlight).Padding(1, 2)
	if m.width > 0 && m.width < 40 || m.height > 0 && m.height < 16 {
		panel = panel.Padding(0, 1)
	}
	return panel
}

func (m Model) notesWidth() int {
	width := 60
	if m.width > 0 {
		width = min(width, m.width-4-m.notesPanelStyle().GetHorizontalFrameSize())
	}
	return max(1, width)
}

func (m Model) notesHeight() int {
	rows := max(3, lipgloss.Height(lipgloss.Wrap(plainNotes(m.notesInput.Value()), m.notesWidth(), "")))
	limit := 12
	if m.height > 0 {
		overhead := m.notesPanelStyle().GetVerticalFrameSize() + lipgloss.Height(m.notesHeading()) + lipgloss.Height(m.notesHelp()) + 2
		limit = min(limit, m.height-2-overhead)
	}
	return max(1, min(rows, limit))
}

func (m Model) updateNotes(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc":
			tasks := m.Data[m.notesKey]
			if tasks[m.notesRow].Notes == m.notesInput.Value() && m.saveErr == nil {
				m.notesInput.Blur()
				m.State = Browsing
				m.Err = nil
				return m, nil
			}
			if tasks[m.notesRow].Notes != m.notesInput.Value() {
				m.captureMoveUndo()
				tasks[m.notesRow].Notes = m.notesInput.Value()
			}
			m.feedback = ""
			m.persist()
			m.resizeNotes()
			if m.saveErr == nil {
				m.notesInput.Blur()
				m.State = Browsing
			}
			return m, nil
		case "tab":
			m.notesPreview = !m.notesPreview
			m.notesOffset = 0
			m.resizeNotes()
			if m.notesPreview {
				m.notesInput.Blur()
				return m, nil
			}
			return m, m.notesInput.Focus()
		}
	}
	if m.notesPreview {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "up", "k":
				m.notesOffset--
			case "down", "j":
				m.notesOffset++
			case "pgup":
				m.notesOffset -= m.notesHeight()
			case "pgdown":
				m.notesOffset += m.notesHeight()
			}
		}
		m.notesOffset = min(max(0, m.notesOffset), max(0, len(m.notesLines())-m.notesHeight()))
		return m, nil
	}
	if paste, ok := msg.(tea.PasteMsg); ok {
		m.notesInput.InsertString(plainNotes(paste.Content))
		m.resizeNotes()
		return m, nil
	}
	var cmd tea.Cmd
	m.notesInput, cmd = m.notesInput.Update(msg)
	m.resizeNotes()
	return m, cmd
}

// Strip terminal controls from displayed or pasted text, preserving paragraphs.
func plainNotes(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

var notesURL = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)

func linkedNotes(s string) string {
	return notesURL.ReplaceAllStringFunc(plainNotes(s), func(match string) string {
		target := strings.TrimRight(match, ".,;:!?")
		for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}} {
			for strings.HasSuffix(target, pair[1]) && strings.Count(target, pair[1]) > strings.Count(target, pair[0]) {
				target = strings.TrimSuffix(target, pair[1])
			}
		}
		u, err := url.Parse(target)
		if err != nil || u.Hostname() == "" {
			return match
		}
		return ansi.SetHyperlink(target) + lipgloss.NewStyle().Underline(true).Render(target) + ansi.ResetHyperlink() + match[len(target):]
	})
}

func (m Model) notesHeading() string {
	title := plainNotes(m.Data[m.notesKey][m.notesRow].Title)
	label := lipgloss.NewStyle().Foreground(styles.Highlight).Render("▤  Notes")
	return label + "\n" + lipgloss.NewStyle().Foreground(styles.Text).Bold(true).Render(fitLines(title, m.notesWidth(), 1))
}

func (m Model) notesHelp() string {
	mode := "tab links"
	if m.notesPreview {
		mode = "tab edit · ↑/↓ scroll"
	}
	text := "esc close · " + mode
	if m.Err != nil || m.saveErr != nil {
		err := m.Err
		if err == nil {
			err = m.saveErr
		}
		prefix := "Error"
		if m.saveErr != nil {
			prefix = "Not saved — draft kept in this session"
		}
		text = lipgloss.NewStyle().Foreground(styles.Warning).Render(fitLines(fmt.Sprintf("%s: %v", prefix, err), m.notesWidth(), 2)) + "\n" + text
	}
	return lipgloss.Wrap(text, m.notesWidth(), "")
}

func (m Model) notesLines() []string {
	return strings.Split(lipgloss.Wrap(linkedNotes(m.notesInput.Value()), m.notesWidth(), ""), "\n")
}

func (m Model) notesView() string {
	body := m.notesInput.View()
	if m.notesPreview {
		lines := m.notesLines()
		offset := min(max(0, m.notesOffset), max(0, len(lines)-m.notesHeight()))
		body = strings.Join(lines[offset:min(len(lines), offset+m.notesHeight())], "\n")
	}

	separator := lipgloss.NewStyle().Foreground(styles.Subtle).Render(strings.Repeat("─", m.notesWidth()))
	content := m.notesHeading() + "\n" + separator + "\n" +
		lipgloss.NewStyle().Height(m.notesHeight()).Render(body) + "\n" +
		lipgloss.NewStyle().Foreground(styles.Subtle).Render(m.notesHelp())
	return m.notesPanelStyle().Render(content)
}

func (m Model) notesOverlay() string {
	modal := m.notesView()
	if m.width <= 0 || m.height <= 0 {
		return modal
	}
	board := m
	board.State = Browsing
	board.ShowHelp = false
	background := lipgloss.NewStyle().Foreground(styles.Subtle).Render(ansi.Strip(board.View().Content))
	x := max(0, (m.width-lipgloss.Width(modal))/2)
	y := max(0, (m.height-lipgloss.Height(modal))/2)
	compositor := lipgloss.NewCompositor(
		lipgloss.NewLayer(background),
		lipgloss.NewLayer(modal).X(x).Y(y).Z(1),
	)
	return lipgloss.NewCanvas(m.width, m.height).Compose(compositor).Render()
}

func (m *Model) styleNotes() {
	s := m.notesInput.Styles()
	s.Focused.Text = lipgloss.NewStyle().Foreground(styles.Text)
	s.Focused.Placeholder = lipgloss.NewStyle().Foreground(styles.Subtle)
	s.Focused.CursorLine = lipgloss.NewStyle()
	s.Blurred = s.Focused
	m.notesInput.SetStyles(s)
}
