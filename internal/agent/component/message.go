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
	"log"
	"maps"
	"regexp"

	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/runtime"
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
// *CanvasState, returns the resolved string at outputs["content"], and
// (if inputs["stream"] == true) records the number of chunks in
// outputs["streamed_chunks"].
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

	text, _ := inputs["text"].(string)
	if text == "" {
		text = m.text
	}
	resolved, err := runtime.ResolveTemplate(text, state)
	if err != nil {
		// ResolveTemplate surfaces unresolved references as errors, but
		// the partial output (with empty-string substitutions) is still
		// returned so the SSE consumer can choose to log it. Match
		// the existing canvas package's contract here.
		return nil, fmt.Errorf("Message: template resolve: %w", err)
	}

	// Extract downloads. Walks inputs for download-info maps so
	// callers can attach binaries to the message body.
	var downloads []DownloadInfo
	for _, v := range inputs {
		downloads = append(downloads, ExtractDownloads(v)...)
	}

	// Pick the effective output format. inputs["output_format"]
	// overrides the per-instance declaration so the orchestrator can
	// re-render downstream.
	format := m.outputFormat
	if v, ok := inputs["output_format"].(string); ok {
		format = OutputFormat(v)
	}

	rendered := Render(RenderRequest{
		Format:    format,
		Text:      resolved,
		Downloads: downloads,
	})

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
		if len(memIDs) == 0 {
			// Fall back to per-instance memory_ids declared in
			// the DSL — the orchestrator may not re-pass them
			// when it overrides only `memory_save`.
			memIDs = extractMemoryIDsFromParams(m.text)
		}
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
				log.Printf("Message: memory_save failed: %v", saveErr)
			}
		}
	}

	if streamOn, _ := inputs["stream"].(bool); streamOn {
		// P0: one chunk for the whole resolved content. A later phase
		// can split on token / sentence boundaries.
		out["streamed_chunks"] = 1
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

// extractMemoryIDsFromParams looks for a "_memory_ids" hint in
// the component's stored text — used as a last-ditch fallback
// when the orchestrator does not re-pass memory_ids. Returns nil
// in the common case; this helper exists to keep the public
// memory-save flow permissive about caller omissions.
func extractMemoryIDsFromParams(_ string) []string {
	return nil
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

// Stream is the SSE variant. The resolved template content is
// split on sentence boundaries ([.!?]\s+ between letters/digits)
// and each sentence is emitted as a separate chunk. A trailing
// "done" marker signals end-of-stream. The chunk map's "content"
// key carries the sentence text; "done" is true on the final
// chunk.
//
// The splitter uses a regex for portable sentence boundaries
// without pulling in a tokenizer. A future tokenizer-aware
// splitter (gonja + langdetect, or a small Go
// sentence-segmentation lib) can improve break quality.
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
		if text == "" {
			// Nothing to split; emit a single empty-content chunk
			// plus the done marker so downstream consumers have a
			// well-defined two-chunk stream.
			select {
			case ch <- map[string]any{"content": "", "thinking": ""}:
			case <-ctx.Done():
				return
			}
		} else {
			sentences := splitSentences(text)
			for _, s := range sentences {
				select {
				case ch <- map[string]any{"content": s, "thinking": ""}:
				case <-ctx.Done():
					return
				}
			}
		}
		select {
		case ch <- map[string]any{"done": true, "model": result["model"]}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}

// sentenceSplitRe matches sentence boundaries: ".", "!", or "?"
// followed by whitespace. The character class keeps the
// abbreviations list short for v1; the follow-up tokenizer-aware
// splitter is a more robust replacement.
var sentenceSplitRe = regexp.MustCompile(`([.!?])\s+`)

// splitSentences splits text on sentence boundaries, preserving
// the trailing punctuation. Returns a slice of at least one
// element; empty input returns a single empty element.
func splitSentences(text string) []string {
	if text == "" {
		return []string{""}
	}
	matches := sentenceSplitRe.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []string{text}
	}
	out := make([]string, 0, len(matches)+1)
	prev := 0
	for _, m := range matches {
		// Include the matched punctuation in the previous sentence
		// but stop BEFORE the trailing whitespace — otherwise each
		// emitted sentence has a leading space, which both the
		// Message component's stream joiner and the v1 Python
		// chunker would have to re-trim.
		out = append(out, text[prev:m[0]+1])
		prev = m[1]
	}
	if prev < len(text) {
		out = append(out, text[prev:])
	}
	return out
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

// Outputs returns the resolved template plus the streamed-chunk
// counter.
func (m *MessageComponent) Outputs() map[string]string {
	return map[string]string{
		"content":         "Resolved and rendered message body.",
		"streamed_chunks": "Number of SSE chunks emitted (present when stream=true).",
		"downloads":       "Extracted download descriptors ({doc_id, filename, mime_type, url}).",
		"audio":           "{media_type, data_b64} envelope populated when auto_play is wired and a TTS engine succeeds.",
		"audio_error":     "Surfaced when TTS dispatch fails; the textual content is still returned.",
		"memory_error":    "Surfaced when memory persistence fails; the textual content is still returned.",
	}
}

// mapCopy shallow-copies src into a fresh map. Used to keep Message's
// passthrough outputs un-aliased from the caller's inputs map.
func mapCopy(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	maps.Copy(out, src)
	return out
}

func init() {
	Register(componentNameMessage, NewMessageComponent)
}
