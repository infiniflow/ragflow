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
	"ragflow/internal/agentic_rag"
)

// OntologyDigestReceipt renders a digest as the wire shape the UI reads.
//
// It is one function rather than struct tags so the receipt has exactly one
// definition: the REST preview and the chat answer's receipt must describe the
// same retrieval the same way, and two marshalling paths would drift the moment
// one of them gained a field.
//
// Nothing here is optional-by-omission except the parts that are genuinely
// absent: a nil degrade means the structure applied cleanly, and the UI is
// expected to say nothing rather than to say "no degrade". Every count is
// always present, including the zeroes — a citation count that disappears when
// it is zero cannot be told apart from a receipt that never computed it.
func OntologyDigestReceipt(digest *agentic_rag.OntologyDigest) map[string]interface{} {
	if digest == nil {
		return nil
	}
	out := map[string]interface{}{
		"model": map[string]interface{}{
			"id":             digest.Model.ID,
			"name":           digest.Model.Name,
			"version":        digest.Model.Version,
			"publish_status": digest.Model.PublishStatus,
		},
		"vocabulary":        digest.Vocabulary,
		"applied_filter":    digest.AppliedFilter,
		"structure_applied": digest.StructureApplied,
		// The evidence grade is part of the receipt for the same reason the
		// counts are: "these rows are the model's own narrowing" and "these rows
		// were retrieved without it" must not be told apart by reading the
		// absence of a filter.
		"evidence_level":   digest.EvidenceLevel,
		"unreviewed_count": digest.UnreviewedCount,
		"rows_carried":     len(digest.Assertions),
		"rows_total":       digest.Total,
		"citations": map[string]interface{}{
			"resolvable":   digest.Citations.Resolvable,
			"unresolvable": digest.Citations.Unresolvable,
		},
		"coverage": map[string]interface{}{
			"declared_classes":    digest.Coverage.DeclaredClasses,
			"declared_properties": digest.Coverage.DeclaredProperties,
			"matched_classes":     digest.Coverage.MatchedClasses,
			"matched_properties":  digest.Coverage.MatchedProperties,
			"missing_classes":     digest.Coverage.MissingClasses,
			"missing_properties":  digest.Coverage.MissingProperties,
			"sampled":             digest.Coverage.Sampled,
			"sample_limit":        digest.Coverage.SampleLimit,
			"sample_truncated":    digest.Coverage.Truncated,
		},
	}
	if digest.Degrade != nil {
		out["degrade"] = map[string]interface{}{
			"code":   digest.Degrade.Code,
			"reason": digest.Degrade.Reason,
		}
	}
	return out
}

// OntologyAssertionsReceipt renders the assertions a digest carried.
//
// Each row names its source chunk ids and deliberately carries no citation
// index: the answer's `[ID:n]` numbering is published per answer (see
// ResolveCitations), so a second numbering here would be a different number for
// the same passage. The UI matches a row to the answer by chunk id, which is
// the one identifier both sides agree on.
func OntologyAssertionsReceipt(digest *agentic_rag.OntologyDigest) []map[string]interface{} {
	if digest == nil || len(digest.Assertions) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(digest.Assertions))
	for _, a := range digest.Assertions {
		row := map[string]interface{}{
			"kind":          a.Kind,
			"type":          a.Type,
			"name":          a.Name,
			"review_status": a.ReviewStatus,
			"chunk_ids":     a.ChunkIDs,
			"doc_ids":       a.DocIDs,
			"evidence":      a.Evidence,
		}
		if a.From != "" || a.To != "" {
			row["from"] = a.From
			row["from_type"] = a.FromType
			row["to"] = a.To
			row["to_type"] = a.ToType
		}
		if len(a.Attributes) > 0 {
			row["attributes"] = a.Attributes
		}
		out = append(out, row)
	}
	return out
}
