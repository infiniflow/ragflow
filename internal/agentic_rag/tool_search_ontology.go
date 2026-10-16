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

package agentic_rag

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	searchOntologyToolName = "search_ontology"
)

// searchOntologyToolDescription teaches the one thing that is easy to get
// wrong about this tool: its arguments decide what kind of lookup this is.
//
// There is no mode parameter on purpose. A mode flag would be a second way to
// say what the arguments already say, and the two would drift — an empty
// constraint set with mode=drill is a call that means nothing.
const searchOntologyToolDescription = `Look up what the selected ontology model says, in the knowledge scope bound to this conversation.

Two uses, distinguished by the arguments alone:
- Retrieval: call it with no typed constraints to get the assertions that match your question. This is the whole knowledge scope, not the documents you have been reading.
- Drill-down: pass entity_types / properties / from_types / to_types to narrow to those declared classes and properties. The declared hierarchy is expanded for you (an abstract class also means its declared subclasses), and the filter that was actually applied comes back with the result.

What you cannot do here: choose the model or the knowledge scope. Both are bound to the conversation. Every result states its evidence grade: only "structured" rows are what the model says, and a "schema_free" result carries the reason its narrowing was dropped — read it before concluding anything about the answer.

Every assertion with a quotation carries a [ID:n] handle. Citing that handle cites the source's own wording, which is stronger than citing a passage you re-read yourself. An assertion without a handle is one whose source document is not readable in this conversation: use its wording as a search hint, but do not present its quotation as evidence.`

// SearchOntologyTool reads the knowledge layer on demand.
//
// Its binding (model, knowledge scope, caller) comes from the run's context, not
// from construction, because the tool registry is process-wide and the binding
// is per-turn. It implements eino's tool.InvokableTool.
type SearchOntologyTool struct{}

// NewSearchOntologyTool returns the knowledge-layer tool.
func NewSearchOntologyTool() *SearchOntologyTool {
	return &SearchOntologyTool{}
}

// searchOntologyArgs is the tool's argument shape. Empty constraints are valid
// and mean "retrieve", not "invalid".
type searchOntologyArgs struct {
	Question            string   `json:"question"`
	EntityTypes         []string `json:"entity_types,omitempty"`
	Properties          []string `json:"properties,omitempty"`
	FromTypes           []string `json:"from_types,omitempty"`
	ToTypes             []string `json:"to_types,omitempty"`
	TopK                int      `json:"top_k,omitempty"`
	SimilarityThreshold float64  `json:"similarity_threshold,omitempty"`
}

// Info returns the tool's metadata for the chat model.
func (t *SearchOntologyTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: searchOntologyToolName,
		Desc: searchOntologyToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"question": {
				Type: schema.String, Required: true,
				Desc: "What you are looking for, in the knowledge layer's own terms when you know them.",
			},
			"entity_types": {
				Type:     schema.Array,
				Desc:     "Optional declared classes to narrow to (an abstract class also matches its declared subclasses). Omit for a plain retrieval.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"properties": {
				Type:     schema.Array,
				Desc:     "Optional declared object properties the assertions must use.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"from_types": {
				Type:     schema.Array,
				Desc:     "Optional declared classes for the assertion's from side.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"to_types": {
				Type:     schema.Array,
				Desc:     "Optional declared classes for the assertion's to side.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"top_k": {
				Type: schema.Number,
				Desc: "How many assertions to carry back. Omit for the default.",
			},
			"similarity_threshold": {
				Type: schema.Number,
				Desc: "Dense-leg floor. Omit for the default.",
			},
		}),
	}, nil
}

// InvokableRun reads the knowledge layer through the run's binding.
func (t *SearchOntologyTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, searchOntologyToolName, t.invokableRun, argumentsInJSON)
}

