package core

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

const defaultHTTPTimeout = 30 * time.Second

// ClientOption configures the service-independent parts of an SDK client.
// Service packages expose thin wrappers so callers can keep using options such
// as blockstorage.WithPAT and server.WithHTTPClient.
type ClientOption func(*clientOptions)

type clientOptions struct {
	httpClient      *http.Client
	retry           *RetryConfig
	retryAllMethods bool
	logger          *slog.Logger
	userAgent       string
	patToken        string
	requestEditors  []RequestEditorFn
}

// ResolvedClientConfig contains the shared HTTP client and request editors a
// service NewClient adapter passes to its internal generated client.
type ResolvedClientConfig struct {
	HTTPClient     *http.Client
	RequestEditors []RequestEditorFn
}

// ResolveClientOptions applies options and constructs a client configuration.
// A custom http.Client is cloned so transport wrapping does not mutate the
// caller's instance; its jar, redirect policy, timeout and other fields remain
// intact.
func ResolveClientOptions(opts ...ClientOption) ResolvedClientConfig {
	var cfg clientOptions
	for _, option := range opts {
		if option != nil {
			option(&cfg)
		}
	}

	requestEditors := make([]RequestEditorFn, 0, 3+len(cfg.requestEditors))
	if cfg.patToken != "" {
		token := cfg.patToken
		requestEditors = append(requestEditors, func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Token "+token)
			return nil
		})
	}

	userAgent := "viettelcloud-go-sdk/" + Version
	if cfg.userAgent != "" {
		userAgent = cfg.userAgent + " " + userAgent
	}
	requestEditors = append(requestEditors, func(_ context.Context, req *http.Request) error {
		req.Header.Set("User-Agent", userAgent)
		return nil
	})
	requestEditors = append(requestEditors, func(ctx context.Context, req *http.Request) error {
		if req.Method == http.MethodPost {
			if key := IdempotencyKeyFromContext(ctx); key != "" {
				req.Header.Set("Idempotency-Key", key)
			}
		}
		return nil
	})
	requestEditors = append(requestEditors, cfg.requestEditors...)

	var httpClient *http.Client
	if cfg.httpClient != nil {
		clone := *cfg.httpClient
		httpClient = &clone
	} else {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	transport := httpClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if cfg.retry != nil {
		transport = &RetryRoundTripper{
			Inner:           transport,
			Config:          cfg.retry,
			RetryAllMethods: cfg.retryAllMethods,
		}
	}
	if cfg.logger != nil {
		transport = &LoggingRoundTripper{Inner: transport, Logger: cfg.logger}
	}
	transport = &responseLimitRoundTripper{inner: transport, maxBytes: DefaultMaxResponseBytes}
	httpClient.Transport = transport

	return ResolvedClientConfig{
		HTTPClient:     httpClient,
		RequestEditors: requestEditors,
	}
}

// WithPAT sets the Personal Access Token used by the PATAuth scheme.
func WithPAT(token string) ClientOption {
	return func(options *clientOptions) {
		options.patToken = token
	}
}

// WithHTTPClient supplies a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(options *clientOptions) {
		options.httpClient = client
	}
}

// WithRetry enables retries with the supplied configuration.
func WithRetry(config *RetryConfig) ClientOption {
	return func(options *clientOptions) {
		options.retry = config
	}
}

// WithRetryAllMethods enables retrying methods such as POST and PATCH.
func WithRetryAllMethods(enabled bool) ClientOption {
	return func(options *clientOptions) {
		options.retryAllMethods = enabled
	}
}

// WithUserAgent prepends an application identifier to the SDK User-Agent.
func WithUserAgent(userAgent string) ClientOption {
	return func(options *clientOptions) {
		options.userAgent = userAgent
	}
}

// WithLogger enables structured request/response logging.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(options *clientOptions) {
		options.logger = logger
	}
}

// WithRequestEditor appends a custom editor that runs on every request.
func WithRequestEditor(editor RequestEditorFn) ClientOption {
	return func(options *clientOptions) {
		if editor != nil {
			options.requestEditors = append(options.requestEditors, editor)
		}
	}
}
