package tronzap_test

import (
	"errors"
	"fmt"
	"testing"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

func TestAPIErrorMessage(t *testing.T) {
	withKey := &tronzap.APIError{
		Code:    tronzap.CodeInvalidTronAddress,
		Message: "Invalid TRON address",
		Key:     "invalid_tron_address.from_address",
	}
	want := "tronzap: api error 10 (invalid_tron_address.from_address): Invalid TRON address"
	if got := withKey.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	withoutKey := &tronzap.APIError{Code: 1, Message: "Incorrect token or signature"}
	want = "tronzap: api error 1: Incorrect token or signature"
	if got := withoutKey.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestHTTPErrorMessage(t *testing.T) {
	err := &tronzap.HTTPError{StatusCode: 429, Message: "too many requests"}
	if want := "tronzap: too many requests (status 429)"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestInvalidResponseErrorMessage(t *testing.T) {
	bare := &tronzap.InvalidResponseError{StatusCode: 200, Message: "missing result in response"}
	if want := "tronzap: missing result in response (status 200)"; bare.Error() != want {
		t.Errorf("Error() = %q, want %q", bare.Error(), want)
	}

	wrapped := &tronzap.InvalidResponseError{
		StatusCode: 200,
		Message:    "invalid JSON response",
		Err:        errors.New("unexpected end of JSON input"),
	}
	if want := "tronzap: invalid JSON response (status 200): unexpected end of JSON input"; wrapped.Error() != want {
		t.Errorf("Error() = %q, want %q", wrapped.Error(), want)
	}
	if !errors.Is(wrapped, tronzap.ErrInvalidResponse) {
		t.Error("errors.Is(err, ErrInvalidResponse) = false")
	}
}

func TestNetworkErrorMessage(t *testing.T) {
	cause := errors.New("connection refused")
	err := &tronzap.NetworkError{Kind: tronzap.NetworkErrorKindConnection, Err: cause}

	if want := "tronzap: connection error: connection refused"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	if !errors.Is(err, cause) {
		t.Error("the underlying error must stay reachable through Unwrap")
	}
}

func TestNetworkErrorKindString(t *testing.T) {
	tests := map[tronzap.NetworkErrorKind]string{
		tronzap.NetworkErrorKindUnknown:    "unknown",
		tronzap.NetworkErrorKindConnection: "connection",
		tronzap.NetworkErrorKindTimeout:    "timeout",
		tronzap.NetworkErrorKindTLS:        "tls",
		tronzap.NetworkErrorKindCanceled:   "canceled",
	}
	for kind, want := range tests {
		if got := kind.String(); got != want {
			t.Errorf("kind %d String() = %q, want %q", kind, got, want)
		}
	}
}

// Each error type must match its own sentinels and nobody else's, including
// after being wrapped by caller code.
func TestSentinelMatching(t *testing.T) {
	all := []error{
		tronzap.ErrAPI,
		tronzap.ErrHTTP, tronzap.ErrRateLimit, tronzap.ErrUnauthorized, tronzap.ErrServer,
		tronzap.ErrNetwork, tronzap.ErrConnection, tronzap.ErrTimeout, tronzap.ErrTLS,
		tronzap.ErrInvalidResponse,
		tronzap.ErrInvalidRequest,
	}

	tests := []struct {
		name  string
		err   error
		match []error
	}{
		{
			name:  "api error",
			err:   &tronzap.APIError{Code: 6},
			match: []error{tronzap.ErrAPI},
		},
		{
			name:  "rate limit",
			err:   &tronzap.HTTPError{StatusCode: 429},
			match: []error{tronzap.ErrHTTP, tronzap.ErrRateLimit},
		},
		{
			name:  "unauthorized",
			err:   &tronzap.HTTPError{StatusCode: 401},
			match: []error{tronzap.ErrHTTP, tronzap.ErrUnauthorized},
		},
		{
			name:  "forbidden",
			err:   &tronzap.HTTPError{StatusCode: 403},
			match: []error{tronzap.ErrHTTP, tronzap.ErrUnauthorized},
		},
		{
			name:  "server error",
			err:   &tronzap.HTTPError{StatusCode: 503},
			match: []error{tronzap.ErrHTTP, tronzap.ErrServer},
		},
		{
			name:  "other http status",
			err:   &tronzap.HTTPError{StatusCode: 418},
			match: []error{tronzap.ErrHTTP},
		},
		{
			name:  "connection failure",
			err:   &tronzap.NetworkError{Kind: tronzap.NetworkErrorKindConnection},
			match: []error{tronzap.ErrNetwork, tronzap.ErrConnection},
		},
		{
			name:  "timeout",
			err:   &tronzap.NetworkError{Kind: tronzap.NetworkErrorKindTimeout},
			match: []error{tronzap.ErrNetwork, tronzap.ErrTimeout},
		},
		{
			name:  "tls failure",
			err:   &tronzap.NetworkError{Kind: tronzap.NetworkErrorKindTLS},
			match: []error{tronzap.ErrNetwork, tronzap.ErrTLS},
		},
		{
			name:  "cancellation",
			err:   &tronzap.NetworkError{Kind: tronzap.NetworkErrorKindCanceled},
			match: []error{tronzap.ErrNetwork},
		},
		{
			name:  "unclassified network failure",
			err:   &tronzap.NetworkError{Kind: tronzap.NetworkErrorKindUnknown},
			match: []error{tronzap.ErrNetwork},
		},
		{
			name:  "invalid response",
			err:   &tronzap.InvalidResponseError{},
			match: []error{tronzap.ErrInvalidResponse},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wanted := make(map[error]bool, len(tt.match))
			for _, sentinel := range tt.match {
				wanted[sentinel] = true
			}
			// Wrapping is what callers do with returned errors, and it must not
			// break classification.
			wrapped := fmt.Errorf("calling the API: %w", tt.err)

			for _, sentinel := range all {
				for _, err := range []error{tt.err, wrapped} {
					if got := errors.Is(err, sentinel); got != wanted[sentinel] {
						t.Errorf("errors.Is(%v, %v) = %v, want %v", err, sentinel, got, wanted[sentinel])
					}
				}
			}
		})
	}
}

func TestErrorCodeValues(t *testing.T) {
	tests := map[string]int{
		"auth":                      tronzap.CodeAuth,
		"invalid service or params": tronzap.CodeInvalidServiceOrParams,
		"wallet not found":          tronzap.CodeWalletNotFound,
		"insufficient funds":        tronzap.CodeInsufficientFunds,
		"invalid tron address":      tronzap.CodeInvalidTronAddress,
		"invalid energy amount":     tronzap.CodeInvalidEnergyAmount,
		"invalid duration":          tronzap.CodeInvalidDuration,
		"transaction not found":     tronzap.CodeTransactionNotFound,
		"cannot stop subscription":  tronzap.CodeCannotStopSubscription,
		"address not activated":     tronzap.CodeAddressNotActivated,
		"address already activated": tronzap.CodeAddressAlreadyActivated,
		"aml check not found":       tronzap.CodeAMLCheckNotFound,
		"service not available":     tronzap.CodeServiceNotAvailable,
		"invalid bandwidth amount":  tronzap.CodeInvalidBandwidthAmount,
		"internal server error":     tronzap.CodeInternalServerError,
	}
	want := map[string]int{
		"auth":                      1,
		"invalid service or params": 2,
		"wallet not found":          5,
		"insufficient funds":        6,
		"invalid tron address":      10,
		"invalid energy amount":     11,
		"invalid duration":          12,
		"transaction not found":     20,
		"cannot stop subscription":  21,
		"address not activated":     24,
		"address already activated": 25,
		"aml check not found":       30,
		"service not available":     35,
		"invalid bandwidth amount":  50,
		"internal server error":     500,
	}
	for name, got := range tests {
		if got != want[name] {
			t.Errorf("%s = %d, want %d", name, got, want[name])
		}
	}
}
