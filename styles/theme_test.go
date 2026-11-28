package styles

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"charm.land/lipgloss/v2"
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

func colorToRGBA(t *testing.T, c color.Color) color.RGBA {
	t.Helper()
	r, g, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func TestBuiltinThemes(t *testing.T) {
	names := BuiltinThemeNames()
	if len(names) == 0 {
		t.Fatal("no embedded themes found")
	}
	for _, name := range names {
		data, err := embeddedThemes.ReadFile("themes/" + name + ".toml")
		if err != nil {
			t.Errorf("read theme %q: %v", name, err)
			continue
		}
		palette := ParsePalette(data)
		theme, err := BuiltinTheme(name)
		if err != nil {
			t.Errorf("theme %q: %v", name, err)
			continue
		}
		if theme.Text == nil || theme.Highlight == nil {
			t.Errorf("theme %q has nil colors", name)
		}
		bg, ok := parseHexColor(palette["background"])
		if !ok {
			t.Errorf("theme %q has invalid background %q", name, palette["background"])
			continue
		}
		ratio := contrastRatio(colorToRGBA(t, theme.Subtle), bg)
		if ratio < minSubtleContrast {
			t.Errorf("theme %q Subtle contrast = %.2f against %s, want >= %.1f", name, ratio, palette["background"], minSubtleContrast)
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
	if theme.Subtle == lipgloss.Color("#565f89") || theme.Subtle == theme.Text {
		t.Errorf("Subtle = %v, want lifted above raw dark_foreground #565f89 while staying distinct from Text", theme.Subtle)
	}
	bg, _ := parseHexColor("#1a1b26")
	if got := contrastRatio(colorToRGBA(t, theme.Subtle), bg); got < minSubtleContrast {
		t.Errorf("Subtle contrast = %.2f, want >= %.1f", got, minSubtleContrast)
	}
	if theme.Border != lipgloss.Color("#414868") {
		t.Errorf("Border = %v, want the tokyo-night muted #414868", theme.Border)
	}
}

func TestBuiltinThemeMatteBlackSubtleContrast(t *testing.T) {
	theme, err := BuiltinTheme("matte-black")
	if err != nil {
		t.Fatal(err)
	}
	if theme.Subtle == lipgloss.Color("#555555") {
		t.Fatalf("matte-black Subtle should be lifted above #555555 for readability on #121212")
	}
	if theme.Subtle == theme.Text {
		t.Fatalf("matte-black Subtle should remain distinct from Text")
	}
	bg, _ := parseHexColor("#121212")
	if got := contrastRatio(colorToRGBA(t, theme.Subtle), bg); got < minSubtleContrast {
		t.Errorf("matte-black Subtle contrast = %.2f, want >= %.1f", got, minSubtleContrast)
	}
}

func TestBuiltinThemePreservesReadableDarkForeground(t *testing.T) {
	theme, err := BuiltinTheme("osaka-jade")
	if err != nil {
		t.Fatal(err)
	}
	if theme.Subtle != lipgloss.Color("#81B8A8") {
		t.Errorf("osaka-jade Subtle = %v, want untouched #81B8A8 (already >= %.1f contrast)", theme.Subtle, minSubtleContrast)
	}
}

func TestThemeFromPaletteSubtleFallsBackToMuted(t *testing.T) {
	// When dark_foreground is absent and muted is already readable, keep muted as-is.
	readable, err := ThemeFromPalette(map[string]string{
		"background": "#121212",
		"foreground": "#eeeeee",
		"muted":      "#888888",
		"accent":     "#7aa2f7",
		"red":        "#f7768e",
		"green":      "#9ece6a",
		"magenta":    "#ad8ee6",
	})
	if err != nil {
		t.Fatal(err)
	}
	if readable.Subtle != lipgloss.Color("#888888") {
		t.Errorf("Subtle = %v, want fallback to readable muted #888888", readable.Subtle)
	}

	// When muted is too dim against background, lift it to meet the contrast floor.
	dim, err := ThemeFromPalette(ParsePalette([]byte(sampleColors)))
	if err != nil {
		t.Fatal(err)
	}
	bg, _ := parseHexColor("#1a1b26")
	if got := contrastRatio(colorToRGBA(t, dim.Subtle), bg); got < minSubtleContrast {
		t.Errorf("lifted muted Subtle contrast = %.2f, want >= %.1f", got, minSubtleContrast)
	}
	if dim.Border != lipgloss.Color("#414868") {
		t.Errorf("Border = %v, want untouched muted #414868", dim.Border)
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
