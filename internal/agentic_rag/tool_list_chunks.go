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
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

// listChunksToolName is the constrained deep-read runtime. It reads chunks of
// exactly ONE document in reading order. The document is scoped by doc_id
// (authoritative and unique — it can only belong to one dataset), and each
// returned chunk carries its owning dataset_id in the output.
const listChunksToolName = "list_chunks"

const listChunksToolDescription = `Read the full original text of SELECTED chunks from ONE dataset document, in reading order (Deep Read). Coverage is exactly what you request: the anchor_chunk_ids you pass, expanded by number_neighbors when > 0 — not necessarily the whole document. ANCHOR-DRIVEN: you must name chunk_ids seen in locate output.

Use this AFTER grep_chunks / search_chunks / search_bm25_chunks locate documents: pass their chunk_ids as anchor_chunk_ids to read the complete chunk text — including surrounding context that match snippets and graph triples omit. Cover a document by chaining anchored calls — each reply's doc_chunks_total tells you what is still uncovered — until the span you need is fetched.

The FIRST call for a set of anchors returns, for each anchor, its COMPILED statements when that chunk has any — one atomic fact per line, the verbatim quote it was compiled from, and a ref handle for citing the source chunk — and the chunk's original text when it has none. That is the whole of what these chunks state, so judge on it directly: if the statements already state the fact you need, answer from them and cite their ref handles; if not, or you need the surrounding context, call again with raw=true to read the original text of every anchor. Match snippets and graph triples never carry enough to cite on their own.

## Input
- doc_id: REQUIRED — the document id to read (use the doc_id value from locate-tool output).
- anchor_chunk_ids: REQUIRED — one or more chunk_ids from locate output (at most 20). The read covers these chunks (and their neighborhoods, see below).
- number_neighbors (optional, default 0): 0 fetches ONLY the anchored chunks themselves; N>0 centers a window of N chunks before AND after EVERY anchor (overlapping windows merge).
- raw (optional, default false): false returns each anchor as its compiled statements, or as its text when it has none (see above); true reads the original text of every anchor.

## Output (XML)
Root <chunks> element carries doc_id, fetched, anchored_chunk_ids and number_neighbors; whenever number_neighbors > 0 it also carries doc_chunks_total — the document's total readable-chunk count — so you can tell whether the returned window covered the entire document. Each chunk carries chunk_id/doc_id/page_num/chunk_index/dataset_id/doc_name plus a <content> element with the FULL original text in reading order; doc_name is the doc engine's document name (the docnm field — typically the source file name such as 66090.md). Graph relation/entity chunks are excluded.`

// listChunksArgs is the JSON the model sends into InvokableRun.
type listChunksArgs struct {
	DocID string `json:"doc_id"`
	// Intent is what THIS call is trying to find, in the caller's own words.
	// The filter judges a hit against THIS rather than against the overall
	// question alone, because a necessary middle step of a multi-hop run is
	// often irrelevant to the final question while being exactly what the step
	// needed. Optional: omitted means the filter falls back to the question.
	Intent string `json:"intent,omitempty"`
	// Raw opts out of the claim-first gate: the caller that has already been
	// shown the compiled statements of these chunks and still needs the passage
	// asks again with raw=true.
	Raw             bool     `json:"raw,omitempty"`
	AnchorChunkIDs  []string `json:"anchor_chunk_ids"`
	NumberNeighbors int      `json:"number_neighbors,omitempty"`
}

// Anchored-window bounds: neighbors per side, and the anchor-count cap.
const (
	listChunksMaxNeighbors = 100
	listChunksMaxAnchors   = 20
)

// deepReadService is the capability list_chunks needs on top of
// GrepService. *GrepAdapter implements it via two scoped recalls:
//
//	ListDocumentChunkIndex — Pass 1: id + chunk_order_int for ALL chunks.
//	FetchChunksByID        — Pass 2: full text for the selected ids only.
type deepReadService interface {
	runtime.GrepService
	// ResolveDocDatasetID returns the knowledge-base id that owns the document in
	// req.DocScope, restricted to req.DatasetIDs. It backs list_chunks so the
	// deep-read is scoped to the document's own dataset rather than the whole
	// bound set.
	ResolveDocDatasetID(ctx context.Context, req runtime.GrepRequest) (string, error)
	// ListDocumentChunkIndex returns every ordinary-text chunk of req.DocScope
	// in reading order, carrying only ids and order positions (no content).
	ListDocumentChunkIndex(ctx context.Context, req runtime.GrepRequest) ([]runtime.RetrievalChunk, error)
	// FetchChunksByID returns the full original text of the chunks named in
	// req.ChunkScope, in reading order, skipping graph relation payloads.
	FetchChunksByID(ctx context.Context, req runtime.GrepRequest) ([]runtime.RetrievalChunk, error)
}

