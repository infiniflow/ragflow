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

// node_body_per_class_timeout_integration_test.go — pins the
// per-class timeout integration.
//
// Verifies that buildNodeBody's runtime.Component path actually
// plumbs the per-class timeout (canvas/timeout.go's
// componentDefaults table) end-to-end. The per-class table is
// real; realComponentBody must call resolveTimeout(class) instead
// of the uniform componentTimeout() — this test pins that
// contract so a future refactor cannot silently regress to a
// uniform timeout.

package canvas

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
)

// captureCtxComponent is a runtime.Component that records the
// deadline of the context it was invoked with. The integration
// test then asserts the deadline matches the per-class timeout
// (ExeSQL=3s, TavilySearch=12s, LLM=600s, ...). The Component
// returns immediately on success; on timeout it surfaces a
// context.DeadlineExceeded error so the body can wrap and surface
// it as a timeout error.
type captureCtxComponent struct {
	name      string
	deadline  atomic.Int64 // unix-nano deadline observed
	timeoutOK atomic.Bool  // true if ctx had a non-zero deadline
}

func (c *captureCtxComponent) Name() string { return c.name }
func (c *captureCtxComponent) Invoke(ctx context.Context, _ map[string]any) (map[string]any, error) {
	if dl, ok := ctx.Deadline(); ok {
		c.deadline.Store(dl.UnixNano())
		c.timeoutOK.Store(true)
	} else {
		c.timeoutOK.Store(false)
	}
	// Block until ctx times out so the body can wrap and surface
	// the timeout error; that is the path real components hit
	// when they overrun the configured ceiling.
	<-ctx.Done()
	return nil, ctx.Err()
}
func (c *captureCtxComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}
func (c *captureCtxComponent) Inputs() map[string]string  { return nil }
func (c *captureCtxComponent) Outputs() map[string]string { return nil }

var _ runtime.Component = (*captureCtxComponent)(nil)

// TestBuildNodeBody_PerClassTimeout_ExeSQL_3s asserts that an
// ExeSQL-class component body wraps the Invoke context with a
// 3-second deadline (per the componentDefaults table in
// timeout.go), not the uniform 600s fallback or the
// COMPONENT_EXEC_TIMEOUT value (which we leave at the default).
//
// Steps:
//
//  1. Register a factory that returns a captureCtxComponent for
//     the class "ExeSQL".
//  2. Call buildNodeBody("cpn", "ExeSQL", nil) — must NOT take
//     the legacyNoOp / placeholder / UserFillUp branch.
//  3. Invoke the body with a fresh context, capture the
//     component-observed deadline via the captureCtxComponent.
//  4. Assert the deadline is within ±500ms of 3s, AND the body
//     surfaces a wrapped context.DeadlineExceeded (the
//     realComponentBody timeout-wrap contract).
func TestBuildNodeBody_PerClassTimeout_ExeSQL_3s(t *testing.T) {
	// Make sure uniform env is unset so the per-class table
	// is the only authority; the per-class table is the one
	// that defines "ExeSQL → 3s".
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "")
	t.Setenv("COMPONENT_EXEC_TIMEOUT_EXESQL", "")

	cap := &captureCtxComponent{name: "ExeSQL"}
	origFactory := runtime.DefaultFactory()
	runtime.ResetDefaultFactoryForTesting()
	runtime.SetDefaultFactory(func(class string, _ map[string]any) (runtime.Component, error) {
		if class == "ExeSQL" {
			return cap, nil
		}
		return nil, errors.New("not used in this test: " + class)
	})
	t.Cleanup(func() {
		runtime.ResetDefaultFactoryForTesting()
		if origFactory != nil {
			runtime.SetDefaultFactory(origFactory)
		}
	})

	body, err := buildNodeBody("cpn-exe", "ExeSQL", nil)
	if err != nil {
		t.Fatalf("buildNodeBody: %v", err)
	}
	start := time.Now()
	_, err = body(context.Background(), nil)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("body err = %v, want context.DeadlineExceeded (per-class timeout should fire)", err)
	}
	if !cap.timeoutOK.Load() {
		t.Fatal("component never observed a deadline — realComponentBody did not wrap with context.WithTimeout")
	}
	// Allow a 500ms slack on each side: lower bound is to ensure
	// the body actually waited for the timeout, upper bound is to
	// catch over-eager timeouts (e.g. 600s would make the test
	// hang for 10 minutes).
	deadline := time.Unix(0, cap.deadline.Load())
	sinceDeadline := deadline.Sub(start)
	if sinceDeadline < 2500*time.Millisecond || sinceDeadline > 3500*time.Millisecond {
		t.Errorf("ExeSQL deadline offset = %s, want ~3s (got actual elapsed %s)", sinceDeadline, elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("body did not honour 3s timeout: elapsed=%s", elapsed)
	}
}

