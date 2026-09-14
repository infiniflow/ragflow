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

// Tests for MetadataService.GetMetaValueSpaceByKBs.
//
// The value space is what the metadata filter generator picks a filter from,
// and that filter is applied as a document scope before scoring -- so a value
// the model was never shown cannot be chosen.
//
// GetFlattedMetaByKBs cannot supply it on a large dataset: it reads the
// doc-meta index with a fixed size cap, so everything past that cap is simply
// absent. The stub engines here make the difference observable -- the scan is
// capped, the aggregation is not -- and the incomplete case proves the scan is
// never silently substituted for a read that failed halfway.

package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/tokenizer"
)

// cappedScanDocEngine answers SearchMetadata with at most req.Limit records,
// the way a doc store bounded by its result window does.
type cappedScanDocEngine struct {
	fakeChatDocEngine
	records  []map[string]interface{}
	searches int
}

func (e *cappedScanDocEngine) SearchMetadata(_ context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	e.searches++
	records := e.records
	if req.Limit > 0 && len(records) > req.Limit {
		records = records[:req.Limit]
	}
	return &types.SearchMetadataResult{MetadataRecords: records, Total: int64(len(e.records))}, nil
}

// aggregatingDocEngine additionally implements the aggregation path.
type aggregatingDocEngine struct {
	cappedScanDocEngine
	space map[string][]string
	err   error

	gotTenantID string
	gotKBIDs    []string
}

func (e *aggregatingDocEngine) MetaValueSpace(_ context.Context, tenantID string, kbIDs []string) (map[string][]string, error) {
	e.gotTenantID = tenantID
	e.gotKBIDs = kbIDs
	if e.err != nil {
		return nil, e.err
	}
	return e.space, nil
}

func seedValueSpaceKB(t *testing.T) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.Create(&entity.Knowledgebase{
		ID:        "kb-1",
		TenantID:  "tenant-1",
		Name:      "kb-1",
		CreatedBy: "tenant-1",
		EmbdID:    "embd",
	}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
}

// The aggregation is the source of the value space when the engine offers it,
// and the capped scan is not consulted at all.
func TestGetMetaValueSpaceByKBs_UsesAggregation(t *testing.T) {
	seedValueSpaceKB(t)
	docEngine := &aggregatingDocEngine{space: map[string][]string{
		"project": {"alpha", "beta"},
		"phase":   {"draft"},
	}}
	svc := NewMetadataServiceForTest(nil, docEngine)

	space, err := svc.GetMetaValueSpaceByKBs(t.Context(), []string{"kb-1"})
	if err != nil {
		t.Fatalf("GetMetaValueSpaceByKBs: %v", err)
	}
	want := common.MetaValueSpace{"project": {"alpha", "beta"}, "phase": {"draft"}}
	if !reflect.DeepEqual(space, want) {
		t.Errorf("space: got %v, want %v", space, want)
	}
	if docEngine.searches != 0 {
		t.Errorf("the capped scan ran %d times; the aggregation replaces it", docEngine.searches)
	}
	if docEngine.gotTenantID != "tenant-1" {
		t.Errorf("tenant id: got %q, want tenant-1", docEngine.gotTenantID)
	}
}

// A value carried only by documents past the scan's cap is invisible to the
// flattened scan and present in the aggregated space. Without that difference
// the aggregation would be pointless, so assert both halves.
func TestGetMetaValueSpaceByKBs_AggregationSeesPastTheScanCap(t *testing.T) {
	const scanCap = 10000
	records := make([]map[string]interface{}, 0, scanCap+1)
	for i := range scanCap {
		records = append(records, map[string]interface{}{
			"id":          fmt.Sprintf("doc-%d", i),
			"meta_fields": map[string]interface{}{"phase": "early"},
		})
	}
	records = append(records, map[string]interface{}{
		"id":          "doc-late",
		"meta_fields": map[string]interface{}{"phase": "late-phase"},
	})

	seedValueSpaceKB(t)
	scanOnly := &cappedScanDocEngine{records: records}
	scanned, err := NewMetadataServiceForTest(nil, scanOnly).GetMetaValueSpaceByKBs(t.Context(), []string{"kb-1"})
	if err != nil {
		t.Fatalf("scan value space: %v", err)
	}
	if got := scanned["phase"]; !reflect.DeepEqual(got, []string{"early"}) {
		t.Fatalf("the capped scan returned %v; the fixture must hide late-phase past the cap", got)
	}

	aggregating := &aggregatingDocEngine{
		cappedScanDocEngine: cappedScanDocEngine{records: records},
		space:               map[string][]string{"phase": {"early", "late-phase"}},
	}
	space, err := NewMetadataServiceForTest(nil, aggregating).GetMetaValueSpaceByKBs(t.Context(), []string{"kb-1"})
	if err != nil {
		t.Fatalf("aggregated value space: %v", err)
	}
	if got := space["phase"]; !reflect.DeepEqual(got, []string{"early", "late-phase"}) {
		t.Errorf("aggregated phase values: got %v, want [early late-phase]", got)
	}
}

