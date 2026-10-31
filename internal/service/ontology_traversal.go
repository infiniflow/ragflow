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
	"context"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// The typed walk exists because a similarity search cannot reach an entity the
// question never names.
//
// A multi-hop question is usually joined by an entity that appears in no
// sentence the reader wrote: "which papers used a model that includes an ocean
// variable" is answered by a row in one document (paper —uses→ model) and a row
// in another (model —has_variable→ variable), and the model's name is the only
// thing that connects them. Neither row reads like the question, so neither is
// retrieved by ranking; the join is structural, not semantic.
//
// So the walk is deterministic: the caller names the starting entities and the
// constraints (which properties, which classes, how many hops), and this service
// follows the assertion rows. No model decides the path, and every hop reports
// what it matched, what it refused and why — a walk that went nowhere has to be
// distinguishable from a corpus that says nothing.

// Bounds. Hops are capped because a typed walk grows by branching factor, not by
// one row per level: three hops over a graph with twenty assertions per entity is
// already eight thousand candidate rows, and a question needing more than three
// is a question about the model rather than through it.
const (
	OntologyTraversalMaxHops       = 3
	OntologyTraversalDefaultHops   = 2
	ontologyTraversalDefaultPaths  = 8
	ontologyTraversalDefaultRows   = 200
	ontologyTraversalFrontierBatch = 64
)

// OntologyTraversalRequest is one typed walk.
//
// Every field is explicit input. The starting entities are named rather than
// derived from the question: a start the reader can see and correct is worth
// more than a derivation nobody can argue with.
type OntologyTraversalRequest struct {
	UserID     string
	DatasetIDs []string
	// OntologyTemplateID names the model whose declarations constrain the walk
	// and whose class hierarchy decides whether an endpoint satisfies a domain or
	// range. Required: without it a walk cannot tell a declared property from an
	// invented one.
	OntologyTemplateID string
	OntologyVersion    int
	// Starts are the entity names to walk from, as the compiled rows spell them.
	Starts []string
	// Hops bounds the walk. Zero means the default; above the cap is clamped.
	Hops int
	// AllowProperties restricts every hop to these object properties. Empty means
	// every declared object property.
	AllowProperties []string
	// AllowTypes restricts the classes an entity reached by the walk may have.
	// Empty means no class restriction.
	AllowTypes []string
	// MaxPaths bounds the returned paths; MaxRows bounds the rows read per hop.
	MaxPaths int
	MaxRows  int
	// DocumentScope is the conversation's readable document set, used to mark a
	// step whose source cannot be cited. Nil means unrestricted.
	DocumentScope []string
}

// OntologyTraversalStep is one assertion the walk followed.
type OntologyTraversalStep struct {
	From      string `json:"from"`
	Property  string `json:"property"`
	To        string `json:"to"`
	FromType  string `json:"from_type,omitempty"`
	ToType    string `json:"to_type,omitempty"`
	Direction string `json:"direction"`
	// SourceChunkIDs are the chunks the assertion was compiled from, so a path
	// can be checked against the documents rather than taken on trust.
	SourceChunkIDs []string `json:"source_chunk_ids,omitempty"`
}

// OntologyTraversalPath is one chain of assertions from a starting entity.
type OntologyTraversalPath struct {
	Start string                  `json:"start"`
	Nodes []string                `json:"nodes"`
	Steps []OntologyTraversalStep `json:"steps"`
}

// OntologyTraversalHop reports what one hop did. It exists so a failed walk can
// be attributed: an empty result caused by "the frontier found nothing" is a
// corpus answer, one caused by "domain/range rejected the rows" is a declaration
// answer, and the two call for different next actions.
type OntologyTraversalHop struct {
	Hop                   int  `json:"hop"`
	Frontier              int  `json:"frontier"`
	Rows                  int  `json:"rows"`
	Matched               int  `json:"matched"`
	RejectedByDeclaration int  `json:"rejected_by_declaration"`
	RejectedByAllowList   int  `json:"rejected_by_allow_list"`
	Truncated             bool `json:"truncated,omitempty"`
}

