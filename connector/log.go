package connector

import (
	"context"
	"log/slog"
	"strings"
)

// newLogger returns the default logger tagged with the connector's name and
// type. A "logLevel" in the config overrides the level for this connector
// only — like Python's per-connector logger.setLevel — and may be lower
// than the global level.
func newLogger(cfg Config) *slog.Logger {
	l := slog.Default().With("connector", cfg.Name, "type", string(cfg.Type))
	if lvl := cfg.LogLevel(); lvl != "" {
		l = slog.New(levelHandler{inner: l.Handler(), min: parseLevel(lvl)})
	}
	return l
}

// parseLevel maps Python level names to slog levels. Unknown names fall
// back to DEBUG, as Python's getattr(logging, name, logging.DEBUG) did.
func parseLevel(name string) slog.Level {
	switch strings.ToUpper(name) {
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	case "CRITICAL", "FATAL":
		return slog.LevelError + 4
	default:
		return slog.LevelDebug
	}
}

// levelHandler filters records by its own minimum level instead of the
// wrapped handler's.
type levelHandler struct {
	inner slog.Handler
	min   slog.Level
}

func (h levelHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.min }

func (h levelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}

func (h levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelHandler{inner: h.inner.WithAttrs(attrs), min: h.min}
}

func (h levelHandler) WithGroup(name string) slog.Handler {
	return levelHandler{inner: h.inner.WithGroup(name), min: h.min}
}
