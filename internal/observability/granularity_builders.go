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

package observability

import (
	"context"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// BuildComponentIngestLog assembles a component-scope ingestion log. It delegates
// to BuildIngestLog so scope is a single, genuinely multi-valued dimension:
// task | component | progress all flow through the same constructor and carry
// the same identity and run-derived fields. The task status is captured as the
// state snapshot at emit time; the component lifecycle phase lives in Phase.
func BuildComponentIngestLog(ctx context.Context, task *entity.IngestionTask, doc *entity.Document, run *entity.PipelineOperationLog, component string, phase uint8, message string) common.IngestionLog {
	ev := BuildIngestLog(ctx, task, doc, run, "component", statusOf(task), "")
	ev.Component = normalizeStage(component)
	ev.Phase = phase
	ev.Message = message
	return ev
}

// BuildProgressIngestLog assembles a progress-scope ingestion log carrying a
// page/chunk/batch/entity position. Like the component builder it goes through
// BuildIngestLog so progress events share identity and run-derived fields with
// task-scope events.
func BuildProgressIngestLog(ctx context.Context, task *entity.IngestionTask, doc *entity.Document, run *entity.PipelineOperationLog, component, unit string, current, total uint32) common.IngestionLog {
	ev := BuildIngestLog(ctx, task, doc, run, "progress", statusOf(task), "")
	ev.Component = normalizeStage(component)
	ev.Unit = unit
	ev.Current = current
	ev.Total = total
	return ev
}

// statusOf returns the task's status snapshot, or empty when the task is nil.
// Component/progress events use it as the status snapshot at emit time (scope
// is carried separately in EventType).
func statusOf(task *entity.IngestionTask) string {
	if task == nil {
		return ""
	}
	return task.Status
}

// normalizeStage maps a canvas component id / friendly name onto the bounded
// stage enum. Unknown identifiers fall back to the input so a custom component
// is still visible; empty becomes "Other" to keep LowCardinality effective.
func normalizeStage(component string) string {
	// Canvas component ids are "<Kind>:<instance>" (e.g. "Parser:node-1").
	// Stage aggregation keys on Kind only, so strip the instance suffix first.
	if kind, _, ok := strings.Cut(component, ":"); ok {
		component = kind
	}
	switch component {
	case common.ComponentNameParser:
		return "Parser"
	case common.ComponentNameGeneralChunker, common.ComponentNameTokenChunker,
		common.ComponentNameTitleChunker, common.ComponentNameManualChunker:
		return "Chunker"
	case common.ComponentNameTokenizer:
		return "Tokenizer"
	case common.ComponentNameExtractor:
		return "Extractor"
	case common.ComponentNameCompiler:
		return "Compiler"
	default:
		if component == "" {
			return "Other"
		}
		return component
	}
}
