package core

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// LoggingRoundTripper logs one record for each logical SDK request. When it
// wraps RetryRoundTripper, duration includes retry attempts and backoff.
type LoggingRoundTripper struct {
	Inner  http.RoundTripper
	Logger *slog.Logger
}

type idempotencyKeyContextKey struct{}

// WithIdempotencyKey returns a context that adds Idempotency-Key to POST
// requests. Callers should use it only for operations documented by the API as
// supporting idempotent replay.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, idempotencyKeyContextKey{}, key)
}

// IdempotencyKeyFromContext returns an idempotency key previously attached by
// WithIdempotencyKey.
func IdempotencyKeyFromContext(ctx context.Context) string {
	key, _ := ctx.Value(idempotencyKeyContextKey{}).(string)
	return key
}

func (l *LoggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := l.Inner
	if inner == nil {
		inner = http.DefaultTransport
	}

	started := time.Now()
	resp, err := inner.RoundTrip(req)
	if l.Logger == nil {
		return resp, err
	}

	attrs := []slog.Attr{
		slog.String("method", req.Method),
		slog.String("path", req.URL.EscapedPath()),
		slog.Duration("duration", time.Since(started)),
	}
	if resp != nil {
		attrs = append(attrs, slog.Int("status", resp.StatusCode))
		if requestID := RequestIDFromHeader(resp.Header); requestID != "" {
			attrs = append(attrs, slog.String("request_id", requestID))
		}
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	l.Logger.LogAttrs(req.Context(), slog.LevelDebug, "viettelcloud sdk request", attrs...)
	return resp, err
}

// RequestIDFromHeader returns the request identifier used by Viettel Cloud APIs.
func RequestIDFromHeader(header http.Header) string {
	if header == nil {
		return ""
	}
	if requestID := header.Get("X-Request-ID"); requestID != "" {
		return requestID
	}
	return header.Get("Request-ID")
}
