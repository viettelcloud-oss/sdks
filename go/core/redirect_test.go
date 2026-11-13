package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const redirectTestBody = `{"root_password":"S3cret!"}`

func doRedirectTestRequest(t *testing.T, client *http.Client, target string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, bytes.NewReader([]byte(redirectTestBody)))
	if err != nil {
		t.Fatal(err)
	}
	return client.Do(req)
}

func TestResolveClientOptionsRefusesCrossOriginRedirect(t *testing.T) {
	var hitsA, hitsB atomic.Int32
	var bodyAtB atomic.Value
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB.Add(1)
		body, _ := io.ReadAll(r.Body)
		bodyAtB.Store(string(body))
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer serverB.Close()

	// 127.0.0.1 and localhost are different origins.
	targetB := strings.Replace(serverB.URL, "127.0.0.1", "localhost", 1) + "/clusters/"
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA.Add(1)
		http.Redirect(w, r, targetB, http.StatusTemporaryRedirect)
	}))
	defer serverA.Close()

	for _, tc := range []struct {
		name string
		opts []ClientOption
	}{
		{name: "default client"},
		{name: "custom client without CheckRedirect", opts: []ClientOption{WithHTTPClient(&http.Client{})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hitsA.Store(0)
			hitsB.Store(0)
			cfg := ResolveClientOptions(tc.opts...)
			resp, err := doRedirectTestRequest(t, cfg.HTTPClient, serverA.URL+"/clusters/")
			if resp != nil {
				_ = resp.Body.Close()
			}
			if !errors.Is(err, ErrCrossOriginRedirect) {
				t.Fatalf("error = %v, want ErrCrossOriginRedirect", err)
			}
			if got := hitsA.Load(); got != 1 {
				t.Errorf("server A requests = %d, want 1", got)
			}
			if got := hitsB.Load(); got != 0 {
				t.Errorf("server B requests = %d, want 0 (body %v)", got, bodyAtB.Load())
			}
		})
	}
}

func TestResolveClientOptionsFollowsSameOriginRedirect(t *testing.T) {
	var body string
	mux := http.NewServeMux()
	mux.HandleFunc("/old/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/new/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusCreated)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := ResolveClientOptions()
	resp, err := doRedirectTestRequest(t, cfg.HTTPClient, server.URL+"/old/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if body != redirectTestBody {
		t.Errorf("body = %q, want %q", body, redirectTestBody)
	}
}

func TestResolveClientOptionsKeepsCustomCheckRedirect(t *testing.T) {
	called := false
	custom := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		called = true
		return http.ErrUseLastResponse
	}}
	cfg := ResolveClientOptions(WithHTTPClient(custom))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "https://other.example.com/", nil)
	via := []*http.Request{httptest.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.example.com/", nil)}
	if err := cfg.HTTPClient.CheckRedirect(req, via); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("error = %v, want http.ErrUseLastResponse", err)
	}
	if !called {
		t.Error("custom CheckRedirect was not called")
	}
}

func TestSameOriginRedirectPolicy(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		allowed  bool
	}{
		{"https://api.example.com/a", "https://api.example.com/b", true},
		{"https://api.example.com/a", "https://API.example.com:443/b", true},
		{"http://api.example.com/a", "http://api.example.com:80/b", true},
		{"https://api.example.com/a", "http://api.example.com/b", false},
		{"https://api.example.com/a", "https://api.example.com:8443/b", false},
		{"https://api.example.com/a", "https://evil.api.example.com/b", false},
		{"https://api.example.com/a", "https://other.example.com/b", false},
	} {
		from, _ := url.Parse(tc.from)
		to, _ := url.Parse(tc.to)
		err := sameOriginRedirectPolicy(&http.Request{URL: to}, []*http.Request{{URL: from}})
		if tc.allowed && err != nil {
			t.Errorf("%s -> %s: error = %v, want nil", tc.from, tc.to, err)
		}
		if !tc.allowed && !errors.Is(err, ErrCrossOriginRedirect) {
			t.Errorf("%s -> %s: error = %v, want ErrCrossOriginRedirect", tc.from, tc.to, err)
		}
	}

	from, _ := url.Parse("https://api.example.com/")
	via := make([]*http.Request, maxRedirects)
	for i := range via {
		via[i] = &http.Request{URL: from}
	}
	if err := sameOriginRedirectPolicy(&http.Request{URL: from}, via); err == nil {
		t.Errorf("error = nil after %d redirects, want an error", maxRedirects)
	}
}
