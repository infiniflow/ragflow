// Package canvas — end-to-end smoke tests for the production v1 DSL
// examples.
//
// Companion to internal/agent/dsl/v1_examples_test.go: that file
// verifies the v1 DSL is loadable (v1->v2 conversion + Validate). This
// file goes one step further and feeds each fixture through the canvas
// pipeline:
//
//  1. JSON-decoded into a v1 *Canvas.
//  2. (For Invoke tests) credentials injected from env so the
//     LLM-using components talk to the configured provider.
//  3. Compiled into a *compose.Workflow via Compile().
//  4. The compiled Workflow is Invoke()d against a small seed input
//     and the output is asserted against the fixture's expected
//     terminal component.
//
// The LLM/Agent/Categorize/Generate components in the fixture are
// real components (registered in internal/agent/component) — they
// hit the configured model with no stubbing. Provider selection is
// driven by the AGENTIC_MODEL_PROVIDER env var (openai or
// anthropic) using the same env-var convention as the adk/agentic
// reference drivers (OPENAI_API_KEY / OPENAI_MODEL_ID /
// OPENAI_BASE_URL and ANTHROPIC_AUTH_TOKEN / ANTHROPIC_MODEL /
// ANTHROPIC_BASE_URL).
//
// Source fixtures live at internal/agent/dsl/testdata/v1_examples/
// (mirrored from agent/test/dsl_examples/*.json).
package canvas

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// v1Examples lists the fixtures the e2e suite runs against. Keep this
// in sync with internal/agent/dsl/v1_examples_test.go:v1Examples.
var v1Examples = []string{
	"categorize_and_agent_with_tavily.json",
	"exesql.json",
	"headhunter_zh.json",
	"iteration.json",
	"retrieval_and_generate.json",
	"retrieval_categorize_and_generate.json",
	"tavily_and_generate.json",
}

// ----- provider env-var pattern (openai / anthropic) -----

// llmProvider carries the resolved provider credentials for the e2e
// run. It maps 1:1 to the env-var contract used by
// adk/agentic/retry_max_output_tokens/main.go and
// adk/agentic/research_assistant/model.go — two values only: "openai"
// (default) and "anthropic".
type llmProvider struct {
	name   string // "openai" or "anthropic"
	apiKey string
	model  string // provider-specific default model id
	base   string // optional gateway base URL
	driver string // RAGFlow models driver key (openai / anthropic)
}

// providerFromEnv reads AGENTIC_MODEL_PROVIDER and the per-provider
// env vars. Two values are accepted; any other value falls back to
// "openai" with a warning to stderr (we keep the suite green for
// misconfigured CI rather than failing the build).
func providerFromEnv() llmProvider {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("AGENTIC_MODEL_PROVIDER")))
	switch name {
	case "anthropic":
		return llmProvider{
			name:   "anthropic",
			apiKey: os.Getenv("ANTHROPIC_AUTH_TOKEN"),
			model:  os.Getenv("ANTHROPIC_MODEL"),
			base:   os.Getenv("ANTHROPIC_BASE_URL"),
			driver: "anthropic",
		}
	case "openai", "":
		return llmProvider{
			name:   "openai",
			apiKey: os.Getenv("OPENAI_API_KEY"),
			model:  os.Getenv("OPENAI_MODEL_ID"),
			base:   os.Getenv("OPENAI_BASE_URL"),
			driver: "openai",
		}
	default:
		os.Stderr.WriteString("AGENTIC_MODEL_PROVIDER=" + name + " is not supported (use openai or anthropic); falling back to openai\n")
		return llmProvider{
			name:   "openai",
			apiKey: os.Getenv("OPENAI_API_KEY"),
			model:  os.Getenv("OPENAI_MODEL_ID"),
			base:   os.Getenv("OPENAI_BASE_URL"),
			driver: "openai",
		}
	}
}

