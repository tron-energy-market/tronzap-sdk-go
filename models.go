package tronzap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Service names accepted by the transaction endpoint.
const (
	// ServiceEnergy purchases energy.
	ServiceEnergy = "energy"
	// ServiceBandwidth purchases bandwidth.
	ServiceBandwidth = "bandwidth"
	// ServiceResourceBundle purchases energy and bandwidth in one transaction.
	ServiceResourceBundle = "resource_bundle"
	// ServiceActivateAddress activates a TRON address.
	ServiceActivateAddress = "activate_address"
)

// Transaction statuses. A transaction moves new -> pending -> success or failed.
const (
	// TransactionStatusNew means the transaction was created but processing has not started.
	TransactionStatusNew = "new"
	// TransactionStatusPending means the transaction is being processed.
	TransactionStatusPending = "pending"
	// TransactionStatusSuccess means the transaction completed successfully.
	TransactionStatusSuccess = "success"
	// TransactionStatusFailed means the transaction failed.
	TransactionStatusFailed = "failed"
)

// Subscription statuses.
const (
	// SubscriptionStatusNew means the subscription was created but not started yet.
	SubscriptionStatusNew = "new"
	// SubscriptionStatusPending means the subscription is being started.
	SubscriptionStatusPending = "pending"
	// SubscriptionStatusError means the subscription could not be started.
	SubscriptionStatusError = "error"
	// SubscriptionStatusActive means the subscription is delegating energy.
	SubscriptionStatusActive = "active"
	// SubscriptionStatusStopped means the subscription was stopped.
	SubscriptionStatusStopped = "stopped"
	// SubscriptionStatusExpired means the subscription ran out of time or transactions.
	SubscriptionStatusExpired = "expired"
)

// AML check types.
const (
	// AMLTypeAddress screens a wallet address.
	AMLTypeAddress = "address"
	// AMLTypeHash screens a transaction hash.
	AMLTypeHash = "hash"
)

// AML transaction directions, used with [AMLTypeHash].
const (
	// AMLDirectionDeposit screens an incoming transaction. This is the API default.
	AMLDirectionDeposit = "deposit"
	// AMLDirectionWithdrawal screens an outgoing transaction.
	AMLDirectionWithdrawal = "withdrawal"
)

// AML check statuses.
const (
	// AMLStatusPending means the check is queued.
	AMLStatusPending = "pending"
	// AMLStatusProcessing means the check is running.
	AMLStatusProcessing = "processing"
	// AMLStatusCompleted means results are available.
	AMLStatusCompleted = "completed"
	// AMLStatusFailed means the check could not be completed.
	AMLStatusFailed = "failed"
)

// AML risk levels.
const (
	// AMLRiskLevelLow indicates low risk.
	AMLRiskLevelLow = "low"
	// AMLRiskLevelMedium indicates medium risk.
	AMLRiskLevelMedium = "medium"
	// AMLRiskLevelHigh indicates high risk.
	AMLRiskLevelHigh = "high"
)

// DefaultContractAddress is the USDT (TRC20) contract the API assumes when
// [EstimateEnergyRequest.ContractAddress] is empty.
const DefaultContractAddress = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"

// Number is a decimal value that the API may encode either as a JSON number or
// as a JSON string. Both forms decode into the same value; a JSON null decodes
// to zero.
type Number float64

// UnmarshalJSON accepts a JSON number, a numeric JSON string or null.
func (n *Number) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" {
		*n = 0
		return nil
	}
	s = strings.Trim(s, `"`)
	if s == "" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("tronzap: cannot decode %s as a number: %w", data, err)
	}
	*n = Number(f)
	return nil
}

// MarshalJSON encodes the value as a JSON number.
func (n Number) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(n), 'f', -1, 64)), nil
}

// Float64 returns the value as a float64.
func (n Number) Float64() float64 { return float64(n) }

// String returns the value without a trailing exponent or padding zeros.
func (n Number) String() string { return strconv.FormatFloat(float64(n), 'f', -1, 64) }

// timeLayouts are tried in order when decoding a [Time].
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// Time is a timestamp that tolerates the several encodings the API uses:
// RFC 3339, a space-separated date-time, or a Unix timestamp. The original
// encoding is preserved in Raw, and an unrecognised value leaves the embedded
// time zero rather than failing the whole response.
type Time struct {
	time.Time
	// Raw is the timestamp exactly as the API sent it.
	Raw string
}

