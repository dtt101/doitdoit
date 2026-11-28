package styles

import (
	"embed"
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Theme holds the colors used to build the app styles. The fields map onto
// the roles the UI needs rather than a full terminal palette.
type Theme struct {
	Text      color.Color // regular task text
	Subtle    color.Color // completed tasks, help text
	Border    color.Color // unfocused column borders
	Highlight color.Color // selection, focused column
	Key       color.Color // key hints in the help bar
	Special   color.Color // day titles
	Warning   color.Color // errors
	MovingFg  color.Color // task being moved (foreground)
	MovingBg  color.Color // task being moved (background)
}

// DefaultTheme is the original adaptive palette, used when no theme is
// configured and no Omarchy install is present.
func DefaultTheme() Theme {
	return Theme{
		Text:      compat.AdaptiveColor{Light: lipgloss.Color("#191919"), Dark: lipgloss.Color("#F8F8F2")},
		Subtle:    compat.AdaptiveColor{Light: lipgloss.Color("#62685A"), Dark: lipgloss.Color("#6272A4")},
		Border:    compat.AdaptiveColor{Light: lipgloss.Color("#D9DCCF"), Dark: lipgloss.Color("#6272A4")},
		Highlight: compat.AdaptiveColor{Light: lipgloss.Color("#874BFD"), Dark: lipgloss.Color("#FF79C6")},
		Key:       compat.AdaptiveColor{Light: lipgloss.Color("#9B9B9B"), Dark: lipgloss.Color("#BD93F9")},
		Special:   compat.AdaptiveColor{Light: lipgloss.Color("#43BF6D"), Dark: lipgloss.Color("#50FA7B")},
		Warning:   compat.AdaptiveColor{Light: lipgloss.Color("#F25D94"), Dark: lipgloss.Color("#FF5555")},
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
	// dim *text* colour is `dark_foreground`. Some palettes set
	// `dark_foreground` too close to `background` for comfortable reading
	// (e.g. matte-black's #555555 on #121212), so lift it toward `foreground`
	// just enough to meet WCAG AA text contrast while keeping `muted` for
	// unfocused borders.
	subtle := palette["dark_foreground"]
	if subtle == "" {
		subtle = palette["muted"]
	}
	subtle = ensureSubtleContrast(subtle, palette["foreground"], palette["background"], minSubtleContrast)
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

const minSubtleContrast = 4.5 // WCAG AA contrast ratio for normal text

// ensureSubtleContrast blends subtleHex toward fgHex when its contrast against
// bgHex falls below minRatio, preserving the theme's hue while keeping subtle
// text legible on dark and light backgrounds alike.
func ensureSubtleContrast(subtleHex, fgHex, bgHex string, minRatio float64) string {
	subtle, okS := parseHexColor(subtleHex)
	fg, okF := parseHexColor(fgHex)
	bg, okB := parseHexColor(bgHex)
	if !okS || !okF || !okB {
		return subtleHex
	}
	if contrastRatio(subtle, bg) >= minRatio {
		return subtleHex
	}
	target := min(minRatio, contrastRatio(fg, bg))
	if contrastRatio(subtle, bg) >= target {
		return subtleHex
	}
	lo, hi := 0.0, 1.0
	for range 20 {
		mid := (lo + hi) / 2
		if contrastRatio(blendRGBA(subtle, fg, mid), bg) < target {
			lo = mid
		} else {
			hi = mid
		}
	}
	lifted := blendRGBA(subtle, fg, hi)
	return fmt.Sprintf("#%02x%02x%02x", lifted.R, lifted.G, lifted.B)
}

func parseHexColor(s string) (color.RGBA, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	switch len(s) {
	case 3:
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	case 6:
	default:
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
		A: 255,
	}, true
}

func blendRGBA(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8) uint8 {
		v := math.Round(float64(x) + (float64(y)-float64(x))*t)
		return uint8(max(0, min(255, v)))
	}
	return color.RGBA{
		R: lerp(a.R, b.R),
		G: lerp(a.G, b.G),
		B: lerp(a.B, b.B),
		A: 255,
	}
}

func contrastRatio(a, b color.RGBA) float64 {
	l1 := relativeLuminance(a)
	l2 := relativeLuminance(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func relativeLuminance(c color.RGBA) float64 {
	linearize := func(v uint8) float64 {
		s := float64(v) / 255.0
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*linearize(c.R) + 0.7152*linearize(c.G) + 0.0722*linearize(c.B)
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
