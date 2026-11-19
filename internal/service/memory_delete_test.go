package service

import (
	"context"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
)

type memoryDeleteDocEngine struct {
	memoryMessageDocEngine
	dropIndex, dropID     string
	dropCalls             int
	deleteCond            map[string]interface{}
	deleteIndex, deleteID string
	deleteCalls           int
}

func (e *memoryDeleteDocEngine) DropChunkStore(_ context.Context, name, id string) error {
	e.dropCalls++
	e.dropIndex, e.dropID = name, id
	return nil
}

func (e *memoryDeleteDocEngine) DeleteChunks(_ context.Context, cond map[string]interface{}, name, id string) (int64, error) {
	e.deleteCalls++
	e.deleteCond, e.deleteIndex, e.deleteID = cond, name, id
	return 0, nil
}

func TestDeleteMemoryRemovesStoredMessages(t *testing.T) {
	for _, test := range []struct {
		engineType string
		wantDrop   bool
	}{
		{engineType: "elasticsearch"},
		{engineType: "opensearch"},
		{engineType: "infinity", wantDrop: true},
		{engineType: "oceanbase", wantDrop: true},
		{engineType: "vastbase", wantDrop: true},
	} {
		t.Run(test.engineType, func(t *testing.T) {
			setupMemoryMessageTestDB(t)
			if err := dao.DB.Create(&entity.Memory{
				ID: "memory-1", Name: "mem", UserID: "user-1", TenantID: "user-1",
				MemoryType: dao.MemoryTypeRaw, StorageType: "table", EmbdID: "embd", LLMID: "llm",
				Permissions: string(entity.TenantPermissionMe), MemorySize: MemorySizeLimit,
			}).Error; err != nil {
				t.Fatalf("seed memory: %v", err)
			}
			docEngine := &memoryDeleteDocEngine{memoryMessageDocEngine: memoryMessageDocEngine{engineType: test.engineType}}
			service := NewMemoryService(dao.DB, engine.Get())
			service.docEngine = docEngine

			if err := service.DeleteMemory(serviceTestContext(t, "user-1", "user-1"), "memory-1"); err != nil {
				t.Fatalf("DeleteMemory() error = %v", err)
			}
			if test.wantDrop {
				if docEngine.dropCalls != 1 || docEngine.dropIndex != "memory_user-1" || docEngine.dropID != "memory-1" || docEngine.deleteCalls != 0 {
					t.Fatalf("drop = %d %q %q, delete calls = %d", docEngine.dropCalls, docEngine.dropIndex, docEngine.dropID, docEngine.deleteCalls)
				}
			} else if docEngine.deleteCalls != 1 || docEngine.deleteIndex != "memory_user-1" || docEngine.deleteID != "memory-1" || len(docEngine.deleteCond) != 0 || docEngine.dropCalls != 0 {
				t.Fatalf("delete = %d %q %q %v, drop calls = %d", docEngine.deleteCalls, docEngine.deleteIndex, docEngine.deleteID, docEngine.deleteCond, docEngine.dropCalls)
			}
			var n int64
			dao.DB.Model(&entity.Memory{}).Where("id = ?", "memory-1").Count(&n)
			if n != 0 {
				t.Fatalf("memory row still present")
			}
		})
	}
}