// Backends without an aggregation path keep the flattened scan, unchanged.
func TestGetMetaValueSpaceByKBs_FallsBackWithoutAggregation(t *testing.T) {
	seedValueSpaceKB(t)
	docEngine := &cappedScanDocEngine{records: []map[string]interface{}{
		{"id": "doc-1", "meta_fields": map[string]interface{}{"project": "alpha"}},
		{"id": "doc-2", "meta_fields": map[string]interface{}{"project": "beta"}},
	}}
	svc := NewMetadataServiceForTest(nil, docEngine)

	space, err := svc.GetMetaValueSpaceByKBs(t.Context(), []string{"kb-1"})
	if err != nil {
		t.Fatalf("GetMetaValueSpaceByKBs: %v", err)
	}
	want := common.MetaValueSpace{"project": {"alpha", "beta"}}
	if !reflect.DeepEqual(space, want) {
		t.Errorf("space: got %v, want %v", space, want)
	}
	if docEngine.searches != 1 {
		t.Errorf("scan searches: got %d, want 1", docEngine.searches)
	}
}

// An incomplete read must surface as ErrMetaValueSpaceIncomplete and must NOT
// be answered with the capped scan: that would replace one incomplete value
// space with a differently incomplete one.
func TestGetMetaValueSpaceByKBs_PropagatesIncomplete(t *testing.T) {
	seedValueSpaceKB(t)
	docEngine := &aggregatingDocEngine{
		cappedScanDocEngine: cappedScanDocEngine{records: []map[string]interface{}{
			{"id": "doc-1", "meta_fields": map[string]interface{}{"project": "alpha"}},
		}},
		err: fmt.Errorf("%w: failed shard", types.ErrMetaValueSpaceIncomplete),
	}
	svc := NewMetadataServiceForTest(nil, docEngine)

	space, err := svc.GetMetaValueSpaceByKBs(t.Context(), []string{"kb-1"})
	if !errors.Is(err, types.ErrMetaValueSpaceIncomplete) {
		t.Fatalf("error: got %v, want ErrMetaValueSpaceIncomplete", err)
	}
	if space != nil {
		t.Errorf("space: got %v, want nil", space)
	}
	if docEngine.searches != 0 {
		t.Errorf("the capped scan ran %d times; an incomplete read must not be softened into it", docEngine.searches)
	}
}

func TestGetMetaValueSpaceByKBs_NoKBs(t *testing.T) {
	docEngine := &aggregatingDocEngine{space: map[string][]string{"project": {"alpha"}}}
	svc := NewMetadataServiceForTest(nil, docEngine)

	space, err := svc.GetMetaValueSpaceByKBs(t.Context(), nil)
	if err != nil {
		t.Fatalf("GetMetaValueSpaceByKBs: %v", err)
	}
	if len(space) != 0 {
		t.Errorf("space: got %v, want empty", space)
	}
	if docEngine.gotKBIDs != nil {
		t.Errorf("the engine was called with %v; no kb ids means no lookup", docEngine.gotKBIDs)
	}
}

// ValueSpace keeps the distinct values per key and drops the doc ids, which the
// filter generator never reads.
func TestMetaDataValueSpace(t *testing.T) {
	metas := common.MetaData{
		"project": {"beta": {"doc-2"}, "alpha": {"doc-1", "doc-3"}},
		"phase":   {"draft": {"doc-1"}},
	}
	want := common.MetaValueSpace{"project": {"alpha", "beta"}, "phase": {"draft"}}
	if got := metas.ValueSpace(); !reflect.DeepEqual(got, want) {
		t.Errorf("ValueSpace: got %v, want %v", got, want)
	}
}

// capturingFilterDriver records the system prompt gen_meta_filter sends and
// answers with an empty filter, so a test can assert what the model was shown.
type capturingFilterDriver struct {
	*modelModule.DummyModel
	calls  int
	system string
}

func (d *capturingFilterDriver) ChatWithMessages(
	_ context.Context,
	_ string,
	messages []modelModule.Message,
	_ *modelModule.APIConfig,
	_ *modelModule.ChatConfig,
	_ *common.ModelUsage,
) (*modelModule.ChatResponse, error) {
	d.calls++
	if len(messages) > 0 {
		if s, ok := messages[0].Content.(string); ok {
			d.system = s
		}
	}
	answer := `{"conditions": [], "logic": "and"}`
	return &modelModule.ChatResponse{Answer: &answer}, nil
}

