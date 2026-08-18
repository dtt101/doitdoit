package styles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestParsePalette(t *testing.T) {
	data := []byte(`mode = "dark"

# a comment
accent = "#7aa2f7"
muted = '#414868'
hyprland_active_border = "rgba(26a269ee) rgba(2ec27eee) 45deg"
not-a-pair
`)
	palette := ParsePalette(data)
	if palette["accent"] != "#7aa2f7" {
		t.Errorf("accent = %q, want #7aa2f7", palette["accent"])
	}
	if palette["muted"] != "#414868" {
		t.Errorf("muted = %q, want #414868", palette["muted"])
	}
	if palette["mode"] != "dark" {
		t.Errorf("mode = %q, want dark", palette["mode"])
	}
	if _, ok := palette["not-a-pair"]; ok {
		t.Error("malformed line should be skipped")
	}
}

func TestThemeFromPaletteMissingKey(t *testing.T) {
	if _, err := ThemeFromPalette(map[string]string{"foreground": "#fff"}); err == nil {
		t.Error("expected error for incomplete palette")
	}
}

func TestBuiltinThemes(t *testing.T) {
	names := BuiltinThemeNames()
	if len(names) == 0 {
		t.Fatal("no embedded themes found")
	}
	for _, name := range names {
		theme, err := BuiltinTheme(name)
		if err != nil {
			t.Errorf("theme %q: %v", name, err)
			continue
		}
		if theme.Text == nil || theme.Highlight == nil {
			t.Errorf("theme %q has nil colors", name)
		}
	}
}

func TestBuiltinThemeUnknown(t *testing.T) {
	if _, err := BuiltinTheme("no-such-theme"); err == nil {
		t.Error("expected error for unknown theme")
	}
}

func TestBuiltinThemeTokyoNight(t *testing.T) {
	theme, err := BuiltinTheme("tokyo-night")
	if err != nil {
		t.Fatal(err)
	}
	if theme.Highlight != lipgloss.Color("#7aa2f7") {
		t.Errorf("Highlight = %v, want the tokyo-night accent #7aa2f7", theme.Highlight)
	}
	if theme.Text != lipgloss.Color("#a9b1d6") {
		t.Errorf("Text = %v, want the tokyo-night foreground #a9b1d6", theme.Text)
	}
	if theme.Subtle != lipgloss.Color("#565f89") {
		t.Errorf("Subtle = %v, want the tokyo-night dark_foreground #565f89", theme.Subtle)
	}
	if theme.Border != lipgloss.Color("#414868") {
		t.Errorf("Border = %v, want the tokyo-night muted #414868", theme.Border)
	}
}

func TestThemeFromPaletteSubtleFallsBackToMuted(t *testing.T) {
	theme, err := ThemeFromPalette(ParsePalette([]byte(sampleColors)))
	if err != nil {
		t.Fatal(err)
	}
	if theme.Subtle != lipgloss.Color("#414868") {
		t.Errorf("Subtle = %v, want fallback to muted #414868", theme.Subtle)
	}
}

// writeOmarchyColors creates a fake Omarchy current-theme state under a temp
// HOME and returns that home dir.
func writeOmarchyColors(t *testing.T, contents string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".local", "state", "omarchy", "current", "theme")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "colors.toml"), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	return home
}

const sampleColors = `mode = "dark"
accent = "#7aa2f7"
muted = "#414868"
background = "#1a1b26"
foreground = "#a9b1d6"
red = "#f7768e"
green = "#9ece6a"
magenta = "#ad8ee6"
`

func TestOmarchyTheme(t *testing.T) {
	writeOmarchyColors(t, sampleColors)

	if !OmarchyAvailable() {
		t.Fatal("OmarchyAvailable() = false, want true")
	}
	theme, err := OmarchyTheme()
	if err != nil {
		t.Fatal(err)
	}
	if theme.Highlight != lipgloss.Color("#7aa2f7") {
		t.Errorf("Highlight = %v, want #7aa2f7", theme.Highlight)
	}
	if theme.MovingBg != lipgloss.Color("#7aa2f7") {
		t.Errorf("MovingBg = %v, want #7aa2f7", theme.MovingBg)
	}
	if theme.MovingFg != lipgloss.Color("#1a1b26") {
		t.Errorf("MovingFg = %v, want #1a1b26", theme.MovingFg)
	}
}

func TestOmarchyThemeMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if OmarchyAvailable() {
		t.Error("OmarchyAvailable() = true, want false")
	}
	if _, err := OmarchyTheme(); err == nil {
		t.Error("expected error when no Omarchy state exists")
	}
}

func TestResolveTheme(t *testing.T) {
	writeOmarchyColors(t, sampleColors)

	// Auto follows Omarchy when present.
	theme, err := ResolveTheme("")
	if err != nil {
		t.Fatal(err)
	}
	if theme.Highlight != lipgloss.Color("#7aa2f7") {
		t.Errorf("auto Highlight = %v, want the omarchy accent", theme.Highlight)
	}

	// Explicit names resolve too.
	if _, err := ResolveTheme(ThemeNameSystem); err != nil {
		t.Errorf("system: %v", err)
	}
	if _, err := ResolveTheme("nord"); err != nil {
		t.Errorf("nord: %v", err)
	}
	if _, err := ResolveTheme("bogus"); err == nil {
		t.Error("expected error for unknown theme name")
	}
}

func TestResolveThemeSystemWithoutOmarchy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"", ThemeNameSystem} {
		theme, err := ResolveTheme(name)
		if err != nil {
			t.Fatal(err)
		}
		if theme != DefaultTheme() {
			t.Errorf("ResolveTheme(%q) without omarchy should be the default theme", name)
		}
	}
}

func TestValidThemeName(t *testing.T) {
	for _, name := range []string{"system", "tokyo-night", "nord"} {
		if !ValidThemeName(name) {
			t.Errorf("ValidThemeName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "bogus", "Tokyo Night", "omarchy", "default"} {
		if ValidThemeName(name) {
			t.Errorf("ValidThemeName(%q) = true, want false", name)
		}
	}
}
