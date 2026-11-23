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

//go:build !ee

package agentic_rag

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	einocommon "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// The community edition ships no mode 5/6 engine: the plan-explorer-auditor
// stage, the answer auditor, the delivery gate, the locate/served-unread
// watchdogs and the survivors-chain repair all live in the *_ee.go files. These
// stubs keep the shared agent loop and tool registry compiling, and every call
// site degrades to the plain ReAct path (the gate and the plan stage are simply
// absent).

// PlanProducerTemplateID names the planer-explorer-auditor template. In the OSS
// build the template is never resolved, because PlanEngineAvailable reports
// false and the dispatcher keeps the turn on the regular pipeline. Should a
// caller force the template anyway, runPlanStage returns an empty plan and Run
// fails with "plan stage returned no plan" rather than silently answering
// without a plan.
const PlanProducerTemplateID = "planer-explorer-auditor"

// PlanEngineAvailable reports whether this build ships the plan-explorer-auditor
// engine. The community edition does not, so the dispatcher must keep a level-6
// turn on the regular pipeline instead of selecting an engine whose plan stage
// is a stub.
func PlanEngineAvailable() bool { return false }

// IterativeEngineAvailable reports whether this build ships the
// iterative-synthesis engine. The community edition does not.
func IterativeEngineAvailable() bool { return false }

// resolvedQuestionHeader is the plan section the EE stage writes. Unused here.
const resolvedQuestionHeader = "## Resolved question"

// locateWatch and servedUnreadWatch are the EE run watchdogs. In OSS they are
// inert: they hold no state and never nudge the model.
type locateWatch struct{}
type servedUnreadWatch struct{}

func newLocateWatch() *locateWatch { return &locateWatch{} }

func newServedUnreadWatch(served *servedLedger, deep *docIDLedger) *servedUnreadWatch {
	return &servedUnreadWatch{}
}

// observe always returns "": the OSS build never appends a watchdog reminder.
func (*locateWatch) observe(name string) string       { return "" }
func (*servedUnreadWatch) observe(name string) string { return "" }

// markSemanticAvailable is a no-op: the OSS watchdog has no state to record.
func (*locateWatch) markSemanticAvailable() {}

// runPlanStage is unreachable in OSS: the template id never matches.
func runPlanStage(ctx context.Context, in Input) (string, bool, error) { return "", false, nil }

// runConversationalReply is unreachable in OSS for the same reason.
func runConversationalReply(ctx context.Context, in Input) (string, error) { return "", nil }

// extractResolvedQuestion returns "" so the caller falls back to the last user
// message, which is the only question on file without the EE plan stage.
func extractResolvedQuestion(plan string) string { return "" }

// NewAnswerAuditorAgent returns an error so a template that asks for audit
// drops auditMaxPass to 0 (no gate auditing) instead of failing the run.
func NewAnswerAuditorAgent(
	ctx context.Context,
	model einocommon.BaseChatModel,
	tenantID string,
	datasetIDs []string,
	question string,
	toolDurations *durationAccumulator,
	retrievedDocs *docIDLedger,
	chunkReads *chunkReadLedger,
	searches *searchLedger,
) (*adk.ChatModelAgent, error) {
	return nil, errAgenticEngineUnavailable
}

// CheckDecompositionTool and NewCheckDecompositionTool keep the shared tool
// registry compiling. The OSS build registers the tool under its name but the
// plan stage that would call it never runs, so both methods are inert.
type CheckDecompositionTool struct{}

func NewCheckDecompositionTool() *CheckDecompositionTool { return &CheckDecompositionTool{} }

func (t *CheckDecompositionTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "check_decomposition",
		Desc: "Mechanically checks a draft decomposition.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"plan": {Type: schema.String, Required: true, Desc: "The drafted decomposition."},
		}),
	}, nil
}

func (t *CheckDecompositionTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return "", errAgenticEngineUnavailable
}

// decompositionBlock is the EE plan-block type; OSS never builds one.
type decompositionBlock struct{}

// Delivery-gate helpers. OSS never runs the gate, so each returns the zero
// value the shared code treats as "no finding".
func answerLabel(final string) string                           { return "" }
func answerLineCount(s string) int                              { return 0 }
func hasAnswerLine(final string) bool                           { return true }
func hasFOSStructure(final string) bool                         { return true }
func auditPassed(verdict string) bool                           { return true }
func auditSuspectCount(verdict string) int                      { return 0 }
func auditShipsOnAdvisory(verdict string) bool                  { return true }
func auditFindings(verdict string) string                       { return "" }
func auditOpinions(verdict string) []string                     { return nil }
func searchedLineCount(matrix string) int                       { return 0 }
func recordAuditVerdict(audit *GateAuditRecord, verdict string) {}

func buildAuditPayload(final string, searches *searchLedger, unreadDocs []string) string {
	return ""
}

func checkDecompositionBlocks(plan string) ([]*decompositionBlock, []string) { return nil, nil }

func survivorsChainBreaks(final string, blocks []*decompositionBlock) []string { return nil }

func survivorsChainDirective(breaks []string) string { return "" }

func unreadCitations(final string, reads *chunkReadLedger) []string { return nil }

func unreadCitationDirective(unread []string) string { return "" }

func unrecordedLocateTools(matrix string, toolCallCounts map[string]int) []string { return nil }

// errAgenticEngineUnavailable is returned by the OSS stubs that stand in for a
// mode-5/6 capability the community build does not ship.
var errAgenticEngineUnavailable = errors.New("agentic_rag: plan-explorer-auditor engine is not available in the community edition")

// IterativeStatKindRound / IterativeStatKindCommit label a mode-6 round stat.
const (
	IterativeStatKindRound  = "round"
	IterativeStatKindCommit = "commit"
)

// IterativeRoundStat is the mode-6 per-round cost record. The struct is declared
// here because the shared chat result carries it as JSON; the OSS build never
// produces a non-empty slice.
type IterativeRoundStat struct {
	Kind             string         `json:"kind"`
	Round            int            `json:"round,omitempty"`
	ElapsedSeconds   float64        `json:"elapsed_seconds"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	TotalTokens      int            `json:"total_tokens"`
	LLMCalls         int            `json:"llm_calls"`
	ToolCalls        int            `json:"tool_calls,omitempty"`
	ToolCallsByName  map[string]int `json:"tool_calls_by_name,omitempty"`
	Docs             int            `json:"docs,omitempty"`
	SummaryBytes     int            `json:"summary_bytes,omitempty"`
	RubricScore      float64        `json:"rubric_score,omitempty"`
	RubricScored     bool           `json:"rubric_scored,omitempty"`
	Rewritten        bool           `json:"rewritten,omitempty"`
	Stopped          bool           `json:"stopped,omitempty"`
	Error            string         `json:"error,omitempty"`
}

// PlanAuditorTemplateID names the plan auditor template (mode 6). The OSS build
// never resolves it.
const PlanAuditorTemplateID = "plan_auditor"

// PlannerTemperature / AuditTemperature pin the mode-6 planner and auditor
// sampling temperature. OSS has no mode 6, so both report "no override".
func PlannerTemperature() *float64 { return nil }
func AuditTemperature() *float64   { return nil }
