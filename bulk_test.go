package tronzap_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

const bulkCSV = "row,address,amount,status,reason,charged,transaction_id\n" +
	"1,TPY1Kb8cKAZQfm95gXQQs2Mh8Uygtos21D,65000,delivered,,3.25,01kb2m9c4de5fg6hj7km8np9qr\n" +
	"2,TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE,131000,skipped,address_not_activated,,\n"

const bulkCalculationJSON = `{
	"id": "01kb2m7a1bc2de3fg4hj5km6np",
	"status": "draft",
	"resource": "energy",
	"duration": 1,
	"total_amount": 1240.1,
	"accepted": 998,
	"rejected_count": 2,
	"rejected": [
		{"row": 5, "address": "TPY1Kb8cKAZQfm95gXQQs2Mh8Uygtos21D", "reason": "duplicate"},
		{"row": 9, "address": "abc", "reason": "invalid_address"}
	],
	"expires_at": "2026-11-16T12:15:00+00:00"
}`

const bulkOrderJSON = `{
	"id": "01kb2m5x9qj4v8n3c7d6e5f4g3",
	"external_id": "my-order-42",
	"status": "partial",
	"resource": "energy",
	"duration": 1,
	"force": true,
	"max_price_increase": 20,
	"webhook_url": null,
	"total": 998,
	"processed": 998,
	"delivered": 966,
	"failed": 2,
	"in_review": 0,
	"skipped": 30,
	"skipped_by_reason": {"address_not_activated": 28, "price_increased": 2},
	"rejected_count": 2,
	"total_amount": 1234.56,
	"charged_amount": "1195.02",
	"created_at": "2026-11-16T12:00:00+00:00",
	"started_at": "2026-11-16T12:02:00+00:00",
	"finalized_at": null
}`

func TestBulkCalculate(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(bulkCalculationJSON))

	calculation, err := client.BulkCalculate(context.Background(), tronzap.BulkCalculateRequest{
		Items: []tronzap.BulkItem{
			{Address: "TPY1Kb8cKAZQfm95gXQQs2Mh8Uygtos21D", Amount: 65000},
			{Address: "TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE", Amount: 131000},
		},
		Duration: 24,
	})
	if err != nil {
		t.Fatalf("BulkCalculate: %v", err)
	}
	if got.Path() != "/v1/bulk/calculate" {
		t.Errorf("path = %q", got.Path())
	}
	assertJSONBody(t, got.Body(), `{"resource":"energy","duration":24,"items":[
		{"address":"TPY1Kb8cKAZQfm95gXQQs2Mh8Uygtos21D","amount":65000},
		{"address":"TQn9Y2khEsLJW1ChVWFMSMeRDow5KcbLSE","amount":131000}]}`)

	if calculation.ID != "01kb2m7a1bc2de3fg4hj5km6np" || calculation.Status != tronzap.BulkStatusDraft {
		t.Errorf("calculation = %+v", calculation)
	}
	if calculation.TotalAmount != 1240.1 || calculation.Accepted != 998 || calculation.RejectedCount != 2 {
		t.Errorf("totals = %v %d %d", calculation.TotalAmount, calculation.Accepted, calculation.RejectedCount)
	}
	wantRejected := []tronzap.BulkRejectedRow{
		{Row: 5, Address: "TPY1Kb8cKAZQfm95gXQQs2Mh8Uygtos21D", Reason: "duplicate"},
		{Row: 9, Address: "abc", Reason: "invalid_address"},
	}
	if !reflect.DeepEqual(calculation.Rejected, wantRejected) {
		t.Errorf("Rejected = %+v", calculation.Rejected)
	}
	if !calculation.ExpiresAt.Equal(time.Date(2026, 11, 16, 12, 15, 0, 0, time.UTC)) {
		t.Errorf("ExpiresAt = %v", calculation.ExpiresAt)
	}
}

func TestBulkCalculateDefaultsTheDuration(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(bulkCalculationJSON))

	_, err := client.BulkCalculate(context.Background(), tronzap.BulkCalculateRequest{
		Items: []tronzap.BulkItem{{Address: "TAddress", Amount: 65000}},
	})
	if err != nil {
		t.Fatalf("BulkCalculate: %v", err)
	}
	assertJSONBody(t, got.Body(), `{"resource":"energy","duration":1,"items":[{"address":"TAddress","amount":65000}]}`)
}

