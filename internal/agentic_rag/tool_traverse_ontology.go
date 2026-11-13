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
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const traverseOntologyToolName = "traverse_ontology"

// traverseOntologyToolDescription teaches the one thing a lookup cannot do: reach
// an entity the question never names.
//
// The distinction matters because the two tools look interchangeable from the
// outside and are not. search_ontology ranks assertions against a question, so it
// finds what the question's own words are near. A multi-hop question is joined by
// an entity that appears in none of those words — the middle of the chain — and no
// amount of ranking will surface it. This tool follows the chain instead.
const traverseOntologyToolDescription = `Walk the selected ontology model's assertions outward from named entities.

Use this when the question is joined by an entity you already found but the answer needs what that entity connects to — for example "which papers used a model that includes an ocean variable": the model is not in the question, but it is the only thing linking the paper to the variable. search_ontology cannot reach it by ranking; this can, by following declared properties.

You choose the start entities and the constraints; the walk itself is deterministic. Results are paths, not ranked rows: each path lists the entities in order and the assertion crossed at each step, with the chunks that assertion was compiled from. Follow those chunk ids with list_chunks when you need the wording itself — a chunk id is a pointer, not evidence.

Each hop reports how many rows it matched and how many it refused, and it separates refusals by cause: rejected_by_declaration means the model's own domain/range refused the row, rejected_by_allow_list means your properties/types filter did. A walk that matched nothing but refused a lot is a declaration problem; a walk that read nothing is a coverage problem. Do not present either as an answer.`

// TraverseOntologyTool walks compiled assertions along declared properties.
type TraverseOntologyTool struct{}

// NewTraverseOntologyTool returns the typed-walk tool.
func NewTraverseOntologyTool() *TraverseOntologyTool {
	return &TraverseOntologyTool{}
}

// traverseOntologyArgs is the tool's argument shape.
type traverseOntologyArgs struct {
	Starts     []string `json:"starts"`
	Hops       int      `json:"hops,omitempty"`
	Properties []string `json:"properties,omitempty"`
	Types      []string `json:"types,omitempty"`
	MaxPaths   int      `json:"max_paths,omitempty"`
}

// Info returns the tool's metadata for the chat model.
func (t *TraverseOntologyTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: traverseOntologyToolName,
		Desc: traverseOntologyToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"starts": {
				Type:     schema.Array,
				Required: true,
				Desc:     "Entity names to walk from, spelled as the compiled assertions spell them.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"hops": {
				Type: schema.Number,
				Desc: "How many assertions to cross. Omit for the default; requests above the cap are clamped.",
			},
			"properties": {
				Type:     schema.Array,
				Desc:     "Optional declared object properties the walk may follow. Omit to allow every declared one.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"types": {
				Type:     schema.Array,
				Desc:     "Optional declared classes an entity reached by the walk may have.",
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
			"max_paths": {
				Type: schema.Number,
				Desc: "How many chains to carry back. Omit for the default.",
			},
		}),
	}, nil
}

// InvokableRun walks the knowledge layer through the run's binding.
func (t *TraverseOntologyTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, traverseOntologyToolName, t.invokableRun, argumentsInJSON)
}

func (t *TraverseOntologyTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args traverseOntologyArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("traverse_ontology: parse arguments: %w", err)
	}
	starts := make([]string, 0, len(args.Starts))
	for _, start := range args.Starts {
		if trimmed := strings.TrimSpace(start); trimmed != "" {
			starts = append(starts, trimmed)
		}
	}
	if len(starts) == 0 {
		return "", fmt.Errorf("traverse_ontology: starts must name at least one entity")
	}
	binding, ok := ontologyBindingFrom(ctx)
	if !ok || !binding.Config.Enabled() {
		return ontologyToolNotice(traverseOntologyToolName, "no_ontology_bound",
			"No ontology model is bound to this conversation, so there is no declared structure to walk. "+
				"Use search or list_chunks instead, and do not claim the knowledge layer supported anything."), nil
	}

	traversal, err := GetOntologyService().Traverse(ctx, OntologyTraverseRequest{
		UserID:              binding.UserID,
		KnowledgeDatasetIDs: binding.Config.KnowledgeDatasetIDs,
		ModelID:             binding.Config.ModelID,
		ModelVersion:        binding.Config.ModelVersion,
		Starts:              starts,
		Hops:                args.Hops,
		AllowProperties:     args.Properties,
		AllowTypes:          args.Types,
		MaxPaths:            args.MaxPaths,
		DocumentScope:       binding.DocumentScope,
	})
	if err != nil {
		// The layer being unreachable is a fact the model has to know, so it is a
		// readable notice rather than a Go error (which would end the turn).
		return ontologyToolNotice(traverseOntologyToolName, "unavailable", err.Error()), nil
	}
	return formatOntologyTraversal(traversal), nil
}

