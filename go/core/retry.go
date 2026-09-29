package core

import (
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// RetryConfig controls retry behaviour.
type RetryConfig struct {
	MaxAttempts int           // total attempts including the first (0 = no retry)
	BaseDelay   time.Duration // initial backoff delay
	MaxDelay    time.Duration // ceiling for backoff
}

// DefaultRetry returns a retry config with 3 attempts, 500ms base, 10s cap.
func DefaultRetry() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    10 * time.Second,
	}
}

// NoRetry disables retries (single attempt).
func NoRetry() *RetryConfig {
	return &RetryConfig{MaxAttempts: 1}
}

// ---------------------------------------------------------------------------
// Round tripper
// ---------------------------------------------------------------------------

// RetryRoundTripper wraps an http.RoundTripper with exponential backoff +
// jitter on 429 and 5xx responses. It honours the Retry-After header on 429.
type RetryRoundTripper struct {
	Inner           http.RoundTripper
	Config          *RetryConfig
	RetryAllMethods bool // when true, POST also retries
}

func (r *RetryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := r.Inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	cfg := r.Config
	if cfg == nil {
		cfg = NoRetry()
	}

	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	if maxAttempts == 1 || !isRetryableMethod(req.Method, r.RetryAllMethods) {
		return inner.RoundTrip(req)
	}

	// A request body must be reproducible before the first attempt. Sending an
	// unreplayable body once is safer than retrying it as an empty body.
	if req.Body != nil && req.GetBody == nil {
		return inner.RoundTrip(req)
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		attemptReq := req
		if attempt > 0 {
			attemptReq = req.Clone(req.Context())
			if req.Body != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				attemptReq.Body = body
			}
		}

		resp, err := inner.RoundTrip(attemptReq)
		if err != nil {
			return nil, err
		}

		if !isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		// Don't retry if this is the last attempt.
		if attempt == maxAttempts-1 {
			return resp, nil
		}

		delay := r.computeDelay(resp, attempt, cfg)
		closeRetryResponse(resp)
		select {
		case <-attemptReq.Context().Done():
			return nil, attemptReq.Context().Err()
		case <-time.After(delay):
		}
	}

	panic("unreachable")
}

func isRetryableMethod(method string, allowNonIdempotent bool) bool {
	if allowNonIdempotent {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		(statusCode >= http.StatusInternalServerError && statusCode <= 599)
}

func closeRetryResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	// A bounded drain allows connection reuse for normal error envelopes while
	// avoiding an unbounded read from a misbehaving server.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
	_ = resp.Body.Close()
}

// computeDelay calculates the backoff delay, honouring Retry-After on 429.
func (r *RetryRoundTripper) computeDelay(resp *http.Response, attempt int, cfg *RetryConfig) time.Duration {
	// Honour Retry-After header on 429.
	if resp.StatusCode == 429 {
		if retryAfter := parseRetryAfter(resp); retryAfter > 0 {
			if cfg.MaxDelay > 0 && retryAfter > cfg.MaxDelay {
				retryAfter = cfg.MaxDelay
			}
			return retryAfter
		}
	}

	// Exponential backoff: baseDelay * 2^attempt + jitter.
	delay := cfg.BaseDelay * time.Duration(math.Pow(2, float64(attempt)))
	if cfg.MaxDelay > 0 && delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	// Add jitter: ±25% of delay.
	jitter := float64(delay) * 0.25
	delay = time.Duration(float64(delay) + (rand.Float64()*2-1)*jitter)

	return delay
}

// parseRetryAfter parses the Retry-After header (seconds or HTTP-date).
func parseRetryAfter(resp *http.Response) time.Duration {
	val := resp.Header.Get("Retry-After")
	if val == "" {
		return 0
	}

	// Try integer seconds first.
	if secs, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
		return time.Duration(secs) * time.Second
	}

	// Try HTTP-date (RFC 7231).
	if t, err := time.Parse(time.RFC1123, strings.TrimSpace(val)); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}

	return 0
}
