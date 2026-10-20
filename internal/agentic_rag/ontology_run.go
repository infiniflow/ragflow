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
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

// ontologyMode is how this run uses the knowledge layer.
//
// The mode is not a user preference: it follows from whether the turn has a
// document corpus to read. With documents, the knowledge layer guides; without
// them, it is the only answer surface there is, and a mode that behaved like
// the guiding one would leave the agent with no tools that can answer.
type ontologyMode int

const (
	// ontologyModeOff runs the turn exactly as if the feature did not exist.
	ontologyModeOff ontologyMode = iota
	// ontologyModeGuide mounts the digest onto a document-reading loop.
	ontologyModeGuide
	// ontologyModeDirect answers from the knowledge layer alone.
	ontologyModeDirect
)

func (m ontologyMode) String() string {
	switch m {
	case ontologyModeGuide:
		return "guide"
	case ontologyModeDirect:
		return "direct"
	default:
		return "off"
	}
}

// OntologyRunConfig is the knowledge-layer binding for one turn: which
// published model to read, and which datasets hold its compiled rows.
//
// The model is part of the configuration rather than a tool argument because
// the ontology is a governance boundary, not a search option: a model that
// could be switched by the model itself would let one conversation's retrieval
// be judged against another's declarations.
type OntologyRunConfig struct {
	ModelID             string
	ModelVersion        int
	KnowledgeDatasetIDs []string
	// HypergraphRecall adds the compiled hyperedge section to this turn's
	// digest. It is a per-binding switch rather than a deployment-wide one
	// because it exists to be measured: the same assistant, the same questions,
	// one boolean apart.
	HypergraphRecall bool
}

// Enabled reports whether the binding is complete enough to run the knowledge
// layer at all. A model without a scope has nowhere to look; a scope without a
// model has no declarations to read, and reading the declared structure is the
// whole point.
func (c *OntologyRunConfig) Enabled() bool {
	return c != nil && strings.TrimSpace(c.ModelID) != "" && len(c.KnowledgeDatasetIDs) > 0
}

// ontologyBinding is the resolved run context a knowledge-layer call needs:
// who is asking, under which model and scope, and which documents the caller
// may read (for counting resolvable citations).
type ontologyBinding struct {
	// UserID is the caller, for the knowledge datasets' access check.
	UserID string
	// TenantID scopes the model lookup.
	TenantID string
	// Config is the model + knowledge scope.
	Config OntologyRunConfig
	// DocumentScope is the conversation's readable document set. Nil is
	// unrestricted, which is what a direct-answer run has.
	DocumentScope []string
	// Direct reports that this run has no document corpus, so the knowledge
	// layer is the answer surface rather than a guide.
	Direct bool
}

type ontologyBindingKey struct{}

// WithOntologyBinding installs the run's knowledge-layer binding. Tools read it
// from the context rather than from their construction arguments: the binding
// is per-run state, and the tool registry is process-wide.
func WithOntologyBinding(ctx context.Context, b ontologyBinding) context.Context {
	return context.WithValue(ctx, ontologyBindingKey{}, b)
}

// ontologyBindingFrom returns the installed binding.
func ontologyBindingFrom(ctx context.Context) (ontologyBinding, bool) {
	b, ok := ctx.Value(ontologyBindingKey{}).(ontologyBinding)
	return b, ok
}

// resolveOntologyMode decides which of the three modes this turn runs in.
func resolveOntologyMode(in Input) ontologyMode {
	if !in.Ontology.Enabled() {
		return ontologyModeOff
	}
	if len(in.DatasetIDs) == 0 {
		// No document corpus: the knowledge layer is the only thing there is
		// to read, so it answers rather than guides.
		return ontologyModeDirect
	}
	return ontologyModeGuide
}

// documentCorpusToolNames are the tools that read the conversation's document
// datasets. The direct-answer mode removes them: leaving one in place would
// offer the model a tool whose scope is empty, and a forced corpus search it
// cannot satisfy is exactly the failure the mode exists to avoid.
var documentCorpusToolNames = map[string]bool{
	"search_chunks":          true,
	"search_bm25_chunks":     true,
	"search_semantic_chunks": true,
	"grep_chunks":            true,
	"list_chunks":            true,
	"search_metadata":        true,
	"navigate_tree":          true,
	"navigate_structure":     true,
	"graph_explore":          true,
}

// isDocumentCorpusTool reports whether a tool reads the document datasets.
func isDocumentCorpusTool(name string) bool {
	return documentCorpusToolNames[name]
}

