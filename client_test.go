package tronzap_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

const (
	testToken  = "test-token"
	testSecret = "test-secret"
)

// capture records what the server received, so tests can assert on the wire
// format. It is guarded by a mutex because the client is exercised concurrently.
type capture struct {
	mu       sync.Mutex
	path     string
	method   string
	header   http.Header
	body     string
	requests int
}

func (c *capture) record(r *http.Request, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = r.URL.Path
	c.method = r.Method
	c.header = r.Header.Clone()
	c.body = body
	c.requests++
}

func (c *capture) Path() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.path
}

func (c *capture) Method() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.method
}

func (c *capture) Body() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

func (c *capture) Requests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

func (c *capture) HeaderValue(name string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.header.Get(name)
}

// newServer starts a test API that answers every request with the given raw body
// and status, and returns a client pointed at it plus the captured request.
func newServer(t *testing.T, status int, response string, opts ...tronzap.Option) (*tronzap.Client, *capture) {
	t.Helper()

	got := &capture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		got.record(r, string(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := io.WriteString(w, response); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	opts = append([]tronzap.Option{tronzap.WithBaseURL(server.URL)}, opts...)
	return tronzap.NewClient(testToken, testSecret, opts...), got
}

// newBlockingServer starts a test API whose handler never answers until the test
// finishes, for exercising timeouts and cancellation. The handler is released
// before the server is closed, otherwise Close would wait on it forever.
func newBlockingServer(t *testing.T) string {
	t.Helper()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	return server.URL
}

// ok wraps a result in a successful API envelope.
func ok(result string) string {
	return `{"code":0,"request_id":"req-1","result":` + result + `}`
}

// assertJSONBody compares a request body against expected JSON, ignoring key order.
func assertJSONBody(t *testing.T, got, want string) {
	t.Helper()

	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("request body is not valid JSON: %v (body %q)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("expected body is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("request body:\n got %s\nwant %s", got, want)
	}
}

func TestClientSendsSignedRequest(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(`{"balance":1,"address":"T1"}`))

	if _, err := client.GetBalance(context.Background()); err != nil {
		t.Fatalf("GetBalance: %v", err)
	}

	if got.Method() != http.MethodPost {
		t.Errorf("method = %q, want POST", got.Method())
	}
	if got.Path() != "/v1/balance" {
		t.Errorf("path = %q, want /v1/balance", got.Path())
	}
	if got.Body() != "{}" {
		t.Errorf("body = %q, want {}", got.Body())
	}
	if authorization := got.HeaderValue("Authorization"); authorization != "Bearer "+testToken {
		t.Errorf("Authorization = %q", authorization)
	}
	if contentType := got.HeaderValue("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type = %q", contentType)
	}
	if userAgent := got.HeaderValue("User-Agent"); !strings.HasPrefix(userAgent, "tronzap-sdk-go/") {
		t.Errorf("User-Agent = %q", userAgent)
	}

	digest := sha256.Sum256([]byte(got.Body() + testSecret))
	if want := hex.EncodeToString(digest[:]); got.HeaderValue("X-Signature") != want {
		t.Errorf("X-Signature = %q, want %q", got.HeaderValue("X-Signature"), want)
	}
}

func TestSignatureCoversRequestBody(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(`{"id":"tx-1"}`))

	_, err := client.CreateEnergyTransaction(context.Background(), tronzap.EnergyTransactionRequest{
		Address: "TAddress",
		Energy:  65000,
	})
	if err != nil {
		t.Fatalf("CreateEnergyTransaction: %v", err)
	}

	digest := sha256.Sum256([]byte(got.Body() + testSecret))
	if want := hex.EncodeToString(digest[:]); got.HeaderValue("X-Signature") != want {
		t.Errorf("signature does not match the body actually sent:\nbody %s", got.Body())
	}
}

func TestSignatureCoversNonASCIIBody(t *testing.T) {
	const externalID = "pedido-año-订单-😀"
	client, got := newServer(t, http.StatusOK, ok(`{"id":"tx-1"}`))

	_, err := client.CreateEnergyTransaction(context.Background(), tronzap.EnergyTransactionRequest{
		Address:    "TAddress",
		Energy:     65000,
		ExternalID: externalID,
	})
	if err != nil {
		t.Fatalf("CreateEnergyTransaction: %v", err)
	}

	var body struct {
		ExternalID string `json:"external_id"`
	}
	if err := json.Unmarshal([]byte(got.Body()), &body); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if body.ExternalID != externalID {
		t.Errorf("external_id = %q, want %q", body.ExternalID, externalID)
	}
	digest := sha256.Sum256([]byte(got.Body() + testSecret))
	if want := hex.EncodeToString(digest[:]); got.HeaderValue("X-Signature") != want {
		t.Errorf("signature does not match the body actually sent:\nbody %s", got.Body())
	}
}

