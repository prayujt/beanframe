// Package log provides a shared slog logger and immutable context attributes.
package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
)

// Logger is the logging interface available to application components.
// Arguments accept alternating keys and values or slog.Attr values.
type Logger interface {
	Debug(string, ...any)
	Info(string, ...any)
	Warn(string, ...any)
	Error(string, ...any)
	DebugCtx(context.Context, string, ...any)
	InfoCtx(context.Context, string, ...any)
	WarnCtx(context.Context, string, ...any)
	ErrorCtx(context.Context, string, ...any)
}

// Params configures a logger. Zero values select DEBUG, text, and stderr.
type Params struct {
	Level  slog.Leveler
	Format string
	Output io.Writer
}

type logger struct{ *slog.Logger }

var _ Logger = (*logger)(nil)

var shared atomic.Pointer[logger]

func init() { _ = Configure(Params{}) }

func newLogger(params Params) (*logger, error) {
	if params.Output == nil {
		params.Output = os.Stderr
	}
	if params.Level == nil {
		params.Level = slog.LevelDebug
	}
	options := &slog.HandlerOptions{Level: params.Level}
	var handler slog.Handler
	switch params.Format {
	case "json":
		handler = slog.NewJSONHandler(params.Output, options)
	case "", "text":
		handler = slog.NewTextHandler(params.Output, options)
	default:
		return nil, fmt.Errorf("log format must be json or text, got %q", params.Format)
	}
	return &logger{slog.New(contextHandler{handler})}, nil
}

// New creates an independent logger for injection, without changing the global logger.
func New(params Params) (Logger, error) {
	l, err := newLogger(params)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Configure atomically replaces the global logger. Existing instances are unaffected.
func Configure(params Params) error {
	l, err := newLogger(params)
	if err != nil {
		return err
	}
	shared.Store(l)
	return nil
}

func Default() Logger { return shared.Load() }

// Slog exposes the same context-aware logger for libraries that require *slog.Logger.
func Slog() *slog.Logger { return shared.Load().Logger }

func Debug(msg string, args ...any)                         { Default().Debug(msg, args...) }
func Info(msg string, args ...any)                          { Default().Info(msg, args...) }
func Warn(msg string, args ...any)                          { Default().Warn(msg, args...) }
func Error(msg string, args ...any)                         { Default().Error(msg, args...) }
func DebugCtx(ctx context.Context, msg string, args ...any) { Default().DebugCtx(ctx, msg, args...) }
func InfoCtx(ctx context.Context, msg string, args ...any)  { Default().InfoCtx(ctx, msg, args...) }
func WarnCtx(ctx context.Context, msg string, args ...any)  { Default().WarnCtx(ctx, msg, args...) }
func ErrorCtx(ctx context.Context, msg string, args ...any) { Default().ErrorCtx(ctx, msg, args...) }

func (l *logger) DebugCtx(ctx context.Context, msg string, args ...any) {
	l.DebugContext(ctx, msg, args...)
}
func (l *logger) InfoCtx(ctx context.Context, msg string, args ...any) {
	l.InfoContext(ctx, msg, args...)
}
func (l *logger) WarnCtx(ctx context.Context, msg string, args ...any) {
	l.WarnContext(ctx, msg, args...)
}
func (l *logger) ErrorCtx(ctx context.Context, msg string, args ...any) {
	l.ErrorContext(ctx, msg, args...)
}

type attrsKey struct{}

// WithAttrs returns a derived context with additional logging fields. Reassign the
// result: ctx = log.WithAttrs(ctx, "request_id", id). Parent and sibling contexts
// are unchanged. Values follow slog semantics and must not be mutated concurrently.
func WithAttrs(ctx context.Context, args ...any) context.Context {
	parent, _ := ctx.Value(attrsKey{}).([]slog.Attr)
	attrs := append([]slog.Attr(nil), parent...)
	var record slog.Record
	record.Add(args...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	return context.WithValue(ctx, attrsKey{}, attrs)
}

type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	attrs, _ := ctx.Value(attrsKey{}).([]slog.Attr)
	if len(attrs) > 0 {
		// Context fields precede fields supplied at the call site, as with slog.With.
		merged := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
		merged.AddAttrs(attrs...)
		record.Attrs(func(attr slog.Attr) bool { merged.AddAttrs(attr); return true })
		record = merged
	}
	return h.Handler.Handle(ctx, record)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
