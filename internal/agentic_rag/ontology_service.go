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