func (t *SearchOntologyTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args searchOntologyArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("search_ontology: parse arguments: %w", err)
	}
	if strings.TrimSpace(args.Question) == "" {
		return "", fmt.Errorf("search_ontology: question must be a non-empty string")
	}
	binding, ok := ontologyBindingFrom(ctx)
	if !ok || !binding.Config.Enabled() {
		return ontologyToolNotice(searchOntologyToolName, "no_ontology_bound",
			"No ontology model is bound to this conversation, so there is no declared structure to look at. "+
				"Answer from the corpus and do not claim the knowledge layer supported anything."), nil
	}

	digest, err := GetOntologyService().Retrieve(ctx, OntologyRetrieveRequest{
		UserID:              binding.UserID,
		KnowledgeDatasetIDs: binding.Config.KnowledgeDatasetIDs,
		ModelID:             binding.Config.ModelID,
		ModelVersion:        binding.Config.ModelVersion,
		Question:            strings.TrimSpace(args.Question),
		EntityTypes:         args.EntityTypes,
		Properties:          args.Properties,
		FromTypes:           args.FromTypes,
		ToTypes:             args.ToTypes,
		TopK:                args.TopK,
		SimilarityThreshold: args.SimilarityThreshold,
		DocumentScope:       binding.DocumentScope,
	})
	if err != nil {
		// The layer being unreachable is a fact the model has to know, so it is
		// returned as a readable notice rather than as a Go error (which would
		// end the whole turn).
		return ontologyToolNotice(searchOntologyToolName, "unavailable", err.Error()), nil
	}
	return formatOntologyResults(ctx, searchOntologyToolName, args.Question, digest), nil
}

// formatOntologyResults renders a knowledge-layer result.
//
// The receipt travels with the rows, in the same result the model reads. The
// model has to know whether the narrowing applied, what the declared structure
// does not cover, and which quotations it may cite — deciding any of that from
// the rows alone is guessing.
func formatOntologyResults(ctx context.Context, toolName, question string, digest *OntologyDigest) string {
	reg := evidenceRegistryFrom(ctx)
	var b strings.Builder
	b.WriteString(`<ontology_result tool="` + xmlEscape(toolName) + `" question="` + xmlEscape(question) + `"`)
	if digest == nil {
		b.WriteString(` degraded="true">`)
		b.WriteString(`<notice>the knowledge layer returned nothing</notice></ontology_result>`)
		return b.String()
	}
	model := digest.Model
	b.WriteString(` model_id="` + xmlEscape(model.ID) + `"`)
	b.WriteString(fmt.Sprintf(` model_version="%d"`, model.Version))
	b.WriteString(fmt.Sprintf(` rows="%d" total="%d"`, len(digest.Assertions), digest.Total))
	// The grade is published instead of a "narrowing applied" flag: the flag is
	// one bit of the grade, and what decides how the rows may be used is which
	// grade they got, not whether one stage of the search ran.
	if digest.EvidenceLevel != "" {
		b.WriteString(` evidence="` + xmlEscape(digest.EvidenceLevel) + `"`)
	}
	if digest.Degrade != nil {
		b.WriteString(` degrade_code="` + xmlEscape(digest.Degrade.Code) + `"`)
	}
	b.WriteString(">\n")

	if digest.Degrade != nil {
		b.WriteString("  <degrade code=\"" + xmlEscape(digest.Degrade.Code) + "\">" +
			xmlEscape(digest.Degrade.Reason) + "</degrade>\n")
	}
	if digest.StructureApplied && digest.AppliedFilter != nil {
		b.WriteString("  <applied_filter>" + xmlEscape(renderFilter(digest.AppliedFilter)) + "</applied_filter>\n")
	}
	if missing := digest.Coverage.MissingClasses; len(missing) > 0 {
		b.WriteString("  <declared_but_empty classes=\"" + xmlEscape(strings.Join(limitStrings(missing, ontologyDigestMaxVocabulary), ",")) + "\"/>\n")
	}
	if digest.Coverage.Truncated {
		b.WriteString(fmt.Sprintf("  <coverage sample_limit=\"%d\" truncated=\"true\"/>\n", digest.Coverage.SampleLimit))
	}
	if digest.Citations.Unresolvable > 0 {
		b.WriteString(fmt.Sprintf("  <citations resolvable=\"%d\" unresolvable=\"%d\"/>\n",
			digest.Citations.Resolvable, digest.Citations.Unresolvable))
	}
	if digest.UnreviewedCount > 0 {
		b.WriteString(fmt.Sprintf("  <review unreviewed=\"%d\"/>\n", digest.UnreviewedCount))
	}

	for i, a := range digest.Assertions {
		b.WriteString(renderOntologyAssertionXML(i+1, a, reg))
	}
	if len(digest.Assertions) == 0 {
		b.WriteString("  <notice>no assertion in the knowledge scope matches this request" +
			" — this is a coverage gap in the model or the corpus, not an answer</notice>\n")
	}
	b.WriteString("</ontology_result>")
	return b.String()
}

