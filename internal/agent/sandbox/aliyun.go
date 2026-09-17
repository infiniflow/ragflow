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

// Aliyun uses the installed Go SDK for sandbox lifecycle and the
// Python SDK's AgentRun RAM signature for the data-plane execute endpoint.

package sandbox

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"ragflow/internal/common"
	"strings"
	"time"

	"github.com/alibabacloud-go/agentrun-20250910/v5/client"
	agentrun "github.com/alibabacloud-go/agentrun-20250910/v5/client"
	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	"github.com/alibabacloud-go/tea/tea"
)

// aliyunDefaultRegion is the canonical region baked into the Python
// side. Operators override via AGENTRUN_REGION.
const aliyunDefaultRegion = "cn-hangzhou"

const aliyunExecutePath = "/sandboxes/%s/contexts/execute"

// AliyunCodeInterpreterProvider is the Go port of
// `agent/sandbox/providers/aliyun_codeinterpreter.py::AliyunCodeInterpreterProvider`.
type AliyunCodeInterpreterProvider struct {
	accessKeyID     string
	accessKeySecret string
	accountID       string
	region          string
	templateName    string
	timeout         int // seconds, hard cap 30
	executeHost     string

	sdk    *client.Client
	helper *HTTPClient

	initialized bool
}

// newAliyunProviderFromEnv reads AGENTRUN_* env vars and returns a
// provider ready for Initialize. We do NOT call Initialize here —
// the manager does it on first use so env changes are picked up.
func newAliyunProviderFromEnv() *AliyunCodeInterpreterProvider {
	return newAliyunProviderFromConfig(aliyunConfigFromEnv())
}

// aliyunConfigFromEnv builds a config map from the AGENTRUN_*
// env vars, mirroring the admin-panel settings JSON shape.
func aliyunConfigFromEnv() map[string]any {
	return map[string]any{
		"access_key_id":     common.GetEnv(common.EnvAgentRunAccessKeyID),
		"access_key_secret": common.GetEnv(common.EnvAgentRunAccessKeySecret),
		"account_id":        common.GetEnv(common.EnvAgentRunAccountID),
		"region":            common.GetEnv(common.EnvAgentRunRegion),
		"template_name":     common.GetEnv(common.EnvAgentRunTemplateName),
		"execute_host":      common.GetEnv(common.EnvAgentRunExecuteHost),
		"timeout":           common.GetEnv(common.EnvAgentRunTimeout),
	}
}

// newAliyunProviderFromConfig builds the provider from a JSON
// config map (as stored in the system_settings table for the
// aliyun_codeinterpreter provider). Config keys use the lowercase
// Python schema names.
func newAliyunProviderFromConfig(cfg map[string]any) *AliyunCodeInterpreterProvider {
	p := &AliyunCodeInterpreterProvider{
		accessKeyID:     configString(cfg, "access_key_id"),
		accessKeySecret: configString(cfg, "access_key_secret"),
		accountID:       configString(cfg, "account_id"),
		region:          configString(cfg, "region"),
		templateName:    configString(cfg, "template_name"),
		executeHost:     configString(cfg, "execute_host"),
	}
	if p.region == "" {
		p.region = aliyunDefaultRegion
	}
	p.timeout = configInt(cfg, "timeout", 30)
	// Hard cap matches the Python side.
	if p.timeout > 30 {
		p.timeout = 30
	}
	return p
}

// ProviderType returns ProviderAliyun.
func (p *AliyunCodeInterpreterProvider) ProviderType() ProviderType {
	return ProviderAliyun
}

// Initialize constructs the signed control-plane client without creating resources.
func (p *AliyunCodeInterpreterProvider) Initialize(ctx context.Context) error {
	if p.accessKeyID == "" || p.accessKeySecret == "" {
		return errors.New("aliyun: AGENTRUN_ACCESS_KEY_ID and AGENTRUN_ACCESS_KEY_SECRET are required")
	}
	if p.accountID == "" {
		return errors.New("aliyun: AGENTRUN_ACCOUNT_ID is required")
	}

	endpoint, protocol := aliyunControlEndpoint(p.executeHost)
	if endpoint == nil {
		endpoint = stringPtr("agentrun." + p.region + ".aliyuncs.com")
	}
	requestTimeout := p.timeout * 1000
	cfg := &openapiutil.Config{
		AccessKeyId:     &p.accessKeyID,
		AccessKeySecret: &p.accessKeySecret,
		Type:            stringPtr("access_key"),
		RegionId:        &p.region,
		Endpoint:        endpoint,
		Protocol:        protocol,
		ReadTimeout:     &requestTimeout,
		ConnectTimeout:  &requestTimeout,
	}
	ua := "ragflow-go-agent"
	cfg.UserAgent = &ua

	sdk, err := agentrun.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("aliyun: build agentrun client: %w", err)
	}
	p.sdk = sdk
	p.helper = NewHTTPClient(HTTPConfig{
		Timeout:     time.Duration(p.timeout) * time.Second,
		MaxAttempts: 1,
	})
	p.initialized = true
	return nil
}

