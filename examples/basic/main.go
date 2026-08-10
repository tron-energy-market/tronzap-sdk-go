// Command basic walks through the TronZap API operations.
//
// By default it only reads and spends nothing. Credentials come from the
// environment, and the optional variables unlock the calls that need a subject to
// look at.
//
//	export TRONZAP_API_TOKEN=your_api_token
//	export TRONZAP_API_SECRET=your_api_secret
//	export TRONZAP_BASE_URL=api.tronzap.com      # optional, e.g. a dev host
//	export TRONZAP_ADDRESS=TRON_ADDRESS          # optional
//	export TRONZAP_FROM_ADDRESS=TRON_ADDRESS     # optional, with TO_ADDRESS
//	export TRONZAP_TO_ADDRESS=TRON_ADDRESS       # optional, with FROM_ADDRESS
//	export TRONZAP_TRANSACTION_ID=id             # optional
//	export TRONZAP_AML_CHECK_ID=id               # optional
//	go run ./examples/basic
//
// Setting TRONZAP_ALLOW_PURCHASES=1 additionally exercises the endpoints that
// create transactions and AML checks. Those DEBIT THE ACCOUNT BALANCE. It is
// meant for verifying an integration against a development environment, and it
// also needs TRONZAP_ADDRESS.
//
//	export TRONZAP_ALLOW_PURCHASES=1
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

// Purchase sizes for the opt-in write calls. Energy and bandwidth are at the
// documented minimums so a verification run costs as little as possible.
const (
	energyAmount    = 65000
	bandwidthAmount = 345
	bundleEnergy    = 65000
	bundleBandwidth = 345
)

