// Package mindmap implements the "mindmap" variant of KnowledgeCompiler,
// mirroring Python's MindMapExtractor: the source chunks are packed into
// token-budget batches; each batch gets one LLM call (system = the rendered
// MIND_MAP_EXTRACTION_PROMPT, user = "Output:") whose markdown reply is
// parsed (dictify semantics), list-to-kv converted, merged across batches,
// and shaped into the {"id","children"} mind-map tree. The tree emits as one
// product per node with parent links.
//
// Per PORT_PLAN.md the Markdown source is the LLM's reply, NOT the source
// document Markdown, so the Parser component is not reused.
package mindmap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/utility"
)

const mindmapJSONRetryMax = 3

// batchSubmitter fans out the batch extraction jobs on the process-wide
// knowledge-compilation pool. It is injected by the knowledge_compiler wiring
// (component.go) so every stage shares one vCPU-sized concurrency bound; when
// nil the batches run sequentially (the historic default).
var batchSubmitter func(ctx context.Context, jobs []func() error) error

// SetBatchSubmitter installs the shared-pool fan-out used by Run's extraction
// stage. Pass nil to revert to serial execution.
func SetBatchSubmitter(submit func(ctx context.Context, jobs []func() error) error) {
	batchSubmitter = submit
}

// runBatches mirrors structure.runBatches: concurrent under the wired global
// compiler pool, or serial when no submitter is set. The first error is
// returned after all jobs settle; the global pool is never StopWait'd.
func runBatches(ctx context.Context, jobs []func() error) error {
	if len(jobs) == 0 {
		return nil
	}
	if batchSubmitter != nil {
		return batchSubmitter(ctx, jobs)
	}
	for _, j := range jobs {
		if err := j(); err != nil {
			return err
		}
	}
	return nil
}

// Run executes the mindmap variant.
func Run(ctx context.Context, deps common.Deps, param common.Param, inputs common.Inputs) (common.Outputs, error) {
	docID := firstNonEmpty(inputs.DocID, deps.DatasetID)
	if docID == "" {
		docID = "unknown"
	}
	llmID := firstNonEmpty(param.LLMID, inputs.LLMID)
	tenantID := deps.TenantID

	if deps.Chat == nil {
		return common.Outputs{}, fmt.Errorf("mindmap: chat model required")
	}

	// One LLM task per token-budget batch (mirrors __call__'s task fan-out).
	batches := packMindmapBatches(inputs.Chunks, deps.Tokenizer)
	results := make([]mindmapBatchResult, len(batches))
	jobs := make([]func() error, 0, len(batches))
	for i, batch := range batches {
		i, batch := i, batch
		jobs = append(jobs, func() error {
			var lastContent string
			for attempt := 0; attempt < mindmapJSONRetryMax; attempt++ {
				resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
					LLMID:           llmID,
					SystemPrompt:    renderPrompt(batch.text),
					UserPrompt:      userMessage,
					JSONMode:        true,
					DisableThinking: true,
				})
				if err != nil {
					return err
				}
				lastContent = resp.Content
				// Distinct slice index per batch → no cross-goroutine contention.
				if tree, ok := parseJSONTree(resp.Content, batch.ids); ok {
					results[i].tree = tree
					return nil
				}
			}
			// Keep the old Markdown protocol as a compatibility fallback after
			// exhausting JSON retries.
			results[i].outline = utility.Todict(utility.Dictify(utility.StripFences(lastContent)))
			return nil
		})
	}
	// The extraction batches are LLM-bounded, not CPU-bounded: run them on the
	// shared global compiler pool (vCPU-sized) when a submitter is wired in,
	// otherwise fall back to serial execution (historic default).
	if err := runBatches(ctx, jobs); err != nil {
		return common.Outputs{}, err
	}

	// Merge batch dicts in batch order (mirrors reduce(self._merge, res)) and
	// shape the final tree. Python returns a bare root when nothing parsed.
	var root *utility.Node
	allJSON := len(results) > 0
	for _, result := range results {
		if result.tree == nil {
			allJSON = false
			break
		}
		root = mergeMindmapTrees(root, result.tree)
	}
	if !allJSON {
		var merged utility.OMap
		for _, result := range results {
			if len(result.outline) == 0 {
				continue
			}
			if len(merged) == 0 {
				merged = result.outline
			} else {
				merged = utility.MergeDicts(merged, result.outline)
			}
		}
		root = utility.ShapeTree(merged)
	}

	products := treeToProducts(tenantID, docID, root, mindmapSourceChunkIDs(inputs.Chunks))

	// Batched embedding of each node's content for downstream vector search.
	if len(products) > 0 && deps.Embed != nil {
		texts := make([]string, len(products))
		for i, p := range products {
			texts[i] = p.Content
		}
		vectors, err := deps.Embed.Encode(ctx, texts)
		if err != nil {
			return common.Outputs{}, err
		}
		for i := range products {
			if i < len(vectors) {
				products[i].Vector = vectors[i]
			}
		}
	}

	// Buffer every tree node in one slice; the component merges them into the
	// upstream chunk stream (matching Python, which appends compiled units onto
	// the chunk list).
	out := common.Outputs{
		Products: products,
	}

	return out, nil
}

