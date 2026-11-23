package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Set via GoReleaser's default -ldflags at release time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func versionString() string {
	return formatVersion(version, commit, date, debug.ReadBuildInfo)
}

func formatVersion(ver, rev, buildDate string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if ver != "" && ver != "dev" {
		if !strings.HasPrefix(ver, "v") {
			ver = "v" + ver
		}
		return fmt.Sprintf("doitdoit %s", ver)
	}
	if readBuildInfo != nil {
		if info, ok := readBuildInfo(); ok && info != nil {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				return fmt.Sprintf("doitdoit %s", info.Main.Version)
			}
			var vcsRev string
			var dirty bool
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					vcsRev = setting.Value
				case "vcs.modified":
					dirty = setting.Value == "true"
				}
			}
			if vcsRev != "" {
				if len(vcsRev) > 7 {
					vcsRev = vcsRev[:7]
				}
				if dirty {
					vcsRev += "-dirty"
				}
				return fmt.Sprintf("doitdoit dev (%s)", vcsRev)
			}
		}
	}
	if rev != "" {
		if len(rev) > 7 {
			rev = rev[:7]
		}
		if buildDate != "" {
			return fmt.Sprintf("doitdoit dev (%s, %s)", rev, buildDate)
		}
		return fmt.Sprintf("doitdoit dev (%s)", rev)
	}
	return "doitdoit dev"
}
