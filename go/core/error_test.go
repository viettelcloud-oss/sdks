package core

import (
	"errors"
	"net/http"
	"testing"
)

type samplePayload struct {
	Name string `json:"name"`
}

func TestParseResponseMultipleStatuses(t *testing.T) {
	body := []byte(`{"name":"from-body"}`)
	json200 := &samplePayload{Name: "json200"}
	json202 := &samplePayload{Name: "json202"}

	tests := []struct {
		name     string
		status   int
		payload  *samplePayload
		body     []byte
		others   []int
		wantName string
		// wantErr is the expected error type; nil means success.
		wantErr     any
		wantMessage string
	}{
		{
			name:     "first status uses its field",
			status:   200,
			payload:  FirstNonNil(json200, nil),
			body:     body,
			others:   []int{202},
			wantName: "json200",
		},
		{
			name:     "other status uses its field",
			status:   202,
			payload:  FirstNonNil(nil, json202),
			body:     body,
			others:   []int{202},
			wantName: "json202",
		},
		{
			name:     "other status falls back to body decode",
			status:   202,
			payload:  nil,
			body:     body,
			others:   []int{202},
			wantName: "from-body",
		},
		{
			name:     "documented status outside the original generator list",
			status:   203,
			payload:  nil,
			body:     body,
			others:   []int{203},
			wantName: "from-body",
		},
		{
			name:        "undocumented status outside the original generator list",
			status:      206,
			payload:     nil,
			others:      []int{202},
			wantErr:     &ResponseError{},
			wantMessage: "unexpected HTTP status; expected one of 200, 202",
		},
		{
			name:        "undocumented 2xx lists every expected status",
			status:      204,
			payload:     nil,
			others:      []int{202},
			wantErr:     &ResponseError{},
			wantMessage: "unexpected HTTP status; expected one of 200, 202",
		},
		{
			name:        "single status keeps the original message",
			status:      202,
			payload:     nil,
			wantErr:     &ResponseError{},
			wantMessage: "unexpected HTTP status; expected 200",
		},
		{
			name:    "4xx is still parsed as an API error",
			status:  404,
			payload: nil,
			body:    []byte(`{"message":"load balancer not found"}`),
			others:  []int{202},
			wantErr: &APIError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResponse(&http.Response{StatusCode: tt.status, Header: make(http.Header)}, tt.body, tt.payload, 200, tt.others...)

			switch want := tt.wantErr.(type) {
			case nil:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil || got.Name != tt.wantName {
					t.Fatalf("got payload %+v, want name %q", got, tt.wantName)
				}
			case *ResponseError:
				if !errors.As(err, &want) {
					t.Fatalf("got %T (%v), want *ResponseError", err, err)
				}
				if want.StatusCode != tt.status {
					t.Errorf("StatusCode = %d, want %d", want.StatusCode, tt.status)
				}
				if want.Message != tt.wantMessage {
					t.Errorf("Message = %q, want %q", want.Message, tt.wantMessage)
				}
			case *APIError:
				if !errors.As(err, &want) {
					t.Fatalf("got %T (%v), want *APIError", err, err)
				}
				if want.StatusCode != tt.status {
					t.Errorf("StatusCode = %d, want %d", want.StatusCode, tt.status)
				}
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("errors.Is(err, ErrNotFound) = false for %v", err)
				}
			}
		})
	}
}

func TestParseEmptyResponseMultipleStatuses(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		others      []int
		wantMessage string // empty means success
	}{
		{name: "first status", status: 200, others: []int{202}},
		{name: "other status", status: 202, others: []int{202}},
		{name: "documented empty status outside the original generator list", status: 205, others: []int{205}},
		{
			name:        "undocumented 2xx lists every expected status",
			status:      204,
			others:      []int{202},
			wantMessage: "unexpected HTTP status; expected one of 200, 202",
		},
		{
			name:        "single status keeps the original message",
			status:      204,
			wantMessage: "unexpected HTTP status; expected 200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ParseEmptyResponse(&http.Response{StatusCode: tt.status, Header: make(http.Header)}, nil, 200, tt.others...)
			if tt.wantMessage == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var respErr *ResponseError
			if !errors.As(err, &respErr) {
				t.Fatalf("got %T (%v), want *ResponseError", err, err)
			}
			if respErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", respErr.StatusCode, tt.status)
			}
			if respErr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", respErr.Message, tt.wantMessage)
			}
		})
	}

	t.Run("4xx is still parsed as an API error", func(t *testing.T) {
		err := ParseEmptyResponse(&http.Response{StatusCode: 409, Header: make(http.Header)}, []byte(`{"message":"busy"}`), 200, 202)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %T (%v), want *APIError", err, err)
		}
		if !errors.Is(err, ErrConflict) {
			t.Errorf("errors.Is(err, ErrConflict) = false for %v", err)
		}
	})
}

func TestFirstNonNil(t *testing.T) {
	first, second := "first", "second"

	tests := []struct {
		name string
		ptrs []*string
		want *string
	}{
		{name: "first set", ptrs: []*string{&first, &second}, want: &first},
		{name: "skips nil", ptrs: []*string{nil, nil, &second}, want: &second},
		{name: "all nil", ptrs: []*string{nil, nil}, want: nil},
		{name: "no arguments", ptrs: nil, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FirstNonNil(tt.ptrs...); got != tt.want {
				t.Errorf("FirstNonNil() = %v, want %v", got, tt.want)
			}
		})
	}
}