// OntologyTraversalResult is the walk together with its accounting.
type OntologyTraversalResult struct {
	Paths []OntologyTraversalPath `json:"paths"`
	Hops  []OntologyTraversalHop  `json:"hops"`
	Model OntologyModelRef        `json:"model"`
	// DegradeCode is set when the walk could not be constrained by the model.
	// The walk still runs, unnarrowed by declarations, because an unreadable
	// model must not make the assertions unreachable — but the caller has to see
	// which of the two it got.
	DegradeCode   string `json:"degrade_code,omitempty"`
	DegradeReason string `json:"degrade_reason,omitempty"`
	// Truncated reports that a bound was hit, so a short answer is not read as a
	// complete one.
	Truncated bool `json:"truncated,omitempty"`
}

// ontologyTraversalRowReader reads assertion rows for one hop. It is a seam so
// the walk's logic (frontier, pruning, path assembly) is testable without a
// document engine.
type ontologyTraversalRowReader func(
	ctx context.Context, tenantID, datasetID string,
	filter map[string]interface{}, limit int,
) ([]map[string]interface{}, error)

// ontologyTraversalScopeResolver resolves the tenants that own the datasets.
// It is deliberately smaller than ontologyABoxScopeResolver: a walk needs no
// embedding model, and initialising one would make a structural lookup pay for a
// dense-retrieval dependency it never uses.
type ontologyTraversalScopeResolver interface {
	ResolveTenants(ctx context.Context, userID string, datasetIDs []string) ([]string, error)
}

// OntologyTraversalService walks compiled assertions along their declared
// properties.
type OntologyTraversalService struct {
	scopeResolver   ontologyTraversalScopeResolver
	structureLoader ontologyStructureLoader
	rowReader       ontologyTraversalRowReader
}

// NewOntologyTraversalService wires the production boundaries.
func NewOntologyTraversalService() *OntologyTraversalService {
	return &OntologyTraversalService{
		scopeResolver:   &kbTenantResolver{kbDAO: dao.NewKnowledgebaseDAO()},
		structureLoader: loadOntologyTemplate,
		rowReader:       readOntologyTraversalRows,
	}
}

// readOntologyTraversalRows reads one page of assertion rows per dataset.
func readOntologyTraversalRows(
	ctx context.Context, tenantID, datasetID string,
	filter map[string]interface{}, limit int,
) ([]map[string]interface{}, error) {
	fields := []string{
		"prop_kwd", "from_entity_kwd", "to_entity_kwd",
		"from_type_kwd", "to_type_kwd", "source_chunk_ids", "doc_id",
	}
	rows, _, err := graphRowSearch(ctx, tenantID, datasetID, fields, filter, nil, 0, limit, nil)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out, nil
}

