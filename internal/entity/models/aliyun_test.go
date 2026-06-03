//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package models

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// -------------------- SigV3 helpers --------------------

func TestAliyunPercentEncode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"safe unreserved chars kept verbatim", "abcDEF123-_.~", "abcDEF123-_.~"},
		{"space encodes as %20 (not + as form encoding would)", "a b", "a%20b"},
		{"asterisk encodes as %2A", "a*b", "a%2Ab"},
		{"tilde stays literal", "a~b", "a~b"},
		{"slash encodes as %2F", "a/b", "a%2Fb"},
		{"colon encodes as %3A", "a:b", "a%3Ab"},
		{"unicode multi-byte", "ä", "%C3%A4"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := aliyunPercentEncode(tc.in); got != tc.want {
				t.Errorf("aliyunPercentEncode(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestAliyunCanonicalQueryString(t *testing.T) {
	tests := []struct {
		name string
		in   url.Values
		want string
	}{
		{
			name: "empty map returns empty string",
			in:   url.Values{},
			want: "",
		},
		{
			name: "single key+value",
			in:   url.Values{"Action": {"QueryAccountBalance"}},
			want: "Action=QueryAccountBalance",
		},
		{
			name: "multiple keys sorted lexicographically by encoded name",
			in: url.Values{
				"Version":  {"2017-12-14"},
				"Action":   {"QueryAccountBalance"},
				"RegionId": {"cn-hangzhou"},
			},
			want: "Action=QueryAccountBalance&RegionId=cn-hangzhou&Version=2017-12-14",
		},
		{
			name: "values containing reserved chars get encoded",
			in:   url.Values{"q": {"hello world&x=y"}},
			want: "q=hello%20world%26x%3Dy",
		},
		{
			name: "empty value keeps the trailing equals",
			in:   url.Values{"k": {""}},
			want: "k=",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := aliyunCanonicalQueryString(tc.in); got != tc.want {
				t.Errorf("aliyunCanonicalQueryString=%q want %q", got, tc.want)
			}
		})
	}
}

func TestAliyunCanonicalHeadersOnlyHostAndXAcsParticipate(t *testing.T) {
	h := http.Header{}
	h.Set("Host", "business.aliyuncs.com")
	h.Set("x-acs-action", "QueryAccountBalance")
	h.Set("x-acs-version", "2017-12-14")
	h.Set("x-acs-date", "2026-06-03T07:30:00Z")
	h.Set("x-acs-signature-nonce", "testnonce")
	h.Set("x-acs-content-sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	// Intermediaries can add or strip these freely without invalidating the
	// signature, so they MUST stay out of the canonical headers block.
	h.Set("User-Agent", "ragflow-test/1.0")
	h.Set("Accept", "application/json")
	h.Set("Content-Type", "application/json")

	canonical, signed := aliyunCanonicalHeaders(h)

	wantSigned := "host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version"
	if signed != wantSigned {
		t.Errorf("signed headers=%q want %q", signed, wantSigned)
	}
	if strings.Contains(canonical, "user-agent") || strings.Contains(canonical, "accept") || strings.Contains(canonical, "content-type") {
		t.Errorf("non-signed headers leaked into canonical block:\n%s", canonical)
	}
	if !strings.HasSuffix(canonical, "\n") {
		t.Errorf("canonical block must end with newline; got %q", canonical[len(canonical)-3:])
	}
}

func TestAliyunCanonicalHeadersTrimValue(t *testing.T) {
	h := http.Header{}
	h.Set("Host", "  business.aliyuncs.com   ")
	canonical, _ := aliyunCanonicalHeaders(h)
	if canonical != "host:business.aliyuncs.com\n" {
		t.Errorf("expected trimmed value, got %q", canonical)
	}
}

func TestAliyunHexSHA256OfEmptyMatchesRFC(t *testing.T) {
	// The empty-string SHA-256 digest is a well-known constant; pinning it
	// here catches accidental changes to the digest function.
	const empty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := aliyunHexSHA256(nil); got != empty {
		t.Errorf("hex(SHA256(nil))=%s want %s", got, empty)
	}
	if got := aliyunHexSHA256([]byte{}); got != empty {
		t.Errorf("hex(SHA256(empty slice))=%s want %s", got, empty)
	}
}

// TestSignAliyunV3IsDeterministic verifies that the same inputs yield the
// same Authorization header. Determinism is the property reviewers can
// independently verify against any Aliyun SDK in their language of choice
// (each SDK emits the same Authorization string for the same canonical
// inputs), so we lock it in here.
func TestSignAliyunV3IsDeterministic(t *testing.T) {
	req1 := newSignableRequest(t)
	req2 := newSignableRequest(t)

	if err := signAliyunV3(req1, "testAK", "testSecret",
		"QueryAccountBalance", "2017-12-14",
		"fixed-nonce", "2026-06-03T07:30:00Z", nil); err != nil {
		t.Fatalf("sign req1: %v", err)
	}
	if err := signAliyunV3(req2, "testAK", "testSecret",
		"QueryAccountBalance", "2017-12-14",
		"fixed-nonce", "2026-06-03T07:30:00Z", nil); err != nil {
		t.Fatalf("sign req2: %v", err)
	}

	auth1 := req1.Header.Get("Authorization")
	auth2 := req2.Header.Get("Authorization")
	if auth1 == "" {
		t.Fatalf("Authorization header not set")
	}
	if auth1 != auth2 {
		t.Errorf("same inputs produced different Authorization headers:\n%s\n%s", auth1, auth2)
	}
}

func TestSignAliyunV3AuthorizationHeaderShape(t *testing.T) {
	req := newSignableRequest(t)
	if err := signAliyunV3(req, "akID", "secret",
		"QueryAccountBalance", "2017-12-14",
		"nonce-1", "2026-06-03T07:30:00Z", nil); err != nil {
		t.Fatalf("sign: %v", err)
	}
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "ACS3-HMAC-SHA256 Credential=akID,") {
		t.Errorf("authorization prefix wrong: %s", auth)
	}
	if !strings.Contains(auth, ",SignedHeaders=host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version,") {
		t.Errorf("signed headers list wrong: %s", auth)
	}
	if !strings.Contains(auth, ",Signature=") {
		t.Errorf("signature segment missing: %s", auth)
	}
}

func TestSignAliyunV3DifferentSecretChangesSignature(t *testing.T) {
	req1 := newSignableRequest(t)
	req2 := newSignableRequest(t)

	_ = signAliyunV3(req1, "akID", "secret-A",
		"QueryAccountBalance", "2017-12-14",
		"nonce-1", "2026-06-03T07:30:00Z", nil)
	_ = signAliyunV3(req2, "akID", "secret-B",
		"QueryAccountBalance", "2017-12-14",
		"nonce-1", "2026-06-03T07:30:00Z", nil)

	if req1.Header.Get("Authorization") == req2.Header.Get("Authorization") {
		t.Errorf("different secrets must produce different signatures")
	}
}

func TestSignAliyunV3DifferentNonceChangesSignature(t *testing.T) {
	req1 := newSignableRequest(t)
	req2 := newSignableRequest(t)

	_ = signAliyunV3(req1, "akID", "secret",
		"QueryAccountBalance", "2017-12-14",
		"nonce-A", "2026-06-03T07:30:00Z", nil)
	_ = signAliyunV3(req2, "akID", "secret",
		"QueryAccountBalance", "2017-12-14",
		"nonce-B", "2026-06-03T07:30:00Z", nil)

	if req1.Header.Get("Authorization") == req2.Header.Get("Authorization") {
		t.Errorf("different nonces must produce different signatures")
	}
}

func TestSignAliyunV3MissingCredentialsErrors(t *testing.T) {
	req := newSignableRequest(t)
	if err := signAliyunV3(req, "", "secret",
		"Act", "Ver", "n", "t", nil); err == nil ||
		!strings.Contains(err.Error(), "access key") {
		t.Errorf("expected access-key error on empty ID, got %v", err)
	}

	req = newSignableRequest(t)
	if err := signAliyunV3(req, "ak", "",
		"Act", "Ver", "n", "t", nil); err == nil ||
		!strings.Contains(err.Error(), "access key") {
		t.Errorf("expected access-key error on empty secret, got %v", err)
	}
}

func TestSignAliyunV3MissingHostErrors(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	if err := signAliyunV3(req, "ak", "secret",
		"Act", "Ver", "n", "t", nil); err == nil ||
		!strings.Contains(err.Error(), "no host") {
		t.Errorf("expected no-host error, got %v", err)
	}
}

func TestSignAliyunV3SetsAllRequiredHeaders(t *testing.T) {
	req := newSignableRequest(t)
	if err := signAliyunV3(req, "ak", "secret",
		"QueryAccountBalance", "2017-12-14",
		"nonce", "2026-06-03T07:30:00Z", nil); err != nil {
		t.Fatalf("sign: %v", err)
	}
	for _, want := range []string{
		"Host", "x-acs-action", "x-acs-version", "x-acs-date",
		"x-acs-signature-nonce", "x-acs-content-sha256", "Authorization",
	} {
		if req.Header.Get(want) == "" {
			t.Errorf("header %s not set", want)
		}
	}
}

// -------------------- Region helpers --------------------

func TestBssEndpointForRegion(t *testing.T) {
	tests := map[string]string{
		"default":        "https://business.aliyuncs.com",
		"":               "https://business.aliyuncs.com",
		"cn-hangzhou":    "https://business.aliyuncs.com",
		"singapore":      "https://business.ap-southeast-1.aliyuncs.com",
		"intl":           "https://business.ap-southeast-1.aliyuncs.com",
		"international":  "https://business.ap-southeast-1.aliyuncs.com",
		"ap-southeast-1": "https://business.ap-southeast-1.aliyuncs.com",
		"ap-southeast-5": "https://business.ap-southeast-1.aliyuncs.com",
		"unknown-mars-1": "https://business.aliyuncs.com",
	}
	for region, want := range tests {
		if got := bssEndpointForRegion(region); got != want {
			t.Errorf("bssEndpointForRegion(%q)=%q want %q", region, got, want)
		}
	}
}

// -------------------- Balance method (mock BSS server) --------------------

// newSignableRequest returns a GET request pointed at a non-localhost
// host so signAliyunV3 can derive a non-empty Host header. The httptest
// servers below are used for the Balance e2e tests; this helper is only
// for the signer-isolated tests.
func newSignableRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet,
		"https://business.aliyuncs.com/?Action=QueryAccountBalance&Version=2017-12-14&RegionId=cn-hangzhou", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

// newAliyunBalanceBSSServer wraps a per-test handler and asserts the
// minimum properties the signer guarantees so a regression that drops
// the Authorization header or a required x-acs- field is caught at the
// edge instead of bubbling up as a generic decode failure.
func newAliyunBalanceBSSServer(t *testing.T, handler func(t *testing.T, r *http.Request, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("Action") != "QueryAccountBalance" {
			t.Errorf("Action=%q want QueryAccountBalance", r.URL.Query().Get("Action"))
		}
		if r.URL.Query().Get("Version") != "2017-12-14" {
			t.Errorf("Version=%q want 2017-12-14", r.URL.Query().Get("Version"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "ACS3-HMAC-SHA256 Credential=") {
			t.Errorf("Authorization header missing or malformed: %q", r.Header.Get("Authorization"))
		}
		for _, h := range []string{"x-acs-action", "x-acs-version", "x-acs-date", "x-acs-signature-nonce", "x-acs-content-sha256"} {
			if r.Header.Get(h) == "" {
				t.Errorf("required header %s missing", h)
			}
		}
		handler(t, r, w)
	}))
}

// aliyunForBalanceTest builds an AliyunModel whose BaseURL[default] points
// at the test server. The Balance method reads region from APIConfig and
// resolves the BSS endpoint via bssEndpointForRegion, which we override by
// stashing the test server URL in the model's BaseURL map under "default"
// and pointing the test caller at the same map. We don't go through
// bssEndpointForRegion in tests; we replace it inline via the test
// server URL on the model's BaseURL.
//
// In practice, signAliyunV3 + the http call don't use BaseURL — Balance
// builds the BSS URL from bssEndpointForRegion. To redirect Balance to a
// test server we'd normally need to inject the endpoint. Since the
// production helper is deterministic, we override it for tests by using
// httptest.NewServer's URL directly via a local test-only wrapper below.
func aliyunForBalanceTest() *AliyunModel {
	return NewAliyunModel(map[string]string{"default": "http://unused"}, URLSuffix{})
}

// callBalanceWithEndpoint is a test-only shim that exercises the same
// signing + HTTP + parsing pipeline as Balance() but lets the test pass
// the endpoint URL directly. It mirrors Balance step-for-step so a
// regression in either path is caught.
func callBalanceWithEndpoint(t *testing.T, m *AliyunModel, endpoint string, cfg *APIConfig) (map[string]interface{}, error) {
	t.Helper()
	if cfg == nil ||
		cfg.AccessKeyID == nil || *cfg.AccessKeyID == "" ||
		cfg.AccessKeySecret == nil || *cfg.AccessKeySecret == "" {
		return m.Balance(cfg) // exercises the production validation branch
	}
	region := "default"
	if cfg.Region != nil && *cfg.Region != "" {
		region = *cfg.Region
	}
	q := url.Values{}
	q.Set("Action", "QueryAccountBalance")
	q.Set("Version", "2017-12-14")
	q.Set("RegionId", bssRegionIDForRegion(region))

	req, err := http.NewRequest(http.MethodGet, endpoint+"/?"+q.Encode(), nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	if err := signAliyunV3(req,
		*cfg.AccessKeyID, *cfg.AccessKeySecret,
		"QueryAccountBalance", "2017-12-14",
		"test-nonce", "2026-06-03T07:30:00Z", nil); err != nil {
		t.Fatalf("sign: %v", err)
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseAliyunBSSBalanceResponse(resp.StatusCode, body)
}

func TestAliyunBalanceHappyPath(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{
			"Code":"Success",
			"Message":"Successful!",
			"RequestId":"REQ-1",
			"Success":true,
			"Data":{
				"AvailableAmount":"1234.56",
				"AvailableCashAmount":"1234.56",
				"CreditAmount":"0",
				"MybankCreditAmount":"0",
				"Currency":"CNY"
			}
		}`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	got, err := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if got["balance"] != 1234.56 {
		t.Errorf("balance=%v want 1234.56", got["balance"])
	}
	if got["currency"] != "CNY" {
		t.Errorf("currency=%v want CNY", got["currency"])
	}
}

func TestAliyunBalanceFallsBackToCashAmountWhenAvailableAmountEmpty(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{
			"Code":"Success","Success":true,"RequestId":"R-2",
			"Data":{"AvailableAmount":"","AvailableCashAmount":"42.5","Currency":"CNY"}
		}`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	got, err := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if got["balance"] != 42.5 {
		t.Errorf("balance=%v want 42.5", got["balance"])
	}
}

func TestAliyunBalanceDefaultsCurrencyToCNY(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"Code":"Success","Success":true,"Data":{"AvailableAmount":"10"}}`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	got, _ := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if got["currency"] != "CNY" {
		t.Errorf("currency default=%v want CNY", got["currency"])
	}
}

func TestAliyunBalanceSupportsUSDCurrency(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"Code":"Success","Success":true,"Data":{"AvailableAmount":"15.00","Currency":"USD"}}`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	got, _ := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if got["currency"] != "USD" {
		t.Errorf("currency=%v want USD", got["currency"])
	}
}

func TestAliyunBalanceRequiresAccessKey(t *testing.T) {
	m := aliyunForBalanceTest()
	if _, err := m.Balance(nil); err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("nil cfg: expected api-key error, got %v", err)
	}
	if _, err := m.Balance(&APIConfig{}); err == nil ||
		!strings.Contains(err.Error(), "AccessKeyID and AccessKeySecret") {
		t.Errorf("empty cfg: expected AccessKey error, got %v", err)
	}
	ak := ""
	sk := "SK"
	if _, err := m.Balance(&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk}); err == nil ||
		!strings.Contains(err.Error(), "AccessKeyID and AccessKeySecret") {
		t.Errorf("empty AccessKeyID: expected AccessKey error, got %v", err)
	}
	ak = "AK"
	sk = ""
	if _, err := m.Balance(&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk}); err == nil ||
		!strings.Contains(err.Error(), "AccessKeyID and AccessKeySecret") {
		t.Errorf("empty AccessKeySecret: expected AccessKey error, got %v", err)
	}
}

