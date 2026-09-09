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

package harness

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	einotool "github.com/cloudwego/eino/components/tool"

	"gorm.io/gorm"
	"ragflow/internal/agent/tool"
	"ragflow/internal/service/nav"
)

// ToolResult is the normalized result of executing one agent tool, mirroring
// Python harness/types.py ToolResult.
type ToolResult struct {
	Chunks []map[string]interface{}
	Docs   []string // doc ids produced by a routing tool (dataset_navigation_*)
	Answer string   // direct answer (ontology_navigate / structured_query)
	Error  string
	// EvidenceIndices holds the GLOBAL indices of Chunks after they were merged
	// into the shared kbinfos (populated by Execute, under mu). Consumers must
	// use these rather than re-indexing the shared slice, which is mutated
	// concurrently by parallel claim research.
	EvidenceIndices []int
}

// docScopeConsumers are the tools that retrieve *within* a document set. When a
// routing tool has produced a relevant-doc set, these inherit it as doc_scope
// unless the caller passed one explicitly.
var docScopeConsumers = map[string]bool{
	"ontology_navigate": true,
	"mindmap_navigate":  true,
	"graph_explore":     true,
	"hybrid_search":     true,
	"vector_search":     true,
	"bm25_search":       true,
	"structured_query":  true,
}

// Pipeline is the unified tool-execution dispatcher (mirrors Python
// harness/pipeline.py). It routes a tool name + args to the concrete tool,
// injects doc_scope for within-document tools, merges evidence into the shared
// Kbinfos, and filters the tool list by compilation availability.
//
// It composes a *ProductionRunner (which owns the real tool instances) so the
// agent loop drives the SAME retrieval path as the linear Run flow.
type Pipeline struct {
	db         *gorm.DB
	tenantID   string
	datasetIDs []string
	runner     *ProductionRunner
	kbinfos    *Kbinfos
	// compilation[kbID] = set of compiled-artifact kinds. Empty map disables
	// compilation gating.
	compilation map[string]map[string]bool
	// routedDocs are the latest relevant-doc ids produced by a routing tool.
	routedDocs []string
	// lastEntity is the most recently discovered entity/document name (Step A.5).
	// It gates graph_explore eligibility (mirrors OrchestratorContext.last_entity).
	lastEntity string
	trace      []string
	// mu guards kbinfos.Merge + routedDocs, which the parallel claim research in
	// AgenticResearch mutates concurrently (Python relies on the single-threaded
	// event loop; Go must serialize).
	mu sync.Mutex
}

// NewPipeline builds a Pipeline over the given production runner and evidence
// store. compilation may be nil (no gating).
func NewPipeline(db *gorm.DB, tenantID string, datasetIDs []string, runner *ProductionRunner, kbinfos *Kbinfos, compilation map[string]map[string]bool) *Pipeline {
	if kbinfos == nil {
		kbinfos = &Kbinfos{}
	}
	return &Pipeline{
		db: db, tenantID: tenantID, datasetIDs: datasetIDs, runner: runner,
		kbinfos: kbinfos, compilation: compilation,
	}
}

// HasRoutedScope mirrors Python gating.py tool_fits_context has_routed_scope
// (agent.py:167 uses bool(pipeline._routed_docs)). ontology/mindmap/graph tools
// are only callable once a routing tool has produced a doc set.
func (p *Pipeline) HasRoutedScope() bool { return len(p.scope()) > 0 }

// scope returns a snapshot of the routed docs under mu (mirrors Python
// pipeline._routed_docs; the parallel claim research mutates it concurrently).
func (p *Pipeline) scope() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.routedDocs...)
}

// chunksSnapshot returns a snapshot of the accumulated evidence chunks under mu.
func (p *Pipeline) chunksSnapshot() []map[string]interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]map[string]interface{}(nil), p.kbinfos.Chunks...)
}

// noteEntity records the most recently discovered entity (mirrors
// OrchestratorContext.note_entity). Ignores empty values so a fruitless round
// cannot clear a prior discovery.
func (p *Pipeline) noteEntity(name string) {
	if p == nil {
		return
	}
	if name = strings.TrimSpace(name); name != "" {
		p.mu.Lock()
		p.lastEntity = name
		p.mu.Unlock()
	}
}

