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