func main() {
	apiToken := os.Getenv("TRONZAP_API_TOKEN")
	apiSecret := os.Getenv("TRONZAP_API_SECRET")
	if apiToken == "" || apiSecret == "" {
		log.Fatal("set TRONZAP_API_TOKEN and TRONZAP_API_SECRET")
	}

	opts := []tronzap.Option{
		tronzap.WithTimeout(20 * time.Second),
		tronzap.WithUserAgent("tronzap-example/1.0"),
	}
	baseURL := os.Getenv("TRONZAP_BASE_URL")
	if baseURL != "" {
		opts = append(opts, tronzap.WithBaseURL(baseURL))
	}
	client := tronzap.NewClient(apiToken, apiSecret, opts...)

	if baseURL == "" {
		baseURL = tronzap.DefaultBaseURL
	}
	fmt.Printf("Calling %s\n", baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := run(ctx, client); err != nil {
		log.Fatal(describe(err))
	}
}

// run exercises every read-only endpoint. It keeps going after a failure so that
// one endpoint being unavailable does not hide the state of the others, and
// reports which ones failed at the end.
func run(ctx context.Context, client *tronzap.Client) error {
	var failed []string
	step := func(name string, call func() error) {
		if err := call(); err != nil {
			failed = append(failed, name)
			fmt.Printf("\nFAILED %s: %s\n", name, describe(err))
		}
	}

	step("balance", func() error { return showAccount(ctx, client) })
	step("services", func() error { return showCatalogue(ctx, client) })
	step("aml-checks/history", func() error { return showAMLHistory(ctx, client) })

	// The remaining calls need something to look at, so each is skipped unless its
	// variable is set.
	if address := os.Getenv("TRONZAP_ADDRESS"); address != "" {
		step("address-info, calculate", func() error { return showAddress(ctx, client, address) })
	} else {
		skipped("address-info, calculate", "TRONZAP_ADDRESS")
	}

	from, to := os.Getenv("TRONZAP_FROM_ADDRESS"), os.Getenv("TRONZAP_TO_ADDRESS")
	if from != "" && to != "" {
		step("estimate-energy", func() error { return showEstimate(ctx, client, from, to) })
	} else {
		skipped("estimate-energy", "TRONZAP_FROM_ADDRESS and TRONZAP_TO_ADDRESS")
	}

	if id := os.Getenv("TRONZAP_TRANSACTION_ID"); id != "" {
		step("transaction/check", func() error { return showTransaction(ctx, client, id) })
	} else {
		skipped("transaction/check", "TRONZAP_TRANSACTION_ID")
	}

	if id := os.Getenv("TRONZAP_AML_CHECK_ID"); id != "" {
		step("aml-checks/check", func() error { return showAMLCheck(ctx, client, id) })
	} else {
		skipped("aml-checks/check", "TRONZAP_AML_CHECK_ID")
	}

	switch address := os.Getenv("TRONZAP_ADDRESS"); {
	case !purchasesAllowed():
		skipped("transaction/new and aml-checks/new, which spend funds", "TRONZAP_ALLOW_PURCHASES=1")
	case address == "":
		skipped("transaction/new and aml-checks/new", "TRONZAP_ADDRESS as well")
	default:
		fmt.Println("\n=== TRONZAP_ALLOW_PURCHASES is set: the calls below DEBIT THE BALANCE ===")
		step("transaction/new activate_address", func() error { return buyActivation(ctx, client, address) })
		step("transaction/new energy", func() error { return buyEnergy(ctx, client, address) })
		step("transaction/new bandwidth", func() error { return buyBandwidth(ctx, client, address) })
		step("transaction/new resource_bundle", func() error { return buyResourceBundle(ctx, client, address) })
		step("aml-checks/new", func() error { return createAMLCheck(ctx, client, address) })
	}

	if len(failed) > 0 {
		return fmt.Errorf("%d of the attempted endpoints failed: %v", len(failed), failed)
	}
	fmt.Println("\nEvery configured endpoint answered successfully.")
	return nil
}

func showAccount(ctx context.Context, client *tronzap.Client) error {
	balance, err := client.GetBalance(ctx)
	if err != nil {
		return fmt.Errorf("balance: %w", err)
	}
	fmt.Printf("\nBalance: %s (deposit to %s)\n", balance.Balance, balance.Address)
	return nil
}

func showCatalogue(ctx context.Context, client *tronzap.Client) error {
	services, err := client.GetServices(ctx)
	if err != nil {
		return fmt.Errorf("services: %w", err)
	}
	fmt.Println("\nEnergy tiers:")
	for _, tier := range services.Energy {
		fmt.Printf("  %d-%d energy for %dh: %s per unit, 65k costs %s\n",
			tier.MinEnergy, tier.MaxEnergy, tier.Duration, tier.Price, tier.Price65K)
	}
	fmt.Println("Bandwidth tiers:")
	for _, tier := range services.Bandwidth {
		fmt.Printf("  %d-%d bandwidth for %dh: %s per 1000 units, %d costs %s\n",
			tier.MinAmount, tier.MaxAmount, tier.Duration, tier.Price,
			bandwidthAmount, tronzap.Number(tier.Price.Float64()*bandwidthAmount/1000))
	}
	fmt.Printf("Address activation: %s\n", services.ActivateAddress.Price)

	amlServices, err := client.GetAMLServices(ctx)
	if err != nil {
		return fmt.Errorf("aml services: %w", err)
	}
	fmt.Println("AML services:")
	for _, service := range amlServices {
		fmt.Printf("  %s check: %s\n", service.Type, service.Price)
	}

	recharge, err := client.GetDirectRechargeInfo(ctx)
	if err != nil {
		return fmt.Errorf("direct recharge info: %w", err)
	}
	fmt.Printf("Direct recharge: pay %s, %d rates available\n", recharge.Address, len(recharge.Rates))
	return nil
}

func showAMLHistory(ctx context.Context, client *tronzap.Client) error {
	history, err := client.GetAMLHistory(ctx, tronzap.AMLHistoryRequest{PerPage: 3})
	if err != nil {
		return fmt.Errorf("aml history: %w", err)
	}
	fmt.Printf("\nAML history: %d checks total, %d on page %d\n",
		history.Total, len(history.Items), history.Page)
	for _, check := range history.Items {
		fmt.Printf("  %s %s %s risk=%s\n", check.ID, check.Type, check.Status, check.RiskLevel)
	}
	return nil
}

func showAddress(ctx context.Context, client *tronzap.Client, address string) error {
	info, err := client.GetAddressInfo(ctx, address)
	if err != nil {
		return fmt.Errorf("address info: %w", err)
	}
	fmt.Printf("\n%s: %d energy, %d bandwidth, %s TRX, %s USDT\n",
		address, info.Resources.Energy, info.Resources.Bandwidth,
		info.Balances["TRX"], info.Balances["USDT"])

	quote, err := client.Calculate(ctx, tronzap.CalculateRequest{
		Address:  address,
		Energy:   65000,
		Duration: 1,
	})
	if err != nil {
		return fmt.Errorf("calculate: %w", err)
	}
	fmt.Printf("65000 energy for 1h costs %s (activation fee %s)\n", quote.Total, quote.ActivationFee)
	return nil
}

func showEstimate(ctx context.Context, client *tronzap.Client, from, to string) error {
	estimate, err := client.EstimateEnergy(ctx, tronzap.EstimateEnergyRequest{
		FromAddress: from,
		ToAddress:   to,
	})
	if err != nil {
		return fmt.Errorf("estimate energy: %w", err)
	}
	fmt.Printf("\nTransfer %s -> %s needs %d energy, costs %s\n",
		from, to, estimate.Energy, estimate.Total)
	return nil
}

func showTransaction(ctx context.Context, client *tronzap.Client, id string) error {
	tx, err := client.CheckTransaction(ctx, tronzap.CheckTransactionRequest{ID: id})
	if err != nil {
		return fmt.Errorf("check transaction: %w", err)
	}
	fmt.Printf("\nTransaction %s: service=%s status=%s amount=%s created=%s hash=%s\n",
		tx.ID, tx.Service, tx.Status, tx.Amount, timestamp(tx.CreatedAt), tx.Hash)
	return nil
}

// timestamp renders the parsed time, falling back to the raw text when the API
// used a format the SDK does not recognise. Printing Raw unconditionally would
// hide exactly that case.
func timestamp(t tronzap.Time) string {
	switch {
	case !t.IsZero():
		return t.UTC().Format(time.RFC3339)
	case t.Raw != "":
		return fmt.Sprintf("UNPARSED(%q)", t.Raw)
	default:
		return "none"
	}
}

func showAMLCheck(ctx context.Context, client *tronzap.Client, id string) error {
	check, err := client.CheckAMLStatus(ctx, id)
	if err != nil {
		return fmt.Errorf("check aml status: %w", err)
	}
	score := "n/a"
	if check.RiskScore != nil {
		score = check.RiskScore.String()
	}
	fmt.Printf("\nAML check %s: status=%s risk=%s score=%s blacklist=%t factors=%d checked=%s\n",
		check.ID, check.Status, check.RiskLevel, score, check.Blacklist,
		len(check.RiskFactors), timestamp(check.CheckedAt))
	for _, factor := range check.RiskFactors {
		fmt.Printf("  %s (%s): %s\n", factor.Label, factor.Group, factor.Score)
	}
	return nil
}

// purchasesAllowed reports whether the caller opted into the write endpoints.
func purchasesAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRONZAP_ALLOW_PURCHASES"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// externalID builds an identifier unique per run, so repeated runs never collide
// on an external id the API has already seen.
func externalID(prefix string) string {
	return fmt.Sprintf("example-%s-%d", prefix, time.Now().UnixNano())
}

// An address must be activated once before it can hold resources. Being already
// activated is the expected outcome on a reused address, not a failure.
func buyActivation(ctx context.Context, client *tronzap.Client, address string) error {
	tx, err := client.CreateAddressActivationTransaction(ctx, tronzap.AddressActivationRequest{
		Address:    address,
		ExternalID: externalID("activation"),
	})

	var apiErr *tronzap.APIError
	if errors.As(err, &apiErr) && apiErr.Code == tronzap.CodeAddressAlreadyActivated {
		fmt.Println("\nactivate_address: already activated, nothing to buy")
		return nil
	}
	if err != nil {
		return err
	}
	return confirm(ctx, client, "activate_address", tx)
}

func buyEnergy(ctx context.Context, client *tronzap.Client, address string) error {
	tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
		Address:    address,
		Energy:     energyAmount,
		Duration:   1,
		ExternalID: externalID("energy"),
	})
	if err != nil {
		return err
	}
	return confirm(ctx, client, "energy", tx)
}