// HasDiscoveredEntity reports whether graph_explore is eligible (mirrors
// gating.py:81 `graph_explore and not context.last_entity`).
func (p *Pipeline) HasDiscoveredEntity() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastEntity != ""
}

// Kbinfos returns the shared evidence store.
func (p *Pipeline) Kbinfos() *Kbinfos { return p.kbinfos }

// Execute dispatches a tool call and merges the result into kbinfos. It mirrors
// Python Pipeline.execute: doc_scope injection + kbinfos merge + routing-tool
// doc tracking.
func (p *Pipeline) Execute(ctx context.Context, toolName string, args map[string]interface{}) ToolResult {
	res := p.executeTool(ctx, toolName, args)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(res.Docs) > 0 {
		// A routing tool yielded relevant doc ids — remember them so downstream
		// within-document tools inherit the scope.
		p.routedDocs = res.Docs
	}
	if len(res.Chunks) > 0 {
		res.EvidenceIndices = p.kbinfos.Merge(res.Chunks, nil)
	}
	p.trace = append(p.trace, toolName)
	return res
}

// executeTool runs one concrete tool. It injects doc_scope for within-document
// tools and returns the normalized result (without merging).
func (p *Pipeline) executeTool(ctx context.Context, toolName string, args map[string]interface{}) ToolResult {
	if args == nil {
		args = map[string]interface{}{}
	}
	// doc_scope inheritance: within-document tools take the routed docs unless
	// the caller passed an explicit doc_scope.
	if docScopeConsumers[toolName] {
		if scope := p.scope(); len(scope) > 0 {
			if _, has := args["doc_scope"]; !has {
				args["doc_scope"] = scope
			}
		}
	}

	switch toolName {
	case "hybrid_search", "vector_search", "bm25_search":
		return p.runSearchTool(ctx, toolName, args)
	case "web_search":
		return p.runWebTool(ctx, args)
	case "dataset_navigation_by_tree":
		return p.runNavTool(ctx, args)
	case "wiki_query":
		return p.runWikiTool(ctx, args)
	case "ontology_navigate", "mindmap_navigate":
		return p.runStructureTool(ctx, toolName, args)
	case "graph_explore":
		return p.runGraphExploreTool(ctx, args)
	case "inspector_open_context", "inspector_compare", "inspector_grep_within", "inspector_request_adjacent":
		return p.runInspectorTool(toolName, args)
	case "structured_query":
		return p.runSQLTool(ctx, args)
	default:
		return ToolResult{Error: "Unknown tool: " + toolName}
	}
}