// treeToProducts flattens the shaped mind-map tree into entity/relation Products
// so mindmap participates in dataset-level merge exactly like graph (plan §1.2,
// aligning with Python's dataset_structure_merger which merges
// knowledge_graph_kwd IN {entity,relation} rows for structure_mindmap too).
//
// Mapping:
//   - each node (including the root) → an entity product (kind="entity",
//     name = node id, type = "mindmap").
//   - each parent→child edge → a relation product (kind="relation",
//     from = parent id, to = child id, type = "related", matching Python's
//     mindmap structure-graph projection).
//
// The entity/relation discriminator is carried in Meta["kind"] so the consumer's
// mergeStructureDataset buckets entities by (name,type) and relations by
// (from,type,to), matching graph/timeline.
func treeToProducts(tenantID, docID string, root *utility.Node, fallbackSourceChunkIDs []string) []common.Product {
	var out []common.Product
	if root == nil || root.ID == "" {
		return out
	}
	seen := map[string]bool{}
	// Entity: root node.
	rootSourceChunkIDs := nodeSourceChunkIDs(root, fallbackSourceChunkIDs)
	rootPayload := map[string]any{"name": root.ID, "type": "mindmap"}
	rootMeta := map[string]any{
		"kind":        "entity",
		"name":        root.ID,
		"entity_type": "mindmap",
		"compile_kwd": "mindmap",
	}
	if len(rootSourceChunkIDs) > 0 {
		rootPayload["source_chunk_ids"] = rootSourceChunkIDs
		rootMeta["source_chunk_ids"] = rootSourceChunkIDs
	}
	out = append(out, common.Product{
		ID:       common.StableRowID(tenantID, docID, string(common.VariantMindmap), "entity", root.ID),
		DocID:    docID,
		TenantID: tenantID,
		Variant:  common.VariantMindmap,
		Content:  payloadJSON(rootPayload),
		Meta:     rootMeta,
	})
	seen[root.ID] = true

	type pending struct {
		node   *utility.Node
		parent string
	}
	queue := []pending{{root, root.ID}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, child := range p.node.Children {
			if child.ID == "" {
				continue
			}
			// Entity: child node (dedup by id so a DAG-shaped tree does not emit
			// the same node twice).
			if !seen[child.ID] {
				seen[child.ID] = true
				childSourceChunkIDs := nodeSourceChunkIDs(child, fallbackSourceChunkIDs)
				childPayload := map[string]any{"name": child.ID, "type": "mindmap"}
				childMeta := map[string]any{
					"kind":        "entity",
					"name":        child.ID,
					"entity_type": "mindmap",
					"compile_kwd": "mindmap",
				}
				if len(childSourceChunkIDs) > 0 {
					childPayload["source_chunk_ids"] = childSourceChunkIDs
					childMeta["source_chunk_ids"] = childSourceChunkIDs
				}
				out = append(out, common.Product{
					ID:       common.StableRowID(tenantID, docID, string(common.VariantMindmap), "entity", child.ID),
					DocID:    docID,
					TenantID: tenantID,
					Variant:  common.VariantMindmap,
					Content:  payloadJSON(childPayload),
					Meta:     childMeta,
				})
			}
			// Relation: parent → child edge (type = "related", Python default).
			out = append(out, common.Product{
				ID:       common.StableRowID(tenantID, docID, string(common.VariantMindmap), "relation", p.parent, child.ID),
				DocID:    docID,
				TenantID: tenantID,
				Variant:  common.VariantMindmap,
				Content: payloadJSON(map[string]any{
					"source": p.parent,
					"target": child.ID,
					"type":   "related",
				}),
				Meta: map[string]any{
					"kind":          "relation",
					"from":          p.parent,
					"to":            child.ID,
					"relation_type": "related",
					"compile_kwd":   "mindmap",
				},
			})
			queue = append(queue, pending{child, child.ID})
		}
	}
	return out
}

