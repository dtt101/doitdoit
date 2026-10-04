package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/styles"
	"github.com/dtt101/doitdoit/taskstore"
)

func reviewDate(bucket string) string {
	if bucket == time.Now().Format(dateLayout) {
		return "Today"
	}
	return plainNotes(bucket)
}

func taskStatus(task taskstore.Task) string {
	if task.Completed {
		return "Done"
	}
	return "Not done"
}

func locatedText(task *taskstore.LocatedTask) string {
	if task == nil {
		return ""
	}
	text := plainNotes(task.Task.Title) + "\n" + reviewDate(task.Bucket) + " · " + taskStatus(task.Task)
	if task.Task.Notes != "" {
		text += "\n" + plainNotes(task.Task.Notes)
	}
	return text
}

func listText(data taskstore.Data, buckets []string) string {
	var lines []string
	for _, bucket := range buckets {
		lines = append(lines, reviewDate(bucket)+":")
		if len(data[bucket]) == 0 {
			lines = append(lines, "  No tasks")
		}
		for _, task := range data[bucket] {
			lines = append(lines, locatedText(&taskstore.LocatedTask{Bucket: bucket, Task: task}))
		}
	}
	return strings.Join(lines, "\n")
}

func reviewActions(d taskstore.Difference) (string, string) {
	switch d.Kind {
	case "add":
		return "Don't add", "Add task"
	case "remove":
		return "Keep task", "Delete task"
	case "order":
		return "Keep this order", "Change order"
	case "lists":
		return "Keep my tasks", "Use suggested list"
	}
	action := "Update task"
	if d.Current != nil && d.Alternative != nil {
		a, b := d.Current.Task, d.Alternative.Task
		if a.Completed != b.Completed {
			if b.Completed {
				action = "Mark done"
			} else {
				action = "Mark not done"
			}
		}
		if a.Title != b.Title || a.Notes != b.Notes || d.Current.Bucket != d.Alternative.Bucket || a.DueDate != b.DueDate {
			action = "Update task"
		}
	}
	return "Keep as is", action
}

// Describe only the fields that differ, so a status decision is readable without
// comparing two almost-identical task records or interpreting creation times.
func changedTaskText(d taskstore.Difference) string {
	if d.Current == nil || d.Alternative == nil {
		return ""
	}
	a, b := d.Current.Task, d.Alternative.Task
	lines := []string{plainNotes(a.Title)}
	if a.Title != b.Title {
		lines = append(lines, "Title: "+plainNotes(a.Title)+" → "+plainNotes(b.Title))
	}
	if a.Completed != b.Completed {
		lines = append(lines, "Status: "+taskStatus(a)+" → "+taskStatus(b))
	}
	if d.Current.Bucket != d.Alternative.Bucket {
		lines = append(lines, "When: "+reviewDate(d.Current.Bucket)+" → "+reviewDate(d.Alternative.Bucket))
	}
	if a.DueDate != b.DueDate {
		date := func(s string) string {
			if s == "" {
				return "No due date"
			}
			return plainNotes(s)
		}
		lines = append(lines, "Due date: "+date(a.DueDate)+" → "+date(b.DueDate))
	}
	if a.Notes != b.Notes {
		notes := func(s string) string {
			if s == "" {
				return "No notes"
			}
			return plainNotes(s)
		}
		lines = append(lines, "Notes now: "+notes(a.Notes), "Suggested notes: "+notes(b.Notes))
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		lines = append(lines, "Created: "+a.CreatedAt.Format(time.RFC3339)+" → "+b.CreatedAt.Format(time.RFC3339))
	}
	return strings.Join(lines, "\n")
}

