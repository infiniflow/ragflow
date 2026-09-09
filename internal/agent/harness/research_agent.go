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

package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/schema"

	"gorm.io/gorm"
	"ragflow/internal/agent/chat"
)

// Research agent — inner tool-calling loop for high/ultra modes (mirrors Python
// harness/agent.py research_agent_loop + _research_text). The Go chat invoker has
// no native bind_tools, so we implement the *text-fallback* path: the tools are
// described in the prompt and the model emits `<tool_call>` JSON that the loop
// parses and routes to the harness Pipeline. generate_report is captured (not
// executed) and returned as the claim result.

// Tool-call extraction patterns, compiled once at package scope. The (?s) flag
// makes `.` match newlines so a multi-line JSON body (as the prompt shows for
// generate_report) still parses.
var (
	reToolCallTag   = regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	reToolCallFence = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")
	reToolCallBare  = regexp.MustCompile(`\{\s*"name"\s*:`)
)

// researchAgentTextPrompt mirrors rag/prompts/research_agent_prompt.py
// RESEARCH_AGENT_TEXT_PROMPT.
const researchAgentTextPrompt = `You are a research assistant. For the given research task, use the available tools to search for information.

Research task: %s

Current phase: %s
Phase hint: %s

Available tools:
%s

Rules:
1. Go coarse-to-fine: first narrow the corpus with navigation tools, then search only if needed.
2. After a navigation tool returns passages, judge whether they already answer the task.
   If they do, call generate_report immediately instead of searching further.
3. Only use hybrid_search / broad search tools when navigation did not yield sufficient evidence.
4. Use think_tool to analyze results after each step.
5. When you are confident enough to answer the research task, call generate_report.

ATTRIBUTE FIDELITY (CRITICAL):
Answer the EXACT attribute/relation the research task asks for. Do NOT substitute a similar
but different attribute (e.g. do not report HOMETOWN as BIRTHPLACE, do not swap "first" for
"largest", "age at death" for "birth year"). In SEARCH queries you MAY use synonymous,
translated, or corpus-specific terms as long as they still TARGET the exact requested
attribute. The REPORT must state the exact attribute asked for and never present a different
attribute's value as the answer. If the exact attribute cannot be found in evidence, mark the
claim unverified and list it in gaps.

SOURCE ANCHORING (CRITICAL):
If the research task names a specific source, treat THAT named source as authoritative and
retrieve the value from it. Do NOT substitute another source's value as the answer. If the
named source is found, its value wins. Only if it cannot be located may you fall back, and
then say so in the report and lower confidence.

Tool call format: output exactly one JSON tool call per round:
<tool_call>{"name": "tool_name", "arguments": {"parameter_name": "value"}}</tool_call>

generate_report argument format:
{
    "report": "Research result report, factual and unformatted",
    "is_verified": true/false,
    "confidence": 0.0-1.0,
    "evidence_ids": [0, 3],
    "gaps": ["Information that was not found"],
    "grounded": ["answer-critical fact verbatim from evidence"],
    "numbers": ["<figure> from <source/context>"],
    "discovered_claims": ["New research directions discovered during research"]
}

Maximum %d rounds. Output one <tool_call> tag in each round and no other text.`

// navChunkTools mirror Python _NAV_CHUNK_TOOLS: navigation tools that return
// passages, after which we judge sufficiency before broadening into a search.
var navChunkTools = map[string]bool{"ontology_navigate": true, "mindmap_navigate": true}

// researchReport is the normalized generate_report output (mirrors Python
// _generate_report_schema fields).
type researchReport struct {
	Report           string   `json:"report"`
	IsVerified       bool     `json:"is_verified"`
	Confidence       float64  `json:"confidence"`
	EvidenceIDs      []int    `json:"evidence_ids"`
	Gaps             []string `json:"gaps"`
	Grounded         []string `json:"grounded"`
	Numbers          []string `json:"numbers"`
	DiscoveredClaims []string `json:"discovered_claims"`
}

