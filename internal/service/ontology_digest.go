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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"ragflow/internal/agentic_rag"
)

// ontologyAssertionRowFields is what the digest needs from a compiled row.
//
// It is the union of the graph readers' own projections rather than a list
// written out again: these are the same rows read by readers that have to agree,
// and a column added for one of them going silently missing for the other is
// exactly how the two come to disagree about a row. The union over-asks on
// purpose — the search is one request, and a column that was not requested reads
// empty with nothing to say it was never fetched.
var ontologyAssertionRowFields = ontologyFieldUnion(
	graphEntityFields,
	graphRelationFields,
	graphAllFields,
)

// ontologyFieldUnion joins projections in order, dropping repeats: the engines
// receive this list as their _source filter, where a column named twice only
// inflates the request.
func ontologyFieldUnion(groups ...[]string) []string {
	var out []string
	for _, group := range groups {
		out = append(out, group...)
	}
	return dedupeStrings(out)
}

// ontologyCoverageRowFields is the minimal projection the coverage scan needs:
// the class column of entity rows and the property column of assertion rows.
var ontologyCoverageRowFields = []string{"type_kwd", "knowledge_graph_kwd", "entity_type_kwd", "prop_kwd", "compilation_template_ids"}

// buildOntologyAssertion converts one compiled row into the digest's assertion
// form.
//
// A row that is neither an entity nor a relation returns false: the knowledge
// scope can hold navigation clusters, wiki pages and build markers, and a
// digest that carried them as "assertions" would present a page title as a fact
// the model stated.
func buildOntologyAssertion(row map[string]interface{}) (agentic_rag.OntologyAssertion, bool) {
	kind := ontologyRowKind(row)
	if kind != "entity" && kind != "relation" {
		return agentic_rag.OntologyAssertion{}, false
	}
	out := agentic_rag.OntologyAssertion{Kind: kind}
	if kind == "entity" {
		out.Type = firstStringValue(row["entity_type_kwd"])
		out.Name = firstStringValue(row["name_kwd"])
		if out.Name == "" {
			out.Name = firstStringValue(row["content_with_weight"])
		}
		out.Attributes = ontologyRowAttributes(row["attr"])
	} else {
		out.Type = firstStringValue(row["prop_kwd"])
		out.Name = out.Type
		out.From = firstStringValue(row["from_entity_kwd"])
		out.FromType = firstStringValue(row["from_type_kwd"])
		out.To = firstStringValue(row["to_entity_kwd"])
		out.ToType = firstStringValue(row["to_type_kwd"])
	}
	out.ChunkIDs = stringSliceValue(row["source_chunk_ids"])
	out.DocIDs = ontologyRowDocIDs(row)
	// The quotations live inside the row's JSON payload, not in a column of their
	// own: the writer stores the compiled payload verbatim and the graph readers
	// lift the quotes out of it (see ontologyEvidenceOf). A top-level "evidence"
	// lookup reads empty on every row, which would ship assertions that no longer
	// say where they came from.
	if quote, chunkID := ontologyPayloadEvidence(row); quote != "" {
		out.Evidence = quote
		if chunkID != "" && !containsStringValue(out.ChunkIDs, chunkID) {
			out.ChunkIDs = append([]string{chunkID}, out.ChunkIDs...)
		}
	}
	out.ReviewStatus = ontologyRowReviewStatus(row)
	out.ReviewBy = firstStringValue(row["review_by_kwd"])
	return out, true
}

// ontologyRowKind reads a compiled row's role.
//
// type_kwd is the current column and knowledge_graph_kwd the pre-rename one.
// Both are read because the filter key of the same name matches either, so a
// reader that understood only one of them would drop rows the filter returned.
func ontologyRowKind(row map[string]interface{}) string {
	if kind := strings.TrimSpace(firstStringValue(row["type_kwd"])); kind != "" {
		return kind
	}
	return strings.TrimSpace(firstStringValue(row["knowledge_graph_kwd"]))
}

