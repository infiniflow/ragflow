package component

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

func TestParserGlobalVisionModel(t *testing.T) {
	originalAnalyzer := deepdoctype.NativeDocAnalyzerFactory
	deepdoctype.NativeDocAnalyzerFactory = func() (deepdoctype.DocAnalyzer, bool) { return &pictureOCRAnalyzer{}, true }
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = originalAnalyzer })
	for _, fileType := range []string{"html", "image"} {
		for _, tc := range []struct {
			name    string
			enabled bool
			model   string
			want    string
		}{
			{"selected", true, "model-B", "model-B"},
			{"default", true, "", "tenant-default"},
			{"disabled", false, "model-B", ""},
		} {
			t.Run(fileType+"/"+tc.name, func(t *testing.T) {
				originalSpecific := resolveModelConfig
				t.Cleanup(func() { resolveModelConfig = originalSpecific })
				var resolved []string
				swapVisionGlobals(t, func(context.Context, *gorm.DB, string, entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
					resolved = append(resolved, "tenant-default")
					return &imagePromptCaptureDriver{}, "tenant-default", &modelModule.APIConfig{}, 0, nil
				}, (&visionEnhanceCaptureInvoker{}).invoke, fakePrompt)
				resolveModelConfig = func(_ context.Context, _ *gorm.DB, _ string, kind entity.ModelType, ref string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
					if kind != entity.ModelTypeImage2Text {
						t.Fatalf("model type = %v", kind)
					}
					resolved = append(resolved, ref)
					return &imagePromptCaptureDriver{}, ref, &modelModule.APIConfig{}, 0, nil
				}
				component, err := NewParserComponent(map[string]any{
					"enable_vision_enhancement": tc.enabled,
					"vlm":                       map[string]any{"llm_id": tc.model},
					"html":                      map[string]any{"vlm": map[string]any{"llm_id": "stale-family-model"}},
					"image":                     map[string]any{"parse_method": "ocr", "vlm": map[string]any{"llm_id": "stale-family-model"}},
				})
				if err != nil {
					t.Fatal(err)
				}
				binary := picturePNG(t)
				filename := "figure.png"
				if fileType == "html" {
					filename = "figure.html"
					binary = []byte(`<p><img src="data:image/png;base64,` + visionTestPNGBase64(t) + `" alt="figure"></p>`)
				}
				result, err := component.Invoke(t.Context(), nil, map[string]any{"name": filename, "file_type": fileType, "binary": binary, "tenant_id": "tenant-1"})
				if err != nil {
					t.Fatal(err)
				}
				if tc.want == "" {
					if len(resolved) != 0 {
						t.Fatalf("disabled enhancement resolved %v", resolved)
					}
				} else if len(resolved) != 1 || resolved[0] != tc.want {
					t.Fatalf("resolved %v, want [%s]", resolved, tc.want)
				}
				if fileType == "image" {
					items := result["json"].([]map[string]any)
					if !strings.Contains(items[0]["text"].(string), "OCR text") {
						t.Fatalf("OCR text lost: %v", items)
					}
				}
			})
		}
	}
}

func TestParserGlobalVisionConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		value     any
		wantError string
	}{
		{"model-B", "vlm must be an object"},
		{map[string]any{"llm_id": 42}, "vlm.llm_id must be a string"},
	} {
		_, err := NewParserComponent(map[string]any{"vlm": tc.value})
		if err == nil || !strings.Contains(err.Error(), tc.wantError) {
			t.Fatalf("vlm %v: error = %v, want %s", tc.value, err, tc.wantError)
		}
	}
	component, err := NewParserComponent(map[string]any{"vlm": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := component.(*ParserComponent).setups["vlm"]; exists {
		t.Fatal("global vlm was interpreted as a file family")
	}
}

func TestParserGlobalVisionDoesNotOverrideASR(t *testing.T) {
	original := resolveModelConfig
	t.Cleanup(func() { resolveModelConfig = original })
	var calls int
	resolveModelConfig = func(_ context.Context, _ *gorm.DB, _ string, kind entity.ModelType, ref string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		calls++
		if kind != entity.ModelTypeSpeech2Text || ref != "asr-model" {
			t.Fatalf("audio resolved %s / %s, want ASR / asr-model", kind, ref)
		}
		return &audioTranscribeDriver{transcription: "transcribed"}, ref, &modelModule.APIConfig{}, 0, nil
	}
	component, err := NewParserComponent(map[string]any{
		"enable_vision_enhancement": true,
		"vlm":                       map[string]any{"llm_id": "model-B"},
		"audio":                     map[string]any{"vlm": map[string]any{"llm_id": "asr-model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := component.Invoke(t.Context(), nil, map[string]any{"name": "test.mp3", "file_type": "audio", "binary": []byte("fake-audio"), "tenant_id": "tenant-1"})
	if err != nil {
		t.Fatal(err)
	}
	items := result["json"].([]map[string]any)
	if calls != 1 || len(items) != 1 || items[0]["text"] != "transcribed" {
		t.Fatalf("ASR calls = %d, items = %v", calls, items)
	}
}