// UnmarshalJSON decodes any of the supported timestamp encodings.
func (t *Time) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" {
		*t = Time{}
		return nil
	}
	if unquoted, err := strconv.Unquote(s); err == nil {
		s = unquoted
	}
	t.Raw = s
	t.Time = time.Time{}
	if s == "" {
		return nil
	}
	for _, layout := range timeLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed
			return nil
		}
	}
	if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
		t.Time = time.Unix(unix, 0).UTC()
	}
	return nil
}

// MarshalJSON re-emits the original encoding when one was captured.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.Raw != "" {
		return json.Marshal(t.Raw)
	}
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Format(time.RFC3339Nano))
}

// EstimateEnergyRequest asks how much energy a transfer needs.
type EstimateEnergyRequest struct {
	// FromAddress is the sender's TRON address. Required.
	FromAddress string
	// ToAddress is the recipient's TRON address. Required.
	ToAddress string
	// ContractAddress is the TRC20 contract to estimate against. When empty the
	// API uses [DefaultContractAddress].
	ContractAddress string
}

// CalculateRequest prices a resource purchase without creating a transaction.
type CalculateRequest struct {
	// Address is the TRON address that would receive the resources. Required.
	Address string
	// Energy is the amount of energy to price, sent as the API's "amount" field. Required.
	Energy int64
	// Duration is the rental duration in hours. Defaults to 1 when zero.
	Duration int
}

// EnergyTransactionRequest buys energy for an address.
type EnergyTransactionRequest struct {
	// Address is the TRON address that receives the energy. Required.
	Address string
	// Energy is the amount of energy to buy. Required.
	Energy int64
	// Duration is the rental duration in hours. Defaults to 1 when zero.
	Duration int
	// ExternalID is your own identifier for the transaction. Optional.
	ExternalID string
	// ActivateAddress additionally activates the address if it is not active yet.
	ActivateAddress bool
}

// BandwidthTransactionRequest buys bandwidth for an address.
type BandwidthTransactionRequest struct {
	// Address is the TRON address that receives the bandwidth. Required.
	Address string
	// Bandwidth is the amount of bandwidth to buy. Required.
	Bandwidth int64
	// ExternalID is your own identifier for the transaction. Optional.
	ExternalID string
}

// ResourceBundleTransactionRequest buys energy and bandwidth in one transaction.
type ResourceBundleTransactionRequest struct {
	// Address is the TRON address that receives the resources. Required.
	Address string
	// Energy is the amount of energy to buy. Required.
	Energy int64
	// Bandwidth is the amount of bandwidth to buy. Required.
	Bandwidth int64
	// Duration is the rental duration in hours. Defaults to 1 when zero.
	Duration int
	// ExternalID is your own identifier for the transaction. Optional.
	ExternalID string
	// ActivateAddress additionally activates the address if it is not active yet.
	ActivateAddress bool
}

// AddressActivationRequest activates a TRON address.
type AddressActivationRequest struct {
	// Address is the TRON address to activate. Required.
	Address string
	// ExternalID is your own identifier for the transaction. Optional.
	ExternalID string
}

// CheckTransactionRequest looks up one transaction. Exactly one of the two
// identifiers is enough; supplying neither is rejected before any request is
// sent.
type CheckTransactionRequest struct {
	// ID is the identifier the API assigned to the transaction.
	ID string
	// ExternalID is the identifier you supplied when creating the transaction.
	ExternalID string
}

// AMLCheckRequest starts an AML screening.
type AMLCheckRequest struct {
	// Type selects what to screen: [AMLTypeAddress] or [AMLTypeHash]. Required.
	Type string
	// Network is the blockchain network code, for example "TRX", "BTC" or "ETH". Required.
	Network string
	// Address is the address to screen. For [AMLTypeHash] it is the recipient
	// address of the transaction, where the funds were received. Required.
	Address string
	// Hash is the transaction hash. Required for [AMLTypeHash].
	Hash string
	// Direction is [AMLDirectionDeposit] or [AMLDirectionWithdrawal], used with
	// [AMLTypeHash]. The API defaults to deposit.
	Direction string
}