// Traverse walks the assertions reachable from the starting entities.
func (s *OntologyTraversalService) Traverse(ctx context.Context, req *OntologyTraversalRequest) (*OntologyTraversalResult, error) {
	if s == nil || s.scopeResolver == nil || s.rowReader == nil || s.structureLoader == nil {
		return nil, fmt.Errorf("ontology traversal is not wired")
	}
	if req == nil {
		return nil, fmt.Errorf("a request is required")
	}
	if strings.TrimSpace(req.UserID) == "" {
		return nil, fmt.Errorf("a caller is required to check dataset ownership")
	}
	if len(req.DatasetIDs) == 0 {
		return nil, fmt.Errorf("dataset_ids is required")
	}
	starts := dedupeStrings(req.Starts)
	if len(starts) == 0 {
		return nil, fmt.Errorf("at least one starting entity is required")
	}

	tenants, err := s.scopeResolver.ResolveTenants(ctx, req.UserID, req.DatasetIDs)
	if err != nil {
		return nil, err
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("no tenant owns the given datasets")
	}
	tenantID := tenants[0]

	out := &OntologyTraversalResult{}
	hops := req.Hops
	if hops <= 0 {
		hops = OntologyTraversalDefaultHops
	}
	if hops > OntologyTraversalMaxHops {
		hops = OntologyTraversalMaxHops
	}
	maxPaths := req.MaxPaths
	if maxPaths <= 0 {
		maxPaths = ontologyTraversalDefaultPaths
	}
	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = ontologyTraversalDefaultRows
	}

	declared, ref, modelErr := s.structureLoader(ctx, tenantID, req.OntologyTemplateID, req.OntologyVersion)
	out.Model = ref
	if modelErr != nil {
		// The walk still runs: the assertions are real whether or not the model
		// that declared them can be read, and refusing would make a broken model
		// hide the corpus. What is lost is the pruning, and the degrade says so.
		out.DegradeCode = OntologyABoxDegradeNoModel
		out.DegradeReason = modelErr.Error()
		declared = declaredOntology{classes: map[string]declaredClass{}, props: map[string]declaredProperty{}}
	}

	allowProps := dedupeStrings(req.AllowProperties)
	allowTypes := dedupeStrings(req.AllowTypes)
	children := declaredChildren(declared)
	allowedTypes := expandDeclaredClasses(allowTypes, children)

	frontier := starts
	visited := make(map[string]bool, len(starts))
	for _, start := range starts {
		visited[start] = true
	}
	// edges keeps every followed assertion, in the order it was found, so the
	// path assembly below works off the walk that actually happened rather than a
	// second scan.
	edges := make([]OntologyTraversalStep, 0, maxRows)

	for hop := 1; hop <= hops; hop++ {
		if len(frontier) == 0 {
			// Nothing left to expand: the walk ended here, and a hop that read no
			// row is not a hop the caller should count as attempted.
			break
		}
		stats := OntologyTraversalHop{Hop: hop, Frontier: len(frontier)}
		next := make([]string, 0, len(frontier))
		seenNext := make(map[string]bool, len(frontier))
		for _, batch := range batchStrings(frontier, ontologyTraversalFrontierBatch) {
			rows, err := s.readHop(ctx, tenantID, req.DatasetIDs, batch, maxRows)
			if err != nil {
				return nil, err
			}
			stats.Rows += len(rows)
			if len(rows) >= maxRows {
				stats.Truncated = true
				out.Truncated = true
			}
			for _, row := range rows {
				step, ok := traversalStepFromRow(row)
				if !ok {
					continue
				}
				if len(allowProps) > 0 && !containsStringValue(allowProps, step.Property) {
					stats.RejectedByAllowList++
					continue
				}
				if !traversalUsesDeclaredProperty(declared, step.Property) {
					// With a readable model, an undeclared property is drift: the
					// walk refuses it here so a path cannot be built out of a
					// relation the model never declared. Without a model there is
					// nothing to check against, and the degrade already said so.
					if modelErr == nil {
						stats.RejectedByDeclaration++
						continue
					}
				}
				if modelErr == nil && !traversalSatisfiesDeclaredEnds(declared, step) {
					stats.RejectedByDeclaration++
					continue
				}
				if len(allowedTypes) > 0 {
					if !traversalEndpointAllowed(step, batch, allowedTypes) {
						stats.RejectedByAllowList++
						continue
					}
				}
				stats.Matched++
				edges = append(edges, step)
				other := step.To
				if !containsStringValue(batch, step.From) {
					other = step.From
				}
				if other == "" || visited[other] {
					continue
				}
				if !seenNext[other] {
					seenNext[other] = true
					next = append(next, other)
				}
			}
		}
		// Marking the whole next frontier visited is what makes the walk
		// terminate on cyclic assertions without dropping the edges that close a
		// loop: the path assembly still sees them, the frontier does not revisit
		// them.
		for _, name := range next {
			visited[name] = true
		}
		frontier = next
		out.Hops = append(out.Hops, stats)
	}

	out.Paths = assembleOntologyTraversalPaths(starts, edges, hops, maxPaths)
	if len(out.Paths) >= maxPaths {
		out.Truncated = true
	}
	return out, nil
}

