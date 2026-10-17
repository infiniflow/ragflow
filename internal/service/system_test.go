//go:build !integration
// +build !integration

package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/common"
)

type oceanBaseStatusTestEngine struct {
	fakeChatDocEngine
	pingErr error
}

func (oceanBaseStatusTestEngine) GetType() string { return "oceanbase" }

func (e oceanBaseStatusTestEngine) Ping(context.Context) error { return e.pingErr }

func TestOceanBaseStatusRejectsNonOceanBaseEngine(t *testing.T) {
	data, code, err := oceanBaseStatus(t.Context(), fakeChatDocEngine{})
	if data != nil {
		t.Fatalf("data=%v, want nil", data)
	}
	if code != common.CodeServerError {
		t.Fatalf("code=%d, want %d", code, common.CodeServerError)
	}
	if err == nil || err.Error() != "OceanBase is not in use." {
		t.Fatalf("err=%v, want OceanBase is not in use", err)
	}
}

func TestOceanBaseStatusReportsHealthyEngine(t *testing.T) {
	data, code, err := oceanBaseStatus(t.Context(), oceanBaseStatusTestEngine{})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("oceanBaseStatus() = data=%v code=%d err=%v", data, code, err)
	}
	if data["status"] != "alive" {
		t.Fatalf("status=%v, want alive", data["status"])
	}
	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("message=%T, want object", data["message"])
	}
	health, ok := message["health"].(map[string]interface{})
	if !ok || health["status"] != "healthy" || health["type"] != "oceanbase" {
		t.Fatalf("health=%v, want healthy oceanbase", message["health"])
	}
	performance, ok := message["performance"].(map[string]interface{})
	if !ok || performance["connection"] != "connected" {
		t.Fatalf("performance=%v, want connected", message["performance"])
	}
	if latency, ok := performance["latency_ms"].(float64); !ok || latency < 0 {
		t.Fatalf("latency_ms=%v, want non-negative number", performance["latency_ms"])
	}
}

func TestOceanBaseStatusConvertsPingFailureToTimeoutPayload(t *testing.T) {
	data, code, err := oceanBaseStatus(t.Context(), oceanBaseStatusTestEngine{pingErr: errors.New("connection refused")})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("oceanBaseStatus() = data=%v code=%d err=%v", data, code, err)
	}
	if data["status"] != "timeout" {
		t.Fatalf("status=%v, want timeout", data["status"])
	}
	if data["message"] != "error: connection refused" {
		t.Fatalf("message=%v, want masked timeout message", data["message"])
	}
}

func TestSystemServiceHealthzReportsUnhealthyDependencies(t *testing.T) {
	result, allOK := NewSystemService().Healthz(t.Context())
	if allOK {
		t.Fatal("allOK=true, want false without initialized dependencies")
	}
	if result.Status != "nok" {
		t.Fatalf("status=%q, want nok", result.Status)
	}
	if result.DB != "nok" || result.Redis != "nok" || result.DocEngine != "nok" || result.Storage != "nok" || result.MessageQueue != "nok" {
		t.Fatalf("unexpected health result: %+v", result)
	}
	if len(result.Meta) == 0 {
		t.Fatal("expected failure metadata")
	}
}