// researchSession accumulates evidence ids across a single claim's tool loop
// (mirrors Python ResearchToolSession).
type researchSession struct {
	pipeline    *Pipeline
	evidenceIDs []int
	seenIDs     map[int]bool
}

// runTool executes one tool call through the pipeline and records evidence.
func (s *researchSession) runTool(ctx context.Context, db *gorm.DB, name string, args map[string]interface{}) (string, bool) {
	res := s.pipeline.Execute(ctx, name, args)
	if res.Error != "" {
		return "[tool error] " + res.Error, false
	}
	// Record the GLOBAL indices Execute captured under mu, so evidence_ids stay
	// stable across concurrent claims (re-indexing the shared slice after the
	// lock is released would race with other goroutines' merges).
	if len(res.EvidenceIndices) > 0 {
		s.recordEvidence(res.EvidenceIndices)
	}
	return formatToolResult(res), len(res.Chunks) > 0
}

// recordEvidence records the global evidence indices Execute already resolved,
// de-duplicating against this session's seen set.
func (s *researchSession) recordEvidence(indices []int) {
	for _, idx := range indices {
		if s.seenIDs[idx] {
			continue
		}
		s.seenIDs[idx] = true
		s.evidenceIDs = append(s.evidenceIDs, idx)
	}
}

// formatToolResult mirrors Python _fmt_tool_result: surface the direct answer
// and up to 6 chunks ordered by similarity, truncated.
func formatToolResult(res ToolResult) string {
	parts := []string{}
	if res.Answer != "" {
		parts = append(parts, "Answer: "+res.Answer)
	}
	chunks := append([]map[string]interface{}(nil), res.Chunks...)
	// Sort by similarity descending (best matches first).
	for i := 0; i < len(chunks); i++ {
		for j := i + 1; j < len(chunks); j++ {
			if chunkSim(chunks[j]) > chunkSim(chunks[i]) {
				chunks[i], chunks[j] = chunks[j], chunks[i]
			}
		}
	}
	for i, c := range chunks {
		if i >= 6 {
			break
		}
		text := chunkText(c)
		if text == "" {
			continue
		}
		if len(text) > 300 {
			text = text[:300]
		}
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return "[no results found]"
	}
	return strings.Join(parts, "\n\n")
}

func chunkSim(c map[string]interface{}) float64 {
	if v, ok := c["similarity"].(float64); ok {
		return v
	}
	if v, ok := c["similarity"].(float32); ok {
		return float64(v)
	}
	return 0
}

