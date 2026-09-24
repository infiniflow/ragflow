package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
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
	ctx := t.Context()
	resp, err := client.ExecuteCode(ctx, agenttool.SandboxRequest{
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
	ctx := t.Context()
	resp, err := client.ExecuteCode(ctx, agenttool.SandboxRequest{
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

type managerClientFlattenedResultProvider struct{}

func (managerClientFlattenedResultProvider) Initialize(context.Context) error { return nil }
func (managerClientFlattenedResultProvider) ProviderType() ProviderType {
	return ProviderUCloudAgentSandbox
}
func (managerClientFlattenedResultProvider) CreateInstance(context.Context, string) (*SandboxInstance, error) {
	return &SandboxInstance{InstanceID: "inst-flat", Provider: ProviderUCloudAgentSandbox, Status: "running"}, nil
}
func (managerClientFlattenedResultProvider) ExecuteCode(context.Context, *SandboxInstance, string, string, int, map[string]any) (*ExecutionResult, error) {
	return &ExecutionResult{Metadata: map[string]any{
		"result_present": true,
		"result_value":   "ucloud result",
		"result_type":    "string",
	}}, nil
}
func (managerClientFlattenedResultProvider) DestroyInstance(context.Context, *SandboxInstance) error {
	return nil
}
func (managerClientFlattenedResultProvider) HealthCheck(context.Context) error { return nil }
func (managerClientFlattenedResultProvider) SupportedLanguages() []string      { return []string{"python"} }

func TestManagerClient_MapsFlattenedProviderResult(t *testing.T) {
	mgr := &ProviderManager{}
	mgr.SetProvider(managerClientFlattenedResultProvider{})

	resp, err := (&ManagerClient{manager: mgr}).ExecuteCode(t.Context(), agenttool.SandboxRequest{Lang: "python"})
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if resp.StructuredResult["value"] != "ucloud result" || resp.Returned != "ucloud result" {
		t.Fatalf("flattened result = %#v, Returned=%q", resp.StructuredResult, resp.Returned)
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

func TestManagerClient_RefreshDuringExecution(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	previousDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previousDB })
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/run" {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			handleRun(t, w, r, "old", "")
		}
	}))
	defer oldServer.Close()
	defer once.Do(func() { close(release) })
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/run" {
			handleRun(t, w, r, "new", "")
		}
	}))
	defer newServer.Close()
	d := dao.NewSystemSettingsDAO()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := d.SaveOrCreate(ctx, db, "sandbox.provider_type", "self_managed", "admin", "string"); err != nil {
		t.Fatal(err)
	}
	save := func(endpoint string) {
		t.Helper()
		cfg, _ := json.Marshal(map[string]any{"endpoint": endpoint})
		if err := d.SaveOrCreate(ctx, db, "sandbox.self_managed", string(cfg), "admin", "json"); err != nil {
			t.Fatal(err)
		}
	}
	save(oldServer.URL)
	client := &ManagerClient{manager: &ProviderManager{}}
	type outcome struct {
		response *agenttool.SandboxResponse
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		response, err := client.ExecuteCode(ctx, agenttool.SandboxRequest{Lang: "python", Script: "print('old')"})
		done <- outcome{response, err}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	save(":invalid")
	if _, err := client.ExecuteCode(ctx, agenttool.SandboxRequest{Lang: "python"}); err == nil {
		t.Fatal("new execution silently accepted failed replacement")
	}
	save(newServer.URL)
	response, err := client.ExecuteCode(ctx, agenttool.SandboxRequest{Lang: "python", Script: "print('new')"})
	if err != nil || response.Stdout != "new" {
		t.Fatalf("next execution = %+v, %v", response, err)
	}
	once.Do(func() { close(release) })
	select {
	case result := <-done:
		if result.err != nil || result.response.Stdout != "old" {
			t.Fatalf("in-flight execution = %+v, %v", result.response, result.err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

type cleanupTrackingProvider struct {
	managerClientStubProvider
	started chan struct{}
	release chan struct{}
	cleaned chan struct{}
}

func (p *cleanupTrackingProvider) ExecuteCode(ctx context.Context, inst *SandboxInstance, code, lang string, timeout int, args map[string]any) (*ExecutionResult, error) {
	close(p.started)
	select {
	case <-p.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return p.managerClientStubProvider.ExecuteCode(ctx, inst, code, lang, timeout, args)
}

func (p *cleanupTrackingProvider) DestroyInstance(ctx context.Context, _ *SandboxInstance) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	close(p.cleaned)
	return nil
}

func TestManagerClient_CleanupUsesOriginalProvider(t *testing.T) {
	p := &cleanupTrackingProvider{started: make(chan struct{}), release: make(chan struct{}), cleaned: make(chan struct{})}
	m := &ProviderManager{}
	m.SetProvider(p)
	client := &ManagerClient{manager: m}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.ExecuteCode(ctx, agenttool.SandboxRequest{Lang: "python"}); done <- err }()
	select {
	case <-p.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	m.SetProvider(managerClientStubProvider{})
	close(p.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.cleaned:
	default:
		t.Fatal("original provider was not cleaned up")
	}
}
