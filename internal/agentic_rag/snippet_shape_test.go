/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package agentic_rag

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestSnippetShipsShortChunkWhole pins rule 1: a chunk under the whole-chunk
// threshold has no window at all, so the sentence after the keyword can never be
// cut away (the failure this shape exists to kill).
func TestSnippetShipsShortChunkWhole(t *testing.T) {
	content := "Intro line. The Bronner archive moved to Fresno in 1994. Closing line."
	first := strings.Index(content, "Bronner")
	snippet, truncated := snippetForMatches(content, first, first+len("Bronner"))
	if truncated {
		t.Error("a short chunk must not be reported as truncated")
	}
	if snippet != content {
		t.Fatalf("short chunk must ship whole:\n got %q\nwant %q", snippet, content)
	}
}

// TestSnippetAlignsToParagraph pins rule 3: the fragment is the whole paragraph
// around the hits, not a fixed rune window ending mid-sentence.
func TestSnippetAlignsToParagraph(t *testing.T) {
	para := "The archive lists every Bronner recording. Its catalogue notes the Bronner estate gave the tapes away."
	content := strings.Repeat("padding prose that goes on. ", 12) + "\n\n" + para + "\n\n" + strings.Repeat("more padding prose. ", 12)
	first := strings.Index(content, "Bronner")
	last := strings.LastIndex(content, "Bronner") + len("Bronner")
	snippet, truncated := snippetForMatches(content, first, last)
	if !truncated {
		t.Error("a long chunk with a bounded paragraph must be truncated")
	}
	if !strings.Contains(snippet, para) {
		t.Fatalf("snippet must contain the complete paragraph:\n got %q", snippet)
	}
}

// TestSnippetAlignsToSentenceBoundaries pins rule 4: with no paragraph break to
// use, the window still opens and closes on sentence edges.
func TestSnippetAlignsToSentenceBoundaries(t *testing.T) {
	// One long paragraph, no blank lines: only sentences separate the text.
	sentence := "Sentence number filler keeps the paragraph long. "
	content := strings.Repeat(sentence, 40) +
		"Here the Bronner trail begins. It continues right here with more Bronner detail. " +
		strings.Repeat(sentence, 40)
	first := strings.Index(content, "Bronner")
	last := strings.LastIndex(content, "Bronner") + len("Bronner")
	snippet, _ := snippetForMatches(content, first, last)
	// Strip the truncation marker only: the fragment itself must then end on a
	// sentence terminator (a plain HasSuffix(".") would be satisfied by "...").
	core := strings.TrimSuffix(snippet, "...")
	if !strings.HasSuffix(core, ".") {
		t.Fatalf("snippet must close on a sentence end, got ...%.60q", core[len(core)-60:])
	}
	if strings.Count(snippet, "Bronner") != 2 {
		t.Fatalf("both hits must survive inside one fragment, got %q", snippet)
	}
	if n := len([]rune(snippet)); n > snippetMaxRunes+snippetAlignTolerance+6 {
		t.Fatalf("snippet over budget: %d runes", n)
	}
}

// TestSnippetShipsWholeWhenHitsAreScattered pins rule 2: hits further apart than
// the budget cannot be windowed without dropping one, so a bounded chunk ships
// whole instead.
func TestSnippetShipsWholeWhenHitsAreScattered(t *testing.T) {
	content := "Bronner opens the record. " + strings.Repeat("filler words follow. ", 80) + "And Bronner closes it."
	first := strings.Index(content, "Bronner")
	last := strings.LastIndex(content, "Bronner") + len("Bronner")
	snippet, truncated := snippetForMatches(content, first, last)
	if truncated {
		t.Error("a bounded chunk with scattered hits ships whole, not truncated")
	}
	if snippet != collapseSpaces(content) {
		t.Fatalf("expected the whole chunk, got %d runes of %d", len([]rune(snippet)), len([]rune(content)))
	}

	// Past the whole-chunk cap the fragment keeps the FIRST hit cluster and is
	// marked truncated, so the model is told to read the rest rather than being
	// handed a fragment that silently lost a keyword.
	huge := "Bronner opens the record. " + strings.Repeat("filler words follow. ", 300) + "And Bronner closes it."
	first = strings.Index(huge, "Bronner")
	last = strings.LastIndex(huge, "Bronner") + len("Bronner")
	snippet, truncated = snippetForMatches(huge, first, last)
	if !truncated {
		t.Error("an over-cap chunk must report truncation")
	}
	if len([]rune(huge)) <= snippetSpanWholeChunkRunes {
		t.Fatalf("test setup: chunk %d runes is under the cap", len([]rune(huge)))
	}
	if !strings.Contains(snippet, "Bronner opens") {
		t.Fatalf("fragment must keep the first hit cluster, got %.60q", snippet)
	}
	if n := len([]rune(snippet)); n > snippetMaxRunes+6 {
		t.Fatalf("fragment over budget: %d runes", n)
	}
}

// TestPreviewForChunk covers the "surfaced but never read" gap: a lexical hit
// whose surface term is absent still ships its own opening text.
func TestPreviewForChunk(t *testing.T) {
	short := "A short chunk body."
	if got, truncated := previewForChunk(short); got != short || truncated {
		t.Fatalf("short preview = (%q, %v), want the whole body", got, truncated)
	}
	long := strings.Repeat("mineralization research continued. ", 100)
	got, truncated := previewForChunk(long)
	if !truncated {
		t.Error("a long preview must be marked truncated")
	}
	if n := len([]rune(got)); n > snippetMaxRunes+3 {
		t.Fatalf("preview over budget: %d runes", n)
	}
	if !strings.HasPrefix(got, "mineralization research") {
		t.Fatalf("preview must be the chunk opening, got %.60q", got)
	}
}

// TestFormatLocateResultsXMLMarksFragments asserts the model-visible contract:
// partial snippets are labelled, and the results carry the deep-read nudge.
func TestFormatLocateResultsXMLMarksFragments(t *testing.T) {
	hits := []snippetHit{
		{chunk: runtime.RetrievalChunk{ID: "c1", Content: strings.Repeat("x ", 900)}, snippet: "...window...", truncated: true},
		{chunk: runtime.RetrievalChunk{ID: "c2", Content: strings.Repeat("y ", 900)}, snippet: "opening", preview: true},
		{chunk: runtime.RetrievalChunk{ID: "c3", Content: "short body", DocumentID: "d3"}, snippet: "short body"},
	}
	out := formatLocateResultsXML(context.Background(), "grep_chunks", "q", hits)
	if !strings.Contains(out, `chunk_id="c1"`) || !strings.Contains(out, `truncated="true"`) {
		t.Errorf("truncated hit must carry the attribute: %.200q", out)
	}
	if !strings.Contains(out, `match="none"`) {
		t.Errorf("preview hit must carry match=none: %.200q", out)
	}
	if !strings.Contains(out, "<snippet_note>") || !strings.Contains(out, "list_chunks") {
		t.Errorf("results must explain how to read the rest: %.300q", out)
	}
	if !strings.Contains(out, `chunk_runes="1800"`) {
		t.Errorf("chunk size must be visible: %.200q", out)
	}
	// A results set with no partial snippet stays clean.
	plain := formatLocateResultsXML(context.Background(), "grep_chunks", "q", hits[2:])
	if strings.Contains(plain, "<snippet_note>") {
		t.Errorf("no note expected when every snippet is complete: %.200q", plain)
	}
}