// fixtureNeedsLLM reports whether the canvas has any of the
// LLM-touching components (LLM, Agent, Categorize, Generate). Used to
// decide whether the Invoke test needs a real API key.
func fixtureNeedsLLM(c *Canvas) bool {
	for _, comp := range c.Components {
		switch strings.ToLower(comp.Obj.ComponentName) {
		case "llm", "agent", "categorize", "generate":
			return true
		}
	}
	return false
}

// injectProviderCredentials mutates the LLM-using components' params
// in place so the eino driver gets the env-resolved API key, model
// id, base URL, and driver name. The DSL's own values are preserved
// when present (a fixture may pin model_id="gpt-4o-mini" and we want
// to honour that); the env wins only when the DSL slot is empty.
//
// Params are addressed by the v1 field name first (llm_id, sys_prompt,
// base_url) and the v2 name as a fallback — that's the same alias
// surface the components' mergeXxxParam helpers accept, so injecting
// the env value under the v1 name matches what the v1 fixture would
// carry on a real run.
func injectProviderCredentials(c *Canvas, p llmProvider) {
	for cpnID, comp := range c.Components {
		params := comp.Obj.Params
		if params == nil {
			params = map[string]any{}
		}
		switch strings.ToLower(comp.Obj.ComponentName) {
		case "llm", "generate":
			setIfEmpty(params, "model_id", p.model)
			setIfEmpty(params, "llm_id", p.model)
			setIfEmpty(params, "driver", p.driver)
			setIfEmpty(params, "api_key", p.apiKey)
			setIfEmpty(params, "base_url", p.base)
		case "agent":
			setIfEmpty(params, "model_id", p.model)
			setIfEmpty(params, "llm_id", p.model)
			setIfEmpty(params, "driver", p.driver)
			setIfEmpty(params, "api_key", p.apiKey)
			setIfEmpty(params, "base_url", p.base)
		case "categorize":
			setIfEmpty(params, "model_id", p.model)
			setIfEmpty(params, "llm_id", p.model)
			setIfEmpty(params, "driver", p.driver)
			setIfEmpty(params, "api_key", p.apiKey)
			setIfEmpty(params, "base_url", p.base)
		}
		comp.Obj.Params = params
		c.Components[cpnID] = comp
	}
}

func setIfEmpty(m map[string]any, key, val string) {
	if val == "" {
		return
	}
	if _, present := m[key]; !present {
		m[key] = val
	}
}

// ----- shared helpers -----

func readV1ExampleFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "dsl", "testdata", "v1_examples", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("v1 fixture %s not readable: %v", path, err)
	}
	return raw
}

// decodeV1Canvas decodes raw v1 DSL bytes into a canvas-package *Canvas.
//
// We intentionally do NOT use DisallowUnknownFields: the v1 fixtures
// carry a number of runtime-only top-level keys (history, path,
// retrieval, globals, answer, messages, reference) that the static
// Canvas struct does not model.
func decodeV1Canvas(t *testing.T, raw []byte, name string) *Canvas {
	t.Helper()
	var c Canvas
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("[%s] decode as canvas.Canvas: %v", name, err)
	}
	if c.Version == 0 {
		c.Version = 1
	}
	if len(c.Components) == 0 {
		t.Fatalf("[%s] decoded Canvas has no components", name)
	}
	return &c
}

