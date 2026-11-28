package model

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/styles"
)

func TestTaskFadeVisibleWithIdenticalThemeAccents(t *testing.T) {
	originalHighlight, originalSpecial := styles.Highlight, styles.Special
	t.Cleanup(func() { styles.Highlight, styles.Special = originalHighlight, originalSpecial })
	for _, level := range []uint8{40, 128, 220} {
		styles.Highlight = color.RGBA{R: level, G: level, B: level, A: 255}
		styles.Special = styles.Highlight
		m := Model{taskFrame: 1, taskGlow: 0.65}
		first, _, _, _ := m.taskAccent().RGBA()
		base, _, _, _ := styles.Highlight.RGBA()
		delta := int(first/257) - int(base/257)
		if delta > -40 && delta < 40 {
			t.Fatalf("accent %d: pulse has too little brightness contrast: %d", level, delta)
		}
		m.taskFrame = taskAnimationFrames
		last, _, _, _ := m.taskAccent().RGBA()
		if last != base {
			t.Fatal("last frame should return to the theme accent")
		}
	}
}

func TestTaskAnimationSettlesAndIgnoresOldTicks(t *testing.T) {
	m := newFeedbackTestModel(t)
	m = pressRune(m, 'j')
	if m.taskFrame != 1 {
		t.Fatal("navigation should start a selection fade")
	}
	oldID := m.taskAnimationID
	m = pressRune(m, 'k')
	updated, cmd := m.Update(taskAnimationMsg{id: oldID, frame: 5})
	m = updated.(Model)
	if m.taskFrame != 1 || cmd != nil {
		t.Fatal("an old timer disturbed the new animation")
	}
	task := m.Data[m.getCurrentKey()][m.RowIdx]
	plain := ansi.Strip(m.taskView(task, true, 24))
	for frame := 2; frame <= taskAnimationFrames+1; frame++ {
		updated, cmd = m.Update(taskAnimationMsg{id: m.taskAnimationID, frame: frame})
		m = updated.(Model)
		if ansi.Strip(m.taskView(task, true, 24)) != plain {
			t.Fatal("animation changed the task text or layout")
		}
	}
	if m.taskFrame != 0 || cmd != nil || m.taskAccent() != styles.Highlight {
		t.Fatal("animation should settle to the normal highlight and stop ticking")
	}
}

func TestCompletionGlowAndEditingCancellation(t *testing.T) {
	m := newFeedbackTestModel(t)
	m = pressSpace(m)
	if m.taskFrame != 1 || m.taskGlow != 0.8 {
		t.Fatal("completion should start a stronger glow")
	}
	id := m.taskAnimationID
	m = pressRune(m, 'e')
	updated, cmd := m.Update(taskAnimationMsg{id: id, frame: 2})
	m = updated.(Model)
	if m.taskFrame != 0 || cmd != nil {
		t.Fatal("editing should cancel the task animation")
	}
}

func TestCompletionAnimatesCheckboxAndStrikethroughSweep(t *testing.T) {
	m := newFeedbackTestModel(t)
	key := m.getCurrentKey()
	m.Data[key][0].Title = "Ship animated checkbox"
	m.Data[key][0].Notes = "note body"

	m = pressSpace(m)
	if !m.taskCompleting || m.taskFrame != 1 {
		t.Fatalf("expected completion animation to start, got completing=%v frame=%d", m.taskCompleting, m.taskFrame)
	}

	var completed Task
	for _, task := range m.Data[key] {
		if task.ID == m.taskAnimationTaskID {
			completed = task
			break
		}
	}
	if !completed.Completed {
		t.Fatal("animated task should be marked completed")
	}

	frame1 := m.taskView(completed, false, 30)
	if !strings.HasPrefix(ansi.Strip(frame1), "[·] ") {
		t.Fatalf("frame 1 checkbox = %q, want [·] prefix", ansi.Strip(frame1))
	}
	if !strings.Contains(frame1, ";9m") && !strings.Contains(frame1, "[9m") && !strings.Contains(frame1, "[9;") {
		t.Fatalf("frame 1 should include strikethrough on swept prefix: %q", frame1)
	}

	// Mid-sweep: checkmark glyph and preserved text + note icon without shifting layout.
	m.taskFrame = 4
	frame4 := m.taskView(completed, false, 30)
	plain4 := ansi.Strip(frame4)
	if !strings.HasPrefix(plain4, "[✓] ") {
		t.Fatalf("frame 4 checkbox = %q, want [✓] prefix", plain4)
	}
	if !strings.Contains(plain4, "Ship animated checkbox") || !strings.Contains(plain4, "▤") {
		t.Fatalf("frame 4 lost title or note icon: %q", plain4)
	}

	// Advance to completion of animation.
	for frame := 5; frame <= taskAnimationFrames+1; frame++ {
		updated, _ := m.Update(taskAnimationMsg{id: m.taskAnimationID, frame: frame})
		m = updated.(Model)
	}
	if m.taskFrame != 0 || m.taskCompleting {
		t.Fatalf("completion animation did not settle: frame=%d completing=%v", m.taskFrame, m.taskCompleting)
	}
	settled := ansi.Strip(m.taskView(completed, false, 30))
	if !strings.HasPrefix(settled, "[x] ") {
		t.Fatalf("settled checkbox = %q, want [x] prefix", settled)
	}
	if lipgloss.Height(frame1) != lipgloss.Height(settled) || lipgloss.Height(frame4) != lipgloss.Height(settled) {
		t.Fatalf("sweep changed line count: frame1=%d (%q) frame4=%d (%q) settled=%d (%q)",
			lipgloss.Height(frame1), ansi.Strip(frame1),
			lipgloss.Height(frame4), ansi.Strip(frame4),
			lipgloss.Height(settled), settled)
	}
}
