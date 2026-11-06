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
	"errors"
	"sync"
)

// OntologyDegrade* are the machine-readable reasons a digest carries no
// structure-narrowed rows. They are mirrored from the retrieval service so a
// caller (and the receipt) can branch on a code instead of matching prose.
const (
	// OntologyDegradeNoModel means the declared structure could not be read:
	// a missing model, a foreign tenant's model, a kind that is not an
	// ontology, or a model that was never published.
	OntologyDegradeNoModel = "no_model"
	// OntologyDegradeNoMatch means the structure was read and applied and
	// matched no assertion.
	OntologyDegradeNoMatch = "no_match"
	// OntologyDegradeCitationsUnresolvable means assertions came back but none
	// of their source chunks are readable in the document scope, so the digest
	// can contribute vocabulary but no evidence.
	OntologyDegradeCitationsUnresolvable = "citations_unresolvable"
	// OntologyDegradeCoverageUnknown means the declared-structure coverage could
	// not be sampled at all, so which declared classes have instances is
	// unknown. It is deliberately distinct from an empty coverage: "we could not
	// look" and "there is nothing there" are different facts, and only one of
	// them is about the corpus.
	OntologyDegradeCoverageUnknown = "coverage_unknown"
	// OntologyDegradeHypergraphAbsent means the hypergraph recall was asked for
	// and produced no factual block: the knowledge scope has no compiled
	// hyperedge rows, or the recalled slots belong to none. It is a degrade of
	// the hypergraph section alone — the assertion rows are unaffected — and it
	// is stated rather than implied because "no block was assembled" and "the
	// corpora holds no slots for this question" read the same from an empty
	// section.
	OntologyDegradeHypergraphAbsent = "hypergraph_absent"
)

// OntologyEvidence* grade how much the rows of one digest may be leaned on.
//
// The knowledge layer is searched in stages: the declared structure narrows
// first, and when that matches nothing the same question is asked without the
// narrowing rather than answered with nothing. The grade is what keeps the two
// outcomes readable: a caller that only learned "was the structure applied"
// could not tell a model that declares nothing the question names from a
// narrowed search that came up empty, and it could not present the weaker rows
// as the weaker ones they are. Degrade carries the reason; the grade carries the
// strength, and both travel with the digest.
const (
	// OntologyEvidenceStructured: the rows were narrowed by the declared
	// structure and matched. It is the only grade that may be presented as what
	// the model says.
	OntologyEvidenceStructured = "structured"
	// OntologyEvidenceSchemaFree: the rows are the knowledge scope's compiled
	// assertions retrieved without a structural narrowing — because the caller's
	// constraints matched nothing and were dropped, because the question named
	// nothing the model declares, or because the model could not be read at all.
	// They are usable knowledge and their quotations are citable (the citation
	// counts say which), but they are not a statement this model makes about the
	// question, and the reason they are unnarrowed is in Degrade when there is one.
	OntologyEvidenceSchemaFree = "schema_free"
	// OntologyEvidenceNone: the search returned no assertion at all, in either
	// stage. In the direct-answer mode this is the case where the question must
	// be reported as a coverage gap rather than answered from declarations.
	OntologyEvidenceNone = "none"
)

// OntologyModelRef names the published model a digest was produced under. The
// version is carried because the same question asked either side of a publish
// may legitimately get different answers, and nothing else can explain that.
// The JSON tags are the REST contract the panel reads. Without them the digest
// would be the only response in this API spelled in Go field names.
type OntologyModelRef struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       int    `json:"version"`
	PublishStatus string `json:"publish_status"`
}