// SupportedLanguages mirrors the Python provider.
func (p *AliyunCodeInterpreterProvider) SupportedLanguages() []string {
	return []string{"python", "javascript"}
}

// CreateInstance creates a sandbox from the configured or default template.
func (p *AliyunCodeInterpreterProvider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.initialized {
		return nil, fmt.Errorf("aliyun: provider not initialized")
	}
	lang := normalizeLanguage(template)
	if lang == "" {
		return nil, fmt.Errorf("aliyun: unsupported language %q", template)
	}

	templateName := p.templateName
	if templateName == "" {
		templateName = fmt.Sprintf("ragflow-%s-default", lang)
		if _, err := p.sdk.GetTemplate(&templateName); err != nil {
			var sdkErr *tea.SDKError
			if !errors.As(err, &sdkErr) || sdkErr.StatusCode == nil || *sdkErr.StatusCode != http.StatusNotFound {
				return nil, fmt.Errorf("aliyun: GetTemplate: %w", err)
			}
			input := &client.CreateTemplateInput{
				TemplateName: &templateName,
				TemplateType: stringPtr("CodeInterpreter"),
			}
			if _, createErr := p.sdk.CreateTemplate(&client.CreateTemplateRequest{Body: input}); createErr != nil {
				return nil, fmt.Errorf("aliyun: CreateTemplate(%s): %w", templateName, createErr)
			}
		}
	}

	timeout := int32(p.timeout)
	input := &client.CreateSandboxInput{
		TemplateName:                &templateName,
		SandboxIdleTimeoutInSeconds: &timeout,
	}
	resp, err := p.sdk.CreateSandbox(&client.CreateSandboxRequest{Body: input})
	if err != nil {
		return nil, fmt.Errorf("aliyun: CreateSandbox: %w", err)
	}
	if resp == nil || resp.Body == nil || resp.Body.Data == nil || resp.Body.Data.SandboxId == nil {
		return nil, fmt.Errorf("aliyun: CreateSandbox returned empty response")
	}
	id := *resp.Body.Data.SandboxId

	return &SandboxInstance{
		InstanceID: id,
		Provider:   ProviderAliyun,
		Status:     derefString(resp.Body.Data.Status),
		Metadata: map[string]any{
			"language":      lang,
			"region":        p.region,
			"account_id":    p.accountID,
			"template_name": templateName,
			"created_at":    derefString(resp.Body.Data.CreatedAt),
		},
	}, nil
}

