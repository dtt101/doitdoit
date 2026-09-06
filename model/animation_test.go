package model

import (
	"image/color"
	"testing"

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
