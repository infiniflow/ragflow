// Package planexecute provides the Plan-Execute-Replan agent pattern.
//
// Architecture:
//
//	SequentialAgent(Planner, LoopAgent(Executor, Replanner))
//
// The Planner generates an initial step-by-step plan.
// The Executor executes the first uncompleted step.
// The Replanner evaluates progress and either replans (plan_tool) or responds (respond_tool).
// The loop repeats until MaxLoopIterations is reached. The respond_tool is configured
// as ReturnDirectly, which causes the replanner sub-agent to return early, but does NOT
// propagate a BreakLoopAction to the outer LoopAgent — loop termination is guaranteed
// only by MaxLoopIterations. For custom termination, provide a RespondTool that emits
// an Exit action or set MaxLoopIterations appropriately.
package planexecute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ============================================================
// Session value keys
// ============================================================

const (
	sessionKeyPlan      = "__planexecute_plan"
	sessionKeyStepsDone = "__planexecute_steps_done"
)

// ============================================================
// Plan interface and default implementation
// ============================================================

// Plan represents a structured step-by-step plan.
type Plan interface {
	json.Marshaler
	json.Unmarshaler
	Steps() []string
}

// defaultPlan is the default Plan implementation.
type defaultPlan struct {
	StepList []string `json:"steps"`
}

func (p *defaultPlan) Steps() []string { return p.StepList }
func (p *defaultPlan) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct{ Steps []string `json:"steps"` }{Steps: p.StepList})
}
func (p *defaultPlan) UnmarshalJSON(data []byte) error {
	var aux struct{ Steps []string `json:"steps"` }
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	p.StepList = aux.Steps
	return nil
}

// ============================================================
// Config
// ============================================================

// PlannerConfig configures the planner agent.
type PlannerConfig struct {
	Model       core.Model[*schema.Message]
	Instruction string // overrides default PlannerPrompt
}

// ExecutorConfig configures the executor agent.
type ExecutorConfig struct {
	Model       core.Model[*schema.Message]
	Instruction string // overrides default ExecutorPrompt
	Tools       []core.Tool
}

// ReplannerConfig configures the replanner agent.
type ReplannerConfig struct {
	Model       core.Model[*schema.Message]
	Instruction string // overrides default ReplannerPrompt
	Tools       []core.Tool
}

// Config configures the PlanExecute agent.
type Config struct {
	Planner           *PlannerConfig
	Executor          *ExecutorConfig
	Replanner         *ReplannerConfig
	Name              string
	MaxLoopIterations int // default 10
}

// ============================================================
// Tool definitions
// ============================================================

const (
	toolPlan    = "plan_tool"
	toolRespond = "respond_tool"
)

// planTool allows the planner to output a structured plan.
// The replanner uses it to update the plan.
var planToolDef = core.NewBaseTool(
	toolPlan,
	`Create or update a step-by-step plan. Args: {"steps":["step1","step2",...]}`,
	func(ctx context.Context, args string) (string, error) {
		var in struct {
			Steps []string `json:"steps"`
		}
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("invalid plan args: %w", err)
		}
		plan := &defaultPlan{StepList: in.Steps}
		if err := core.SetRunLocalValue(ctx, sessionKeyPlan, plan); err != nil {
			return "", err
		}
		// Reset steps done when plan changes
		if err := core.SetRunLocalValue(ctx, sessionKeyStepsDone, 0); err != nil {
			return "", err
		}
		return fmt.Sprintf("Plan updated with %d steps", len(in.Steps)), nil
	},
)

// respondTool allows the replanner to signal completion.
var respondToolDef = core.NewBaseTool(
	toolRespond,
	`Signal that the task is complete and respond to the user. Args: {"response":"your final answer"}`,
	func(ctx context.Context, args string) (string, error) {
		var in struct {
			Response string `json:"response"`
		}
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("invalid respond args: %w", err)
		}
		return in.Response, nil
	},
)

// ============================================================
// Prompts
// ============================================================

const PlannerPrompt = `You are a planner agent. Your job is to create a detailed step-by-step plan to accomplish the user's task.

IMPORTANT RULES:
1. Break the task into clear, actionable steps
2. Each step should be a single, focused action
3. Steps should be in logical order
4. Use the plan_tool to output your plan
5. After creating the plan, transfer to the executor agent

Use the plan_tool with the following JSON format:
{"steps": ["Step 1: ...", "Step 2: ...", ...]}`