// readHop reads the assertions touching any entity of one batch, in either
// direction, from every dataset in the knowledge scope.
func (s *OntologyTraversalService) readHop(
	ctx context.Context, tenantID string, datasetIDs, frontier []string, limit int,
) ([]map[string]interface{}, error) {
	// Only assertions: an entity row has no endpoints to follow, and a
	// dropped_relation row is one the declaration already refused.
	base := map[string]interface{}{"knowledge_graph_kwd": []string{"relation"}}
	out := make([]map[string]interface{}, 0, limit)
	for _, direction := range []string{"from_entity_kwd", "to_entity_kwd"} {
		filter := mergeOntologyFilters(base, map[string]interface{}{direction: frontier})
		for _, datasetID := range datasetIDs {
			rows, err := s.rowReader(ctx, tenantID, datasetID, filter, limit)
			if err != nil {
				return nil, err
			}
			out = append(out, rows...)
			if len(out) >= limit {
				return out[:limit], nil
			}
		}
	}
	return out, nil
}

// traversalStepFromRow reads one assertion row into a step.
func traversalStepFromRow(row map[string]interface{}) (OntologyTraversalStep, bool) {
	from := strings.TrimSpace(firstStringValue(row["from_entity_kwd"]))
	to := strings.TrimSpace(firstStringValue(row["to_entity_kwd"]))
	property := strings.TrimSpace(firstStringValue(row["prop_kwd"]))
	if from == "" || to == "" || property == "" {
		return OntologyTraversalStep{}, false
	}
	return OntologyTraversalStep{
		From:           from,
		Property:       property,
		To:             to,
		FromType:       strings.TrimSpace(firstStringValue(row["from_type_kwd"])),
		ToType:         strings.TrimSpace(firstStringValue(row["to_type_kwd"])),
		Direction:      "forward",
		SourceChunkIDs: ontologyRowStringSlice(row["source_chunk_ids"]),
	}, true
}

// traversalUsesDeclaredProperty reports whether the model declares the property
// as an object property. A datatype property is not an edge, so a walk cannot
// follow it.
func traversalUsesDeclaredProperty(declared declaredOntology, property string) bool {
	prop, ok := declared.props[property]
	if !ok {
		return false
	}
	return prop.isObject()
}

// traversalSatisfiesDeclaredEnds checks the assertion's endpoint classes against
// the declared domain and range.
//
// An endpoint with no class stamp is kept: the entity was compiled in another
// batch and leaving it out would lose real data. A stamped endpoint whose class
// does not satisfy the declaration is refused, because following it would build
// a path the model says cannot exist.
func traversalSatisfiesDeclaredEnds(declared declaredOntology, step OntologyTraversalStep) bool {
	prop, ok := declared.props[step.Property]
	if !ok {
		return false
	}
	if step.FromType != "" && len(prop.domains) > 0 &&
		!ontologyTypeSatisfies(declared, step.FromType, prop.domains) {
		return false
	}
	if step.ToType != "" && len(prop.ranges) > 0 &&
		!ontologyTypeSatisfies(declared, step.ToType, prop.ranges) {
		return false
	}
	return true
}

// ontologyTypeSatisfies reports whether an observed class is one of the declared
// classes or a declared descendant of one. The hierarchy is what makes an
// assertion typed with a subclass satisfy a rule written against its parent.
func ontologyTypeSatisfies(declared declaredOntology, observed string, declaredClasses []string) bool {
	observed = strings.TrimSpace(observed)
	if observed == "" {
		return false
	}
	for _, want := range declaredClasses {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		if strings.EqualFold(observed, want) {
			return true
		}
		for _, ancestor := range declared.ancestorChain(observed) {
			if strings.EqualFold(ancestor, want) {
				return true
			}
		}
	}
	return false
}

// traversalEndpointAllowed applies the caller's class restriction to the end of
// the step the walk is about to move to.
func traversalEndpointAllowed(step OntologyTraversalStep, frontier, allowed []string) bool {
	movingForward := containsStringValue(frontier, step.From)
	observed := step.ToType
	if !movingForward {
		observed = step.FromType
	}
	if observed == "" {
		// Unstamped endpoints cannot be checked, and dropping them would make a
		// class restriction silently lose the rows it cannot judge. They stay,
		// and the receipt's per-hop counts still show how many were judged.
		return true
	}
	for _, want := range allowed {
		if strings.EqualFold(observed, want) {
			return true
		}
	}
	return false
}