// ExecuteCode hits the agentrun REST execute endpoint via raw HTTP
// (SDK gap). The payload mirrors the Python SDK's
// `SandboxContext.execute(code, language, timeout)` call shape.
func (p *AliyunCodeInterpreterProvider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.initialized {
		return nil, fmt.Errorf("aliyun: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return nil, fmt.Errorf("aliyun: instance id required")
	}
	lang := normalizeLanguage(language)
	if lang == "" {
		return nil, fmt.Errorf("aliyun: unsupported language %q", language)
	}

	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	if timeout == 0 {
		timeout = p.timeout
	}
	// 30s hard cap, matches the Python side.
	if timeout > 30 {
		timeout = 30
	}

	// Wrap the code in the result-protocol driver.
	argsJSON, err := argsToJSON(args)
	if err != nil {
		return nil, err
	}
	var wrapped string
	if lang == "python" {
		wrapped = BuildPythonWrapper(code, argsJSON)
	} else {
		wrapped = BuildJavaScriptWrapper(code, argsJSON)
	}

	start := time.Now()
	respBody, err := p.callExecute(ctx, inst.InstanceID, wrapped, lang, timeout)
	if err != nil {
		return nil, err
	}

	// Parse the agentrun execute response. Shape (from
	// `agentrun.sandbox.SandboxContext.execute`):
	//
	//   {
	//     "results": [
	//       {"type": "stdout",    "text": "..."},
	//       {"type": "stderr",    "text": "..."},
	//       {"type": "error",     "text": "..."},
	//       {"type": "endOfExecution", "status": "ok"}
	//     ],
	//     "contextId": "..."
	//   }
	var parsed struct {
		Results []struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"results"`
		ContextID string `json:"contextId"`
	}
	if err = json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("aliyun: decode execute response: %w", err)
	}

	var stdoutParts, stderrParts []string
	exitCode := 0
	for _, item := range parsed.Results {
		switch item.Type {
		case "stdout":
			stdoutParts = append(stdoutParts, item.Text)
		case "stderr":
			stderrParts = append(stderrParts, item.Text)
			exitCode = 1
		case "error":
			stderrParts = append(stderrParts, item.Text)
			exitCode = 1
		case "endOfExecution":
			if item.Status != "" && item.Status != "ok" {
				exitCode = 1
			}
		}
	}
	stdout := strings.Join(stdoutParts, "\n")
	stderr := strings.Join(stderrParts, "\n")

	// Strip the `__RAGFLOW_RESULT__:` marker from stdout, surface
	// the user's main() return value as a structured result.
	cleaned, structured := ExtractStructuredResult(stdout)

	return &ExecutionResult{
		Stdout:        cleaned,
		Stderr:        stderr,
		ExitCode:      exitCode,
		ExecutionTime: time.Since(start).Seconds(),
		Metadata: map[string]any{
			"instance_id":       inst.InstanceID,
			"language":          lang,
			"context_id":        parsed.ContextID,
			"timeout":           timeout,
			"structured_result": structured,
		},
	}, nil
}

func (p *AliyunCodeInterpreterProvider) callExecute(ctx context.Context, sandboxID, code, language string, timeoutSec int) ([]byte, error) {
	endpoint := p.executeHost
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.agentrun-data.%s.aliyuncs.com", p.accountID, p.region)
	} else if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("aliyun: parse execute endpoint: %w", err)
	}
	u.Path = fmt.Sprintf(aliyunExecutePath, sandboxID)
	u.RawQuery = ""
	body, err := json.Marshal(map[string]any{"code": code, "language": language, "timeout": timeoutSec})
	if err != nil {
		return nil, err
	}
	resp, err := p.helper.Do(ctx, http.MethodPost, u.String(), string(body), "application/json", p.aliyunSignedHeaders(u, time.Now().UTC()))
	if err != nil {
		return nil, fmt.Errorf("aliyun: execute request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aliyun: execute returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// aliyunSignedHeaders matches agentrun-sdk's AGENTRUN4-HMAC-SHA256 signer.
// Only the fixed execute endpoint is signed; it has no query parameters.
func (p *AliyunCodeInterpreterProvider) aliyunSignedHeaders(u *url.URL, now time.Time) map[string]string {
	timestamp, date := now.UTC().Format("2006-01-02T15:04:05Z"), now.UTC().Format("20060102")
	const signed = "content-type;host;x-acs-content-sha256;x-acs-date"
	const algorithm = "AGENTRUN4-HMAC-SHA256"
	canonicalHeaders := "content-type:application/json\nhost:" + u.Host + "\nx-acs-content-sha256:UNSIGNED-PAYLOAD\nx-acs-date:" + timestamp + "\n"
	canonical := "POST\n" + u.Path + "\n\n" + canonicalHeaders + "\n" + signed + "\nUNSIGNED-PAYLOAD"
	digest := sha256.Sum256([]byte(canonical))
	key := []byte("aliyun_v4" + p.accessKeySecret)
	for _, part := range []string{date, p.region, "agentrun", "aliyun_v4_request", algorithm + "\n" + hex.EncodeToString(digest[:])} {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(part))
		key = mac.Sum(nil)
	}
	scope := date + "/" + p.region + "/agentrun/aliyun_v4_request"
	return map[string]string{
		"x-acs-date":             timestamp,
		"x-acs-content-sha256":   "UNSIGNED-PAYLOAD",
		"Agentrun-Authorization": algorithm + " Credential=" + p.accessKeyID + "/" + scope + ",SignedHeaders=" + signed + ",Signature=" + hex.EncodeToString(key),
	}
}

func aliyunControlEndpoint(endpoint string) (*string, *string) {
	if endpoint == "" {
		return nil, nil
	}
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return &u.Host, &u.Scheme
	}
	return &endpoint, nil
}

// DestroyInstance calls DeleteSandbox via the SDK.
func (p *AliyunCodeInterpreterProvider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.initialized {
		return fmt.Errorf("aliyun: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return fmt.Errorf("aliyun: instance id required")
	}
	id := inst.InstanceID
	if _, err := p.sdk.DeleteSandbox(&id); err != nil {
		return fmt.Errorf("aliyun: DeleteSandbox(%s): %w", id, err)
	}
	return nil
}

// HealthCheck verifies access to the template catalog.
func (p *AliyunCodeInterpreterProvider) HealthCheck(ctx context.Context) error {
	if !p.initialized {
		return errors.New("aliyun: provider not initialized")
	}
	_, err := p.sdk.ListTemplates(&client.ListTemplatesRequest{})
	return err
}

func stringPtr(s string) *string { return &s }

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
