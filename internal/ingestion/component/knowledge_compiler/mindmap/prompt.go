package mindmap

import (
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// mindMapExtractionPrompt mirrors MIND_MAP_EXTRACTION_PROMPT verbatim
// (rag/graphrag/general/mind_map_prompt.py). It is the SYSTEM message; the
// user turn is the fixed string "Output:" (see userMessage).
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
  - MUST IN FORMAT OF MARKDOWN

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
