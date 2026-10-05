package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// RequestEditorFn is the signature for functions that edit HTTP requests
// before they are sent. Used for auth headers, user-agent, logging, etc.
type RequestEditorFn func(ctx context.Context, req *http.Request) error

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation")
	ErrRateLimited  = errors.New("rate limited")
	ErrInternal     = errors.New("internal server error")
)

// statusToSentinel maps HTTP status codes to sentinel errors.
var statusToSentinel = map[int]error{
	404: ErrNotFound,
	401: ErrUnauthorized,
	403: ErrUnauthorized,
	409: ErrConflict,
	400: ErrValidation,
	422: ErrValidation,
	429: ErrRateLimited,
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

// ErrorDetail represents a single field-level error from the API.
type ErrorDetail struct {
	Location string `json:"location"` // field path, e.g. "flavor_id" (may be empty)
	Code     string `json:"code"`     // machine-readable, e.g. "value_error"
	Message  string `json:"message"`  // human-readable
}

// APIError represents a structured error response from the Viettel Cloud API.
type APIError struct {
	StatusCode int           `json:"-"`
	RequestID  string        `json:"-"`
	Errors     []ErrorDetail `json:"-"`
	Raw        []byte        `json:"-"`
}

// Error renders the first detail plus the request id, e.g.
//
//	"flavor_id: Flavor does not exist. (value_error) [request_id=bd381cdf...]"
func (e *APIError) Error() string {
	var b strings.Builder
	if len(e.Errors) > 0 {
		d := e.Errors[0]
		if d.Location != "" {
			fmt.Fprintf(&b, "%s: ", d.Location)
		}
		b.WriteString(d.Message)
		if d.Code != "" {
			fmt.Fprintf(&b, " (%s)", d.Code)
		}
	} else {
		fmt.Fprintf(&b, "Viettel Cloud API error %d", e.StatusCode)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " [request_id=%s]", e.RequestID)
	}
	return b.String()
}

// Code returns the first detail's machine-readable code (convenience for
// single-error responses). Returns empty string if no errors.
func (e *APIError) Code() string {
	if len(e.Errors) > 0 {
		return e.Errors[0].Code
	}
	return ""
}

// Unwrap returns the sentinel error matching the status code, enabling
// errors.Is(err, ErrNotFound) etc.
func (e *APIError) Unwrap() error {
	if sent, ok := statusToSentinel[e.StatusCode]; ok {
		return sent
	}
	if e.StatusCode >= 500 {
		return ErrInternal
	}
	return nil
}

// ---------------------------------------------------------------------------
// Error parsing
// ---------------------------------------------------------------------------

// errorEnvelope matches the standard backend error response shape.
type errorEnvelope struct {
	Errors []struct {
		Location string `json:"location"`
		Code     string `json:"code"`
		Message  string `json:"message"`
	} `json:"errors"`
	RequestID string `json:"request_id"`
}

// messageBody matches the simpler {"message": "..."} / {"detail": "..."} shape.
type messageBody struct {
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// ResponseError reports an unexpected or malformed success response.
type ResponseError struct {
	StatusCode int
	RequestID  string
	Message    string
	Raw        []byte
}

func (e *ResponseError) Error() string {
	message := e.Message
	if message == "" {
		message = "unexpected response"
	}
	if e.RequestID != "" {
		return fmt.Sprintf("%s (status=%d) [request_id=%s]", message, e.StatusCode, e.RequestID)
	}
	return fmt.Sprintf("%s (status=%d)", message, e.StatusCode)
}

// ParseErrorResponse parses a JSON error body returned by the Viettel Cloud API.
// It tries the structured ErrorResponse envelope first, then falls back to
// simpler shapes, and finally returns a raw-body error.
func ParseErrorResponse(statusCode int, body []byte) error {
	return ParseErrorResponseWithHeader(statusCode, nil, body)
}

// ParseErrorResponseWithHeader also obtains a request ID from response headers
// when the error envelope does not include one.
func ParseErrorResponseWithHeader(statusCode int, header http.Header, body []byte) error {
	requestID := RequestIDFromHeader(header)
	if len(body) == 0 {
		return &APIError{StatusCode: statusCode, RequestID: requestID}
	}

	// Try structured ErrorResponse envelope.
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil && (len(env.Errors) > 0 || env.RequestID != "") {
		if env.RequestID != "" {
			requestID = env.RequestID
		}
		errDetails := make([]ErrorDetail, len(env.Errors))
		for i, e := range env.Errors {
			errDetails[i] = ErrorDetail{
				Location: e.Location,
				Code:     e.Code,
				Message:  e.Message,
			}
		}
		return &APIError{
			StatusCode: statusCode,
			RequestID:  requestID,
			Errors:     errDetails,
			Raw:        body,
		}
	}

	// Try {"message": "..."} / {"detail": "..."}.
	var mb messageBody
	if err := json.Unmarshal(body, &mb); err == nil && (mb.Message != "" || mb.Detail != "") {
		msg := mb.Message
		if msg == "" {
			msg = mb.Detail
		}
		return &APIError{
			StatusCode: statusCode,
			RequestID:  requestID,
			Errors:     []ErrorDetail{{Message: msg}},
			Raw:        body,
		}
	}

	// Fallback: raw body as the message.
	return &APIError{
		StatusCode: statusCode,
		RequestID:  requestID,
		Errors:     []ErrorDetail{{Message: string(body)}},
		Raw:        body,
	}
}

// ParseResponse validates the documented success status and returns a decoded
// payload. If oapi-codegen skipped decoding because Content-Type was missing or
// incorrect, it makes one JSON decode attempt from the retained raw body.
func ParseResponse[T any](response *http.Response, body []byte, payload *T, expectedStatus int) (*T, error) {
	statusCode, header := responseMetadata(response)
	if statusCode != expectedStatus {
		if statusCode >= http.StatusBadRequest {
			return nil, ParseErrorResponseWithHeader(statusCode, header, body)
		}
		return nil, &ResponseError{
			StatusCode: statusCode,
			RequestID:  RequestIDFromHeader(header),
			Message:    fmt.Sprintf("unexpected HTTP status; expected %d", expectedStatus),
			Raw:        body,
		}
	}
	if payload != nil {
		return payload, nil
	}
	if len(body) == 0 {
		return nil, &ResponseError{
			StatusCode: statusCode,
			RequestID:  RequestIDFromHeader(header),
			Message:    "success response has no payload",
		}
	}
	var decoded T
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, &ResponseError{
			StatusCode: statusCode,
			RequestID:  RequestIDFromHeader(header),
			Message:    "success response payload is not valid JSON: " + err.Error(),
			Raw:        body,
		}
	}
	return &decoded, nil
}

// ParseEmptyResponse validates a successful operation that has no response body.
func ParseEmptyResponse(response *http.Response, body []byte, expectedStatus int) error {
	statusCode, header := responseMetadata(response)
	if statusCode == expectedStatus {
		return nil
	}
	if statusCode >= http.StatusBadRequest {
		return ParseErrorResponseWithHeader(statusCode, header, body)
	}
	return &ResponseError{
		StatusCode: statusCode,
		RequestID:  RequestIDFromHeader(header),
		Message:    fmt.Sprintf("unexpected HTTP status; expected %d", expectedStatus),
		Raw:        body,
	}
}

func responseMetadata(response *http.Response) (int, http.Header) {
	if response == nil {
		return 0, nil
	}
	return response.StatusCode, response.Header
}