func nodeSourceChunkIDs(node *utility.Node, fallback []string) []string {
	if node != nil && len(node.SourceChunkIDs) > 0 {
		return append([]string(nil), node.SourceChunkIDs...)
	}
	return append([]string(nil), fallback...)
}

func mindmapSourceChunkIDs(chunks []common.Chunk) []string {
	seen := make(map[string]struct{}, len(chunks))
	ids := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		if strings.TrimSpace(firstNonEmpty(chunk.Text, chunk.Content)) == "" {
			continue
		}
		id := strings.TrimSpace(chunk.ID)
		if id == "" {
			id = fmt.Sprintf("chunk-%d", i+1)
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

type mindmapBatchResult struct {
	tree    *utility.Node
	outline utility.OMap
}

type jsonMindmapNode struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	SourceChunkIDs []string          `json:"source_chunk_ids"`
	Children       []jsonMindmapNode `json:"children"`
}

func parseJSONTree(content string, batchIDs []string) (*utility.Node, bool) {
	content, err := common.RepairJSONText(content)
	if err != nil {
		return nil, false
	}
	var raw jsonMindmapNode
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, false
	}
	if strings.TrimSpace(raw.ID) == "" {
		raw.ID = raw.Name
	}
	if strings.TrimSpace(raw.ID) == "" {
		return nil, false
	}
	return convertJSONMindmapNode(raw, batchIDs), true
}

func convertJSONMindmapNode(raw jsonMindmapNode, batchIDs []string) *utility.Node {
	if strings.TrimSpace(raw.ID) == "" {
		raw.ID = raw.Name
	}
	node := &utility.Node{
		ID:             strings.TrimSpace(raw.ID),
		SourceChunkIDs: filterMindmapChunkIDs(raw.SourceChunkIDs, batchIDs),
	}
	for _, child := range raw.Children {
		converted := convertJSONMindmapNode(child, batchIDs)
		if converted.ID != "" {
			node.Children = append(node.Children, converted)
		}
	}
	return node
}

func filterMindmapChunkIDs(ids, batchIDs []string) []string {
	allowed := make(map[string]struct{}, len(batchIDs))
	for _, id := range batchIDs {
		allowed[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(ids))
	selected := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := allowed[id]; !ok {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			selected = append(selected, id)
		}
	}
	if len(selected) == 0 {
		return append([]string(nil), batchIDs...)
	}
	return selected
}

func mergeMindmapTrees(left, right *utility.Node) *utility.Node {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.ID != right.ID {
		return &utility.Node{ID: "root", Children: []*utility.Node{left, right}}
	}
	left.SourceChunkIDs = mergeMindmapIDs(left.SourceChunkIDs, right.SourceChunkIDs)
	byID := make(map[string]*utility.Node, len(left.Children))
	for _, child := range left.Children {
		byID[child.ID] = child
	}
	for _, child := range right.Children {
		if existing := byID[child.ID]; existing != nil {
			mergeMindmapTrees(existing, child)
		} else {
			left.Children = append(left.Children, child)
			byID[child.ID] = child
		}
	}
	return left
}

func mergeMindmapIDs(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	merged := make([]string, 0, len(left)+len(right))
	for _, ids := range [][]string{left, right} {
		for _, id := range ids {
			if id != "" {
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					merged = append(merged, id)
				}
			}
		}
	}
	return merged
}

// payloadJSON serializes the graph payload stored in content_with_weight.
// Structure-graph projection expects entity and relation content to be JSON.
func payloadJSON(payload map[string]any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(data)
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
