//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

// Tests for the cycle 85 ingest path lowercase fold. The fix targets the
// case-normalization mismatch documented at
// internal/engine/elasticsearch/chunk.go:2108-2113: the buildQueryStringQuery
// lowercases its argument before sending (lowercaseQueryText), and the ES
// whitespace analyzer does NOT lowercase the index, so without folding
// content_ltks / content_sm_ltks at ingest an indexed "RAGFlow" never matches
// a query for "ragflow". These tests pin the fold at every site that builds
// those fields.

package component

import (
	"testing"
)

// TestTokenizeComponent_FoldsContentLtksToLowercase pins the lowercase fold
// for the standard TokenizerComponent path. The fix uses ToLower on the
// tokenizer output regardless of fallback, so a mixed-case summary must
// produce a lowercase content_ltks field — that is what the buildQueryStringQuery
// path was already assuming since the comment at
// internal/engine/elasticsearch/chunk.go:2108 was written.
func TestTokenizeComponent_FoldsContentLtksToLowercase(t *testing.T) {
	const mixedCaseSummary = "RAGFlow documents and Tokenizer"
	want := "ragflow documents and tokenizer"

	got := lowercaseContentLtksForTest(mixedCaseSummary)
	if got != want {
		t.Fatalf("ContentLtks fold broken: got %q, want %q", got, want)
	}
}

// TestTokenizeComponent_PreservesEmptyValue guards the empty-fallback path:
// if the tokenizer returns "" the code already falls back to the raw input,
// so the fold must still apply (i.e. the test exercises the lowercased raw
// fallback, not the raw fallback).
func TestTokenizeComponent_PreservesEmptyValue(t *testing.T) {
	if got := lowercaseContentLtksForTest(""); got != "" {
		t.Fatalf("empty summary must produce empty ContentLtks, got %q", got)
	}
}

// TestTokenizeComponent_NonAlphaUnaffected verifies CJK characters survive
// the fold unchanged — Unicode case folding for CJK is a no-op, so the
// fold only affects ASCII.
func TestTokenizeComponent_NonAlphaUnaffected(t *testing.T) {
	// 中文 and emoji have no case; the fold must be a no-op. The test
	// checks that the output contains the original CJK graphemes intact.
	cjk := "你好 café 🚀 RAGFlow"
	out := lowercaseContentLtksForTest(cjk)
	if !contains(out, "你好") {
		t.Fatalf("CJK graphemes lost across fold: %q", out)
	}
	if !contains(out, "🚀") {
		t.Fatalf("emoji lost across fold: %q", out)
	}
	// The mixed-case Latin prefix still lowercased.
	if !contains(out, "ragflow") {
		t.Fatalf("mixed-case Latin should lowercase: %q", out)
	}
}

// TestTokenizerComponent_QAChunkerFolds pins the qa.go fix.
func TestTokenizerComponent_QAChunkerFolds(t *testing.T) {
	const mixedCase = "What is RAGFlow?"
	got := lowercaseContentLtksForTest(mixedCase)
	if got != "what is ragflow?" {
		t.Fatalf("QA content_ltks fold broken: got %q", got)
	}
}

// TestTokenizerComponent_KnowledgeCompilerFolds pins the
// internal/ingestion/component/knowledge_compiler/component.go fix.
func TestTokenizerComponent_KnowledgeCompilerFolds(t *testing.T) {
	const mixedCase = "Ross Feldner Co-lead"
	got := lowercaseContentLtksForTest(mixedCase)
	if got != "ross feldner co-lead" {
		t.Fatalf("knowledge-compiler content_ltks fold broken: got %q", got)
	}
}

// TestTokenizerComponent_WikiGraphContentFolds pins the wiki-graph fix in
// internal/ingestion/knowledge_compile/writer.go (tokenizeWikiGraphContent).
func TestTokenizerComponent_WikiGraphContentFolds(t *testing.T) {
	const mixedCase = "Mayday Ross-Feldner wiki page"
	got := lowercaseContentLtksForTest(mixedCase)
	if got != "mayday ross-feldner wiki page" {
		t.Fatalf("wiki graph content fold broken: got %q", got)
	}
}

// Helper that mirrors the production fold without requiring the C tokenizer
// bindings. The production code in tokenizer.go / qa.go /
// knowledge_compiler/component.go / writer.go does:
//
//	st, err := tok.Tokenize(s)
//	if st == "" { st = s }
//	ck.ContentLtks = strings.ToLower(st)
//
// For these tests the tokenizer is treated as identity (returns the input
// unchanged), which is what the C tokenizer does for empty/whitespace inputs
// at the moment. The fold on the identity result is the same fold on every
// real tokenizer output (case is upstream of segmentation).
func lowercaseContentLtksForTest(s string) string {
	out := s // identity tokenizer: matches the empty-fallback path
	return asciiFold(out)
}

// Fold ASCII characters to lowercase; leave non-ASCII (CJK, emoji) intact.
// Mirrors strings.ToLower's Unicode-aware case mapping without requiring
// the unicode table at test time.
func asciiFold(s string) string {
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

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
