package core

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type responseRoundTripFunc func(*http.Request) (*http.Response, error)

func (f responseRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type trackedResponseBody struct {
	reader io.Reader
	reads  int
	closed bool
}

func (b *trackedResponseBody) Read(p []byte) (int, error) {
	b.reads++
	return b.reader.Read(p)
}

func (b *trackedResponseBody) Close() error {
	b.closed = true
	return nil
}

func TestResponseLimitDetectsActualSize(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		contentLength int64
		wantError     bool
	}{
		{name: "below limit", body: "abc", contentLength: -1},
		{name: "at limit", body: "abcd", contentLength: -1},
		{name: "above limit without length", body: "abcde", contentLength: -1, wantError: true},
		{name: "above limit with false length", body: "abcde", contentLength: 2, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedResponseBody{reader: strings.NewReader(test.body)}
			transport := &responseLimitRoundTripper{
				inner: responseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode:    http.StatusOK,
						Body:          body,
						ContentLength: test.contentLength,
						Request:       req,
					}, nil
				}),
				maxBytes: 4,
			}
			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.example.com", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.RoundTrip(request)
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(response.Body)
			if closeErr := response.Body.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if errors.Is(readErr, ErrResponseTooLarge) != test.wantError {
				t.Fatalf("read error = %v, want size error = %t", readErr, test.wantError)
			}
			if len(data) > 4 {
				t.Fatalf("read %d bytes with a four-byte limit", len(data))
			}
			wantData := test.body
			if len(wantData) > 4 {
				wantData = wantData[:4]
			}
			if string(data) != wantData {
				t.Fatalf("read %q, want %q", data, wantData)
			}
			if !body.closed {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestResponseLimitRejectsDeclaredSizeBeforeRead(t *testing.T) {
	body := &trackedResponseBody{reader: strings.NewReader("abc")}
	transport := &responseLimitRoundTripper{
		inner: responseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          body,
				ContentLength: 5,
				Request:       req,
			}, nil
		}),
		maxBytes: 4,
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !errors.Is(readErr, ErrResponseTooLarge) {
		t.Fatalf("declared oversize response returned %v", readErr)
	}
	if body.reads != 0 || !body.closed {
		t.Fatalf("declared oversize response read %d times, closed = %t", body.reads, body.closed)
	}
}

type truncatedReader struct {
	done bool
}

func (r *truncatedReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.ErrUnexpectedEOF
	}
	r.done = true
	return copy(p, "abc"), io.ErrUnexpectedEOF
}

func TestReadResponseBody(t *testing.T) {
	limit := int(DefaultMaxResponseBytes)
	tests := []struct {
		name          string
		size          int
		contentLength int64
		wantError     bool
	}{
		{name: "below limit", size: limit - 1, contentLength: -1},
		{name: "at limit", size: limit, contentLength: -1},
		{name: "above limit without length", size: limit + 1, contentLength: -1, wantError: true},
		{name: "above limit with false length", size: limit + 1, contentLength: 10, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode:    http.StatusOK,
				Header:        http.Header{"X-Request-Id": []string{"req-read"}},
				Body:          io.NopCloser(io.LimitReader(zeroReader{}, int64(test.size))),
				ContentLength: test.contentLength,
			}
			data, err := ReadResponseBody(response)
			if !test.wantError {
				if err != nil {
					t.Fatal(err)
				}
				if len(data) != test.size {
					t.Fatalf("read %d bytes, want %d", len(data), test.size)
				}
				return
			}
			if data != nil {
				t.Fatalf("returned %d bytes with a size error", len(data))
			}
			var sizeErr *ResponseTooLargeError
			if !errors.As(err, &sizeErr) || !errors.Is(err, ErrResponseTooLarge) {
				t.Fatalf("error = %v, want *ResponseTooLargeError", err)
			}
			if sizeErr.StatusCode != http.StatusOK || sizeErr.RequestID != "req-read" {
				t.Fatalf("error = %+v, want status 200 and request id req-read", sizeErr)
			}
		})
	}
}

func TestReadResponseBodyRejectsDeclaredSizeBeforeRead(t *testing.T) {
	body := &trackedResponseBody{reader: strings.NewReader("abc")}
	response := &http.Response{
		StatusCode:    http.StatusInternalServerError,
		Body:          body,
		ContentLength: DefaultMaxResponseBytes + 1,
	}
	_, err := ReadResponseBody(response)
	var sizeErr *ResponseTooLargeError
	if !errors.As(err, &sizeErr) || sizeErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error = %v, want *ResponseTooLargeError with status 500", err)
	}
	if body.reads != 0 {
		t.Fatalf("declared oversize response read %d times", body.reads)
	}
}

func TestReadResponseBodyKeepsReadError(t *testing.T) {
	response := &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(&truncatedReader{}),
		ContentLength: -1,
	}
	data, err := ReadResponseBody(response)
	if !errors.Is(err, io.ErrUnexpectedEOF) || data != nil {
		t.Fatalf("truncated response returned %d bytes and %v", len(data), err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
