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
	"fmt"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/utility"
)

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
	if len(sections) == 0 {
		return common.Outputs{}, nil
	}

	// One LLM task per token-budget batch (mirrors __call__'s task fan-out).
	batches := packSections(sections, deps.Tokenizer)
	results := make([]utility.OMap, len(batches))
	// One LLM task per token-budget batch (mirrors __call__'s task fan-out).
	n := param.MaxWorkers
	if n <= 0 {
		n = 1
	}
	wp := utility.NewWorkerPool[func() error, struct{}](n, n,
		func(_ context.Context, fn func() error) (struct{}, error) { return struct{}{}, fn() })
	var futs []utility.WorkerPoolFuture[func() error, struct{}]
	for i, text := range batches {
		i, text := i, text
		f, err := wp.Submit(ctx, func() error {
			resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
				LLMID:        llmID,
				SystemPrompt: renderPrompt(text),
				UserPrompt:   userMessage,
			})
			if err != nil {
				return err
			}
			results[i] = utility.Todict(utility.Dictify(utility.StripFences(resp.Content)))
			return nil
		})
		if err != nil {
			wp.StopWait()
			return common.Outputs{}, err
		}
		futs = append(futs, f)
	}
	wp.StopWait()
	for _, f := range futs {
		if res, _ := f.Wait(ctx); res.Err != nil {
			return common.Outputs{}, res.Err
		}
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

// serializeNode renders the tree in Python's MindMapResult.output shape:
// {"id": ..., "children": [...]}.
func serializeNode(node *utility.Node) string {
	var b strings.Builder
	b.WriteString(`{"id":`)
	b.WriteString(quoteJSON(node.ID))
	b.WriteString(`,"children":[`)
	for i, c := range node.Children {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(serializeNode(c))
	}
	b.WriteString("]}")
	return b.String()
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
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
