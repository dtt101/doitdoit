package model

import (
	"image/color"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dtt101/doitdoit/styles"
)

const (
	taskAnimationFrames = 14
	taskSweepFrames     = 9
)

type taskAnimationMsg struct {
	id    uint64
	frame int
}

func taskAnimationTick(id uint64, frame int) tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(time.Time) tea.Msg {
		return taskAnimationMsg{id: id, frame: frame}
	})
}

// Ease the accent back to the theme's selection colour without changing layout.
func (m Model) taskAccent() color.Color {
	return m.taskFade(styles.Highlight)
}

func (m Model) taskFade(base color.Color) color.Color {
	if m.taskFrame == 0 {
		return base
	}
	t := float64(taskAnimationFrames-m.taskFrame) / float64(taskAnimationFrames-1)
	weight := m.taskGlow * t * t
	r, g, b, _ := base.RGBA()
	sr, sg, sb, _ := styles.Special.RGBA()
	// Similar theme accents can make a hue-only fade invisible. Give the
	// peak some luminance contrast, darkening light colours and lifting dark
	// ones, while retaining a hint of the theme's special colour.
	contrast := 65535.0
	if 0.2126*float64(r)+0.7152*float64(g)+0.0722*float64(b) > 32767 {
		contrast = 0
	}
	blend := func(a, b uint32) uint8 {
		peak := 0.25*float64(b) + 0.75*contrast
		return uint8((float64(a)*(1-weight) + peak*weight) / 257)
	}
	return color.RGBA{R: blend(r, sr), G: blend(g, sg), B: blend(b, sb), A: 255}
}

// taskCheckbox returns a fixed 4-cell checkbox prefix, morphing briefly while
// a task completes so layout never shifts.
func taskCheckbox(completed bool, frame int, completing bool) string {
	if !completed {
		return "[ ] "
	}
	if completing && frame > 0 {
		if frame <= 2 {
			return "[·] "
		}
		if frame <= 6 {
			return "[✓] "
		}
	}
	return "[x] "
}

func sweepProgress(frame int) float64 {
	if frame <= 0 || frame >= taskSweepFrames {
		return 1
	}
	return float64(frame) / float64(taskSweepFrames)
}
