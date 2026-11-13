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
	"sort"
	"strings"

	"ragflow/internal/engine/types"
)

const (
	// ontologyGraphDefaultLimit is what one canvas asks for when it says nothing.
	// It is the node budget, not a page size: a graph cannot show page 7 without
	// showing pages 1-6, so the read is bounded at the source and the remainder
	// is reported instead of fetched.
	ontologyGraphDefaultLimit = 60
	ontologyGraphMaxLimit     = 200
	// ontologyGraphEdgeCap bounds one directed relation page. Beyond it an edge
	// list is no longer something a reader can follow, and the class-level
	// aggregate is the right view instead.
	ontologyGraphEdgeCap = 400
	// ontologyGraphMaxClasses bounds the class fan-out a hop may pull in. A hub
	// class (an action, an organisation) touches most of the vocabulary, and
	// without a cap one hop would swallow the whole ontology.
	ontologyGraphMaxClasses = 8
	ontologyGraphDefaultHop = 1
	ontologyGraphMaxHop     = 2
	// ontologyGraphMinPerClass keeps a small class visible next to a large one:
	// splitting the budget evenly would round it down to zero.
	ontologyGraphMinPerClass = 12
)

// OntologyGraphInput is one request for the ABox graph of a scope.
//
// Node turns the request into "expand this instance": the walk starts at one
// entity instead of at a class, and Direction says which assertions are walked.
// The two modes are mutually exclusive — a node request ignores Class and Hop —
// because they answer different questions ("what is in this class" versus "what
// touches this instance") and one page shape cannot be both.
type OntologyGraphInput struct {
	TenantID   string
	DatasetID  string
	Class      string
	DocumentID string
	TemplateID string
	Node       string
	Direction  string
	Hop        int
	Limit      int
}

// Neighbour direction values. "both" is the default: a canvas that expands a
// node asks for its neighbourhood, not for one half of it.
const (
	ontologyNeighborDirectionBoth = "both"
	ontologyNeighborDirectionOut  = "out"
	ontologyNeighborDirectionIn   = "in"
)

// normalizeOntologyNeighborDirection folds anything unrecognised to "both": an
// unknown direction is a client bug, and answering with the superset keeps the
// canvas usable instead of failing the whole expansion.
func normalizeOntologyNeighborDirection(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ontologyNeighborDirectionOut:
		return ontologyNeighborDirectionOut
	case ontologyNeighborDirectionIn:
		return ontologyNeighborDirectionIn
	default:
		return ontologyNeighborDirectionBoth
	}
}

// ontologyNeighborEndpointColumns is the directional read plan. The two
// directions cannot be one query: the engine ANDs filters, so asking for
// from_entity_kwd in {x} AND to_entity_kwd in {x} answers "self loops" and
// misses every real assertion.
func ontologyNeighborEndpointColumns(direction string) []string {
	switch direction {
	case ontologyNeighborDirectionOut:
		return []string{"from_entity_kwd"}
	case ontologyNeighborDirectionIn:
		return []string{"to_entity_kwd"}
	default:
		return []string{"from_entity_kwd", "to_entity_kwd"}
	}
}

// OntologyGraphPage is the induced subgraph a canvas draws: the instances of a
// class, the classes one hop out, and the assertions between them.
//
// Truncated is not an error: it says the class holds more instances than the
// budget drew, so the UI must offer the list rather than imply the canvas is
// the whole ABox.
type OntologyGraphPage struct {
	Class     string                   `json:"class"`
	Hop       int                      `json:"hop"`
	Total     int64                    `json:"total"`
	Drawn     int                      `json:"drawn"`
	Truncated bool                     `json:"truncated"`
	Classes   []string                 `json:"classes"`
	Entities  []StructureGraphNode     `json:"entities"`
	Relations []StructureGraphRelation `json:"relations"`
}

// normalizeOntologyGraphInput clamps the budget and the hop count.
func normalizeOntologyGraphInput(in OntologyGraphInput) OntologyGraphInput {
	in.Class = strings.TrimSpace(in.Class)
	in.Node = strings.TrimSpace(in.Node)
	in.Direction = normalizeOntologyNeighborDirection(in.Direction)
	if in.Limit <= 0 {
		in.Limit = ontologyGraphDefaultLimit
	}
	if in.Limit > ontologyGraphMaxLimit {
		in.Limit = ontologyGraphMaxLimit
	}
	if in.Hop <= 0 {
		in.Hop = ontologyGraphDefaultHop
	}
	if in.Hop > ontologyGraphMaxHop {
		in.Hop = ontologyGraphMaxHop
	}
	return in
}