// OntologyAssertion is one compiled row: what the model says exists, with the
// verbatim evidence that produced it and the chunks that evidence came from.
//
// Evidence is quoted text rather than a re-worded claim on purpose — the part
// of an assertion that may be shown as a citation is the text the source
// actually wrote.
type OntologyAssertion struct {
	// Kind is "entity" or "relation", the row's compiled role.
	Kind string `json:"kind"`
	// Type is the declared class (entity) or the property (relation).
	Type string `json:"type"`
	// Name is the instance's canonical name; for a relation it is the property
	// spelled out for the model ("hasStatus").
	Name string `json:"name"`
	// From/To are a relation's endpoints, carrying their declared classes.
	From     string `json:"from"`
	FromType string `json:"from_type"`
	To       string `json:"to"`
	ToType   string `json:"to_type"`
	// Attributes are the datatype values the row carries.
	Attributes map[string]string `json:"attributes"`
	// Evidence is the verbatim quote the compiler kept.
	Evidence string `json:"evidence"`
	// ChunkIDs are the chunks the evidence came from; DocIDs their documents.
	ChunkIDs []string `json:"chunk_ids"`
	DocIDs   []string `json:"doc_ids"`
	// ReviewStatus is the row's governance state. Unreviewed is set when it is
	// pending, because in the direct-answer mode a pending assertion is the
	// whole of the evidence and that has to be visible rather than implied.
	ReviewStatus string `json:"review_status"`
	ReviewBy     string `json:"review_by"`
}

// OntologyCoverage is how much of the declared structure the knowledge scope
// actually exercises.
//
// Missing is split from "not sampled" because the two call for different
// actions: a declared class with no instance is a compilation gap, a truncated
// scan is a reporting limitation. Coverage is computed with zero LLM calls —
// it is a scan of the compiled rows' type columns, not a judgement about the
// question.
type OntologyCoverage struct {
	DeclaredClasses    []string `json:"declared_classes"`
	DeclaredProperties []string `json:"declared_properties"`
	MatchedClasses     []string `json:"matched_classes"`
	MatchedProperties  []string `json:"matched_properties"`
	MissingClasses     []string `json:"missing_classes"`
	MissingProperties  []string `json:"missing_properties"`
	// SampleLimit is the per-kind cap the scan ran under; Sampled is how many
	// rows were read. Truncated reports that the cap was reached, which is what
	// makes "no instance" unreadable for anything past it.
	SampleLimit int  `json:"sample_limit"`
	Sampled     int  `json:"sampled"`
	Truncated   bool `json:"truncated"`
}

// OntologyCitations counts the assertions' source chunks that resolve in the
// document scope. A citation is a chunk-level fact: an assertion whose chunks
// are not readable contributes vocabulary but no evidence, and the count is
// what says so instead of the evidence quietly disappearing.
type OntologyCitations struct {
	Resolvable   int `json:"resolvable"`
	Unresolvable int `json:"unresolvable"`
}

// OntologyHypergraphFact is one `path: value` row of a factual block. The path
// is the natural-language phrasing of the attribute path (the same text the
// key-side vector was built from), never the mechanical dotted form: the block
// is read by a model, and the mechanical form is a storage key.
type OntologyHypergraphFact struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

// OntologyHyperedge is one selected factual block: a set of slots that occur
// together in one root context. It is the retrieval unit and the citation unit
// of the hypergraph layer — several facts below one handle is what says they
// came from the same place.
type OntologyHyperedge struct {
	// RowID is the block's row id. It is a citation handle
	// (`hyperedge:<row id>`) rather than a fact: the row is the compiled
	// product, and the chunk a reader can open is in ChunkIDs.
	RowID string `json:"row_id"`
	// Root is the entity the block was flattened from, and RootType its class.
	Root     string `json:"root"`
	RootType string `json:"root_type"`
	// Depth is how far the block's deepest entity sits below the root.
	Depth int `json:"depth"`
	// Facts are the block's slots, in path order so the rendering is
	// reproducible run to run.
	Facts []OntologyHypergraphFact `json:"facts"`
	// CoveredNodes is how many of this question's recalled slots this block
	// contributed. It is what the coverage selection maximized, and it is what
	// lets a reader see why this block and not another was assembled.
	CoveredNodes int `json:"covered_nodes"`
	// ChunkIDs are the root entity's source chunks: the original text this
	// block was compiled from.
	ChunkIDs []string `json:"chunk_ids"`
	DocIDs   []string `json:"doc_ids"`
}