// ontologyToolsFor adjusts the tool set for the knowledge layer.
//
// In the direct-answer mode the document-corpus tools are removed rather than
// left in place: offering a tool whose scope is empty is how a model ends up
// spending its iterations on searches that cannot return anything, and the
// forced-search gate would otherwise demand a corpus search that nothing can
// satisfy.
//
// The knowledge tool is appended when the template did not list it, because a
// configured knowledge layer the model cannot reach is the same as no knowledge
// layer at all.
func ontologyToolsFor(ctx context.Context, tools []tool.BaseTool, mode ontologyMode) []tool.BaseTool {
	if mode == ontologyModeOff {
		return tools
	}
	out := make([]tool.BaseTool, 0, len(tools)+1)
	hasOntology := false
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil || info == nil {
			out = append(out, t)
			continue
		}
		if info.Name == searchOntologyToolName {
			hasOntology = true
			out = append(out, t)
			continue
		}
		if mode == ontologyModeDirect && isDocumentCorpusTool(info.Name) {
			continue
		}
		out = append(out, t)
	}
	if !hasOntology {
		out = append(out, NewSearchOntologyTool())
	}
	return out
}

// ontologyPrefetch runs the knowledge-layer lookup for this turn.
//
// It is a prefetch rather than a tool the model may choose because it is cheap,
// deterministic and, above all, it is the input to the loop: a knowledge layer
// the model has to remember to consult is one it will sometimes not consult,
// and the miss is invisible in the answer.
//
// A failure here is logged and swallowed. An ontology nobody can read must not
// make the corpus unsearchable.
func ontologyPrefetch(ctx context.Context, mode ontologyMode, in Input, b ontologyBinding) *OntologyDigest {
	if mode == ontologyModeOff {
		return nil
	}
	req := OntologyRetrieveRequest{
		UserID:              b.UserID,
		KnowledgeDatasetIDs: b.Config.KnowledgeDatasetIDs,
		ModelID:             b.Config.ModelID,
		ModelVersion:        b.Config.ModelVersion,
		Question:            lastUserQuestion(in.Messages),
		// The knowledge scope is deliberately NOT narrowed to the turn's
		// documents: a knowledge row compiled from a document outside the scope
		// still carries the wording and the declarations this question needs,
		// and DocumentScope below is what decides whether its quotation may be
		// cited. Narrowing here would silently drop the vocabulary too.
		DocumentScope: b.DocumentScope,
		// The hypergraph section is requested by the binding, not by the mode:
		// a knowledge layer that is on for the run may still be measured
		// without it.
		HypergraphRecall: b.Config.HypergraphRecall,
	}
	digest, err := GetOntologyService().Retrieve(ctx, req)
	if err != nil {
		common.WarnCtx(ctx, "agentic_rag: ontology prefetch failed; running without the knowledge layer",
			zap.String("mode", mode.String()),
			zap.String("model_id", b.Config.ModelID),
			zap.Error(err))
		return nil
	}
	if digest == nil {
		return nil
	}
	return digest
}

// ontologyInstruction renders the digest into the run's instruction.
//
// The block goes into the instruction rather than into the conversation because
// the conversation is persisted: a digest that became a message would still be
// there next turn, steering an answer that has nothing to do with it, and the
// history compactor would treat it as something a user said.
//
// It says what the knowledge layer knows and how to spend it, and never what
// the answer is: the assertions' job is to point at where to read and which
// terms to use. An answer copied out of an assertion would be a conclusion with
// no readable source.
func ontologyInstruction(base string, mode ontologyMode, digest *OntologyDigest, reg *EvidenceRegistry) string {
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\n")
	b.WriteString(ontologyDigestBlock(mode, digest, reg))
	return b.String()
}

// ontologyDigestMaxAssertions bounds how much of the digest reaches the
// instruction. The knowledge layer exists to make retrieval cheaper; a digest
// that carries every row would make the prompt the most expensive part of the
// turn.
const ontologyDigestMaxAssertions = 12

// ontologyDigestMaxVocabulary bounds the vocabulary line the same way.
const ontologyDigestMaxVocabulary = 40

