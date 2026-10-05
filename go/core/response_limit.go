package core

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

// DefaultMaxResponseBytes is the largest response body accepted by public SDK clients.
const DefaultMaxResponseBytes int64 = 16 << 20

// ErrResponseTooLarge means a server response exceeded the SDK size limit.
var ErrResponseTooLarge = errors.New("response body exceeds the SDK size limit")

// ResponseTooLargeError reports a response rejected by the SDK size limit.
// It never includes the response body. errors.Is(err, ErrResponseTooLarge)
// matches it.
type ResponseTooLargeError struct {
	StatusCode int
	RequestID  string
	Limit      int64
}

func (e *ResponseTooLargeError) Error() string {
	message := fmt.Sprintf("%s: maximum %d bytes (status=%d)", ErrResponseTooLarge, e.Limit, e.StatusCode)
	if e.RequestID != "" {
		message += fmt.Sprintf(" [request_id=%s]", e.RequestID)
	}
	return message
}

func (e *ResponseTooLargeError) Unwrap() error {
	return ErrResponseTooLarge
}

func newResponseTooLargeError(response *http.Response, limit int64) *ResponseTooLargeError {
	return &ResponseTooLargeError{
		StatusCode: response.StatusCode,
		RequestID:  RequestIDFromHeader(response.Header),
		Limit:      limit,
	}
}

// ReadResponseBody reads at most DefaultMaxResponseBytes from response.Body.
// A larger body returns *ResponseTooLargeError and no data, so callers never
// treat a cut body as a valid response. Generated Parse*Response functions call
// it instead of io.ReadAll. The caller still closes response.Body.
func ReadResponseBody(response *http.Response) ([]byte, error) {
	limit := DefaultMaxResponseBytes
	if response.ContentLength > limit {
		return nil, newResponseTooLargeError(response, limit)
	}
	// A declared length within the limit sizes the buffer once. The extra byte
	// lets the read reach EOF, or find a body longer than declared, without
	// growing the buffer.
	initial := int64(512)
	if response.ContentLength >= 0 {
		initial = response.ContentLength + 1
	}
	data, err := readAll(io.LimitReader(response.Body, limit+1), make([]byte, 0, initial))
	if int64(len(data)) > limit {
		return nil, newResponseTooLargeError(response, limit)
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// readAll is io.ReadAll with a caller-supplied initial buffer.
func readAll(r io.Reader, data []byte) ([]byte, error) {
	for {
		if len(data) == cap(data) {
			data = append(data, 0)[:len(data)]
		}
		n, err := r.Read(data[len(data):cap(data)])
		data = data[:len(data)+n]
		if err == io.EOF {
			return data, nil
		}
		if err != nil {
			return data, err
		}
	}
}

type responseLimitRoundTripper struct {
	inner    http.RoundTripper
	maxBytes int64
}

func (r *responseLimitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	inner := r.inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	response, err := inner.RoundTrip(req)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}

	limit := r.maxBytes
	if limit <= 0 {
		limit = DefaultMaxResponseBytes
	}
	limitError := newResponseTooLargeError(response, limit)
	if response.ContentLength > limit {
		_ = response.Body.Close()
		response.Body = &limitedResponseBody{
			ReadCloser: http.NoBody,
			limitError: limitError,
			exceeded:   true,
		}
		return response, nil
	}

	response.Body = &limitedResponseBody{
		ReadCloser: response.Body,
		remaining:  limit,
		limitError: limitError,
	}
	return response, nil
}

type limitedResponseBody struct {
	io.ReadCloser
	remaining  int64
	limitError error
	exceeded   bool
}

func (b *limitedResponseBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.exceeded {
		return 0, b.limitError
	}
	if b.remaining == 0 {
		var extra [1]byte
		n, err := b.ReadCloser.Read(extra[:])
		if n > 0 {
			b.exceeded = true
			return 0, b.limitError
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:int(b.remaining)]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, err
}
