package models

import (
	"encoding/json"
	"testing"
)

func TestFlexibleStringUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    FlexibleString
		wantErr bool
	}{
		{"null becomes empty string", `null`, "", false},
		{"plain string value", `"insufficient balance"`, "insufficient balance", false},
		{"empty string value", `""`, "", false},
		{"integer code coerced to string", `403`, "403", false},
		{"float code coerced to string", `4.5`, "4.5", false},
		{"invalid type falls through to error", `{"nested":true}`, "", true},
		{"array falls through to error", `[1,2]`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexibleString
			err := f.UnmarshalJSON([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%q)", f)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f != tt.want {
				t.Errorf("got %q, want %q", f, tt.want)
			}
		})
	}
}

func TestOpenAIAPIErrorWithFlexibleCode(t *testing.T) {
	// Regression test: upstream relays sometimes send a numeric error.code
	// (e.g. raw HTTP status) instead of the string the OpenAI schema
	// specifies, which used to break json.Unmarshal for the entire response.
	raw := `{"message":"insufficient balance","type":"insufficient_quota","code":403}`
	var e OpenAIAPIError
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("unexpected error unmarshaling numeric code: %v", err)
	}
	if e.Code != "403" {
		t.Errorf("got code %q, want %q", e.Code, "403")
	}

	rawStr := `{"message":"bad request","type":"invalid_request_error","code":"invalid_api_key"}`
	var e2 OpenAIAPIError
	if err := json.Unmarshal([]byte(rawStr), &e2); err != nil {
		t.Fatalf("unexpected error unmarshaling string code: %v", err)
	}
	if e2.Code != "invalid_api_key" {
		t.Errorf("got code %q, want %q", e2.Code, "invalid_api_key")
	}
}
