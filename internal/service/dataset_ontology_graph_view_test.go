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
	"fmt"
	"testing"
)

func TestNormalizeOntologyGraphInput(t *testing.T) {
	for _, test := range []struct {
		name             string
		class            string
		limit, hop       int
		wantClass        string
		wantLimitWantHop [2]int
	}{
		{"defaults", " 银行卡 ", 0, 0, "银行卡",
			[2]int{ontologyGraphDefaultLimit, ontologyGraphDefaultHop}},
		{"limit clamps up", "", -5, 1, "",
			[2]int{ontologyGraphDefaultLimit, 1}},
		{"limit clamps down", "", 9999, 1, "",
			[2]int{ontologyGraphMaxLimit, 1}},
		{"hop clamps down", "", 30, 9, "",
			[2]int{30, ontologyGraphMaxHop}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeOntologyGraphInput(OntologyGraphInput{
				Class: test.class, Limit: test.limit, Hop: test.hop,
			})
			if got.Class != test.wantClass {
				t.Fatalf("class = %q, want %q", got.Class, test.wantClass)
			}
			if got.Limit != test.wantLimitWantHop[0] || got.Hop != test.wantLimitWantHop[1] {
				t.Fatalf("limit/hop = %d/%d, want %d/%d",
					got.Limit, got.Hop, test.wantLimitWantHop[0], test.wantLimitWantHop[1])
			}
		})
	}
}

func TestOntologyGraphClassesOfWidensAlongEdges(t *testing.T) {
	relations := []StructureGraphRelation{
		{"from": "卡 6222", "to": "张伟", "source_type": "card", "target_type": "customer"},
		{"from": "账户 A", "to": "卡 6222", "source_type": "account", "target_type": "card"},
		// Same class spelled differently must not be added twice.
		{"from": "卡 6222", "to": "李娜", "source_type": "CARD", "target_type": "Customer"},
		{"from": "x", "to": "y", "source_type": "", "target_type": "  "},
	}
	got := ontologyGraphClassesOf([]string{"card"}, relations)
	want := []string{"card", "customer", "account"}
	if len(got) != len(want) {
		t.Fatalf("classes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("classes = %v, want %v", got, want)
		}
	}
}

func TestOntologyGraphClassesOfCapsFanOut(t *testing.T) {
	classSet := []string{"card"}
	for i := 0; i < ontologyGraphMaxClasses*2; i++ {
		classSet = ontologyGraphClassesOf(classSet, []StructureGraphRelation{
			{"from": "a", "to": "b", "source_type": "card", "target_type": fmt.Sprintf("c%d", i)},
		})
	}
	if len(classSet) != ontologyGraphMaxClasses {
		t.Fatalf("classes = %d, want %d", len(classSet), ontologyGraphMaxClasses)
	}
}

func TestOntologyGraphAssembleKeepsConnectedFirst(t *testing.T) {
	entities := []StructureGraphNode{
		{"name": "孤立", "type": "card"},
		{"name": "卡 6222", "type": "card"},
		{"name": "张伟", "type": "customer"},
	}
	relations := []StructureGraphRelation{
		{"from": "卡 6222", "to": "张伟", "property": "持有"},
	}

	nodes, edges := ontologyGraphAssemble(entities, relations, 2)
	if len(nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(nodes))
	}
	// Both ends of the drawn edge survive; the isolated instance loses its slot.
	if nodes[0]["name"] != "卡 6222" && nodes[1]["name"] != "卡 6222" {
		t.Fatalf("connected node missing from %v", nodes)
	}
	if len(edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(edges))
	}
}

func TestOntologyGraphAssembleDropsDanglingEdges(t *testing.T) {
	entities := []StructureGraphNode{
		{"name": "卡 6222", "type": "card"},
	}
	relations := []StructureGraphRelation{
		{"from": "卡 6222", "to": "不在图里", "property": "持有"},
	}

	nodes, edges := ontologyGraphAssemble(entities, relations, 10)
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(nodes))
	}
	if len(edges) != 0 {
		t.Fatalf("edges = %d, want 0: an edge to a node that did not make the cut must not be drawn", len(edges))
	}
}

func TestOntologyGraphAssembleMatchesNamesLoosely(t *testing.T) {
	entities := []StructureGraphNode{
		{"name": "卡 6222", "type": "card"},
		{"name": "张伟", "type": "customer"},
	}
	relations := []StructureGraphRelation{
		// Padding and case differ between the entity row and the edge row.
		{"from": " 卡 6222 ", "to": "张伟", "property": "持有"},
	}
	_, edges := ontologyGraphAssemble(entities, relations, 10)
	if len(edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(edges))
	}
}

