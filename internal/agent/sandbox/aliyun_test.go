package sandbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAliyun_ConfigUsesCanonicalLowercaseKeys(t *testing.T) {
	p := newAliyunProviderFromConfig(map[string]any{
		"access_key_id":     "LTAI-test",
		"access_key_secret": "secret",
		"account_id":        "account",
		"region":            "cn-shanghai",
		"template_name":     "template",
		"execute_host":      "https://example.test",
		"timeout":           12,
	})
	if p.accessKeyID != "LTAI-test" || p.accessKeySecret != "secret" || p.accountID != "account" {
		t.Fatalf("lowercase credentials not applied: %#v", p)
	}
	if p.region != "cn-shanghai" || p.templateName != "template" || p.executeHost != "https://example.test" || p.timeout != 12 {
		t.Fatalf("lowercase options not applied: %#v", p)
	}
	legacy := newAliyunProviderFromConfig(map[string]any{"ACCESS_KEY_ID": "legacy", "REGION": "cn-beijing"})
	if legacy.accessKeyID != "" || legacy.region != aliyunDefaultRegion {
		t.Fatalf("uppercase configuration unexpectedly accepted: %#v", legacy)
	}
}

func TestAliyun_EnvUsesCanonicalLowercaseKeys(t *testing.T) {
	t.Setenv("AGENTRUN_ACCESS_KEY_ID", "LTAI-env")
	t.Setenv("AGENTRUN_ACCESS_KEY_SECRET", "secret")
	t.Setenv("AGENTRUN_ACCOUNT_ID", "account")
	t.Setenv("AGENTRUN_REGION", "cn-beijing")
	p := newAliyunProviderFromEnv()
	if p.accessKeyID != "LTAI-env" || p.region != "cn-beijing" {
		t.Fatalf("environment configuration not applied: %#v", p)
	}
}

func TestAliyunSignedHeadersMatchPythonSDKVector(t *testing.T) {
	p := newAliyunProviderFromConfig(map[string]any{
		"access_key_id":     "LTAI-test",
		"access_key_secret": "test-secret",
		"account_id":        "account",
		"region":            "cn-hangzhou",
	})
	u, err := url.Parse("https://account.agentrun-data.cn-hangzhou.aliyuncs.com/sandboxes/sbx-test/contexts/execute")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)

	headers := p.aliyunSignedHeaders(u, now)
	want := "AGENTRUN4-HMAC-SHA256 Credential=LTAI-test/20260917/cn-hangzhou/agentrun/aliyun_v4_request,SignedHeaders=content-type;host;x-acs-content-sha256;x-acs-date,Signature=06d2e6526b6611609fcc76039ad0bc6f6fc3a6979f9c3fb1d5640fee6fafd150"
	if headers["Agentrun-Authorization"] != want {
		t.Fatalf("Agentrun-Authorization = %q, want %q", headers["Agentrun-Authorization"], want)
	}
	if headers["x-acs-content-sha256"] != "UNSIGNED-PAYLOAD" || headers["x-acs-date"] != "2026-09-17T00:00:00Z" {
		t.Fatalf("unexpected signed headers: %#v", headers)
	}
}

func TestAliyunProtocolUsesSavedTemplateSignedExecuteAndDelete(t *testing.T) {
	// The Aliyun SDK proxies loopback requests too, and its NO_PROXY matching
	// requires the exact host:port. Keep this local protocol test off CI proxies.
	for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		t.Setenv(key, "")
	}
	var sawCreate, sawExecute, sawDelete bool
	var executeAuth string
	var executeBody struct {
		Code     string `json:"code"`
		Language string `json:"language"`
		Timeout  int    `json:"timeout"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/2025-09-10/sandboxes":
			sawCreate = true
			var body struct {
				TemplateName string `json:"templateName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create body: %v", err)
			}
			if body.TemplateName != "saved-template" {
				t.Errorf("templateName = %q, want saved-template", body.TemplateName)
			}
			_, _ = w.Write([]byte(`{"data":{"sandboxId":"sbx-test","status":"RUNNING","createdAt":"2026-09-17T00:00:00Z"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-test/contexts/execute":
			sawExecute = true
			executeAuth = r.Header.Get("Agentrun-Authorization")
			if err := json.NewDecoder(r.Body).Decode(&executeBody); err != nil {
				t.Errorf("decode execute body: %v", err)
			}
			_, _ = w.Write([]byte(`{"results":[{"type":"stdout","text":"TEST_PASSED\n"},{"type":"endOfExecution","status":"ok"}],"contextId":"ctx-test"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/2025-09-10/sandboxes/sbx-test":
			sawDelete = true
			_, _ = w.Write([]byte(`{"data":{"sandboxId":"sbx-test","status":"DELETED"}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := newAliyunProviderFromConfig(map[string]any{
		"access_key_id":     "LTAI-test",
		"access_key_secret": "test-secret",
		"account_id":        "account",
		"region":            "cn-hangzhou",
		"template_name":     "saved-template",
		"execute_host":      server.URL,
		"timeout":           12,
	})
	if err := p.Initialize(t.Context()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	inst, err := p.CreateInstance(t.Context(), "python")
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	result, err := p.ExecuteCode(t.Context(), inst, "def main():\n    print('TEST_PASSED')", "python", 7, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "TEST_PASSED") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if executeBody.Language != "python" || executeBody.Timeout != 7 || !strings.Contains(executeBody.Code, "def main") {
		t.Fatalf("unexpected execute body: %+v", executeBody)
	}
	if !strings.HasPrefix(executeAuth, "AGENTRUN4-HMAC-SHA256 Credential=LTAI-test/") ||
		!strings.Contains(executeAuth, "SignedHeaders=content-type;host;x-acs-content-sha256;x-acs-date") {
		t.Fatalf("execute auth header = %q", executeAuth)
	}
	if err := p.DestroyInstance(t.Context(), inst); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !sawCreate || !sawExecute || !sawDelete {
		t.Fatalf("requests seen create=%v execute=%v delete=%v", sawCreate, sawExecute, sawDelete)
	}
}