func ontologyDigestBlock(mode ontologyMode, digest *OntologyDigest, reg *EvidenceRegistry) string {
	var b strings.Builder
	b.WriteString("## Declared knowledge (ontology)\n")
	if digest == nil {
		// A failed prefetch is stated rather than hidden: a model that does not
		// know the knowledge layer was unreachable will report its own guesses
		// as if it had checked.
		b.WriteString("The knowledge layer could not be read for this turn. Do not claim that the declared model " +
			"supported or contradicted anything; say the knowledge layer was unavailable if it matters.\n")
		if mode == ontologyModeDirect {
			b.WriteString("This turn has no document corpus either, so answer only what you can support and say plainly " +
				"what you could not check.\n")
		}
		return b.String()
	}

	ref := digest.Model
	b.WriteString(fmt.Sprintf("- Model: %s (id=%s, version=%d, status=%s)\n",
		firstNonEmpty(ref.Name, ref.ID), ref.ID, ref.Version, firstNonEmpty(ref.PublishStatus, "unknown")))
	if len(digest.Vocabulary) > 0 {
		vocab := digest.Vocabulary
		if len(vocab) > ontologyDigestMaxVocabulary {
			vocab = vocab[:ontologyDigestMaxVocabulary]
		}
		b.WriteString("- Terms this question matched: " + strings.Join(vocab, ", ") + "\n")
	}
	// The evidence grade is stated, never implied. The rows below are either the
	// model's own narrowing or a schema-free retrieval of the same scope, and a
	// model left to infer which from the absence of a "narrowing applied" line
	// presents the second as the first — an attribution the knowledge layer
	// cannot support.
	switch digest.EvidenceLevel {
	case OntologyEvidenceStructured:
		b.WriteString("- Evidence: structured")
		if digest.AppliedFilter != nil {
			b.WriteString(" (narrowed by the declared structure: " + renderFilter(digest.AppliedFilter) + ")")
		}
		b.WriteString(".\n")
	case OntologyEvidenceNone:
		b.WriteString("- Evidence: none (neither the narrowed nor the unnarrowed search returned an assertion).\n")
	default:
		reason := "the question named nothing this model declares"
		if digest.Degrade != nil {
			reason = digest.Degrade.Code + ": " + digest.Degrade.Reason
		}
		b.WriteString("- Evidence: schema-free — the rows below were NOT narrowed by the declared structure (" + reason + "). " +
			"They are the knowledge scope's assertions, not a statement this model makes about the question: read them as leads, " +
			"and do not attribute them to the model.\n")
	}
	// A degrade that the grade line already explained is not repeated. The ones
	// it does not explain — an unresolvable citation set, an unsampled coverage —
	// are facts about the rows the grade says nothing about.
	if digest.Degrade != nil && digest.EvidenceLevel != OntologyEvidenceSchemaFree {
		b.WriteString(fmt.Sprintf("- Degraded (%s): %s\n", digest.Degrade.Code, digest.Degrade.Reason))
	}
	if len(digest.Coverage.MissingClasses) > 0 {
		missing := digest.Coverage.MissingClasses
		if len(missing) > ontologyDigestMaxVocabulary {
			missing = missing[:ontologyDigestMaxVocabulary]
		}
		b.WriteString("- Declared classes with no instance in the knowledge scope: " + strings.Join(missing, ", ") + "\n")
	}
	if digest.Coverage.Truncated {
		b.WriteString(fmt.Sprintf("- Coverage was sampled up to %d rows, so \"no instance\" is only known within that sample.\n",
			digest.Coverage.SampleLimit))
	}

	if len(digest.Assertions) == 0 {
		b.WriteString("- No assertion in the knowledge scope matches this question.\n")
	} else {
		b.WriteString("\n### Assertions already retrieved\n")
		shown := digest.Assertions
		if len(shown) > ontologyDigestMaxAssertions {
			shown = shown[:ontologyDigestMaxAssertions]
		}
		for i, a := range shown {
			b.WriteString(renderOntologyAssertion(i+1, a, reg))
		}
		if len(digest.Assertions) > len(shown) {
			b.WriteString(fmt.Sprintf("- (%d further assertions not shown; use search_ontology to look at them.)\n",
				len(digest.Assertions)-len(shown)))
		}
	}

	if digest.Hypergraph != nil {
		b.WriteString(renderOntologyHypergraph(digest.Hypergraph, reg))
	}

	if digest.Citations.Unresolvable > 0 {
		b.WriteString(fmt.Sprintf("- %d assertion(s) cite chunks that are not readable in this conversation's scope; "+
			"their wording is usable as a search hint, their quotation is not citable.\n", digest.Citations.Unresolvable))
	}
	if digest.UnreviewedCount > 0 {
		b.WriteString(fmt.Sprintf("- %d of these assertions are unreviewed: usable, but say so if a conclusion rests on them.\n",
			digest.UnreviewedCount))
	}

	b.WriteString("\n### How to use this\n")
	switch mode {
	case ontologyModeDirect:
		b.WriteString("- There is no document corpus in this turn. Answer from the assertions and declarations above; " +
			"never invent a fact the knowledge layer does not state.\n")
		b.WriteString("- Every factual claim must be backed by an assertion's quotation, cited as its [ID:n]. " +
			"A claim you cannot back must be reported as a gap, with which declared class or term is uncovered.\n")
		b.WriteString("- If the question is about the model's own definitions (what a class means, which relations it " +
			"allows), answer from the declarations and name the model version.\n")
		b.WriteString("- Do not present a declaration as a fact about the world: a class the model declares need not " +
			"have any instance.\n")
	default:
		b.WriteString("- Use the terms above to phrase your searches, and read the original documents to verify. " +
			"The assertions tell you where to look and what to call it, not what to answer.\n")
		b.WriteString("- The documents named above are the ones that produced this knowledge. You may pass them as " +
			"doc_scope to search inside them first; that is a suggestion, not a limit, and you may search more widely " +
			"when they do not answer.\n")
		b.WriteString("- Where an assertion carries a quotation, cite that quotation's chunk rather than a re-read " +
			"paraphrase of the same passage.\n")
		b.WriteString("- A factual block groups slots that came from one place. Route multi-part questions along a " +
			"block's slots — they are the parts that were stated together — and quote the original chunk, not the block.\n")
		b.WriteString("- If the assertions do not cover the question, do not stretch them: search the corpus, and say " +
			"which part of the answer rests on the knowledge layer and which on the documents.\n")
	}
	return b.String()
}