// renderOntologyAssertionXML writes one assertion as an element whose citation
// ref is the [ID:n] handle the model may cite.
func renderOntologyAssertionXML(n int, a OntologyAssertion, reg *EvidenceRegistry) string {
	var b strings.Builder
	kind := firstNonEmpty(a.Kind, "assertion")
	b.WriteString(fmt.Sprintf("  <assertion n=\"%d\" kind=\"%s\"", n, xmlEscape(kind)))
	if a.Type != "" {
		b.WriteString(` type="` + xmlEscape(a.Type) + `"`)
	}
	if a.Name != "" {
		b.WriteString(` name="` + xmlEscape(a.Name) + `"`)
	}
	if a.From != "" {
		b.WriteString(` from="` + xmlEscape(a.From) + `"`)
	}
	if a.To != "" {
		b.WriteString(` to="` + xmlEscape(a.To) + `"`)
	}
	if a.ReviewStatus != "" {
		b.WriteString(` review="` + xmlEscape(a.ReviewStatus) + `"`)
	}
	if ref := ontologyCitationRef(a, reg); ref != "" {
		b.WriteString(` ref="` + xmlEscape(strings.Trim(ref, "[]")) + `"`)
	} else if len(a.ChunkIDs) > 0 {
		b.WriteString(` citable="false"`)
	}
	b.WriteString(">")
	if len(a.Attributes) > 0 {
		keys := make([]string, 0, len(a.Attributes))
		for k := range a.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("<attributes>")
		for _, k := range keys {
			b.WriteString(`<attribute name="` + xmlEscape(k) + `">` + xmlEscape(a.Attributes[k]) + "</attribute>")
		}
		b.WriteString("</attributes>")
	}
	if q := strings.TrimSpace(a.Evidence); q != "" {
		b.WriteString("<quote>" + xmlEscape(q) + "</quote>")
	}
	if len(a.DocIDs) > 0 {
		b.WriteString(`<documents>` + xmlEscape(strings.Join(a.DocIDs, ",")) + "</documents>")
	}
	b.WriteString("</assertion>\n")
	return b.String()
}

// ontologyNotice carries a knowledge-layer status the model must not have to
// infer from an empty result.
type ontologyNotice struct {
	Tool     string `json:"tool"`
	Reason   string `json:"reason"`
	Guidance string `json:"guidance"`
}

// ontologyToolNotice renders a non-fatal knowledge-layer notice.
func ontologyToolNotice(tool, reason, guidance string) string {
	payload := ontologyNotice{Tool: tool, Reason: reason, Guidance: guidance}
	data, err := json.Marshal(payload)
	if err != nil {
		return toolErrorNotice(tool, fmt.Errorf("%s: %s", reason, guidance))
	}
	return string(data)
}

// compile-time proof that the tool satisfies eino's interface.
var _ einotool.InvokableTool = (*SearchOntologyTool)(nil)
