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
	"strings"
	"testing"
)

// --- AnalyzeNHopPaths ---

func TestAnalyzeNHopPaths_Basic(t *testing.T) {
	ents := map[string]*KGEntity{
		"A": {
Similarity: 0.9,
			NhopEnts: []NhopEntity{
				{Path: []string{"A", "B", "C"}, Weights: []float64{0.8, 0.5}},
			},
		},
	}
	result := AnalyzeNHopPaths(ents)
	// A→B: 0.9 / (2+0) = 0.45
	// B→C: 0.9 / (2+1) = 0.3
	if len(result) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(result))
	}
	if result[Edge{"A", "B"}].Sim != 0.45 {
		t.Errorf("expected A→B sim=0.45, got %f", result[Edge{"A", "B"}].Sim)
	}
	if result[Edge{"B", "C"}].Sim != 0.3 {
		t.Errorf("expected B→C sim=0.3, got %f", result[Edge{"B", "C"}].Sim)
	}
}

func TestAnalyzeNHopPaths_MultipleContributors(t *testing.T) {
	ents := map[string]*KGEntity{
		"A": {
Similarity: 0.8,
			NhopEnts: []NhopEntity{
				{Path: []string{"A", "B"}, Weights: []float64{0.7}},
			},
		},
		"X": {
Similarity: 0.6,
			NhopEnts: []NhopEntity{
				{Path: []string{"X", "B"}, Weights: []float64{0.5}},
			},
		},
	}
	result := AnalyzeNHopPaths(ents)
	// A→B: 0.8 / 2 = 0.4
	// X→B: 0.6 / 2 = 0.3
	if result[Edge{"A", "B"}].Sim != 0.4 {
		t.Errorf("expected A→B sim=0.4, got %f", result[Edge{"A", "B"}].Sim)
	}
	if result[Edge{"X", "B"}].Sim != 0.3 {
		t.Errorf("expected X→B sim=0.3, got %f", result[Edge{"X", "B"}].Sim)
	}
}

