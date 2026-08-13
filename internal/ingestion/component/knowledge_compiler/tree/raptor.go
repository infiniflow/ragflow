// Package tree implements the "tree" variant of KnowledgeCompiler: a
// recursive abstractive summarization tree (RAPTOR). It builds a clustering of
// chunk embeddings and summarizes each cluster with the LLM, recursing upward
// until a single root summary remains.
//
// Clustering uses a single method, watershed (raptor.go::watershed): a 1D
// watershed over the document-ordered embeddings. The granularity of each
// level's clustering is controlled by the "tree_order" integer parameter
// (raptor.go::resolveTreeOrder) — the branching factor of the RAPTOR tree
// (the B+ tree "order"): the average chunk count per level-0 cluster. It
// defaults to DefaultTreeOrder.
//
// See PORT_PLAN.md §3.3.
package tree

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	rfcommon "ragflow/internal/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/tokenizer"
)

// Run executes the tree variant.
func Run(ctx context.Context, deps common.Deps, param common.Param, inputs common.Inputs) (common.Outputs, error) {
	if deps.Embed == nil {
		return common.Outputs{}, fmt.Errorf("tree: embedder required")
	}
	if deps.Chat == nil {
		return common.Outputs{}, fmt.Errorf("tree: chat model required")
	}
	docID := firstNonEmpty(inputs.DocID, deps.DatasetID)
	if docID == "" {
		docID = "unknown"
	}
	llmID := firstNonEmpty(param.LLMID, inputs.LLMID)
	tenantID := deps.TenantID

	treeOrder := resolveTreeOrder(param)
	taskPrompt := resolveRaptorPrompt(param)

	// Build leaf summary products (one per cluster) and recursively summarize up,
	// collecting every node into a plain slice. The component merges these into
	// the upstream chunk stream (matching Python, which appends summaries onto the
	// chunk list). buildTree reads the texts, ids, and embeddings directly from
	// the source chunks.
	var products []common.Product
	if err := buildTree(ctx, deps, llmID, tenantID, docID, inputs.Chunks, treeOrder, taskPrompt, param, &products); err != nil {
		return common.Outputs{}, err
	}

	// Project the RAPTOR tree onto the {entities, relations} structure-graph
	// shape (Python raptor_tree_to_graph) and persist it as entity/relation rows
	// plus a compact graph blob (knowledge_graph_kwd="graph"), so the
	// document-structure /structure/graph endpoint can serve the tree. A failure
	// here must not abort the whole tree compile — the summary nodes are already
	// valid on their own — so it is best-effort and surfaced as a log.
	if graphProds, err := buildTreeGraph(ctx, deps, docID, products); err != nil {
		log.Printf("tree: graph projection failed (best-effort, continuing): %v", err)
	} else {
		products = append(products, graphProds...)
	}

	out := common.Outputs{
		Products: products,
	}

	return out, nil
}

// resolveTreeOrder reads the watershed "tree_order" parameter (an integer
// branching factor, the B+ tree "order", in [MinTreeOrder, MaxTreeOrder]) from
// the component extra map, defaulting to DefaultTreeOrder when absent or invalid.
func resolveTreeOrder(param common.Param) int {
	raw, ok := param.Extra["tree_order"]
	if !ok {
		return DefaultTreeOrder
	}
	v, ok := toInt(raw)
	if !ok || v < MinTreeOrder || v > MaxTreeOrder {
		return DefaultTreeOrder
	}
	return v
}

// toInt coerces the common param-extra value types to int.
func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	default:
		return 0, false
	}
}

const (
	raptorMaxRetries       = 3
	raptorDefaultMaxToken  = 512
	raptorDefaultMaxErrors = 3
)

// raptorTruncationMarkerRE strips model truncation notices that some LLMs emit
// when a response is cut short. Mirrors Python _summarize_texts (raptor.py:405).
var raptorTruncationMarkerRE = regexp.MustCompile(
	strings.Repeat("\u00b7", 6) + "\n由于长度的原因，回答被截断了，要继续吗？|For the content length reason, it stopped, continue?",
)