func buyBandwidth(ctx context.Context, client *tronzap.Client, address string) error {
	tx, err := client.CreateBandwidthTransaction(ctx, tronzap.BandwidthTransactionRequest{
		Address:    address,
		Bandwidth:  bandwidthAmount,
		ExternalID: externalID("bandwidth"),
	})
	if err != nil {
		return err
	}
	return confirm(ctx, client, "bandwidth", tx)
}

func buyResourceBundle(ctx context.Context, client *tronzap.Client, address string) error {
	tx, err := client.CreateResourceBundleTransaction(ctx, tronzap.ResourceBundleTransactionRequest{
		Address:    address,
		Energy:     bundleEnergy,
		Bandwidth:  bundleBandwidth,
		Duration:   1,
		ExternalID: externalID("bundle"),
	})
	if err != nil {
		return err
	}
	return confirm(ctx, client, "resource_bundle", tx)
}

func createAMLCheck(ctx context.Context, client *tronzap.Client, address string) error {
	check, err := client.CreateAMLCheck(ctx, tronzap.AMLCheckRequest{
		Type:    tronzap.AMLTypeAddress,
		Network: "TRX",
		Address: address,
	})
	if err != nil {
		return err
	}
	fmt.Printf("\naml-checks/new: created %s status=%s\n", check.ID, check.Status)
	return showAMLCheck(ctx, client, check.ID)
}

