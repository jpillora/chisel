package chshare

import (
	"runtime/debug"
	"testing"
)

func TestFallbackVersion(t *testing.T) {
	infoWith := func(v string) *debug.BuildInfo {
		i := &debug.BuildInfo{}
		i.Main.Version = v
		return i
	}
	for _, tc := range []struct {
		current, module, want string
	}{
		//module install: use the embedded module version
		{"0.0.0-src", "v1.11.6", "1.11.6"},
		//source build: no module version available
		{"0.0.0-src", "(devel)", "0.0.0-src"},
		{"0.0.0-src", "", "0.0.0-src"},
		//ldflags-stamped builds always win
		{"1.12.0", "v1.11.6", "1.12.0"},
	} {
		if got := fallbackVersion(tc.current, infoWith(tc.module)); got != tc.want {
			t.Errorf("fallbackVersion(%q, %q) = %q, want %q", tc.current, tc.module, got, tc.want)
		}
	}
}
