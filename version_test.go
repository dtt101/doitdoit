package main

import (
	"runtime/debug"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	noBuildInfo := func() (*debug.BuildInfo, bool) { return nil, false }

	if got := formatVersion("1.2.3", "abcdef1234", "2026-11-23", noBuildInfo); got != "doitdoit v1.2.3" {
		t.Fatalf("release version: got %q", got)
	}
	if got := formatVersion("v1.2.3", "", "", noBuildInfo); got != "doitdoit v1.2.3" {
		t.Fatalf("prefixed release version: got %q", got)
	}
	if got := formatVersion("dev", "", "", func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}}, true
	}); got != "doitdoit v1.0.0" {
		t.Fatalf("module build info version: got %q", got)
	}
	if got := formatVersion("dev", "", "", func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "2f52738abcdef"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	}); got != "doitdoit dev (2f52738-dirty)" {
		t.Fatalf("vcs build info version: got %q", got)
	}
	if got := formatVersion("dev", "", "", noBuildInfo); got != "doitdoit dev" {
		t.Fatalf("fallback version: got %q", got)
	}
}
