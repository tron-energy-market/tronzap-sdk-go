// Package tronzap is the official Go client for the TronZap API.
//
// TronZap sells TRON energy and bandwidth, which makes USDT (TRC20) transfers
// substantially cheaper. Register for API credentials at https://tronzap.com and
// read the API reference at https://docs.tronzap.com.
//
// # Getting started
//
// Every call goes through a [Client], which is safe for concurrent use and holds
// no global state:
//
//	client := tronzap.NewClient(apiToken, apiSecret)
//
//	estimate, err := client.EstimateEnergy(ctx, tronzap.EstimateEnergyRequest{
//		FromAddress: from,
//		ToAddress:   to,
//	})
//	if err != nil {
//		return err
//	}
//
//	tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
//		Address: to,
//		Energy:  estimate.Amount,
//	})
//
// # Errors
//
// Failures are reported as ordinary Go errors carrying one of four concrete
// types: [APIError] for an application-level error code, [HTTPError] for a
// non-2xx response, [NetworkError] when no response arrived, and
// [InvalidResponseError] for a reply that could not be decoded. Classify them
// with [errors.As] for the details or [errors.Is] against the package sentinels:
//
//	var apiErr *tronzap.APIError
//	switch {
//	case errors.As(err, &apiErr) && apiErr.Code == tronzap.CodeInsufficientFunds:
//		// top up the account
//	case errors.Is(err, tronzap.ErrRateLimit):
//		// back off and retry
//	case errors.Is(err, tronzap.ErrTimeout):
//		// retry
//	}
package tronzap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Version is the SDK version, reported in the default User-Agent.
const Version = "0.1.0"

// DefaultBaseURL is the production API endpoint.
const DefaultBaseURL = "https://api.tronzap.com"

// DefaultTimeout is the per-request timeout applied unless [WithTimeout] or
// [WithHTTPClient] overrides it.
const DefaultTimeout = 30 * time.Second

// maxResponseBytes caps how much of a response body is read, so a malformed or
// hostile endpoint cannot exhaust memory.
const maxResponseBytes = 8 << 20

// API endpoints.
const (
	endpointServices           = "/v1/services"
	endpointBalance            = "/v1/balance"
	endpointAddressInfo        = "/v1/address-info"
	endpointEstimateEnergy     = "/v1/estimate-energy"
	endpointCalculate          = "/v1/calculate"
	endpointTransactionNew     = "/v1/transaction/new"
	endpointTransactionCheck   = "/v1/transaction/check"
	endpointDirectRechargeInfo = "/v1/direct-recharge-info"
	endpointAMLServices        = "/v1/aml-checks"
	endpointAMLCheckNew        = "/v1/aml-checks/new"
	endpointAMLCheckStatus     = "/v1/aml-checks/check"
	endpointAMLHistory         = "/v1/aml-checks/history"
)

// Client talks to the TronZap API. Create one with [NewClient] and share it: all
// methods are safe for concurrent use, and the client keeps no mutable state
// beyond the HTTP client it was given.
type Client struct {
	apiToken   string
	apiSecret  string
	baseURL    string
	userAgent  string
	httpClient *http.Client
	timeout    time.Duration
	timeoutSet bool
}

// NewClient returns a client authenticating with the given API token and secret.
// The token is sent as a bearer token and the secret signs each request body; get
// both from your TronZap dashboard.
//
// Defaults are [DefaultBaseURL] and [DefaultTimeout]; override them and the HTTP
// client with [WithBaseURL], [WithTimeout] and [WithHTTPClient]. Credentials are
// not validated here — an empty token or secret surfaces as an [APIError] with
// [CodeAuth] on the first call.
func NewClient(apiToken, apiSecret string, opts ...Option) *Client {
	c := &Client{
		apiToken:  apiToken,
		apiSecret: apiSecret,
		baseURL:   DefaultBaseURL,
		userAgent: "tronzap-sdk-go/" + Version,
		timeout:   DefaultTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.baseURL = normalizeBaseURL(c.baseURL); c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}

	switch {
	case c.httpClient == nil:
		c.httpClient = &http.Client{Timeout: c.timeout}
	case c.timeoutSet:
		clone := *c.httpClient
		clone.Timeout = c.timeout
		c.httpClient = &clone
	}
	return c
}

