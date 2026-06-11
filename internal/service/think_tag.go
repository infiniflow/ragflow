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

package service

import (
	"context"
	"strings"
)

const thinkOpen = "<think>"
const thinkClose = "</think>"

// ThinkStreamState holds accumulated state across streaming LLM chunks
// so that <think>...</think> tags can be surfaced as structured markers.
//
// Corresponds to _ThinkStreamState in api/db/services/dialog_service.py.
type ThinkStreamState struct {
	// fullText accumulates all text received so far.
	fullText string
	// lastIdx is the last consumed position in fullText.
	lastIdx int
	// lastFull is the previous fullText snapshot.
	lastFull string
	// lastModelFull is the previous model chunk for diffing.
	lastModelFull string
	// inThink is true when we are currently inside a <think> block.
	inThink bool
	// buffer accumulates visible text before flushing (for batching).
	buffer string
	// postThinkText holds text between </think> and the next <think> or end
	// of delta.  Kept for API alignment with Python; may be used by future
	// callers that need per-delta visibility into think boundaries.
	postThinkText string
}

// ThinkDeltaKind describes the type of a think-tag delta event.
type ThinkDeltaKind int

const (
	ThinkDeltaText   ThinkDeltaKind = iota // visible answer text
	ThinkDeltaMarker                       // <think> or </think> tag boundary
)

// ThinkDelta is a single event produced by NextThinkDelta.
type ThinkDelta struct {
	Kind  ThinkDeltaKind
	Value string
}

// NextThinkDelta processes the next chunk of LLM output and returns any
// visible text or tag boundary markers that should be emitted.
//
// Pure function — no side effects beyond updating state.
func NextThinkDelta(state *ThinkStreamState, chunk string) []ThinkDelta {
	if state == nil {
		return nil
	}

	if state.lastFull != "" {
		// Compute the delta: what's new since lastFull.
		delta := strings.TrimPrefix(chunk, state.lastFull)
		state.lastModelFull = delta
	} else {
		state.lastModelFull = chunk
	}
	state.lastFull = chunk

	// Accumulate fullText from the delta.
	state.fullText += state.lastModelFull

	// Extract new content since lastIdx.
	newPart := state.fullText[state.lastIdx:]
	if len(newPart) == 0 {
		return nil
	}

	var deltas []ThinkDelta
	// Process character by character to detect tag boundaries.
	for len(newPart) > 0 {
		if !state.inThink {
			idx := strings.Index(newPart, thinkOpen)
			if idx < 0 {
				// No more think open — buffer everything as visible text.
				state.buffer += newPart
				state.lastIdx += len(newPart)
				break
			}
			// Text before <think> is visible answer.
			if idx > 0 {
				state.buffer += newPart[:idx]
			}
			deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaMarker, Value: thinkOpen})
			newPart = newPart[idx+len(thinkOpen):]
			state.lastIdx += idx + len(thinkOpen)
			state.inThink = true
		} else {
			idx := strings.Index(newPart, thinkClose)
			if idx < 0 {
				// Still inside think, consume all silently.
				state.lastIdx += len(newPart)
				break
			}
			deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaMarker, Value: thinkClose})
			state.postThinkText = newPart[:idx]
			newPart = newPart[idx+len(thinkClose):]
			state.lastIdx += idx + len(thinkClose)
			state.inThink = false
		}
	}

	return deltas
}

// FlushThinkBuffer drains the buffered visible text, if any, as a single delta.
// Call this after all LLM chunks have been processed.
func FlushThinkBuffer(state *ThinkStreamState) []ThinkDelta {
	if state == nil || state.buffer == "" {
		return nil
	}
	text := state.buffer
	state.buffer = ""
	return []ThinkDelta{{Kind: ThinkDeltaText, Value: text}}
}

// StreamThinkTagDelta takes a channel of raw LLM text chunks and produces a
// channel of (kind, value) pairs.  When ctx is cancelled (e.g. client
// disconnect), the goroutine drains the input channel silently and exits,
// preventing the producer goroutine from blocking forever on send.
//
// Markers (<think>, </think>) are emitted immediately without buffering.
func StreamThinkTagDelta(ctx context.Context, chunks <-chan string, minTokens int) <-chan ThinkDelta {
	out := make(chan ThinkDelta, 32)
	go func() {
		defer close(out)
		state := &ThinkStreamState{}
		flushSize := minTokens * 4 // approximate: ~4 bytes per token
		for {
			select {
			case <-ctx.Done():
				go func() {
					for range chunks {
					}
				}()
				return
			case chunk, ok := <-chunks:
				if !ok {
					for _, d := range FlushThinkBuffer(state) {
						select {
						case out <- d:
						case <-ctx.Done():
							return
						}
					}
					return
				}
				deltas := NextThinkDelta(state, chunk)
				for _, d := range deltas {
					if d.Kind == ThinkDeltaMarker {
						select {
						case out <- d:
						case <-ctx.Done():
							go func() {
								for range chunks {
								}
							}()
							return
						}
					}
				}
				// Flush buffered visible text when it reaches the token threshold,
				// matching Python _stream_with_think_delta which yields ("text", ...)
				// per chunk.  Markers are emitted immediately above.
				if len(state.buffer) >= flushSize {
					for _, d := range FlushThinkBuffer(state) {
						select {
						case out <- d:
						case <-ctx.Done():
							go func() {
								for range chunks {
								}
							}()
							return
						}
					}
				}
			}
		}
	}()
	return out
}

// ExtractVisibleAnswer strips <think> blocks from the raw LLM response,
// returning only the visible answer text.  If the response consists
// entirely of think content, returns an empty string.
//
// Corresponds to _extract_visible_answer in dialog_service.py.
func ExtractVisibleAnswer(raw string) string {
	if raw == "" {
		return ""
	}
	// Collect all non-think text.
	var visible []string
	remaining := raw
	hasThink := false

	for {
		openIdx := strings.Index(remaining, thinkOpen)
		if openIdx < 0 {
			// No more think open tags — strip any stray </think> and keep the rest.
			remaining = strings.ReplaceAll(remaining, thinkClose, "")
			visible = append(visible, remaining)
			break
		}
		hasThink = true
		if openIdx > 0 {
			visible = append(visible, remaining[:openIdx])
		}
		remaining = remaining[openIdx+len(thinkOpen):]

		closeIdx := strings.Index(remaining, thinkClose)
		if closeIdx < 0 {
			// Unclosed think — treat rest as visible.
			visible = append(visible, remaining)
			break
		}
		remaining = remaining[closeIdx+len(thinkClose):]
	}

	result := strings.TrimSpace(strings.Join(visible, ""))
	if hasThink && result == "" {
		// Only think content — return empty.
		return ""
	}
	return result
}