// assembleOntologyTraversalPaths turns the followed edges into chains.
//
// The walk follows edges in both directions, so a step may be stored as
// from→to while the path needs to cross it to→from. Assembly therefore treats
// each step as an undirected link and rebuilds direction from the path, which is
// also what keeps a path's own order honest: every step reports the direction it
// was actually walked.
func assembleOntologyTraversalPaths(starts []string, edges []OntologyTraversalStep, maxHops, maxPaths int) []OntologyTraversalPath {
	if len(edges) == 0 {
		return nil
	}
	adjacent := make(map[string][]int, len(edges)*2)
	for i, edge := range edges {
		adjacent[edge.From] = append(adjacent[edge.From], i)
		adjacent[edge.To] = append(adjacent[edge.To], i)
	}
	paths := make([]OntologyTraversalPath, 0, maxPaths)
	for _, start := range starts {
		if len(paths) >= maxPaths {
			break
		}
		walkTraversalPaths(start, start, nil, nil, adjacent, edges, maxHops, maxPaths, &paths)
	}
	return paths
}

func walkTraversalPaths(
	start, current string,
	nodes []string,
	steps []OntologyTraversalStep,
	adjacent map[string][]int,
	edges []OntologyTraversalStep,
	remaining, maxPaths int,
	out *[]OntologyTraversalPath,
) {
	if len(*out) >= maxPaths {
		return
	}
	pathNodes := append(append([]string(nil), nodes...), current)
	if len(steps) > 0 {
		*out = append(*out, OntologyTraversalPath{
			Start: start,
			Nodes: pathNodes,
			Steps: append([]OntologyTraversalStep(nil), steps...),
		})
	}
	if remaining <= 0 {
		return
	}
	for _, index := range adjacent[current] {
		edge := edges[index]
		next := edge.To
		direction := "forward"
		if next == current {
			next = edge.From
			direction = "reverse"
		}
		if next == "" || containsStringValue(pathNodes, next) {
			continue
		}
		stepped := edge
		stepped.Direction = direction
		walkTraversalPaths(start, next, pathNodes, append(steps, stepped), adjacent, edges, remaining-1, maxPaths, out)
		if len(*out) >= maxPaths {
			return
		}
	}
}

// ontologyRowStringSlice reads a keyword column that an engine may hand back as a
// list or as one string.
func ontologyRowStringSlice(raw interface{}) []string {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	case []string:
		return dedupeStrings(v)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return dedupeStrings(out)
	}
	return nil
}

// batchStrings splits a frontier into query-sized batches, keeping order.
func batchStrings(values []string, size int) [][]string {
	if size <= 0 {
		size = ontologyTraversalFrontierBatch
	}
	out := make([][]string, 0, (len(values)+size-1)/size)
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		out = append(out, values[start:end])
	}
	return out
}

// kbTenantResolver resolves only the owners of the given datasets, which is all a
// structural walk needs.
type kbTenantResolver struct {
	kbDAO *dao.KnowledgebaseDAO
}

func (r *kbTenantResolver) ResolveTenants(ctx context.Context, userID string, datasetIDs []string) ([]string, error) {
	if r == nil || r.kbDAO == nil {
		return nil, fmt.Errorf("dataset lookup is not wired")
	}
	seen := make(map[string]bool, len(datasetIDs))
	tenants := make([]string, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		if err := CheckDatasetAccess(ctx, permission.Subject{UserID: userID}, datasetID, permission.OperationUse); err != nil {
			return nil, fmt.Errorf("only the owner of dataset %s is authorized for this operation", datasetID)
		}
		kb, err := r.kbDAO.GetByID(ctx, dao.DB, datasetID)
		if err != nil || kb == nil {
			return nil, fmt.Errorf("dataset %s not found", datasetID)
		}
		if kb.TenantID != "" && !seen[kb.TenantID] {
			seen[kb.TenantID] = true
			tenants = append(tenants, kb.TenantID)
		}
	}
	sort.Strings(tenants)
	return tenants, nil
}
