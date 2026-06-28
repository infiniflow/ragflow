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

package runtime

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// outcomeSuccess / outcomeError / outcomeCancelled are the only outcome
// label values the canvas-run metric emits. Keeping the set closed lets
// downstream alerts reason about the cardinality.
const (
	OutcomeSuccess   = "success"
	OutcomeError     = "error"
	OutcomeCancelled = "cancelled"
)

var (
	canvasRunsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ragflow_canvas_runs_total",
			Help: "Total canvas runs by runtime mode and outcome.",
		},
		[]string{"runtime", "outcome"},
	)

	canvasRunDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ragflow_canvas_run_duration_seconds",
			Help:    "Canvas run latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"runtime"},
	)

	registerOnce sync.Once
)

// init registers the package's metrics with the default Prometheus
// registry. The sync.Once guard means tests that re-import this package
// (or any future entry point that also imports it) won't trigger
// "duplicate metrics collector registration" panics. If a test wants a
// clean registry, call ResetMetrics().
func init() {
	registerOnce.Do(func() {
		prometheus.MustRegister(canvasRunsTotal, canvasRunDuration)
	})
}

// ResetMetricsForTesting unregisters the package metrics from the default
// Prometheus registry, clears any recorded samples, and re-registers them
// so the next ObserveRun call sees a clean slate. Intended for unit tests
// that need to assert on freshly-registered metrics.
func ResetMetricsForTesting() {
	prometheus.DefaultRegisterer.Unregister(canvasRunsTotal)
	prometheus.DefaultRegisterer.Unregister(canvasRunDuration)
	canvasRunsTotal.Reset()
	canvasRunDuration.Reset()
	registerOnce = sync.Once{}
	registerOnce.Do(func() {
		prometheus.MustRegister(canvasRunsTotal, canvasRunDuration)
	})
}

// ObserveRun emits a counter + histogram observation for one canvas run.
// The runtime label is the string form of RuntimeMode; the outcome label
// must be one of the Outcome* constants. Negative or zero durations are
// dropped from the histogram to avoid skewing percentile math.
//
// An empty runtime defaults to the process-wide Default() so the metric
// tag matches what Selector.Select would have returned for the same
// tenant (review follow-up M4). Callers that need to record the
// Python-routed-fallback case explicitly should pass RuntimePython.
func ObserveRun(runtime RuntimeMode, outcome string, duration time.Duration) {
	if runtime == "" {
		runtime = Default()
	}
	if outcome == "" {
		outcome = OutcomeError
	}
	canvasRunsTotal.WithLabelValues(string(runtime), outcome).Inc()
	if duration > 0 {
		canvasRunDuration.WithLabelValues(string(runtime)).Observe(duration.Seconds())
	}
}
