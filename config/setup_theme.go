package config

import (
	"fmt"
	"io"
	"strings"

	"github.com/dtt101/doitdoit/styles"
)

// OfferOmarchyHook is called once during initial interactive setup. Installation
// uses the existing managed-hook command and requires an explicit yes; empty
// input, EOF, and read errors leave the desktop unchanged.
func OfferOmarchyHook(cfg *Config, in io.Reader, out io.Writer) {
	if !styles.OmarchyAvailable() || (cfg.Theme != "" && cfg.Theme != styles.ThemeNameSystem) {
		return
	}
	path, err := omarchyHookPath()
	if err != nil {
		return
	}
	state, err := inspectOmarchyHook(path)
	if err != nil {
		fmt.Fprintf(out, "Could not inspect the Omarchy theme hook: %v\n", err)
		return
	}
	if state == hookManaged {
		fmt.Fprintln(out, "Live Omarchy theme updates are already enabled.")
		return
	}
	if state == hookModified {
		fmt.Fprintln(out, "An existing custom Omarchy hook is preserved. Check it with: doitdoit config omarchy-hook status")
		return
	}
	fmt.Fprintln(out, "doitdoit follows your Omarchy theme on launch.")
	fmt.Fprint(out, "Enable live colour updates when you change themes? [y/N]: ")
	answer, err := bufferedReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintln(out, "Live theme updates were not enabled because the setup response could not be read.")
		return
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "y" || answer == "yes" {
		if runOmarchyHook([]string{"install"}, out) == 0 {
			fmt.Fprintln(out, "Disable later with: doitdoit config omarchy-hook remove")
			return
		}
	}
	fmt.Fprintln(out, "You can enable live updates later with: doitdoit config omarchy-hook install")
}