// GetServices returns the resources on sale and their current prices.
func (c *Client) GetServices(ctx context.Context) (*Services, error) {
	return fetch[Services](ctx, c, endpointServices, nil)
}

// GetBalance returns the account balance and the address it deposits to.
func (c *Client) GetBalance(ctx context.Context) (*Balance, error) {
	return fetch[Balance](ctx, c, endpointBalance, nil)
}

// GetAddressInfo returns the on-chain resources and token balances of a TRON address.
func (c *Client) GetAddressInfo(ctx context.Context, address string) (*AddressInfo, error) {
	if address == "" {
		return nil, fmt.Errorf("%w: address is required", ErrInvalidRequest)
	}
	return fetch[AddressInfo](ctx, c, endpointAddressInfo, struct {
		Address string `json:"address"`
	}{Address: address})
}

// EstimateEnergy reports how much energy a transfer between two addresses needs
// and what that energy costs. Leaving [EstimateEnergyRequest.ContractAddress]
// empty estimates against USDT (TRC20).
func (c *Client) EstimateEnergy(ctx context.Context, req EstimateEnergyRequest) (*EnergyEstimate, error) {
	if req.FromAddress == "" || req.ToAddress == "" {
		return nil, fmt.Errorf("%w: FromAddress and ToAddress are required", ErrInvalidRequest)
	}
	return fetch[EnergyEstimate](ctx, c, endpointEstimateEnergy, struct {
		FromAddress     string `json:"from_address"`
		ToAddress       string `json:"to_address"`
		ContractAddress string `json:"contract_address,omitempty"`
	}{
		FromAddress:     req.FromAddress,
		ToAddress:       req.ToAddress,
		ContractAddress: req.ContractAddress,
	})
}

// Calculate prices an energy purchase without creating a transaction.
func (c *Client) Calculate(ctx context.Context, req CalculateRequest) (*Calculation, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: Address is required", ErrInvalidRequest)
	}
	return fetch[Calculation](ctx, c, endpointCalculate, struct {
		Address  string `json:"address"`
		Amount   int64  `json:"amount"`
		Duration int    `json:"duration"`
	}{
		Address:  req.Address,
		Amount:   req.Energy,
		Duration: durationOrDefault(req.Duration),
	})
}

// CreateEnergyTransaction buys energy for an address. Set
// [EnergyTransactionRequest.ActivateAddress] to activate the address in the same
// call when it is not active yet.
func (c *Client) CreateEnergyTransaction(ctx context.Context, req EnergyTransactionRequest) (*Transaction, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: Address is required", ErrInvalidRequest)
	}
	return c.createTransaction(ctx, newTransactionParams{
		Service:    ServiceEnergy,
		ExternalID: req.ExternalID,
		Params: transactionParams{
			Address:         req.Address,
			Amounts:         &Amounts{Energy: req.Energy},
			Duration:        durationOrDefault(req.Duration),
			ActivateAddress: req.ActivateAddress,
		},
	})
}

// CreateBandwidthTransaction buys bandwidth for an address.
func (c *Client) CreateBandwidthTransaction(ctx context.Context, req BandwidthTransactionRequest) (*Transaction, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: Address is required", ErrInvalidRequest)
	}
	return c.createTransaction(ctx, newTransactionParams{
		Service:    ServiceBandwidth,
		ExternalID: req.ExternalID,
		Params: transactionParams{
			Address:  req.Address,
			Amounts:  &Amounts{Bandwidth: req.Bandwidth},
			Duration: 1,
		},
	})
}

// CreateResourceBundleTransaction buys energy and bandwidth in a single
// transaction, which is cheaper than buying each separately.
func (c *Client) CreateResourceBundleTransaction(ctx context.Context, req ResourceBundleTransactionRequest) (*Transaction, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: Address is required", ErrInvalidRequest)
	}
	return c.createTransaction(ctx, newTransactionParams{
		Service:    ServiceResourceBundle,
		ExternalID: req.ExternalID,
		Params: transactionParams{
			Address:         req.Address,
			Amounts:         &Amounts{Energy: req.Energy, Bandwidth: req.Bandwidth},
			Duration:        durationOrDefault(req.Duration),
			ActivateAddress: req.ActivateAddress,
		},
	})
}