// GetOntologyGraph answers the ABox canvas.
//
// The list view and this one read the same rows, but they cannot share a query:
// a list can say "12-30 of 240" and still let the reader reach row 240, while a
// graph has to decide up front how much it will draw. So the budget lives here
// (node limit, edge cap, class fan-out), the remainder is reported as
// Truncated, and the list stays the only view that can reach the last row.
//
// The walk starts at one class and widens one hop at a time: each hop reads the
// assertions touching the classes found so far and adds the classes those
// assertions point at. That is what makes a class view show its neighbours
// instead of a cloud of same-class dots with no relations between them.
func (s *DatasetArtifactService) GetOntologyGraph(ctx context.Context, in OntologyGraphInput) (*OntologyGraphPage, error) {
	in = normalizeOntologyGraphInput(in)
	if in.Node != "" {
		return s.ontologyGraphNeighbors(ctx, in)
	}
	page := &OntologyGraphPage{
		Class:     in.Class,
		Hop:       in.Hop,
		Classes:   []string{},
		Entities:  []StructureGraphNode{},
		Relations: []StructureGraphRelation{},
	}

	classSet := []string{}
	if in.Class != "" {
		classSet = []string{in.Class}
	}

	var relations []StructureGraphRelation
	for hop := 0; hop < in.Hop; hop++ {
		edges, truncated, err := s.ontologyGraphRelations(ctx, in, classSet)
		if err != nil {
			return nil, err
		}
		if truncated {
			page.Truncated = true
		}
		relations = mergeRelations(append(relations, edges...))
		next := ontologyGraphClassesOf(classSet, relations)
		if len(next) == len(classSet) {
			break
		}
		classSet = next
	}

	entities, total, entitiesTruncated, err := s.ontologyGraphEntities(ctx, in, classSet)
	if err != nil {
		return nil, err
	}
	if entitiesTruncated {
		page.Truncated = true
	}
	page.Total = total
	page.Classes = classSet

	nodes, edges := ontologyGraphAssemble(entities, relations, in.Limit)
	page.Entities = nodes
	page.Relations = edges
	page.Drawn = len(nodes)
	if int64(len(nodes)) < total {
		page.Truncated = true
	}
	markEntityClassConflicts(page.Entities)
	markRelationConflicts(page.Relations)
	return page, nil
}

// ontologyGraphRelations reads the assertions touching the class set.
//
// Two directed reads, not one: the engine ANDs filters, so asking for
// from_type in set AND to_type in set would answer "edges inside the set" and
// miss every edge that leaves it. Stringing the two directions together costs
// one extra query and is the difference between a graph and a closed clique.
func (s *DatasetArtifactService) ontologyGraphRelations(
	ctx context.Context, in OntologyGraphInput, classSet []string,
) ([]StructureGraphRelation, bool, error) {
	reads := []string{"from_type_kwd", "to_type_kwd"}
	if len(classSet) == 0 {
		// No class named: one unfiltered page is the scope-wide view.
		reads = []string{""}
	}
	out := []StructureGraphRelation{}
	truncated := false
	for _, key := range reads {
		filter := ontologyInstanceFilter("relation", in.TemplateID, in.DocumentID)
		if key != "" {
			filter[key] = classSet
		}
		rows, total, err := graphRowSearch(ctx, in.TenantID, in.DatasetID, graphRelationFields, filter,
			nil, 0, ontologyGraphEdgeCap, nil)
		if err != nil {
			return nil, false, err
		}
		if total > int64(len(rows)) {
			truncated = true
		}
		for _, row := range rows {
			if edge := projectRelation(row); edge != nil {
				if evidence := ontologyEvidenceOf(row); len(evidence) > 0 {
					edge["evidence"] = evidence
				}
				out = append(out, edge)
			}
		}
	}
	return mergeRelations(out), truncated, nil
}

