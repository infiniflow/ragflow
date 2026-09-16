package mindmap

import (
	"fmt"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// mindMapExtractionPrompt is based on MIND_MAP_EXTRACTION_PROMPT
// (rag/graphrag/general/mind_map_prompt.py), extended with the JSON tree and
// source-chunk provenance contract. It is the SYSTEM message; the user turn is
// the fixed string "Output:" (see userMessage).
const mindMapExtractionPrompt = `
- Role: You're a talent text processor to summarize a piece of text into a mind map.

- Step of task:
  1. Generate a title for user's 'TEXT'。
  2. Classify the 'TEXT' into sections of a mind map.
  3. If the subject matter is really complex, split them into sub-sections and sub-subsections.
  4. Add a shot content summary of the bottom level section.

- Output requirement:
  - Generate at least 4 levels.
  - Always try to maximize the number of sub-sections.
  - In language of 'Text'
  - Return JSON only, with this shape:
    {"id":"node title","source_chunk_ids":["chunk id",...],"children":[{"id":"child title","source_chunk_ids":["chunk id",...],"children":[]}]}
  - Every node MUST include source_chunk_ids containing only the IDs of source chunks that support that node.
  - Each source chunk is enclosed by [CHUNK_ID: ...] and [END_CHUNK].

-TEXT-
{input_text}

`

// userMessage is the fixed user turn appended after the rendered system
// prompt (Python: [{"role": "user", "content": "Output:"}]).
const userMessage = "Output:"

// mindmapMaxLength stands in for chat_mdl.max_length — the ChatInvoker seam
// does not expose the model window, so the batch budget uses the same
// conservative constant the other variants use. Python derives the budget as
// max(max_length*0.8, max_length-512).
const mindmapMaxLength = 4096

// batchBudget mirrors max(max_length*0.8, max_length-512).
func batchBudget() int {
	a := mindmapMaxLength * 8 / 10
	b := mindmapMaxLength - 512
	if a > b {
		return a
	}
	return b
}

// renderPrompt mirrors perform_variable_replacements for the single
// {input_text} variable (prompt_variables is empty at the Python call site).
func renderPrompt(text string) string {
	return strings.ReplaceAll(mindMapExtractionPrompt, "{input_text}", text)
}

type mindmapBatch struct {
	text string
	ids  []string
}

func packMindmapBatches(chunks []common.Chunk, tok common.Tokenizer) []mindmapBatch {
	var batches []mindmapBatch
	var sections []string
	var ids []string
	cnt := 0
	flush := func() {
		if len(sections) == 0 {
			return
		}
		batches = append(batches, mindmapBatch{
			text: strings.Join(sections, "\n\n"),
			ids:  append([]string(nil), ids...),
		})
		sections = nil
		ids = nil
		cnt = 0
	}
	for i, chunk := range chunks {
		text := common.FirstNonEmpty(chunk.Text, chunk.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		id := strings.TrimSpace(chunk.ID)
		if id == "" {
			id = "chunk-" + fmt.Sprint(i+1)
		}
		section := "[CHUNK_ID: " + id + "]\n" + text + "\n[END_CHUNK]"
		sectionTokens := numTokens(tok, section)
		if cnt+sectionTokens >= batchBudget() && len(sections) > 0 {
			flush()
		}
		sections = append(sections, section)
		ids = append(ids, id)
		cnt += sectionTokens
	}
	flush()
	return batches
}

// packSections mirrors MindMapExtractor.__call__'s batching: sections are
// concatenated WITHOUT a separator into batches whose token count stays
// under the budget; a single section is never split (an oversized section
// forms its own batch).
func packSections(sections []string, tok common.Tokenizer) []string {
	budget := batchBudget()
	var batches []string
	var cur strings.Builder
	cnt := 0
	for _, s := range sections {
		sc := numTokens(tok, s)
		if cnt+sc >= budget && cur.Len() > 0 {
			batches = append(batches, cur.String())
			cur.Reset()
			cnt = 0
		}
		cur.WriteString(s)
		cnt += sc
	}
	if cur.Len() > 0 {
		batches = append(batches, cur.String())
	}
	return batches
}

func numTokens(tok common.Tokenizer, s string) int {
	if tok != nil {
		return tok.NumTokens(s)
	}
	return common.EstimateTokens(s)
}
