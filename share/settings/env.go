package settings

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Env returns a chisel environment variable
func Env(name string) string {
	return os.Getenv("CHISEL_" + name)
}

// EnvInt returns an integer using an environment variable, with a default fallback
func EnvInt(name string, def int) int {
	if n, err := strconv.Atoi(Env(name)); err == nil {
		return n
	}
	return def
}

// EnvDuration returns a duration using an environment variable, with a default fallback
func EnvDuration(name string, def time.Duration) time.Duration {
	if n, err := time.ParseDuration(Env(name)); err == nil {
		return n
	}
	return def
}

// EnvBool returns a boolean using an environment variable
func EnvBool(name string) bool {
	v := Env(name)
	return v == "1" || strings.ToLower(v) == "true"
}