// CreateAddressActivationTransaction activates a TRON address. An address must
// be activated once before it can hold resources.
func (c *Client) CreateAddressActivationTransaction(ctx context.Context, req AddressActivationRequest) (*Transaction, error) {
	if req.Address == "" {
		return nil, fmt.Errorf("%w: Address is required", ErrInvalidRequest)
	}
	return c.createTransaction(ctx, newTransactionParams{
		Service:    ServiceActivateAddress,
		ExternalID: req.ExternalID,
		Params:     transactionParams{Address: req.Address},
	})
}

// CheckTransaction returns the current state of a transaction. Look it up by the
// API's identifier, by your own external identifier, or both.
func (c *Client) CheckTransaction(ctx context.Context, req CheckTransactionRequest) (*Transaction, error) {
	if req.ID == "" && req.ExternalID == "" {
		return nil, fmt.Errorf("%w: either ID or ExternalID is required", ErrInvalidRequest)
	}
	return fetch[Transaction](ctx, c, endpointTransactionCheck, struct {
		ID         string `json:"id,omitempty"`
		ExternalID string `json:"external_id,omitempty"`
	}{ID: req.ID, ExternalID: req.ExternalID})
}

// GetDirectRechargeInfo returns the address to pay for direct energy recharge and
// the rates it is delivered at.
func (c *Client) GetDirectRechargeInfo(ctx context.Context) (*DirectRechargeInfo, error) {
	return fetch[DirectRechargeInfo](ctx, c, endpointDirectRechargeInfo, nil)
}

// GetAMLServices returns the available AML screening products and their prices.
func (c *Client) GetAMLServices(ctx context.Context) ([]AMLService, error) {
	services, err := fetch[[]AMLService](ctx, c, endpointAMLServices, nil)
	if err != nil {
		return nil, err
	}
	return *services, nil
}

// CreateAMLCheck starts an AML screening of an address or a transaction hash.
// Screening runs asynchronously: poll [Client.CheckAMLStatus] until the status is
// [AMLStatusCompleted].
func (c *Client) CreateAMLCheck(ctx context.Context, req AMLCheckRequest) (*AMLCheck, error) {
	switch {
	case req.Type == "":
		return nil, fmt.Errorf("%w: Type is required", ErrInvalidRequest)
	case req.Network == "":
		return nil, fmt.Errorf("%w: Network is required", ErrInvalidRequest)
	case req.Address == "":
		return nil, fmt.Errorf("%w: Address is required", ErrInvalidRequest)
	}
	return fetch[AMLCheck](ctx, c, endpointAMLCheckNew, struct {
		Type      string `json:"type"`
		Network   string `json:"network"`
		Address   string `json:"address"`
		Hash      string `json:"hash,omitempty"`
		Direction string `json:"direction,omitempty"`
	}{
		Type:      req.Type,
		Network:   req.Network,
		Address:   req.Address,
		Hash:      req.Hash,
		Direction: req.Direction,
	})
}

// CheckAMLStatus returns the current state and, once complete, the result of an
// AML check.
func (c *Client) CheckAMLStatus(ctx context.Context, id string) (*AMLCheck, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: id is required", ErrInvalidRequest)
	}
	return fetch[AMLCheck](ctx, c, endpointAMLCheckStatus, struct {
		ID string `json:"id"`
	}{ID: id})
}

// GetAMLHistory returns one page of past AML checks, newest first.
func (c *Client) GetAMLHistory(ctx context.Context, req AMLHistoryRequest) (*AMLHistory, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 {
		perPage = 10
	}
	return fetch[AMLHistory](ctx, c, endpointAMLHistory, struct {
		Page    int    `json:"page"`
		PerPage int    `json:"per_page"`
		Status  string `json:"status,omitempty"`
	}{Page: page, PerPage: perPage, Status: req.Status})
}

// Do sends a signed POST request to an arbitrary API endpoint and decodes the
// response's "result" field into result, which may be nil to discard it. Use it
// to reach endpoints this SDK does not wrap yet; prefer the typed methods
// otherwise.
//
// The endpoint is a path such as "/v1/balance". A nil params encodes as an empty
// JSON object.
func (c *Client) Do(ctx context.Context, endpoint string, params, result any) error {
	body := []byte("{}")
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("%w: cannot encode params: %w", ErrInvalidRequest, err)
		}
		body = encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("X-Signature", c.sign(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return newNetworkError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return newNetworkError(err)
	}
	return decodeResponse(resp.StatusCode, raw, result)
}

