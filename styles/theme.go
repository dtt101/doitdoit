package styles

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme holds the colors used to build the app styles. The fields map onto
// the roles the UI needs rather than a full terminal palette.
type Theme struct {
	Text      lipgloss.TerminalColor // regular task text
	Subtle    lipgloss.TerminalColor // completed tasks, help text
	Border    lipgloss.TerminalColor // unfocused column borders
	Highlight lipgloss.TerminalColor // selection, focused column
	Key       lipgloss.TerminalColor // key hints in the help bar
	Special   lipgloss.TerminalColor // day titles
	Warning   lipgloss.TerminalColor // errors
	MovingFg  lipgloss.TerminalColor // task being moved (foreground)
	MovingBg  lipgloss.TerminalColor // task being moved (background)
}

// DefaultTheme is the original adaptive palette, used when no theme is
// configured and no Omarchy install is present.
func DefaultTheme() Theme {
	return Theme{
		Text:      lipgloss.AdaptiveColor{Light: "#191919", Dark: "#F8F8F2"},
		Subtle:    lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#6272A4"},
		Border:    lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#6272A4"},
		Highlight: lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#FF79C6"},
		Key:       lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#BD93F9"},
		Special:   lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#50FA7B"},
		Warning:   lipgloss.AdaptiveColor{Light: "#F25D94", Dark: "#FF5555"},
		MovingFg:  lipgloss.Color("#FFFFFF"),
		MovingBg:  lipgloss.Color("#FF79C6"),
	}
}

//go:embed themes/*.toml
var embeddedThemes embed.FS

// ParsePalette reads an Omarchy colors.toml: a flat file of `key = "value"`
// pairs. Unknown keys are kept but ignored by the theme mapping, so extra
// entries like hyprland_active_border are harmless.
func ParsePalette(data []byte) map[string]string {
	palette := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" && value != "" {
			palette[key] = value
		}
	}
	return palette
}

// ThemeFromPalette maps an Omarchy palette onto the app's color roles.
// It errors if any required key is missing so a broken colors.toml falls
// back to the default theme instead of rendering half-styled.
func ThemeFromPalette(palette map[string]string) (Theme, error) {
	required := []string{"foreground", "muted", "accent", "magenta", "green", "red", "background"}
	for _, key := range required {
		if palette[key] == "" {
			return Theme{}, fmt.Errorf("palette is missing %q", key)
		}
	}
	// Omarchy's `muted` is a background-tier grey (selections, borders); the
	// dim *text* colour is `dark_foreground`. Use it for subtle text so
	// completed tasks and help stay readable, keeping `muted` for borders.
	subtle := palette["dark_foreground"]
	if subtle == "" {
		subtle = palette["muted"]
	}
	return Theme{
		Text:      lipgloss.Color(palette["foreground"]),
		Subtle:    lipgloss.Color(subtle),
		Border:    lipgloss.Color(palette["muted"]),
		Highlight: lipgloss.Color(palette["accent"]),
		Key:       lipgloss.Color(palette["magenta"]),
		Special:   lipgloss.Color(palette["green"]),
		Warning:   lipgloss.Color(palette["red"]),
		MovingFg:  lipgloss.Color(palette["background"]),
		MovingBg:  lipgloss.Color(palette["accent"]),
	}, nil
}

// BuiltinThemeNames lists the embedded Omarchy stock palettes, sorted.
func BuiltinThemeNames() []string {
	entries, err := fs.ReadDir(embeddedThemes, "themes")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, strings.TrimSuffix(entry.Name(), ".toml"))
	}
	sort.Strings(names)
	return names
}

// BuiltinTheme loads one of the embedded Omarchy stock palettes by name.
func BuiltinTheme(name string) (Theme, error) {
	data, err := embeddedThemes.ReadFile("themes/" + name + ".toml")
	if err != nil {
		return Theme{}, fmt.Errorf("unknown theme %q", name)
	}
	return ThemeFromPalette(ParsePalette(data))
}
