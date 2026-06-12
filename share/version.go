package chshare

import (
	"runtime/debug"
	"strings"
)

//ProtocolVersion of chisel. When backwards
//incompatible changes are made, this will
//be incremented to signify a protocol
//mismatch.
var ProtocolVersion = "chisel-v3"

//BuildVersion is set at build time via ldflags,
//and otherwise falls back to the go module version
//embedded by `go install pkg@version` (see init)
var BuildVersion = "0.0.0-src"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		BuildVersion = fallbackVersion(BuildVersion, info)
	}
}

//fallbackVersion returns the go module version when the build
//version was not stamped via ldflags
func fallbackVersion(current string, info *debug.BuildInfo) string {
	if current != "0.0.0-src" {
		return current //stamped via ldflags
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return current //source build, no module version
	}
	return strings.TrimPrefix(v, "v")
}