// AMLHistoryRequest pages through past AML checks.
type AMLHistoryRequest struct {
	// Page is the 1-based page number. Defaults to 1 when zero.
	Page int
	// PerPage is the page size, between 1 and 50. Defaults to 10 when zero.
	PerPage int
	// Status filters by check status, for example [AMLStatusCompleted]. Optional.
	Status string
}

// StartSubscriptionRequest starts a subscription for an address.
type StartSubscriptionRequest struct {
	// SubscriptionID is the plan to subscribe to, the [SubscriptionPlan.SubscriptionID]
	// returned by [Client.GetSubscriptions], such as "unlimited_energy". Required.
	SubscriptionID string
	// Address is the TRON address the subscription serves. Required.
	Address string
	// DurationDays is how many days the subscription runs, 0 for no time limit.
	DurationDays int
	// TransactionsLimit is how many transactions the subscription covers, 0 for no limit.
	TransactionsLimit int64
	// ExternalID is your own identifier for the subscription. Optional.
	ExternalID string
	// ActivateAddress additionally activates the address if it is not active yet.
	ActivateAddress bool
}

// SubscriptionRequest looks up one subscription. Exactly one of the two
// identifiers is enough; supplying neither is rejected before any request is
// sent.
type SubscriptionRequest struct {
	// ID is the identifier the API assigned to the subscription.
	ID string
	// ExternalID is the identifier you supplied when starting the subscription.
	ExternalID string
}

// SubscriptionHistoryRequest pages through your subscriptions.
type SubscriptionHistoryRequest struct {
	// Page is the 1-based page number. Defaults to 1 when zero.
	Page int
	// PerPage is the page size, between 1 and 50. Defaults to 10 when zero.
	PerPage int
	// Status filters by subscription status, for example [SubscriptionStatusActive]. Optional.
	Status string
}

// Services lists the resources on sale and their prices.
type Services struct {
	// Energy holds one price tier per energy amount range.
	Energy []EnergyRate `json:"energy"`
	// Bandwidth holds one price tier per bandwidth amount range.
	Bandwidth []BandwidthRate `json:"bandwidth"`
	// ActivateAddress holds the flat price of an address activation.
	ActivateAddress ActivateAddressRate `json:"activate_address"`
}

// EnergyRate is one energy price tier.
type EnergyRate struct {
	// Duration is the rental duration in hours this tier applies to.
	Duration int `json:"duration"`
	// MinAmount is the smallest purchasable amount in this tier.
	MinAmount int64 `json:"min_amount"`
	// MaxAmount is the largest purchasable amount in this tier.
	MaxAmount int64 `json:"max_amount"`
	// Price is the cost per 1000 units of energy, so 65,000 energy at a price of
	// 0.03 costs 1.95. Bandwidth is priced the same way, see [BandwidthRate.Price].
	Price Number `json:"price"`
	// Price32K is the price of 32,000 energy at this tier.
	Price32K Number `json:"price_32k"`
	// Price65K is the price of 65,000 energy at this tier.
	Price65K Number `json:"price_65k"`
	// Price131K is the price of 131,000 energy at this tier.
	Price131K Number `json:"price_131k"`
}

// BandwidthRate is one bandwidth price tier.
type BandwidthRate struct {
	// Duration is the rental duration in hours this tier applies to.
	Duration int `json:"duration"`
	// MinAmount is the smallest purchasable amount in this tier.
	MinAmount int64 `json:"min_amount"`
	// MaxAmount is the largest purchasable amount in this tier.
	MaxAmount int64 `json:"max_amount"`
	// Price is the cost per 1000 units of bandwidth, so 345 bandwidth at a price
	// of 1 costs 0.345. Energy is priced the same way, see [EnergyRate.Price].
	Price Number `json:"price"`
}

// ActivateAddressRate is the price of an address activation.
type ActivateAddressRate struct {
	// Price is the flat activation fee.
	Price Number `json:"price"`
}

// AMLService is one AML screening product and its price.
type AMLService struct {
	// ID identifies the service.
	ID string `json:"id"`
	// Type is [AMLTypeAddress] or [AMLTypeHash].
	Type string `json:"type"`
	// Price is the cost of a single check.
	Price Number `json:"price"`
}

