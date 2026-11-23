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

// Package retrievalaccount classifies one retrieval tool call's RENDERED OUTPUT into the
// accounting a benchmark reads: which documents were retrieved or served, and how many
// chunks were put in front of the model at each read depth.
//
// It is a leaf package on purpose. The in-process engines feed these rules from their tool
// wrapper, and the mode-8 runner can only see the same text through the MCP result; sharing
// one implementation is what keeps the two engines' benchmark rows comparable. A leaf has
// to be where it lives: internal/agentic_rag pulls in internal/dao, which imports
// internal/codexagent, so codexagent cannot import the engine.
package retrievalaccount

import (
	"regexp"
	"strings"
)

// fullContentDocTools are the retrieval tools whose output carries COMPLETE chunk content.
// Only these feed the retrieval ledger: the locate-only tools (search_bm25_chunks,
// grep_chunks) surface a <match_snippet> window for relevance triage, not the document
// itself — a doc seen only through them was located, not retrieved.
var fullContentDocTools = map[string]struct{}{
	"search_chunks":          {},
	"search_semantic_chunks": {},
	"list_chunks":            {},
}

// deepReadChunkTools render FULL chunk content — their <chunk> elements are counted as
// deep reads. shallowReadChunkTools render only a <match_snippet> window per chunk —
// counted as shallow reads.
var deepReadChunkTools = map[string]struct{}{
	"search_chunks":          {},
	"search_semantic_chunks": {},
	"list_chunks":            {},
}

var shallowReadChunkTools = map[string]struct{}{
	"grep_chunks":        {},
	"search_bm25_chunks": {},
}

// chunkElementRe counts the <chunk ...> elements a tool rendered. The wrapper is
// <chunks>, which this deliberately does not match (it requires a space or '>' right
// after the name).
var chunkElementRe = regexp.MustCompile(`<chunk[ >]`)

// toolOutputDocNameRe pulls the document names out of the XML every retrieval tool
// renders. The 32-hex doc_id is deliberately not read: a benchmark keys its evidence on
// the file the corpus was built from.
var toolOutputDocNameRe = regexp.MustCompile(`doc_name="([^"]+)"`)

// chunkIDRe pulls the chunk identifiers out of the XML every retrieval tool renders. Each
// `<chunk ...>` element carries exactly one chunk_id and the `<chunks ...>` wrapper
// carries none, so matching the attribute is equivalent to walking the elements
// CountChunkElements counts — one wrapper covers the whole toolset.
var chunkIDRe = regexp.MustCompile(`chunk_id="([^"]+)"`)

// ToolAccounting is what ONE retrieval tool call contributed to a run's accounting.
type ToolAccounting struct {
	// RetrievedDocs are the document stems of a FULL-CONTENT tool's output — those
	// documents were opened, not merely located. Empty for the snippet-only locators.
	RetrievedDocs []string
	// ServedDocs are the document stems this call put in front of the model, for every
	// corpus tool (full-content and locators alike).
	ServedDocs []string
	// DeepChunks counts the <chunk> elements of a full-content tool's output;
	// ShallowChunks counts those of a locator's (a <match_snippet> window each). The ids
	// behind each count ride along.
	DeepChunks      int
	DeepChunkIDs    []string
	ShallowChunks   int
	ShallowChunkIDs []string
}

// AccountToolOutput classifies one tool call's rendered output (see ToolAccounting). It is
// the single implementation of the retrieval accounting rules: the in-process engines call
// it from their tool wrapper, and the mode-8 runner calls it on the MCP result text.
//
// A tool that renders no corpus document (web_search renders url/title) accounts for
// nothing, so a web hit can never inflate retrieval recall.
func AccountToolOutput(tool, out string) ToolAccounting {
	var acc ToolAccounting
	if tool == "" || out == "" {
		return acc
	}
	_, isCorpus := deepReadChunkTools[tool]
	if !isCorpus {
		_, isCorpus = shallowReadChunkTools[tool]
	}
	if !isCorpus {
		return acc
	}
	acc.ServedDocs = RecordedDocIDs(out)
	if _, full := fullContentDocTools[tool]; full {
		acc.RetrievedDocs = acc.ServedDocs
	}
	if n := CountChunkElements(out); n > 0 {
		ids := RecordedChunkIDs(out)
		if _, deep := deepReadChunkTools[tool]; deep {
			acc.DeepChunks, acc.DeepChunkIDs = n, ids
		} else {
			acc.ShallowChunks, acc.ShallowChunkIDs = n, ids
		}
	}
	return acc
}

// CountChunkElements returns the number of <chunk ...> elements in a rendered tool output
// — one per chunk the tool put in front of the model.
func CountChunkElements(out string) int {
	return len(chunkElementRe.FindAllStringIndex(out, -1))
}

// RecordedDocIDs returns the document stems mentioned in a tool's output, in match order
// and NOT deduplicated: the caller's ledger collapses repeats (and a caller that wants the
// weight of a document counts one per call, not one per chunk).
func RecordedDocIDs(out string) []string {
	if out == "" {
		return nil
	}
	ids := make([]string, 0, 8)
	for _, match := range toolOutputDocNameRe.FindAllStringSubmatch(out, -1) {
		if stem := DocNameStem(match[1]); stem != "" {
			ids = append(ids, stem)
		}
	}
	return ids
}

// DocNameStem normalizes a rendered doc_name to the file stem a benchmark keys its
// evidence on: the basename, without the .md suffix ("a/b/75314.md" and "75314" both
// become "75314").
func DocNameStem(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(name, ".md")
}

// RecordedChunkIDs returns the chunk identifiers a tool's output put in front of the
// model, in match order and not deduplicated (same contract as RecordedDocIDs).
func RecordedChunkIDs(out string) []string {
	if out == "" {
		return nil
	}
	ids := make([]string, 0, 8)
	for _, match := range chunkIDRe.FindAllStringSubmatch(out, -1) {
		if id := strings.TrimSpace(match[1]); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
