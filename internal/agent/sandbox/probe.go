package sandbox

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

const connectionProbeCode = `import json
import math

def main() -> dict:
    left = 2
    right = 2
    print(f"2 + 2 = {left + right}")
    print(f"JSON dump: {json.dumps({'test': 'data', 'value': 123})}")
    print(f"Math.sqrt(16) = {math.sqrt(16)}")
    print("TEST_PASSED")
    return {"ok": True, "provider_test": "TEST_PASSED"}
`

// TestConnection creates an independent provider and runs the same small
// diagnostic used by the admin endpoint. It never changes the active manager.
func TestConnection(ctx context.Context, provider string, config map[string]any) (map[string]any, error) {
	if err := ValidateConfig(provider, config); err != nil {
		return nil, err
	}
	p, err := buildProviderFromConfig(ProviderType(provider), config)
	if err != nil {
		return nil, err
	}
	return testConnectionWithProvider(ctx, p)
}

func testConnectionWithProvider(ctx context.Context, provider SandboxProvider) (map[string]any, error) {
	if provider == nil {
		return nil, errors.New("sandbox: provider is nil")
	}
	if err := provider.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("sandbox: initialize provider: %w", err)
	}

	inst, err := provider.CreateInstance(ctx, "python")
	if err != nil {
		return nil, fmt.Errorf("sandbox: create instance: %w", err)
	}
	if inst == nil {
		return nil, errors.New("sandbox: create instance returned no result")
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := provider.DestroyInstance(cleanupCtx, inst); err != nil {
			// Do not include provider configuration or upstream error material.
			log.Printf("sandbox connection probe cleanup failed: provider=%s", provider.ProviderType())
		}
	}()

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := provider.ExecuteCode(execCtx, inst, connectionProbeCode, "python", 10, nil)
	if err != nil {
		return nil, fmt.Errorf("sandbox: execute probe: %w", err)
	}
	if result == nil {
		return nil, errors.New("sandbox: execute probe returned no result")
	}

	success := result.ExitCode == 0 && strings.Contains(result.Stdout, "TEST_PASSED")
	status := "FAILED"
	if success {
		status = "PASSED"
	}
	messageParts := []string{
		"Test " + status,
		fmt.Sprintf("Exit code: %d", result.ExitCode),
		fmt.Sprintf("Execution time: %.2fs", result.ExecutionTime),
	}
	if strings.TrimSpace(result.Stdout) != "" {
		messageParts = append(messageParts, "Output: "+preview(result.Stdout)+"...")
	}
	if strings.TrimSpace(result.Stderr) != "" {
		messageParts = append(messageParts, "Errors: "+preview(result.Stderr)+"...")
	}
	response := map[string]any{
		"success": success,
		"message": strings.Join(messageParts, " | "),
		"details": map[string]any{
			"exit_code":      result.ExitCode,
			"execution_time": result.ExecutionTime,
			"stdout":         result.Stdout,
			"stderr":         result.Stderr,
		},
	}
	return response, nil
}

func preview(value string) string {
	runes := []rune(strings.TrimSpace(value))
	return string(runes[:min(200, len(runes))])
}
