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

// timeout.go — per-component-class Invoke timeout resolution.
//
// Background. The Python port decorates every component's _invoke with
//
//	@timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)))
//
// which applies a uniform 600s ceiling across all components. The Go
// port initially did the same. Operators asked for finer granularity
// because external HTTP lookups (Tavily, DuckDuckGo, ArXiv, ...) and
// fast in-process helpers (ExeSQL, Invoke) blow up the wall-clock budget
// quickly when 10 minutes is the default. The resolution here is:
//
//  1. COMPONENT_EXEC_TIMEOUT_<CLASS>=<seconds> wins for that class.
//  2. Per-class default from componentDefaults.
//  3. COMPONENT_EXEC_TIMEOUT=<seconds> uniform override (kept for
//     back-compat — operators who only set the uniform env var see no
//     change).
//  4. Hard fallback of 600s (Python base.py default).
//
// All env values are parsed as seconds-suffixed durations; invalid or
// non-positive input falls through to the next step. Invalid input must
// never silently widen the timeout.
package canvas

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

type timeoutContextKey struct{}

// WithComponentTimeoutOverride forces all component invokes under ctx to use
// the provided timeout. This lets callers with stricter execution contracts
// reuse canvas execution without mutating the global timeout policy.
func WithComponentTimeoutOverride(ctx context.Context, d time.Duration) context.Context {
	if d <= 0 {
		return ctx
	}
	return context.WithValue(ctx, timeoutContextKey{}, d)
}

// componentDefaults lists the per-class timeout (in seconds) used when
// no per-class env override is set. The class keys are the PascalCase
// names returned by Component.Name() (e.g. "LLM", "Message",
// "TavilySearch", "Retrieval"). Names not in this table fall through to
// the uniform env var or the hard fallback.
//
// Values match the Python port's per-class expectations:
//
//   - LLM / Message / Agent — 600s (long LLM-bound work, matches base.py)
//   - Retrieval — 60s (vector + BM25 + rerank pipeline)
//   - TavilySearch / DuckDuckGo / ArXiv / GoogleScholar / SearXNG / PubMed — 12s
//     (external HTTP — fail fast and let the agent retry/branch)
//   - ExeSQL / Invoke — 3s (in-process / single HTTP hop)
//
// Note: the Go port registers web search as "TavilySearch" (the
// fixture_stubs constant) rather than Python's "Tavily". We list both
// spellings so an operator can target the Go name OR a class name that
// matches Python's.
var componentDefaults = map[string]time.Duration{
	"LLM":           600 * time.Second,
	"Message":       600 * time.Second,
	"Agent":         600 * time.Second,
	"Retrieval":     60 * time.Second,
	"TavilySearch":  12 * time.Second,
	"Tavily":        12 * time.Second, // Python spelling alias
	"DuckDuckGo":    12 * time.Second,
	"ArXiv":         12 * time.Second,
	"GoogleScholar": 12 * time.Second,
	"SearXNG":       12 * time.Second,
	"PubMed":        12 * time.Second,
	"ExeSQL":        3 * time.Second,
	"Invoke":        3 * time.Second,
}

// defaultComponentTimeout is the final fallback when neither the
// per-class env, the per-class default table, nor the uniform env var
// yields a value. Matches Python base.py: 10 * 60 = 600s.
const defaultComponentTimeout = 600 * time.Second

// resolveTimeout returns the Invoke timeout for a component of the
// given class (the PascalCase name from Component.Name()).
//
// Resolution order:
//
//  1. COMPONENT_EXEC_TIMEOUT_<UPPERCASE_CLASS> env var, parsed as
//     "<seconds>s". Invalid or non-positive values are ignored.
//  2. Per-class default from componentDefaults.
//  3. COMPONENT_EXEC_TIMEOUT env var (uniform override, seconds),
//     parsed as "<seconds>s".
//  4. defaultComponentTimeout (600s).
//
// Passing an empty class name still honours steps 3 and 4 — useful for
// callers that don't know the class (e.g. a generic dispatcher).
func resolveTimeout(componentClass string) time.Duration {
	upper := strings.ToUpper(strings.TrimSpace(componentClass))
	if upper != "" {
		if d, ok := parseSecondsEnv("COMPONENT_EXEC_TIMEOUT_" + upper); ok {
			return d
		}
	}
	if upper != "" {
		// componentDefaults uses PascalCase keys; try the original
		// spelling (an uppercased lookup would never match).
		if d, ok := componentDefaults[componentClass]; ok {
			return d
		}
	}
	if d, ok := parseSecondsEnv("COMPONENT_EXEC_TIMEOUT"); ok {
		return d
	}
	return defaultComponentTimeout
}

func resolveTimeoutFromContext(ctx context.Context, componentClass string) time.Duration {
	if ctx != nil {
		if d, ok := ctx.Value(timeoutContextKey{}).(time.Duration); ok && d > 0 {
			return d
		}
	}
	return resolveTimeout(componentClass)
}

// parseSecondsEnv reads an env var and parses it as seconds ("42" →
// 42s). Returns (d, true) on success, (0, false) if the env var is
// unset / empty / non-numeric / non-positive. Invalid input must
// never widen the timeout silently.
func parseSecondsEnv(name string) (time.Duration, bool) {
	v := os.Getenv(name)
	if v == "" {
		return 0, false
	}
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0, false
	}
	return time.Duration(secs) * time.Second, true
}
