package tronzap

import (
	"net/http"
	"time"
)

// Option configures a [Client]. Options are applied in the order they are
// passed to [NewClient].
type Option func(*Client)

// WithBaseURL points the client at a different API host. It accepts either a
// full URL or a bare domain, so all of these are equivalent:
//
//	tronzap.WithBaseURL("https://api.tronzap.com")
//	tronzap.WithBaseURL("api.tronzap.com")
//	tronzap.WithBaseURL("api.tronzap.com/")
//
// A missing scheme becomes https, and a trailing slash is trimmed. Use an
// explicit scheme to opt out, for example "http://localhost:8080" when testing
// against a local mock.
//
// An empty value is ignored, leaving [DefaultBaseURL] in place.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = baseURL
		}
	}
}

// WithHTTPClient sets the HTTP client used for every request, which is how you
// supply your own transport, proxy, retry wrapper or instrumentation. A nil
// client is ignored.
//
// The client is used as given and is never mutated: [WithTimeout] applies its
// deadline to a shallow copy.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithTimeout sets a per-request timeout, replacing [DefaultTimeout]. It is
// applied to a copy of the HTTP client, so passing your own client through
// [WithHTTPClient] alongside this option leaves that client untouched.
//
// A non-positive duration disables the client timeout; per-call deadlines set on
// the context still apply.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.timeout = timeout
		c.timeoutSet = true
	}
}

// WithUserAgent overrides the User-Agent header sent with every request. An
// empty value is ignored.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		if userAgent != "" {
			c.userAgent = userAgent
		}
	}
}
