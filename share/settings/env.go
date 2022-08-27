package settings

import (
	"math"
	"os"
	"strconv"
	"time"
)

//Env returns a chisel environment variable
func Env(name string) string {
	return os.Getenv("CHISEL_" + name)
}

//EnvInt returns an integer using an environment variable, with a default fallback
func EnvInt(name string, def int) int {
	if n, err := strconv.Atoi(Env(name)); err == nil {
		return n
	}
	return def
}

//EnvDuration returns a duration using an environment variable, with a default fallback
func EnvDuration(name string, def time.Duration) time.Duration {
	if n, err := time.ParseDuration(Env(name)); err == nil {
		return n
	}
	return def
}

//EnvTlsVersion returns a tls version protocol number
func EnvTlsVersion(name string) uint16 {
	version := Env(name)
	if n, err := strconv.ParseFloat(version, 64); err == nil {
		version_number := math.Round(n*10) - 10
		return uint16(0x301 + version_number) // conversion to TLS_VERSION (0x030x)
	}
	return 0
}
