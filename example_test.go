package tronzap_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

// The examples below are compiled but not run, because each one would call the
// live API.

func Example() {
	client := tronzap.NewClient("your_api_token", "your_api_secret")

	balance, err := client.GetBalance(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("balance: %s, deposit address: %s\n", balance.Balance, balance.Address)
}

func ExampleNewClient_options() {
	client := tronzap.NewClient("your_api_token", "your_api_secret",
		tronzap.WithBaseURL("https://api.tronzap.com"),
		tronzap.WithTimeout(10*time.Second),
		tronzap.WithHTTPClient(&http.Client{Transport: http.DefaultTransport}),
		tronzap.WithUserAgent("my-app/1.0"),
	)
	_ = client
}

// Estimating first and then buying exactly that much energy is the usual way to
// make a USDT transfer cheaper.
func ExampleClient_CreateEnergyTransaction() {
	client := tronzap.NewClient("your_api_token", "your_api_secret")
	ctx := context.Background()

	estimate, err := client.EstimateEnergy(ctx, tronzap.EstimateEnergyRequest{
		FromAddress: "TSenderAddress",
		ToAddress:   "TRecipientAddress",
	})
	if err != nil {
		log.Fatal(err)
	}

	tx, err := client.CreateEnergyTransaction(ctx, tronzap.EnergyTransactionRequest{
		Address:         "TRecipientAddress",
		Energy:          estimate.Amount,
		Duration:        1,
		ExternalID:      "order-42",
		ActivateAddress: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("transaction %s costs %s and is %s\n", tx.ID, tx.Amount, tx.Status)
}

// A resource bundle buys energy and bandwidth in one transaction.
func ExampleClient_CreateResourceBundleTransaction() {
	client := tronzap.NewClient("your_api_token", "your_api_secret")

	tx, err := client.CreateResourceBundleTransaction(context.Background(),
		tronzap.ResourceBundleTransactionRequest{
			Address:    "TRecipientAddress",
			Energy:     65000,
			Bandwidth:  345,
			Duration:   1,
			ExternalID: "bundle-1",
		})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(tx.ID, tx.Status)
}

// Polling stops once the transaction reaches a terminal status.
func ExampleClient_CheckTransaction() {
	client := tronzap.NewClient("your_api_token", "your_api_secret")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	for {
		tx, err := client.CheckTransaction(ctx, tronzap.CheckTransactionRequest{ExternalID: "order-42"})
		if err != nil {
			log.Fatal(err)
		}
		if tx.Status == tronzap.TransactionStatusSuccess || tx.Status == tronzap.TransactionStatusFailed {
			fmt.Printf("finished as %s, on-chain hash %s\n", tx.Status, tx.Hash)
			return
		}

		select {
		case <-ctx.Done():
			log.Fatal(ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

// An AML screening runs asynchronously, so create it and then poll for the result.
func ExampleClient_CreateAMLCheck() {
	client := tronzap.NewClient("your_api_token", "your_api_secret")
	ctx := context.Background()

	check, err := client.CreateAMLCheck(ctx, tronzap.AMLCheckRequest{
		Type:    tronzap.AMLTypeAddress,
		Network: "TRX",
		Address: "TAddressToScreen",
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := client.CheckAMLStatus(ctx, check.ID)
	if err != nil {
		log.Fatal(err)
	}
	if result.Status == tronzap.AMLStatusCompleted {
		score := "n/a"
		if result.RiskScore != nil {
			score = result.RiskScore.String()
		}
		fmt.Printf("risk %s (%s), blacklisted: %t\n", result.RiskLevel, score, result.Blacklist)
		for _, factor := range result.RiskFactors {
			fmt.Printf("  %s: %s\n", factor.Label, factor.Score)
		}
	}
}

// Errors carry enough structure to decide whether to fix the request, top up the
// account, or retry.
func ExampleAPIError() {
	client := tronzap.NewClient("your_api_token", "your_api_secret")

	_, err := client.CreateEnergyTransaction(context.Background(), tronzap.EnergyTransactionRequest{
		Address: "not-a-tron-address",
		Energy:  65000,
	})

	var apiErr *tronzap.APIError
	switch {
	case err == nil:
		fmt.Println("bought energy")
	case errors.As(err, &apiErr):
		switch apiErr.Code {
		case tronzap.CodeInvalidTronAddress:
			fmt.Println("bad address:", apiErr.Key)
		case tronzap.CodeInsufficientFunds:
			fmt.Println("top up the account")
		case tronzap.CodeAddressNotActivated:
			fmt.Println("activate the address first")
		default:
			fmt.Printf("api error %d: %s (request %s)\n", apiErr.Code, apiErr.Message, apiErr.RequestID)
		}
	case errors.Is(err, tronzap.ErrRateLimit):
		fmt.Println("back off and retry")
	case errors.Is(err, tronzap.ErrUnauthorized):
		fmt.Println("check the api token and secret")
	case errors.Is(err, tronzap.ErrTimeout), errors.Is(err, tronzap.ErrServer):
		fmt.Println("transient failure, safe to retry")
	case errors.Is(err, tronzap.ErrNetwork):
		fmt.Println("network unreachable:", err)
	default:
		fmt.Println("unexpected failure:", err)
	}
}

// Do reaches endpoints this SDK does not wrap yet.
func ExampleClient_Do() {
	client := tronzap.NewClient("your_api_token", "your_api_secret")

	var result struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	params := map[string]any{"page": 1, "per_page": 10}
	if err := client.Do(context.Background(), "/v1/subscriptions/history", params, &result); err != nil {
		log.Fatal(err)
	}
	for _, item := range result.Items {
		fmt.Println(item.ID, item.Status)
	}
}