// ListChunksTool reads the full original chunks of one document. The tenant and
// dataset scope are injected at construction from the session.
type ListChunksTool struct {
	tenantID   string
	datasetIDs []string
}

// NewListChunksTool returns a ListChunksTool scoped to the given tenant and
// datasets, implementing eino's runtime.InvokableTool.
func NewListChunksTool(tenantID string, datasetIDs []string) *ListChunksTool {
	return &ListChunksTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model. The parameter schema is
// declared as a single JSON string so the model sees the required single-id
// contract and numeric ranges in one readable block, then parsed into
// *jsonschema.Schema.
func (l *ListChunksTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "doc_id": {
      "type": "string",
      "description": "REQUIRED: the document id to read (use the doc_id value from grep_chunks / search_chunks / search_bm25_chunks output)."
    },
    "anchor_chunk_ids": {
      "type": "array",
      "description": "REQUIRED: 1-20 chunk_ids seen in earlier locate output; these chunks (and their neighborhoods) will be read.",
      "items": { "type": "string" },
      "minItems": 1,
      "maxItems": 20
    },
    "number_neighbors": {
      "type": "integer",
      "description": "0 (default) fetches only the anchored chunks themselves; N>0 centers a window of N chunks before AND after every anchor (2N+1 each).",
      "default": 0,
      "minimum": 0,
      "maximum": 100
    },
    "intent": {
      "type": "string",
      "description": "REQUIRED: what information THIS call needs, in one sentence, e.g. the first name of Sarah Polk's mother, needed to build the final name. Each returned passage is kept or dropped by whether it serves this stated need, so name the fact you are looking for - not the search terms, and not 'relevant information'."
    },
    "raw": {
      "type": "boolean",
      "description": "false (default): the first call for these anchors returns their COMPILED statements (atomic facts + verbatim quotes + ref handles) instead of the passage. true: read the original text - use it after those statements did not state the fact you need.",
      "default": false
    }
  },
  "required": ["doc_id", "anchor_chunk_ids", "intent"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("list_chunks: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        listChunksToolName,
		Desc:        listChunksToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun deep-reads one document in two passes: a lightweight ordered
// index over all of the document's ordinary-text chunks, then a content fetch
// narrowed to just the selected ids. Anchor windows are computed on the index
// itself, so they can never drift away from what the model actually receives.
func (l *ListChunksTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, listChunksToolName, l.invokableRun, argumentsInJSON)
}

func (l *ListChunksTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args listChunksArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("list_chunks: parse arguments: %w", err)
	}

	// Published for the decision filter: it judges each passage against this
	// stated need, which is narrower than the overall question.
	ctx = withIntent(ctx, args.Intent)

	docID := strings.TrimSpace(args.DocID)
	if docID == "" {
		return "", fmt.Errorf("list_chunks: doc_id is required")
	}
	if _, err := resolveDocumentScope(ctx, []string{docID}); err != nil {
		return "", fmt.Errorf("list_chunks: %w", err)
	}
	anchors := nonEmptyStrings(args.AnchorChunkIDs)
	if len(anchors) == 0 {
		return "", fmt.Errorf(
			"list_chunks: anchor_chunk_ids is required — pass at least one chunk_id from grep_chunks / search_bm25_chunks / search_chunks output")
	}
	if len(anchors) > listChunksMaxAnchors {
		return "", fmt.Errorf("list_chunks: at most %d anchors allowed, got %d",
			listChunksMaxAnchors, len(anchors))
	}

	// Claim-first: a chunk that carries compiled statements is represented by
	// them, so the read returns those statements instead of the passage. A chunk
	// that carries none is represented by its text, in the SAME answer - one call
	// is always enough for "claims where they exist, the chunk itself where they
	// do not". A caller that finds the statements insufficient re-calls with
	// raw=true and gets the passage.
	var claimBlock string
	readAnchors := anchors
	if !args.Raw {
		block, covered := compiledEvidenceGate(ctx, anchors)
		if block != "" {
			claimBlock = block
			readAnchors = anchorsWithout(anchors, covered)
			if len(readAnchors) == 0 {
				return claimBlock, nil
			}
		}
	} else {
		// Paired with the gate's own line, this is the hit/miss ratio of the
		// claim-first rule: gated reads that were answered from the statements
		// never reach here, gated reads that were not come back with raw=true.
		common.InfoCtx(ctx, "agentic_rag: compiled evidence gate overridden",
			zap.Strings("chunks", anchors))
	}

	svc := runtime.GetGrepService()
	tenantID := l.tenantID
	if svc == nil || tenantID == "" {
		return chunksXMLEmpty(), nil
	}
	dr, ok := svc.(deepReadService)
	if !ok {
		return "", fmt.Errorf("list_chunks: configured grep service does not support deep read")
	}

	// Resolve the document's OWNING dataset so the deep-read is scoped to exactly
	// that dataset, not the whole bound set. A failed resolution (e.g. the doc is
	// no longer in the bound scope) must not fall back to an un-scoped read;
	// return empty instead.
	docDatasetID, err := l.resolveDocDatasetID(ctx, tenantID, docID, l.datasetIDs)
	if err != nil {
		return chunksXMLEmpty(), nil
	}
	scope := runtime.GrepRequest{
		TenantID:   tenantID,
		DatasetIDs: []string{docDatasetID},
		DocScope:   []string{docID},
	}

	nb := min(max(args.NumberNeighbors, 0), listChunksMaxNeighbors)

	// Select which positions to read. Two regimes:
	//   nb == 0 → only the anchors themselves; a single id-filtered content
	//             fetch suffices, no ordered-index pass needed.
	//   nb  > 0 → one ordered index over all of the document's readable chunks,
	//             then merge symmetric windows around every anchor (deduped,
	//             reading order). The index size doubles as doc_chunks_total so
	//             the model can tell whether the windows covered the document.
	var selectIDs []string
	var anchorMeta string
	directAnchorsOnly := nb == 0

	switch {
	case directAnchorsOnly:
		selectIDs = dedupStrings(readAnchors)
		anchorMeta = fmt.Sprintf(` anchored_chunk_ids=%q number_neighbors="0"`,
			xmlEscape(strings.Join(readAnchors, ",")))

	default:
		index, err := dr.ListDocumentChunkIndex(ctx, scope)
		if err != nil {
			return "", fmt.Errorf("list_chunks: %w", err)
		}
		if len(index) == 0 {
			return chunksXMLEmpty(), nil
		}

		posOf := make(map[string]int, len(index))
		for i, c := range index {
			posOf[c.ID] = i
		}
		var missing []string
		mark := make([]bool, len(index))
		for _, a := range readAnchors {
			pos, ok := posOf[a]
			if !ok {
				missing = append(missing, a)
				continue
			}
			for i := max(pos-nb, 0); i <= min(pos+nb, len(index)-1); i++ {
				mark[i] = true
			}
		}
		anchorMeta = fmt.Sprintf(` anchored_chunk_ids=%q doc_chunks_total="%d" number_neighbors="%d"`,
			xmlEscape(strings.Join(readAnchors, ",")), len(index), nb)
		// FAIL FAST on unresolved anchors: an anchor absent from this
		// document means the caller cited an id that does not exist here, so
		// there is nothing trustworthy to expand a window around. Report it
		// in-band (an error would abort the whole ReAct turn over one
		// fabricated identifier) and
		// return immediately: no Pass-2 fetch, and NO partial window, because
		// a partial result would silently mean something other than what was
		// asked for and would read as if the resolved anchors had been
		// verified.
		if len(missing) > 0 {
			common.WarnCtx(ctx, "agentic_rag: list_chunks unresolved anchors",
				zap.String("doc_id", docID), zap.Strings("anchors", readAnchors),
				zap.Strings("missing", missing))
			return claimBlock + formatChunksXML(ctx, docID, nil, anchorMeta,
				unresolvedNotice(missing, len(missing) == len(readAnchors))), nil
		}
		for i, m := range mark {
			if m {
				selectIDs = append(selectIDs, index[i].ID)
			}
		}
	}

	// Pass 2: fetch full text for exactly the selected ids.
	chunks, err := dr.FetchChunksByID(ctx, runtime.GrepRequest{
		TenantID:   tenantID,
		DatasetIDs: []string{docDatasetID},
		DocScope:   []string{docID},
		ChunkScope: selectIDs,
	})
	if err != nil {
		return "", fmt.Errorf("list_chunks: %w", err)
	}
	if directAnchorsOnly && len(chunks) == 0 {
		// Empty is a FINDING, not a failure: it means the cited chunk_id does
		// not resolve (a fabricated or stale identifier) or names a
		// knowledge-compiled/graph product that carries no original text.
		// Returning an error here used to abort the whole ReAct turn over
		// one bad identifier, when the
		// caller (the model) needs to SEE the outcome to judge it. Report it
		// in-band instead.
		common.WarnCtx(ctx, "agentic_rag: list_chunks found no text for the anchors",
			zap.String("doc_id", docID), zap.Strings("anchors", readAnchors))
		return claimBlock + chunksXMLNoText(docID, anchorMeta, len(selectIDs)), nil
	}

	common.DebugCtx(ctx, "agentic_rag: list_chunks result",
		zap.Int("anchors", len(anchors)),
		zap.Int("read_as_text", len(readAnchors)),
		zap.Int("neighbors", nb),
		zap.Bool("direct_anchors_only", directAnchorsOnly),
		zap.Int("selected", len(selectIDs)),
		zap.Int("chunks", len(chunks)),
		zap.String("doc_id", docID),
	)

	// Only the nb=0 path reaches here: the nb>0 path returns early on
	// unresolved anchors, and its empty case is reported by chunksXMLNoText.
	return claimBlock + formatChunksXML(ctx, docID, chunks, anchorMeta, ""), nil
}

// anchorsWithout returns the anchors that are not in keep, preserving order.
func anchorsWithout(anchors, keep []string) []string {
	if len(keep) == 0 {
		return anchors
	}
	kept := make(map[string]struct{}, len(keep))
	for _, id := range keep {
		kept[id] = struct{}{}
	}
	out := make([]string, 0, len(anchors))
	for _, a := range anchors {
		if _, ok := kept[a]; ok {
			continue
		}
		out = append(out, a)
	}
	return out
}

// resolveDocDatasetID returns the knowledge-base id that owns the given document,
// restricted to the bound dataset scope. A document belongs to exactly one kb,
// so the deep-read in list_chunks can be scoped to that single dataset instead
// of the whole bound set. It returns an error when the document has no chunk
// visible within the bound scope or the service cannot resolve the owner.
func (l *ListChunksTool) resolveDocDatasetID(
	ctx context.Context, tenantID, docID string, boundDatasetIDs []string,
) (string, error) {
	if l == nil {
		return "", fmt.Errorf("list_chunks: nil tool")
	}
	dr, ok := runtime.GetGrepService().(deepReadService)
	if !ok {
		return "", fmt.Errorf("list_chunks: configured grep service does not support deep read")
	}
	return dr.ResolveDocDatasetID(ctx, runtime.GrepRequest{
		TenantID:   tenantID,
		DatasetIDs: boundDatasetIDs,
		DocScope:   []string{docID},
	})
}

// chunksXMLEmpty returns an empty deep-read result set in the same XML shape as
// formatChunksXML.
func chunksXMLEmpty() string {
	return `<chunks fetched="0">
</chunks>`
}

// chunksXMLNoText reports the anchors that produced no readable text. The
// wording matters: the caller must be able to distinguish "this chunk_id does
// not exist / was fabricated" (a citation defect to judge) from "the tool
// broke" (an infra failure), and must be told how to recover.
func chunksXMLNoText(docID, anchorMeta string, selected int) string {
	return fmt.Sprintf(`<chunks doc_id=%q%s fetched="0" selected="%d">
<notice>NONE of the anchored chunks produced readable text. Either the chunk_id(s) do not exist in this document (fabricated, stale, or from another document), or they name knowledge-compiled/graph products that carry no original text. Treat every cited field from these chunks as UNVERIFIED - do not restate them. Re-run locate (search_chunks / grep_chunks / search_bm25_chunks) to get real chunk_ids for this document, or call list_chunks with number_neighbors greater than 0 to read around a known-good anchor.</notice>
</chunks>`, docID, anchorMeta, selected)
}

// unresolvedNotice explains anchors that are absent from the document (the
// nb>0 path). Wording matters: the caller must read it as "this citation does
// not resolve" - a defect to judge - rather than "the tool broke", and must be
// told how to recover.
// allUnresolved means every anchor failed, so the chunks carried by the result
// are the document head rather than a window around a resolved anchor.
func unresolvedNotice(missing []string, allUnresolved bool) string {
	if len(missing) == 0 {
		return ""
	}
	lead := fmt.Sprintf("%d of the anchored chunk_ids did not resolve in this document (missing: %s).",
		len(missing), strings.Join(missing, ", "))
	// No chunk text is returned in either case: a window expanded around only
	// the resolved anchors would look like verified evidence.
	if allUnresolved {
		lead += " NO anchor resolved, so no window could be read."
	} else {
		lead += " The rest resolved, but a window around only those would misrepresent what was asked, so none was read."
	}
	return lead + " NO chunk text is returned below. The missing ids are fabricated, stale, or belong to another document, so every field cited from them is UNVERIFIED - do not restate those fields. Re-run locate (search_chunks / grep_chunks / search_bm25_chunks) and copy chunk_ids verbatim from its output, then call list_chunks again with only verified ids."
}