func TestRequestBodies(t *testing.T) {
	tests := []struct {
		name string
		call func(*tronzap.Client) error
		// result is the JSON the fake API returns, which has to fit the model
		// the method under test decodes into.
		result   string
		wantPath string
		wantBody string
	}{
		{
			name: "services",
			call: func(c *tronzap.Client) error {
				_, err := c.GetServices(context.Background())
				return err
			},
			wantPath: "/v1/services",
			wantBody: `{}`,
		},
		{
			name: "address info",
			call: func(c *tronzap.Client) error {
				_, err := c.GetAddressInfo(context.Background(), "TAddress")
				return err
			},
			wantPath: "/v1/address-info",
			wantBody: `{"address":"TAddress"}`,
		},
		{
			name: "estimate energy omits the default contract",
			call: func(c *tronzap.Client) error {
				_, err := c.EstimateEnergy(context.Background(), tronzap.EstimateEnergyRequest{
					FromAddress: "TFrom",
					ToAddress:   "TTo",
				})
				return err
			},
			wantPath: "/v1/estimate-energy",
			wantBody: `{"from_address":"TFrom","to_address":"TTo"}`,
		},
		{
			name: "estimate energy with an explicit contract",
			call: func(c *tronzap.Client) error {
				_, err := c.EstimateEnergy(context.Background(), tronzap.EstimateEnergyRequest{
					FromAddress:     "TFrom",
					ToAddress:       "TTo",
					ContractAddress: tronzap.DefaultContractAddress,
				})
				return err
			},
			wantPath: "/v1/estimate-energy",
			wantBody: `{"from_address":"TFrom","to_address":"TTo","contract_address":"` + tronzap.DefaultContractAddress + `"}`,
		},
		{
			name: "calculate defaults the duration to one hour",
			call: func(c *tronzap.Client) error {
				_, err := c.Calculate(context.Background(), tronzap.CalculateRequest{
					Address: "TAddress",
					Energy:  65000,
				})
				return err
			},
			wantPath: "/v1/calculate",
			wantBody: `{"address":"TAddress","amount":65000,"duration":1}`,
		},
		{
			name: "energy transaction",
			call: func(c *tronzap.Client) error {
				_, err := c.CreateEnergyTransaction(context.Background(), tronzap.EnergyTransactionRequest{
					Address:         "TAddress",
					Energy:          65000,
					Duration:        24,
					ExternalID:      "ext-1",
					ActivateAddress: true,
				})
				return err
			},
			wantPath: "/v1/transaction/new",
			wantBody: `{"service":"energy","external_id":"ext-1","params":{"address":"TAddress","amounts":{"energy":65000},"duration":24,"activate_address":true}}`,
		},
		{
			name: "energy transaction without optional fields",
			call: func(c *tronzap.Client) error {
				_, err := c.CreateEnergyTransaction(context.Background(), tronzap.EnergyTransactionRequest{
					Address: "TAddress",
					Energy:  65000,
				})
				return err
			},
			wantPath: "/v1/transaction/new",
			wantBody: `{"service":"energy","params":{"address":"TAddress","amounts":{"energy":65000},"duration":1}}`,
		},
		{
			name: "bandwidth transaction",
			call: func(c *tronzap.Client) error {
				_, err := c.CreateBandwidthTransaction(context.Background(), tronzap.BandwidthTransactionRequest{
					Address:    "TAddress",
					Bandwidth:  345,
					ExternalID: "bw-1",
				})
				return err
			},
			wantPath: "/v1/transaction/new",
			wantBody: `{"service":"bandwidth","external_id":"bw-1","params":{"address":"TAddress","amounts":{"bandwidth":345},"duration":1}}`,
		},
		{
			name: "resource bundle transaction",
			call: func(c *tronzap.Client) error {
				_, err := c.CreateResourceBundleTransaction(context.Background(), tronzap.ResourceBundleTransactionRequest{
					Address:         "TAddress",
					Energy:          65000,
					Bandwidth:       345,
					ExternalID:      "bundle-1",
					ActivateAddress: true,
				})
				return err
			},
			wantPath: "/v1/transaction/new",
			wantBody: `{"service":"resource_bundle","external_id":"bundle-1","params":{"address":"TAddress","amounts":{"energy":65000,"bandwidth":345},"duration":1,"activate_address":true}}`,
		},
		{
			name: "address activation sends no amounts",
			call: func(c *tronzap.Client) error {
				_, err := c.CreateAddressActivationTransaction(context.Background(), tronzap.AddressActivationRequest{
					Address:    "TAddress",
					ExternalID: "act-1",
				})
				return err
			},
			wantPath: "/v1/transaction/new",
			wantBody: `{"service":"activate_address","external_id":"act-1","params":{"address":"TAddress"}}`,
		},
		{
			name: "check transaction by id",
			call: func(c *tronzap.Client) error {
				_, err := c.CheckTransaction(context.Background(), tronzap.CheckTransactionRequest{ID: "tx-1"})
				return err
			},
			wantPath: "/v1/transaction/check",
			wantBody: `{"id":"tx-1"}`,
		},
		{
			name: "check transaction by external id",
			call: func(c *tronzap.Client) error {
				_, err := c.CheckTransaction(context.Background(), tronzap.CheckTransactionRequest{ExternalID: "ext-1"})
				return err
			},
			wantPath: "/v1/transaction/check",
			wantBody: `{"external_id":"ext-1"}`,
		},
		{
			name: "aml check for an address",
			call: func(c *tronzap.Client) error {
				_, err := c.CreateAMLCheck(context.Background(), tronzap.AMLCheckRequest{
					Type:    tronzap.AMLTypeAddress,
					Network: "TRX",
					Address: "TAddress",
				})
				return err
			},
			wantPath: "/v1/aml-checks/new",
			wantBody: `{"type":"address","network":"TRX","address":"TAddress"}`,
		},
		{
			name: "aml check for a hash",
			call: func(c *tronzap.Client) error {
				_, err := c.CreateAMLCheck(context.Background(), tronzap.AMLCheckRequest{
					Type:      tronzap.AMLTypeHash,
					Network:   "BTC",
					Address:   "bc1address",
					Hash:      "E3F2",
					Direction: tronzap.AMLDirectionWithdrawal,
				})
				return err
			},
			wantPath: "/v1/aml-checks/new",
			wantBody: `{"type":"hash","network":"BTC","address":"bc1address","hash":"E3F2","direction":"withdrawal"}`,
		},
		{
			name: "aml check for a hash defaults to deposit",
			call: func(c *tronzap.Client) error {
				_, err := c.CreateAMLCheck(context.Background(), tronzap.AMLCheckRequest{
					Type:    tronzap.AMLTypeHash,
					Network: "TRX",
					Address: "TAddress",
					Hash:    "E3F2",
				})
				return err
			},
			wantPath: "/v1/aml-checks/new",
			wantBody: `{"type":"hash","network":"TRX","address":"TAddress","hash":"E3F2","direction":"deposit"}`,
		},
		{
			name: "aml status",
			call: func(c *tronzap.Client) error {
				_, err := c.CheckAMLStatus(context.Background(), "aml-1")
				return err
			},
			wantPath: "/v1/aml-checks/check",
			wantBody: `{"id":"aml-1"}`,
		},
		{
			name: "aml history applies defaults",
			call: func(c *tronzap.Client) error {
				_, err := c.GetAMLHistory(context.Background(), tronzap.AMLHistoryRequest{})
				return err
			},
			wantPath: "/v1/aml-checks/history",
			wantBody: `{"page":1,"per_page":10}`,
		},
		{
			name: "aml history with filters",
			call: func(c *tronzap.Client) error {
				_, err := c.GetAMLHistory(context.Background(), tronzap.AMLHistoryRequest{
					Page:    2,
					PerPage: 5,
					Status:  tronzap.AMLStatusCompleted,
				})
				return err
			},
			wantPath: "/v1/aml-checks/history",
			wantBody: `{"page":2,"per_page":5,"status":"completed"}`,
		},
		{
			name: "subscriptions",
			call: func(c *tronzap.Client) error {
				_, err := c.GetSubscriptions(context.Background())
				return err
			},
			wantPath: "/v1/subscriptions",
			wantBody: `{}`,
		},
		{
			name: "start subscription",
			call: func(c *tronzap.Client) error {
				_, err := c.StartSubscription(context.Background(), tronzap.StartSubscriptionRequest{
					SubscriptionID:  "unlimited_energy",
					Address:         "TAddress",
					DurationDays:    30,
					ExternalID:      "sub-1",
					ActivateAddress: true,
				})
				return err
			},
			wantPath: "/v1/subscription/start",
			wantBody: `{"subscription_id":"unlimited_energy","external_id":"sub-1","params":{"address":"TAddress","duration":30,"transactions_limit":0,"activate_address":true}}`,
		},
		{
			name: "start subscription sends zero limits",
			call: func(c *tronzap.Client) error {
				_, err := c.StartSubscription(context.Background(), tronzap.StartSubscriptionRequest{
					SubscriptionID: "unlimited_energy",
					Address:        "TAddress",
				})
				return err
			},
			wantPath: "/v1/subscription/start",
			wantBody: `{"subscription_id":"unlimited_energy","params":{"address":"TAddress","duration":0,"transactions_limit":0}}`,
		},
		{
			name: "check subscription by id",
			call: func(c *tronzap.Client) error {
				_, err := c.CheckSubscription(context.Background(), tronzap.SubscriptionRequest{ID: "sub-1"})
				return err
			},
			wantPath: "/v1/subscription/check",
			wantBody: `{"id":"sub-1"}`,
		},
		{
			name: "check subscription by external id",
			call: func(c *tronzap.Client) error {
				_, err := c.CheckSubscription(context.Background(), tronzap.SubscriptionRequest{ExternalID: "ext-1"})
				return err
			},
			wantPath: "/v1/subscription/check",
			wantBody: `{"external_id":"ext-1"}`,
		},
		{
			name: "stop subscription",
			call: func(c *tronzap.Client) error {
				_, err := c.StopSubscription(context.Background(), tronzap.SubscriptionRequest{ID: "sub-1", ExternalID: "ext-1"})
				return err
			},
			wantPath: "/v1/subscription/stop",
			wantBody: `{"id":"sub-1","external_id":"ext-1"}`,
		},
		{
			name: "subscription history applies defaults",
			call: func(c *tronzap.Client) error {
				_, err := c.GetSubscriptionHistory(context.Background(), tronzap.SubscriptionHistoryRequest{})
				return err
			},
			wantPath: "/v1/subscriptions/history",
			wantBody: `{"page":1,"per_page":10}`,
		},
		{
			name: "subscription history with filters",
			call: func(c *tronzap.Client) error {
				_, err := c.GetSubscriptionHistory(context.Background(), tronzap.SubscriptionHistoryRequest{
					Page:    2,
					PerPage: 50,
					Status:  tronzap.SubscriptionStatusActive,
				})
				return err
			},
			wantPath: "/v1/subscriptions/history",
			wantBody: `{"page":2,"per_page":50,"status":"active"}`,
		},
		{
			name: "direct recharge info",
			call: func(c *tronzap.Client) error {
				_, err := c.GetDirectRechargeInfo(context.Background())
				return err
			},
			wantPath: "/v1/direct-recharge-info",
			wantBody: `{}`,
		},
		{
			name: "aml services",
			call: func(c *tronzap.Client) error {
				_, err := c.GetAMLServices(context.Background())
				return err
			},
			result:   `[]`,
			wantPath: "/v1/aml-checks",
			wantBody: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.result
			if result == "" {
				result = `{}`
			}
			client, got := newServer(t, http.StatusOK, ok(result))

			if err := tt.call(client); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.Path() != tt.wantPath {
				t.Errorf("path = %q, want %q", got.Path(), tt.wantPath)
			}
			assertJSONBody(t, got.Body(), tt.wantBody)
		})
	}
}