// runSearchTool runs hybrid/vector/bm25 retrieval.
func (p *Pipeline) runSearchTool(ctx context.Context, toolName string, args map[string]interface{}) ToolResult {
	if p.runner == nil {
		return ToolResult{}
	}
	// Default kb_ids to the pipeline's bound datasets.
	if _, has := args["kb_ids"]; !has {
		args["kb_ids"] = p.datasetIDs
	}

	var inv einotool.InvokableTool
	if toolName == "hybrid_search" {
		inv = p.runner.searchTool
	} else {
		base, err := tool.BuildByName(toolName, nil)
		if err != nil {
			return ToolResult{Error: err.Error()}
		}
		t, ok := base.(einotool.InvokableTool)
		if !ok {
			return ToolResult{Error: toolName + " is not invokable"}
		}
		inv = t
	}
	if inv == nil {
		return ToolResult{}
	}
	raw, err := inv.InvokableRun(ctx, mustJSON(args))
	if err != nil {
		return ToolResult{Error: err.Error()}
	}
	var res struct {
		Chunks []map[string]interface{} `json:"chunks"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return ToolResult{}
	}
	return ToolResult{Chunks: res.Chunks}
}

// runNavTool runs the dataset-navigation router and returns its doc ids.
func (p *Pipeline) runNavTool(ctx context.Context, args map[string]interface{}) ToolResult {
	if p.runner == nil {
		return ToolResult{}
	}
	topic := stringValue(args["topic"])
	keywords := stringValue(args["keywords"])
	query := strings.TrimSpace(topic + " " + keywords)
	if query == "" {
		return ToolResult{}
	}
	ns := p.runner.navSvc
	if ns == nil {
		ns = nav.GetNavService()
	}
	if ns == nil {
		return ToolResult{}
	}
	seen := map[string]bool{}
	var docs []string
	for _, kbID := range p.datasetIDs {
		for _, id := range NavigateDatasetByTree(ctx, p.db, ns, p.tenantID, kbID, query) {
			if id != "" && !seen[id] {
				seen[id] = true
				docs = append(docs, id)
			}
		}
	}
	return ToolResult{Docs: docs}
}

// runWebTool runs web_search via the runner's configured web provider. Returns
// an empty result when no provider is configured (mirrors Python web_search
// has_web() guard).
func (p *Pipeline) runWebTool(ctx context.Context, args map[string]interface{}) ToolResult {
	if p.runner == nil || p.runner.webTool == nil {
		return ToolResult{}
	}
	query := stringValue(args["query"])
	keywords := stringValue(args["keywords"])
	raw, err := p.runner.webTool.InvokableRun(ctx, mustJSON(map[string]interface{}{"query": query, "keywords": keywords}))
	if err != nil {
		return ToolResult{Error: err.Error()}
	}
	chunks := normalizeWebResults([]byte(raw))
	return ToolResult{Chunks: chunks}
}

// runGraphExploreTool runs graph_explore via ExploreGraph (kg_explore.go).
func (p *Pipeline) runGraphExploreTool(ctx context.Context, args map[string]interface{}) ToolResult {
	// Gate: graph_explore is only offered once research has surfaced an entity
	// to expand from (mirrors gating.py:81 tool_fits_context).
	if !p.HasDiscoveredEntity() {
		return ToolResult{}
	}
	topic := stringValue(args["query"])
	if topic == "" {
		topic = stringValue(args["topic"])
	}
	keywords := stringValue(args["keywords"])
	var scope []string
	switch v := args["doc_scope"].(type) {
	case []string:
		scope = v
	case []interface{}:
		for _, x := range v {
			if s, ok := x.(string); ok {
				scope = append(scope, s)
			}
		}
	}
	if len(scope) == 0 {
		scope = p.scope()
	}
	out, err := ExploreGraph(ctx, p.tenantID, p.datasetIDs, topic, keywords, scope)
	if err != nil {
		return ToolResult{Error: err.Error()}
	}
	return ToolResult{Chunks: out.Chunks, Answer: out.Answer}
}

// runSQLTool forwards structured_query to the runner's SQL retrieval path.
func (p *Pipeline) runSQLTool(ctx context.Context, args map[string]interface{}) ToolResult {
	if p.runner == nil {
		return ToolResult{}
	}
	return p.runner.runSQLTool(ctx, args)
}

// runInspectorTool dispatches the four inspector tools over the shared kbinfos.
func (p *Pipeline) runInspectorTool(toolName string, args map[string]interface{}) ToolResult {
	chunks := p.chunksSnapshot()
	switch toolName {
	case "inspector_open_context":
		return ToolResult{Chunks: InspectorOpenContext(chunks, stringValue(args["chunk_id"]))}
	case "inspector_compare":
		var ids []string
		switch v := args["chunk_ids"].(type) {
		case []string:
			ids = v
		case []interface{}:
			for _, x := range v {
				if s, ok := x.(string); ok {
					ids = append(ids, s)
				}
			}
		}
		return ToolResult{Chunks: InspectorCompareSources(chunks, ids)}
	case "inspector_grep_within":
		return ToolResult{Chunks: InspectorGrepWithin(chunks, stringValue(args["doc_id"]), stringValue(args["pattern"]))}
	case "inspector_request_adjacent":
		count := 3
		if n, ok := args["count"].(float64); ok {
			count = int(n)
		}
		return ToolResult{Chunks: InspectorRequestAdjacent(chunks, stringValue(args["chunk_id"]), stringValue(args["direction"]), count)}
	}
	return ToolResult{Error: "Unknown inspector tool: " + toolName}
}

// runWikiTool runs wiki_query via the runner's wiki service.
func (p *Pipeline) runWikiTool(ctx context.Context, args map[string]interface{}) ToolResult {
	if p.runner == nil {
		return ToolResult{}
	}
	query := stringValue(args["query"])
	keywords := stringValue(args["keywords"])
	chunks, _ := p.runner.wikiSearch(ctx, query, keywords)
	return ToolResult{Chunks: chunks}
}

// runStructureTool runs ontology_navigate / mindmap_navigate via NavigateStructure.
func (p *Pipeline) runStructureTool(ctx context.Context, toolName string, args map[string]interface{}) ToolResult {
	if !p.HasRoutedScope() {
		return ToolResult{} // gated: requires a routed doc scope
	}
	topic := stringValue(args["topic"])
	keywords := stringValue(args["keywords"])
	var scope []string
	switch v := args["doc_scope"].(type) {
	case []string:
		scope = v
	case []interface{}:
		for _, x := range v {
			if s, ok := x.(string); ok {
				scope = append(scope, s)
			}
		}
	}
	if len(scope) == 0 {
		scope = p.scope()
	}
	raw, err := NavigateStructure(ctx, p.tenantID, toolName, structureNavArgs{
		Topic: topic, Keywords: keywords, DocScope: scope,
	})
	if err != nil {
		return ToolResult{Error: err.Error()}
	}
	var res struct {
		Chunks []map[string]interface{} `json:"chunks"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return ToolResult{}
	}
	return ToolResult{Chunks: res.Chunks}
}

