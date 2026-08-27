package service

import "testing"

func TestParseTenantModelInstanceExtraNormalizesLegacyValues(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		region string
		base   string
	}{
		{name: "empty", raw: "", region: "default"},
		{name: "legacy status", raw: "active", region: "default"},
		{name: "quoted legacy status", raw: `"active"`, region: "default"},
		{name: "empty object", raw: `{}`, region: "default"},
		{name: "missing region", raw: `{"base_url":"https://example.test"}`, region: "default", base: "https://example.test"},
		{name: "trim region", raw: `{"region":" us-east-1 "}`, region: "us-east-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extra, err := parseTenantModelInstanceExtra(test.raw)
			if err != nil {
				t.Fatalf("parseTenantModelInstanceExtra() error = %v", err)
			}
			if got := tenantModelInstanceExtraString(extra, "region"); got != test.region {
				t.Fatalf("region = %q, want %q", got, test.region)
			}
			if got := tenantModelInstanceExtraString(extra, "base_url"); got != test.base {
				t.Fatalf("base_url = %q, want %q", got, test.base)
			}
		})
	}
}

func TestParseTenantModelInstanceExtraRejectsMalformedJSON(t *testing.T) {
	if _, err := parseTenantModelInstanceExtra(`{"region":`); err == nil {
		t.Fatal("parseTenantModelInstanceExtra() error = nil, want malformed JSON error")
	}
}