// ontologyGraphClassesOf widens the class set with the classes the edges point
// at, keeping the original order so the walk is reproducible.
func ontologyGraphClassesOf(classSet []string, relations []StructureGraphRelation) []string {
	out := append([]string{}, classSet...)
	seen := map[string]bool{}
	for _, c := range out {
		seen[strings.ToLower(strings.TrimSpace(c))] = true
	}
	add := func(raw interface{}) {
		value := strings.TrimSpace(graphStr(raw))
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if seen[key] || len(out) >= ontologyGraphMaxClasses {
			return
		}
		seen[key] = true
		out = append(out, value)
	}
	for _, edge := range relations {
		add(edge["source_type"])
		add(edge["target_type"])
	}
	return out
}

// ontologyGraphEntities reads the instances of each class in the set.
//
// The budget is split but floored: an even split would give a 7-instance class
// zero nodes next to a 268-instance one, and the small class is exactly the one
// whose emptiness a reader came to check.
func (s *DatasetArtifactService) ontologyGraphEntities(
	ctx context.Context, in OntologyGraphInput, classSet []string,
) ([]StructureGraphNode, int64, bool, error) {
	classes := classSet
	if len(classes) == 0 {
		classes = []string{""}
	}
	perClass := in.Limit
	if len(classes) > 1 {
		perClass = in.Limit/len(classes) + 1
		if perClass < ontologyGraphMinPerClass {
			perClass = ontologyGraphMinPerClass
		}
	}
	out := []StructureGraphNode{}
	var total int64
	truncated := false
	for i, class := range classes {
		filter := ontologyInstanceFilter("entity", in.TemplateID, in.DocumentID)
		ontologyTermFilter(filter, "entity_type_kwd", class)
		rows, classTotal, err := graphRowSearch(ctx, in.TenantID, in.DatasetID, graphEntityFields, filter,
			(&types.OrderByExpr{}).Desc("mention_count_int"), 0, perClass, nil)
		if err != nil {
			return nil, 0, false, err
		}
		if i == 0 {
			// The scope's own count drives "drawn N of TOTAL": the walked classes
			// are context, not the question the reader asked.
			total = classTotal
		}
		if classTotal > int64(len(rows)) {
			truncated = true
		}
		for _, row := range rows {
			if node := projectEntity(row); node != nil {
				if evidence := ontologyEvidenceOf(row); len(evidence) > 0 {
					node["evidence"] = evidence
				}
				out = append(out, node)
			}
		}
	}
	return mergeEntities(out), total, truncated, nil
}

// ontologyGraphAssemble orders the nodes and drops what does not connect.
//
// Connected nodes come first: an instance with no drawn edge is still real, but
// it is not what a reader opened the canvas for, so it only gets a slot when the
// connected core leaves budget over. Edges whose endpoints did not make the cut
// are dropped rather than drawn to a missing node.
func ontologyGraphAssemble(
	entities []StructureGraphNode, relations []StructureGraphRelation, limit int,
) ([]StructureGraphNode, []StructureGraphRelation) {
	if limit <= 0 {
		limit = ontologyGraphDefaultLimit
	}
	degree := map[string]int{}
	for _, edge := range relations {
		for _, end := range []interface{}{edge["from"], edge["to"]} {
			if name := normalizeGraphName(end); name != "" {
				degree[name]++
			}
		}
	}

	connected := make([]StructureGraphNode, 0, len(entities))
	isolated := make([]StructureGraphNode, 0, len(entities))
	for _, node := range entities {
		if degree[normalizeGraphName(node["name"])] > 0 {
			connected = append(connected, node)
			continue
		}
		isolated = append(isolated, node)
	}
	sort.SliceStable(connected, func(i, j int) bool {
		return degree[normalizeGraphName(connected[i]["name"])] >
			degree[normalizeGraphName(connected[j]["name"])]
	})

	nodes := connected
	if len(nodes) < limit {
		nodes = append(nodes, isolated...)
	}
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}

	kept := map[string]bool{}
	for _, node := range nodes {
		kept[normalizeGraphName(node["name"])] = true
	}
	edges := make([]StructureGraphRelation, 0, len(relations))
	for _, edge := range relations {
		if kept[normalizeGraphName(edge["from"])] && kept[normalizeGraphName(edge["to"])] {
			edges = append(edges, edge)
		}
	}
	return nodes, edges
}