// TestBuildNodeBody_PerClassTimeout_TavilySearch_12s asserts the
// 12-second web-search class. A Tavily call that hangs on the
// upstream API should give up after 12s and let the agent
// branch / retry, not consume 600s of wall-clock budget.
func TestBuildNodeBody_PerClassTimeout_TavilySearch_12s(t *testing.T) {
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "")
	t.Setenv("COMPONENT_EXEC_TIMEOUT_TAVILYSEARCH", "")

	cap := &captureCtxComponent{name: "TavilySearch"}
	origFactory := runtime.DefaultFactory()
	runtime.ResetDefaultFactoryForTesting()
	runtime.SetDefaultFactory(func(class string, _ map[string]any) (runtime.Component, error) {
		if class == "TavilySearch" {
			return cap, nil
		}
		return nil, errors.New("not used in this test: " + class)
	})
	t.Cleanup(func() {
		runtime.ResetDefaultFactoryForTesting()
		if origFactory != nil {
			runtime.SetDefaultFactory(origFactory)
		}
	})

	body, err := buildNodeBody("cpn-tav", "TavilySearch", nil)
	if err != nil {
		t.Fatalf("buildNodeBody: %v", err)
	}
	start := time.Now()
	_, err = body(context.Background(), nil)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("body err = %v, want context.DeadlineExceeded", err)
	}
	deadline := time.Unix(0, cap.deadline.Load())
	sinceDeadline := deadline.Sub(start)
	if sinceDeadline < 11*time.Second || sinceDeadline > 13*time.Second {
		t.Errorf("TavilySearch deadline offset = %s, want ~12s (got elapsed %s)", sinceDeadline, elapsed)
	}
	if elapsed > 14*time.Second {
		t.Errorf("body did not honour 12s timeout: elapsed=%s", elapsed)
	}
}

// TestBuildNodeBody_PerClassTimeout_UnknownClass_UniformFallback
// asserts that a class NOT in the componentDefaults table (e.g.
// "CustomComponent") falls through to the uniform
// COMPONENT_EXEC_TIMEOUT env var. This is the 4-level resolution
// contract from timeout.go: per-class env → per-class table →
// uniform env → 600s fallback.
//
// We test the uniform-env fallback path (level 3) by setting
// COMPONENT_EXEC_TIMEOUT to a non-default value and registering
// a class that is NOT in componentDefaults.
func TestBuildNodeBody_PerClassTimeout_UnknownClass_UniformFallback(t *testing.T) {
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "5")
	t.Setenv("COMPONENT_EXEC_TIMEOUT_CUSTOMCOMPONENT", "")

	cap := &captureCtxComponent{name: "CustomComponent"}
	origFactory := runtime.DefaultFactory()
	runtime.ResetDefaultFactoryForTesting()
	runtime.SetDefaultFactory(func(class string, _ map[string]any) (runtime.Component, error) {
		if class == "CustomComponent" {
			return cap, nil
		}
		return nil, errors.New("not used in this test: " + class)
	})
	t.Cleanup(func() {
		runtime.ResetDefaultFactoryForTesting()
		if origFactory != nil {
			runtime.SetDefaultFactory(origFactory)
		}
	})

	body, err := buildNodeBody("cpn-cust", "CustomComponent", nil)
	if err != nil {
		t.Fatalf("buildNodeBody: %v", err)
	}
	start := time.Now()
	_, err = body(context.Background(), nil)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("body err = %v, want context.DeadlineExceeded", err)
	}
	deadline := time.Unix(0, cap.deadline.Load())
	sinceDeadline := deadline.Sub(start)
	// 5s uniform env should win for an unknown class. Allow a
	// 1s slack on each side.
	if sinceDeadline < 4*time.Second || sinceDeadline > 6*time.Second {
		t.Errorf("CustomComponent deadline offset = %s, want ~5s (uniform env fallback; elapsed %s)", sinceDeadline, elapsed)
	}
	if elapsed > 7*time.Second {
		t.Errorf("body did not honour 5s uniform timeout: elapsed=%s", elapsed)
	}
}

// TestBuildNodeBody_PerClassTimeout_PerClassEnvOverride asserts
// that COMPONENT_EXEC_TIMEOUT_<CLASS> wins over the table.
// ExeSQL's table entry is 3s; setting the per-class env to 7
// should make the body's deadline ~7s, not 3s.
func TestBuildNodeBody_PerClassTimeout_PerClassEnvOverride(t *testing.T) {
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "")
	t.Setenv("COMPONENT_EXEC_TIMEOUT_EXESQL", "7")

	cap := &captureCtxComponent{name: "ExeSQL"}
	origFactory := runtime.DefaultFactory()
	runtime.ResetDefaultFactoryForTesting()
	runtime.SetDefaultFactory(func(class string, _ map[string]any) (runtime.Component, error) {
		if class == "ExeSQL" {
			return cap, nil
		}
		return nil, errors.New("not used in this test: " + class)
	})
	t.Cleanup(func() {
		runtime.ResetDefaultFactoryForTesting()
		if origFactory != nil {
			runtime.SetDefaultFactory(origFactory)
		}
	})

	body, err := buildNodeBody("cpn-exe-ovr", "ExeSQL", nil)
	if err != nil {
		t.Fatalf("buildNodeBody: %v", err)
	}
	start := time.Now()
	_, err = body(context.Background(), nil)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("body err = %v, want context.DeadlineExceeded", err)
	}
	deadline := time.Unix(0, cap.deadline.Load())
	sinceDeadline := deadline.Sub(start)
	if sinceDeadline < 6500*time.Millisecond || sinceDeadline > 7500*time.Millisecond {
		t.Errorf("ExeSQL deadline offset = %s, want ~7s (per-class env override; elapsed %s)", sinceDeadline, elapsed)
	}
	if elapsed > 9*time.Second {
		t.Errorf("body did not honour 7s per-class env override: elapsed=%s", elapsed)
	}
}
