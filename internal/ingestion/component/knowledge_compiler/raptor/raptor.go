// Package raptor implements the "raptor" variant of KnowledgeCompiler: a
// recursive abstractive summarization tree (RAPTOR). It builds a clustering of
// chunk embeddings and summarizes each cluster with the LLM, recursing upward
// until a single root summary remains. Two builders are delivered:
//
//   - Psi (ClusteringPsi): cosine-similarity union-find forest, zero clustering
//     dependency (lowest risk).
//   - classic (ClusteringAHC): Ward agglomerative clustering with dendrogram-gap
//     cut, preceded by PCA dimensionality reduction (gonum), standing in for
//     Python's unconditional UMAP pre-step.
//   - gmm (ClusteringGMM): diagonal-covariance Gaussian mixture model selected
//     by BIC (pure-Go reimplementation of Python's covariance_type="diag" GMM).
//
// See PORT_PLAN.md §3.3 and the M6 validation gate (缺口 C): classic must meet
// the NMI/ARI/coverage gate or the caller falls back to Psi-only.
package raptor

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// buildTreeMaxDepth caps recursive summarization depth so a pathological
// clustering input (e.g. all-non-negative embeddings that never separate) can
// never drive unbounded recursion in buildTree (M14).
const buildTreeMaxDepth = 12

// Run executes the raptor variant.
func Run(ctx context.Context, deps common.Deps, param common.Param, inputs common.Inputs) (common.Outputs, error) {
	if deps.Embed == nil {
		return common.Outputs{}, fmt.Errorf("raptor: embedder required")
	}
	if deps.Chat == nil {
		return common.Outputs{}, fmt.Errorf("raptor: chat model required")
	}
	docID := firstNonEmpty(inputs.DocID, deps.DatasetID)
	if docID == "" {
		docID = "unknown"
	}
	llmID := firstNonEmpty(param.LLMID, inputs.LLMID)
	tenantID := deps.TenantID

	texts := chunkTexts(inputs.Chunks)
	if len(texts) == 0 {
		return common.Outputs{}, nil
	}
	chunkIDs := chunkIDsOf(inputs.Chunks)

	// Embed all source chunks once.
	vectors, err := deps.Embed.Encode(ctx, texts)
	if err != nil {
		return common.Outputs{}, err
	}
	embeddings := toFloat64Matrix(vectors)

	spec := resolveSpec(param)

	labels, _ := clusterByMethod(embeddings, spec)

	// Build leaf summary products (one per cluster) and recursively summarize up,
	// streaming nodes into the sink so the flush policy caps peak memory.
	sink := common.NewProductSink(ctx, param.Guardrails, inputs.Sink)
	if err := buildTree(ctx, deps, llmID, tenantID, docID, texts, chunkIDs, embeddings, labels, spec, sink); err != nil {
		return common.Outputs{}, err
	}

	out := common.Outputs{
		Products:    sink.Products(),
		VectorBytes: sink.Bytes(),
		Items:       sink.TotalItems(),
		Flushed:     sink.Flushed(),
	}

	if err := out.EnforceGuardrails(param.Guardrails, inputs.Sink, ctx); err != nil {
		return common.Outputs{}, err
	}
	return out, nil
}

func resolveSpec(param common.Param) ClusterSpec {
	spec := ClusterSpec{
		Method:      ClusteringPsi,
		Threshold:   0.0,
		MinClusters: 2,
		MaxClusters: 20,
	}
	if m, ok := param.Extra["clustering_method"].(string); ok && m != "" {
		spec.Method = ClusteringMethod(strings.ToUpper(m))
	}
	if t, ok := param.Extra["clustering_threshold"].(float64); ok {
		spec.Threshold = t
	}
	if v, ok := param.Extra["min_clusters"].(float64); ok {
		spec.MinClusters = int(v)
	}
	if v, ok := param.Extra["max_clusters"].(float64); ok {
		spec.MaxClusters = int(v)
	}
	if spec.Method == ClusteringAHC && spec.Threshold == 0 {
		// AHC uses the dendrogram gap; threshold acts as a *scale* on the gap.
		spec.Threshold = 1.0
	}
	if spec.Method == ClusteringGMM && spec.Threshold == 0 {
		// GMM soft-assign threshold (Python default 0.1).
		spec.Threshold = 0.1
	}
	return spec
}