// ontologyHypergraphMaxBlocks and ontologyHypergraphMaxFactsPerBlock bound the
// hypergraph section. The blocks are pointers into the original text, not a
// substitute for it, so the section stays small on purpose: a block that has to
// be truncated still names its chunks, and reading them is one tool call.
const (
	ontologyHypergraphMaxBlocks        = 4
	ontologyHypergraphMaxFactsPerBlock = 12
)

// renderOntologyHypergraph writes the hypergraph section: the factual blocks
// assembled for this question, and the coverage that says how complete they are.
//
// The coverage line is not decoration. A reader that gets three blocks and no
// coverage number will assume the blocks are the whole story; the number is what
// lets it decide to search instead of to answer.
func renderOntologyHypergraph(hg *OntologyHypergraph, reg *EvidenceRegistry) string {
	if hg == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n### Factual blocks (hypergraph)\n")
	if len(hg.Edges) == 0 {
		reason := "the compiled layer holds no block for this question"
		if hg.Degrade != nil {
			reason = hg.Degrade.Code + ": " + hg.Degrade.Reason
		}
		b.WriteString("- No factual block was assembled (" + reason + ").\n")
		return b.String()
	}
	blocks := hg.Edges
	if len(blocks) > ontologyHypergraphMaxBlocks {
		blocks = blocks[:ontologyHypergraphMaxBlocks]
	}
	for i, block := range blocks {
		head := fmt.Sprintf("%d. [hyperedge:%s] %s (%s",
			i+1, shortOntologyHandle(block.RowID), firstNonEmpty(block.Root, "?"), firstNonEmpty(block.RootType, "?"))
		if block.Depth > 0 {
			head += fmt.Sprintf(", depth %d", block.Depth)
		}
		head += fmt.Sprintf(", explains %d of this question's slots)", block.CoveredNodes)
		if ref := ontologyHypergraphBlockRef(block, reg); ref != "" {
			head += " " + ref
		}
		b.WriteString(head + "\n")
		facts := block.Facts
		if len(facts) > ontologyHypergraphMaxFactsPerBlock {
			facts = facts[:ontologyHypergraphMaxFactsPerBlock]
		}
		for _, fact := range facts {
			b.WriteString("   " + fact.Path + ": " + fact.Value + "\n")
		}
		if len(block.Facts) > len(facts) {
			b.WriteString(fmt.Sprintf("   (%d further slot(s) in this block; read its chunks)\n", len(block.Facts)-len(facts)))
		}
	}
	if len(hg.Edges) > len(blocks) {
		b.WriteString(fmt.Sprintf("- (%d further block(s) not shown.)\n", len(hg.Edges)-len(blocks)))
	}
	cov := hg.Coverage
	b.WriteString(fmt.Sprintf("- Coverage: %d of %d recalled slot(s) covered by %d block(s); node_coverage=%.2f, slot_coverage=%.2f (sufficient: %t).\n",
		cov.CoveredNodes, cov.HitNodes, len(hg.Edges), cov.NodeCoverage, cov.SlotCoverage, cov.Sufficient))
	if cov.OrphanHits > 0 {
		b.WriteString(fmt.Sprintf("- %d recalled slot(s) belong to no block: the blocks are an incomplete view of what was "+
			"recalled, so treat an absent slot as unassembled rather than as absent from the corpus.\n", cov.OrphanHits))
	}
	if cov.Partial {
		b.WriteString("- One recall leg failed, so these coverage numbers are over a smaller slot set than the question warrants.\n")
	}
	return b.String()
}

