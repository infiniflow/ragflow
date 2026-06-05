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