// normalizeGraphName is the join key between an entity and an edge endpoint.
// The writer keeps names verbatim, so the comparison is case/space insensitive
// rather than exact: a row compiled by an older writer may carry padding.
func normalizeGraphName(raw interface{}) string {
	return strings.ToLower(strings.TrimSpace(graphStr(raw)))
}

// ontologyNeighborPeer is one endpoint of the expanded instance, together with
// the class the assertion claims for it. The class is a fallback only: the
// entity row's own entity_type_kwd wins whenever the row is in reach, because a
// relation compiled at document scope can name a class the dataset-level entity
// row has since corrected.
type ontologyNeighborPeer struct {
	Name   string
	Type   string
	Weight int
}

// ontologyNeighborPeers collects the far end of every assertion touching the
// centre, ordered by how many assertions reach it.
//
// The order is the budget policy: a hub instance with 300 neighbours has to
// spend the node budget on the ones it actually talks to, and Weight is that
// signal. Ties break on the normalised name so two identical requests draw the
// same graph.
func ontologyNeighborPeers(
	center string, relations []StructureGraphRelation, budget int,
) ([]ontologyNeighborPeer, bool) {
	key := normalizeGraphName(center)
	order := []string{}
	byKey := map[string]*ontologyNeighborPeer{}
	for _, relation := range relations {
		from := strings.TrimSpace(graphStr(relation["from"]))
		to := strings.TrimSpace(graphStr(relation["to"]))
		peer, peerType := "", ""
		switch {
		case key != "" && normalizeGraphName(from) == key:
			peer, peerType = to, strings.TrimSpace(graphStr(relation["target_type"]))
		case key != "" && normalizeGraphName(to) == key:
			peer, peerType = from, strings.TrimSpace(graphStr(relation["source_type"]))
		default:
			// An edge that does not name the centre is not this node's
			// neighbourhood; the merge upstream can keep an assertion whose
			// endpoint spelling drifted, and expanding from it would be a lie.
			continue
		}
		peerKey := normalizeGraphName(peer)
		if peerKey == "" || peerKey == key {
			continue
		}
		if existing := byKey[peerKey]; existing != nil {
			existing.Weight++
			if existing.Type == "" {
				existing.Type = peerType
			}
			continue
		}
		byKey[peerKey] = &ontologyNeighborPeer{Name: peer, Type: peerType, Weight: 1}
		order = append(order, peerKey)
	}

	out := make([]ontologyNeighborPeer, 0, len(order))
	for _, peerKey := range order {
		out = append(out, *byKey[peerKey])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		return normalizeGraphName(out[i].Name) < normalizeGraphName(out[j].Name)
	})
	if budget > 0 && len(out) > budget {
		return out[:budget], true
	}
	return out, false
}

// ontologyGraphNeighbors answers "expand this instance": the centre, one hop of
// assertions around it, and the instances those assertions point at.
//
// It reads the same rows and projects them with the same functions as the class
// view, so the canvas can merge the answer into what it already drew without a
// second vocabulary for nodes or edges. Nothing here needs a column that does
// not exist: direction comes from from_entity_kwd/to_entity_kwd, identity from
// name_kwd — no reverse adjacency table, no materialised neighbour list.
//
// Hop is fixed at 1 on purpose. A two-hop answer is the union of one-hop answers
// and costs one request per frontier node anyway, so letting the caller ask for
// hop=2 here would only hide that cost inside a single request.
func (s *DatasetArtifactService) ontologyGraphNeighbors(
	ctx context.Context, in OntologyGraphInput,
) (*OntologyGraphPage, error) {
	page := &OntologyGraphPage{
		Hop:       1,
		Classes:   []string{},
		Entities:  []StructureGraphNode{},
		Relations: []StructureGraphRelation{},
	}

	center, err := s.ontologyNeighborCenter(ctx, in)
	if err != nil {
		return nil, err
	}
	if center == nil {
		// Neither a row id nor a name in this scope: an empty page, not an
		// error. The canvas keeps what it already drew instead of turning a
		// stale click into a failed request.
		return page, nil
	}
	centerName := strings.TrimSpace(graphStr(center["name"]))
	if centerName == "" {
		return page, nil
	}

	relations, edgesTruncated, err := s.ontologyNeighborRelations(ctx, in, centerName)
	if err != nil {
		return nil, err
	}
	peers, peersTruncated := ontologyNeighborPeers(centerName, relations, in.Limit-1)
	neighborNodes, peersResolvedTruncated, err := s.ontologyNeighborEntities(ctx, in, peers)
	if err != nil {
		return nil, err
	}

	nodes, edges := ontologyGraphAssemble(
		append([]StructureGraphNode{center}, neighborNodes...), relations, in.Limit)

	page.Entities = nodes
	page.Relations = edges
	page.Drawn = len(nodes)
	// Total counts the centre itself so the class view's "drawn N of TOTAL"
	// reading still holds: a neighbour page is one instance plus its peers.
	page.Total = int64(len(neighborNodes)) + 1
	page.Truncated = edgesTruncated || peersTruncated || peersResolvedTruncated ||
		int64(len(nodes)) < page.Total
	classes := []string{}
	if typ := strings.TrimSpace(graphStr(center["type"])); typ != "" && typ != "other" {
		classes = []string{typ}
	}
	page.Classes = ontologyGraphClassesOf(classes, edges)

	markEntityClassConflicts(page.Entities)
	markRelationConflicts(page.Relations)
	return page, nil
}

