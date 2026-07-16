//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package component — Message component (T3).
//
// Message is the canvas terminal output node. It resolves a
// Jinja2-style {{...}} template against the current *CanvasState
// and (optionally) emits the result as a single SSE chunk.
//
// Capabilities:
//   - output_format rendering (html / markdown / plain) via render.go
//   - auto_play → TTS engine dispatch via internal/agent/audio
//   - download extraction from inputs (the {doc_id, filename,
//     mime_type} walk from Python's _extract_downloads)
//   - memory_save persistence via the registered MemorySaver
//     (default stub returns ErrMemoryServiceMissing until a real
//     implementation is wired at boot)
package component

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

const componentNameMessage = "Message"

// MessageComponent is the canvas terminal output node. It owns
// the resolved text template as a per-instance field — the factory
// sets it from the DSL params at build time, and Invoke falls back
// to it when the input map does not carry a fresh "text" override.
//
// Per-instance format / TTS / memory config lets the build-time
// DSL declarations take effect without input-map plumbing.
type MessageComponent struct {
	name         string
	text         string
	outputFormat OutputFormat
	autoPlay     audio.Engine
	voice        string
	lang         string
}

// NewMessageComponent constructs a Message component. The params map
// may carry:
//
//   - "text"          (string) — the canonical v2 name
//   - "content"       (string | []string | []any) — the v1 name
//   - "output_format" (string) — "html" | "markdown" | "plain"
//   - "auto_play"     (bool | string) — TTS engine toggle
//     (true → "gtts", string → that engine name)
//   - "voice"         (string) — TTS voice hint
//   - "lang"          (string) — TTS language tag
//   - "memory_ids"    ([]string | []any) — list of memory
//     stores to persist into when memory_save=true
//
// At least one of text/content must produce a non-empty string;
// otherwise the node emits an empty content (it is the canvas
// terminal, so a runtime error would be louder than a missing
// template).
func NewMessageComponent(params map[string]any) (Component, error) {
	tpl := extractMessageText(params)
	format := OutputFormatPlain
	if v, ok := params["output_format"].(string); ok {
		format = OutputFormat(v)
	}
	engine, voice, lang := extractAudioConfig(params)
	return &MessageComponent{
		name:         componentNameMessage,
		text:         tpl,
		outputFormat: format,
		autoPlay:     engine,
		voice:        voice,
		lang:         lang,
	}, nil
}

// extractAudioConfig reads auto_play / voice / lang from the
// params map. auto_play=true → EngineGTTS; auto_play="edge-tts"
// → EngineEdge; false/missing → EngineEmpty. The string form
// is preferred when the user typed a specific engine name.
func extractAudioConfig(params map[string]any) (audio.Engine, string, string) {
	var engine audio.Engine
	if v, ok := params["auto_play"]; ok {
		switch x := v.(type) {
		case bool:
			if x {
				engine = audio.EngineGTTS
			}
		case string:
			engine = audio.Engine(x)
		}
	}
	voice, _ := params["voice"].(string)
	lang, _ := params["lang"].(string)
	return engine, voice, lang
}

