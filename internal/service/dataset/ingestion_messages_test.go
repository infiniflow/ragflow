package dataset

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"gorm.io/gorm"
)

func TestListIngestionMessagesUsesRunScopedKeysets(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	insertMessageRun(t, "run-1", "kb-1", "doc-1", entity.TaskStatusRunning, 2)
	for i := 0; i < 5; i++ {
		insertMessageEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, "detail")
	}

	response, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "run-1", 2, nil, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionMessages = (%+v, %v, %v), want success", response, code, err)
	}
	if response.RunCount != 2 || response.OldestID != 4 || response.NewestID != 5 || !response.HasMoreBefore || response.HasMoreAfter || response.Terminal {
		t.Fatalf("first response = %+v, want newest page for active run", response)
	}
	assertMessageEventIDs(t, response.Items, 4, 5)

	afterID := 2
	response, code, err = NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "run-1", 2, &afterID, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("after page = (%+v, %v, %v), want success", response, code, err)
	}
	assertMessageEventIDs(t, response.Items, 3, 4)
	if !response.HasMoreBefore || !response.HasMoreAfter {
		t.Fatalf("after response flags = before:%t after:%t, want true/true", response.HasMoreBefore, response.HasMoreAfter)
	}

	beforeID := 4
	if _, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "run-1", 2, &afterID, &beforeID); err == nil || code != common.CodeArgumentError {
		t.Fatalf("mutually exclusive cursors = code:%v err:%v, want argument error", code, err)
	}
}

func TestListIngestionMessagesRejectsUnnumberedRun(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	insertMessageRun(t, "legacy-run", "kb-1", "doc-1", entity.TaskStatusRunning, 0)
	insertMessageEvent(t, db, "legacy-run", "task-1", dao.EventTypeMessage, "old detail")

	response, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "legacy-run", 200, nil, nil)
	if response != nil || err == nil || code != common.CodeDataError {
		t.Fatalf("ListIngestionMessages = (%+v, %v, %v), want not-found data error", response, code, err)
	}
}

func TestDatasetIngestionLogUsesEventStream(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	if err := db.Create(&entity.PipelineOperationLog{
		ID:              "dataset-run",
		DocumentID:      entity.DatasetLogDocumentID,
		TenantID:        "user-1",
		KbID:            "kb-1",
		ParserID:        "knowledge_compile",
		DocumentName:    "Wiki",
		DocumentType:    "dataset",
		SourceFrom:      "knowledgebase",
		TaskType:        "Wiki",
		OperationStatus: "DONE",
	}).Error; err != nil {
		t.Fatalf("insert dataset run: %v", err)
	}
	insertMessageRun(t, "numbered-sentinel-run", "kb-1", entity.DatasetLogDocumentID, entity.TaskStatusDone, 1)
	insertMessageEvent(t, db, "dataset-run", "dataset-run", dao.EventTypeTerminal, "Knowledge compilation completed")

	result, code, err := NewDatasetService().ListIngestionLogs(t.Context(), "kb-1", "user-1", 1, 30, nil, nil, "", "", "dataset", "", "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionLogs = (%+v, %v, %v), want success", result, code, err)
	}
	logs, ok := result["logs"].([]map[string]interface{})
	if !ok || len(logs) != 1 || logs[0]["id"] != "dataset-run" {
		t.Fatalf("dataset logs = %#v, want only unnumbered dataset run", result["logs"])
	}
	assertLatestEventMap(t, logs[0], 1, "Knowledge compilation completed")

	messages, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "dataset-run", 200, nil, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionMessages = (%+v, %v, %v), want success", messages, code, err)
	}
	if messages.RunCount != 0 || !messages.Terminal || len(messages.Items) != 1 || messages.Items[0].Message != "Knowledge compilation completed" {
		t.Fatalf("dataset messages = %+v, want terminal event stream", messages)
	}

	log, code, err := NewDatasetService().GetIngestionLog(t.Context(), "kb-1", "user-1", "dataset-run")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("GetIngestionLog = (%+v, %v, %v), want success", log, code, err)
	}
	assertLatestEventMap(t, log, 1, "Knowledge compilation completed")
}