func TestGetServices(t *testing.T) {
	response := ok(`{
		"energy": [
			{"duration":1,"min_amount":50000,"max_amount":131000,"min_energy":50000,"max_energy":131000,
			 "price":0.052300000,"price_32k":1.67,"price_65k":3.4,"price_131k":6.85}
		],
		"bandwidth": [{"duration":1,"min_amount":1000,"max_amount":50000,"price":1}],
		"activate_address": {"price":1.4}
	}`)
	client, _ := newServer(t, http.StatusOK, response)

	services, err := client.GetServices(context.Background())
	if err != nil {
		t.Fatalf("GetServices: %v", err)
	}
	if len(services.Energy) != 1 {
		t.Fatalf("energy tiers = %d, want 1", len(services.Energy))
	}
	tier := services.Energy[0]
	if tier.MinAmount != 50000 || tier.MaxAmount != 131000 {
		t.Errorf("energy range = %d..%d", tier.MinAmount, tier.MaxAmount)
	}
	if tier.Price.Float64() != 0.0523 {
		t.Errorf("price = %v, want 0.0523", tier.Price)
	}
	if tier.Price131K.Float64() != 6.85 {
		t.Errorf("price_131k = %v, want 6.85", tier.Price131K)
	}
	if len(services.Bandwidth) != 1 || services.Bandwidth[0].Price.Float64() != 1 {
		t.Errorf("bandwidth = %+v", services.Bandwidth)
	}
	if services.ActivateAddress.Price.Float64() != 1.4 {
		t.Errorf("activation price = %v, want 1.4", services.ActivateAddress.Price)
	}
}

func TestGetBalance(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{"balance":100.50,"address":"TDeposit"}`))

	balance, err := client.GetBalance(context.Background())
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.Balance.Float64() != 100.5 {
		t.Errorf("balance = %v, want 100.5", balance.Balance)
	}
	if balance.Address != "TDeposit" {
		t.Errorf("address = %q", balance.Address)
	}
}

func TestGetAddressInfo(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"resources":{"energy":131000,"bandwidth":600},
		"balances":{"TRX":10,"USDT":2.5}
	}`))

	info, err := client.GetAddressInfo(context.Background(), "TAddress")
	if err != nil {
		t.Fatalf("GetAddressInfo: %v", err)
	}
	if info.Resources.Energy != 131000 || info.Resources.Bandwidth != 600 {
		t.Errorf("resources = %+v", info.Resources)
	}
	if info.Balances["TRX"].Float64() != 10 || info.Balances["USDT"].Float64() != 2.5 {
		t.Errorf("balances = %+v", info.Balances)
	}
}

func TestEstimateEnergy(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"amount":64400,"energy":64400,"duration":1,"price":3.66,"activation_fee":0,"total":3.66,
		"from_address":"TFrom","to_address":"TTo","contract_address":"TContract"
	}`))

	estimate, err := client.EstimateEnergy(context.Background(), tronzap.EstimateEnergyRequest{
		FromAddress: "TFrom",
		ToAddress:   "TTo",
	})
	if err != nil {
		t.Fatalf("EstimateEnergy: %v", err)
	}
	if estimate.Amount != 64400 {
		t.Errorf("amount = %d, want 64400", estimate.Amount)
	}
	if estimate.Total.Float64() != 3.66 {
		t.Errorf("total = %v, want 3.66", estimate.Total)
	}
	if estimate.ContractAddress != "TContract" {
		t.Errorf("contract = %q", estimate.ContractAddress)
	}
}

func TestCalculate(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"address":"TAddress","type":"energy","amount":65000,"energy":65000,
		"duration":1,"price":1.67,"activation_fee":0,"total":1.67
	}`))

	calculation, err := client.Calculate(context.Background(), tronzap.CalculateRequest{
		Address:  "TAddress",
		Energy:   65000,
		Duration: 1,
	})
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if calculation.Type != "energy" || calculation.Amount != 65000 {
		t.Errorf("calculation = %+v", calculation)
	}
	if calculation.Total.Float64() != 1.67 {
		t.Errorf("total = %v, want 1.67", calculation.Total)
	}
}