// OntologyHypergraphChunk is one original chunk a selected block points at. The
// content is carried because the block itself is a compiled digest of it: a
// reader has to be able to get from the block back to the prose.
type OntologyHypergraphChunk struct {
	ChunkID string `json:"chunk_id"`
	DocID   string `json:"doc_id"`
	DocName string `json:"doc_name"`
	Content string `json:"content"`
	// CoveredNodes is the contribution of the block that pulled this chunk in,
	// so the ordering of the chunk block is explainable.
	CoveredNodes int `json:"covered_nodes"`
}

// OntologyHypergraphCoverage is how much of the question's recalled slot set the
// assembled blocks explain. It is computed exactly, over the selected blocks
// rather than over a sample, which is what makes it usable as a sufficiency
// signal: every number here is a count of things that were actually recalled.
type OntologyHypergraphCoverage struct {
	// HitNodes is the number of distinct slots the recall legs returned.
	HitNodes int `json:"hit_nodes"`
	// CoveredNodes is how many of them ended up inside a selected block.
	CoveredNodes int `json:"covered_nodes"`
	// OrphanHits is slots that belong to no block at all — no hyperedge_ids, or
	// only edges outside the selected set. A non-zero value says the blocks are
	// an incomplete view of what was recalled, and it is the one number that
	// must never be read as "the corpus lacks this".
	OrphanHits int `json:"orphan_hits"`
	// NodeCoverage is CoveredNodes/HitNodes.
	NodeCoverage float64 `json:"node_coverage"`
	// SlotCoverage is the covered (class, attribute) pairs over the pairs the
	// recalled slots name: whether the blocks speak to the attributes this
	// question touched, not merely to its terms.
	SlotCoverage float64 `json:"slot_coverage"`
	// Sufficient is the zero-LLM gate: enough slots covered, and no slot left
	// without a block.
	Sufficient bool `json:"sufficient"`
	// Partial reports that one recall leg failed, so the slot set these numbers
	// are computed over is smaller than it should have been.
	Partial bool `json:"partial"`
}

// OntologyHypergraph is the hypergraph layer's contribution to one digest: the
// blocks it assembled, the original chunks they point at, and the coverage that
// says how complete the assembly is.
type OntologyHypergraph struct {
	Edges    []OntologyHyperedge        `json:"edges"`
	Chunks   []OntologyHypergraphChunk  `json:"chunks"`
	Coverage OntologyHypergraphCoverage `json:"coverage"`
	// Degrade explains an empty or partial section; nil when the recall ran and
	// produced blocks.
	Degrade *OntologyDegrade `json:"degrade,omitempty"`
}

// OntologyDigest is what one L1 retrieval produced. It is the whole interface
// between the knowledge layer and the agent loop: the vocabulary the model
// should speak in, the seeds it should start from, the handles it may read, and
// the receipt that explains all of it.
type OntologyDigest struct {
	Model      OntologyModelRef    `json:"model"`
	Assertions []OntologyAssertion `json:"assertions"`
	Vocabulary []string            `json:"vocabulary"`
	Coverage   OntologyCoverage    `json:"coverage"`
	Citations  OntologyCitations   `json:"citations"`
	// AppliedFilter is the narrowing the rows were actually searched with,
	// including every subclass the declared hierarchy expanded to. It is nil
	// when none was applied.
	AppliedFilter map[string]any `json:"applied_filter"`
	// StructureApplied reports whether the rows were narrowed by the declared
	// structure at all.
	StructureApplied bool `json:"structure_applied"`
	// EvidenceLevel is one of the OntologyEvidence* grades: how strong the
	// evidence these rows carry is, and therefore what may be concluded from
	// them. It is stated rather than left to be inferred from StructureApplied:
	// presenting unnarrowed rows as the model's own statement is the failure
	// mode the grade exists to make impossible.
	EvidenceLevel string `json:"evidence_level"`
	// Degrade is nil when the structure was applied cleanly.
	Degrade *OntologyDegrade `json:"degrade"`
	// UnreviewedCount is how many returned assertions are pending review. It is
	// surfaced separately because a caller has to be able to say "these
	// conclusions rest on unreviewed knowledge" without reading every row.
	UnreviewedCount int `json:"unreviewed_count"`
	// Total is the row count the engine reported, which can exceed
	// len(Assertions) because the request bounds how many rows are carried.
	Total int64 `json:"total"`
	// Hypergraph is the hypergraph layer's contribution, present only when the
	// caller asked for it (ontology_hypergraph_recall). It is a section of the
	// digest rather than a separate result because its blocks belong to the
	// same question, the same model version and the same coverage story as the
	// assertions above — a second payload would let the two disagree.
	Hypergraph *OntologyHypergraph `json:"hypergraph,omitempty"`
}

