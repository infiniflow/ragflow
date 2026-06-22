package sandbox

import (
	"context"
	"testing"

	agenttool "ragflow/internal/agent/tool"
)

type managerClientStubProvider struct{}

func (managerClientStubProvider) Initialize(context.Context) error { return nil }
func (managerClientStubProvider) ProviderType() ProviderType       { return ProviderLocal }
func (managerClientStubProvider) CreateInstance(context.Context, string) (*SandboxInstance, error) {
	return &SandboxInstance{InstanceID: "inst-1", Provider: ProviderLocal, Status: "running"}, nil
}
func (managerClientStubProvider) ExecuteCode(context.Context, *SandboxInstance, string, string, int, map[string]any) (*ExecutionResult, error) {
	return &ExecutionResult{
		Stdout:   "",
		Stderr:   "",
		ExitCode: 0,
		Metadata: map[string]any{
			"structured_result": map[string]any{
				"present":     true,
				"value":       16,
				"actual_type": "int",
			},
		},
	}, nil
}
func (managerClientStubProvider) DestroyInstance(context.Context, *SandboxInstance) error { return nil }
func (managerClientStubProvider) HealthCheck(context.Context) error                       { return nil }
func (managerClientStubProvider) SupportedLanguages() []string                            { return []string{"python"} }

func TestManagerClient_MapsStructuredResultToSandboxResponse(t *testing.T) {
	mgr := &ProviderManager{}
	mgr.SetProvider(managerClientStubProvider{})

	client := &ManagerClient{manager: mgr}
	resp, err := client.ExecuteCode(context.Background(), agenttool.SandboxRequest{
		Lang:   "python",
		Script: "def main(): return 16",
	})
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if resp.Returned != "16" {
		t.Fatalf("Returned=%q, want %q", resp.Returned, "16")
	}
	if got := resp.StructuredResult["actual_type"]; got != "int" {
		t.Fatalf("StructuredResult.actual_type=%v, want int", got)
	}
}

func TestManagerClient_MapsLegacyResultKeyToSandboxResponse(t *testing.T) {
	mgr := &ProviderManager{}
	mgr.SetProvider(managerClientResultKeyProvider{})

	client := &ManagerClient{manager: mgr}
	resp, err := client.ExecuteCode(context.Background(), agenttool.SandboxRequest{
		Lang:   "python",
		Script: "def main(): return 16",
	})
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if resp.Returned != "16" {
		t.Fatalf("Returned=%q, want %q", resp.Returned, "16")
	}
	if got := resp.StructuredResult["value"]; got != 16 {
		t.Fatalf("StructuredResult.value=%v, want 16", got)
	}
}

type managerClientResultKeyProvider struct{}

func (managerClientResultKeyProvider) Initialize(context.Context) error { return nil }
func (managerClientResultKeyProvider) ProviderType() ProviderType       { return ProviderLocal }
func (managerClientResultKeyProvider) CreateInstance(context.Context, string) (*SandboxInstance, error) {
	return &SandboxInstance{InstanceID: "inst-2", Provider: ProviderLocal, Status: "running"}, nil
}
func (managerClientResultKeyProvider) ExecuteCode(context.Context, *SandboxInstance, string, string, int, map[string]any) (*ExecutionResult, error) {
	return &ExecutionResult{
		Stdout:   "",
		Stderr:   "",
		ExitCode: 0,
		Metadata: map[string]any{
			"result": map[string]any{
				"present": true,
				"value":   16,
			},
		},
	}, nil
}
func (managerClientResultKeyProvider) DestroyInstance(context.Context, *SandboxInstance) error {
	return nil
}
func (managerClientResultKeyProvider) HealthCheck(context.Context) error { return nil }
func (managerClientResultKeyProvider) SupportedLanguages() []string      { return []string{"python"} }
