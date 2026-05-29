package handler

import (
	"testing"

	"ragflow/internal/service"
)

func TestPrepareAddCustomModelRequestUsesPathTarget(t *testing.T) {
	req := service.AddCustomModelRequest{
		ModelName:  "custom-chat",
		ModelTypes: []string{"chat"},
	}

	if err := prepareAddCustomModelRequest(&req, "openai", "default"); err != nil {
		t.Fatalf("prepareAddCustomModelRequest returned error: %v", err)
	}
	if req.ProviderName != "openai" {
		t.Fatalf("expected provider name from path, got %q", req.ProviderName)
	}
	if req.InstanceName != "default" {
		t.Fatalf("expected instance name from path, got %q", req.InstanceName)
	}
}

func TestPrepareAddCustomModelRequestAcceptsCaseInsensitivePathMatch(t *testing.T) {
	req := service.AddCustomModelRequest{
		ProviderName: "openai",
		InstanceName: "default",
		ModelName:    "custom-chat",
		ModelTypes:   []string{"chat"},
	}

	if err := prepareAddCustomModelRequest(&req, "OpenAI", "Default"); err != nil {
		t.Fatalf("prepareAddCustomModelRequest returned error: %v", err)
	}
	if req.ProviderName != "OpenAI" {
		t.Fatalf("expected provider name from path, got %q", req.ProviderName)
	}
	if req.InstanceName != "Default" {
		t.Fatalf("expected instance name from path, got %q", req.InstanceName)
	}
}

func TestPrepareAddCustomModelRequestRejectsPathMismatches(t *testing.T) {
	tests := []struct {
		name        string
		req         service.AddCustomModelRequest
		expectedErr string
	}{
		{
			name: "provider",
			req: service.AddCustomModelRequest{
				ProviderName: "deepseek",
				InstanceName: "default",
				ModelName:    "custom-chat",
				ModelTypes:   []string{"chat"},
			},
			expectedErr: "Provider name does not match path",
		},
		{
			name: "instance",
			req: service.AddCustomModelRequest{
				ProviderName: "openai",
				InstanceName: "other",
				ModelName:    "custom-chat",
				ModelTypes:   []string{"chat"},
			},
			expectedErr: "Instance name does not match path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := prepareAddCustomModelRequest(&tt.req, "openai", "default")
			if err == nil {
				t.Fatal("expected mismatch error")
			}
			if err.Error() != tt.expectedErr {
				t.Fatalf("expected %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestPrepareAddCustomModelRequestRejectsEmptyModelTypes(t *testing.T) {
	tests := []struct {
		name       string
		modelTypes []string
	}{
		{name: "nil"},
		{name: "empty", modelTypes: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := service.AddCustomModelRequest{
				ModelName:  "custom-chat",
				ModelTypes: tt.modelTypes,
			}

			err := prepareAddCustomModelRequest(&req, "openai", "default")
			if err == nil {
				t.Fatal("expected empty model_types to return an error")
			}
			if err.Error() != "Model type is required" {
				t.Fatalf("expected model type error, got %q", err.Error())
			}
		})
	}
}