// Balance is the account balance.
type Balance struct {
	// Balance is the available account balance.
	Balance Number `json:"balance"`
	// Address is the TRON address your account deposits to.
	Address string `json:"address"`
}

// AddressInfo reports the on-chain resources and token balances of an address.
type AddressInfo struct {
	// Resources holds the currently available energy and bandwidth.
	Resources Resources `json:"resources"`
	// Balances maps token symbols such as "TRX" or "USDT" to their balances.
	Balances map[string]Number `json:"balances"`
}

// Resources holds the available amounts of each TRON resource.
type Resources struct {
	// Energy is the available energy.
	Energy int64 `json:"energy"`
	// Bandwidth is the available bandwidth.
	Bandwidth int64 `json:"bandwidth"`
}

// EnergyEstimate is the energy a transfer needs and what it would cost.
type EnergyEstimate struct {
	// Amount is the estimated resource amount.
	Amount int64 `json:"amount"`
	// Duration is the rental duration in hours the price refers to.
	Duration int `json:"duration"`
	// Price is the cost of the energy.
	Price Number `json:"price"`
	// ActivationFee is the address activation fee included in Total, if any.
	ActivationFee Number `json:"activation_fee"`
	// Total is the total cost.
	Total Number `json:"total"`
	// FromAddress echoes the sender address.
	FromAddress string `json:"from_address"`
	// ToAddress echoes the recipient address.
	ToAddress string `json:"to_address"`
	// ContractAddress echoes the contract the estimate was made against.
	ContractAddress string `json:"contract_address"`
}

// Calculation is the price of a resource purchase.
type Calculation struct {
	// Address echoes the address the quote was made for.
	Address string `json:"address"`
	// Type is the resource type that was priced, for example "energy".
	Type string `json:"type"`
	// Amount is the resource amount that was priced.
	Amount int64 `json:"amount"`
	// Duration is the rental duration in hours.
	Duration int `json:"duration"`
	// Price is the cost of the resources.
	Price Number `json:"price"`
	// ActivationFee is the address activation fee included in Total, if any.
	ActivationFee Number `json:"activation_fee"`
	// Total is the total cost.
	Total Number `json:"total"`
}

// Transaction is a resource purchase or an address activation.
type Transaction struct {
	// ID is the identifier assigned by the API.
	ID string `json:"id"`
	// ExternalID is the identifier you supplied, empty if you supplied none.
	ExternalID string `json:"external_id"`
	// Service is the purchased service, one of the Service* constants.
	Service string `json:"service"`
	// Params echoes the parameters the transaction was created with.
	Params TransactionParams `json:"params"`
	// Status is the current status, one of the TransactionStatus* constants.
	Status string `json:"status"`
	// Amount is the amount charged to your balance.
	Amount Number `json:"amount"`
	// CreatedAt is when the transaction was created.
	CreatedAt Time `json:"created_at"`
	// Hash is the on-chain transaction hash, empty until the transaction settles.
	Hash string `json:"hash"`
}

// Amounts holds the per-resource amounts of a purchase.
type Amounts struct {
	// Energy is the energy amount.
	Energy int64 `json:"energy,omitempty"`
	// Bandwidth is the bandwidth amount.
	Bandwidth int64 `json:"bandwidth,omitempty"`
}