// buildTree summarizes each cluster (level 0) and recurses upward: each parent
// node is the LLM summary of its child cluster's texts. The root is a single
// "raptor" product with a stable id. Nodes are streamed into sink as they are
// produced (so the flush policy can cap peak memory rather than holding the
// whole tree), and only the deepest-level summaries are retained for the root
// synthesis. spec controls both the top-level clustering (already applied by
// the caller via clusterByMethod) and the recursive sub-clustering, so the same
// method is used at every level of the tree.
func buildTree(ctx context.Context, deps common.Deps, llmID, tenantID, docID string, texts, chunkIDs []string, embeddings [][]float64, labels []int, spec ClusterSpec, sink *common.ProductSink) error {
	n := len(labels)
	// Group point indices by cluster label.
	groups := map[int][]int{}
	for i := 0; i < n; i++ {
		groups[labels[i]] = append(groups[labels[i]], i)
	}

	rootID := common.StableRowID(tenantID, docID, string(common.VariantRaptor), "root")
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

		// Gather the source text for this cluster's points.
		var sb strings.Builder
		for _, pi := range task.pointIdxs {
			sb.WriteString(texts[pi])
			sb.WriteString("\n\n")
		}
		summary, err := summarizeTexts(ctx, deps, llmID, sb.String())
		if err != nil {
			return err
		}
		nodeID := common.StableRowID(tenantID, docID, string(common.VariantRaptor),
			fmt.Sprintf("L%d", task.level), summary)
		embedding, err := deps.Embed.Encode(ctx, []string{summary})
		if err != nil {
			return err
		}
		var vec []float32
		if len(embedding) > 0 {
			vec = embedding[0]
		}
		if err := sink.Add(common.Product{
			ID:       nodeID,
			DocID:    docID,
			TenantID: tenantID,
			Variant:  common.VariantRaptor,
			Content:  summary,
			Vector:   vec,
			ParentID: task.parentID,
			Meta: map[string]any{
				"kind":             "summary",
				"level":            task.level,
				"source_chunk_ids": collectIDs(chunkIDs, task.pointIdxs),
			},
		}); err != nil {
			return err
		}
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

		// If this cluster is large and we are still below the depth cap,
		// recurse: re-cluster its points and enqueue sub-clusters under this
		// node. The sub-clustering uses the same method as the top-level pass
		// (via clusterByMethod) so a user who picks AHC gets AHC at every level,
		// not Psi below AHC. The depth cap guarantees termination even when a
		// pathological clustering input (e.g. all-non-negative embeddings that
		// never separate) would otherwise recurse without progress (M14).
		if task.level < buildTreeMaxDepth && len(task.pointIdxs) > RecursionMinClusterSize {
			subEmb := make([][]float64, 0, len(task.pointIdxs))
			for _, pi := range task.pointIdxs {
				subEmb = append(subEmb, embeddings[pi])
			}
			subLabels, _ := clusterByMethod(subEmb, spec)
			subGroups := map[int][]int{}
			for i, li := range subLabels {
				subGroups[li] = append(subGroups[li], task.pointIdxs[i])
			}
			// Only recurse when the sub-cluster was actually split into more than
			// one group. If clustering returns a single label covering every point
			// (the degenerate case — e.g. PSI threshold=0 on a corpus whose vectors
			// all share one sign, or AHC/GMM forced to a single component), the
			// subGroups map has exactly one entry equal to task.pointIdxs. Re-enqueuing
			// that same work item at level+1 would loop forever with no progress and
			// OOM. A single group means the cluster is already atomic, so stop.
			if len(subGroups) > 1 {
				for _, idxs := range subGroups {
					if len(idxs) <= 1 {
						continue
					}
					queue = append(queue, nodeTask{pointIdxs: idxs, parentID: nodeID, level: task.level + 1})
				}
			}
		}
	}

	// Root product ties the tree together. The root summary is the synthesis
	// of the highest-level summaries (the leaves of the upper tree), NOT the
	// union of all leaf-level summaries — otherwise the root would just
	// re-summarize the same text multiple times.
	rootSummary, err := summarizeTexts(ctx, deps, llmID, "Synthesize the overall document theme from these section summaries:\n\n"+strings.Join(topLevelTexts, "\n"))
	if err != nil {
		return fmt.Errorf("raptor: root summarization failed: %w", err)
	}
	embedding, err := deps.Embed.Encode(ctx, []string{rootSummary})
	if err != nil {
		return fmt.Errorf("raptor: root embedding failed: %w", err)
	}
	if len(embedding) == 0 {
		return fmt.Errorf("raptor: root embedding returned no vectors")
	}
	return sink.Add(common.Product{
		ID:       rootID,
		DocID:    docID,
		TenantID: tenantID,
		Variant:  common.VariantRaptor,
		Content:  rootSummary,
		Vector:   embedding[0],
		ParentID: "",
		Meta:     map[string]any{"kind": "root", "level": -1},
	})
}

func summarizeTexts(ctx context.Context, deps common.Deps, llmID, text string) (string, error) {
	resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: raptorSystemPrompt,
		UserPrompt:   text,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func chunkTexts(chunks []common.Chunk) []string {
	var out []string
	for _, c := range chunks {
		t := firstNonEmpty(c.Text, c.Content)
		if strings.TrimSpace(t) == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// chunkIDsOf returns the id of every non-empty chunk, parallel to chunkTexts.
// When a chunk lacks an id, a synthetic positional id is used so source
// provenance is still recoverable.
func chunkIDsOf(chunks []common.Chunk) []string {
	var out []string
	for i, c := range chunks {
		t := firstNonEmpty(c.Text, c.Content)
		if strings.TrimSpace(t) == "" {
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

const raptorSystemPrompt = `You are a summarization assistant. Produce a concise, information-dense summary of the provided text that preserves key entities, facts, and relationships. Output the summary only.`
