package tronzap

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"
)

// API error codes returned in the "code" field of an API response.
//
// A successful response always carries code 0; any other value is reported as
// an [APIError]. Compare against [APIError.Code] to branch on a specific
// failure.
const (
	// CodeAuth means the API token or the request signature is not valid.
	CodeAuth = 1
	// CodeInvalidServiceOrParams means the service name or its parameters are not valid.
	CodeInvalidServiceOrParams = 2
	// CodeWalletNotFound means the internal wallet was not found; contact support.
	CodeWalletNotFound = 5
	// CodeInsufficientFunds means the account balance is too low for the request.
	CodeInsufficientFunds = 6
	// CodeInvalidTronAddress means a supplied TRON address is malformed, or that
	// the address already has an active subscription.
	CodeInvalidTronAddress = 10
	// CodeInvalidEnergyAmount means the requested energy amount is out of range.
	CodeInvalidEnergyAmount = 11
	// CodeInvalidDuration means the requested duration is not supported.
	CodeInvalidDuration = 12
	// CodeTransactionNotFound means no transaction or subscription matches the
	// given id or external id. The API reports it under the key
	// "subscription_not_found".
	CodeTransactionNotFound = 20
	// CodeCannotStopSubscription means the subscription cannot be stopped, for
	// example because it has a transactions limit.
	CodeCannotStopSubscription = 21
	// CodeAddressNotActivated means the address must be activated before use.
	CodeAddressNotActivated = 24
	// CodeAddressAlreadyActivated means the address is already activated; no action needed.
	CodeAddressAlreadyActivated = 25
	// CodeAMLCheckNotFound means no AML check matches the given id.
	CodeAMLCheckNotFound = 30
	// CodeServiceNotAvailable means the service is temporarily unavailable.
	CodeServiceNotAvailable = 35
	// CodeInvalidBandwidthAmount means the requested bandwidth amount is out of range.
	CodeInvalidBandwidthAmount = 50
	// CodeBulkNotFound means no bulk order matches the given id or external id.
	// A calculation that was never started is not an order.
	CodeBulkNotFound = 90
	// CodeBulkExpired means the bulk calculation expired; calculate again. A
	// calculation is valid for 10 minutes.
	CodeBulkExpired = 91
	// CodeBulkPriceChanged means the price changed since the calculation. The new
	// calculation is in the error's result, see [APIError.DecodeResult]; start the
	// order with its ID.
	CodeBulkPriceChanged = 92
	// CodeBulkTooManyItems means a calculation has more than 10 000 rows.
	CodeBulkTooManyItems = 93
	// CodeBulkNoValidItems means no row of a calculation is valid. The rejected
	// rows are in the error's result, see [APIError.DecodeResult].
	CodeBulkNoValidItems = 94
	// CodeBulkResourceNotSupported means the resource cannot be ordered in bulk;
	// only energy can.
	CodeBulkResourceNotSupported = 95
	// CodePayloadTooLarge means the request body is too large (HTTP 413); send
	// fewer rows in one calculation.
	CodePayloadTooLarge = 96
	// CodeRateLimited means too many requests (HTTP 429). Wait
	// [APIError.RetryAfter], when it is set, and retry.
	CodeRateLimited = 429
	// CodeInternalServerError means the API failed internally; contact support if it persists.
	CodeInternalServerError = 500
)