func TestCreateEnergyTransaction(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"id":"tx-1","external_id":"ext-1","service":"energy",
		"params":{"address":"TAddress","amounts":{"energy":65000},"duration":1,"activate_address":true},
		"status":"new","amount":3.4,"created_at":"2026-08-07T10:42:12Z","hash":""
	}`))

	tx, err := client.CreateEnergyTransaction(context.Background(), tronzap.EnergyTransactionRequest{
		Address:         "TAddress",
		Energy:          65000,
		ExternalID:      "ext-1",
		ActivateAddress: true,
	})
	if err != nil {
		t.Fatalf("CreateEnergyTransaction: %v", err)
	}
	if tx.ID != "tx-1" || tx.ExternalID != "ext-1" {
		t.Errorf("ids = %q / %q", tx.ID, tx.ExternalID)
	}
	if tx.Status != tronzap.TransactionStatusNew {
		t.Errorf("status = %q", tx.Status)
	}
	if tx.Amount.Float64() != 3.4 {
		t.Errorf("amount = %v, want 3.4", tx.Amount)
	}
	if tx.Params.Amounts.Energy != 65000 || !tx.Params.ActivateAddress {
		t.Errorf("params = %+v", tx.Params)
	}
	if want := time.Date(2026, 8, 7, 10, 42, 12, 0, time.UTC); !tx.CreatedAt.Equal(want) {
		t.Errorf("created_at = %v, want %v", tx.CreatedAt, want)
	}
}

// The transaction endpoints report the charged amount as a number when creating a
// transaction and as a string when checking one.
func TestCheckTransactionDecodesStringAmount(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"id":"tx-1","external_id":null,"service":"energy",
		"params":{"address":"TAddress","energy_amount":65000,"duration":1},
		"status":"success","amount":"3.40","created_at":"2026-08-07 10:42:12","hash":"abc"
	}`))

	tx, err := client.CheckTransaction(context.Background(), tronzap.CheckTransactionRequest{ID: "tx-1"})
	if err != nil {
		t.Fatalf("CheckTransaction: %v", err)
	}
	if tx.Amount.Float64() != 3.4 {
		t.Errorf("amount = %v, want 3.4", tx.Amount)
	}
	if tx.ExternalID != "" {
		t.Errorf("external_id = %q, want empty for null", tx.ExternalID)
	}
	if tx.Params.EnergyAmount != 65000 {
		t.Errorf("energy_amount = %d", tx.Params.EnergyAmount)
	}
	if tx.Hash != "abc" || tx.Status != tronzap.TransactionStatusSuccess {
		t.Errorf("transaction = %+v", tx)
	}
	if want := time.Date(2026, 8, 7, 10, 42, 12, 0, time.UTC); !tx.CreatedAt.Equal(want) {
		t.Errorf("created_at = %v, want %v", tx.CreatedAt, want)
	}
}

func TestGetDirectRechargeInfo(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"address":"TPublic",
		"rates":[{"duration":1,"min_energy":50000,"max_energy":131000,"price":0.0523,"price_32k":1.67,"price_65k":3.4,"price_131k":6.85}]
	}`))

	info, err := client.GetDirectRechargeInfo(context.Background())
	if err != nil {
		t.Fatalf("GetDirectRechargeInfo: %v", err)
	}
	if info.Address != "TPublic" {
		t.Errorf("address = %q", info.Address)
	}
	if len(info.Rates) != 1 || info.Rates[0].Price65K.Float64() != 3.4 {
		t.Errorf("rates = %+v", info.Rates)
	}
}

func TestGetAMLServices(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`[
		{"id":"01K834","type":"address","price":2.5},
		{"id":"01K835","type":"hash","price":3.75}
	]`))

	services, err := client.GetAMLServices(context.Background())
	if err != nil {
		t.Fatalf("GetAMLServices: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("services = %d, want 2", len(services))
	}
	if services[1].Type != tronzap.AMLTypeHash || services[1].Price.Float64() != 3.75 {
		t.Errorf("second service = %+v", services[1])
	}
}

func TestCreateAMLCheckPending(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"id":"aml-1","type":"hash","address":"bc1address","hash":"E3F2","direction":"withdrawal",
		"network":"BTC","status":"processing","risk_score":null,"risk_level":null,
		"blacklist":false,"risk_factors":[],"checked_at":"2026-08-07T10:42:12Z"
	}`))

	check, err := client.CreateAMLCheck(context.Background(), tronzap.AMLCheckRequest{
		Type:      tronzap.AMLTypeHash,
		Network:   "BTC",
		Address:   "bc1address",
		Hash:      "E3F2",
		Direction: tronzap.AMLDirectionWithdrawal,
	})
	if err != nil {
		t.Fatalf("CreateAMLCheck: %v", err)
	}
	if check.Status != tronzap.AMLStatusProcessing {
		t.Errorf("status = %q", check.Status)
	}
	if check.RiskScore != nil {
		t.Errorf("risk_score = %v, want nil while pending", check.RiskScore)
	}
	if check.RiskLevel != "" {
		t.Errorf("risk_level = %q, want empty for null", check.RiskLevel)
	}
}

func TestCheckAMLStatusCompleted(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"id":"aml-1","type":"address","address":"0x6Dc1","hash":null,"direction":null,
		"network":"ETH","status":"completed","risk_score":"12.5","risk_level":"medium",
		"blacklist":false,
		"risk_factors":[{"name":"exchange","label":"Exchange","group":"low","score":0.203}],
		"checked_at":"2026-08-07T10:42:12Z"
	}`))

	check, err := client.CheckAMLStatus(context.Background(), "aml-1")
	if err != nil {
		t.Fatalf("CheckAMLStatus: %v", err)
	}
	if check.RiskScore == nil || check.RiskScore.Float64() != 12.5 {
		t.Fatalf("risk_score = %v, want 12.5", check.RiskScore)
	}
	if check.RiskLevel != tronzap.AMLRiskLevelMedium {
		t.Errorf("risk_level = %q", check.RiskLevel)
	}
	if len(check.RiskFactors) != 1 || check.RiskFactors[0].Score.Float64() != 0.203 {
		t.Errorf("risk_factors = %+v", check.RiskFactors)
	}
	if check.CheckedAt.IsZero() {
		t.Error("checked_at was not parsed")
	}
}

func TestGetAMLHistory(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"page":1,"per_page":2,"total":5,
		"items":[{"id":"aml-1","type":"hash","status":"completed","risk_score":"64.3","risk_level":"high","blacklist":true,"risk_factors":[]}]
	}`))

	history, err := client.GetAMLHistory(context.Background(), tronzap.AMLHistoryRequest{PerPage: 2})
	if err != nil {
		t.Fatalf("GetAMLHistory: %v", err)
	}
	if history.Total != 5 || history.PerPage != 2 {
		t.Errorf("history = %+v", history)
	}
	if len(history.Items) != 1 || !history.Items[0].Blacklist {
		t.Fatalf("items = %+v", history.Items)
	}
	if history.Items[0].RiskScore.Float64() != 64.3 {
		t.Errorf("risk_score = %v, want 64.3", history.Items[0].RiskScore)
	}
}