// resolveMaxToken returns the generation cap (max_tokens) for summaries. Python
// uses max(self._max_token, 512) (raptor.py:403, issue #10235); we honour an
// extra["max_token"] override with the same 512 floor.
func resolveMaxToken(param common.Param) int {
	if v, ok := param.Extra["max_token"]; ok {
		if n, ok := toInt(v); ok && n > 0 {
			if n < raptorDefaultMaxToken {
				return raptorDefaultMaxToken
			}
			return n
		}
	}
	return raptorDefaultMaxToken
}

// resolveMaxErrors returns the per-tree error ceiling before RAPTOR aborts.
// Python defaults _max_errors to 3 and skips individual failed clusters below
// that threshold (raptor.py:_summarize_texts).
func resolveMaxErrors(param common.Param) int {
	if v, ok := param.Extra["max_errors"]; ok {
		if n, ok := toInt(v); ok && n > 0 {
			return n
		}
	}
	return raptorDefaultMaxErrors
}

// buildTree summarizes each cluster (level 0) and recurses upward: each parent
// node is the LLM summary of its child cluster's texts. The root is a single
// "tree" product with a stable id. Every node is appended to products (a
// caller-owned slice), and only the deepest-level summaries are retained for the
// root synthesis. buildTree reads the source texts, ids, and embeddings directly
// from the passed chunks; when the embeddings are missing or incomplete, it
// embeds the texts itself. Every summary node is embedded here too, so the caller
// never calls the embedder directly. treeOrder drives both the top-level
// clustering (performed inside buildTree via watershed) and the recursive
// sub-clustering, so the same method is used at every level of the tree.
func buildTree(ctx context.Context, deps common.Deps, llmID, tenantID, docID string, chunks []common.Chunk, treeOrder int, taskPrompt string, param common.Param, products *[]common.Product) error {
	maxToken := resolveMaxToken(param)
	maxErrors := resolveMaxErrors(param)
	errorCount := 0
	texts := chunkTexts(chunks)
	if len(texts) == 0 {
		return nil
	}
	chunkIDs := chunkIDsOf(chunks)
	// Reuse the embeddings already carried on the source chunks when they are
	// complete; only re-embed the texts as a fallback (e.g. the caller has not
	// pre-embedded, or some rows lack a vector).
	embeddings := chunkVectors(chunks)
	if !embeddingsReady(embeddings, len(texts)) {
		vectors, err := deps.Embed.Encode(ctx, texts)
		if err != nil {
			return err
		}
		embeddings = toFloat64Matrix(vectors)
	}
	labels, err := watershed(embeddings, treeOrder)
	if err != nil {
		return err
	}
	n := len(labels)
	// Group point indices by cluster label.
	groups := map[int][]int{}
	for i := 0; i < n; i++ {
		groups[labels[i]] = append(groups[labels[i]], i)
	}

	rootID := common.StableRowID(tenantID, docID, string(common.VariantTree), "root")
	var (
		maxLevel      = -1
		topLevelTexts []string
	)

	type nodeTask struct {
		pointIdxs []int
		parentID  string
		level     int
	}
	queue := []nodeTask{}
	for _, idxs := range groups {
		queue = append(queue, nodeTask{pointIdxs: idxs, parentID: rootID, level: 0})
	}

	for len(queue) > 0 {
		task := queue[0]
		queue = queue[1:]

		// Gather the source text for this cluster's points and truncate each
		// text to a per-chunk token budget so the cluster fits the LLM context
		// (Python: len_per_chunk = (max_length - max_token) / len(texts);
		// truncate(t, len_per_chunk), raptor.py:389-390).
		content := buildClusterContent(texts, task.pointIdxs, deps.ModelContextLen, maxToken)
		system := raptorSystemHelper + strings.Replace(taskPrompt, "{cluster_content}", content, 1)
		summary, err := summarizeTexts(ctx, deps, llmID, system, raptorTitleInstruction, maxToken)
		if err != nil {
			errorCount++
			if errorCount >= maxErrors {
				return fmt.Errorf("tree: aborted after %d summarization errors: %w", errorCount, err)
			}
			log.Printf("tree: skipping cluster due to summarization error (continuing): %v", err)
			continue
		}
		nodeID := common.StableRowID(tenantID, docID, string(common.VariantTree),
			fmt.Sprintf("L%d", task.level), summary)
		embedding, err := deps.Embed.Encode(ctx, []string{summary})
		if err != nil {
			return err
		}
		var vec []float32
		if len(embedding) > 0 {
			vec = embedding[0]
		}
		*products = append(*products, common.Product{
			ID:       nodeID,
			DocID:    docID,
			TenantID: tenantID,
			Variant:  common.VariantTree,
			Content:  summary,
			Vector:   vec,
			ParentID: task.parentID,
			Meta: map[string]any{
				"title":            titleOf(summary),
				"kind":             "summary",
				"level":            task.level,
				"source_chunk_ids": collectIDs(chunkIDs, task.pointIdxs),
			},
		})
		// Track the deepest-level summaries for the root synthesis: only those
		// are fed to the root, so the root summarises the top of the tree
		// rather than re-summarising every leaf.
		if task.level > maxLevel {
			maxLevel = task.level
			topLevelTexts = topLevelTexts[:0]
		}
		if task.level == maxLevel {
			topLevelTexts = append(topLevelTexts, summary)
		}

		// Recurse: re-cluster this cluster's points and enqueue sub-clusters
		// under this node. The sub-clustering uses the same watershed pass as the
		// top-level (treeOrder is carried through), so every level of the tree is
		// built with the same method. Recursion terminates because the
		// sub-clustering is only re-enqueued when it actually splits the cluster
		// into more than one group (the degenerate single-label guard after the
		// watershed call); once a cluster cannot be split further it stops, and a
		// cluster is always driven down to single-point leaves, so depth is
		// naturally bounded by log_2(len(embeddings)).
		subEmb := make([][]float64, 0, len(task.pointIdxs))
		for _, pi := range task.pointIdxs {
			subEmb = append(subEmb, embeddings[pi])
		}
		subLabels, err := watershed(subEmb, treeOrder)
		if err != nil {
			return err
		}
		subGroups := map[int][]int{}
		for i, li := range subLabels {
			subGroups[li] = append(subGroups[li], task.pointIdxs[i])
		}
		// Only recurse when the sub-cluster was actually split into more than
		// one group. If clustering returns a single label covering every point
		// (the degenerate case — e.g. a corpus whose vectors all share one sign
		// so the watershed never cuts), the subGroups map has exactly one entry
		// equal to task.pointIdxs. Re-enqueuing that same work item at level+1
		// would loop forever with no progress and OOM. A single group means the
		// cluster is already atomic, so stop.
		if len(subGroups) > 1 {
			for _, idxs := range subGroups {
				if len(idxs) <= 1 {
					continue
				}
				queue = append(queue, nodeTask{pointIdxs: idxs, parentID: nodeID, level: task.level + 1})
			}
		}
	}

	// Root product ties the tree together. The root summary is the synthesis of
	// the highest-level summaries (the leaves of the upper tree) using the same
	// standard task prompt Python applies to every cluster — NOT a separate
	// "Synthesize the overall theme" prompt, and NOT the union of all leaf
	// summaries (which would just re-summarize the same text repeatedly).
	if len(topLevelTexts) == 0 {
		// No summaries survived (e.g. every deepest cluster failed while the
		// error budget was not yet exhausted). Return the partial tree without a
		// root node — Python drops the root in this case rather than crashing.
		log.Printf("tree: no top-level summaries produced, skipping root node")
		return nil
	}
	rootContent := buildClusterContent(topLevelTexts, allIndices(len(topLevelTexts)), deps.ModelContextLen, maxToken)
	rootSummary, err := summarizeTexts(ctx, deps, llmID,
		raptorSystemHelper+strings.Replace(taskPrompt, "{cluster_content}", rootContent, 1),
		raptorTitleInstruction, maxToken)
	if err != nil {
		// A failed root must not abort the whole tree: Python drops the root
		// node and returns the rest of the tree.
		log.Printf("tree: root synthesis failed, skipping root node: %v", err)
		return nil
	}
	embedding, err := deps.Embed.Encode(ctx, []string{rootSummary})
	if err != nil {
		log.Printf("tree: root embedding failed, skipping root node: %v", err)
		return nil
	}
	if len(embedding) == 0 {
		log.Printf("tree: root embedding returned no vectors, skipping root node")
		return nil
	}
	*products = append(*products, common.Product{
		ID:       rootID,
		DocID:    docID,
		TenantID: tenantID,
		Variant:  common.VariantTree,
		Content:  rootSummary,
		Vector:   embedding[0],
		ParentID: "",
		Meta:     map[string]any{"title": titleOf(rootSummary), "kind": "root", "level": -1},
	})
	return nil
}

