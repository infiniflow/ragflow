//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// labelsMatch reports whether the metric carries a label named "runtime"
// with the supplied value.
func labelsMatch(pairs []*dto.LabelPair, wantRuntime string) bool {
	for _, p := range pairs {
		if p.GetName() == "runtime" && p.GetValue() == wantRuntime {
			return true
		}
	}
	return false
}

func TestObserveRun_IncrementsCounter(t *testing.T) {
	// ResetMetricsForTesting re-registers the metrics on the default
	// registry so testutil can read them.
	ResetMetricsForTesting()

	ObserveRun(RuntimeGo, OutcomeSuccess, 250*time.Millisecond)
	ObserveRun(RuntimeGo, OutcomeSuccess, 500*time.Millisecond)
	ObserveRun(RuntimePython, OutcomeError, 750*time.Millisecond)

	if got := testutil.ToFloat64(canvasRunsTotal.WithLabelValues("go", "success")); got != 2 {
		t.Errorf("go/success counter = %v, want 2", got)
	}
	if got := testutil.ToFloat64(canvasRunsTotal.WithLabelValues("python", "error")); got != 1 {
		t.Errorf("python/error counter = %v, want 1", got)
	}
	if got := testutil.ToFloat64(canvasRunsTotal.WithLabelValues("python", "success")); got != 0 {
		t.Errorf("python/success counter = %v, want 0", got)
	}
}

func TestObserveRun_RecordsDuration(t *testing.T) {
	ResetMetricsForTesting()

	ObserveRun(RuntimeGo, OutcomeSuccess, time.Second)
	ObserveRun(RuntimeGo, OutcomeSuccess, 2*time.Second)

	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var found bool
	for _, mf := range gathered {
		if !strings.Contains(mf.GetName(), "canvas_run_duration_seconds") {
			continue
		}
		for _, m := range mf.Metric {
			if labelsMatch(m.Label, "go") {
				found = true
				if m.Histogram == nil || m.Histogram.GetSampleCount() != 2 {
					t.Errorf("go histogram count = %v, want 2", m.Histogram.GetSampleCount())
				}
			}
		}
	}
	if !found {
		t.Fatal("did not find canvas_run_duration_seconds metric for runtime=go")
	}
}

func TestObserveRun_NormalisesEmptyArgs(t *testing.T) {
	ResetMetricsForTesting()

	// Pin the env-driven default to Python so the test is hermetic
	// regardless of the host environment. The assertion is "the
	// empty-runtime label equals Default()", so we set the env to
	// make the expected value explicit.
	t.Setenv("RAGFLOW_CANVAS_DEFAULT_RUNTIME", string(RuntimePython))
	ResetDefaultCache()

	ObserveRun("", "", 0) // all empty — should default to python/error and not observe histogram

	if got := testutil.ToFloat64(canvasRunsTotal.WithLabelValues("python", "error")); got != 1 {
		t.Errorf("defaulted counter = %v, want 1", got)
	}
}

// TestObserveRun_DefaultFallsToGoAfterPhase7 documents the
// empty-runtime fallback: the metric's fallback now follows
// selector.Default() (which is RuntimeGo), so the runtime label
// is consistent with what Selector.Select would have returned
// for the same tenant.
func TestObserveRun_DefaultFallsToGoAfterPhase7(t *testing.T) {
	ResetMetricsForTesting()
	t.Setenv("RAGFLOW_CANVAS_DEFAULT_RUNTIME", "")
	ResetDefaultCache()

	ObserveRun("", OutcomeError, 0)

	if got := testutil.ToFloat64(canvasRunsTotal.WithLabelValues(string(Default()), "error")); got != 1 {
		t.Errorf("defaulted counter = %v, want 1 for runtime=%s outcome=error", got, Default())
	}
}