func TestBulkCalculateWithNoValidRows(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, `{"code":94,"key":"bulk_no_valid_items","error":"No valid items",
		"result":{"rejected":[{"row":1,"address":"abc","reason":"invalid_address"}]}}`)

	_, err := client.BulkCalculate(context.Background(), tronzap.BulkCalculateRequest{
		Items: []tronzap.BulkItem{{Address: "abc", Amount: 65000}},
	})

	var apiErr *tronzap.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != tronzap.CodeBulkNoValidItems {
		t.Fatalf("err = %v", err)
	}
	var calculation tronzap.BulkCalculation
	if found, err := apiErr.DecodeResult(&calculation); !found || err != nil {
		t.Fatalf("DecodeResult = %v, %v", found, err)
	}
	if len(calculation.Rejected) != 1 || calculation.Rejected[0].Reason != "invalid_address" {
		t.Errorf("Rejected = %+v", calculation.Rejected)
	}
}

func TestBulkCreate(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(bulkOrderJSON))

	order, err := client.BulkCreate(context.Background(), tronzap.BulkCreateRequest{ID: "calc-1"})
	if err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
	if got.Path() != "/v1/bulk/create" {
		t.Errorf("path = %q", got.Path())
	}
	assertJSONBody(t, got.Body(), `{"id":"calc-1"}`)

	want := tronzap.BulkOrder{
		ID:               "01kb2m5x9qj4v8n3c7d6e5f4g3",
		ExternalID:       "my-order-42",
		Status:           tronzap.BulkStatusPartial,
		Resource:         "energy",
		Duration:         1,
		Force:            true,
		MaxPriceIncrease: 20,
		Total:            998,
		Processed:        998,
		Delivered:        966,
		Failed:           2,
		Skipped:          30,
		SkippedByReason:  map[string]int{"address_not_activated": 28, "price_increased": 2},
		RejectedCount:    2,
		TotalAmount:      1234.56,
		ChargedAmount:    1195.02,
	}
	got2 := *order
	got2.CreatedAt, got2.StartedAt, got2.FinalizedAt = tronzap.Time{}, tronzap.Time{}, tronzap.Time{}
	if !reflect.DeepEqual(got2, want) {
		t.Errorf("order = %+v, want %+v", got2, want)
	}
	if order.StartedAt.IsZero() || !order.FinalizedAt.IsZero() {
		t.Errorf("StartedAt = %v, FinalizedAt = %v", order.StartedAt, order.FinalizedAt)
	}
}

func TestBulkCreateWithAllOptions(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(bulkOrderJSON))
	noIncrease := 0

	_, err := client.BulkCreate(context.Background(), tronzap.BulkCreateRequest{
		ID:               "calc-1",
		ExternalID:       "order-42",
		Force:            true,
		MaxPriceIncrease: &noIncrease,
		WebhookURL:       "https://example.com/hook",
	})
	if err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
	assertJSONBody(t, got.Body(), `{"id":"calc-1","force":true,"max_price_increase":0,
		"webhook_url":"https://example.com/hook","external_id":"order-42"}`)
}

func TestBulkCreateWhenThePriceChanged(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, `{"code":92,"key":"bulk_price_changed",
		"error":"Price changed, confirmation required","request_id":"req-92","result":`+bulkCalculationJSON+`}`)

	_, err := client.BulkCreate(context.Background(), tronzap.BulkCreateRequest{ID: "calc-old"})

	var apiErr *tronzap.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != tronzap.CodeBulkPriceChanged {
		t.Fatalf("err = %v", err)
	}
	if apiErr.Key != "bulk_price_changed" || apiErr.RequestID != "req-92" {
		t.Errorf("Key = %q, RequestID = %q", apiErr.Key, apiErr.RequestID)
	}
	var calculation tronzap.BulkCalculation
	if found, err := apiErr.DecodeResult(&calculation); !found || err != nil {
		t.Fatalf("DecodeResult = %v, %v", found, err)
	}
	if calculation.ID != "01kb2m7a1bc2de3fg4hj5km6np" || calculation.TotalAmount != 1240.1 {
		t.Errorf("calculation = %+v", calculation)
	}
}

func TestBulkCheck(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(bulkOrderJSON))

	if _, err := client.BulkCheck(context.Background(), tronzap.BulkOrderRequest{ID: "order-1"}); err != nil {
		t.Fatalf("BulkCheck: %v", err)
	}
	if got.Path() != "/v1/bulk/check" {
		t.Errorf("path = %q", got.Path())
	}
	assertJSONBody(t, got.Body(), `{"id":"order-1"}`)

	if _, err := client.BulkCheck(context.Background(), tronzap.BulkOrderRequest{ExternalID: "my-order"}); err != nil {
		t.Fatalf("BulkCheck: %v", err)
	}
	assertJSONBody(t, got.Body(), `{"external_id":"my-order"}`)
}