func newCapturingFilterModel(t *testing.T) (*modelModule.ChatModel, *capturingFilterDriver) {
	t.Helper()
	driver := &capturingFilterDriver{DummyModel: modelModule.NewDummyModel(nil, modelModule.URLSuffix{})}
	name := "fake-model"
	return &modelModule.ChatModel{ModelDriver: driver, ModelName: &name, APIConfig: &modelModule.APIConfig{}}, driver
}

// stubValueSpaceLoader swaps the value-space loader for one test.
func stubValueSpaceLoader(t *testing.T, space common.MetaValueSpace, err error) {
	t.Helper()
	orig := metaValueSpaceLoader
	metaValueSpaceLoader = func(context.Context, []string) (common.MetaValueSpace, error) {
		return space, err
	}
	t.Cleanup(func() { metaValueSpaceLoader = orig })
}

// The aggregated value space, not the capped scan, is what the model is shown.
func TestApplyMetaDataFilter_ShowsTheAggregatedValueSpace(t *testing.T) {
	stubValueSpaceLoader(t, common.MetaValueSpace{"phase": {"early", "late-phase"}}, nil)
	chatModel, driver := newCapturingFilterModel(t)
	// What the capped scan saw: the late value is missing from it.
	metas := common.MetaData{"phase": {"early": {"doc-1"}}}

	ApplyMetaDataFilter(t.Context(), map[string]interface{}{"method": "auto"}, metas, "which phase?", chatModel, nil, []string{"kb-1"})

	if driver.calls != 1 {
		t.Fatalf("model calls: got %d, want 1", driver.calls)
	}
	if !strings.Contains(driver.system, "late-phase") {
		t.Errorf("the prompt was built from the capped scan, not the value space: %q", driver.system)
	}
}

// A value space that could not be read in full leaves the search unscoped: no
// filter is generated, and the model is never asked to pick from a partial list.
func TestApplyMetaDataFilter_SkipsFilteringOnIncompleteValueSpace(t *testing.T) {
	for _, method := range []string{"auto", "semi_auto"} {
		t.Run(method, func(t *testing.T) {
			stubValueSpaceLoader(t, nil, fmt.Errorf("%w: failed shard", types.ErrMetaValueSpaceIncomplete))
			chatModel, driver := newCapturingFilterModel(t)
			metas := common.MetaData{"phase": {"early": {"doc-1"}}}
			filter := map[string]interface{}{"method": method}
			if method == "semi_auto" {
				filter["semi_auto"] = []interface{}{"phase"}
			}

			docIDs, noMatches := ApplyMetaDataFilter(t.Context(), filter, metas, "which phase?", chatModel, []string{"doc-1"}, []string{"kb-1"})

			if driver.calls != 0 {
				t.Errorf("the model was asked to pick from a partial value space (%d calls)", driver.calls)
			}
			if docIDs != nil || !noMatches {
				t.Errorf("got (%v, %v), want (nil, true) so retrieval answers from the whole corpus", docIDs, noMatches)
			}
		})
	}
}

// When the value space cannot be read at all -- as opposed to incompletely --
// the flattened metadata still feeds the prompt, so the filter keeps working.
func TestApplyMetaDataFilter_FallsBackToFlattenedMetaOnLookupError(t *testing.T) {
	stubValueSpaceLoader(t, nil, errors.New("doc store unreachable"))
	chatModel, driver := newCapturingFilterModel(t)
	metas := common.MetaData{"phase": {"early": {"doc-1"}}}

	ApplyMetaDataFilter(t.Context(), map[string]interface{}{"method": "auto"}, metas, "which phase?", chatModel, nil, []string{"kb-1"})

	if driver.calls != 1 {
		t.Fatalf("model calls: got %d, want 1", driver.calls)
	}
	if !strings.Contains(driver.system, "early") {
		t.Errorf("the flattened metadata did not reach the prompt: %q", driver.system)
	}
}

// bigValueSpace returns a value space of n distinct values under one key, which
// is how a real high-cardinality metadata key renders into the prompt.
func bigValueSpace(n int) common.MetaValueSpace {
	values := make([]string, 0, n)
	for i := range n {
		values = append(values, fmt.Sprintf("value-%06d", i))
	}
	return common.MetaValueSpace{"project": values}
}

// requireTokenCounts skips when the cl100k BPE table is absent: without it
// NumTokensFromString returns 0, every prompt "fits", and a budget assertion
// would be meaningless rather than wrong. The table is a local asset fetched by
// ragflow_deps/download_deps.py, not an external service.
func requireTokenCounts(t *testing.T) {
	t.Helper()
	if tokenizer.NumTokensFromString("hello world") == 0 {
		t.Skip("cl100k BPE table missing (run `uv run ragflow_deps/download_deps.py`); token counts are 0")
	}
}

