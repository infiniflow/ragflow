// Package mindmap implements the "mindmap" variant of KnowledgeCompiler,
// mirroring Python's MindMapExtractor: the source chunks are packed into
// token-budget batches; each batch gets one LLM call (system = the rendered
// MIND_MAP_EXTRACTION_PROMPT, user = "Output:") whose markdown reply is
// parsed (dictify semantics), list-to-kv converted, merged across batches,
// and shaped into the {"id","children"} mind-map tree. The tree emits as one
// product per node with parent links.
//
// Per PORT_PLAN.md the markdown source is the LLM's reply, NOT the source
// document markdown, so the Parser component is not reused.
package mindmap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/utility"
)

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

	sections := chunkTexts(inputs.Chunks)

	// One LLM task per token-budget batch (mirrors __call__'s task fan-out).
	batches := packSections(sections, deps.Tokenizer)
	results := make([]utility.OMap, len(batches))
	jobs := make([]func() error, 0, len(batches))
	for i, text := range batches {
		i, text := i, text
		jobs = append(jobs, func() error {
			resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
				LLMID:        llmID,
				SystemPrompt: renderPrompt(text),
				UserPrompt:   userMessage,
			})
			if err != nil {
				return err
			}
			// Distinct slice index per batch → no cross-goroutine contention.
			results[i] = utility.Todict(utility.Dictify(utility.StripFences(resp.Content)))
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
	var merged utility.OMap
	if len(results) > 0 {
		merged = results[0]
		for _, r := range results[1:] {
			merged = utility.MergeDicts(merged, r)
		}
	}
	root := utility.ShapeTree(merged)

	products := treeToProducts(tenantID, docID, root)

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

// treeToProducts flattens the shaped mind-map tree into Products. The root
// becomes a "root" product whose content is the serialized {"id","children"}
// tree (Python's MindMapResult.output shape); each inner node becomes a
// "node" product linked via parent_id.
//
// NOTE: the per-node "node" products (content = the node title, plus a
// meta["children"] list of immediate child titles) are a Go component-layer
// contract for streaming the tree into the upstream chunk list. They go
// beyond Python's nested dict output and exist so downstream code can index
// each node independently with parent links.
func treeToProducts(tenantID, docID string, root *utility.Node) []common.Product {
	var out []common.Product
	rootID := common.StableRowID(tenantID, docID, string(common.VariantMindmap), "root")
	out = append(out, common.Product{
		ID:       rootID,
		DocID:    docID,
		TenantID: tenantID,
		Variant:  common.VariantMindmap,
		Content:  serializeNode(root),
		Meta: map[string]any{
			"kind":  "root",
			"level": 0,
			"name":  root.ID,
		},
	})

	type pending struct {
		node     *utility.Node
		parentID string
		level    int
	}
	queue := []pending{{root, rootID, 0}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, child := range p.node.Children {
			if child.ID == "" {
				continue
			}
			level := p.level + 1
			id := common.StableRowID(tenantID, docID, string(common.VariantMindmap), "node", child.ID)
			childTitles := make([]string, 0, len(child.Children))
			for _, gc := range child.Children {
				if gc.ID != "" {
					childTitles = append(childTitles, gc.ID)
				}
			}
			meta := map[string]any{
				"kind":  "node",
				"level": level,
				"name":  child.ID,
			}
			if len(childTitles) > 0 {
				meta["children"] = childTitles
			}
			out = append(out, common.Product{
				ID:       id,
				DocID:    docID,
				TenantID: tenantID,
				Variant:  common.VariantMindmap,
				Content:  child.ID,
				ParentID: p.parentID,
				Meta:     meta,
			})
			queue = append(queue, pending{child, id, level})
		}
	}
	return out
}

// jsonNode mirrors the {"id","children"} mind-map node shape (Python's
// MindMapResult.output) for JSON serialization.
type jsonNode struct {
	ID       string      `json:"id"`
	Children []*jsonNode `json:"children"`
}

func toJSONNode(n *utility.Node) *jsonNode {
	jn := &jsonNode{ID: n.ID}
	for _, c := range n.Children {
		jn.Children = append(jn.Children, toJSONNode(c))
	}
	return jn
}

// serializeNode renders the tree in Python's MindMapResult.output shape:
// {"id": ..., "children": [...]}. Uses encoding/json so every control
// character (including \r, \b, \f, etc.) is escaped correctly; the previous
// hand-rolled serializer only escaped ", \, \n and \t.
func serializeNode(node *utility.Node) string {
	b, err := json.Marshal(toJSONNode(node))
	if err != nil {
		return `{"id":"` + node.ID + `","children":[]}`
	}
	return string(b)
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
