package tronzap_test

import (
	"encoding/json"
	"testing"
	"time"

	tronzap "github.com/tron-energy-market/tronzap-sdk-go"
)

func TestNumberUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{name: "integer", input: `42`, want: 42},
		{name: "float", input: `3.66`, want: 3.66},
		{name: "quoted float", input: `"3.66"`, want: 3.66},
		{name: "quoted integer", input: `"64300"`, want: 64300},
		{name: "high precision", input: `0.052300000`, want: 0.0523},
		{name: "negative", input: `-1.5`, want: -1.5},
		{name: "quoted negative", input: `"-1.5"`, want: -1.5},
		{name: "exponent", input: `1e3`, want: 1000},
		{name: "null", input: `null`, want: 0},
		{name: "empty string", input: `""`, want: 0},
		{name: "not a number", input: `"abc"`, wantErr: true},
		{name: "object", input: `{"a":1}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got tronzap.Number
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %s, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal %s: %v", tt.input, err)
			}
			if got.Float64() != tt.want {
				t.Errorf("got %v, want %v", got.Float64(), tt.want)
			}
		})
	}
}

func TestNumberMarshal(t *testing.T) {
	tests := []struct {
		value tronzap.Number
		want  string
	}{
		{value: 0, want: `0`},
		{value: 3.66, want: `3.66`},
		{value: 0.0523, want: `0.0523`},
		{value: 64300, want: `64300`},
		{value: -1.5, want: `-1.5`},
	}

	for _, tt := range tests {
		encoded, err := json.Marshal(tt.value)
		if err != nil {
			t.Fatalf("marshal %v: %v", tt.value, err)
		}
		if string(encoded) != tt.want {
			t.Errorf("marshal %v = %s, want %s", tt.value, encoded, tt.want)
		}
	}
}

func TestNumberString(t *testing.T) {
	if got := tronzap.Number(0.0523).String(); got != "0.0523" {
		t.Errorf("String() = %q, want 0.0523", got)
	}
	if got := tronzap.Number(64300).String(); got != "64300" {
		t.Errorf("String() = %q, want 64300", got)
	}
}

// A Number nested in a struct must round-trip through both encodings.
func TestNumberInStruct(t *testing.T) {
	var decoded struct {
		Price tronzap.Number `json:"price"`
		Total tronzap.Number `json:"total"`
	}
	if err := json.Unmarshal([]byte(`{"price":"1.67","total":3.4}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Price.Float64() != 1.67 || decoded.Total.Float64() != 3.4 {
		t.Fatalf("decoded = %+v", decoded)
	}

	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `{"price":1.67,"total":3.4}` {
		t.Errorf("marshal = %s", encoded)
	}
}

func TestTimeUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantRaw string
	}{
		{
			name:    "RFC 3339",
			input:   `"2026-08-07T10:42:12Z"`,
			want:    time.Date(2026, 8, 7, 10, 42, 12, 0, time.UTC),
			wantRaw: "2026-08-07T10:42:12Z",
		},
		{
			name:    "RFC 3339 with nanoseconds",
			input:   `"2026-08-07T10:42:12.5Z"`,
			want:    time.Date(2026, 8, 7, 10, 42, 12, 500000000, time.UTC),
			wantRaw: "2026-08-07T10:42:12.5Z",
		},
		{
			name:    "RFC 3339 with an offset",
			input:   `"2026-08-07T13:42:12+03:00"`,
			want:    time.Date(2026, 8, 7, 10, 42, 12, 0, time.UTC),
			wantRaw: "2026-08-07T13:42:12+03:00",
		},
		{
			name:    "space separated",
			input:   `"2026-08-07 10:42:12"`,
			want:    time.Date(2026, 8, 7, 10, 42, 12, 0, time.UTC),
			wantRaw: "2026-08-07 10:42:12",
		},
		{
			name:    "without a zone",
			input:   `"2026-08-07T10:42:12"`,
			want:    time.Date(2026, 8, 7, 10, 42, 12, 0, time.UTC),
			wantRaw: "2026-08-07T10:42:12",
		},
		{
			name:    "date only",
			input:   `"2026-08-07"`,
			want:    time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
			wantRaw: "2026-08-07",
		},
		{
			name:    "unix seconds",
			input:   `1786185732`,
			want:    time.Unix(1786185732, 0).UTC(),
			wantRaw: "1786185732",
		},
		{
			name:  "null",
			input: `null`,
		},
		{
			name:  "empty string",
			input: `""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got tronzap.Time
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("unmarshal %s: %v", tt.input, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("time = %v, want %v", got.Time, tt.want)
			}
			if got.Raw != tt.wantRaw {
				t.Errorf("raw = %q, want %q", got.Raw, tt.wantRaw)
			}
		})
	}
}

// An unrecognised timestamp must not fail the surrounding response: the value is
// kept verbatim so the caller can still see what arrived.
func TestTimeTolerantOfUnknownFormats(t *testing.T) {
	var got tronzap.Time
	if err := json.Unmarshal([]byte(`"07/08/2026 tea time"`), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("time = %v, want the zero time", got.Time)
	}
	if got.Raw != "07/08/2026 tea time" {
		t.Errorf("raw = %q", got.Raw)
	}
}

func TestTimeMarshal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "re-emits the original encoding", input: `"2026-08-07 10:42:12"`, want: `"2026-08-07 10:42:12"`},
		{name: "null stays null", input: `null`, want: `null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var value tronzap.Time
			if err := json.Unmarshal([]byte(tt.input), &value); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(encoded) != tt.want {
				t.Errorf("marshal = %s, want %s", encoded, tt.want)
			}
		})
	}

	value := tronzap.Time{Time: time.Date(2026, 8, 7, 10, 42, 12, 0, time.UTC)}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `"2026-08-07T10:42:12Z"` {
		t.Errorf("marshal of a constructed time = %s", encoded)
	}
}

// The API has spelled the resource amount several ways, so every known field is
// decoded and the original object is kept for anything new.
func TestTransactionParamsDecoding(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantEnergy       int64
		wantBandwidth    int64
		wantAmount       int64
		wantEnergyAmount int64
	}{
		{
			name:       "amounts map",
			input:      `{"address":"T1","amounts":{"energy":65000},"duration":1}`,
			wantEnergy: 65000,
		},
		{
			name:          "resource bundle",
			input:         `{"address":"T1","amounts":{"energy":65000,"bandwidth":345},"duration":1}`,
			wantEnergy:    65000,
			wantBandwidth: 345,
		},
		{
			name:             "legacy energy_amount",
			input:            `{"address":"T1","energy_amount":65000,"duration":1}`,
			wantEnergyAmount: 65000,
		},
		{
			name:       "single amount field",
			input:      `{"address":"T1","amount":65000,"duration":1}`,
			wantAmount: 65000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var params tronzap.TransactionParams
			if err := json.Unmarshal([]byte(tt.input), &params); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if params.Address != "T1" || params.Duration != 1 {
				t.Errorf("params = %+v", params)
			}
			if params.Amounts.Energy != tt.wantEnergy {
				t.Errorf("amounts.energy = %d, want %d", params.Amounts.Energy, tt.wantEnergy)
			}
			if params.Amounts.Bandwidth != tt.wantBandwidth {
				t.Errorf("amounts.bandwidth = %d, want %d", params.Amounts.Bandwidth, tt.wantBandwidth)
			}
			if params.Amount != tt.wantAmount {
				t.Errorf("amount = %d, want %d", params.Amount, tt.wantAmount)
			}
			if params.EnergyAmount != tt.wantEnergyAmount {
				t.Errorf("energy_amount = %d, want %d", params.EnergyAmount, tt.wantEnergyAmount)
			}
			if string(params.Raw) != tt.input {
				t.Errorf("raw = %s, want %s", params.Raw, tt.input)
			}
		})
	}
}

// Fields the SDK does not model yet stay reachable through Raw.
func TestTransactionParamsKeepsUnknownFields(t *testing.T) {
	var params tronzap.TransactionParams
	input := `{"address":"T1","duration":1,"future_field":"value"}`
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(params.Raw, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if raw["future_field"] != "value" {
		t.Errorf("raw = %v", raw)
	}
}
