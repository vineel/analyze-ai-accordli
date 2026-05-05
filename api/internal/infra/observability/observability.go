// Package observability is the logging/tracing seam.
//
// SoloMocky:  stdout JSON logger.
// Phase obs:  Azure Monitor + Helicone wrappers.
package observability

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"
)

type Logger interface {
	Info(ctx context.Context, msg string, fields map[string]any)
	Error(ctx context.Context, msg string, fields map[string]any)
}

type StdoutLogger struct {
	w io.Writer
}

func NewStdout() *StdoutLogger { return &StdoutLogger{w: os.Stdout} }

func (l *StdoutLogger) emit(level, msg string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["level"] = level
	fields["msg"] = msg
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	enc := json.NewEncoder(l.w)
	_ = enc.Encode(fields)
}

func (l *StdoutLogger) Info(_ context.Context, msg string, fields map[string]any) {
	l.emit("info", msg, fields)
}

func (l *StdoutLogger) Error(_ context.Context, msg string, fields map[string]any) {
	l.emit("error", msg, fields)
}