func TestGetSubscriptions(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"unlimited_energy":{"id":8,"name":"Unlimited Energy","activation_fee":0,"initial_price":8,"price":2.8,"transactions_limit":0,"duration_days":0},
		"energy_pack_100":{"id":2,"name":"Energy Pack 100","activation_fee":"2.0","initial_price":10,"price":5,"transactions_limit":10,"duration_days":5}
	}`))

	plans, err := client.GetSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("plans = %d, want 2", len(plans))
	}
	first := plans[0]
	if first.SubscriptionID != "unlimited_energy" || first.ID != 8 || first.Name != "Unlimited Energy" {
		t.Errorf("first plan = %+v", first)
	}
	if first.InitialPrice.Float64() != 8 || first.Price.Float64() != 2.8 || first.ActivationFee.Float64() != 0 {
		t.Errorf("first plan prices = %+v", first)
	}
	second := plans[1]
	if second.SubscriptionID != "energy_pack_100" || second.ActivationFee.Float64() != 2 {
		t.Errorf("second plan = %+v", second)
	}
	if second.TransactionsLimit != 10 || second.DurationDays != 5 {
		t.Errorf("second plan limits = %+v", second)
	}
}

func TestGetSubscriptionsEmpty(t *testing.T) {
	for _, result := range []string{`{}`, `[]`} {
		client, _ := newServer(t, http.StatusOK, ok(result))

		plans, err := client.GetSubscriptions(context.Background())
		if err != nil {
			t.Fatalf("GetSubscriptions with %s: %v", result, err)
		}
		if len(plans) != 0 {
			t.Errorf("plans with %s = %+v, want none", result, plans)
		}
	}
}

func TestGetSubscriptionsRejectsUnexpectedResult(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`"unlimited_energy"`))

	_, err := client.GetSubscriptions(context.Background())
	if !errors.Is(err, tronzap.ErrInvalidResponse) {
		t.Errorf("errors.Is(err, ErrInvalidResponse) = false for %v", err)
	}
}

func TestStartSubscription(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"id":"01m4e1z3q0r7x225zc6p63m5ey","subscription_id":"unlimited_energy",
		"created_at":"2026-10-08T15:26:32+00:00","expire_at":"2026-11-07T15:26:32+00:00",
		"address":"TAddress","status":"active","external_id":"sub-1",
		"params":{"address":"TAddress","duration":30,"transactions_limit":0,"activate_address":false}
	}`))

	sub, err := client.StartSubscription(context.Background(), tronzap.StartSubscriptionRequest{
		SubscriptionID: "unlimited_energy",
		Address:        "TAddress",
		DurationDays:   30,
		ExternalID:     "sub-1",
	})
	if err != nil {
		t.Fatalf("StartSubscription: %v", err)
	}
	if sub.ID != "01m4e1z3q0r7x225zc6p63m5ey" || sub.SubscriptionID != "unlimited_energy" || sub.ExternalID != "sub-1" {
		t.Errorf("subscription = %+v", sub)
	}
	if sub.Status != tronzap.SubscriptionStatusActive || sub.Address != "TAddress" {
		t.Errorf("subscription = %+v", sub)
	}
	if sub.Params.DurationDays != 30 || sub.Params.Address != "TAddress" || sub.Params.ActivateAddress {
		t.Errorf("params = %+v", sub.Params)
	}
	if want := time.Date(2026, 11, 7, 15, 26, 32, 0, time.UTC); !sub.ExpireAt.Equal(want) {
		t.Errorf("expire_at = %v, want %v", sub.ExpireAt, want)
	}
	if !sub.StoppedAt.IsZero() {
		t.Errorf("stopped_at = %v, want zero", sub.StoppedAt)
	}
}

// A stopped subscription is reported without its address and expiry date.
func TestStopSubscription(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"id":"01m4e1z3q0r7x225zc6p63m5ey","subscription_id":"unlimited_energy",
		"created_at":"2026-10-08T15:26:32+00:00","stopped_at":"2026-10-08T15:28:44+00:00",
		"status":"stopped","external_id":null,
		"params":{"address":"TAddress","duration":30,"transactions_limit":0,"activate_address":false,"future_field":1}
	}`))

	sub, err := client.StopSubscription(context.Background(), tronzap.SubscriptionRequest{ID: "01m4e1z3q0r7x225zc6p63m5ey"})
	if err != nil {
		t.Fatalf("StopSubscription: %v", err)
	}
	if sub.Status != tronzap.SubscriptionStatusStopped || sub.ExternalID != "" {
		t.Errorf("subscription = %+v", sub)
	}
	if sub.Address != "" || !sub.ExpireAt.IsZero() {
		t.Errorf("address = %q, expire_at = %v, want both empty", sub.Address, sub.ExpireAt)
	}
	if want := time.Date(2026, 10, 8, 15, 28, 44, 0, time.UTC); !sub.StoppedAt.Equal(want) {
		t.Errorf("stopped_at = %v, want %v", sub.StoppedAt, want)
	}
	if sub.Params.Address != "TAddress" {
		t.Errorf("params = %+v", sub.Params)
	}
	var raw map[string]any
	if err := json.Unmarshal(sub.Params.Raw, &raw); err != nil || raw["future_field"] != float64(1) {
		t.Errorf("params raw = %s", sub.Params.Raw)
	}
}

func TestGetSubscriptionHistory(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{
		"page":1,"per_page":10,"total":1,
		"items":[{
			"id":"01m4e1z3q0r7x225zc6p63m5ey","status":"active","subscription_id":"unlimited_energy",
			"address":"TAddress","transactions_limit":0,"transactions_used":4,"energy_used":262000,
			"total_price":13.6,"started_at":"2026-10-08T15:26:33+00:00","renewed_at":"2026-10-08T15:27:35+00:00",
			"stopped_at":null,"expire_at":"2026-11-07T15:26:32+00:00","created_at":"2026-10-08T15:26:32+00:00"
		}]
	}`))

	history, err := client.GetSubscriptionHistory(context.Background(), tronzap.SubscriptionHistoryRequest{})
	if err != nil {
		t.Fatalf("GetSubscriptionHistory: %v", err)
	}
	if history.Total != 1 || history.Page != 1 || history.PerPage != 10 {
		t.Errorf("history = %+v", history)
	}
	if len(history.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(history.Items))
	}
	item := history.Items[0]
	if item.TransactionsUsed != 4 || item.EnergyUsed != 262000 || item.TotalPrice.Float64() != 13.6 {
		t.Errorf("usage = %+v", item)
	}
	if want := time.Date(2026, 10, 8, 15, 27, 35, 0, time.UTC); !item.RenewedAt.Equal(want) {
		t.Errorf("renewed_at = %v, want %v", item.RenewedAt, want)
	}
	if item.StartedAt.IsZero() || !item.StoppedAt.IsZero() {
		t.Errorf("started_at = %v, stopped_at = %v", item.StartedAt, item.StoppedAt)
	}
}

