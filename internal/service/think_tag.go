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
	"ragflow/internal/tokenizer"
	"strings"
)

const thinkOpen = "<think>"
const thinkClose = "</think>"

var stripThinkReplacer = strings.NewReplacer("<think>", "", "</think>", "")

// ThinkStreamState holds accumulated state across streaming LLM chunks
// so that <think>...</think> tags can be surfaced as structured markers
type ThinkStreamState struct {
	// fullText accumulates all text received so far.
	fullText string
	// lastModelFull is the previous model-full snapshot for diffing
	lastModelFull string
	// inThink is true when we are currently inside a <think> block.
	inThink bool
	// closePending defers emission of </think> when no visible text follows the tag
	closePending bool
	// pendingAfterClose collects text received after a deferred </think>
	pendingAfterClose string
	// thinkBuffer is the think-buffer
	thinkBuffer string
	// answerBuffer accumulates answer-side text before token-batch flushing
	answerBuffer string
}

// ThinkDeltaKind describes the type of a think-tag delta event.
type ThinkDeltaKind int

const (
	ThinkDeltaText   ThinkDeltaKind = iota // think-side or answer-side text
	ThinkDeltaMarker                       // <think> or </think> tag boundary
)

// ThinkDelta is a single event produced by NextThinkDelta.
type ThinkDelta struct {
	Kind  ThinkDeltaKind
	Value string
}

// emitText returns the batched text and its kind.
func emitText(state *ThinkStreamState, section string, text string, minTokens int) (string, ThinkDeltaKind) {
	if text == "" {
		return "", 0
	}
	if section == "think" {
		return text, ThinkDeltaText
	}
	state.answerBuffer += text
	if tokenizer.NumTokensFromString(state.answerBuffer) >= minTokens {
		out := state.answerBuffer
		state.answerBuffer = ""
		return out, ThinkDeltaText
	}
	return "", 0
}

func flushThinkBufferInternal(state *ThinkStreamState) ThinkDelta {
	if state.thinkBuffer == "" {
		return ThinkDelta{}
	}
	out := state.thinkBuffer
	state.thinkBuffer = ""
	return ThinkDelta{Kind: ThinkDeltaText, Value: out}
}

func flushAnswerBufferInternal(state *ThinkStreamState) ThinkDelta {
	if state.answerBuffer == "" {
		return ThinkDelta{}
	}
	out := state.answerBuffer
	state.answerBuffer = ""
	return ThinkDelta{Kind: ThinkDeltaText, Value: out}
}

func stripThinkTags(s string) string {
	if s == "" {
		return ""
	}
	return stripThinkReplacer.Replace(s)
}

// NextThinkDelta processes the next chunk of LLM output and returns any
// visible text or tag boundary markers that should be emitted.
func NextThinkDelta(state *ThinkStreamState, chunk string, minTokens int) []ThinkDelta {
	if state == nil {
		return nil
	}

	var newPart string
	if strings.HasPrefix(chunk, state.lastModelFull) {
		newPart = chunk[len(state.lastModelFull):]
		state.lastModelFull = chunk
	} else {
		newPart = chunk
		state.lastModelFull += chunk
	}
	if newPart == "" {
		return nil
	}
	state.fullText += newPart
	pending := newPart

	var deltas []ThinkDelta

	// Phase 1: handle deferred </think> from a previous chunk.
	if state.closePending && !strings.Contains(pending, thinkClose) {
		state.closePending = false
		if piece := flushThinkBufferInternal(state); piece.Value != "" {
			deltas = append(deltas, piece)
		}
		state.inThink = false
		deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaMarker, Value: thinkClose})
		if state.pendingAfterClose != "" {
			answerPiece := state.pendingAfterClose
			state.pendingAfterClose = ""
			if out, kind := emitText(state, "answer", answerPiece, minTokens); out != "" {
				deltas = append(deltas, ThinkDelta{Kind: kind, Value: out})
			}
		}
		if answerPiece := stripThinkTags(pending); answerPiece != "" {
			if out, kind := emitText(state, "answer", answerPiece, minTokens); out != "" {
				deltas = append(deltas, ThinkDelta{Kind: kind, Value: out})
			}
		}
		return deltas
	}

	// Phase 2: process pending text for think tags.
	for pending != "" {
		openIdx := strings.Index(pending, thinkOpen)
		closeIdx := strings.Index(pending, thinkClose)

		// No tags remaining — emit to the appropriate section.
		if openIdx == -1 && closeIdx == -1 {
			if piece := stripThinkTags(pending); piece != "" {
				section := "answer"
				if state.inThink {
					section = "think"
				}
				if out, kind := emitText(state, section, piece, minTokens); out != "" {
					deltas = append(deltas, ThinkDelta{Kind: kind, Value: out})
				}
			}
			break
		}

		// <think> appears first (or no </think> found).
		if openIdx != -1 && (closeIdx == -1 || openIdx < closeIdx) {
			before := pending[:openIdx]
			if before != "" {
				piece := stripThinkTags(before)
				section := "answer"
				if state.inThink {
					section = "think"
				}
				if out, kind := emitText(state, section, piece, minTokens); out != "" {
					deltas = append(deltas, ThinkDelta{Kind: kind, Value: out})
				}
			}
			pending = pending[openIdx+len(thinkOpen):]
			if !state.inThink {
				if answerPiece := flushAnswerBufferInternal(state); answerPiece.Value != "" {
					deltas = append(deltas, answerPiece)
				}
				if thinkPiece := flushThinkBufferInternal(state); thinkPiece.Value != "" {
					deltas = append(deltas, thinkPiece)
				}
				state.inThink = true
				deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaMarker, Value: thinkOpen})
			}
			continue
		}

		// </think> appears first.
		before := pending[:closeIdx]
		after := pending[closeIdx+len(thinkClose):]
		if before != "" {
			piece := stripThinkTags(before)
			section := "answer"
			if state.inThink {
				section = "think"
			}
			if out, kind := emitText(state, section, piece, minTokens); out != "" {
				deltas = append(deltas, ThinkDelta{Kind: kind, Value: out})
			}
		}
		afterVisible := stripThinkTags(after)
		if strings.TrimSpace(afterVisible) != "" {
			if thinkPiece := flushThinkBufferInternal(state); thinkPiece.Value != "" {
				deltas = append(deltas, thinkPiece)
			}
			state.inThink = false
			deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaMarker, Value: thinkClose})
			pending = afterVisible
			continue
		}
		// No visible text after close — defer the marker.
		state.closePending = true
		if afterVisible != "" {
			state.pendingAfterClose += afterVisible
		}
		pending = ""
		break
	}

	return deltas
}