func (m Model) reviewBody() string {
	r := m.review
	if r.discarding {
		return "Discard your unsaved changes?\nYour saved tasks will stay as they are.\ny discard / n cancel"
	}
	if r.previewing {
		if r.fullPreview {
			keys := make([]string, 0, len(r.preview))
			for key := range r.preview {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			return "Your tasks after saving:\n\n" + listText(r.preview, keys)
		}
		changes := taskstore.Compare(r.comparison.Current, r.preview, nil)
		lines := []string{"Here's what will change:"}
		if len(changes.Differences) == 0 {
			lines = append(lines, "Nothing — keep all your tasks as they are.")
		}
		for _, d := range changes.Differences {
			switch d.Kind {
			case "add":
				lines = append(lines, "\nAdd task:\n"+locatedText(d.Alternative))
			case "remove":
				lines = append(lines, "\nDelete task:\n"+locatedText(d.Current))
			case "change":
				lines = append(lines, "\nUpdate task:\n"+changedTaskText(d))
			case "order":
				lines = append(lines, "\nChange task order:\n"+listText(r.preview, d.Buckets))
			case "lists":
				lines = append(lines, "\nUse this task list:\n"+listText(r.preview, d.Buckets))
			}
		}
		before, _ := r.comparison.Resolve()
		lifecycle := taskstore.Compare(r.preview, before, nil)
		for _, d := range lifecycle.Differences {
			if d.Kind == "add" {
				lines = append(lines, "\nYour history setting will also remove:\n"+locatedText(d.Alternative))
			} else if d.Kind == "change" {
				lines = append(lines, "\nCarried forward to Today:\n"+locatedText(d.Current))
			}
		}
		lines = append(lines, "\nEverything else stays as it is.", "Enter saves your choices and finishes the cleanup.")
		return strings.Join(lines, "\n")
	}
	return ansi.Strip(m.reviewCardBody(60))
}

// The review uses the same bounded panel and dimmed board as Notes. Choices and
// navigation stay outside the scrolling task details.
func (m Model) reviewCardBody(width int) string {
	r := m.review
	text := lipgloss.NewStyle().Foreground(styles.Text)
	muted := lipgloss.NewStyle().Foreground(styles.Subtle)
	if r.discarding {
		return text.Render(lipgloss.Wrap("Discard your unsaved changes?\nYour saved tasks will stay as they are.", width, ""))
	}
	if r.moreHelp {
		return muted.Render(lipgloss.Wrap("↑/↓  previous / next task\n1/2  choose an action\np    preview your choices\nK    keep everything as is\nPgUp/PgDn  scroll details\nv    all tasks in preview\nb    before your edits\nx    save a recovery copy\nD    discard unsaved changes\nq    quit\n?    back to review", width, ""))
	}
	if r.previewing {
		body := m.reviewBody()
		body = strings.TrimPrefix(body, "Here's what will change:\n")
		body = strings.TrimSuffix(body, "\nEverything else stays as it is.\nEnter saves your choices and finishes the cleanup.")
		var lines []string
		for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
			style := text
			if strings.HasSuffix(line, ":") {
				style = lipgloss.NewStyle().Foreground(styles.Highlight)
			}
			lines = append(lines, style.Render(lipgloss.Wrap(line, width, "")))
		}
		return strings.Join(lines, "\n")
	}
	if len(r.comparison.Differences) == 0 {
		return muted.Render("Your tasks are up to date.")
	}
	d := r.comparison.Differences[r.index]
	label := map[string]string{"add": "Missing task", "change": "Task changed", "remove": "Keep or delete", "order": "Task order", "lists": "Review task list"}[d.Kind]
	lines := []string{muted.Render(label), ""}
	if d.Kind == "add" || d.Kind == "remove" {
		task := d.Alternative
		if task == nil {
			task = d.Current
		}
		lines = append(lines, styles.SelectedTaskStyle.Render(lipgloss.Wrap(plainNotes(task.Task.Title), width, "")), muted.Render(reviewDate(task.Bucket)+" · "+taskStatus(task.Task)))
		if task.Task.Notes != "" {
			lines = append(lines, "", text.Render(lipgloss.Wrap(plainNotes(task.Task.Notes), width, "")))
		}
	} else if d.Kind == "change" {
		detail := strings.SplitN(changedTaskText(d), "\n", 2)
		lines = append(lines, styles.SelectedTaskStyle.Render(lipgloss.Wrap(detail[0], width, "")))
		if len(detail) > 1 {
			lines = append(lines, "", text.Render(lipgloss.Wrap(detail[1], width, "")))
		}
	} else {
		if d.Kind == "lists" {
			lines = append(lines, muted.Render(lipgloss.Wrap("Choose a complete list; these tasks couldn't be matched reliably.", width, "")), "")
		}
		lines = append(lines, muted.Render("Now"), text.Render(lipgloss.Wrap(listText(r.comparison.Current, d.Buckets), width, "")), "", muted.Render("Suggested"), text.Render(lipgloss.Wrap(listText(r.comparison.Alternative, d.Buckets), width, "")))
	}
	if r.original && r.source == nil {
		original := locatedText(d.Original)
		if d.Kind == "lists" || d.Kind == "order" {
			original = listText(r.comparison.Base, d.Buckets)
		}
		if original == "" {
			original = "Not present before your edits."
		}
		lines = append(lines, "", muted.Render("Before your edits"), text.Render(lipgloss.Wrap(original, width, "")))
	}
	return strings.Join(lines, "\n")
}