func TestListIngestionLogsIncludesPythonFileLogsAndStatusFilters(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	insertMessageRun(t, "run-1", "kb-1", "doc-1", entity.TaskStatusDone, 1)
	insertMessageRun(t, "zero-run", "kb-1", "doc-3", entity.TaskStatusDone, 0)
	progressMessage := "Python parse completed"
	if err := db.Create(&entity.PipelineOperationLog{
		ID: "python-run", DocumentID: "doc-2", TenantID: "user-1", KbID: "kb-1",
		ParserID: "naive", DocumentName: "doc.txt", DocumentSuffix: ".txt", DocumentType: "text",
		SourceFrom: "local", TaskType: string(entity.PipelineTaskTypeParse), OperationStatus: "3",
		ProgressMsg: &progressMessage,
	}).Error; err != nil {
		t.Fatalf("insert Python run: %v", err)
	}

	result, code, err := NewDatasetService().ListIngestionLogs(t.Context(), "kb-1", "user-1", 1, 30, nil, []string{"DONE"}, "", "", "file", "", "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionLogs = (%+v, %v, %v), want success", result, code, err)
	}
	logs, ok := result["logs"].([]map[string]interface{})
	if !ok || len(logs) != 2 || result["total"] != int64(2) {
		t.Fatalf("logs = %#v total=%#v, want numbered and Python runs only", result["logs"], result["total"])
	}
	byID := make(map[string]map[string]interface{}, len(logs))
	for _, log := range logs {
		byID[log["id"].(string)] = log
	}
	if byID["python-run"]["operation_status"] != "3" {
		t.Fatalf("Python operation_status = %#v, want stored status 3", byID["python-run"]["operation_status"])
	}
	if byID["python-run"]["progress_msg"] != progressMessage {
		t.Fatalf("Python progress_msg = %#v, want %q", byID["python-run"]["progress_msg"], progressMessage)
	}
	if _, ok := byID["python-run"]["parser_id"]; !ok {
		t.Fatalf("Python file log = %#v, want file-log fields", byID["python-run"])
	}

	detail, code, err := NewDatasetService().GetIngestionLog(t.Context(), "kb-1", "user-1", "python-run")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("GetIngestionLog = (%+v, %v, %v), want success", detail, code, err)
	}
	if detail["progress_msg"] != progressMessage {
		t.Fatalf("Python detail progress_msg = %#v, want %q", detail["progress_msg"], progressMessage)
	}
	if _, ok := detail["parser_id"]; !ok {
		t.Fatalf("Python detail = %#v, want file-log fields", detail)
	}

	messages, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "python-run", 200, nil, nil)
	if err != nil || code != common.CodeSuccess || !messages.Terminal || len(messages.Items) != 0 {
		t.Fatalf("Python ListIngestionMessages = (%+v, %v, %v), want empty terminal success", messages, code, err)
	}
}

func TestPythonDatasetLogIsReadableAndKeepsProgressMessage(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	progressMessage := "Knowledge compilation completed"
	if err := db.Create(&entity.PipelineOperationLog{
		ID: "python-dataset-run", DocumentID: entity.DatasetLogDocumentID, TenantID: "user-1", KbID: "kb-1",
		ParserID: "knowledge_compile", DocumentName: "Wiki", DocumentType: "dataset", SourceFrom: "knowledgebase",
		TaskType: "Wiki", OperationStatus: "3", ProgressMsg: &progressMessage,
	}).Error; err != nil {
		t.Fatalf("insert Python dataset run: %v", err)
	}

	result, code, err := NewDatasetService().ListIngestionLogs(t.Context(), "kb-1", "user-1", 1, 30, nil, []string{"DONE"}, "", "", "dataset", "", "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionLogs = (%+v, %v, %v), want success", result, code, err)
	}
	logs, ok := result["logs"].([]map[string]interface{})
	if !ok || len(logs) != 1 || logs[0]["id"] != "python-dataset-run" {
		t.Fatalf("dataset logs = %#v, want the Python log", result["logs"])
	}
	if logs[0]["operation_status"] != "3" || logs[0]["progress_msg"] != progressMessage {
		t.Fatalf("Python dataset log fields = %#v, want stored status and progress_msg", logs[0])
	}

	log, code, err := NewDatasetService().GetIngestionLog(t.Context(), "kb-1", "user-1", "python-dataset-run")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("GetIngestionLog = (%+v, %v, %v), want success", log, code, err)
	}
	if log["progress_msg"] != progressMessage {
		t.Fatalf("detail progress_msg = %#v, want %q", log["progress_msg"], progressMessage)
	}

	messages, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "python-dataset-run", 200, nil, nil)
	if err != nil || code != common.CodeSuccess || !messages.Terminal || len(messages.Items) != 0 {
		t.Fatalf("legacy ListIngestionMessages = (%+v, %v, %v), want empty terminal success", messages, code, err)
	}
}

