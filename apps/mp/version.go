package main

import "runtime/debug"

// version is injected at build time via -ldflags "-X main.version=v1.2.3".
// `go install module@version` builds fall back to the module version recorded
// in build info; bare `go build` reports "dev".
var version = ""

func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func init() {
	rootCmd.Version = resolveVersion()
}