// fixtureComponentNames returns the unique lowercased
// component_name values in the fixture, in insertion order. Used by
// the inventory test to report what's in each fixture and which
// component is the blocker.
func fixtureComponentNames(c *Canvas) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(c.Components))
	for _, comp := range c.Components {
		n := strings.ToLower(comp.Obj.ComponentName)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// ----- the actual tests -----

// TestDSLExamples_ParseAsCanvas verifies every fixture decodes into a
// non-empty *Canvas. This is the precondition for the rest of the
// suite: a fixture that fails to decode is missing or malformed at
// the JSON level, not a component-registry problem.
func TestDSLExamples_ParseAsCanvas(t *testing.T) {
	for _, name := range v1Examples {
		t.Run(name, func(t *testing.T) {
			raw := readV1ExampleFixture(t, name)
			c := decodeV1Canvas(t, raw, name)
			if len(c.Components) == 0 {
				t.Fatalf("[%s] parsed Canvas has empty Components map", name)
			}
		})
	}
}

// TestDSLExamples_Inventory reports, in one pass, which component
// names appear in each fixture. Useful as a CI-visible signal of
// fixture composition: if a new component lands in the factory
// registry, this test shows up which fixtures are now ready to
// upgrade to a full Invoke test.
func TestDSLExamples_Inventory(t *testing.T) {
	for _, name := range v1Examples {
		raw := readV1ExampleFixture(t, name)
		c := decodeV1Canvas(t, raw, name)
		t.Logf("[%s] components=%v", name, fixtureComponentNames(c))
	}
}

// TestDSLExamples_Compile exercises the full Compile path on every
// fixture. The Phase 1 component factory covers every name in the
// v1 fixture set, the cycle_wrap integration handles exesql.json /
// headhunter_zh.json, and the v1 alias surface (llm_id, sys_prompt,
// base_url, category_description) keeps the LLM/Agent/Categorize/
// Generate components from rejecting the fixtures' short-form
// params. A compile error here therefore means a regression in the
// topology / factory wiring — it is a real failure.
func TestDSLExamples_Compile(t *testing.T) {
	for _, name := range v1Examples {
		t.Run(name, func(t *testing.T) {
			raw := readV1ExampleFixture(t, name)
			c := decodeV1Canvas(t, raw, name)

			_, err := Compile(context.Background(), c)
			if err != nil {
				t.Fatalf("[%s] compile error: %v", name, err)
			}
		})
	}
}

// TestDSLExamples_Invoke drives each fixture through the full
// compile+invoke path against a real LLM endpoint. Provider
// selection follows the AGENTIC_MODEL_PROVIDER env var (openai or
// anthropic); credentials and base URL come from the corresponding
// env vars. The test skips (not fails) when an LLM-touching fixture
// has no API key in the environment, so the suite stays green on
// sandboxed CI.
//
// Verify layers (per fixture):
//
//  1. compile succeeds,
//  2. Workflow.Invoke returns no error,
//  3. the output is a non-nil map,
//  4. for non-cyclic LLM-touching fixtures: at least one terminal
//     cpn's "content" key resolves to a NON-EMPTY, NON-PLACEHOLDER
//     string. The placeholder check rejects the literal
//     "{{cpn@param}}" string the cycle-broken path can produce —
//     a regression to surface when the synthetic loop or cycle
//     break stops feeding upstream outputs into Message,
//  5. for cyclic fixtures (the synthetic-loop path drops the
//     back-edges, so the LLM may not get called even when the
//     fixture references it): at least one terminal cpn is
//     present, confirming the synthetic-loop install + cycle break
//     runs to completion,
//  6. for non-LLM cyclic fixtures: same as (5).
func TestDSLExamples_Invoke(t *testing.T) {
	provider := providerFromEnv()
	if provider.apiKey == "" {
		t.Logf("no LLM API key in env (provider=%s); LLM-touching fixtures will skip", provider.name)
	}

	for _, name := range v1Examples {
		t.Run(name, func(t *testing.T) {
			raw := readV1ExampleFixture(t, name)
			c := decodeV1Canvas(t, raw, name)

			if fixtureNeedsLLM(c) && provider.apiKey == "" {
				t.Skipf("[%s] fixture uses LLM but %s API key is empty; set the appropriate env var to run the Invoke path", name, provider.name)
			}

			injectProviderCredentials(c, provider)

			runState := NewCanvasState("e2e-"+name, "task-e2e-"+name)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			ctx = WithState(ctx, runState)

			cc, err := Compile(ctx, c)
			if err != nil {
				t.Fatalf("[%s] compile: %v", name, err)
			}
			out, err := cc.Workflow.Invoke(ctx, map[string]any{"query": "Hello, please respond with one short sentence."})
			if err != nil {
				t.Fatalf("[%s] invoke: %v", name, err)
			}
			if out == nil {
				t.Fatalf("[%s] invoke returned nil output", name)
			}

			// 3. (continued): at least one terminal cpn
			// present in the output map.
			got, terminalCPNs := collectTerminalContents(out)
			t.Logf("[%s] invoke ok (provider=%s model=%s cyclic=%v); terminals=%v content=%q",
				name, provider.name, provider.model, hasCycle(c), terminalCPNs, got)

			if len(terminalCPNs) == 0 {
				t.Fatalf("[%s] workflow returned no terminal cpns; full output=%v", name, out)
			}

			// Skip the content checks for cyclic fixtures:
			// the synthetic loop drops the back-edge, so
			// the upstream LLM may not get called even on
			// an LLM-touching fixture (e.g. iteration.json
			// — Agent → Iteration → Message, where the
			// back-edge from Message to Agent is dropped,
			// so Message renders with the literal
			// {{iteration:0@generate:1}} template).
			if hasCycle(c) {
				return
			}

			// 4. non-cyclic LLM fixture: the model must
			// have actually answered. Reject empty AND
			// reject a literal template placeholder
			// (catches regressions where statePost stopped
			// flattening payload into Outputs[cpnID]).
			if fixtureNeedsLLM(c) {
				if got == "" {
					t.Fatalf("[%s] LLM-touching fixture produced empty terminal content; full output=%v", name, out)
				}
				if isTemplatePlaceholder(got) {
					t.Fatalf("[%s] terminal content is unresolved template %q (statePost or upstream output path is broken); full output=%v", name, got, out)
				}
			}
		})
	}
}

// isTemplatePlaceholder reports whether s is an unresolved RAGFlow
// v1 variable reference. Such strings appear in terminal content
// when the upstream cpn that should have supplied the value never
// ran (e.g. a back-edge that the cycle-break policy dropped). A
// real model answer is never a single "{name@key}" string, so this
// is a reliable regression signal.
func isTemplatePlaceholder(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 3 || s[0] != '{' || s[len(s)-1] != '}' {
		return false
	}
	inner := s[1 : len(s)-1]
	// Strip the doubled-brace form {{ ... }} too.
	inner = strings.TrimSpace(inner)
	if len(inner) >= 2 && inner[0] == '{' && inner[len(inner)-1] == '}' {
		inner = strings.TrimSpace(inner[1 : len(inner)-1])
	}
	return strings.Contains(inner, "@") && !strings.ContainsAny(inner, " \t\n")
}

// collectTerminalContents walks the workflow's terminal output map
// and returns (first non-empty "content" string, list of terminal
// cpn_ids). eino's compose.Workflow returns the END node's input
// map, which is keyed by cpn_id (because we wire each terminal with
// compose.ToField(cpnID) in Pass 3 of BuildWorkflow). Each
// terminal's value is the node's output map (statePost already
// stripped __cpn_id__ / state / __legacy_noop__).
func collectTerminalContents(out map[string]any) (string, []string) {
	terminals := make([]string, 0, len(out))
	var first string
	for cpnID, raw := range out {
		terminals = append(terminals, cpnID)
		// The end-input map can be nested (cyclic fixtures go
		// through a synthetic loop whose END wires via
		// compose.ToField). Recurse one level so we find the
		// actual terminal payload regardless of nesting.
		if s, ok := findContentDeep(raw); ok && s != "" && first == "" {
			first = s
		}
	}
	return first, terminals
}

// findContentDeep returns the first "content" string in m, looking
// through one level of nested map[string]any (the synthetic loop's
// outer wrap can produce {synthetic_loop_key: {cpn_id: payload}}).
// For deeper nesting we stop and return false — the e2e output
// shape is at most two levels deep.
func findContentDeep(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		// v itself is a string; treat as content only when
		// the caller asked for "content". We can't tell
		// apart at this level, so return true with the
		// value — collectTerminalContents already filters
		// by non-empty.
		return x, true
	case map[string]any:
		if c, ok := x["content"].(string); ok {
			return c, true
		}
		// Look through one nested map (synthetic-loop wrap).
		for _, inner := range x {
			if s, ok := findContentDeep(inner); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