// extractMessageText reads text / content from params in the v1 / v2
// order documented on NewMessageComponent. Returns the empty string
// when neither key is present or the value is not a string-shaped
// scalar.
func extractMessageText(params map[string]any) string {
	if v, ok := params["text"].(string); ok {
		return v
	}
	if v, ok := params["content"]; ok {
		switch x := v.(type) {
		case string:
			return x
		case []string:
			if len(x) > 0 {
				return x[0]
			}
		case []any:
			if len(x) > 0 {
				if s, ok := x[0].(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

// Name returns the registered component name.
func (m *MessageComponent) Name() string { return m.name }

// Invoke resolves inputs["text"] (or the per-instance text seeded
// from params at build time) as a template against the current
// *CanvasState and returns the resolved string at outputs["content"].
//
// Message Invoke behaviour:
//   - input-format override: inputs["output_format"] wins over the
//     per-instance format so an orchestrator can re-render
//     downstream
//   - downloads: walks inputs for {doc_id, filename, mime_type}
//     entries; sets outputs["downloads"] when any are present
//   - auto_play: when m.autoPlay is non-empty, dispatches the
//     resolved content through the registered audio.Synthesizer
//     and surfaces base64 audio under outputs["audio"]
//   - memory_save: when true, calls the registered MemorySaver
//     with the resolved content (errors are surfaced but not fatal
//     so a missing memory service does not break the message)
//
// inputs["text"] takes precedence over the per-instance text so the
// same node can be reused with different templates at run time when
// the orchestrator wants to override the DSL-declared value.
func (m *MessageComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("Message: nil canvas state")
	}

	text := extractMessageText(inputs)
	if text == "" {
		text = m.text
	}
	if text == "" {
		text = fallbackMessageText(inputs)
	}

	resolved, err := runtime.ResolveTemplate(text, state)
	if err != nil {
		return nil, fmt.Errorf("Message: %w", err)
	}

	// Extract downloads. Walks inputs for download-info maps so
	// callers can attach binaries to the message body.
	downloads := ExtractDownloads(resolved)
	if len(downloads) > 0 && downloadInfoString(resolved) {
		resolved = ""
	}
	for key, v := range inputs {
		if key == "text" {
			continue
		}
		downloads = appendUniqueDownloads(downloads, ExtractDownloads(v))
	}

	// Pick the effective output format. inputs["output_format"]
	// overrides the per-instance declaration so the orchestrator can
	// re-render downstream.
	format := m.outputFormat
	if v, ok := inputs["output_format"].(string); ok {
		format = OutputFormat(v)
	}

	rendered := ""
	if resolved != "" {
		rendered = Render(RenderRequest{
			Format: format,
			Text:   resolved,
		})
	}

	out := map[string]any{"content": rendered}
	if len(downloads) > 0 {
		out["downloads"] = downloads
	}

	// auto_play TTS dispatch. The audio bytes are returned under
	// outputs["audio"] as a structured envelope; the SSE layer
	// can choose to forward them on a separate event channel.
	if m.autoPlay != audio.EngineEmpty {
		engine := m.autoPlay
		if v, ok := inputs["auto_play"]; ok {
			switch x := v.(type) {
			case bool:
				if x {
					engine = audio.EngineGTTS
				}
			case string:
				engine = audio.Engine(x)
			}
		}
		voice := m.voice
		if v, ok := inputs["voice"].(string); ok && v != "" {
			voice = v
		}
		lang := m.lang
		if v, ok := inputs["lang"].(string); ok && v != "" {
			lang = v
		}
		synth := audio.GetSynthesizer()
		resp, ttsErr := synth.Synthesize(ctx, audio.SynthesizeRequest{
			Engine: engine,
			Text:   rendered,
			Voice:  voice,
			Lang:   lang,
		})
		if ttsErr != nil {
			// TTS failures are non-fatal — the textual content is
			// already in `content`. Surface the error under a
			// dedicated key so callers can decide whether to retry.
			out["audio_error"] = ttsErr.Error()
		} else if resp != nil && len(resp.Audio) > 0 {
			out["audio"] = map[string]any{
				"media_type": resp.MediaType,
				// Base64 is the standard SSE wire shape for
				// binary payloads.
				"data_b64": resp.Audio,
			}
		}
	}

	// Memory persistence. The call is best-effort: a missing
	// memory service returns ErrMemoryServiceMissing which we
	// surface under outputs["memory_error"] so the message still
	// flows.
	memSave, _ := inputs["memory_save"].(bool)
	if memSave {
		memIDs := extractMemoryIDs(inputs)
		if len(memIDs) > 0 {
			saver := GetMemorySaver()
			saveErr := saver.Save(ctx, MemorySaveRequest{
				MemoryIDs:     memIDs,
				AgentID:       state.TaskID,
				SessionID:     state.RunID,
				UserInput:     stringFromStateSys(state, "query"),
				AgentResponse: rendered,
			})
			if saveErr != nil {
				out["memory_error"] = saveErr.Error()
				common.Error("Message: memory_save failed", saveErr)
			}
		}
	}

	return out, nil
}

// extractMemoryIDs normalises a memory_ids value from inputs /
// params. Accepts []string and []any[string].
func extractMemoryIDs(inputs map[string]any) []string {
	v, ok := inputs["memory_ids"]
	if !ok {
		return nil
	}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func fallbackMessageText(inputs map[string]any) string {
	if inputs == nil {
		return ""
	}
	if text, _ := inputs["formalized_content"].(string); strings.TrimSpace(text) != "" {
		return text
	}

	var only string
	count := 0
	for key, value := range inputs {
		if isMessageInfraInput(key) {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		only = text
		count++
		if count > 1 {
			return ""
		}
	}
	if count == 1 {
		return only
	}
	return ""
}

func isMessageInfraInput(key string) bool {
	switch key {
	case "state", "__cpn_id__", "__legacy_noop__", "_created_time", "_elapsed_time",
		"output_format", "voice", "lang", "auto_play", "memory_save", "stream":
		return true
	default:
		return false
	}
}

// stringFromStateSys reads a sys-level state value. Returns ""
// when state or the key is missing. Used by the memory-save path
// to pull the user's original query.
func stringFromStateSys(state *runtime.CanvasState, key string) string {
	if state == nil {
		return ""
	}
	if v, ok := state.Sys[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Stream resolves the message and emits the content chunk. The outer
// Agent SSE handler owns the final [DONE] frame, matching Python's
// agent_api.py rather than leaking a component-local done marker.
func (m *MessageComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	ch := make(chan map[string]any, 16)
	go func() {
		defer close(ch)
		result, err := m.Invoke(ctx, inputs)
		if err != nil {
			select {
			case ch <- map[string]any{"error": err.Error()}:
			case <-ctx.Done():
			}
			return
		}
		text, _ := result["content"].(string)
		select {
		case ch <- map[string]any{"content": text, "thinking": ""}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}

// Inputs returns the public parameter surface. Field types match
// the Python DSL contract (text template, stream toggle,
// memory_save toggle).
func (m *MessageComponent) Inputs() map[string]string {
	return map[string]string{
		"text":          "Template string with {{...}} references; resolved against the canvas state.",
		"stream":        "When true, the resolved content is delivered as an SSE stream.",
		"memory_save":   "When true, persist the message via the registered MemorySaver (default stub returns ErrMemoryServiceMissing).",
		"memory_ids":    "List of memory-store IDs to persist into (used when memory_save=true).",
		"output_format": "'html' | 'markdown' | 'plain'. Default 'plain' when unset.",
		"auto_play":     "When truthy, dispatch the resolved text through the audio.Synthesizer.",
		"voice":         "TTS voice hint (engine-specific).",
		"lang":          "TTS language tag (BCP-47, e.g. 'en' or 'zh-CN').",
	}
}

// Outputs returns the resolved template plus optional side-channel outputs.
func (m *MessageComponent) Outputs() map[string]string {
	return map[string]string{
		"content":      "Resolved and rendered message body.",
		"downloads":    "Extracted download descriptors ({doc_id, filename, mime_type, url}).",
		"audio":        "{media_type, data_b64} envelope populated when auto_play is wired and a TTS engine succeeds.",
		"audio_error":  "Surfaced when TTS dispatch fails; the textual content is still returned.",
		"memory_error": "Surfaced when memory persistence fails; the textual content is still returned.",
	}
}

func init() {
	Register(componentNameMessage, NewMessageComponent)
}