func TestNormalizeOntologyNeighborDirection(t *testing.T) {
	for _, test := range []struct{ raw, want string }{
		{"", ontologyNeighborDirectionBoth},
		{" out ", ontologyNeighborDirectionOut},
		{"IN", ontologyNeighborDirectionIn},
		// An unknown value is a client bug: answer with the superset instead of
		// failing the expansion.
		{"sideways", ontologyNeighborDirectionBoth},
	} {
		if got := normalizeOntologyNeighborDirection(test.raw); got != test.want {
			t.Errorf("normalizeOntologyNeighborDirection(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestOntologyNeighborEndpointColumns(t *testing.T) {
	for _, test := range []struct {
		direction string
		want      []string
	}{
		{ontologyNeighborDirectionOut, []string{"from_entity_kwd"}},
		{ontologyNeighborDirectionIn, []string{"to_entity_kwd"}},
		{ontologyNeighborDirectionBoth, []string{"from_entity_kwd", "to_entity_kwd"}},
	} {
		got := ontologyNeighborEndpointColumns(test.direction)
		if len(got) != len(test.want) {
			t.Fatalf("%s columns = %v, want %v", test.direction, got, test.want)
		}
		for i := range test.want {
			if got[i] != test.want[i] {
				t.Fatalf("%s columns = %v, want %v", test.direction, got, test.want)
			}
		}
	}
}

func TestOntologyNeighborPeersRanksAndDedupes(t *testing.T) {
	relations := []StructureGraphRelation{
		{"from": "Acme Corp", "to": "Beta Ltd", "property": "supplies", "target_type": "organization"},
		{"from": " acme corp ", "to": "BETA LTD", "property": "owns", "target_type": "organization"},
		{"from": "Gamma", "to": "Acme Corp", "property": "employs", "source_type": "person"},
		// A self loop is not a neighbour.
		{"from": "Acme Corp", "to": "Acme Corp", "property": "alias_of"},
		// An assertion that does not name the centre is not its neighbourhood.
		{"from": "Beta Ltd", "to": "Gamma", "property": "partners"},
	}
	peers, truncated := ontologyNeighborPeers("Acme Corp", relations, 10)
	if truncated {
		t.Fatalf("10 peers fit a budget of 10")
	}
	if len(peers) != 2 {
		t.Fatalf("peers = %v, want Beta Ltd and Gamma", peers)
	}
	if peers[0].Name != "Beta Ltd" || peers[0].Weight != 2 {
		t.Fatalf("first peer = %+v, want Beta Ltd reached by 2 assertions", peers[0])
	}
	if peers[0].Type != "organization" {
		t.Fatalf("first peer type = %q, want the assertion's target class", peers[0].Type)
	}
	if peers[1].Name != "Gamma" || peers[1].Weight != 1 {
		t.Fatalf("second peer = %+v, want Gamma reached by 1 assertion", peers[1])
	}
	if peers[1].Type != "person" {
		t.Fatalf("second peer type = %q, want the in-edge's source class", peers[1].Type)
	}
}

func TestOntologyNeighborPeersCapsTheBudget(t *testing.T) {
	relations := []StructureGraphRelation{}
	for _, name := range []string{"a", "b", "c", "d"} {
		relations = append(relations, StructureGraphRelation{"from": "center", "to": name, "property": "p"})
	}
	// Give "d" the strongest tie so the ranking, not the input order, decides.
	relations = append(relations, StructureGraphRelation{"from": "center", "to": "d", "property": "q"})

	peers, truncated := ontologyNeighborPeers("center", relations, 2)
	if !truncated {
		t.Fatalf("4 neighbours behind a budget of 2 must report truncation")
	}
	if len(peers) != 2 || peers[0].Name != "d" {
		t.Fatalf("peers = %v, want the strongest neighbour first and the list capped at 2", peers)
	}
	// Deterministic tie break: "a" before "b" regardless of map order.
	if peers[1].Name != "a" {
		t.Fatalf("tie break = %q, want a", peers[1].Name)
	}
}

func TestOntologyNeighborNodesKeepsRankingAndStubs(t *testing.T) {
	peers := []ontologyNeighborPeer{
		{Name: "Beta Ltd", Type: "organization", Weight: 2},
		{Name: "Nowhere Inc", Type: "organization", Weight: 1},
	}
	byName := map[string]StructureGraphNode{
		// The entity row carries the display spelling; the join is normalised.
		"beta ltd": {"name": "Beta Ltd", "type": "organization", "mention_count": 3},
	}
	nodes := ontologyNeighborNodes(peers, byName)
	if len(nodes) != 2 {
		t.Fatalf("nodes = %v, want one per peer", nodes)
	}
	if nodes[0]["mention_count"] != 3 {
		t.Fatalf("first node = %v, want the entity row reused", nodes[0])
	}
	if nodes[1]["stub"] != true || nodes[1]["mention_count"] != 0 {
		t.Fatalf("second node = %v, want a stub carrying the assertion's class", nodes[1])
	}
	if nodes[1]["type"] != "organization" {
		t.Fatalf("stub type = %v, want the class the assertion claimed", nodes[1]["type"])
	}
}

func TestOntologyNeighborNodesStubsWithUnknownClass(t *testing.T) {
	nodes := ontologyNeighborNodes([]ontologyNeighborPeer{{Name: "Nowhere Inc"}}, nil)
	if len(nodes) != 1 || nodes[0]["type"] != "other" {
		t.Fatalf("node = %v, want a stub typed other when no assertion named a class", nodes)
	}
}

func TestNormalizeOntologyGraphInputNeighborMode(t *testing.T) {
	got := normalizeOntologyGraphInput(OntologyGraphInput{
		Node: " ent-1 ", Direction: " OUT ",
	})
	if got.Node != "ent-1" || got.Direction != ontologyNeighborDirectionOut {
		t.Fatalf("node/direction = %q/%q, want ent-1/out", got.Node, got.Direction)
	}
	if got.Limit != ontologyGraphDefaultLimit || got.Hop != ontologyGraphDefaultHop {
		t.Fatalf("limit/hop = %d/%d, want the class-mode defaults", got.Limit, got.Hop)
	}
}