// ResearchAgentLoop runs the inner tool loop for one claim (mirrors Python
// research_agent_loop → _research_text). It returns the claim result as an
// AgentResult.
func ResearchAgentLoop(ctx context.Context, db *gorm.DB, pipeline *Pipeline, claim ClaimTarget, mode ExecutionStrategy, followups []string) AgentResult {
	inv := chat.GetDefaultInvoker()
	if inv == nil {
		return AgentResult{ClaimID: claim.ClaimID, IsVerified: false, Confidence: 0, Gaps: []string{"no chat invoker configured"}}
	}

	phase := determinePhase(pipeline)
	phaseHint := phaseHintFor(phase)
	toolList := formatToolList(pipeline.AvailableTools(mode.AvailableTools))

	system := fmt.Sprintf(researchAgentTextPrompt,
		claim.Description, phase, phaseHint, toolList, mode.MaxAgentCycles)

	history := []schema.Message{}
	if len(followups) > 0 {
		history = append(history, schema.Message{Role: schema.User, Content: "Previous evidence was incomplete. Run targeted searches specifically for the following missing pieces:\n- " + strings.Join(followups, "\n- ")})
	}

	session := &researchSession{pipeline: pipeline, seenIDs: map[int]bool{}}

	for cycle := 0; cycle < mode.MaxAgentCycles; cycle++ {
		if err := ctx.Err(); err != nil {
			return AgentResult{ClaimID: claim.ClaimID, IsVerified: false, Confidence: 0, Gaps: []string{"research cancelled: " + err.Error()}}
		}
		msgs := make([]schema.Message, 0, len(history)+1)
		msgs = append(msgs, schema.Message{Role: schema.System, Content: system})
		msgs = append(msgs, history...)
		resp, err := inv.Invoke(ctx, db, chat.Request{
			Messages:    msgs,
			Temperature: floatPtr(0.3),
		})
		if err != nil {
			if ctx.Err() != nil {
				return AgentResult{ClaimID: claim.ClaimID, IsVerified: false, Confidence: 0, Gaps: []string{"research cancelled: " + ctx.Err().Error()}}
			}
			continue
		}
		ans := resp.Content
		history = append(history, schema.Message{Role: schema.Assistant, Content: ans})

		toolCall := parseToolCall(ans)
		if toolCall == nil {
			history = append(history, schema.Message{Role: schema.User, Content: "Please call a tool. Do not output plain text."})
			continue
		}

		name, _ := toolCall["name"].(string)
		if name == "generate_report" {
			args, _ := toolCall["arguments"].(map[string]interface{})
			return reportToAgentResult(claim.ClaimID, args, session.evidenceIDs)
		}
		if name == "think_tool" {
			history = append(history, schema.Message{Role: schema.User, Content: "[continue]"})
			continue
		}

		args, _ := toolCall["arguments"].(map[string]interface{})
		resultText, gotEvidence := session.runTool(ctx, db, name, args)
		// Post-navigation sufficiency gate: steer to finalize when nav passages
		// already answer (best-effort; the LLM judge is skipped for determinism).
		msg := resultText
		if navChunkTools[name] && gotEvidence {
			msg += "\n\n[sufficiency check] These passages may answer the task. If so, call generate_report now — do not run further searches."
		}
		history = append(history, schema.Message{Role: schema.User, Content: msg})
	}

	// Max cycles reached without generate_report — force a final report.
	return forceGenerateReport(ctx, db, inv, claim.ClaimID, history, session)
}

// forceGenerateReport mirrors Python _force_generate_report.
func forceGenerateReport(ctx context.Context, db *gorm.DB, inv chat.Invoker, claimID string, history []schema.Message, session *researchSession) AgentResult {
	if err := ctx.Err(); err != nil {
		return AgentResult{ClaimID: claimID, IsVerified: false, Confidence: 0, Gaps: []string{"research cancelled: " + err.Error()}}
	}
	resp, err := inv.Invoke(ctx, db, chat.Request{
		Messages: append(append([]schema.Message{}, history...),
			schema.Message{Role: schema.User, Content: "We've reached the research limit. Please output a final report as JSON."}),
		Temperature: floatPtr(0.3),
	})
	if err != nil {
		return AgentResult{ClaimID: claimID, IsVerified: false, Confidence: 0, Gaps: []string{"forced report failed: " + err.Error()}}
	}
	var rep researchReport
	if err := unmarshalModelJSON(resp.Content, &rep); err != nil {
		return AgentResult{ClaimID: claimID, IsVerified: false, Confidence: 0, Gaps: []string{"forced report — data may be incomplete"}}
	}
	return reportToAgentResult(claimID, map[string]interface{}{
		"report": rep.Report, "is_verified": rep.IsVerified, "confidence": rep.Confidence,
		"evidence_ids": rep.EvidenceIDs, "gaps": rep.Gaps, "grounded": rep.Grounded,
		"numbers": rep.Numbers, "discovered_claims": rep.DiscoveredClaims,
	}, session.evidenceIDs)
}