// ontologyHypergraphBlockRef stamps a block's first readable chunk and returns
// the [ID:n] handle to render. A block whose chunks are unreadable gets no
// handle: the block's facts are then a lead, not a citation.
func ontologyHypergraphBlockRef(block OntologyHyperedge, reg *EvidenceRegistry) string {
	if reg == nil || len(block.ChunkIDs) == 0 {
		return ""
	}
	if n := reg.Stamp(block.ChunkIDs[0]); n >= 0 {
		return fmt.Sprintf("[ID:%d]", n)
	}
	return ""
}

// shortOntologyHandle keeps a handle readable: a row id is a 64-character hash
// and the section has to stay scannable.
func shortOntologyHandle(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

// renderOntologyAssertion writes one assertion line, stamping its citation so
// the model can cite a quotation instead of a re-read passage.
func renderOntologyAssertion(n int, a OntologyAssertion, reg *EvidenceRegistry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d. [%s] ", n, firstNonEmpty(a.Kind, "assertion")))
	switch {
	case a.From != "" || a.To != "":
		b.WriteString(fmt.Sprintf("%s -%s-> %s", firstNonEmpty(a.From, "?"), firstNonEmpty(a.Name, a.Type), firstNonEmpty(a.To, "?")))
		if a.FromType != "" || a.ToType != "" {
			b.WriteString(fmt.Sprintf(" (%s -> %s)", a.FromType, a.ToType))
		}
	default:
		b.WriteString(fmt.Sprintf("%s: %s", firstNonEmpty(a.Type, "?"), firstNonEmpty(a.Name, "?")))
	}
	if len(a.Attributes) > 0 {
		keys := make([]string, 0, len(a.Attributes))
		for k := range a.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+a.Attributes[k])
		}
		b.WriteString(" {" + strings.Join(parts, ", ") + "}")
	}
	if a.ReviewStatus != "" {
		b.WriteString(" (" + a.ReviewStatus + ")")
	}
	b.WriteString("\n")
	if q := strings.TrimSpace(a.Evidence); q != "" {
		b.WriteString("   quote: " + truncateQuote(q, 240))
		if ref := ontologyCitationRef(a, reg); ref != "" {
			b.WriteString(" " + ref)
		}
		b.WriteString("\n")
	}
	if len(a.DocIDs) > 0 {
		b.WriteString("   documents: " + strings.Join(limitStrings(a.DocIDs, 3), ", ") + "\n")
	}
	return b.String()
}

// ontologyCitationRef stamps the assertion's first readable chunk and returns
// the [ID:n] handle to render. An assertion whose chunks are all unreadable
// gets no handle: a citation the reference builder cannot resolve is worse than
// no citation at all.
func ontologyCitationRef(a OntologyAssertion, reg *EvidenceRegistry) string {
	if reg == nil || len(a.ChunkIDs) == 0 {
		return ""
	}
	if n := reg.Stamp(a.ChunkIDs[0]); n >= 0 {
		return fmt.Sprintf("[ID:%d]", n)
	}
	return ""
}

// truncateQuote cuts a quotation on a rune boundary so a multi-byte character
// is never split into an invalid one.
func truncateQuote(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

func limitStrings(values []string, max int) []string {
	if len(values) <= max {
		return values
	}
	return values[:max]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// renderFilter renders the applied narrowing as a stable, sorted string so the
// same filter always reads the same way in a receipt.
func renderFilter(filter map[string]any) string {
	keys := make([]string, 0, len(filter))
	for k := range filter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		switch v := filter[k].(type) {
		case []string:
			parts = append(parts, k+" in ["+strings.Join(v, ", ")+"]")
		case []any:
			items := make([]string, 0, len(v))
			for _, item := range v {
				items = append(items, fmt.Sprint(item))
			}
			parts = append(parts, k+" in ["+strings.Join(items, ", ")+"]")
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return strings.Join(parts, "; ")
}