func TestListIngestionLogsEmbedsLatestRunEventWithoutProgressMessage(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	insertMessageRun(t, "run-1", "kb-1", "doc-1", entity.TaskStatusDone, 1)
	insertMessageRun(t, "run-2", "kb-1", "doc-2", entity.TaskStatusDone, 1)
	if err := db.Model(&entity.PipelineOperationLog{}).Where("id IN ?", []string{"run-1", "run-2"}).Update("progress_msg", "obsolete snapshot").Error; err != nil {
		t.Fatalf("seed obsolete progress message: %v", err)
	}
	insertMessageEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, "first")
	insertMessageEvent(t, db, "run-1", "task-1", dao.EventTypeTerminal, "latest first")
	insertMessageEvent(t, db, "run-2", "task-2", dao.EventTypeMessage, "latest second")

	result, code, err := NewDatasetService().ListIngestionLogs(t.Context(), "kb-1", "user-1", 1, 30, nil, nil, "", "", "file", "", "")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionLogs = (%+v, %v, %v), want success", result, code, err)
	}
	logs, ok := result["logs"].([]map[string]interface{})
	if !ok || len(logs) != 2 {
		t.Fatalf("logs = %#v, want two file logs", result["logs"])
	}
	byID := make(map[string]map[string]interface{}, len(logs))
	for _, log := range logs {
		byID[log["id"].(string)] = log
	}
	assertLatestEventMap(t, byID["run-1"], 2, "latest first")
	assertLatestEventMap(t, byID["run-2"], 3, "latest second")
	for runID, log := range byID {
		if _, ok := log["progress_msg"]; ok {
			t.Fatalf("%s exposes obsolete progress_msg: %#v", runID, log["progress_msg"])
		}
	}
}

func TestGetIngestionLogEmbedsLatestRunEvent(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	insertMessageRun(t, "run-1", "kb-1", "doc-1", entity.TaskStatusDone, 1)
	insertMessageEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, "latest detail")

	result, code, err := NewDatasetService().GetIngestionLog(t.Context(), "kb-1", "user-1", "run-1")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("GetIngestionLog = (%+v, %v, %v), want success", result, code, err)
	}
	assertLatestEventMap(t, result, 1, "latest detail")
}

func TestIngestionEventWriterFeedsRunScopedLogReaders(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	insertMessageRun(t, "run-1", "kb-1", "doc-1", entity.TaskStatusRunning, 1)

	writer := service.NewIngestionTaskService()
	if err := writer.RecordLifecycle(t.Context(), "run-1", "task-1", "Parser", 0, "Parser started"); err != nil {
		t.Fatalf("record lifecycle event: %v", err)
	}
	if err := writer.RecordMessage(t.Context(), "run-1", "task-1", "Parsing pages 2/8"); err != nil {
		t.Fatalf("record progress event: %v", err)
	}

	messages, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "run-1", 200, nil, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionMessages = (%+v, %v, %v), want success", messages, code, err)
	}
	if len(messages.Items) != 2 || messages.Items[0].Message != "Parser started" || messages.Items[1].Message != "Parsing pages 2/8" {
		t.Fatalf("messages = %+v, want lifecycle then latest progress", messages.Items)
	}

	log, code, err := NewDatasetService().GetIngestionLog(t.Context(), "kb-1", "user-1", "run-1")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("GetIngestionLog = (%+v, %v, %v), want success", log, code, err)
	}
	assertLatestEventMap(t, log, messages.Items[1].ID, "Parsing pages 2/8")
}

func insertMessageRun(t *testing.T, id, kbID, documentID string, status entity.TaskStatus, runCount int) {
	t.Helper()
	runCountPtr := &runCount
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              id,
		DocumentID:      documentID,
		TenantID:        "user-1",
		KbID:            kbID,
		ParserID:        "naive",
		DocumentName:    "doc.txt",
		DocumentSuffix:  ".txt",
		DocumentType:    "text",
		SourceFrom:      "local",
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: string(status),
		RunCount:        runCountPtr,
	}).Error; err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func insertMessageEvent(t *testing.T, db *gorm.DB, runID, taskID string, eventType int, message string) {
	t.Helper()
	if err := db.Model(&entity.IngestionTaskLog{}).Create(map[string]interface{}{
		"task_id":         taskID,
		"pipeline_log_id": runID,
		"checkpoint":      entity.JSONMap{},
		"event_type":      eventType,
		"component":       "",
		"phase":           0,
		"message":         message,
	}).Error; err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func assertMessageEventIDs(t *testing.T, items []service.IngestionEventItem, want ...int) {
	t.Helper()
	if len(items) != len(want) {
		t.Fatalf("item count = %d, want %d (%v)", len(items), len(want), want)
	}
	for i, item := range items {
		if item.ID != want[i] {
			t.Fatalf("item %d ID = %d, want %d", i, item.ID, want[i])
		}
	}
}

func assertLatestEventMap(t *testing.T, log map[string]interface{}, wantID int, wantMessage string) {
	t.Helper()
	event, ok := log["latest_ingestion_event"].(*service.IngestionEventItem)
	if !ok || event == nil {
		t.Fatalf("latest_ingestion_event = %#v, want event item", log["latest_ingestion_event"])
	}
	if event.ID != wantID {
		t.Fatalf("latest event ID = %d, want %d", event.ID, wantID)
	}
	if event.Message != wantMessage {
		t.Fatalf("latest event message = %q, want %q", event.Message, wantMessage)
	}
}
