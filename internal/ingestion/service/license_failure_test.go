package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/compose"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	servicepkg "ragflow/internal/service"
	"ragflow/internal/utility"
)

func TestRunTaskLicenseFailure(t *testing.T) {
	graph := compose.NewGraph[string, string]()
	if err := graph.AddLambdaNode("Parser", compose.InvokableLambda(func(context.Context, string) (string, error) {
		return "", fmt.Errorf("parser: visio: %w", utility.ErrFeatureDisabled)
	})); err != nil {
		t.Fatal(err)
	}
	if err := graph.AddEdge(compose.START, "Parser"); err != nil {
		t.Fatal(err)
	}
	if err := graph.AddEdge("Parser", compose.END); err != nil {
		t.Fatal(err)
	}
	runner, err := graph.Compile(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, nodeErr := runner.Invoke(t.Context(), "")
	if nodeErr == nil || !strings.Contains(nodeErr.Error(), "[NodeRunError]") {
		t.Fatalf("expected real Eino node failure, got %v", nodeErr)
	}
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"wrapped license failure", fmt.Errorf("pipeline: run canvas workflow: %w", nodeErr), "license_required"},
		{"ordinary failure", errors.New("invalid document"), ""},
		{"matching text without sentinel", errors.New(utility.ErrFeatureDisabled.Error()), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			cleanup := testutil.ReplaceDBForTest(t, db)
			defer cleanup()
			_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))
			ingestor := newUnitIngestor("test", 1)
			ingestor.runDocumentTask = func(context.Context, *entity.IngestionTask) error { return tc.err }
			runID := "run-" + taskID
			if !ingestor.runTask(t.Context(), &entity.IngestionTask{
				ID: taskID, DocumentID: docID, DatasetID: "kb-1",
				Status: common.RUNNING, PipelineLogID: &runID,
			}) {
				t.Fatal("failure was not durably recorded")
			}
			task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
			if err != nil || task.Status != common.FAILED {
				t.Fatalf("task = %+v, err = %v, want FAILED", task, err)
			}
			logs, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(t.Context(), db, runID)
			if err != nil || len(logs) != 1 {
				t.Fatalf("terminal logs = %+v, err = %v", logs, err)
			}
			event := servicepkg.IngestionEventItemFromLog(logs[0])
			if event.ErrorCode != tc.code || event.Message != fmt.Sprintf("Task failed: %v", tc.err) {
				t.Fatalf("event = %+v, want code %q with original failure detail", event, tc.code)
			}
		})
	}
}
