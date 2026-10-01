// Package env reads typed settings from environment variables.
//
// Unlike the Python gateway (int(os.getenv(...)) raises on a bad value and
// crashes the process), an unparsable value is logged and the default is used.
package env

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Int returns the integer value of key, or def when unset or invalid.
func Int(key string, def int) int {
	raw, ok := lookup(key)
	if !ok {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		slog.Warn("invalid integer env var, using default", "key", key, "value", raw, "default", def)
		return def
	}
	return v
}

// Seconds parses key as a (possibly fractional) number of seconds,
// e.g. "30" or "0.5". Returns def when unset or invalid.
func Seconds(key string, def time.Duration) time.Duration {
	raw, ok := lookup(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 {
		slog.Warn("invalid seconds env var, using default", "key", key, "value", raw, "default", def)
		return def
	}
	return time.Duration(f * float64(time.Second))
}

// lookup returns the trimmed value of key; empty values count as unset.
func lookup(key string) (string, bool) {
	raw, ok := os.LookupEnv(key)
	raw = strings.TrimSpace(raw)
	return raw, ok && raw != ""
}