// formatOntologyTraversal renders a walk.
//
// The hop accounting is part of the result, not a debug detail: an empty path list
// has at least three different causes (no such entities, everything refused by the
// declaration, everything refused by the caller's filter) and the model cannot
// tell them apart from the paths alone.
func formatOntologyTraversal(traversal *OntologyTraversal) string {
	var b strings.Builder
	b.WriteString(`<ontology_traversal`)
	if traversal == nil {
		b.WriteString(` degraded="true"><notice>the walk returned nothing</notice></ontology_traversal>`)
		return b.String()
	}
	b.WriteString(` model_id="` + xmlEscape(traversal.Model.ID) + `"`)
	b.WriteString(fmt.Sprintf(` model_version="%d"`, traversal.Model.Version))
	b.WriteString(fmt.Sprintf(` hops="%d" paths="%d"`, len(traversal.Hops), len(traversal.Paths)))
	if traversal.Truncated {
		b.WriteString(` truncated="true"`)
	}
	if traversal.DegradeCode != "" {
		b.WriteString(` degrade_code="` + xmlEscape(traversal.DegradeCode) + `"`)
	}
	b.WriteString(">\n")

	if traversal.DegradeCode != "" {
		b.WriteString("  <degrade code=\"" + xmlEscape(traversal.DegradeCode) + "\">" +
			xmlEscape(traversal.DegradeReason) + "</degrade>\n")
	}
	for _, hop := range traversal.Hops {
		b.WriteString(fmt.Sprintf("  <hop n=\"%d\" frontier=\"%d\" rows=\"%d\" matched=\"%d\" rejected_by_declaration=\"%d\" rejected_by_allow_list=\"%d\"",
			hop.Hop, hop.Frontier, hop.Rows, hop.Matched, hop.RejectedByDeclaration, hop.RejectedByAllowList))
		if hop.Truncated {
			b.WriteString(` truncated="true"`)
		}
		b.WriteString("/>\n")
	}
	for i, path := range traversal.Paths {
		b.WriteString(fmt.Sprintf("  <path n=\"%d\" start=\"%s\" nodes=\"%s\" steps=\"%d\">\n",
			i+1, xmlEscape(path.Start), xmlEscape(strings.Join(path.Nodes, " > ")), len(path.Steps)))
		for _, step := range path.Steps {
			b.WriteString(fmt.Sprintf("    <step from=\"%s\" property=\"%s\" to=\"%s\"",
				xmlEscape(step.From), xmlEscape(step.Property), xmlEscape(step.To)))
			if step.FromType != "" {
				b.WriteString(` from_type="` + xmlEscape(step.FromType) + `"`)
			}
			if step.ToType != "" {
				b.WriteString(` to_type="` + xmlEscape(step.ToType) + `"`)
			}
			if step.Direction != "" {
				b.WriteString(` direction="` + xmlEscape(step.Direction) + `"`)
			}
			if len(step.SourceChunkIDs) > 0 {
				b.WriteString(` chunks="` + xmlEscape(strings.Join(step.SourceChunkIDs, ",")) + `"`)
			}
			b.WriteString("/>\n")
		}
		b.WriteString("  </path>\n")
	}
	if len(traversal.Paths) == 0 {
		b.WriteString("  <notice>no path was found — read the hop counts before concluding: no rows read means the entities " +
			"are not in the knowledge scope, while rows refused by the declaration or by your filter mean the walk was " +
			"stopped rather than absent</notice>\n")
	}
	b.WriteString("</ontology_traversal>")
	return b.String()
}

// compile-time proof that the tool satisfies eino's interface.
var _ einotool.InvokableTool = (*TraverseOntologyTool)(nil)
