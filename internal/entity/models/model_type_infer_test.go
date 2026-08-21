package models

import (
	"reflect"
	"testing"
)

func withProviderManager(t *testing.T, manager *ProviderManager) {
	t.Helper()
	previous := providerManager
	providerManager = manager
	t.Cleanup(func() {
		providerManager = previous
	})
}

func TestInferMissingModelTypesByNameHints(t *testing.T) {
	withProviderManager(t, nil)

	tests := []struct {
		name string
		want []string
	}{
		{name: "bge-reranker-v2-m3", want: []string{"rerank"}},
		{name: "text-embedding-3-large", want: []string{"embedding"}},
		{name: "whisper-large-v3", want: []string{"asr"}},
		{name: "cosyvoice-tts", want: []string{"tts"}},
		{name: "paddle-ocr-v5", want: []string{"ocr"}},
		{name: "qwen-vl-max", want: []string{"chat", "vision"}},
		{name: "unknown-model", want: []string{"chat"}},
	}

	for _, tt := range tests {
		if got := InferMissingModelTypes(tt.name); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("InferMissingModelTypes(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestInferMissingModelTypesPrefersExactMetadata(t *testing.T) {
	withProviderManager(t, &ProviderManager{
		AllModels: []Model{
			{Name: "custom-embedding-name", ModelTypes: []string{"chat"}},
		},
		Alias2ModelIndex: map[string]int{"custom-embedding-name": 0},
	})

	if got := InferMissingModelTypes("custom-embedding-name"); !reflect.DeepEqual(got, []string{"chat"}) {
		t.Fatalf("InferMissingModelTypes exact = %v, want [chat]", got)
	}
}

func TestInferMissingModelTypesDoesNotUseSimilarKnownModel(t *testing.T) {
	withProviderManager(t, &ProviderManager{
		AllModels: []Model{
			{Name: "vendor/foo-32b-instruct", ModelTypes: []string{"tts"}},
		},
		Alias2ModelIndex: map[string]int{},
	})

	if got := InferMissingModelTypes("foo"); !reflect.DeepEqual(got, []string{"chat"}) {
		t.Fatalf("InferMissingModelTypes similar = %v, want [chat]", got)
	}
}

func TestInferMissingModelTypesKeepsOnlyChatVisionCoexistence(t *testing.T) {
	withProviderManager(t, &ProviderManager{
		AllModels: []Model{
			{Name: "speech-vision-model", ModelTypes: []string{"asr", "vision"}},
			{Name: "vision-model", ModelTypes: []string{"vision"}},
		},
		Alias2ModelIndex: map[string]int{
			"speech-vision-model": 0,
			"vision-model":        1,
		},
	})

	if got := InferMissingModelTypes("speech-vision-model"); !reflect.DeepEqual(got, []string{"asr"}) {
		t.Fatalf("InferMissingModelTypes asr+vision = %v, want [asr]", got)
	}
	if got := InferMissingModelTypes("vision-model"); !reflect.DeepEqual(got, []string{"chat", "vision"}) {
		t.Fatalf("InferMissingModelTypes vision = %v, want [chat vision]", got)
	}
}

func TestParseListModelInfersMissingModelTypes(t *testing.T) {
	withProviderManager(t, nil)

	models := ParseListModel(ModelList{Models: []ModelListItem{{ID: "bge-reranker-v2-m3"}}})
	if len(models) != 1 {
		t.Fatalf("models len = %d, want 1", len(models))
	}
	if got := models[0].ModelTypes; !reflect.DeepEqual(got, []string{"rerank"}) {
		t.Fatalf("ModelTypes = %v, want [rerank]", got)
	}
}
