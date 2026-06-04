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

package common

import (
	"fmt"
	"sort"
	"strings"
)

// KGEntity represents a knowledge graph entity with its scores.
type KGEntity struct {
	Sim         float64
	PageRank    float64
	Description string
	NhopEnts    []NhopEntity
}

// NhopEntity represents an N-hop neighbor path.
type NhopEntity struct {
	Path    []string  // entity names along the path
	Weights []float64 // pagerank weights per hop
}

// KGRelation represents a knowledge graph relation with its scores.
type KGRelation struct {
	Sim         float64
	PageRank    float64
	Description string
}

// Edge represents a directed (from_entity, to_entity) pair.
type Edge struct {
	From, To string
}

// EdgeScore represents the accumulated score for an edge from N-hop analysis.
type EdgeScore struct {
	Sim      float64
	PageRank float64
}

// ScoredEntity is a scored entity ready for output.
type ScoredEntity struct {
	Entity      string
	Score       float64
	Description string
}

// ScoredRelation is a scored relation ready for output.
type ScoredRelation struct {
	From        string
	To          string
	Score       float64
	Description string
}

// AnalyzeNHopPaths decomposes N-hop paths into edges with distance-decayed scores.
// Python equivalent: rag/graphrag/search.py lines 172-187
func AnalyzeNHopPaths(entsFromQuery map[string]*KGEntity) map[Edge]EdgeScore {
	nhopPathes := make(map[Edge]EdgeScore)
	for _, ent := range entsFromQuery {
		for _, nbr := range ent.NhopEnts {
			path := nbr.Path
			weights := nbr.Weights
			for i := 0; i < len(path)-1; i++ {
				f, t := path[i], path[i+1]
				edge := Edge{From: f, To: t}
				es := nhopPathes[edge]
				es.Sim += ent.Sim / (2.0 + float64(i))
				if i < len(weights) {
					es.PageRank = weights[i]
				}
				nhopPathes[edge] = es
			}
		}
	}
	return nhopPathes
}

// DoubleHitBoost doubles the similarity of entities found in both
// keyword search and type search. Python equivalent: lines 194-198
func DoubleHitBoost(entsFromQuery map[string]*KGEntity, entsFromTypes map[string]struct{}) {
	for ent := range entsFromQuery {
		if _, ok := entsFromTypes[ent]; ok {
			entsFromQuery[ent].Sim *= 2
		}
	}
}

// FuseRelationScores integrates N-hop contributions and type boosts
// into relation scores. New edges from N-hop are added as relations.
// Python equivalent: lines 200-222
func FuseRelationScores(
	relsFromText map[Edge]*KGRelation,
	entsFromTypes map[string]struct{},
	nhopPathes map[Edge]EdgeScore,
) {
	// Boost existing relations with N-hop and type scores
	for edge, rel := range relsFromText {
		s := 0.0
		if np, ok := nhopPathes[edge]; ok {
			s += np.Sim
			delete(nhopPathes, edge)
		}
		if _, ok := entsFromTypes[edge.From]; ok {
			s += 1
		}
		if _, ok := entsFromTypes[edge.To]; ok {
			s += 1
		}
		rel.Sim *= s + 1
	}

	// N-hop discovered edges become new relations
	for edge, np := range nhopPathes {
		s := 0.0
		if _, ok := entsFromTypes[edge.From]; ok {
			s += 1
		}
		if _, ok := entsFromTypes[edge.To]; ok {
			s += 1
		}
		relsFromText[edge] = &KGRelation{
			Sim:      np.Sim * (s + 1),
			PageRank: np.PageRank,
		}
	}
}

