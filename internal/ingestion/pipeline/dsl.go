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

// Package pipeline — Phase 3 of port-rag-flow-pipeline-to-go.md.
//
// The pipeline package owns the DAG runner that drives the
// internal/ingestion/component implementations end-to-end. It
// replaces the placeholder 5-second sleep loop in
// internal/ingestion/ingestion_service.go (`Ingestor.executeTask`)
// with a real, checkpointed, resumable run.
//
// This file (dsl.go) defines the on-the-wire JSON shape the
// pipeline executor consumes. The Go DSL is a deliberate redesign
// of the Python rag/flow/pipeline.py DSL — the Python code never
// had a structured JSON schema; it constructed a Graph from an
// ad-hoc string and used a `total_step: 5` placeholder in the
// Go Ingestor as a stand-in for the five-stage flow. The Go
// pipeline encodes the DAG explicitly:
//
//	version     schema revision; "1" today.
//	name        human-readable name; informational only.
//	description informational only.
//	stage_count replaces the Python `total_step` placeholder.
//	stages[]    ordered list of component invocations. Today the
//	           runner executes stages in declaration order (the
//	           ingestion flow is linear; branches/loops are an
//	           agent-canvas concern, not an ingestion concern).
//
// Each stage's `type` is the registered component name (one of
// `File | Parser | Chunker | Tokenizer | Extractor` for the
// production flow; the registry accepts any CategoryIngestion
// component). The `params` map is the same `map[string]any` the
// component's New<Name>Component factory accepts.
//
// SCHEMA DELTA vs. Python (deferred to a §11 follow-up migration
// tool): the Python DSL is currently a runtime-generated string
// that the Ingestor never actually consumes — the Ingestor only
// references `total_step: 5` as a sleep-loop counter. The Go
// pipeline is the first structured encoding of the flow; a
// one-shot migrator from the python `Pipeline.__init__` arguments
// to this Go DSL is a follow-up (plan §4 Phase 3 task 0b).
package pipeline

// PipelineDSL is the JSON shape consumed by NewPipelineFromDSL.
// The on-the-wire form is intentionally flat: linear order, one
// stage per component, no explicit edges. Branching / loops are
// the agent canvas's job (plan §2 AD-2).
type PipelineDSL struct {
	Version     string     `json:"version"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	StageCount  int        `json:"stage_count"`
	Stages      []StageDSL `json:"stages"`
}

// StageDSL is one stage in the pipeline. The Type must match a
// component registered under runtime.CategoryIngestion; the
// Params are passed verbatim to that component's factory.
type StageDSL struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
}

// IsValid returns a structured error if the DSL is malformed.
// The runner calls this once at NewPipelineFromDSL time so a
// malformed DSL fails fast before any side effects.
func (d *PipelineDSL) IsValid() error {
	if d == nil {
		return errNilDSL
	}
	if d.StageCount > 0 && d.StageCount != len(d.Stages) {
		return errStageCountMismatch
	}
	if len(d.Stages) == 0 {
		return errEmptyStages
	}
	seen := make(map[string]struct{}, len(d.Stages))
	for i := range d.Stages {
		s := &d.Stages[i]
		if s.Type == "" {
			return errEmptyStageType(i)
		}
		if _, dup := seen[s.Type]; dup {
			return errDuplicateStage(s.Type)
		}
		seen[s.Type] = struct{}{}
	}
	return nil
}
