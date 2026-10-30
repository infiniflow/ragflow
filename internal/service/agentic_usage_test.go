package service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ragflow/internal/tokenizer"
)

// TestAgenticUsageFields pins the per-turn cost record that stands in for the
// token usage the chat-completions response never carries: totals, call count,
// per-phase split, and the count of calls whose provider reported nothing.
func TestAgenticUsageFields(t *testing.T) {
	base := tokenizer.WithRunUsage(context.Background())
	explore := tokenizer.WithUsagePhase(base, "explore")
	tokenizer.RecordRunTokenUsageFor(explore, "MiniMax-M3", 1000, 100, 1100)
	tokenizer.RecordRunTokenUsageFor(explore, "MiniMax-M3", 2000, 200, 2200)
	synth := tokenizer.WithUsagePhase(base, "synth")
	tokenizer.RecordRunTokenUsageFor(synth, "MiniMax-M3", 100, 300, 400)
	// A provider that reports no usage must be counted, not silently folded in
	// as a free call.
	tokenizer.RecordRunTokenUsageFor(synth, "MiniMax-M3", 0, 0, 0)

	fields := agenticLogFields(t, agenticUsageFields(base, tokenizer.GetRunUsage(base)))
	assertLogField(t, fields, "prompt_tokens", 3100)
	assertLogField(t, fields, "completion_tokens", 600)
	assertLogField(t, fields, "total_tokens", 3700)
	assertLogField(t, fields, "llm_calls", 4)
	assertLogField(t, fields, "usage_missing_calls", 1)
	assertLogField(t, fields, "tokens_per_call", 925)
	assertLogField(t, fields, "top_model", "MiniMax-M3")
	assertLogField(t, fields, "usage_incomplete", true)

	phases, ok := fields["tokens_by_phase"].(map[string]interface{})
	if !ok {
		t.Fatalf("tokens_by_phase has type %T", fields["tokens_by_phase"])
	}
	if phases["explore"] != float64(3300) || phases["synth"] != float64(400) {
		t.Fatalf("tokens_by_phase = %v", phases)
	}
}

// TestAgenticUsageFieldsWithoutSink covers the path where accounting was never
// installed: the log line must say so instead of reporting zeros as a real
// measurement.
func TestAgenticUsageFieldsWithoutSink(t *testing.T) {
	fields := agenticLogFields(t, agenticUsageFields(context.Background(), nil))
	assertLogField(t, fields, "usage_tracked", false)
	if _, ok := fields["total_tokens"]; ok {
		t.Fatalf("untracked turn must not report totals: %v", fields)
	}
}

// agenticLogFields renders zap fields through a real JSON encoder so the test
// observes exactly what the log sink would emit.
func agenticLogFields(t *testing.T, fields []zap.Field) map[string]interface{} {
	t.Helper()
	var buf bytes.Buffer
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&buf),
		zapcore.InfoLevel,
	))
	logger.Info("agentic turn done", fields...)
	var decoded map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("decode log line %q: %v", buf.String(), err)
	}
	return decoded
}

func assertLogField(t *testing.T, fields map[string]interface{}, key string, want interface{}) {
	t.Helper()
	got, ok := fields[key]
	if !ok {
		t.Fatalf("missing log field %q in %v", key, fields)
	}
	switch expected := want.(type) {
	case int:
		number, ok := got.(float64)
		if !ok || number != float64(expected) {
			t.Fatalf("%s = %v, want %d", key, got, expected)
		}
	case bool:
		flag, ok := got.(bool)
		if !ok || flag != expected {
			t.Fatalf("%s = %v, want %v", key, got, expected)
		}
	case string:
		text, ok := got.(string)
		if !ok || text != expected {
			t.Fatalf("%s = %v, want %q", key, got, expected)
		}
	default:
		t.Fatalf("unsupported assertion type %T for %s", want, key)
	}
}
