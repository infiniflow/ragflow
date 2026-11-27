//go:build manual

package models

import (
	"context"
	"os"
	"testing"

	"ragflow/internal/common"
)

func newSarvamLiveForTest(t *testing.T) (*SarvamModel, *APIConfig) {
	t.Helper()
	apiKey := os.Getenv("SARVAM_API_KEY")
	if apiKey == "" {
		t.Skip("SARVAM_API_KEY is not set")
	}
	return NewSarvamModel(
		map[string]string{"default": "https://api.sarvam.ai/v2"},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	), &APIConfig{ApiKey: &apiKey}
}

func TestSarvamLiveChat(t *testing.T) {
	model, apiConfig := newSarvamLiveForTest(t)

	usage := &common.ModelUsage{}
	resp, err := model.ChatWithMessages(
		context.Background(),
		"sarvam-105b",
		[]Message{{Role: "user", Content: "Reply with exactly one word: ok"}},
		apiConfig,
		nil,
		usage,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer == "" {
		t.Fatal("empty answer")
	}
	t.Logf("answer=%q usage=%+v", *resp.Answer, usage)
}

func TestSarvamLiveListModels(t *testing.T) {
	model, apiConfig := newSarvamLiveForTest(t)

	list, err := model.ListModels(context.Background(), apiConfig)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("no models returned")
	}
	t.Logf("models=%s", joinModelNames(list, ","))
}