// The documentation shows total_price as a string.
func TestSubscriptionHistoryDecodesStringPrice(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{"page":1,"per_page":10,"total":1,"items":[{"id":"sub-1","total_price":"8.00"}]}`))

	history, err := client.GetSubscriptionHistory(context.Background(), tronzap.SubscriptionHistoryRequest{})
	if err != nil {
		t.Fatalf("GetSubscriptionHistory: %v", err)
	}
	if history.Items[0].TotalPrice.Float64() != 8 {
		t.Errorf("total_price = %v, want 8", history.Items[0].TotalPrice)
	}
}

func TestAPIError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		response   string
		wantCode   int
		wantKey    string
		wantMsg    string
		wantReqID  string
		wantStatus int
	}{
		{
			name:       "reported with a 4xx status",
			status:     http.StatusBadRequest,
			response:   `{"code":10,"key":"invalid_tron_address","request_id":"req-9","error":"Invalid TRON address"}`,
			wantCode:   tronzap.CodeInvalidTronAddress,
			wantKey:    "invalid_tron_address",
			wantMsg:    "Invalid TRON address",
			wantReqID:  "req-9",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "reported with a 200 status",
			status:     http.StatusOK,
			response:   `{"code":6,"key":"insufficient_funds","error":"Insufficient funds"}`,
			wantCode:   tronzap.CodeInsufficientFunds,
			wantKey:    "insufficient_funds",
			wantMsg:    "Insufficient funds",
			wantStatus: http.StatusOK,
		},
		{
			name:       "sub-key preserved",
			status:     http.StatusOK,
			response:   `{"code":10,"key":"invalid_tron_address.from_address","error":"Invalid TRON address"}`,
			wantCode:   tronzap.CodeInvalidTronAddress,
			wantKey:    "invalid_tron_address.from_address",
			wantMsg:    "Invalid TRON address",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing code is an unidentified failure",
			status:     http.StatusOK,
			response:   `{"result":{"balance":1}}`,
			wantCode:   1,
			wantMsg:    "Unknown API error",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing error message",
			status:     http.StatusOK,
			response:   `{"code":500}`,
			wantCode:   tronzap.CodeInternalServerError,
			wantMsg:    "Unknown API error",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid JSON that is not an object",
			status:     http.StatusOK,
			response:   `"unexpected"`,
			wantCode:   1,
			wantMsg:    "Unknown API error",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newServer(t, tt.status, tt.response)

			_, err := client.GetBalance(context.Background())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, tronzap.ErrAPI) {
				t.Errorf("errors.Is(err, ErrAPI) = false for %v", err)
			}

			var apiErr *tronzap.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("errors.As(err, *APIError) = false for %v", err)
			}
			if apiErr.Code != tt.wantCode {
				t.Errorf("Code = %d, want %d", apiErr.Code, tt.wantCode)
			}
			if apiErr.Key != tt.wantKey {
				t.Errorf("Key = %q, want %q", apiErr.Key, tt.wantKey)
			}
			if apiErr.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", apiErr.Message, tt.wantMsg)
			}
			if apiErr.RequestID != tt.wantReqID {
				t.Errorf("RequestID = %q, want %q", apiErr.RequestID, tt.wantReqID)
			}
			if apiErr.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.wantStatus)
			}
			if errors.Is(err, tronzap.ErrHTTP) {
				t.Error("an API error must not match ErrHTTP")
			}
		})
	}
}

func TestHTTPError(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantMessage string
		wantIs      error
		wantIsNot   []error
	}{
		{
			name:        "rate limited",
			status:      http.StatusTooManyRequests,
			body:        "slow down",
			wantMessage: "too many requests",
			wantIs:      tronzap.ErrRateLimit,
			wantIsNot:   []error{tronzap.ErrUnauthorized, tronzap.ErrServer, tronzap.ErrAPI},
		},
		{
			name:        "unauthorized",
			status:      http.StatusUnauthorized,
			body:        "no",
			wantMessage: "unauthorized",
			wantIs:      tronzap.ErrUnauthorized,
			wantIsNot:   []error{tronzap.ErrRateLimit, tronzap.ErrServer},
		},
		{
			name:        "forbidden counts as unauthorized",
			status:      http.StatusForbidden,
			body:        "no",
			wantMessage: "unauthorized",
			wantIs:      tronzap.ErrUnauthorized,
			wantIsNot:   []error{tronzap.ErrServer},
		},
		{
			name:        "server error",
			status:      http.StatusBadGateway,
			body:        "<html>bad gateway</html>",
			wantMessage: "server error",
			wantIs:      tronzap.ErrServer,
			wantIsNot:   []error{tronzap.ErrRateLimit, tronzap.ErrUnauthorized},
		},
		{
			name:        "other status",
			status:      http.StatusTeapot,
			body:        "teapot",
			wantMessage: "HTTP error 418",
			wantIs:      tronzap.ErrHTTP,
			wantIsNot:   []error{tronzap.ErrServer, tronzap.ErrRateLimit, tronzap.ErrUnauthorized},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newServer(t, tt.status, tt.body)

			_, err := client.GetBalance(context.Background())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, tronzap.ErrHTTP) {
				t.Errorf("errors.Is(err, ErrHTTP) = false for %v", err)
			}
			if !errors.Is(err, tt.wantIs) {
				t.Errorf("errors.Is(err, %v) = false for %v", tt.wantIs, err)
			}
			for _, unwanted := range tt.wantIsNot {
				if errors.Is(err, unwanted) {
					t.Errorf("errors.Is(err, %v) = true, want false", unwanted)
				}
			}

			var httpErr *tronzap.HTTPError
			if !errors.As(err, &httpErr) {
				t.Fatalf("errors.As(err, *HTTPError) = false for %v", err)
			}
			if httpErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", httpErr.StatusCode, tt.status)
			}
			if httpErr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", httpErr.Message, tt.wantMessage)
			}
			if httpErr.Body != tt.body {
				t.Errorf("Body = %q, want %q", httpErr.Body, tt.body)
			}
		})
	}
}

// A non-2xx response carrying a well-formed API payload is an API error, because
// the code identifies the failure more precisely than the status does.
func TestAPIErrorWinsOverHTTPStatus(t *testing.T) {
	client, _ := newServer(t, http.StatusInternalServerError, `{"code":500,"key":"internal_server_error","error":"Internal server error"}`)

	_, err := client.GetBalance(context.Background())
	if !errors.Is(err, tronzap.ErrAPI) {
		t.Errorf("errors.Is(err, ErrAPI) = false for %v", err)
	}
	if errors.Is(err, tronzap.ErrServer) {
		t.Errorf("errors.Is(err, ErrServer) = true for %v", err)
	}
}

func TestInvalidResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantMsg  string
	}{
		{
			name:     "malformed JSON",
			response: `{"code":0,"result":{`,
			wantMsg:  "invalid JSON response",
		},
		{
			name:     "not JSON at all",
			response: `<html>maintenance</html>`,
			wantMsg:  "invalid JSON response",
		},
		{
			name:     "empty body",
			response: ``,
			wantMsg:  "invalid JSON response",
		},
		{
			name:     "missing result",
			response: `{"code":0,"request_id":"req-1"}`,
			wantMsg:  "missing result in response",
		},
		{
			name:     "null result",
			response: `{"code":0,"result":null}`,
			wantMsg:  "missing result in response",
		},
		{
			name:     "result of the wrong shape",
			response: `{"code":0,"result":{"balance":{"nested":true}}}`,
			wantMsg:  "unexpected result in response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newServer(t, http.StatusOK, tt.response)

			_, err := client.GetBalance(context.Background())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, tronzap.ErrInvalidResponse) {
				t.Fatalf("errors.Is(err, ErrInvalidResponse) = false for %v", err)
			}

			var invalidErr *tronzap.InvalidResponseError
			if !errors.As(err, &invalidErr) {
				t.Fatalf("errors.As(err, *InvalidResponseError) = false for %v", err)
			}
			if invalidErr.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", invalidErr.Message, tt.wantMsg)
			}
			if invalidErr.StatusCode != http.StatusOK {
				t.Errorf("StatusCode = %d, want 200", invalidErr.StatusCode)
			}
			if invalidErr.Body != tt.response {
				t.Errorf("Body = %q, want %q", invalidErr.Body, tt.response)
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	client := tronzap.NewClient(testToken, testSecret, tronzap.WithBaseURL(newBlockingServer(t)))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := client.GetBalance(ctx)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false for %v", err)
	}
	if !errors.Is(err, tronzap.ErrNetwork) {
		t.Errorf("errors.Is(err, ErrNetwork) = false for %v", err)
	}
	if errors.Is(err, tronzap.ErrTimeout) {
		t.Errorf("a cancellation must not report as a timeout: %v", err)
	}

	var netErr *tronzap.NetworkError
	if !errors.As(err, &netErr) {
		t.Fatalf("errors.As(err, *NetworkError) = false for %v", err)
	}
	if netErr.Kind != tronzap.NetworkErrorKindCanceled {
		t.Errorf("Kind = %v, want canceled", netErr.Kind)
	}
}

func TestContextDeadline(t *testing.T) {
	client := tronzap.NewClient(testToken, testSecret, tronzap.WithBaseURL(newBlockingServer(t)))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := client.GetBalance(ctx)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false for %v", err)
	}
	if !errors.Is(err, tronzap.ErrTimeout) {
		t.Errorf("errors.Is(err, ErrTimeout) = false for %v", err)
	}
	if !errors.Is(err, tronzap.ErrNetwork) {
		t.Errorf("errors.Is(err, ErrNetwork) = false for %v", err)
	}
}

func TestClientTimeout(t *testing.T) {
	client := tronzap.NewClient(testToken, testSecret,
		tronzap.WithBaseURL(newBlockingServer(t)),
		tronzap.WithTimeout(20*time.Millisecond),
	)

	_, err := client.GetBalance(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, tronzap.ErrTimeout) {
		t.Errorf("errors.Is(err, ErrTimeout) = false for %v", err)
	}

	var netErr *tronzap.NetworkError
	if !errors.As(err, &netErr) || netErr.Kind != tronzap.NetworkErrorKindTimeout {
		t.Errorf("expected a timeout NetworkError, got %v", err)
	}
}

func TestConnectionRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()

	client := tronzap.NewClient(testToken, testSecret, tronzap.WithBaseURL(serverURL))

	_, err := client.GetBalance(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, tronzap.ErrNetwork) {
		t.Errorf("errors.Is(err, ErrNetwork) = false for %v", err)
	}
	if !errors.Is(err, tronzap.ErrConnection) {
		t.Errorf("errors.Is(err, ErrConnection) = false for %v", err)
	}
}

func TestUnknownHost(t *testing.T) {
	client := tronzap.NewClient(testToken, testSecret,
		tronzap.WithBaseURL("https://tronzap-sdk-go.invalid"),
		tronzap.WithTimeout(5*time.Second),
	)

	_, err := client.GetBalance(context.Background())
	if err == nil {
		t.Skip("the resolver answered for a .invalid host; skipping")
	}
	if !errors.Is(err, tronzap.ErrConnection) {
		t.Errorf("errors.Is(err, ErrConnection) = false for %v", err)
	}
}

func TestTLSVerificationFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	// The default transport does not trust the test server's self-signed
	// certificate, which is exactly the failure being classified here.
	client := tronzap.NewClient(testToken, testSecret, tronzap.WithBaseURL(server.URL))

	_, err := client.GetBalance(context.Background())
	if err == nil {
		t.Fatal("expected a certificate verification error")
	}
	if !errors.Is(err, tronzap.ErrTLS) {
		t.Errorf("errors.Is(err, ErrTLS) = false for %v", err)
	}
	if !errors.Is(err, tronzap.ErrNetwork) {
		t.Errorf("errors.Is(err, ErrNetwork) = false for %v", err)
	}
}

func TestTLSVerificationSucceedsWithServerCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, ok(`{"balance":1,"address":"T1"}`)); err != nil {
			t.Errorf("writing response: %v", err)
		}
	}))
	defer server.Close()

	client := tronzap.NewClient(testToken, testSecret,
		tronzap.WithBaseURL(server.URL),
		tronzap.WithHTTPClient(server.Client()),
	)

	if _, err := client.GetBalance(context.Background()); err != nil {
		t.Fatalf("GetBalance over TLS: %v", err)
	}
}

func TestInvalidRequests(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(`{}`))

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "check transaction without identifiers",
			call: func() error {
				_, err := client.CheckTransaction(context.Background(), tronzap.CheckTransactionRequest{})
				return err
			},
		},
		{
			name: "address info without an address",
			call: func() error {
				_, err := client.GetAddressInfo(context.Background(), "")
				return err
			},
		},
		{
			name: "estimate energy without addresses",
			call: func() error {
				_, err := client.EstimateEnergy(context.Background(), tronzap.EstimateEnergyRequest{ToAddress: "TTo"})
				return err
			},
		},
		{
			name: "calculate without an address",
			call: func() error {
				_, err := client.Calculate(context.Background(), tronzap.CalculateRequest{Energy: 65000})
				return err
			},
		},
		{
			name: "energy transaction without an address",
			call: func() error {
				_, err := client.CreateEnergyTransaction(context.Background(), tronzap.EnergyTransactionRequest{Energy: 65000})
				return err
			},
		},
		{
			name: "bandwidth transaction without an address",
			call: func() error {
				_, err := client.CreateBandwidthTransaction(context.Background(), tronzap.BandwidthTransactionRequest{Bandwidth: 345})
				return err
			},
		},
		{
			name: "resource bundle without an address",
			call: func() error {
				_, err := client.CreateResourceBundleTransaction(context.Background(), tronzap.ResourceBundleTransactionRequest{Energy: 1})
				return err
			},
		},
		{
			name: "activation without an address",
			call: func() error {
				_, err := client.CreateAddressActivationTransaction(context.Background(), tronzap.AddressActivationRequest{})
				return err
			},
		},
		{
			name: "aml check without a type",
			call: func() error {
				_, err := client.CreateAMLCheck(context.Background(), tronzap.AMLCheckRequest{Network: "TRX", Address: "T1"})
				return err
			},
		},
		{
			name: "aml check without a network",
			call: func() error {
				_, err := client.CreateAMLCheck(context.Background(), tronzap.AMLCheckRequest{Type: tronzap.AMLTypeAddress, Address: "T1"})
				return err
			},
		},
		{
			name: "aml check without an address",
			call: func() error {
				_, err := client.CreateAMLCheck(context.Background(), tronzap.AMLCheckRequest{Type: tronzap.AMLTypeAddress, Network: "TRX"})
				return err
			},
		},
		{
			name: "aml status without an id",
			call: func() error {
				_, err := client.CheckAMLStatus(context.Background(), "")
				return err
			},
		},
		{
			name: "start subscription without a plan",
			call: func() error {
				_, err := client.StartSubscription(context.Background(), tronzap.StartSubscriptionRequest{Address: "T1"})
				return err
			},
		},
		{
			name: "start subscription without an address",
			call: func() error {
				_, err := client.StartSubscription(context.Background(), tronzap.StartSubscriptionRequest{SubscriptionID: "unlimited_energy"})
				return err
			},
		},
		{
			name: "start subscription with a negative duration",
			call: func() error {
				_, err := client.StartSubscription(context.Background(), tronzap.StartSubscriptionRequest{
					SubscriptionID: "unlimited_energy",
					Address:        "T1",
					DurationDays:   -1,
				})
				return err
			},
		},
		{
			name: "start subscription with a negative transactions limit",
			call: func() error {
				_, err := client.StartSubscription(context.Background(), tronzap.StartSubscriptionRequest{
					SubscriptionID:    "unlimited_energy",
					Address:           "T1",
					TransactionsLimit: -1,
				})
				return err
			},
		},
		{
			name: "check subscription without identifiers",
			call: func() error {
				_, err := client.CheckSubscription(context.Background(), tronzap.SubscriptionRequest{})
				return err
			},
		},
		{
			name: "stop subscription without identifiers",
			call: func() error {
				_, err := client.StopSubscription(context.Background(), tronzap.SubscriptionRequest{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, tronzap.ErrInvalidRequest) {
				t.Errorf("errors.Is(err, ErrInvalidRequest) = false for %v", err)
			}
		})
	}

	if got.Requests() != 0 {
		t.Errorf("%d requests were sent; invalid arguments must be rejected before any request", got.Requests())
	}
}

func TestOptions(t *testing.T) {
	t.Run("base URL trailing slash is trimmed", func(t *testing.T) {
		got := &capture{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got.record(r, "")
			if _, err := io.WriteString(w, ok(`{"balance":1}`)); err != nil {
				t.Errorf("writing response: %v", err)
			}
		}))
		defer server.Close()

		client := tronzap.NewClient(testToken, testSecret, tronzap.WithBaseURL(server.URL+"/"))
		if _, err := client.GetBalance(context.Background()); err != nil {
			t.Fatalf("GetBalance: %v", err)
		}
		if got.Path() != "/v1/balance" {
			t.Errorf("path = %q, want /v1/balance", got.Path())
		}
	})

	t.Run("empty and nil options are ignored", func(t *testing.T) {
		client, _ := newServer(t, http.StatusOK, ok(`{"balance":1}`),
			tronzap.WithUserAgent(""),
			tronzap.WithHTTPClient(nil),
			nil,
		)
		if _, err := client.GetBalance(context.Background()); err != nil {
			t.Fatalf("GetBalance: %v", err)
		}
	})

	t.Run("custom user agent", func(t *testing.T) {
		client, got := newServer(t, http.StatusOK, ok(`{"balance":1}`), tronzap.WithUserAgent("my-app/2.0"))
		if _, err := client.GetBalance(context.Background()); err != nil {
			t.Fatalf("GetBalance: %v", err)
		}
		if userAgent := got.HeaderValue("User-Agent"); userAgent != "my-app/2.0" {
			t.Errorf("User-Agent = %q", userAgent)
		}
	})

	t.Run("custom HTTP client is used", func(t *testing.T) {
		transport := &countingTransport{}
		client, _ := newServer(t, http.StatusOK, ok(`{"balance":1}`),
			tronzap.WithHTTPClient(&http.Client{Transport: transport}),
		)
		if _, err := client.GetBalance(context.Background()); err != nil {
			t.Fatalf("GetBalance: %v", err)
		}
		if transport.calls != 1 {
			t.Errorf("custom transport calls = %d, want 1", transport.calls)
		}
	})

	t.Run("a supplied HTTP client is never mutated", func(t *testing.T) {
		shared := &http.Client{Timeout: time.Minute}
		tronzap.NewClient(testToken, testSecret,
			tronzap.WithHTTPClient(shared),
			tronzap.WithTimeout(time.Second),
		)
		if shared.Timeout != time.Minute {
			t.Errorf("shared client timeout = %v, want 1m", shared.Timeout)
		}
	})

	t.Run("options apply in order", func(t *testing.T) {
		client, got := newServer(t, http.StatusOK, ok(`{"balance":1}`),
			tronzap.WithUserAgent("first"),
			tronzap.WithUserAgent("second"),
		)
		if _, err := client.GetBalance(context.Background()); err != nil {
			t.Fatalf("GetBalance: %v", err)
		}
		if userAgent := got.HeaderValue("User-Agent"); userAgent != "second" {
			t.Errorf("User-Agent = %q, want second", userAgent)
		}
	})
}

type countingTransport struct {
	calls int
}

func (t *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls++
	return http.DefaultTransport.RoundTrip(r)
}

// recordingTransport captures the URL a request was addressed to and answers it
// without touching the network.
type recordingTransport struct {
	url string
}

func (t *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.url = r.URL.String()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(ok(`{"balance":1,"address":"T1"}`))),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    r,
	}, nil
}

func TestWithBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantURL string
	}{
		{
			name:    "full URL",
			baseURL: "https://api.tronzap.com",
			wantURL: "https://api.tronzap.com/v1/balance",
		},
		{
			name:    "bare domain defaults to https",
			baseURL: "api.tronzap.com",
			wantURL: "https://api.tronzap.com/v1/balance",
		},
		{
			name:    "bare domain with a trailing slash",
			baseURL: "api.tronzap.com/",
			wantURL: "https://api.tronzap.com/v1/balance",
		},
		{
			name:    "trailing slash on a full URL",
			baseURL: "https://api.tronzap.com/",
			wantURL: "https://api.tronzap.com/v1/balance",
		},
		{
			name:    "surrounding whitespace",
			baseURL: "  api.tronzap.com  ",
			wantURL: "https://api.tronzap.com/v1/balance",
		},
		{
			name:    "an explicit scheme is respected",
			baseURL: "http://localhost:8080",
			wantURL: "http://localhost:8080/v1/balance",
		},
		{
			name:    "host and port without a scheme",
			baseURL: "localhost:8080",
			wantURL: "https://localhost:8080/v1/balance",
		},
		{
			name:    "scheme-relative URL",
			baseURL: "//api.tronzap.com",
			wantURL: "https://api.tronzap.com/v1/balance",
		},
		{
			name:    "an empty value keeps the default",
			baseURL: "",
			wantURL: tronzap.DefaultBaseURL + "/v1/balance",
		},
		{
			name:    "a value that trims to nothing keeps the default",
			baseURL: "///",
			wantURL: tronzap.DefaultBaseURL + "/v1/balance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &recordingTransport{}
			client := tronzap.NewClient(testToken, testSecret,
				tronzap.WithBaseURL(tt.baseURL),
				tronzap.WithHTTPClient(&http.Client{Transport: transport}),
			)

			if _, err := client.GetBalance(context.Background()); err != nil {
				t.Fatalf("GetBalance: %v", err)
			}
			if transport.url != tt.wantURL {
				t.Errorf("request URL = %q, want %q", transport.url, tt.wantURL)
			}
		})
	}
}

func TestDo(t *testing.T) {
	t.Run("decodes into a caller-supplied value", func(t *testing.T) {
		client, got := newServer(t, http.StatusOK, ok(`{"items":[{"id":"item-1"}]}`))

		var result struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		params := map[string]any{"page": 1}
		if err := client.Do(context.Background(), "/v1/future-endpoint", params, &result); err != nil {
			t.Fatalf("Do: %v", err)
		}
		if got.Path() != "/v1/future-endpoint" {
			t.Errorf("path = %q", got.Path())
		}
		assertJSONBody(t, got.Body(), `{"page":1}`)
		if len(result.Items) != 1 || result.Items[0].ID != "item-1" {
			t.Errorf("result = %+v", result)
		}
	})

	t.Run("a nil result discards the payload", func(t *testing.T) {
		client, _ := newServer(t, http.StatusOK, ok(`{"anything":true}`))

		if err := client.Do(context.Background(), "/v1/balance", nil, nil); err != nil {
			t.Fatalf("Do: %v", err)
		}
	})

	t.Run("unencodable params are rejected", func(t *testing.T) {
		client, got := newServer(t, http.StatusOK, ok(`{}`))

		err := client.Do(context.Background(), "/v1/balance", func() {}, nil)
		if !errors.Is(err, tronzap.ErrInvalidRequest) {
			t.Errorf("errors.Is(err, ErrInvalidRequest) = false for %v", err)
		}
		if got.Requests() != 0 {
			t.Error("no request should have been sent")
		}
	})
}

func TestClientIsConcurrencySafe(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(`{"balance":1,"address":"T1"}`))

	const workers = 16
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := client.GetBalance(context.Background())
			errs <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent GetBalance: %v", err)
		}
	}
	if got.Requests() != workers {
		t.Errorf("requests = %d, want %d", got.Requests(), workers)
	}
}
