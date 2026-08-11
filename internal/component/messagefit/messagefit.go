//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (License); you may not
//  use this file except in compliance with the License. You may obtain a
//  copy of the License at http://www.apache.org/licenses/LICENSE-2.0
//

// Package messagefit trims a message list so its total token count fits
// within a budget. It mirrors Python's rag/prompts/generator.py:message_fit_in.
//
// The package is shared by the agent canvas LLM component and the ingestion
// Extractor component. Callers convert their message type to []Message,
// call Fit, then write the results back.
package messagefit

import (
	"ragflow/internal/tokenizer"
)

// Message is the minimal representation the fitter needs. Both the agent's
// schema.Message and ingestion's eschema.Message convert to this.
type Message struct {
	// Role is "system", "user", "assistant", etc. Only "system" receives
	// special treatment during fitting.
	Role string
	// Content is the text content that may be truncated.
	Content string
}

// Fit trims msgs so its total token count fits within maxTokens.
//
// Strategy (mirrors Python's message_fit_in):
//  1. If everything fits, return as-is.
//  2. Keep all system messages + the last non-system message, drop the
//     rest; if that fits, return.
//  3. If still over, trim proportionally:
//     - System dominates (>80% of tokens) → preserve the last message,
//     give remaining budget to system.
//     - Otherwise → preserve system, give remaining budget to last.
//     - Single message → trim to maxTokens directly.
//
// maxTokens <= 0 is treated as 8192 (Python's default).
// Returns the token count after fitting. msgs is modified in place:
// entries that were dropped have their Content set to "".
func Fit(msgs []Message, maxTokens int) int {
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	if len(msgs) == 0 {
		return 0
	}

	// Step 1: everything fits.
	if total := countTokens(msgs); total < maxTokens {
		return total
	}

	// Step 2: keep all system + last non-system.
	kept := make([]int, 0, len(msgs))
	lastNonSystem := -1
	for i := range msgs {
		if msgs[i].Role == "system" {
			kept = append(kept, i)
		} else {
			lastNonSystem = i
		}
	}
	if lastNonSystem >= 0 {
		kept = append(kept, lastNonSystem)
	}
	if len(kept) == 0 {
		return 0
	}

	keptMsgs := make([]Message, len(kept))
	for i, idx := range kept {
		keptMsgs[i] = msgs[idx]
	}
	if total := countTokens(keptMsgs); total < maxTokens {
		// Zero out the dropped entries so the caller can filter them.
		zeroDropped(msgs, kept)
		return total
	}

	// Step 3: trim proportionally.
	if len(kept) == 1 {
		msgs[kept[0]].Content = tokenizer.TrimContentToTokenLimit(msgs[kept[0]].Content, maxTokens)
		zeroDropped(msgs, kept)
		return countTokens(msgs[kept[0] : kept[0]+1])
	}

	sysIdx := kept[0]
	lastIdx := kept[len(kept)-1]
	ll := tokenizer.NumTokensFromString(msgs[sysIdx].Content)
	ll2 := tokenizer.NumTokensFromString(msgs[lastIdx].Content)
	total := ll + ll2
	if total <= 0 {
		zeroDropped(msgs, kept)
		return 0
	}

	if float64(ll)/float64(total) > 0.8 {
		preserved := min(ll2, maxTokens)
		msgs[lastIdx].Content = tokenizer.TrimContentToTokenLimit(msgs[lastIdx].Content, preserved)
		msgs[sysIdx].Content = tokenizer.TrimContentToTokenLimit(msgs[sysIdx].Content, max(0, maxTokens-preserved))
	} else {
		preserved := min(ll, maxTokens)
		msgs[sysIdx].Content = tokenizer.TrimContentToTokenLimit(msgs[sysIdx].Content, preserved)
		msgs[lastIdx].Content = tokenizer.TrimContentToTokenLimit(msgs[lastIdx].Content, max(0, maxTokens-preserved))
	}
	zeroDropped(msgs, kept)
	return countTokensMulti(msgs, kept)
}

func countTokens(msgs []Message) int {
	total := 0
	for i := range msgs {
		total += tokenizer.NumTokensFromString(msgs[i].Content)
	}
	return total
}

func countTokensMulti(msgs []Message, indices []int) int {
	total := 0
	for _, i := range indices {
		total += tokenizer.NumTokensFromString(msgs[i].Content)
	}
	return total
}

// zeroDropped sets Content to "" for every index in msgs that is not in kept.
func zeroDropped(msgs []Message, kept []int) {
	keptSet := make(map[int]struct{}, len(kept))
	for _, i := range kept {
		keptSet[i] = struct{}{}
	}
	for i := range msgs {
		if _, ok := keptSet[i]; !ok {
			msgs[i].Content = ""
		}
	}
}