// sign returns the request signature: the hex-encoded SHA-256 of the request
// body concatenated with the API secret.
func (c *Client) sign(body []byte) string {
	digest := sha256.New()
	digest.Write(body)
	digest.Write([]byte(c.apiSecret))
	return hex.EncodeToString(digest.Sum(nil))
}

func (c *Client) createTransaction(ctx context.Context, params newTransactionParams) (*Transaction, error) {
	return fetch[Transaction](ctx, c, endpointTransactionNew, params)
}

// newTransactionParams is the wire format of a /v1/transaction/new request.
type newTransactionParams struct {
	Service    string            `json:"service"`
	Params     transactionParams `json:"params"`
	ExternalID string            `json:"external_id,omitempty"`
}

type transactionParams struct {
	Address         string   `json:"address"`
	Amounts         *Amounts `json:"amounts,omitempty"`
	Duration        int      `json:"duration,omitempty"`
	ActivateAddress bool     `json:"activate_address,omitempty"`
}

func fetch[T any](ctx context.Context, c *Client, endpoint string, params any) (*T, error) {
	var result T
	if err := c.Do(ctx, endpoint, params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// decodeResponse turns an API reply into either a decoded result or an error.
//
// The order of checks matters: the API reports some application-level failures
// with a 2xx status and others with a 4xx status, so a decodable payload with a
// non-zero code always wins over the HTTP status.
func decodeResponse(statusCode int, raw []byte, result any) error {
	var envelope struct {
		Code      *int            `json:"code"`
		Error     string          `json:"error"`
		Key       string          `json:"key"`
		RequestID string          `json:"request_id"`
		Result    json.RawMessage `json:"result"`
	}
	decodeErr := json.Unmarshal(raw, &envelope)

	switch {
	case decodeErr == nil && (envelope.Code == nil || *envelope.Code != 0):
		code := 1
		if envelope.Code != nil {
			code = *envelope.Code
		}
		message := envelope.Error
		if message == "" {
			message = "Unknown API error"
		}
		return &APIError{
			Code:       code,
			Message:    message,
			Key:        envelope.Key,
			RequestID:  envelope.RequestID,
			StatusCode: statusCode,
		}
	case decodeErr != nil && json.Valid(raw):
		// Well-formed JSON that is not an object cannot carry a code, which the
		// API treats the same as an unidentified failure.
		return &APIError{Code: 1, Message: "Unknown API error", StatusCode: statusCode}
	}

	if statusCode < 200 || statusCode >= 300 {
		return newHTTPError(statusCode, string(raw))
	}
	if decodeErr != nil {
		return &InvalidResponseError{
			StatusCode: statusCode,
			Message:    "invalid JSON response",
			Body:       string(raw),
			Err:        decodeErr,
		}
	}
	if len(envelope.Result) == 0 || bytes.Equal(envelope.Result, []byte("null")) {
		return &InvalidResponseError{
			StatusCode: statusCode,
			Message:    "missing result in response",
			Body:       string(raw),
		}
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return &InvalidResponseError{
			StatusCode: statusCode,
			Message:    "unexpected result in response",
			Body:       string(raw),
			Err:        err,
		}
	}
	return nil
}

func newHTTPError(statusCode int, body string) *HTTPError {
	message := fmt.Sprintf("HTTP error %d", statusCode)
	switch {
	case statusCode == http.StatusTooManyRequests:
		message = "too many requests"
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		message = "unauthorized"
	case statusCode >= http.StatusInternalServerError:
		message = "server error"
	}
	return &HTTPError{StatusCode: statusCode, Message: message, Body: body}
}

// normalizeBaseURL turns a full URL or a bare domain into a URL the endpoint
// paths can be appended to.
func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + strings.TrimPrefix(baseURL, "//")
	}
	return baseURL
}

func durationOrDefault(duration int) int {
	if duration < 1 {
		return 1
	}
	return duration
}