// Sentinel errors for classifying failures with [errors.Is]. Every error
// returned by a Client method matches exactly one of ErrAPI, ErrHTTP,
// ErrNetwork, ErrInvalidResponse or ErrInvalidRequest, and may additionally
// match one of the narrower sentinels below.
var (
	// ErrAPI matches any [APIError]: the API answered with a non-zero code.
	ErrAPI = errors.New("tronzap: api error")

	// ErrHTTP matches any [HTTPError]: a non-2xx response without a usable API payload.
	ErrHTTP = errors.New("tronzap: http error")
	// ErrRateLimit matches an [HTTPError] with status 429. A rate limit reported by
	// the API itself is an [APIError] with [CodeRateLimited] instead.
	ErrRateLimit = errors.New("tronzap: too many requests")
	// ErrUnauthorized matches an [HTTPError] with status 401 or 403.
	ErrUnauthorized = errors.New("tronzap: unauthorized")
	// ErrServer matches an [HTTPError] with a 5xx status.
	ErrServer = errors.New("tronzap: server error")

	// ErrNetwork matches any [NetworkError]: the request never produced a response.
	ErrNetwork = errors.New("tronzap: network error")
	// ErrConnection matches a [NetworkError] caused by a failed DNS lookup or connect.
	ErrConnection = errors.New("tronzap: connection failed")
	// ErrTimeout matches a [NetworkError] caused by a deadline being exceeded.
	ErrTimeout = errors.New("tronzap: request timed out")
	// ErrTLS matches a [NetworkError] caused by TLS or certificate verification.
	ErrTLS = errors.New("tronzap: tls error")

	// ErrInvalidResponse matches an [InvalidResponseError]: a 2xx response the SDK could not decode.
	ErrInvalidResponse = errors.New("tronzap: invalid response")

	// ErrInvalidRequest reports arguments rejected by the SDK before any request was sent.
	ErrInvalidRequest = errors.New("tronzap: invalid request")
)

// APIError is an application-level error: the API returned a well-formed
// payload whose code is not 0. It is returned regardless of the HTTP status,
// because the API reports some failures with a 2xx status.
type APIError struct {
	// Code is the API error code, one of the Code* constants.
	Code int
	// Message is the human-readable message from the "error" field.
	Message string
	// Key is the machine-readable error alias from the "key" field, such as
	// "invalid_tron_address" or the sub-key "invalid_tron_address.from_address".
	// It may be empty.
	Key string
	// RequestID identifies the request on the API side; quote it in support requests.
	RequestID string
	// StatusCode is the HTTP status the payload arrived with.
	StatusCode int
	// Result is the "result" field sent with the error, such as the new
	// calculation of [CodeBulkPriceChanged], or nil. Decode it with
	// [APIError.DecodeResult].
	Result json.RawMessage
	// RetryAfter is how long to wait before retrying, from the Retry-After
	// header or the "retry_after" field, or zero when the API sent neither.
	RetryAfter time.Duration
}

// DecodeResult decodes [APIError.Result] into v, such as a *[BulkCalculation]
// for [CodeBulkPriceChanged] or [CodeBulkNoValidItems]. It reports whether the
// error carried a result.
func (e *APIError) DecodeResult(v any) (bool, error) {
	if len(e.Result) == 0 || bytes.Equal(e.Result, []byte("null")) {
		return false, nil
	}
	if err := json.Unmarshal(e.Result, v); err != nil {
		return false, err
	}
	return true, nil
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("tronzap: api error %d (%s): %s", e.Code, e.Key, e.Message)
	}
	return fmt.Sprintf("tronzap: api error %d: %s", e.Code, e.Message)
}

// Is reports whether the error matches [ErrAPI].
func (e *APIError) Is(target error) bool { return target == ErrAPI }

// HTTPError is a transport-level failure: a non-2xx response that carried no
// usable API payload.
type HTTPError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int
	// Message describes the failure.
	Message string
	// Body is the raw response body, truncated to nothing more than what was read.
	Body string
	// RetryAfter is how long to wait before retrying, from the Retry-After
	// header, or zero when it was not sent.
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("tronzap: %s (status %d)", e.Message, e.StatusCode)
}

// Is reports whether the error matches [ErrHTTP] or the narrower sentinel that
// corresponds to its status code.
func (e *HTTPError) Is(target error) bool {
	switch target {
	case ErrHTTP:
		return true
	case ErrRateLimit:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
	case ErrServer:
		return e.StatusCode >= http.StatusInternalServerError
	}
	return false
}

// InvalidResponseError reports a successful HTTP response the SDK could not
// interpret: malformed JSON, a missing "result" field, or a result that does
// not fit the expected model.
type InvalidResponseError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int
	// Message describes what was wrong with the response.
	Message string
	// Body is the raw response body.
	Body string
	// Err is the underlying decoding error, if any.
	Err error
}