const ExecutorPrompt = `You are an executor agent. Execute the first uncompleted step of the plan.

IMPORTANT RULES:
1. The plan and completed steps are available as context
2. Execute ONLY the current step — do not skip ahead
3. Use available tools to accomplish the step
4. When you finish the step, it will be marked as completed
5. After completing the step, transfer to the replanner agent for evaluation

Current objective: {objective}
Current plan: {plan}
Completed steps: {completed_steps}`

const ReplannerPrompt = `You are a replanner agent. Evaluate the progress made and decide whether to continue or respond.

IMPORTANT RULES:
1. Review what was accomplished
2. If more work is needed: use the plan_tool to update the plan, then transfer to the executor
3. If the task is complete: use the respond_tool to provide the final answer
4. Use plan_tool to update the plan when replanning
5. Use respond_tool when the task is done

Available tools:
- plan_tool: Update the plan with new steps (replan)
- respond_tool: Provide the final answer (task complete)

Current objective: {objective}
Current plan: {plan}
Completed steps: {completed_steps}`

// ============================================================
// Agent names
// ============================================================

const (
	agentNamePlanner   = "planner"
	agentNameExecutor  = "executor"
	agentNameReplanner = "replanner"
	agentNameLoop      = "planexecute_loop"
)

// ============================================================
// New — main constructor
// ============================================================

// New creates a Plan-Execute-Replan agent as a ResumableAgent.
func New(ctx context.Context, cfg *Config) (core.ResumableAgent, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.MaxLoopIterations <= 0 {
		cfg.MaxLoopIterations = 10
	}
	if cfg.Name == "" {
		cfg.Name = "plan_execute_agent"
	}

	// Validate configs
	if cfg.Planner == nil || cfg.Planner.Model == nil {
		return nil, fmt.Errorf("planexecute: Planner.Model is required")
	}
	if cfg.Executor == nil || cfg.Executor.Model == nil {
		return nil, fmt.Errorf("planexecute: Executor.Model is required")
	}
	if cfg.Replanner == nil || cfg.Replanner.Model == nil {
		return nil, fmt.Errorf("planexecute: Replanner.Model is required")
	}

	// ---- Create Planner ----
	plannerInstruction := cfg.Planner.Instruction
	if plannerInstruction == "" {
		plannerInstruction = PlannerPrompt
	}

	planner := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model:       cfg.Planner.Model,
		Instruction: plannerInstruction,
		Tools:       []core.Tool{planToolDef},
		MaxIterations: 5,
		GenModelInput: genPlannerInput,
	}).WithName(agentNamePlanner).WithDescription("Generates a step-by-step plan")

	// ---- Create Executor ----
	executorInstruction := cfg.Executor.Instruction
	if executorInstruction == "" {
		executorInstruction = ExecutorPrompt
	}

	executorTools := make([]core.Tool, 0, len(cfg.Executor.Tools)+1)
	executorTools = append(executorTools, cfg.Executor.Tools...)

	executor := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model:         cfg.Executor.Model,
		Instruction:   executorInstruction,
		Tools:         executorTools,
		MaxIterations: 15,
		GenModelInput: genExecutorInput,
	}).WithName(agentNameExecutor).WithDescription("Executes the current plan step")

	// ---- Create Replanner ----
	replannerInstruction := cfg.Replanner.Instruction
	if replannerInstruction == "" {
		replannerInstruction = ReplannerPrompt
	}

	replannerTools := make([]core.Tool, 0, len(cfg.Replanner.Tools)+2)
	replannerTools = append(replannerTools, cfg.Replanner.Tools...)
	replannerTools = append(replannerTools, planToolDef)
	// respond_tool is marked as ReturnDirectly so the agent exits after using it
	returnDirectly := map[string]bool{toolRespond: true}

	replanner := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model:         cfg.Replanner.Model,
		Instruction:   replannerInstruction,
		Tools:         replannerTools,
		ReturnDirectly: returnDirectly,
		MaxIterations: 5,
		GenModelInput: genReplannerInput,
	}).WithName(agentNameReplanner).WithDescription("Evaluates progress and replans or responds")

	// ---- Compose: Sequential(Planner, Loop(Executor, Replanner)) ----
	// The Loop runs: Executor -> Replanner, repeating until Replanner signals done
	loopAgent, err := core.NewLoop(ctx, &core.LoopConfig{
		Name:          agentNameLoop,
		Description:   "Plan-Execute-Replan loop",
		SubAgents:     []core.Agent{executor, replanner},
		MaxIterations: cfg.MaxLoopIterations,
	})
	if err != nil {
		return nil, fmt.Errorf("planexecute: create loop: %w", err)
	}

	// Sequential: Planner -> Loop
	seqAgent, err := core.NewSequential(ctx, &core.SequentialConfig{
		Name:        cfg.Name,
		Description: "Plan-Execute-Replan agent",
		SubAgents:   []core.Agent{planner, loopAgent},
	})
	if err != nil {
		return nil, fmt.Errorf("planexecute: create sequential: %w", err)
	}

	return seqAgent, nil
}