func TestBulkHistory(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(`{"page":3,"per_page":50,"total":101,"items":[`+bulkOrderJSON+`]}`))

	history, err := client.BulkHistory(context.Background(), tronzap.BulkHistoryRequest{Page: 3, PerPage: 50})
	if err != nil {
		t.Fatalf("BulkHistory: %v", err)
	}
	if got.Path() != "/v1/bulk/history" {
		t.Errorf("path = %q", got.Path())
	}
	assertJSONBody(t, got.Body(), `{"page":3,"per_page":50}`)
	if history.Total != 101 || len(history.Items) != 1 || history.Items[0].Delivered != 966 {
		t.Errorf("history = %+v", history)
	}

	if _, err := client.BulkHistory(context.Background(), tronzap.BulkHistoryRequest{}); err != nil {
		t.Fatalf("BulkHistory: %v", err)
	}
	assertJSONBody(t, got.Body(), `{"page":1,"per_page":10}`)
}

func TestBulkReport(t *testing.T) {
	client, got := newServerWithHeaders(t, http.StatusOK,
		map[string]string{"Content-Type": "text/csv; charset=UTF-8"}, bulkCSV)

	report, err := client.BulkReport(context.Background(), tronzap.BulkReportRequest{
		ID:     "order-1",
		Status: tronzap.BulkRowSkipped,
	})
	if err != nil {
		t.Fatalf("BulkReport: %v", err)
	}
	if report != bulkCSV {
		t.Errorf("report = %q", report)
	}
	if got.Path() != "/v1/bulk/report" {
		t.Errorf("path = %q", got.Path())
	}
	assertJSONBody(t, got.Body(), `{"id":"order-1","status":"skipped"}`)
}

func TestBulkReportErrorsAreJSON(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, `{"code":90,"key":"bulk_not_found","error":"Bulk order not found"}`)

	_, err := client.BulkReport(context.Background(), tronzap.BulkReportRequest{ExternalID: "missing"})

	var apiErr *tronzap.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != tronzap.CodeBulkNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestBulkReportThatIsNotCSV(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, ok(`{"rows":[]}`))

	_, err := client.BulkReport(context.Background(), tronzap.BulkReportRequest{ID: "order-1"})

	var invalid *tronzap.InvalidResponseError
	if !errors.As(err, &invalid) || invalid.Message != "expected a CSV report" {
		t.Fatalf("err = %v", err)
	}
}

func TestBulkReportCSVWithAnErrorStatus(t *testing.T) {
	client, _ := newServerWithHeaders(t, http.StatusBadGateway, map[string]string{"Content-Type": "text/csv"}, bulkCSV)

	_, err := client.BulkReport(context.Background(), tronzap.BulkReportRequest{ID: "order-1"})

	if !errors.Is(err, tronzap.ErrServer) {
		t.Fatalf("err = %v", err)
	}
}

