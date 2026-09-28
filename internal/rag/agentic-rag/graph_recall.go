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

package agentic_rag

import (
	"context"
	"log"
)

// RecallChannel names the way a passage was FOUND. It is part of the recall contract, not a label
// for the log: the channel decides how the passage should be ranked when it is delivered (a
// member's own document is not a scan window of prose, however the scores compare), and a caller
// that loses the channel can no longer say what it is looking at.
type RecallChannel string

const (
	// RecallFanout is the ranked legs: one search per opening query, over the retrieval stack.
	RecallFanout RecallChannel = "fanout"
	// RecallScan is the enumeration channel: the plan's declared probe terms asked of the corpus
	// with containment as the match (see runtime.ScanMatchAny). It answers where a member is
	// MENTIONED.
	RecallScan RecallChannel = "scan"
	// RecallTitle is the entity channel: the plan's declared subjects resolved against the
	// dataset's own title field and read from the head of their documents. It answers what a
	// member's OWN document says.
	RecallTitle RecallChannel = "title"
)

// RecallSpec is what one channel is asked for. The fields a channel does not read are ignored —
// a spec is the request's whole shape, not a union of three channels' private argument lists.
type RecallSpec struct {
	Channel RecallChannel
	// Queries are the fan-out's opening queries.
	Queries []string
	// TopN is the fan-out's per-query depth.
	TopN int
	// UseMetadata lets the fan-out run the metadata channel on its first round (see
	// graph_fanout's fetchPreSearch).
	UseMetadata bool
}

// RecallResult is what one channel contributed to the round.
type RecallResult struct {
	Channel RecallChannel
	// Added is how many NEW passages the channel admitted to the pool.
	Added int
	// Head are the channel's chunk ids in rank order: what the round's delivery should look at
	// FIRST. A caller merges the heads in the order it wants them delivered (the entity channel
	// leads with the members' own documents; the scan's windows come next).
	Head []string
	// Line is the channel's own coverage line for the log (the scan fills it today).
	Line string
}

// Recall runs ONE recall channel: it executes that channel, admits what it found into the shared
// pool, and returns what was NEW plus the ids that should lead the delivery ranking.
//
// It exists so that "how a passage was found" is decided in one place rather than at each call site:
// the three channels used to be three functions called in a row, each with its own clock, its own
// gating and its own way of prepending to the ranking — so the order the session saw was a property
// of the CALL ORDER, and the channel a passage came from was not recorded anywhere at all.
//
// The channels themselves are unchanged: this is the entry point, and each case below is the same
// call the prefetch node used to make (see RecallFanout / RecallScan / RecallTitle).
func Recall(ctx context.Context, deps RAGTools, st *AgenticState, spec RecallSpec, logger *log.Logger) RecallResult {
	switch spec.Channel {
	case RecallFanout:
		added, head := FanoutSearch(ctx, deps, st, spec.Queries, spec.TopN, spec.UseMetadata)
		return RecallResult{Channel: RecallFanout, Added: added, Head: head}
	case RecallScan:
		added, head, line := scanDeclaredProbes(ctx, deps, st, logger)
		return RecallResult{Channel: RecallScan, Added: added, Head: head, Line: line}
	case RecallTitle:
		added, head := entityTitlePrefetch(ctx, deps, st, logger)
		return RecallResult{Channel: RecallTitle, Added: added, Head: head}
	}
	// An unknown channel recalls nothing rather than guessing: a caller must name the channel it
	// wants, which is the point of the type.
	return RecallResult{Channel: spec.Channel}
}