// summarizeTexts asks the LLM for a summary and normalises the response to match
// Python _summarize_texts / _chat:
//   - retries up to raptorMaxRetries times with linear backoff (Python: 3 attempts);
//   - strips a reasoning-model preamble up to the final </think> / </think:6124c78e> tag;
//   - treats a "**ERROR**" marker as a transient failure (Python raises on it, so
//     it is retried like any other LLM error);
//   - strips model truncation notices (raptor.py:405);
//   - trims surrounding whitespace.
//
// systemText is the fully-built system prompt (helper + filled task template) and
// userText is the user turn; the title instruction is passed as the user turn so
// the model emits a one-line title on the first line of the summary.
func summarizeTexts(ctx context.Context, deps common.Deps, llmID, systemText, userText string, maxToken int) (string, error) {
	mt := maxToken
	for attempt := 0; attempt < raptorMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(1+attempt) * time.Second):
			}
		}
		resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
			LLMID:        llmID,
			SystemPrompt: systemText,
			UserPrompt:   userText,
			MaxTokens:    &mt,
		})
		if err != nil {
			continue
		}
		content := resp.Content
		// Strip reasoning preamble up to the final thinking close tag
		// (Python: re.sub(r"^.*</think>", "", response, DOTALL)). The
		// else-if keeps the vendor-specific </think:6124c78e> fallback
		// active only when no standard </think> is present, preserving
		// the original control flow.
		if strings.Contains(content, "</think>") {
			content = rfcommon.StripThinkTrailing(content)
		} else if i := strings.LastIndex(content, "</think:6124c78e>"); i >= 0 {
			content = content[i+len("</think:6124c78e>"):]
		}
		// Python raises on the "**ERROR**" marker; treat it as a retryable error.
		if strings.Contains(content, "**ERROR**") {
			continue
		}
		// Strip model truncation notices (Python _summarize_texts).
		content = raptorTruncationMarkerRE.ReplaceAllString(content, "")
		return strings.TrimSpace(content), nil
	}
	return "", fmt.Errorf("raptor: summarization failed after %d attempts", raptorMaxRetries)
}

