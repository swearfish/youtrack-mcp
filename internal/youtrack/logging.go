package youtrack

import (
	"context"
	"log/slog"
)

type LogFunc func(context.Context, slog.Level, string, ...any)

type logContextKey struct{}

func WithLogger(ctx context.Context, logf LogFunc) context.Context {
	if logf == nil {
		return ctx
	}
	return context.WithValue(ctx, logContextKey{}, logf)
}

func logDebug(ctx context.Context, msg string, args ...any) {
	logMessage(ctx, slog.LevelDebug, msg, args...)
}

func logWarn(ctx context.Context, msg string, args ...any) {
	logMessage(ctx, slog.LevelWarn, msg, args...)
}

func logMessage(ctx context.Context, level slog.Level, msg string, args ...any) {
	logf, _ := ctx.Value(logContextKey{}).(LogFunc)
	if logf == nil {
		return
	}
	logf(ctx, level, msg, args...)
}