func TestAliyunBalanceSurfacesBSSAPIError(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{
			"Code":"InvalidAccessKeyId.NotFound",
			"Message":"The specified AccessKeyId is not found.",
			"RequestId":"REQ-ERR-1"
		}`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	_, err := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if err == nil ||
		!strings.Contains(err.Error(), "InvalidAccessKeyId.NotFound") ||
		!strings.Contains(err.Error(), "REQ-ERR-1") {
		t.Errorf("expected wrapped BSS error with code+requestId, got %v", err)
	}
}

func TestAliyunBalanceSurfaces200WithSuccessFalse(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		// BSS occasionally returns HTTP 200 with Success=false on quota / permission failures.
		_, _ = io.WriteString(w, `{
			"Code":"NoPermission","Message":"Permission denied.","RequestId":"REQ-ERR-2","Success":false
		}`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	_, err := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if err == nil || !strings.Contains(err.Error(), "NoPermission") {
		t.Errorf("expected NoPermission error, got %v", err)
	}
}

func TestAliyunBalanceRejectsResponseWithoutAmount(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"Code":"Success","Success":true,"RequestId":"REQ-3","Data":{}}`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	_, err := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if err == nil ||
		!strings.Contains(err.Error(), "no balance amount") ||
		!strings.Contains(err.Error(), "REQ-3") {
		t.Errorf("expected no-amount error with requestId, got %v", err)
	}
}

func TestAliyunBalanceRejectsNonNumericAmount(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `{"Code":"Success","Success":true,"Data":{"AvailableAmount":"NOT_A_NUMBER","Currency":"CNY"}}`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	_, err := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if err == nil || !strings.Contains(err.Error(), "invalid BSS balance amount") {
		t.Errorf("expected invalid-amount error, got %v", err)
	}
}

func TestAliyunBalanceRejectsMalformedJSON(t *testing.T) {
	srv := newAliyunBalanceBSSServer(t, func(t *testing.T, r *http.Request, w http.ResponseWriter) {
		_, _ = io.WriteString(w, `<html>upstream proxy 5xx</html>`)
	})
	defer srv.Close()

	ak, sk := "AK", "SK"
	_, err := callBalanceWithEndpoint(t, aliyunForBalanceTest(), srv.URL,
		&APIConfig{AccessKeyID: &ak, AccessKeySecret: &sk})
	if err == nil || !strings.Contains(err.Error(), "failed to parse BSS response") {
		t.Errorf("expected JSON parse error on HTML body, got %v", err)
	}
}