// reportToAgentResult normalizes a generate_report argument map into an
// AgentResult, backfilling evidence ids when the model omitted them.
func reportToAgentResult(claimID string, args map[string]interface{}, sessionEvidenceIDs []int) AgentResult {
	report, _ := args["report"].(string)
	isVerified, _ := args["is_verified"].(bool)
	confidence := floatVal(args["confidence"])
	evidenceIDs := intSlice(args["evidence_ids"])
	if len(evidenceIDs) == 0 {
		evidenceIDs = append([]int(nil), sessionEvidenceIDs...)
	}
	gaps := strSlice(args["gaps"])
	grounded := strSlice(args["grounded"])
	numbers := strSlice(args["numbers"])
	discovered := strSlice(args["discovered_claims"])

	return AgentResult{
		ClaimID: claimID, Report: report, IsVerified: isVerified, Confidence: confidence,
		EvidenceIDs: evidenceIDs, Gaps: gaps, Grounded: grounded, Numbers: numbers,
		DiscoveredClaims: discovered,
	}
}

// parseToolCall extracts a tool-call JSON from the model output (mirrors Python
// _parse_tool_call): <tool_call>…</tool_call>, ```json … ```, or bare {"name":…}.
func parseToolCall(text string) map[string]interface{} {
	// <tool_call>…</tool_call>
	if m := reToolCallTag.FindStringSubmatch(text); m != nil {
		if v := tryJSON(m[1]); v != nil {
			return v
		}
	}
	// ```json … ```
	if m := reToolCallFence.FindStringSubmatch(text); m != nil {
		if v := tryJSON(m[1]); v != nil {
			return v
		}
	}
	// bare {"name": …}
	if m := reToolCallBare.FindStringIndex(text); m != nil {
		if v := tryJSON(text[m[0]:]); v != nil {
			return v
		}
	}
	return nil
}

func tryJSON(s string) map[string]interface{} {
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &out); err == nil {
		return out
	}
	return nil
}

func floatVal(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

func intSlice(v interface{}) []int {
	switch x := v.(type) {
	case []int:
		return x
	case []interface{}:
		var out []int
		for _, item := range x {
			switch n := item.(type) {
			case int:
				out = append(out, n)
			case float64:
				out = append(out, int(n))
			}
		}
		return out
	}
	return nil
}

func strSlice(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []interface{}:
		var out []string
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func floatPtr(v float64) *float64 { return &v }

// determinePhase mirrors Python determine_current_phase (simplified: no verdict).
func determinePhase(p *Pipeline) string {
	if p == nil || !p.kbinfos.HasChunks() {
		return "locate"
	}
	return "explore"
}

func phaseHintFor(phase string) string {
	switch phase {
	case "locate":
		return "Prefer navigation tools to locate document regions before directly searching keywords."
	case "explore":
		return "Prefer retrieval tools to gather detailed information within the located region."
	default:
		return ""
	}
}

// formatToolList mirrors Python _fmt_tool_list: name + description + params.
func formatToolList(tools []string) string {
	var lines []string
	for _, name := range tools {
		lines = append(lines, "- "+name+": "+toolDescription(name))
	}
	return strings.Join(lines, "\n")
}

// toolDescription returns a one-line description for prompt tool listing.
func toolDescription(name string) string {
	switch name {
	case "hybrid_search":
		return "Hybrid (vector + keyword) search over the knowledge base."
	case "vector_search":
		return "Dense-vector semantic search over the knowledge base."
	case "bm25_search":
		return "Lexical BM25 keyword search over the knowledge base."
	case "dataset_navigation_by_tree":
		return "Navigate the document tree to locate relevant documents."
	case "ontology_navigate":
		return "Navigate a document's structure/outline to find relevant sections."
	case "mindmap_navigate":
		return "Navigate a mindmap-structured document."
	case "wiki_query":
		return "Query the compiled wiki pages."
	case "graph_explore":
		return "Explore the knowledge graph around a discovered entity."
	case "web_search":
		return "Search the web for external/current facts."
	case "inspector_open_context":
		return "Expand context around an already-returned chunk (2 neighbours each side)."
	case "inspector_compare":
		return "List the document sources of given chunk ids."
	case "inspector_grep_within":
		return "Find a keyword within a document and narrow its chunks to matching sentences."
	case "inspector_request_adjacent":
		return "Get adjacent chunks before/after a given chunk."
	default:
		return "Search tool."
	}
}
