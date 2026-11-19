# Changelog

All notable changes to this project are documented in this file. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.0] - 2026-11-20

### Added

- Bulk orders: `BulkCalculate`, `BulkCreate`, `BulkCheck`, `BulkHistory` and `BulkReport` for `/v1/bulk/calculate`,
  `/v1/bulk/create`, `/v1/bulk/check`, `/v1/bulk/history` and `/v1/bulk/report`, with the types `BulkItem`,
  `BulkCalculation`, `BulkRejectedRow`, `BulkOrder`, `BulkHistory` and their request types, and the `BulkStatus*` and
  `BulkRow*` constants. `BulkCalculate` always orders energy and rejects a request without rows before sending;
  invalid rows are not checked by the SDK and come back in `Rejected` with their row number. `BulkReport` returns the
  CSV report as a string.
- `VerifyWebhookSignature` checks the `X-Signature` header of a bulk order webhook against the raw body. The body
  decodes into a `BulkOrder`.
- `APIError.Result` holds the `result` the API sends with an error, and `APIError.DecodeResult` decodes it. Until now
  it was dropped, so the new calculation of `CodeBulkPriceChanged`, the rejected rows of `CodeBulkNoValidItems` and
  the like could not be read through the SDK.
- `APIError.RetryAfter` and `HTTPError.RetryAfter` hold how long to wait before retrying, from the `Retry-After`
  header (seconds or an HTTP date) or the `retry_after` field, or zero when the API sent neither.
- Error codes `CodeBulkNotFound` (90), `CodeBulkExpired` (91), `CodeBulkPriceChanged` (92), `CodeBulkTooManyItems`
  (93), `CodeBulkNoValidItems` (94), `CodeBulkResourceNotSupported` (95), `CodePayloadTooLarge` (96) and
  `CodeRateLimited` (429).
- `examples/basic` lists the bulk orders, calculates a bulk order for `TRONZAP_ADDRESS`, checks the order in
  `TRONZAP_BULK_ID` and prints its report, and with purchases allowed starts a one-address bulk order.
- This changelog.

### Changed

- The README and the `ErrRateLimit` docs say that a rate limit reported by the API, HTTP 429 with a JSON body, is an
  `*APIError` with `CodeRateLimited`, as it always has been; `ErrRateLimit` matches only an HTTP 429 without an API
  body, such as one from a proxy.

## [1.0.0] - 2026-10-09

The first stable release. The public API follows semantic versioning: a breaking change ships only in a new major
version, under a new module path.

### Added

- Subscriptions: `GetSubscriptions`, `StartSubscription`, `CheckSubscription`, `StopSubscription` and
  `GetSubscriptionHistory`, with the types `SubscriptionPlan`, `Subscription`, `SubscriptionHistory` and the
  `SubscriptionStatus*` constants.

### Changed

- A hash AML check without a `Direction` sends `deposit` explicitly. Address checks are unchanged.

### Removed

- **Breaking:** the energy fields that duplicated `amount`. Use `EnergyRate.MinAmount` and `EnergyRate.MaxAmount`
  instead of `MinEnergy` and `MaxEnergy`, `Calculation.Amount` instead of `Calculation.Energy`, and
  `EnergyEstimate.Amount` instead of `EnergyEstimate.Energy`.

### Fixed

- The docs of `EnergyRate.Price` and `DirectRechargeRate.Price`: they are the price of 1000 units, like
  `BandwidthRate.Price`, not of one unit. The values are unchanged.
- The descriptions of error codes 10 (`CodeInvalidTronAddress`), which also means the address already has an active
  subscription, and 21 (`CodeCannotStopSubscription`).

## [0.1.0] - 2026-08-10

### Added

- The first release: `GetServices`, `GetBalance`, `GetAddressInfo`, `EstimateEnergy`, `Calculate`, energy,
  bandwidth, resource bundle and address activation transactions, `CheckTransaction`, `GetDirectRechargeInfo`, AML
  checks and `Do` for endpoints the SDK does not wrap.
- The options `WithBaseURL`, `WithHTTPClient`, `WithTimeout` and `WithUserAgent`.
- The error types `APIError`, `HTTPError`, `NetworkError` and `InvalidResponseError`, and the sentinels for
  `errors.Is`.

[Unreleased]: https://github.com/tron-energy-market/tronzap-sdk-go/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/tron-energy-market/tronzap-sdk-go/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/tron-energy-market/tronzap-sdk-go/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/tron-energy-market/tronzap-sdk-go/releases/tag/v0.1.0
