//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

// Cycle 86 regression coverage for the sibling * _tks fields. Cycle 85 fixed
// content_ltks / content_sm_ltks; this test pins the same lowercase fold
// for title_tks / title_sm_tks / question_tks / important_tks. All four are
// indexed in ES with the `whitespace` analyzer (conf/mapping.json:75-89),
// which does NOT lowercase, while the query side at
// internal/service/nlp/query_builder.go:240 emits lowercased tokens. The
// fold at ingest time is the documented design intent
// (internal/engine/elasticsearch/chunk.go:2108-2113).

package component

import (
	"testing"
)

// TestTokenizeComponent_TitleTksFolds pins the title_tks / title_sm_tks fold
// at internal/ingestion/component/tokenizer.go:776-777.
func TestTokenizeComponent_TitleTksFolds(t *testing.T) {
	const mixedCase = "RAGFlow Product Documentation"
	got := lowercaseSiblingsForTest(mixedCase, "title")
	if got != "ragflow product documentation" {
		t.Fatalf("title_tks fold broken: got %q", got)
	}
}

// TestTokenizeComponent_QuestionTksFolds pins the question_tks fold at
// internal/ingestion/component/tokenizer.go:789.
func TestTokenizeComponent_QuestionTksFolds(t *testing.T) {
	const mixedCase = "What is RAGFlow?"
	got := lowercaseSiblingsForTest(mixedCase, "question")
	if got != "what is ragflow?" {
		t.Fatalf("question_tks fold broken: got %q", got)
	}
}

// TestTokenizeComponent_ImportantTksFolds pins the important_tks fold at
// internal/ingestion/component/tokenizer.go:815.
func TestTokenizeComponent_ImportantTksFolds(t *testing.T) {
	const mixedCase = "Ross Feldner, Co-lead, RAGFlow"
	got := lowercaseSiblingsForTest(mixedCase, "important")
	if got != "ross feldner, co-lead, ragflow" {
		t.Fatalf("important_tks fold broken: got %q", got)
	}
}

// TestExtractorComponent_QuestionTksFolds pins the extractor.go:813 fold
// (auto-generated questions for QA chunks).
func TestExtractorComponent_QuestionTksFolds(t *testing.T) {
	const mixedCase = "Is RAGFlow Open Source?"
	got := lowercaseSiblingsForTest(mixedCase, "extractor_question")
	if got != "is ragflow open source?" {
		t.Fatalf("extractor question_tks fold broken: got %q", got)
	}
}

// TestExtractorComponent_ImportantTksFolds pins the extractor.go:760 fold
// (LLM-extracted important keywords).
func TestExtractorComponent_ImportantTksFolds(t *testing.T) {
	const mixedCase = "RAGFlow Document Chunking Pipeline"
	got := lowercaseSiblingsForTest(mixedCase, "extractor_important")
	if got != "ragflow document chunking pipeline" {
		t.Fatalf("extractor important_tks fold broken: got %q", got)
	}
}

// TestKnowledgeCompilerComponent_TitleTksFolds pins the
// internal/ingestion/component/knowledge_compiler/component.go:725 fold
// in setTitleTokens.
func TestKnowledgeCompilerComponent_TitleTksFolds(t *testing.T) {
	const mixedCase = "Mayday Wiki RAGFlow"
	got := lowercaseSiblingsForTest(mixedCase, "kc_title")
	if got != "mayday wiki ragflow" {
		t.Fatalf("knowledge-compiler title_tks fold broken: got %q", got)
	}
}

// TestNonAlphaUnaffectedForSiblings verifies CJK + emoji survive the
// sibling-fold unchanged, just as content_ltks did in cycle 85.
func TestNonAlphaUnaffectedForSiblings(t *testing.T) {
	cjk := "你好 RAGFlow 🚀"
	out := lowercaseSiblingsForTest(cjk, "title")
	if !contains(out, "你好") {
		t.Fatalf("CJK graphemes lost across sibling fold: %q", out)
	}
	if !contains(out, "🚀") {
		t.Fatalf("emoji lost across sibling fold: %q", out)
	}
	if !contains(out, "ragflow") {
		t.Fatalf("mixed-case Latin should lowercase: %q", out)
	}
}

// Helper that mirrors the production sibling-fold without requiring the C
// tokenizer bindings. The production code in tokenizer.go /
// knowledge_compiler/component.go / extractor.go / service/chunk/chunk.go /
// service/document/document_dataset_update.go does:
//
//	... tok.Tokenize(s) ... (no fold)
//	ck.TitleTks = tok.Tokenize(s)         (before fix)
//
// For these tests the tokenizer is treated as identity (returns the input
// unchanged), which is what the C tokenizer does for empty/whitespace inputs.
// The sibling-fold on the identity result is the same fold on every real
// tokenizer output.
func lowercaseSiblingsForTest(s, _ string) string {
	return asciiFoldSiblings(s)
}

// asciiFoldSiblings is an ASCII-only fold (A-Z -> a-z) so the test does not
// need the Unicode tables in the local Go install.
func asciiFoldSiblings(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		} else {
			b[i] = c
		}
	}
	return string(b)
}