// OntologyDegrade is the machine-readable reason plus the sentence a human
// reads, so the UI never has to invent wording for a code it does not know.
type OntologyDegrade struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// OntologyRetrieveRequest is one knowledge-layer lookup.
//
// Nothing here is parsed out of the question: the constraints are the caller's
// statement about which classes and properties to look at, which is why the
// candidate constraints come from a deterministic match against the declared
// vocabulary rather than from the model reading its own prompt.
type OntologyRetrieveRequest struct {
	// UserID is the caller, used for the knowledge datasets' access check.
	UserID string
	// KnowledgeDatasetIDs is the knowledge scope (§1.2 of the design): a
	// different set from the conversation's document datasets.
	KnowledgeDatasetIDs []string
	// ModelID names the published ontology model whose declared hierarchy
	// expands the constraints and whose declarations the digest reports.
	ModelID string
	// ModelVersion pins the snapshot to read. Zero means "the model's current
	// published version, or the working config when it was never published".
	ModelVersion int
	// Question is what the dense leg ranks against.
	Question string
	// DocumentIDs narrows the knowledge scope to those documents.
	DocumentIDs []string
	// EntityTypes / Properties / FromTypes / ToTypes are the optional explicit
	// constraints. All empty is a valid request: it is "search the compiled
	// rows in the knowledge scope", which is what both the prefetch and the
	// direct-answer mode do.
	EntityTypes []string
	Properties  []string
	FromTypes   []string
	ToTypes     []string
	// TopK bounds the rows carried. Defaults to the service's own default.
	TopK int
	// SimilarityThreshold is the dense-leg floor. Zero uses the default.
	SimilarityThreshold float64
	// DocumentScope is the conversation's readable document set, used to count
	// resolvable citations. Nil means unrestricted (every source chunk
	// resolves); a non-nil empty slice means no document is readable.
	DocumentScope []string
	// SkipUnreviewed drops pending assertions. The default keeps them, tagged,
	// because a model whose latest compile is unreviewed would otherwise be
	// invisible until a human got to it.
	SkipUnreviewed bool
	// HypergraphRecall adds the compiled hyperedge layer to the digest: two
	// hypernode recall legs, greedy block selection and the coverage those
	// blocks explain. It is off by default so the layer can be measured against
	// the assertion-only path on the same questions (see
	// hypergraph_retrieval.md §4), and off is exactly the previous behaviour.
	HypergraphRecall bool
}