// SortAndTrimEntities sorts entities by sim*pagerank and takes top N.
// Python equivalent: lines 224-225
func SortAndTrimEntities(entsFromQuery map[string]*KGEntity, topN int) []ScoredEntity {
	if topN <= 0 {
		topN = 6
	}
	var scored []ScoredEntity
	for name, ent := range entsFromQuery {
		scored = append(scored, ScoredEntity{
			Entity:      name,
			Score:       ent.Sim * ent.PageRank,
			Description: ent.Description,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > topN {
		scored = scored[:topN]
	}
	return scored
}

// SortAndTrimRelations sorts relations by sim*pagerank and takes top N.
// Python equivalent: lines 226-227
func SortAndTrimRelations(relsFromText map[Edge]*KGRelation, topN int) []ScoredRelation {
	if topN <= 0 {
		topN = 6
	}
	var scored []ScoredRelation
	for edge, rel := range relsFromText {
		scored = append(scored, ScoredRelation{
			From:        edge.From,
			To:          edge.To,
			Score:       rel.Sim * rel.PageRank,
			Description: rel.Description,
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > topN {
		scored = scored[:topN]
	}
	return scored
}

// NumTokensFromString estimates the number of tokens in a string.
// Uses a simple approximation: len/4 characters per token (roughly matching cl100k_base).
func NumTokensFromString(s string) int {
	return len(s) / 4
}

// FormatEntitiesToCSV formats scored entities as a CSV string and tracks token count.
func FormatEntitiesToCSV(entities []ScoredEntity, maxToken int) (csv string, remainingToken int) {
	if len(entities) == 0 {
		return "", maxToken
	}
	var b strings.Builder
	b.WriteString("---- Entities ----\n")
	b.WriteString("Entity,Score,Description\n")
	for i, ent := range entities {
		desc := extractDescription(ent.Description)
		line := fmt.Sprintf("%s,%.2f,%s\n", ent.Entity, ent.Score, desc)
		tokens := NumTokensFromString(line)
		if maxToken-tokens <= 0 {
			entities = entities[:i]
			break
		}
		b.WriteString(line)
		maxToken -= tokens
	}
	return b.String(), maxToken
}

// FormatRelationsToCSV formats scored relations as a CSV string and tracks token count.
func FormatRelationsToCSV(relations []ScoredRelation, maxToken int) (csv string, remainingToken int) {
	if len(relations) == 0 {
		return "", maxToken
	}
	var b strings.Builder
	b.WriteString("---- Relations ----\n")
	b.WriteString("From Entity,To Entity,Score,Description\n")
	for i, rel := range relations {
		desc := extractDescription(rel.Description)
		line := fmt.Sprintf("%s,%s,%.2f,%s\n", rel.From, rel.To, rel.Score, desc)
		tokens := NumTokensFromString(line)
		if maxToken-tokens <= 0 {
			relations = relations[:i]
			break
		}
		b.WriteString(line)
		maxToken -= tokens
	}
	return b.String(), maxToken
}

// BuildKGContent assembles the final knowledge graph content string.
// Python equivalent: lines 267-291
func BuildKGContent(
	entities []ScoredEntity,
	relations []ScoredRelation,
	maxToken int,
) string {
	entityCSV, remaining := FormatEntitiesToCSV(entities, maxToken)
	relCSV, _ := FormatRelationsToCSV(relations, remaining)
	return entityCSV + relCSV
}

// extractDescription tries to parse a description from a JSON-like string.
// Python equivalent: json.loads(desc).get("description", "")
func extractDescription(desc string) string {
	if desc == "" {
		return ""
	}
	// If the description looks like JSON, try to extract the "description" field
	desc = strings.TrimSpace(desc)
	if strings.HasPrefix(desc, "{") && strings.HasSuffix(desc, "}") {
		// Simple extraction: find "description" key value
		// This matches Python's json.loads(desc).get("description", "") behavior
		idx := strings.Index(desc, `"description"`)
		if idx >= 0 {
			remain := desc[idx+len(`"description"`):]
			colonIdx := strings.Index(remain, ":")
			if colonIdx >= 0 {
				valPart := strings.TrimSpace(remain[colonIdx+1:])
				if strings.HasPrefix(valPart, `"`) {
					valPart = strings.TrimPrefix(valPart, `"`)
					endQuote := strings.Index(valPart, `"`)
					if endQuote >= 0 {
						return valPart[:endQuote]
					}
				}
			}
		}
	}
	return desc
}
