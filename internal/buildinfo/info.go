// Package buildinfo exposes immutable release metadata embedded during compilation.
package buildinfo

import "runtime/debug"

var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
	dirty   = "true"
)

// Info identifies the source and build represented by an executable.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
	Dirty   bool   `json:"dirty"`
}

// Current returns linker-provided identity, falling back to Go VCS metadata when available.
func Current() Info {
	info := Info{Version: version, Commit: commit, BuiltAt: builtAt, Dirty: dirty != "false"}
	// Plain go build/Air retain useful Go VCS metadata when linker overrides are absent.
	if commit == "unknown" {
		if b, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range b.Settings {
				switch setting.Key {
				case "vcs.revision":
					info.Commit = setting.Value
				case "vcs.modified":
					info.Dirty = setting.Value == "true"
				}
			}
		}
	}
	return info
}