// Error implements the error interface.
func (e *InvalidResponseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("tronzap: %s (status %d): %v", e.Message, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("tronzap: %s (status %d)", e.Message, e.StatusCode)
}

// Is reports whether the error matches [ErrInvalidResponse].
func (e *InvalidResponseError) Is(target error) bool { return target == ErrInvalidResponse }

// Unwrap returns the underlying decoding error.
func (e *InvalidResponseError) Unwrap() error { return e.Err }

// NetworkErrorKind classifies why a request never produced a response.
type NetworkErrorKind int

// Network error kinds.
const (
	// NetworkErrorKindUnknown is an unclassified transport failure.
	NetworkErrorKindUnknown NetworkErrorKind = iota
	// NetworkErrorKindConnection covers DNS resolution and connect failures.
	NetworkErrorKindConnection
	// NetworkErrorKindTimeout covers exceeded deadlines and timeouts.
	NetworkErrorKindTimeout
	// NetworkErrorKindTLS covers TLS handshake and certificate verification failures.
	NetworkErrorKindTLS
	// NetworkErrorKindCanceled means the caller's context was canceled.
	NetworkErrorKindCanceled
)

// String returns the kind as a lowercase word.
func (k NetworkErrorKind) String() string {
	switch k {
	case NetworkErrorKindConnection:
		return "connection"
	case NetworkErrorKindTimeout:
		return "timeout"
	case NetworkErrorKindTLS:
		return "tls"
	case NetworkErrorKindCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// NetworkError reports that the request never produced an HTTP response.
// Unwrapping it yields the underlying transport error, so context failures stay
// detectable with errors.Is(err, context.Canceled) and
// errors.Is(err, context.DeadlineExceeded).
type NetworkError struct {
	// Kind classifies the failure.
	Kind NetworkErrorKind
	// Err is the underlying transport error.
	Err error
}

// Error implements the error interface.
func (e *NetworkError) Error() string {
	return fmt.Sprintf("tronzap: %s error: %v", e.Kind, e.Err)
}

// Is reports whether the error matches [ErrNetwork] or the sentinel that
// corresponds to its kind.
func (e *NetworkError) Is(target error) bool {
	switch target {
	case ErrNetwork:
		return true
	case ErrConnection:
		return e.Kind == NetworkErrorKindConnection
	case ErrTimeout:
		return e.Kind == NetworkErrorKindTimeout
	case ErrTLS:
		return e.Kind == NetworkErrorKindTLS
	}
	return false
}

// Unwrap returns the underlying transport error.
func (e *NetworkError) Unwrap() error { return e.Err }

func newNetworkError(err error) *NetworkError {
	return &NetworkError{Kind: classifyNetworkError(err), Err: err}
}

func classifyNetworkError(err error) NetworkErrorKind {
	switch {
	case errors.Is(err, context.Canceled):
		return NetworkErrorKindCanceled
	case isTLSError(err):
		return NetworkErrorKindTLS
	case isTimeoutError(err):
		return NetworkErrorKindTimeout
	case isConnectionError(err):
		return NetworkErrorKindConnection
	default:
		return NetworkErrorKindUnknown
	}
}

func isTLSError(err error) bool {
	var certVerify *tls.CertificateVerificationError
	var recordHeader tls.RecordHeaderError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var certInvalid x509.CertificateInvalidError
	return errors.As(err, &certVerify) ||
		errors.As(err, &recordHeader) ||
		errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostname) ||
		errors.As(err, &certInvalid)
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isConnectionError(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	for _, errno := range []syscall.Errno{
		syscall.ECONNREFUSED,
		syscall.ECONNRESET,
		syscall.ECONNABORTED,
		syscall.EHOSTUNREACH,
		syscall.ENETUNREACH,
		syscall.EPIPE,
	} {
		if errors.Is(err, errno) {
			return true
		}
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}