// ============================================================
// GenModelInput functions
// ============================================================

// genPlannerInput builds the input for the planner.
func genPlannerInput(ctx context.Context, instruction string, input *core.AgentInput) ([]*schema.Message, error) {
	msgs := make([]*schema.Message, 0, len(input.Messages)+1)
	if instruction != "" {
		msgs = append(msgs, schema.SystemMessage(instruction))
	}
	msgs = append(msgs, input.Messages...)
	return msgs, nil
}

// genContextualInput builds input with plan context substituted into the instruction.
func genContextualInput(ctx context.Context, instruction string, input *core.AgentInput) ([]*schema.Message, error) {
	planStr := getPlanStr(ctx)
	stepsDone := getStepsDone(ctx)
	objective := getObjective(input.Messages)

	contextStr := strings.NewReplacer(
		"{objective}", objective,
		"{plan}", planStr,
		"{completed_steps}", fmt.Sprintf("%d", stepsDone),
	).Replace(instruction)

	msgs := make([]*schema.Message, 0, len(input.Messages)+1)
	msgs = append(msgs, schema.SystemMessage(contextStr))
	msgs = append(msgs, input.Messages...)
	return msgs, nil
}

// genExecutorInput delegates to the shared helper.
func genExecutorInput(ctx context.Context, instruction string, input *core.AgentInput) ([]*schema.Message, error) {
	return genContextualInput(ctx, instruction, input)
}

// genReplannerInput increments the step counter, then delegates to the shared helper.
func genReplannerInput(ctx context.Context, instruction string, input *core.AgentInput) ([]*schema.Message, error) {
	// Increment steps done: each time the replanner runs, it means the executor
	// just completed a step. The counter is reset to 0 by planTool when the plan
	// is updated, making the next count start fresh.
	currentSteps := getStepsDone(ctx)
	currentSteps++
	_ = core.SetRunLocalValue(ctx, sessionKeyStepsDone, currentSteps)
	return genContextualInput(ctx, instruction, input)
}

// ============================================================
// Helpers
// ============================================================

// getPlanStr retrieves the plan from session and formats it.
func getPlanStr(ctx context.Context) string {
	v, ok, err := core.GetRunLocalValue(ctx, sessionKeyPlan)
	if err != nil || !ok || v == nil {
		return "(no plan yet)"
	}
	p, ok := v.(Plan)
	if !ok {
		return "(plan format error)"
	}
	steps := p.Steps()
	if len(steps) == 0 {
		return "(empty plan)"
	}
	var sb strings.Builder
	for i, s := range steps {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("%d. %s", i+1, s))
	}
	return sb.String()
}

// getStepsDone retrieves the number of completed steps.
func getStepsDone(ctx context.Context) int {
	v, ok, err := core.GetRunLocalValue(ctx, sessionKeyStepsDone)
	if err != nil || !ok {
		return 0
	}
	if n, ok := v.(int); ok {
		return n
	}
	return 0
}

// getObjective extracts the objective from user messages.
func getObjective(msgs []*schema.Message) string {
	for _, m := range msgs {
		if m.Role == schema.RoleUser {
			return m.Content
		}
	}
	return ""
}

func init() {
	schema.RegisterName[defaultPlan]("planexecute_default_plan")
}