// OntologyTraverseRequest asks for a typed walk along compiled assertions.
//
// It exists as a separate request from OntologyRetrieveRequest because a walk and
// a lookup answer different questions: a lookup ranks rows against a question,
// a walk follows the declared properties outward from an entity the caller named.
// Folding one into the other as a "mode" would make the mode and the arguments
// two ways to say the same thing, and the two would drift.
type OntologyTraverseRequest struct {
	UserID string
	// KnowledgeDatasetIDs is the knowledge scope, a different set from the
	// conversation's document datasets.
	KnowledgeDatasetIDs []string
	// ModelID names the model whose declarations prune the walk. Required: without
	// it a walk cannot tell a declared property from an invented one.
	ModelID      string
	ModelVersion int
	// Starts are the entity names to walk from, as the compiled rows spell them.
	Starts []string
	// Hops bounds the walk. Zero means the service default; it is clamped to the
	// service's cap rather than refused.
	Hops int
	// AllowProperties restricts every hop to these object properties. Empty means
	// every declared object property.
	AllowProperties []string
	// AllowTypes restricts the classes an entity reached by the walk may have.
	AllowTypes []string
	// MaxPaths bounds the returned chains.
	MaxPaths int
	// DocumentScope is the conversation's readable document set, used to mark a
	// step whose source cannot be cited.
	DocumentScope []string
}

// OntologyTraverseStep is one assertion a walk followed.
type OntologyTraverseStep struct {
	From     string
	Property string
	To       string
	FromType string
	ToType   string
	// Direction is how the path crosses this assertion: "forward" follows
	// from→to, "reverse" follows to→from.
	Direction      string
	SourceChunkIDs []string
}

// OntologyTraversePath is one chain from a starting entity.
type OntologyTraversePath struct {
	Start string
	Nodes []string
	Steps []OntologyTraverseStep
}

// OntologyTraverseHop is what one hop did, so a failed walk can be attributed:
// "the frontier found nothing" is a corpus answer, "the declaration refused the
// rows" is a declaration answer.
type OntologyTraverseHop struct {
	Hop                   int
	Frontier              int
	Rows                  int
	Matched               int
	RejectedByDeclaration int
	RejectedByAllowList   int
	Truncated             bool
}

// OntologyTraversal is the walk together with its accounting.
type OntologyTraversal struct {
	Paths []OntologyTraversePath
	Hops  []OntologyTraverseHop
	Model OntologyModelRef
	// DegradeCode is set when the walk could not be constrained by the model. The
	// walk still runs unnarrowed: an unreadable model must not make the
	// assertions unreachable, but the caller has to see which of the two it got.
	DegradeCode   string
	DegradeReason string
	Truncated     bool
}

// ErrOntologyServiceMissing is returned when no knowledge-layer service was
// installed. It is deliberately not fatal to a run: an ontology that cannot be
// read must not make the corpus unsearchable.
var ErrOntologyServiceMissing = errors.New("agentic_rag: no ontology retrieval service installed")

// OntologyService is the knowledge layer's boundary. It is installed at boot
// (see runtime wiring) and resolved per call, so a deployment without a
// compiled knowledge scope pays nothing for it.
type OntologyService interface {
	Retrieve(ctx context.Context, req OntologyRetrieveRequest) (*OntologyDigest, error)
	Traverse(ctx context.Context, req OntologyTraverseRequest) (*OntologyTraversal, error)
}

var (
	ontologyServiceMu   sync.RWMutex
	ontologyServiceImpl OntologyService = stubOntologyService{}
)

// SetOntologyService installs the knowledge-layer service. nil restores the
// stub, which reports the layer as unavailable instead of panicking.
func SetOntologyService(svc OntologyService) {
	ontologyServiceMu.Lock()
	defer ontologyServiceMu.Unlock()
	if svc == nil {
		ontologyServiceImpl = stubOntologyService{}
		return
	}
	ontologyServiceImpl = svc
}

// GetOntologyService returns the installed knowledge-layer service.
func GetOntologyService() OntologyService {
	ontologyServiceMu.RLock()
	defer ontologyServiceMu.RUnlock()
	return ontologyServiceImpl
}

type stubOntologyService struct{}

func (stubOntologyService) Retrieve(context.Context, OntologyRetrieveRequest) (*OntologyDigest, error) {
	return nil, ErrOntologyServiceMissing
}

func (stubOntologyService) Traverse(context.Context, OntologyTraverseRequest) (*OntologyTraversal, error) {
	return nil, ErrOntologyServiceMissing
}