// TransactionParams echoes the parameters a transaction was created with. The
// API has encoded the resource amount under several field names over time, so
// each known spelling is decoded and Raw keeps the object verbatim.
type TransactionParams struct {
	// Address is the address that receives the resources.
	Address string `json:"address"`
	// Duration is the rental duration in hours.
	Duration int `json:"duration"`
	// Amounts holds the per-resource amounts.
	Amounts Amounts `json:"amounts"`
	// Amount is the resource amount when the API reports a single figure.
	Amount int64 `json:"amount"`
	// EnergyAmount is the energy amount under its legacy field name.
	EnergyAmount int64 `json:"energy_amount"`
	// ActivateAddress reports whether activation was requested.
	ActivateAddress bool `json:"activate_address"`
	// Raw is the params object exactly as the API returned it.
	Raw json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the known fields and keeps the original object in Raw.
func (p *TransactionParams) UnmarshalJSON(data []byte) error {
	type params TransactionParams
	var decoded params
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = TransactionParams(decoded)
	p.Raw = bytes.Clone(data)
	return nil
}

// DirectRechargeInfo describes the direct recharge service: pay the returned
// address directly and energy is delivered at the listed rates.
type DirectRechargeInfo struct {
	// Address is the TronZap address to send payment to.
	Address string `json:"address"`
	// Rates lists the available energy rates.
	Rates []DirectRechargeRate `json:"rates"`
}

// DirectRechargeRate is one direct recharge price tier.
type DirectRechargeRate struct {
	// Duration is the rental duration in hours this tier applies to.
	Duration int `json:"duration"`
	// MinEnergy is the smallest purchasable energy amount in this tier.
	MinEnergy int64 `json:"min_energy"`
	// MaxEnergy is the largest purchasable energy amount in this tier.
	MaxEnergy int64 `json:"max_energy"`
	// Price is the cost of 1000 units of energy.
	Price Number `json:"price"`
	// Price32K is the price of 32,000 energy at this tier.
	Price32K Number `json:"price_32k"`
	// Price65K is the price of 65,000 energy at this tier.
	Price65K Number `json:"price_65k"`
	// Price131K is the price of 131,000 energy at this tier.
	Price131K Number `json:"price_131k"`
}

// AMLCheck is an AML screening and, once completed, its result.
type AMLCheck struct {
	// ID identifies the check.
	ID string `json:"id"`
	// Type is [AMLTypeAddress] or [AMLTypeHash].
	Type string `json:"type"`
	// Address is the screened address.
	Address string `json:"address"`
	// Hash is the screened transaction hash, empty for address checks.
	Hash string `json:"hash"`
	// Direction is the screened transaction direction, empty for address checks.
	Direction string `json:"direction"`
	// Network is the blockchain network code.
	Network string `json:"network"`
	// Status is the current status, one of the AMLStatus* constants.
	Status string `json:"status"`
	// RiskScore is the risk score from 0 to 100, nil until the check completes.
	RiskScore *Number `json:"risk_score"`
	// RiskLevel is one of the AMLRiskLevel* constants, empty until the check completes.
	RiskLevel string `json:"risk_level"`
	// Blacklist reports whether the subject appears on a blacklist.
	Blacklist bool `json:"blacklist"`
	// RiskFactors lists the signals that contributed to the score.
	RiskFactors []AMLRiskFactor `json:"risk_factors"`
	// CheckedAt is when the screening ran.
	CheckedAt Time `json:"checked_at"`
}

// AMLRiskFactor is one signal that contributed to an AML risk score.
type AMLRiskFactor struct {
	// Name is the machine-readable factor name.
	Name string `json:"name"`
	// Label is the human-readable factor name.
	Label string `json:"label"`
	// Group is the risk group the factor belongs to, such as "low" or "medium".
	Group string `json:"group"`
	// Score is the factor's weight, from 0 to 1.
	Score Number `json:"score"`
}

// AMLHistory is one page of past AML checks.
type AMLHistory struct {
	// Page is the 1-based number of this page.
	Page int `json:"page"`
	// PerPage is the page size.
	PerPage int `json:"per_page"`
	// Total is the number of checks matching the query across all pages.
	Total int `json:"total"`
	// Items holds the checks on this page.
	Items []AMLCheck `json:"items"`
}

// SubscriptionPlan is a subscription plan on sale.
type SubscriptionPlan struct {
	// SubscriptionID identifies the plan, such as "unlimited_energy". Pass it as
	// [StartSubscriptionRequest.SubscriptionID].
	SubscriptionID string `json:"-"`
	// ID is the plan's numeric identifier.
	ID int64 `json:"id"`
	// Name is the human-readable plan name.
	Name string `json:"name"`
	// ActivationFee is the one-time fee charged when the subscription starts.
	ActivationFee Number `json:"activation_fee"`
	// InitialPrice is the amount charged when the subscription starts.
	InitialPrice Number `json:"initial_price"`
	// Price is the cost of each transaction the subscription serves.
	Price Number `json:"price"`
	// TransactionsLimit is how many transactions the plan covers, 0 for no limit.
	TransactionsLimit int64 `json:"transactions_limit"`
	// DurationDays is how many days the plan runs, 0 for no time limit.
	DurationDays int `json:"duration_days"`
}

// subscriptionPlans decodes the plans object, keyed by plan identifier, into a
// slice that keeps the API's order.
type subscriptionPlans []SubscriptionPlan

func (p *subscriptionPlans) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	plans := subscriptionPlans{}
	switch token {
	case json.Delim('['):
		// An empty plan list can arrive as [], the JSON encoding of an empty PHP array.
		for decoder.More() {
			var plan SubscriptionPlan
			if err := decoder.Decode(&plan); err != nil {
				return err
			}
			plans = append(plans, plan)
		}
	case json.Delim('{'):
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			var plan SubscriptionPlan
			if err := decoder.Decode(&plan); err != nil {
				return err
			}
			plan.SubscriptionID, _ = key.(string)
			plans = append(plans, plan)
		}
	default:
		return fmt.Errorf("tronzap: cannot decode %s as subscription plans", data)
	}
	*p = plans
	return nil
}