// confirm reads a freshly created transaction back, which checks that the id the
// API returned is one it can be looked up by.
func confirm(ctx context.Context, client *tronzap.Client, label string, tx *tronzap.Transaction) error {
	fmt.Printf("\n%s: created %s external=%s status=%s charged=%s\n",
		label, tx.ID, tx.ExternalID, tx.Status, tx.Amount)

	reread, err := client.CheckTransaction(ctx, tronzap.CheckTransactionRequest{ID: tx.ID})
	if err != nil {
		return fmt.Errorf("re-reading %s: %w", label, err)
	}
	fmt.Printf("%s: re-read status=%s hash=%s\n", label, reread.Status, reread.Hash)
	return nil
}

func skipped(endpoints, variables string) {
	fmt.Printf("\nSkipped %s — set %s to include it.\n", endpoints, variables)
}

// describe turns an SDK error into an actionable message.
func describe(err error) string {
	var apiErr *tronzap.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case tronzap.CodeAuth:
			return "authentication failed: check TRONZAP_API_TOKEN and TRONZAP_API_SECRET"
		case tronzap.CodeInsufficientFunds:
			return "insufficient funds: top up the account at https://tronzap.com"
		case tronzap.CodeInvalidTronAddress:
			return fmt.Sprintf("invalid TRON address (%s)", apiErr.Key)
		default:
			return fmt.Sprintf("api error %d [%s]: %s (request %s)",
				apiErr.Code, apiErr.Key, apiErr.Message, apiErr.RequestID)
		}
	}

	switch {
	case errors.Is(err, tronzap.ErrRateLimit):
		return "rate limited: slow down and retry"
	case errors.Is(err, tronzap.ErrUnauthorized):
		return "unauthorized: the token or the signature was rejected"
	case errors.Is(err, tronzap.ErrTimeout):
		return "the request timed out"
	case errors.Is(err, tronzap.ErrTLS):
		return fmt.Sprintf("TLS failure: %v", err)
	case errors.Is(err, tronzap.ErrNetwork):
		return fmt.Sprintf("network failure: %v", err)
	case errors.Is(err, tronzap.ErrInvalidResponse):
		return fmt.Sprintf("the API returned something unexpected: %v", err)
	default:
		return err.Error()
	}
}