// ontologyNeighborCenter resolves the expansion origin.
//
// The caller normally sends a row id, because that is what the canvas holds for
// a node it drew from a row. The name fallback exists for the other half of the
// graph: a relation's far end is often a label that never had an entity row, and
// a canvas that hands back that label must still be able to expand it. When
// neither lookup finds a row the label itself is the centre — assertions can
// name an instance that was never folded into an entity row.
func (s *DatasetArtifactService) ontologyNeighborCenter(
	ctx context.Context, in OntologyGraphInput,
) (StructureGraphNode, error) {
	filter := ontologyInstanceFilter("entity", in.TemplateID, in.DocumentID)
	filter["id"] = []string{in.Node}
	rows, _, err := graphRowSearch(ctx, in.TenantID, in.DatasetID, graphEntityFields, filter,
		nil, 0, 1, nil)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if node := projectEntity(row); node != nil {
			if evidence := ontologyEvidenceOf(row); len(evidence) > 0 {
				node["evidence"] = evidence
			}
			return node, nil
		}
	}

	name := strings.TrimSpace(in.Node)
	if name == "" {
		return nil, nil
	}
	if node := s.ontologyNeighborEntityByName(ctx, in, name); node != nil {
		return node, nil
	}
	return StructureGraphNode{
		"name":          name,
		"type":          "other",
		"mention_count": 0,
		"stub":          true,
	}, nil
}

// ontologyNeighborEntityByName looks an instance up by the column the writer
// fills for it: name_kwd, which the writer lowercases. Filtering by the display
// spelling would therefore never match on a keyword column, so the lookup folds
// case and trims — the same normalisation the canvas merges on.
func (s *DatasetArtifactService) ontologyNeighborEntityByName(
	ctx context.Context, in OntologyGraphInput, name string,
) StructureGraphNode {
	filter := ontologyInstanceFilter("entity", in.TemplateID, in.DocumentID)
	filter["name_kwd"] = []string{strings.ToLower(strings.TrimSpace(name))}
	rows, _, err := graphRowSearch(ctx, in.TenantID, in.DatasetID, graphEntityFields, filter,
		nil, 0, 1, nil)
	if err != nil {
		return nil
	}
	for _, row := range rows {
		if node := projectEntity(row); node != nil {
			if evidence := ontologyEvidenceOf(row); len(evidence) > 0 {
				node["evidence"] = evidence
			}
			return node
		}
	}
	return nil
}

// ontologyNeighborRelations reads the assertions touching one instance, one read
// per direction (see ontologyNeighborEndpointColumns).
func (s *DatasetArtifactService) ontologyNeighborRelations(
	ctx context.Context, in OntologyGraphInput, centerName string,
) ([]StructureGraphRelation, bool, error) {
	out := []StructureGraphRelation{}
	truncated := false
	// A self loop is returned by both reads; mergeRelations below folds it, but
	// the row id is the cheap place to notice it first.
	seen := map[string]bool{}
	for _, column := range ontologyNeighborEndpointColumns(in.Direction) {
		rows, total, err := s.ontologyNeighborRelationPage(ctx, in, column, centerName)
		if err != nil {
			return nil, false, err
		}
		if total > int64(len(rows)) {
			truncated = true
		}
		for _, row := range rows {
			if id := strings.TrimSpace(firstStringValue(row["id"])); id != "" {
				if seen[id] {
					continue
				}
				seen[id] = true
			}
			if edge := projectRelation(row); edge != nil {
				if evidence := ontologyEvidenceOf(row); len(evidence) > 0 {
					edge["evidence"] = evidence
				}
				out = append(out, edge)
			}
		}
	}
	return mergeRelations(out), truncated, nil
}