func TestBulkInvalidRequests(t *testing.T) {
	client, got := newServer(t, http.StatusOK, ok(`{}`))
	ctx := context.Background()

	calls := map[string]func() error{
		"BulkCalculate without items": func() error {
			_, err := client.BulkCalculate(ctx, tronzap.BulkCalculateRequest{})
			return err
		},
		"BulkCreate without ID": func() error {
			_, err := client.BulkCreate(ctx, tronzap.BulkCreateRequest{ExternalID: "order-42"})
			return err
		},
		"BulkCheck without ID": func() error {
			_, err := client.BulkCheck(ctx, tronzap.BulkOrderRequest{})
			return err
		},
		"BulkReport without ID": func() error {
			_, err := client.BulkReport(ctx, tronzap.BulkReportRequest{Status: tronzap.BulkRowFailed})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, tronzap.ErrInvalidRequest) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if got.Requests() != 0 {
		t.Errorf("%d requests were sent", got.Requests())
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	client := tronzap.NewClient(testToken, testSecret)
	body := []byte(`{"id":"01kb2m5x9qj4v8n3c7d6e5f4g3","external_id":"my-order-42","status":"partial"}`)
	sign := func(secret string) string {
		digest := sha256.Sum256(append(append([]byte{}, body...), secret...))
		return hex.EncodeToString(digest[:])
	}
	signature := sign(testSecret)

	tests := []struct {
		name      string
		body      []byte
		signature string
		want      bool
	}{
		{"valid", body, signature, true},
		{"upper case with spaces", body, " " + strings.ToUpper(signature) + "\n", true},
		{"changed body", append(append([]byte{}, body...), ' '), signature, false},
		{"empty signature", body, "", false},
		{"non-ASCII signature", body, "підпис", false},
		{"other secret", body, sign("other-secret"), false},
	}
	for _, tt := range tests {
		if got := client.VerifyWebhookSignature(tt.body, tt.signature); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestAPIErrorResult(t *testing.T) {
	tests := map[string]struct {
		response string
		found    bool
	}{
		"with result":    {`{"code":92,"error":"Price changed","result":{"id":"calc-2"}}`, true},
		"null result":    {`{"code":6,"error":"Insufficient funds","result":null}`, false},
		"without result": {`{"code":6,"error":"Insufficient funds"}`, false},
	}
	for name, tt := range tests {
		client, _ := newServer(t, http.StatusOK, tt.response)
		_, err := client.GetBalance(context.Background())

		var apiErr *tronzap.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("%s: err = %v", name, err)
		}
		var result struct {
			ID string `json:"id"`
		}
		found, decodeErr := apiErr.DecodeResult(&result)
		if found != tt.found || decodeErr != nil {
			t.Errorf("%s: DecodeResult = %v, %v", name, found, decodeErr)
		}
		if tt.found && result.ID != "calc-2" {
			t.Errorf("%s: result = %+v", name, result)
		}
		if !tt.found && apiErr.Result != nil {
			t.Errorf("%s: Result = %s, want nil", name, apiErr.Result)
		}
	}
}

func TestAPIErrorResultOfTheWrongShape(t *testing.T) {
	apiErr := &tronzap.APIError{Result: []byte(`[1,2]`)}
	var calculation tronzap.BulkCalculation

	if found, err := apiErr.DecodeResult(&calculation); found || err == nil {
		t.Errorf("DecodeResult = %v, %v", found, err)
	}
}

func TestRetryAfter(t *testing.T) {
	rateLimited := `{"code":429,"error":"Too Many Requests"}`
	tests := map[string]struct {
		header   string
		response string
		want     time.Duration
	}{
		"seconds":                   {"17", rateLimited, 17 * time.Second},
		"date in the past":          {"Wed, 21 Oct 2015 07:28:00 GMT", rateLimited, 0},
		"body field":                {"", `{"code":429,"error":"Too Many Requests","retry_after":30}`, 30 * time.Second},
		"header wins over body":     {"5", `{"code":429,"error":"Too Many Requests","retry_after":30}`, 5 * time.Second},
		"unparsable header":         {"soon", `{"code":429,"error":"Too Many Requests","retry_after":30}`, 30 * time.Second},
		"negative body field":       {"", `{"code":429,"error":"Too Many Requests","retry_after":-1}`, 0},
		"string body field":         {"", `{"code":429,"error":"Too Many Requests","retry_after":"30"}`, 0},
		"signed header":             {"+5", rateLimited, 0},
		"overflowing header":        {"99999999999999999999", rateLimited, 0},
		"neither header nor field":  {"", rateLimited, 0},
		"seconds past the duration": {"9223372037", rateLimited, 0},
	}
	for name, tt := range tests {
		headers := map[string]string{}
		if tt.header != "" {
			headers["Retry-After"] = tt.header
		}
		client, _ := newServerWithHeaders(t, http.StatusTooManyRequests, headers, tt.response)
		_, err := client.GetBalance(context.Background())

		var apiErr *tronzap.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != tronzap.CodeRateLimited || apiErr.StatusCode != 429 {
			t.Fatalf("%s: err = %v", name, err)
		}
		if apiErr.RetryAfter != tt.want {
			t.Errorf("%s: RetryAfter = %v, want %v", name, apiErr.RetryAfter, tt.want)
		}
	}
}

func TestRetryAfterAsAFutureDate(t *testing.T) {
	when := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	client, _ := newServerWithHeaders(t, http.StatusTooManyRequests,
		map[string]string{"Retry-After": when}, `{"code":429,"error":"Too Many Requests"}`)

	_, err := client.GetBalance(context.Background())

	var apiErr *tronzap.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v", err)
	}
	if apiErr.RetryAfter < 110*time.Second || apiErr.RetryAfter > 120*time.Second {
		t.Errorf("RetryAfter = %v", apiErr.RetryAfter)
	}
	if apiErr.RetryAfter%time.Second != 0 {
		t.Errorf("RetryAfter = %v, want whole seconds", apiErr.RetryAfter)
	}
}

func TestRateLimitWithoutAPIPayload(t *testing.T) {
	client, _ := newServerWithHeaders(t, http.StatusTooManyRequests,
		map[string]string{"Retry-After": "60", "Content-Type": "text/html"}, "<html>slow down</html>")

	_, err := client.GetBalance(context.Background())

	var httpErr *tronzap.HTTPError
	if !errors.As(err, &httpErr) || !errors.Is(err, tronzap.ErrRateLimit) {
		t.Fatalf("err = %v", err)
	}
	if httpErr.RetryAfter != time.Minute || httpErr.Body != "<html>slow down</html>" {
		t.Errorf("RetryAfter = %v, Body = %q", httpErr.RetryAfter, httpErr.Body)
	}
}