// The value space is the whole dataset's, so it can exceed the model's context
// on its own. A filter picked from a value list that lost entries is applied as
// a hard document scope, and the model cannot report that it only saw part of
// the metadata -- so no conditions at all, and no model call.
func TestGenMetaFilter_RefusesAnOversizedValueSpace(t *testing.T) {
	requireTokenCounts(t)
	chatModel, driver := newCapturingFilterModel(t)
	chatModel.ContextLength = 256

	result, err := GenMetaFilter(t.Context(), chatModel, bigValueSpace(2000), "which project?", nil)
	if err != nil {
		t.Fatalf("GenMetaFilter: %v", err)
	}
	if len(result.Conditions) != 0 || result.Logic != "and" {
		t.Errorf("result: got %+v, want no conditions with logic and", result)
	}
	if driver.calls != 0 {
		t.Errorf("the model was called %d times with a prompt that does not fit its context", driver.calls)
	}
}

// A value space that fits is sent untouched.
func TestGenMetaFilter_SendsAValueSpaceThatFits(t *testing.T) {
	requireTokenCounts(t)
	chatModel, driver := newCapturingFilterModel(t)
	chatModel.ContextLength = 128000

	if _, err := GenMetaFilter(t.Context(), chatModel, common.MetaValueSpace{"project": {"alpha", "beta"}}, "which project?", nil); err != nil {
		t.Fatalf("GenMetaFilter: %v", err)
	}
	if driver.calls != 1 {
		t.Fatalf("model calls: got %d, want 1", driver.calls)
	}
	if !strings.Contains(driver.system, "alpha") {
		t.Errorf("the value space did not reach the prompt: %q", driver.system)
	}
}

// A context window that could not be resolved is 0, which must mean 8192 --
// Python's message_fit_in normalizes a non-positive max_length the same way --
// and NOT "no budget at all".
func TestGenMetaFilter_UnresolvedContextLengthUsesTheDefaultBudget(t *testing.T) {
	requireTokenCounts(t)

	t.Run("under the default budget the prompt is sent", func(t *testing.T) {
		chatModel, driver := newCapturingFilterModel(t)
		chatModel.ContextLength = 0

		if _, err := GenMetaFilter(t.Context(), chatModel, common.MetaValueSpace{"project": {"alpha"}}, "which project?", nil); err != nil {
			t.Fatalf("GenMetaFilter: %v", err)
		}
		if driver.calls != 1 {
			t.Errorf("model calls: got %d, want 1", driver.calls)
		}
	})

	t.Run("over the default budget the prompt is refused", func(t *testing.T) {
		chatModel, driver := newCapturingFilterModel(t)
		chatModel.ContextLength = 0
		space := bigValueSpace(4000)
		if got := tokenizer.NumTokensFromString(fmt.Sprint(space)); got <= 8192 {
			t.Fatalf("fixture is only %d tokens; it must exceed the 8192 default to test the budget", got)
		}

		result, err := GenMetaFilter(t.Context(), chatModel, space, "which project?", nil)
		if err != nil {
			t.Fatalf("GenMetaFilter: %v", err)
		}
		if len(result.Conditions) != 0 {
			t.Errorf("conditions: got %+v, want none", result.Conditions)
		}
		if driver.calls != 0 {
			t.Errorf("the model was called %d times past the default budget", driver.calls)
		}
	})
}

// End to end at the caller: an oversized value space produces the same
// whole-corpus signal as a value space that could not be read in full.
func TestApplyMetaDataFilter_OversizedValueSpaceLeavesTheSearchUnscoped(t *testing.T) {
	requireTokenCounts(t)
	for _, method := range []string{"auto", "semi_auto"} {
		t.Run(method, func(t *testing.T) {
			stubValueSpaceLoader(t, bigValueSpace(2000), nil)
			chatModel, driver := newCapturingFilterModel(t)
			chatModel.ContextLength = 256
			filter := map[string]interface{}{"method": method}
			if method == "semi_auto" {
				filter["semi_auto"] = []interface{}{"project"}
			}

			docIDs, noMatches := ApplyMetaDataFilter(t.Context(), filter, common.MetaData{}, "which project?", chatModel, []string{"doc-1"}, []string{"kb-1"})

			if driver.calls != 0 {
				t.Errorf("the model was asked to pick from a prompt that does not fit (%d calls)", driver.calls)
			}
			if docIDs != nil || !noMatches {
				t.Errorf("got (%v, %v), want (nil, true) so retrieval answers from the whole corpus", docIDs, noMatches)
			}
		})
	}
}