// ontologyNeighborRelationPage reads one directed page of assertions.
//
// The endpoint columns keep the spelling the extraction produced, while the
// entity column next to them is lowercased. A term match on a keyword column is
// exact, so the display spelling is tried first and a case-folded spelling once
// more only when the first read finds nothing: that is the one drift a keyword
// filter cannot absorb itself, and it costs an extra read only on a node whose
// spelling actually disagrees.
func (s *DatasetArtifactService) ontologyNeighborRelationPage(
	ctx context.Context, in OntologyGraphInput, column, centerName string,
) (map[string]map[string]interface{}, int64, error) {
	names := []string{centerName}
	if folded := strings.ToLower(centerName); folded != centerName {
		names = append(names, folded)
	}
	for i, name := range names {
		filter := ontologyInstanceFilter("relation", in.TemplateID, in.DocumentID)
		filter[column] = []string{name}
		rows, total, err := graphRowSearch(ctx, in.TenantID, in.DatasetID, graphRelationFields,
			filter, nil, 0, ontologyGraphEdgeCap, nil)
		if err != nil {
			return nil, 0, err
		}
		if len(rows) > 0 || i == len(names)-1 {
			return rows, total, nil
		}
	}
	return nil, 0, nil
}

// ontologyNeighborEntities resolves the peers to instances, in the order the
// peers were ranked.
//
// A peer with no entity row is not dropped: the assertion is real, and drawing
// it to a missing node would make the edge disappear. It becomes a stub node
// carrying the class the assertion claimed — a response-side label, not a stored
// row, so no column is needed for it.
func (s *DatasetArtifactService) ontologyNeighborEntities(
	ctx context.Context, in OntologyGraphInput, peers []ontologyNeighborPeer,
) ([]StructureGraphNode, bool, error) {
	if len(peers) == 0 {
		return nil, false, nil
	}
	names := make([]string, 0, len(peers))
	for _, peer := range peers {
		if folded := strings.ToLower(strings.TrimSpace(peer.Name)); folded != "" {
			names = append(names, folded)
		}
	}
	if len(names) == 0 {
		return nil, false, nil
	}

	filter := ontologyInstanceFilter("entity", in.TemplateID, in.DocumentID)
	filter["name_kwd"] = names
	rows, total, err := graphRowSearch(ctx, in.TenantID, in.DatasetID, graphEntityFields, filter,
		nil, 0, len(names), nil)
	if err != nil {
		return nil, false, err
	}
	byName := map[string]StructureGraphNode{}
	for _, row := range rows {
		node := projectEntity(row)
		if node == nil {
			continue
		}
		key := normalizeGraphName(node["name"])
		if key == "" {
			continue
		}
		if _, ok := byName[key]; ok {
			continue
		}
		if evidence := ontologyEvidenceOf(row); len(evidence) > 0 {
			node["evidence"] = evidence
		}
		byName[key] = node
	}

	return ontologyNeighborNodes(peers, byName), total > int64(len(rows)), nil
}

// ontologyNeighborNodes maps the ranked peers to nodes, keeping the ranking.
// A peer the entity query did not return becomes a stub: the canvas must draw
// the assertion, so the far end cannot simply be missing.
func ontologyNeighborNodes(
	peers []ontologyNeighborPeer, byName map[string]StructureGraphNode,
) []StructureGraphNode {
	out := make([]StructureGraphNode, 0, len(peers))
	for _, peer := range peers {
		if node, ok := byName[normalizeGraphName(peer.Name)]; ok {
			out = append(out, node)
			continue
		}
		typ := strings.TrimSpace(peer.Type)
		if typ == "" {
			typ = "other"
		}
		out = append(out, StructureGraphNode{
			"name":          peer.Name,
			"type":          typ,
			"mention_count": 0,
			"stub":          true,
		})
	}
	return out
}