// titleOf returns the first line of a summary completion (Python stores
// summary_ti = cnt.split("\n")[0]).
func titleOf(summary string) string {
	if i := strings.IndexByte(summary, '\n'); i >= 0 {
		return summary[:i]
	}
	return summary
}

// buildClusterContent joins the selected texts with a single newline (matching
// Python's "\n".join) and truncates each text to a per-chunk token budget so the
// whole cluster fits the LLM context window (Python: len_per_chunk =
// (max_length - max_token) / len(texts); truncate(t, len_per_chunk)). The token
// budget uses the cl100k_base encoder, mirroring Python's truncate (token-level,
// not character-level).
func buildClusterContent(texts []string, idxs []int, modelContextLen, maxToken int) string {
	if len(idxs) == 0 {
		return ""
	}
	if modelContextLen <= 0 {
		modelContextLen = common.DefaultLLMContextLength
	}
	per := (modelContextLen - maxToken) / len(idxs)
	if per < 1 {
		per = 1
	}
	parts := make([]string, 0, len(idxs))
	for _, i := range idxs {
		parts = append(parts, tokenizer.TrimContentToTokenLimit(texts[i], per))
	}
	return strings.Join(parts, "\n")
}

// allIndices returns [0, 1, ..., n-1] so a whole slice can be fed to
// buildClusterContent without copying.
func allIndices(n int) []int {
	idxs := make([]int, n)
	for i := range idxs {
		idxs[i] = i
	}
	return idxs
}

// chunkTextOf returns the text of a chunk, preferring Text over Content.
func chunkTextOf(c common.Chunk) string {
	return firstNonEmpty(c.Text, c.Content)
}