// ontologyRowAttributes reads an entity's declared datatype values. They are
// stored as one JSON object because a class's attribute set is unbounded, and
// an unreadable payload yields no attributes rather than an error: a row whose
// attributes cannot be parsed is still an assertion about an entity.
func ontologyRowAttributes(raw interface{}) map[string]string {
	if raw == nil {
		return nil
	}
	payload := map[string]interface{}{}
	switch v := raw.(type) {
	case map[string]interface{}:
		payload = v
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(v), &payload); err != nil {
			return nil
		}
	default:
		return nil
	}
	out := make(map[string]string, len(payload))
	for key, value := range payload {
		if value == nil {
			continue
		}
		text := strings.TrimSpace(toStringValue(value))
		if text != "" {
			out[key] = text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ontologyRowDocIDs reads the documents a row came from: the single doc_id for
// per-document rows, and the aggregated source document list for merged ones.
func ontologyRowDocIDs(row map[string]interface{}) []string {
	out := stringSliceValue(row["doc_id"], row["doc_ids_kwd"], row["source_doc_ids"])
	return dedupeStrings(out)
}

// ontologyPayloadEvidence reads the first verified quotation out of a compiled
// row's payload, together with the chunk it was located in.
//
// Only entries that survived the evidence gate are readable: the writer stores a
// quote together with the span it was located at, so an entry without one never
// made it past validation (see keptEvidence). A payload that cannot be parsed
// yields no quotation rather than an error — the row is still an assertion.
func ontologyPayloadEvidence(row map[string]interface{}) (string, string) {
	for _, entry := range keptEvidence(graphLoadPayload(row)["evidence"]) {
		if quote := strings.TrimSpace(toStringValue(entry["quote"])); quote != "" {
			return quote, strings.TrimSpace(toStringValue(entry["chunk_id"]))
		}
	}
	return "", ""
}

// ontologyRowReviewStatus normalizes a row's review state. An absent or empty
// column is "unreviewed", which is the same rule the review queue counts by:
// an unset status and a reset status are one state, not two.
func ontologyRowReviewStatus(row map[string]interface{}) string {
	status := strings.ToLower(strings.TrimSpace(firstStringValue(row["review_status_kwd"])))
	if status == "" {
		return string(AssertionReviewUnreviewed)
	}
	return status
}

// buildOntologyDigest converts one knowledge-layer page into the digest the
// agent loop consumes, counting what may be cited and what is unreviewed.
//
// The counts are computed here rather than left to the reader because both are
// load-bearing: an assertion whose chunks are unreadable contributes wording but
// must not contribute a quotation, and a pending assertion is the whole of the
// evidence in the direct-answer mode.
func buildOntologyDigest(
	rows []map[string]interface{},
	total int64,
	declared declaredOntology,
	ref OntologyModelRef,
	filter map[string]interface{},
	structureApplied bool,
	degrade *agentic_rag.OntologyDegrade,
	documentScope []string,
) *agentic_rag.OntologyDigest {
	out := &agentic_rag.OntologyDigest{
		Model: agentic_rag.OntologyModelRef{
			ID:            ref.ID,
			Name:          ref.Name,
			Version:       ref.Version,
			PublishStatus: ref.PublishStatus,
		},
		Assertions:       make([]agentic_rag.OntologyAssertion, 0, len(rows)),
		AppliedFilter:    filter,
		StructureApplied: structureApplied,
		EvidenceLevel:    ontologyEvidenceLevel(total, structureApplied),
		Degrade:          degrade,
		Total:            total,
	}
	vocabulary := make([]string, 0, len(rows))
	for _, row := range rows {
		assertion, ok := buildOntologyAssertion(row)
		if !ok {
			continue
		}
		out.Assertions = append(out.Assertions, assertion)
		if assertion.ReviewStatus == string(AssertionReviewUnreviewed) {
			out.UnreviewedCount++
		}
		for _, term := range []string{assertion.Type, assertion.Name, assertion.From, assertion.To} {
			term = strings.TrimSpace(term)
			if term != "" && !containsStringValue(vocabulary, term) {
				vocabulary = append(vocabulary, term)
			}
		}
		out.Citations.Resolvable, out.Citations.Unresolvable = countOntologyCitations(
			out.Citations.Resolvable, out.Citations.Unresolvable, assertion, documentScope)
	}
	// The declared vocabulary the question matched belongs in the digest too:
	// those are the terms the model should phrase its searches with, and a
	// matched class with no instance yet still names the domain.
	for _, name := range declaredNames(declared) {
		if !containsStringValue(vocabulary, name) {
			vocabulary = append(vocabulary, name)
		}
	}
	out.Vocabulary = vocabulary
	return out
}

// ontologyEvidenceLevel grades the rows one knowledge search produced.
//
// The grade follows from the two facts the search already knows — how many rows
// came back, and whether the declared structure is what produced them — and it
// is computed once, here, because every consumer reads it (the REST result, the
// run's instruction, the tool's own result). A consumer that re-derived it would
// eventually disagree with the others about the same rows.
//
// An empty result is graded none whatever the reason: not finding an assertion
// and finding one whose quotation cannot be cited are different facts, but
// neither of them is evidence, and the reason travels in Degrade.
func ontologyEvidenceLevel(total int64, structureApplied bool) string {
	if total <= 0 {
		return agentic_rag.OntologyEvidenceNone
	}
	if structureApplied {
		return agentic_rag.OntologyEvidenceStructured
	}
	return agentic_rag.OntologyEvidenceSchemaFree
}

// declaredNames lists every declared class and property, in declaration order.
func declaredNames(declared declaredOntology) []string {
	out := make([]string, 0, len(declared.classOrder)+len(declared.propOrder))
	out = append(out, declared.classOrder...)
	out = append(out, declared.propOrder...)
	return out
}

// countOntologyCitations classifies one assertion's source chunks as readable or
// not in the conversation's document scope.
//
// The scope is checked at document granularity because that is the finest one
// the row carries: a chunk's access is its dataset's, and the datasets
// themselves were already authorized when the scope was resolved. A nil scope
// means the conversation is not pinned to any document set, so every source is
// readable.
func countOntologyCitations(resolvable, unresolvable int, a agentic_rag.OntologyAssertion, documentScope []string) (int, int) {
	chunks := a.ChunkIDs
	if len(chunks) == 0 {
		// No source chunk is not an unreadable citation: there is nothing to
		// cite, and counting it as unresolvable would report a permission
		// problem where there is only an absence of evidence.
		return resolvable, unresolvable
	}
	if documentScope == nil {
		return resolvable + len(chunks), unresolvable
	}
	if len(documentScope) == 0 {
		return resolvable, unresolvable + len(chunks)
	}
	allowed := make(map[string]struct{}, len(documentScope))
	for _, id := range documentScope {
		allowed[strings.TrimSpace(id)] = struct{}{}
	}
	readable := false
	for _, docID := range a.DocIDs {
		if _, ok := allowed[strings.TrimSpace(docID)]; ok {
			readable = true
			break
		}
	}
	if readable {
		return resolvable + len(chunks), unresolvable
	}
	return resolvable, unresolvable + len(chunks)
}

// ontologyCoverage computes how much of the declared structure the knowledge
// scope actually exercises.
//
// It is a scan of the compiled rows' type columns and nothing more: no LLM
// judges whether the coverage is "sufficient", because a judgement that cannot
// be reproduced cannot be argued with. The two facts it does produce are the
// ones a reader can act on — a declared class with no instance is a compilation
// gap, and a scan that hit its cap is a limit of this report rather than a
// statement about the corpus.
func ontologyCoverage(
	declared declaredOntology,
	matchedClasses map[string]bool,
	matchedProperties map[string]bool,
	sampled, sampleLimit int,
	truncated bool,
) agentic_rag.OntologyCoverage {
	out := agentic_rag.OntologyCoverage{
		DeclaredClasses:    append([]string{}, declared.classOrder...),
		DeclaredProperties: append([]string{}, declared.propOrder...),
		SampleLimit:        sampleLimit,
		Sampled:            sampled,
		Truncated:          truncated,
	}
	for _, name := range declared.classOrder {
		if matchedClasses[name] {
			out.MatchedClasses = append(out.MatchedClasses, name)
			continue
		}
		out.MissingClasses = append(out.MissingClasses, name)
	}
	for _, name := range declared.propOrder {
		if matchedProperties[name] {
			out.MatchedProperties = append(out.MatchedProperties, name)
			continue
		}
		out.MissingProperties = append(out.MissingProperties, name)
	}
	return out
}

// applyOntologyHypergraph attaches the hypergraph section and lets it narrow the
// assertion list it belongs to.
//
// One rule is applied: a relation whose two endpoints both appear inside ONE
// assembled block is not a separate fact — the block already states it, in the
// same place, in the same words — and listing it again spends the assertion
// budget on a duplicate. The endpoints are compared lowercased because the
// compiled name columns are lowercased while a relation row stores the display
// spelling.
//
// The counts are then recomputed rather than adjusted: the digest's citation and
// review numbers are what a caller reads to decide whether a quotation may be
// used, and a decrement applied by hand would drift from the assertions the
// block now carries.
func applyOntologyHypergraph(digest *agentic_rag.OntologyDigest, hg *agentic_rag.OntologyHypergraph, documentScope []string) {
	if digest == nil || hg == nil {
		return
	}
	digest.Hypergraph = hg
	if len(digest.Assertions) == 0 {
		return
	}
	blockNames := ontologyHypergraphBlockNames(hg)
	kept := make([]agentic_rag.OntologyAssertion, 0, len(digest.Assertions))
	digest.UnreviewedCount = 0
	digest.Citations = agentic_rag.OntologyCitations{}
	for _, assertion := range digest.Assertions {
		if assertion.Kind == "relation" && ontologyRelationStatedByBlock(assertion, blockNames) {
			continue
		}
		kept = append(kept, assertion)
		if assertion.ReviewStatus == string(AssertionReviewUnreviewed) {
			digest.UnreviewedCount++
		}
		digest.Citations.Resolvable, digest.Citations.Unresolvable = countOntologyCitations(
			digest.Citations.Resolvable, digest.Citations.Unresolvable, assertion, documentScope)
	}
	digest.Assertions = kept
}

// ontologyHypergraphBlockNames returns, per assembled block, the lowercased
// names of the entities the block states. Only `:name` slots count: an attribute
// slot that happens to hold an entity's name is a statement about a value, not
// about the related entity.
func ontologyHypergraphBlockNames(hg *agentic_rag.OntologyHypergraph) []map[string]bool {
	out := make([]map[string]bool, 0, len(hg.Edges))
	for _, edge := range hg.Edges {
		names := make(map[string]bool, len(edge.Facts))
		for _, fact := range edge.Facts {
			if !strings.HasSuffix(strings.TrimSpace(fact.Path), ":name") {
				continue
			}
			if value := strings.ToLower(strings.TrimSpace(fact.Value)); value != "" {
				names[value] = true
			}
		}
		if len(names) > 0 {
			out = append(out, names)
		}
	}
	return out
}

// ontologyRelationStatedByBlock reports whether one block already states this
// relation by naming both of its endpoints.
func ontologyRelationStatedByBlock(assertion agentic_rag.OntologyAssertion, blockNames []map[string]bool) bool {
	from := strings.ToLower(strings.TrimSpace(assertion.From))
	to := strings.ToLower(strings.TrimSpace(assertion.To))
	if from == "" || to == "" {
		return false
	}
	for _, names := range blockNames {
		if names[from] && names[to] {
			return true
		}
	}
	return false
}

// toStringValue renders an engine field as text: numbers keep their exact
// value (an attribute is a value, and 1.5 must not become "1.5" by way of a
// lossy format), and composite payloads print as they are.
func toStringValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	default:
		raw := strings.TrimSpace(fmt.Sprint(v))
		return strings.TrimSpace(strings.Trim(raw, `"`))
	}
}