// FlushRemaining drains all remaining buffered text and handles deferred
// markers. Call this after all LLM chunks have been processed.
func FlushRemaining(state *ThinkStreamState) []ThinkDelta {
	if state == nil {
		return nil
	}
	var deltas []ThinkDelta
	if state.thinkBuffer != "" {
		deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaText, Value: state.thinkBuffer})
		state.thinkBuffer = ""
	}
	if state.closePending {
		state.inThink = false
		state.closePending = false
		deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaMarker, Value: thinkClose})
	}
	if state.answerBuffer != "" {
		deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaText, Value: state.answerBuffer})
		state.answerBuffer = ""
	}
	if state.pendingAfterClose != "" {
		deltas = append(deltas, ThinkDelta{Kind: ThinkDeltaText, Value: state.pendingAfterClose})
		state.pendingAfterClose = ""
	}
	return deltas
}

// StreamThinkTagDelta — channel-based pipeline.
// ---------------------------------------------------------------------------

// StreamThinkTagDelta takes a channel of raw LLM text chunks and produces a
// channel of structured deltas.  When ctx is cancelled (e.g. client
// disconnect), the goroutine drains the input channel silently and exits.
func StreamThinkTagDelta(ctx context.Context, chunks <-chan string, minTokens int) <-chan ThinkDelta {
	out := make(chan ThinkDelta, 32)
	go func() {
		defer close(out)
		state := &ThinkStreamState{}
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
					for _, d := range FlushRemaining(state) {
						select {
						case out <- d:
						case <-ctx.Done():
							return
						}
					}
					return
				}
				deltas := NextThinkDelta(state, chunk, minTokens)
				for _, d := range deltas {
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
	}()
	return out
}

// ExtractVisibleAnswer normalizes think tags in raw model output
func ExtractVisibleAnswer(raw string) string {
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, thinkClose) {
		return stripThinkTags(raw)
	}

	lastClose := strings.LastIndex(raw, thinkClose)
	thought := raw[:lastClose]
	answer := raw[lastClose+len(thinkClose):]

	thought = strings.TrimSpace(stripThinkTags(thought))
	answer = stripThinkTags(answer)
	if thought == "" {
		return answer
	}
	return thinkOpen + thought + thinkClose + answer
}

// BufferAnswerDelta accumulates answer text in state.answerBuffer.
func BufferAnswerDelta(state *ThinkStreamState, text string, minTokens int) string {
	if text == "" {
		return ""
	}
	state.answerBuffer += text
	if tokenizer.NumTokensFromString(state.answerBuffer) < minTokens {
		return ""
	}
	out := state.answerBuffer
	state.answerBuffer = ""
	return out
}