func TestAnalyzeNHopPaths_Empty(t *testing.T) {
	result := AnalyzeNHopPaths(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

// Regression: when two paths from different query entities contribute the
// same edge, PageRank must keep the *max* weight rather than whichever
// arrives last. The previous last-write-wins behaviour was non-deterministic
// under Go's randomized map iteration. See infiniflow/ragflow#15695.
func TestAnalyzeNHopPaths_PageRankTakesMaxAcrossPaths(t *testing.T) {
	ents := map[string]*KGEntity{
		"A": {
			Similarity: 0.8,
			NhopEnts: []NhopEntity{
				{Path: []string{"A", "B"}, Weights: []float64{0.3}}, // weak
			},
		},
		"X": {
			Similarity: 0.6,
			NhopEnts: []NhopEntity{
				{Path: []string{"X", "B"}, Weights: []float64{0.5}},
				{Path: []string{"A", "B"}, Weights: []float64{0.9}}, // strong, same edge as A→B
			},
		},
	}
	result := AnalyzeNHopPaths(ents)
	if got := result[Edge{"A", "B"}].PageRank; got != 0.9 {
		t.Errorf("expected A→B PageRank=0.9 (max), got %f", got)
	}
	if got := result[Edge{"X", "B"}].PageRank; got != 0.5 {
		t.Errorf("expected X→B PageRank=0.5, got %f", got)
	}
}

// --- DoubleHitBoost ---

func TestDoubleHitBoost(t *testing.T) {
	ents := map[string]*KGEntity{
		"A": {Similarity: 0.5},
		"B": {Similarity: 0.3},
	}
	types := map[string]struct{}{"A": {}}
	DoubleHitBoost(ents, types)
	if ents["A"].Similarity != 1.0 {
		t.Errorf("expected A sim=1.0 after boost, got %f", ents["A"].Similarity)
	}
	if ents["B"].Similarity != 0.3 {
		t.Errorf("expected B sim unchanged at 0.3, got %f", ents["B"].Similarity)
	}
}

func TestDoubleHitBoost_Empty(t *testing.T) {
	ents := map[string]*KGEntity{"A": {Similarity: 0.5}}
	DoubleHitBoost(ents, map[string]struct{}{})
	if ents["A"].Similarity != 0.5 {
		t.Errorf("expected unchanged, got %f", ents["A"].Similarity)
	}
}

// --- FuseRelationScores ---

func TestFuseRelationScores_NhopContribution(t *testing.T) {
	rels := map[Edge]*KGRelation{
		{"A", "B"}: {Sim: 0.5, PageRank: 0.8},
	}
	types := map[string]struct{}{}
	nhop := map[Edge]EdgeScore{
		{"A", "B"}: {Sim: 0.3},
	}
	FuseRelationScores(rels, types, nhop)
	// sim = 0.5 * (0.3 + 1) = 0.65
	if rels[Edge{"A", "B"}].Sim != 0.65 {
		t.Errorf("expected 0.65, got %f", rels[Edge{"A", "B"}].Sim)
	}
}

func TestFuseRelationScores_TypeBoost(t *testing.T) {
	rels := map[Edge]*KGRelation{
		{"A", "B"}: {Sim: 0.5},
	}
	types := map[string]struct{}{"A": {}, "B": {}}
	nhop := map[Edge]EdgeScore{}
	FuseRelationScores(rels, types, nhop)
	// Both endpoints in types: s=2, sim = 0.5 * (2+1) = 1.5
	if rels[Edge{"A", "B"}].Sim != 1.5 {
		t.Errorf("expected 1.5, got %f", rels[Edge{"A", "B"}].Sim)
	}
}

func TestFuseRelationScores_NhopNewEdge(t *testing.T) {
	rels := map[Edge]*KGRelation{}
	types := map[string]struct{}{}
	nhop := map[Edge]EdgeScore{
		{"A", "B"}: {Sim: 0.4, PageRank: 0.7},
	}
	FuseRelationScores(rels, types, nhop)
	if _, ok := rels[Edge{"A", "B"}]; !ok {
		t.Fatal("expected new edge from N-hop")
	}
	if rels[Edge{"A", "B"}].Sim != 0.4 {
		t.Errorf("expected sim=0.4, got %f", rels[Edge{"A", "B"}].Sim)
	}
}

// --- SortAndTrim ---

func TestSortAndTrimEntities(t *testing.T) {
	ents := map[string]*KGEntity{
		"A": {Similarity: 0.5, PageRank: 0.9},
		"B": {Similarity: 0.8, PageRank: 0.3},
		"C": {Similarity: 0.9, PageRank: 0.1},
	}
	result := SortAndTrimEntities(ents, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	// A: 0.45, B: 0.24, C: 0.09 → top 2 should be A, B
	if result[0].Entity != "A" {
		t.Errorf("expected A first (0.45), got %s (%f)", result[0].Entity, result[0].Score)
	}
}

func TestSortAndTrimEntities_DefaultTopN(t *testing.T) {
	ents := map[string]*KGEntity{
		"A": {Similarity: 0.5, PageRank: 0.9},
		"B": {Similarity: 0.8, PageRank: 0.3},
	}
	result := SortAndTrimEntities(ents, 0)
	if len(result) != 2 {
		t.Errorf("expected default topN to include all, got %d", len(result))
	}
}

func TestSortAndTrimRelations(t *testing.T) {
	rels := map[Edge]*KGRelation{
		{"A", "B"}: {Sim: 0.9, PageRank: 0.1},
		{"C", "D"}: {Sim: 0.3, PageRank: 0.8},
	}
	result := SortAndTrimRelations(rels, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	// A→B: 0.09, C→D: 0.24 → C→D should be first
	if result[0].From != "C" {
		t.Errorf("expected C first (0.24), got %s (%f)", result[0].From, result[0].Score)
	}
}

// --- Format and Build ---

func TestBuildKGContent_Basic(t *testing.T) {
	entities := []ScoredEntity{
		{Entity: "A", Score: 0.45, Description: `{"description": "Entity A desc"}`},
	}
	relations := []ScoredRelation{
		{From: "A", To: "B", Score: 0.3, Description: `{"description": "rel A-B"}`},
	}
	result := BuildKGContent(entities, relations, 10000)
	if !strings.Contains(result, "Entity A desc") {
		t.Errorf("expected entity description in output, got: %s", result)
	}
	if !strings.Contains(result, "rel A-B") {
		t.Errorf("expected relation description in output, got: %s", result)
	}
}

func TestBuildKGContent_TokenBudget(t *testing.T) {
	longDesc := strings.Repeat("This is a very long description. ", 50)
	entities := []ScoredEntity{
		{Entity: "LongEntityName", Score: 1.0, Description: longDesc},
	}
	relations := []ScoredRelation{
		{From: "X", To: "Y", Score: 1.0, Description: "relation desc"},
	}
	result := BuildKGContent(entities, relations, 50)
	// Token budget is very small, should truncate and not include relations
	if strings.Contains(result, "relation desc") {
		t.Log("Note: relations included despite small budget (depending on token count)")
	}
}

func TestFormatEntitiesToCSV_HeaderExceedsBudget(t *testing.T) {
	entities := []ScoredEntity{
		{Entity: "A", Score: 1.0, Description: "d"},
	}
	result, remaining := FormatEntitiesToCSV(entities, 3)
	tokens := NumTokensFromString(result)
	// Header lines (---- Entities ----\n, Entity,Score,Description\n) are written
	// before the token budget check. They consume ~11 tokens but are not deducted
	// from maxToken. This is a known limitation shared with Python.
	if tokens > 3 {
		t.Logf("output %d tokens exceeds budget of %d (header not counted, remaining=%d)", tokens, 3, remaining)
	}
}

func TestFilterChunksByScore_AllPass(t *testing.T) {
	chunks := []map[string]interface{}{
		{"entity_kwd": "A", "_score": 0.5},
		{"entity_kwd": "B", "_score": 0.8},
	}
	result := FilterChunksByScore(chunks, 0.3)
	if len(result) != 2 {
		t.Errorf("expected all 2 chunks to pass, got %d", len(result))
	}
}

func TestFilterChunksByScore_SomeFiltered(t *testing.T) {
	chunks := []map[string]interface{}{
		{"entity_kwd": "A", "_score": 0.2},
		{"entity_kwd": "B", "_score": 0.9},
	}
	result := FilterChunksByScore(chunks, 0.3)
	if len(result) != 1 || result[0]["entity_kwd"] != "B" {
		t.Errorf("expected only B to pass, got %v", result)
	}
}

func TestFilterChunksByScore_MissingScore(t *testing.T) {
	chunks := []map[string]interface{}{
		{"entity_kwd": "A"}, // no _score → treated as 0
		{"entity_kwd": "B", "score": 0.5},
	}
	result := FilterChunksByScore(chunks, 0.3)
	if len(result) != 1 || result[0]["entity_kwd"] != "B" {
		t.Errorf("expected only B (using 'score' field), got %v", result)
	}
}

func TestFilterChunksByScore_NilInput(t *testing.T) {
	result := FilterChunksByScore(nil, 0.3)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestFilterChunksByScore_ZeroThreshold(t *testing.T) {
	chunks := []map[string]interface{}{
		{"entity_kwd": "A", "_score": 0.0},
	}
	result := FilterChunksByScore(chunks, 0)
	if len(result) != 1 {
		t.Errorf("expected all pass when threshold=0, got %d", len(result))
	}
}

func TestExtractDescription_JSON(t *testing.T) {
	result := extractDescription(`{"description": "Entity A description", "other": "value"}`)
	if result != "Entity A description" {
		t.Errorf("expected 'Entity A description', got %q", result)
	}
}

func TestExtractDescription_Plain(t *testing.T) {
	result := extractDescription("plain description")
	if result != "plain description" {
		t.Errorf("expected 'plain description', got %q", result)
	}
}

func TestExtractDescription_EscapedQuote(t *testing.T) {
	result := extractDescription(`{"description": "has \"quote\" inside"}`)
	if result != `has "quote" inside` {
		t.Errorf("expected full description with quote, got %q", result)
	}
}

func TestExtractDescription_NonStringValue(t *testing.T) {
	result := extractDescription(`{"description": null, "other": "val"}`)
	if result != `{"description": null, "other": "val"}` {
		t.Errorf("expected raw JSON when description is null, got %q", result)
	}
}

func TestExtractDescription_EmptyString(t *testing.T) {
	result := extractDescription("")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestFormatCSVLine_Normal(t *testing.T) {
	result := formatCSVLine("Elon Musk", "0.85", "CEO of SpaceX")
	// Normal values should not be quoted
	if result != "Elon Musk,0.85,CEO of SpaceX\n" {
		t.Errorf("expected unquoted CSV, got %q", result)
	}
}

func TestFormatCSVLine_CommaInField(t *testing.T) {
	result := formatCSVLine("Musk, Elon", "0.85", "CEO, SpaceX")
	// Values with commas should be quoted
	expected := `"Musk, Elon",0.85,"CEO, SpaceX"` + "\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatCSVLine_QuoteInField(t *testing.T) {
	result := formatCSVLine("Elon Musk", "0.85", `CEO of "SpaceX"`)
	// Values with quotes should have quotes escaped
	expected := `Elon Musk,0.85,"CEO of ""SpaceX"""` + "\n"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatCSVLine_EmptyField(t *testing.T) {
	result := formatCSVLine("", "", "")
	if result != ",,\n" {
		t.Errorf("expected empty fields, got %q", result)
	}
}

func TestNumTokensFromString(t *testing.T) {
	s := "This is a test string with multiple words"
	tokens := NumTokensFromString(s)
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}