// GetChunks retrieves raw chunks by global evidence id (mirrors Python
// Pipeline.get_chunks).
func (p *Pipeline) GetChunks(evidenceIDs []int) map[int]map[string]interface{} {
	chunks := p.chunksSnapshot()
	out := map[int]map[string]interface{}{}
	for _, eid := range evidenceIDs {
		if eid >= 0 && eid < len(chunks) {
			out[eid] = chunks[eid]
		}
	}
	return out
}

// implementedTools is the whitelist of tools the Pipeline can actually dispatch.
// Tools listed in a mode's AvailableTools but not here (structured_query,
// inspector_*) are NOT yet implemented in the Go harness, so they are filtered
// out to keep the LLM from calling an unknown tool.
var implementedTools = map[string]bool{
	"hybrid_search":              true,
	"vector_search":              true,
	"bm25_search":                true,
	"web_search":                 true,
	"dataset_navigation_by_tree": true,
	"wiki_query":                 true,
	"ontology_navigate":          true,
	"mindmap_navigate":           true,
	"graph_explore":              true,
	"structured_query":           true,
	"inspector_open_context":     true,
	"inspector_compare":          true,
	"inspector_grep_within":      true,
	"inspector_request_adjacent": true,
}

// AvailableTools filters a mode's tool list to (a) tools the Pipeline can
// actually dispatch, and (b) compilation-availability (mirrors Python
// filter_available_tools). When the compilation map is empty, compilation gating
// is disabled but the implementation whitelist still applies.
func (p *Pipeline) AvailableTools(modeTools []string) []string {
	var out []string
	for _, name := range modeTools {
		if !implementedTools[name] {
			continue
		}
		if len(p.compilation) > 0 {
			if req, ok := toolCompilationRequirement(name); ok && !p.compilationSatisfied(req) {
				continue
			}
		}
		out = append(out, name)
	}
	return out
}

func (p *Pipeline) compilationSatisfied(wanted []string) bool {
	for _, comps := range p.compilation {
		for _, w := range wanted {
			if comps[w] {
				return true
			}
		}
	}
	return false
}

// toolCompilationRequirement returns the compilation artifact a tool needs
// (mirrors Python TOOL_REGISTRY requires_compilation / compilation_type).
func toolCompilationRequirement(name string) ([]string, bool) {
	switch name {
	case "ontology_navigate":
		return []string{"toc", "tree", "page_index", "timeline", "raptor"}, true
	case "mindmap_navigate":
		return []string{"mindmap"}, true
	case "graph_explore":
		return []string{"graph", "knowledge_graph"}, true
	case "wiki_query":
		return []string{"wiki"}, true
	default:
		return nil, false
	}
}
