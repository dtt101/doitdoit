package styles

import (
	"fmt"
	"os"
	"path/filepath"
)

// ThemeNameOmarchy follows the live Omarchy theme instead of a fixed palette.
const ThemeNameOmarchy = "omarchy"

// ThemeNameDefault forces the original built-in palette.
const ThemeNameDefault = "default"

// OmarchyColorsPath returns the colors.toml of the currently applied Omarchy
// theme. Since Omarchy 4 (Quattro) the generated current-theme state lives
// under ~/.local/state/omarchy/current/theme/.
func OmarchyColorsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "omarchy", "current", "theme", "colors.toml"), nil
}

// OmarchyTheme loads the palette of the currently applied Omarchy theme.
func OmarchyTheme() (Theme, error) {
	path, err := OmarchyColorsPath()
	if err != nil {
		return Theme{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, fmt.Errorf("no Omarchy theme at %s: %w", path, err)
	}
	return ThemeFromPalette(ParsePalette(data))
}

// OmarchyAvailable reports whether a current Omarchy theme exists on this
// machine.
func OmarchyAvailable() bool {
	path, err := OmarchyColorsPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// ResolveTheme turns a configured theme name into a Theme.
//
//	""          -> follow Omarchy when present, otherwise the default palette
//	"omarchy"   -> follow the current Omarchy theme
//	"default"   -> the original built-in palette
//	anything else -> an embedded Omarchy stock palette by name
func ResolveTheme(name string) (Theme, error) {
	switch name {
	case "":
		if OmarchyAvailable() {
			return OmarchyTheme()
		}
		return DefaultTheme(), nil
	case ThemeNameOmarchy:
		return OmarchyTheme()
	case ThemeNameDefault:
		return DefaultTheme(), nil
	default:
		return BuiltinTheme(name)
	}
}

// ValidThemeName reports whether name is accepted by ResolveTheme. The empty
// string is valid config (auto) but not a name a user sets explicitly.
func ValidThemeName(name string) bool {
	if name == ThemeNameOmarchy || name == ThemeNameDefault {
		return true
	}
	for _, builtin := range BuiltinThemeNames() {
		if name == builtin {
			return true
		}
	}
	return false
}
