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

package kg

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/service"
)

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
				es.Sim += ent.Similarity / (2.0 + float64(i))
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
			entsFromQuery[ent].Similarity *= 2
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
			Score:       ent.Similarity * ent.PageRank,
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
// Delegates to the shared implementation in the parent service package.
func NumTokensFromString(s string) int {
	return service.NumTokensFromString(s)
}

// formatCSVLine formats fields as a single CSV record with trailing newline.
// Handles commas, quotes, and newlines in field values correctly — unlike fmt.Sprintf.
// Matches Python: pd.DataFrame(...).to_csv() quoting behavior.
func formatCSVLine(fields ...string) string {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write(fields)
	w.Flush()
	return buf.String()
}

// FilterChunksByScore filters chunks where _score >= threshold.
// Chunks missing _score are treated as score=0.
// Pure function — no I/O, no external dependencies.
// Matches Python: _ent_info_from_ and _relation_info_from_ sim_thr filtering.
func FilterChunksByScore(chunks []map[string]interface{}, threshold float64) []map[string]interface{} {
	if threshold <= 0 || len(chunks) == 0 {
		return chunks
	}
	result := make([]map[string]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		score := 0.0
		if v, ok := chunk["_score"].(float64); ok {
			score = v
		} else if v, ok := chunk["score"].(float64); ok {
			score = v
		}
		if score >= threshold {
			result = append(result, chunk)
		}
	}
	return result
}

// FormatEntitiesToCSV formats scored entities as a CSV string and tracks token count.
func FormatEntitiesToCSV(entities []ScoredEntity, maxToken int) (csv string, remainingToken int) {
	if len(entities) == 0 {
		return "", maxToken
	}
	var b strings.Builder
	b.WriteString("---- Entities ----\n")
	b.WriteString("Entity,Score,Description\n")
	for _, ent := range entities {
		desc := extractDescription(ent.Description)
		line := formatCSVLine(ent.Entity, fmt.Sprintf("%.2f", ent.Score), desc)
		tokens := NumTokensFromString(line)
		if maxToken-tokens <= 0 {
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
	for _, rel := range relations {
		desc := extractDescription(rel.Description)
		line := formatCSVLine(rel.From, rel.To, fmt.Sprintf("%.2f", rel.Score), desc)
		tokens := NumTokensFromString(line)
		if maxToken-tokens <= 0 {
			break
		}
		b.WriteString(line)
		maxToken -= tokens
	}
	return b.String(), maxToken
}

// BuildContent assembles the final knowledge graph content string.
// Python equivalent: lines 267-291
func BuildContent(
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
	// Try to parse as JSON and extract the "description" field.
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(desc), &data); err == nil {
		if v, ok := data["description"]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return desc
}
