package releasecheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestThirdPartyNoticesCoverCompiledModulesAndThemes(t *testing.T) {
	noticesBytes, err := os.ReadFile(filepath.Join("..", "THIRD_PARTY_NOTICES.md"))
	if err != nil {
		t.Fatal(err)
	}
	notices := string(noticesBytes)
	modules := make(map[string]struct{})
	for _, target := range []struct{ goos, goarch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"windows", "amd64"},
	} {
		cmd := exec.Command("go", "list", "-deps", "-f", `{{if .Module}}{{if not .Module.Main}}{{.Module.Path}}|{{.Module.Version}}{{end}}{{end}}`, ".")
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "GOOS="+target.goos, "GOARCH="+target.goarch, "CGO_ENABLED=0")
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list for %s/%s: %v", target.goos, target.goarch, err)
		}
		for _, line := range strings.Fields(string(output)) {
			modules[line] = struct{}{}
		}
	}

	var missing []string
	for module := range modules {
		parts := strings.SplitN(module, "|", 2)
		row := fmt.Sprintf("| `%s` | %s |", parts[0], parts[1])
		if !strings.Contains(notices, row) {
			missing = append(missing, module)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("THIRD_PARTY_NOTICES.md is missing compiled modules: %v", missing)
	}

	themeFiles, err := filepath.Glob(filepath.Join("..", "styles", "themes", "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(themeFiles) != 22 {
		t.Fatalf("theme inventory assumption changed: got %d files, want 22", len(themeFiles))
	}
	for _, path := range themeFiles {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if !strings.Contains(notices, name) {
			t.Errorf("notices do not name embedded theme %q", name)
		}
	}
}

func TestDistributionLicenseFiles(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "LICENSE")); err != nil {
		t.Fatalf("LICENSE: %v", err)
	}
	if _, err := os.Stat(filepath.Join("..", "LICENCE.md")); !os.IsNotExist(err) {
		t.Fatalf("obsolete LICENCE.md still exists or could not be checked: %v", err)
	}
}
