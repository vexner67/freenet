// Package reqid gives every request an ID that follows it from the gateway
// through every service it calls, so its log lines can be found together.
// The ID travels in the context, in the X-Request-ID HTTP header and in the
// x-request-id gRPC metadata, and every log line written with that context
// carries it as request_id.
package reqid

import (
	"context"
	"crypto/rand"
	"log/slog"
)

const (
	// Header is the HTTP header of the ID: the gateway answers with it and,
	// behind its own proxy, takes it from the request.
	Header = "X-Request-ID"
	// LogKey is the attribute of every log line written with an ID.
	LogKey = "request_id"

	metadataKey = "x-request-id"
	maxLen      = 64
)

type idKey struct{}

// New returns a random ID.
func New() string {
	return rand.Text()
}

// Valid reports whether an ID that came from outside may be used: short
// and of letters, digits and -_.: only, so it cannot forge log lines.
func Valid(id string) bool {
	if id == "" || len(id) > maxLen {
		return false
	}

	for _, c := range id {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '-', c == '_', c == '.', c == ':':
		default:
			return false
		}
	}

	return true
}

func ContextWithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, idKey{}, id)
}

// FromContext returns the ID of ctx, or "" without one.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(idKey{}).(string)
	return id
}

// LogHandler adds request_id to every record logged with a context that
// has an ID. Groups opened with WithGroup would hold it too; freenet logs
// without them.
func LogHandler(h slog.Handler) slog.Handler {
	return logHandler{Handler: h}
}

type logHandler struct {
	slog.Handler
}

func (h logHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := FromContext(ctx); id != "" {
		r.AddAttrs(slog.String(LogKey, id))
	}

	return h.Handler.Handle(ctx, r)
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h logHandler) WithGroup(name string) slog.Handler {
	return logHandler{Handler: h.Handler.WithGroup(name)}
}