// Subscription is an energy subscription for an address.
//
// [Client.StartSubscription], [Client.CheckSubscription] and
// [Client.StopSubscription] report the identifiers, status, dates and Params;
// [Client.GetSubscriptionHistory] reports the usage counters and dates instead
// of Params and ExternalID. Fields a response does not carry are left zero.
type Subscription struct {
	// ID is the identifier assigned by the API.
	ID string `json:"id"`
	// SubscriptionID is the plan the subscription belongs to, such as "unlimited_energy".
	SubscriptionID string `json:"subscription_id"`
	// ExternalID is the identifier you supplied, empty if you supplied none.
	ExternalID string `json:"external_id"`
	// Address is the TRON address the subscription serves.
	Address string `json:"address"`
	// Status is the current status, one of the SubscriptionStatus* constants.
	Status string `json:"status"`
	// Params echoes the parameters the subscription was started with.
	Params SubscriptionParams `json:"params"`
	// TransactionsLimit is how many transactions the subscription covers, 0 for no limit.
	TransactionsLimit int64 `json:"transactions_limit"`
	// TransactionsUsed is how many transactions the subscription has served.
	TransactionsUsed int64 `json:"transactions_used"`
	// EnergyUsed is how much energy the subscription has delegated.
	EnergyUsed int64 `json:"energy_used"`
	// TotalPrice is the amount charged for the subscription so far.
	TotalPrice Number `json:"total_price"`
	// CreatedAt is when the subscription was created.
	CreatedAt Time `json:"created_at"`
	// StartedAt is when the subscription started.
	StartedAt Time `json:"started_at"`
	// RenewedAt is when the subscription was last renewed, zero if never.
	RenewedAt Time `json:"renewed_at"`
	// StoppedAt is when the subscription was stopped, zero if it was not.
	StoppedAt Time `json:"stopped_at"`
	// ExpireAt is when the subscription ends, zero if it has no time limit.
	ExpireAt Time `json:"expire_at"`
}

// SubscriptionParams echoes the parameters a subscription was started with. Raw
// keeps the object verbatim.
type SubscriptionParams struct {
	// Address is the TRON address the subscription serves.
	Address string `json:"address"`
	// DurationDays is how many days the subscription runs, 0 for no time limit.
	DurationDays int `json:"duration"`
	// TransactionsLimit is how many transactions the subscription covers, 0 for no limit.
	TransactionsLimit int64 `json:"transactions_limit"`
	// ActivateAddress reports whether activation was requested.
	ActivateAddress bool `json:"activate_address"`
	// Raw is the params object exactly as the API returned it.
	Raw json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the known fields and keeps the original object in Raw.
func (p *SubscriptionParams) UnmarshalJSON(data []byte) error {
	type params SubscriptionParams
	var decoded params
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = SubscriptionParams(decoded)
	p.Raw = bytes.Clone(data)
	return nil
}

// SubscriptionHistory is one page of your subscriptions.
type SubscriptionHistory struct {
	// Page is the 1-based number of this page.
	Page int `json:"page"`
	// PerPage is the page size.
	PerPage int `json:"per_page"`
	// Total is the number of subscriptions matching the query across all pages.
	Total int `json:"total"`
	// Items holds the subscriptions on this page.
	Items []Subscription `json:"items"`
}
