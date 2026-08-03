package models

import (
	"strings"
	"testing"
)

func TestResolveSparkModel(t *testing.T) {
	cases := map[string]string{
		"Spark-Max":       "generalv3.5",
		"Spark-Max-32K":   "max-32k",
		"Spark-Lite":      "lite",
		"Spark-Pro":       "generalv3",
		"Spark-Pro-128K":  "pro-128k",
		"Spark-4.0-Ultra": "4.0Ultra",
		// Unknown names pass through unchanged (e.g. "spark-x").
		"spark-x": "spark-x",
	}
	for name, want := range cases {
		if got := resolveSparkModel(name); got != want {
			t.Errorf("resolveSparkModel(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestResolveBearerToken(t *testing.T) {
	bundle := `{"spark_api_password":"pwd","spark_app_id":"app","spark_api_secret":"secret","spark_api_key":"key"}`
	cases := []struct {
		name string
		key  *string
		want string
	}{
		{"nil key", nil, ""},
		{"plain key", strPtr("sk-plain"), "sk-plain"},
		{"bundle uses password", strPtr(bundle), "pwd"},
		{"bundle without password falls back to raw", strPtr(`{"spark_app_id":"app"}`), `{"spark_app_id":"app"}`},
		{"malformed json falls back to raw", strPtr(`{not-json`), `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveBearerToken(&APIConfig{ApiKey: tc.key})
			if got != tc.want {
				t.Errorf("resolveBearerToken() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestXunFeiCheckConnectionRequiresAPIKey(t *testing.T) {
	driver := NewXunFeiModel(map[string]string{"default": "http://unused"}, URLSuffix{}).
		NewInstance(map[string]string{"default": "http://unused"})
	err := driver.CheckConnection(t.Context(), &APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("CheckConnection with empty key = %v, want 'api key is required'", err)
	}
}

func strPtr(s string) *string { return &s }

func TestXunFeiUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	driver := NewXunFeiModel(map[string]string{"default": "http://unused"}, URLSuffix{}).
		NewInstance(map[string]string{"default": "http://unused"})
	modelName := "spark"
	text := "hello"

	checks := []struct {
		name string
		call func() error
	}{
		{"Embed", func() error {
			_, err := driver.Embed(ctx, &modelName, []string{text}, &APIConfig{}, nil, nil)
			return err
		}},
		{"Rerank", func() error {
			_, err := driver.Rerank(ctx, &modelName, text, []string{text}, &APIConfig{}, nil, nil)
			return err
		}},
		{"TranscribeAudio", func() error {
			_, err := driver.TranscribeAudio(ctx, &modelName, &text, &APIConfig{}, nil, nil)
			return err
		}},
		{"TranscribeAudioWithSender", func() error {
			return driver.TranscribeAudioWithSender(ctx, &modelName, &text, &APIConfig{}, nil, nil, nil)
		}},
		{"AudioSpeech", func() error {
			_, err := driver.AudioSpeech(ctx, &modelName, &text, &APIConfig{}, nil, nil)
			return err
		}},
		{"AudioSpeechWithSender", func() error {
			return driver.AudioSpeechWithSender(ctx, &modelName, &text, &APIConfig{}, nil, nil, nil)
		}},
		{"OCRFile", func() error {
			_, err := driver.OCRFile(ctx, &modelName, nil, &text, &APIConfig{}, nil, nil)
			return err
		}},
		{"ParseFile", func() error {
			_, err := driver.ParseFile(ctx, &modelName, nil, &text, &APIConfig{}, nil, nil)
			return err
		}},
		{"Balance", func() error {
			_, err := driver.Balance(ctx, &APIConfig{})
			return err
		}},
		{"ListTasks", func() error {
			_, err := driver.ListTasks(ctx, &APIConfig{})
			return err
		}},
		{"ShowTask", func() error {
			_, err := driver.ShowTask(ctx, "task-id", &APIConfig{})
			return err
		}},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			requireNoSuchMethod(t, check.name, check.call())
		})
	}
}
