# Tron Energy Rental via API
## Go SDK by TronZap.com

**[English](README.md)** | [Español](README.es.md) | [Português](README.pt-br.md) | [Русский](README.ru.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/tron-energy-market/tronzap-sdk-go.svg)](https://pkg.go.dev/github.com/tron-energy-market/tronzap-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/tron-energy-market/tronzap-sdk-go)](https://goreportcard.com/report/github.com/tron-energy-market/tronzap-sdk-go)
[![CI](https://github.com/tron-energy-market/tronzap-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/tron-energy-market/tronzap-sdk-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Official Go SDK for the TronZap API.
This SDK allows you to easily integrate with TronZap services for TRON energy rental.

TronZap.com allows you to [buy TRON energy](https://tronzap.com/), making USDT (TRC20) transfers cheaper by significantly reducing transaction fees.

👉 [Register for an API key](https://tronzap.com) to start using TronZap API and integrate it via the SDK.

- Website: https://tronzap.com
- API reference: https://docs.tronzap.com/
- Package documentation: https://pkg.go.dev/github.com/tron-energy-market/tronzap-sdk-go

## Installation

```bash
go get github.com/tron-energy-market/tronzap-sdk-go
```

## Requirements

- Go 1.21 or newer
- No third-party dependencies

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

func main() {
	client := tronzap.NewClient("your_api_token", "your_api_secret")
	ctx := context.Background()

	balance, err := client.GetBalance(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("balance: %s (deposit to %s)\n", balance.Balance, balance.Address)

	// Estimate how much energy a USDT transfer needs, then buy exactly that much.
	estimate, err := client.EstimateEnergy(ctx, tronzap.EstimateEnergyRequest{
		FromAddress: "TSenderAddress",
		ToAddress:   "TRecipientAddress",
	})
	if err != nil {
		log.Fatal(err)
	}

	tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
		Address:         "TRecipientAddress",
		Energy:          estimate.Energy,
		Duration:        1,
		ExternalID:      "order-42",
		ActivateAddress: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("transaction %s costs %s and is %s\n", tx.ID, tx.Amount, tx.Status)
}
```

A runnable walkthrough of every operation lives in
[`examples/basic`](examples/basic/main.go):

```bash
export TRONZAP_API_TOKEN=your_api_token
export TRONZAP_API_SECRET=your_api_secret
export TRONZAP_BASE_URL=api.tronzap.com   # optional
go run ./examples/basic
```

By default it only reads and spends nothing. Setting `TRONZAP_ALLOW_PURCHASES=1`
also exercises the endpoints that create transactions and AML checks, which debit
the account balance. See the comment at the top of the file for the other optional
variables.

## Configuration

`NewClient` takes the two credentials from your dashboard — the API token is sent
as a bearer token, and the API secret signs every request body. Everything else is
an option:

```go
client := tronzap.NewClient(apiToken, apiSecret,
	tronzap.WithBaseURL("api.tronzap.com"), // defaults to tronzap.DefaultBaseURL
	tronzap.WithTimeout(10*time.Second),    // defaults to tronzap.DefaultTimeout (30s)
	tronzap.WithHTTPClient(myClient),       // bring your own transport, proxy or retries
	tronzap.WithUserAgent("my-app/1.0"),
)
```

`WithBaseURL` takes either a bare domain or a full URL — a missing scheme becomes
`https` and a trailing slash is trimmed, so `"api.tronzap.com"`,
`"api.tronzap.com/"` and `"https://api.tronzap.com"` are equivalent. Pass an
explicit scheme to opt out, for example `"http://localhost:8080"` against a local
mock.

A `Client` is safe for concurrent use and holds no global state, so create one per
set of credentials and share it. A client you pass through `WithHTTPClient` is
never mutated: `WithTimeout` applies its deadline to a copy.

Every request-issuing method takes a `context.Context` as its first argument, so
per-call deadlines and cancellation work as usual:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

services, err := client.GetServices(ctx)
```

## Available methods

| Method | Endpoint | Description |
|---|---|---|
| `GetServices(ctx)` | `/v1/services` | Available services and prices |
| `GetBalance(ctx)` | `/v1/balance` | Current account balance |
| `GetAddressInfo(ctx, address)` | `/v1/address-info` | Address resources (energy, bandwidth) and balances (TRX, USDT) |
| `EstimateEnergy(ctx, req)` | `/v1/estimate-energy` | Energy a transfer needs, and its cost |
| `Calculate(ctx, req)` | `/v1/calculate` | Price a purchase without creating a transaction |
| `CreateEnergyTransaction(ctx, req)` | `/v1/transaction/new` | Buy energy |
| `CreateBandwidthTransaction(ctx, req)` | `/v1/transaction/new` | Buy bandwidth |
| `CreateResourceBundleTransaction(ctx, req)` | `/v1/transaction/new` | Buy energy and bandwidth in one transaction |
| `CreateAddressActivationTransaction(ctx, req)` | `/v1/transaction/new` | Activate a TRON address |
| `CheckTransaction(ctx, req)` | `/v1/transaction/check` | Status of a transaction, by id or external id |
| `GetDirectRechargeInfo(ctx)` | `/v1/direct-recharge-info` | Direct recharge address and rates |
| `GetAMLServices(ctx)` | `/v1/aml-checks` | AML services and pricing |
| `CreateAMLCheck(ctx, req)` | `/v1/aml-checks/new` | Start an AML screening |
| `CheckAMLStatus(ctx, id)` | `/v1/aml-checks/check` | Status and result of an AML check |
| `GetAMLHistory(ctx, req)` | `/v1/aml-checks/history` | Paginated AML check history |
| `Do(ctx, endpoint, params, result)` | any | Escape hatch for endpoints not wrapped yet |

Optional fields live in request structs rather than in long parameter lists, so
new API fields can be added without breaking your code. Zero values mean "use the
API default": `Duration` becomes 1 hour, and AML history paging defaults to page 1
with 10 items.

### Buying resources

```go
// Energy, optionally activating the address in the same call.
tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
	Address:         "TRecipientAddress",
	Energy:          65000,
	Duration:        1,      // hours; 1 or 24
	ExternalID:      "order-42",
	ActivateAddress: true,
})

// Bandwidth.
tx, err = client.CreateBandwidthTransaction(ctx, tronzap.BandwidthTransactionRequest{
	Address:    "TRecipientAddress",
	Bandwidth:  345,
	ExternalID: "bandwidth-1",
})

// Energy and bandwidth together, which is cheaper than buying each separately.
tx, err = client.CreateResourceBundleTransaction(ctx, tronzap.ResourceBundleTransactionRequest{
	Address:    "TRecipientAddress",
	Energy:     65000,
	Bandwidth:  345,
	Duration:   1,
	ExternalID: "bundle-1",
})

// Activation on its own.
tx, err = client.CreateAddressActivationTransaction(ctx, tronzap.AddressActivationRequest{
	Address:    "TRecipientAddress",
	ExternalID: "activation-1",
})
```

### Following a transaction

A transaction moves through `new` → `pending` → `success` or `failed`:

```go
for {
	tx, err := client.CheckTransaction(ctx, tronzap.CheckTransactionRequest{ExternalID: "order-42"})
	if err != nil {
		return err
	}
	if tx.Status == tronzap.TransactionStatusSuccess || tx.Status == tronzap.TransactionStatusFailed {
		fmt.Println("finished as", tx.Status, "hash", tx.Hash)
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}
}
```

### AML screening

```go
check, err := client.CreateAMLCheck(ctx, tronzap.AMLCheckRequest{
	Type:    tronzap.AMLTypeAddress, // or tronzap.AMLTypeHash
	Network: "TRX",
	Address: "TAddressToScreen",
})
if err != nil {
	return err
}

result, err := client.CheckAMLStatus(ctx, check.ID)
if err != nil {
	return err
}
if result.Status == tronzap.AMLStatusCompleted {
	fmt.Println(result.RiskLevel, result.Blacklist, result.RiskFactors)
}
```

`RiskScore` is a `*Number` because the API leaves it null until screening
finishes; check for nil before reading it.

## Error handling

Every failure is a plain Go `error`. Four concrete types carry the details, and
each one matches package sentinels through `errors.Is`, so you can branch as
coarsely or as precisely as you need:

| Type | Meaning | Matches |
|---|---|---|
| `*APIError` | API answered with a non-zero `code` | `ErrAPI` |
| `*HTTPError` | Non-2xx response with no usable API payload | `ErrHTTP`, plus `ErrRateLimit` (429), `ErrUnauthorized` (401/403) or `ErrServer` (5xx) |
| `*NetworkError` | No response arrived at all | `ErrNetwork`, plus `ErrConnection`, `ErrTimeout` or `ErrTLS` |
| `*InvalidResponseError` | 2xx response the SDK could not decode | `ErrInvalidResponse` |

Arguments the SDK rejects before sending anything match `ErrInvalidRequest`.

```go
var apiErr *tronzap.APIError
switch {
case errors.As(err, &apiErr):
	// Application-level failure: the code says exactly what went wrong.
	switch apiErr.Code {
	case tronzap.CodeInvalidTronAddress:
		// apiErr.Key may narrow it down, e.g. "invalid_tron_address.from_address"
		log.Println("bad address:", apiErr.Key)
	case tronzap.CodeInsufficientFunds:
		log.Println("top up the account")
	case tronzap.CodeAddressNotActivated:
		log.Println("activate the address first")
	default:
		log.Printf("api error %d: %s (request %s)", apiErr.Code, apiErr.Message, apiErr.RequestID)
	}
case errors.Is(err, tronzap.ErrRateLimit):
	// Back off and retry.
case errors.Is(err, tronzap.ErrUnauthorized):
	// Bad token or signature.
case errors.Is(err, tronzap.ErrTimeout), errors.Is(err, tronzap.ErrServer):
	// Transient; safe to retry.
case errors.Is(err, tronzap.ErrNetwork):
	// Unreachable.
}
```

`APIError.RequestID` is the identifier the API assigns to each request — quote it
when contacting support.

Because a `NetworkError` wraps the underlying transport error, context failures
stay detectable directly:

```go
if errors.Is(err, context.Canceled) { /* the caller gave up */ }
if errors.Is(err, context.DeadlineExceeded) { /* the deadline passed */ }
```

An API error takes precedence over the HTTP status: the API reports some failures
with a 2xx status and others with a 4xx or 5xx status, so a decodable payload with
a non-zero code is always reported as `*APIError`, never as `*HTTPError`.

### API error codes

| Code | Constant | Description |
|------|----------|-------------|
| 1 | `CodeAuth` | Authentication error – invalid API token or signature |
| 2 | `CodeInvalidServiceOrParams` | Invalid service or parameters |
| 5 | `CodeWalletNotFound` | Internal wallet not found. Contact support. |
| 6 | `CodeInsufficientFunds` | Insufficient funds |
| 10 | `CodeInvalidTronAddress` | Invalid TRON address |
| 11 | `CodeInvalidEnergyAmount` | Invalid energy amount |
| 12 | `CodeInvalidDuration` | Invalid duration |
| 20 | `CodeTransactionNotFound` | Transaction/subscription not found |
| 21 | `CodeCannotStopSubscription` | Cannot stop subscription |
| 24 | `CodeAddressNotActivated` | Address not activated |
| 25 | `CodeAddressAlreadyActivated` | Address already activated |
| 30 | `CodeAMLCheckNotFound` | AML check not found |
| 35 | `CodeServiceNotAvailable` | Service not available |
| 50 | `CodeInvalidBandwidthAmount` | Invalid bandwidth amount |
| 500 | `CodeInternalServerError` | Internal server error – contact support |

## Decimal and timestamp fields

The API encodes money as a JSON number in some responses and as a JSON string in
others, so amounts and prices use the `Number` type, which decodes both and
exposes `Float64()` and `String()`. Timestamps use `Time`, which embeds
`time.Time`, accepts the several formats the API emits, and keeps the original
text in `Raw`. An unrecognised timestamp leaves the embedded time zero instead of
failing the whole response.

## Reaching unwrapped endpoints

`Do` signs and sends a request to any endpoint and decodes the `result` field into
a value you supply, which is how you use API features this SDK has not wrapped yet:

```go
var result struct {
	Items []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"items"`
}
err := client.Do(ctx, "/v1/subscriptions/history", map[string]any{"page": 1}, &result)
```

## Testing

```bash
go test ./...
go test -race -cover ./...
go vet ./...
gofmt -l .
```

## License

The MIT License (MIT). Please see [License File](LICENSE) for more information.

## Support

For support, please contact [support@tronzap.com](mailto:support@tronzap.com).
