// SPDX-License-Identifier: AGPL-3.0-only
// Project attribution: Xpotato.net / Xpotato1024. Third-party rights are retained.
package licensing

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"runtime/debug"
)

//go:embed LICENSE.txt
var Text string

//go:embed COPYRIGHT.txt
var Attribution string

// BuildCommit identifies source-archive builds without VCS metadata. Forks must
// set SourceRepository to their own publicly accessible corresponding source.
var BuildCommit string
var SourceRepository = "https://github.com/Xpotato1024/xpotato-mcp-tunnel"

type Information struct {
	Project      string `json:"project"`
	Attribution  string `json:"attribution"`
	License      string `json:"license"`
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	Modified     bool   `json:"modified"`
	Repository   string `json:"repository"`
	Source       string `json:"source"`
	SourceBundle string `json:"sourceBundle"`
	Notice       string `json:"notice"`
}

func Info(version string) Information {
	revision, modified := BuildCommit, false
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value != "false"
			}
		}
	}
	info := Information{Project: "xpotato-mcp-tunnel", Attribution: "Xpotato.net / Xpotato1024", License: "AGPL-3.0-only", Version: version, Commit: revision, Modified: modified, Repository: SourceRepository,
		Notice: "Project-specific code: AGPL-3.0-only; third-party components retain their own licenses. No warranty. See LICENSE, COPYRIGHT and THIRD-PARTY-NOTICES.txt."}
	if regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(revision) && !modified {
		info.Source = SourceRepository + "/tree/" + revision
		info.SourceBundle = SourceRepository + "/releases/download/v" + version + "/xpotato-tunnel-" + version + "-source.tar.gz"
	} else {
		info.Modified = true
		info.Notice += " Development/modified build: the operator must provide the actual corresponding source before distribution or remote use of a modified version."
	}
	return info
}

func JSON(version string) string {
	b, _ := json.Marshal(Info(version))
	return string(b)
}
