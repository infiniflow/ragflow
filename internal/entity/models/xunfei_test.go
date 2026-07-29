package models

import "testing"

func TestXunFeiUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
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
		{"CheckConnection", func() error {
			return driver.CheckConnection(ctx, &APIConfig{})
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
