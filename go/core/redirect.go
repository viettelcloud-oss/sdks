package core

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// maxRedirects matches the limit of the net/http default redirect policy.
const maxRedirects = 10

// ErrCrossOriginRedirect means the SDK refused to follow a redirect to a
// different origin (scheme, host or port). The SDK does not send the request
// body or the credentials to the redirect target.
var ErrCrossOriginRedirect = errors.New("refused to follow a redirect to a different origin")

// sameOriginRedirectPolicy is the CheckRedirect policy of SDK clients. It
// follows a redirect only when the target has the same origin as the first
// request. The net/http default policy follows 307 and 308 redirects to any
// host and sends the request body again, which can expose secrets such as
// passwords to that host.
func sameOriginRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	from := via[0].URL
	if origin(from) != origin(req.URL) {
		return fmt.Errorf("%w: from %s://%s to %s://%s",
			ErrCrossOriginRedirect, from.Scheme, from.Host, req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// origin returns the normalized scheme://host:port of u.
func origin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}