// chunkHasText reports whether a chunk carries non-blank text. Empty chunks
// contribute nothing to clustering or summarization and are skipped uniformly
// across chunkTexts, chunkIDsOf, and chunkVectors.
func chunkHasText(c common.Chunk) bool {
	return strings.TrimSpace(chunkTextOf(c)) != ""
}

func chunkTexts(chunks []common.Chunk) []string {
	var out []string
	for _, c := range chunks {
		if !chunkHasText(c) {
			continue
		}
		out = append(out, chunkTextOf(c))
	}
	return out
}

// chunkIDsOf returns the id of every non-empty chunk, parallel to chunkTexts.
// When a chunk lacks an id, a synthetic positional id is used so source
// provenance is still recoverable.
func chunkIDsOf(chunks []common.Chunk) []string {
	var out []string
	for i, c := range chunks {
		if !chunkHasText(c) {
			continue
		}
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("chunk-%d", i)
		}
		out = append(out, id)
	}
	return out
}

// chunkVectors returns the pre-computed embedding of every non-empty chunk, in
// document order and parallel to chunkTexts/chunkIDsOf. A chunk without a
// Vector yields a nil entry; buildTree re-embeds the texts when any entry is
// missing so the caller can choose whether to pre-embed.
func chunkVectors(chunks []common.Chunk) [][]float64 {
	out := make([][]float64, 0, len(chunks))
	for _, c := range chunks {
		if !chunkHasText(c) {
			continue
		}
		out = append(out, toFloat64Slice(c.Vector))
	}
	return out
}

// toFloat64Slice converts a float32 vector to float64, returning nil for an
// empty input (so a missing embedding is distinguishable from a zero vector).
func toFloat64Slice(v []float32) []float64 {
	if len(v) == 0 {
		return nil
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}

// embeddingsReady reports whether emb is a complete, usable embedding set for
// n points: exactly n rows, none empty.
func embeddingsReady(emb [][]float64, n int) bool {
	if len(emb) != n {
		return false
	}
	for _, e := range emb {
		if len(e) == 0 {
			return false
		}
	}
	return true
}

// collectIDs returns the chunk ids at the given point indices, de-duplicated
// and in stable order.
func collectIDs(chunkIDs []string, pointIdxs []int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(pointIdxs))
	for _, pi := range pointIdxs {
		if pi < 0 || pi >= len(chunkIDs) {
			continue
		}
		id := chunkIDs[pi]
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func toFloat64Matrix(vecs [][]float32) [][]float64 {
	out := make([][]float64, len(vecs))
	for i, v := range vecs {
		row := make([]float64, len(v))
		for j, x := range v {
			row[j] = float64(x)
		}
		out[i] = row
	}
	return out
}

// defaultRaptorPrompt is the summary task template. It mirrors the production
// prompt from api/db/init_data/compilation_templates/tree.yaml (the tree
// compilation template), NOT the Python fallback in compiler.py:128. The
// {cluster_content} placeholder is filled with the joined cluster text. The
// YAML literal block carries a base indent of 6 spaces before {cluster_content};
// those 6 spaces are part of the prompt and MUST be preserved. A caller may
// override it via extra["prompt"] (Python: raptor_cfg["prompt"]).
const defaultRaptorPrompt = "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize."

// raptorSystemHelper mirrors the leading "You're a helpful assistant.\n\nHelp me
// with the following task.\n\n" wrapper Python prepends to the task prompt.
const raptorSystemHelper = "You're a helpful assistant.\n\nHelp me with the following task.\n\n"

// raptorTitleInstruction is sent as the user turn so the model also emits a
// one-line title on the first line of the summary (Python: "Beside the
// summarization, give a title at the first line of your summarization. Must be
// in the same language as the paragraphs.").
const raptorTitleInstruction = "Beside the summarization, give a title at the first line of your summarization. Must be in the same language as the paragraphs."

// resolveRaptorPrompt returns the summary task template, honouring an
// extra["prompt"] override; falls back to defaultRaptorPrompt.
func resolveRaptorPrompt(param common.Param) string {
	if raw, ok := param.Extra["prompt"]; ok {
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return defaultRaptorPrompt
}
