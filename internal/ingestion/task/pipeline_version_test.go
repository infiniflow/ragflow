package task

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// setupVersionTestDB stands up an in-memory sqlite with the canvas and version
// tables, swaps it into dao.DB, and returns a teardown func.
func setupVersionTestDB(t *testing.T) func() {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}, &entity.Document{}, &entity.Knowledgebase{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	return func() { dao.DB = orig }
}

func seedCanvas(t *testing.T, db *gorm.DB, canvasID string) {
	t.Helper()
	canvas := &entity.UserCanvas{
		ID:             canvasID,
		CanvasCategory: "dataflow_canvas",
		Title:          strPtr("flow"),
		DSL:            entity.JSONMap{"components": []any{}},
	}
	if err := db.Create(canvas).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
}

func seedVersion(t *testing.T, db *gorm.DB, canvasID, versionID string, dsl entity.JSONMap, createMs ...int64) {
	t.Helper()
	v := &entity.UserCanvasVersion{
		ID:           versionID,
		UserCanvasID: canvasID,
		DSL:          dsl,
	}
	// An explicit createMs makes GetLatest ordering deterministic; without it the
	// two rows can share the same millisecond timestamp and the latest-by-time
	// order is undefined (flaky test).
	if len(createMs) > 0 {
		ct := createMs[0]
		v.CreateTime = &ct
	}
	if err := db.Create(v).Error; err != nil {
		t.Fatalf("seed version: %v", err)
	}
}

// newVersionTaskCtx builds a TaskContext that satisfies executor validation
// and carries the given pinned version id (empty = latest, design 6.2).
func newVersionTaskCtx(pin string) *TaskContext {
	return &TaskContext{
		Doc: entity.Document{
			ID:   "doc-1",
			KbID: "kb-1",
			Name: strPtr("doc.pdf"),
		},
		KB:                entity.Knowledgebase{ID: "kb-1"},
		Tenant:            entity.Tenant{ID: "tenant-1"},
		PipelineVersionID: pin,
	}
}

// T11: resolvePipelineVersionID maps a document pin to its trimmed version id,
// and to "" when no pin is set (runtime falls back to latest, design 6.2).
func TestResolvePipelineVersionID(t *testing.T) {
	if got := resolvePipelineVersionID(nil); got != "" {
		t.Fatalf("nil doc = %q, want empty", got)
	}
	if got := resolvePipelineVersionID(&entity.Document{}); got != "" {
		t.Fatalf("no pin = %q, want empty", got)
	}
	if got := resolvePipelineVersionID(&entity.Document{PipelineVersionID: strPtr("  v1  ")}); got != "v1" {
		t.Fatalf("pin = %q, want trimmed v1", got)
	}
}

// T10: loadDSLFromCanvas reads a dataflow canvas version by its pinned id.
func TestLoadDSLFromCanvas_VersionedReadsVersionDSL(t *testing.T) {
	teardown := setupVersionTestDB(t)
	defer teardown()
	db := dao.DB
	pinnedDSL := entity.JSONMap{"components": []any{map[string]any{"id": "a"}}}
	seedCanvas(t, db, "flow-1")
	seedVersion(t, db, "flow-1", "v1", pinnedDSL)

	taskCtx := newVersionTaskCtx("v1")
	exec := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0)
	dsl, correctedID, err := exec.loadDSLFromCanvas(context.Background(), "flow-1")
	if err != nil {
		t.Fatalf("loadDSLFromCanvas: %v", err)
	}
	if correctedID != "flow-1" {
		t.Fatalf("correctedID = %q", correctedID)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(dsl), &got); err != nil {
		t.Fatalf("unmarshal dsl: %v", err)
	}
	comps, ok := got["components"].([]any)
	if !ok || len(comps) != 1 {
		t.Fatalf("dsl components = %#v", got["components"])
	}
}

// TestErrPipelineVersionMissingSingleSentinel proves the task-layer sentinel is
// the single dao definition, not a second errors.New with the same message. This
// locks in the review fix that merged the service/task duplicate sentinels into
// one (design §6.2): a runtime Missing error must match a dao.ErrPipelineVersionMissing
// check, and must equal the service-layer sentinel identity.
func TestErrPipelineVersionMissingSingleSentinel(t *testing.T) {
	if !errors.Is(ErrPipelineVersionMissing, dao.ErrPipelineVersionMissing) {
		t.Fatalf("task.ErrPipelineVersionMissing must be dao.ErrPipelineVersionMissing, got distinct identity")
	}
}

// T12: a pinned-but-missing version (and a mis-owned one) fails with
// ErrPipelineVersionMissing (C4, design 6.2) carrying the user-facing hint.
func TestLoadDSLFromCanvas_PinnedMissingVersionFails(t *testing.T) {
	teardown := setupVersionTestDB(t)
	defer teardown()
	db := dao.DB
	seedCanvas(t, db, "flow-1")
	seedVersion(t, db, "flow-1", "v1", entity.JSONMap{"components": []any{}})
	seedCanvas(t, db, "flow-2")
	seedVersion(t, db, "flow-2", "foreign", entity.JSONMap{"components": []any{}})

	cases := []struct {
		name    string
		version string
	}{
		{"missing", "nope"},
		{"foreign", "foreign"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			taskCtx := newVersionTaskCtx(c.version)
			exec := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0)
			_, _, err := exec.loadDSLFromCanvas(context.Background(), "flow-1")
			if err == nil {
				t.Fatalf("want error for version %q", c.version)
			}
			if !errors.Is(err, ErrPipelineVersionMissing) {
				t.Fatalf("err = %v, want ErrPipelineVersionMissing", err)
			}
			if !strings.Contains(err.Error(), "pipeline version does not exist. Please select a version again") {
				t.Fatalf("err message missing user hint: %v", err)
			}
		})
	}
}

// T12b: no pin falls back to the latest version of the bound canvas.
func TestLoadDSLFromCanvas_NoPinUsesLatestVersion(t *testing.T) {
	teardown := setupVersionTestDB(t)
	defer teardown()
	db := dao.DB
	seedCanvas(t, db, "flow-1")
	seedVersion(t, db, "flow-1", "v-old", entity.JSONMap{"components": []any{map[string]any{"id": "old"}}}, 100)
	seedVersion(t, db, "flow-1", "v-new", entity.JSONMap{"components": []any{map[string]any{"id": "new"}}}, 200)

	taskCtx := newVersionTaskCtx("")
	exec := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0)
	dsl, _, err := exec.loadDSLFromCanvas(context.Background(), "flow-1")
	if err != nil {
		t.Fatalf("loadDSLFromCanvas: %v", err)
	}
	if !strings.Contains(dsl, "\"id\":\"new\"") {
		t.Fatalf("expected latest version DSL, got %s", dsl)
	}
}