func reviewShortcut(key, label string) string {
	return styles.KeyStyle.Render(key) + " " + lipgloss.NewStyle().Foreground(styles.Subtle).Render(label)
}

func (m Model) renderConflictPanel() (string, int) {
	r := m.review
	panel := m.notesPanelStyle()
	if m.compactLayout() {
		panel = panel.Padding(0, 1)
	}
	width := 64
	if m.width > 0 {
		width = min(width, m.width-4)
	}
	width = max(panel.GetHorizontalFrameSize()+1, width)
	inner := width - panel.GetHorizontalFrameSize()
	label := "Review tasks"
	count := ""
	if len(r.comparison.Differences) > 0 {
		count = fmt.Sprintf("%d/%d", r.index+1, len(r.comparison.Differences))
	}
	if r.previewing {
		label = "Save changes"
		count = ""
	}
	if r.moreHelp {
		label = "Review shortcuts"
		count = ""
	}
	if r.discarding {
		label = "Discard changes?"
		count = ""
	}
	heading := lipgloss.NewStyle().Foreground(styles.Highlight).Render(ansi.Truncate(label, max(1, inner-len(count)-1), "…"))
	heading += strings.Repeat(" ", max(0, inner-lipgloss.Width(heading)-len(count))) + lipgloss.NewStyle().Foreground(styles.Subtle).Render(count)
	separator := lipgloss.NewStyle().Foreground(styles.Subtle).Render(strings.Repeat("─", inner))
	actions := ""
	if len(r.comparison.Differences) > 0 && !r.previewing && !r.discarding && !r.moreHelp {
		d := r.comparison.Differences[r.index]
		first, second := reviewActions(d)
		choice := func(key, label string, selected bool) string {
			if selected {
				return styles.SelectedTaskStyle.Render(key + " " + label + " ✓")
			}
			return styles.KeyStyle.Render(key) + " " + styles.TaskStyle.Render(label)
		}
		a, b := choice("1", first, d.Choice == taskstore.UseCurrent), choice("2", second, d.Choice == taskstore.UseAlternative)
		actions = a + "    " + b
		if lipgloss.Width(actions) > inner {
			actions = a + "\n" + b
		}
		actions = lipgloss.Wrap(actions, inner, "")
	}
	hints := []string{reviewShortcut("↑↓", "task"), reviewShortcut("p", "preview"), reviewShortcut("esc", "later"), reviewShortcut("?", "help")}
	if r.previewing {
		actions = reviewShortcut("enter", "save & finish")
		hints = []string{reviewShortcut("esc", "back"), reviewShortcut("↑↓", "scroll"), reviewShortcut("v", "all tasks")}
	}
	if r.discarding {
		actions = reviewShortcut("y", "discard") + "    " + reviewShortcut("n", "cancel")
		hints = nil
	}
	if r.moreHelp {
		hints = []string{reviewShortcut("?", "back"), reviewShortcut("PgDn", "scroll")}
	}
	help := wrapFooterItems("", hints, inner)
	if m.height > 0 && m.height < 16 {
		help = reviewShortcut("p", "preview") + "  " + styles.KeyStyle.Render("esc")
		if r.previewing {
			actions = reviewShortcut("enter", "save")
			help = reviewShortcut("esc", "back") + "  " + reviewShortcut("v", "all")
		}
		if r.discarding {
			help = ""
		}
		if r.moreHelp {
			help = reviewShortcut("?", "back")
		}
	}
	message := r.message
	if m.recoveryErr != nil {
		message = "Couldn't protect your changes: " + m.recoveryErr.Error()
	}
	if message != "" {
		message = lipgloss.NewStyle().Foreground(styles.Warning).Render(fitLines(plainNotes(message), inner, 2))
	}
	// Only detail text scrolls; progress, decisions and shortcuts stay visible.
	footer := actions
	if message != "" {
		if footer != "" {
			footer += "\n"
		}
		footer += message
	}
	if help != "" {
		if footer != "" {
			footer += "\n"
		}
		footer += help
	}
	footer = lipgloss.Wrap(footer, inner, "")
	gap := "\n\n"
	if m.height > 0 && m.height < 16 {
		gap = "\n"
	}
	maxHeight := 24
	if m.height > 0 {
		maxHeight = max(1, m.height-2)
		if m.height < 16 {
			maxHeight = m.height
		}
	}
	rows := max(1, maxHeight-panel.GetVerticalFrameSize()-2-strings.Count(gap, "\n")-lipgloss.Height(footer))
	content := m.reviewCardBody(inner)
	if m.height > 0 && m.height < 16 {
		content = strings.ReplaceAll(content, "\n\n", "\n")
	}
	lines := strings.Split(content, "\n")
	rows = min(rows, len(lines))
	maximum := max(0, len(lines)-rows)
	offset := min(max(0, r.offset), maximum)
	body := strings.Join(lines[offset:offset+rows], "\n")
	if maximum > 0 && inner >= 30 {
		heading = lipgloss.NewStyle().Foreground(styles.Highlight).Render(label) + lipgloss.NewStyle().Foreground(styles.Subtle).Render(" · PgDn scroll")
		heading = fitLines(heading, max(1, inner-len(count)-1), 1)
		heading += strings.Repeat(" ", max(0, inner-lipgloss.Width(heading)-len(count))) + lipgloss.NewStyle().Foreground(styles.Subtle).Render(count)
	}
	return panel.Width(width).Render(heading + "\n" + separator + "\n" + body + gap + footer), maximum
}

func (m Model) conflictView() string {
	panel, _ := m.renderConflictPanel()
	return panel
}

func (m Model) conflictOverlay() string {
	modal := m.conflictView()
	if m.width <= 0 || m.height <= 0 {
		return modal
	}
	board := m
	board.review = nil
	board.State = Browsing
	board.ShowHelp = false
	background := lipgloss.NewStyle().Foreground(styles.Subtle).Render(ansi.Strip(board.View().Content))
	x := max(0, (m.width-lipgloss.Width(modal))/2)
	y := max(0, (m.height-lipgloss.Height(modal))/2)
	compositor := lipgloss.NewCompositor(lipgloss.NewLayer(background), lipgloss.NewLayer(modal).X(x).Y(y).Z(1))
	return lipgloss.NewCanvas(m.width, m.height).Compose(compositor).Render()
}
