package knowledge_compile

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func TestMarkWikiDocumentDirtyDebouncesAndMergesChunks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.WikiDocumentDirty{}); err != nil {
		t.Fatalf("migrate dirty table: %v", err)
	}
	previousDB := kcDB
	kcDB = db
	t.Cleanup(func() { kcDB = previousDB })

	if err := MarkWikiDocumentDirty(t.Context(), "tenant", "dataset", "document", []string{"chunk-b", "chunk-a"}); err != nil {
		t.Fatalf("first dirty mark: %v", err)
	}
	var first entity.WikiDocumentDirty
	if err := db.First(&first, "document_id = ?", "document").Error; err != nil {
		t.Fatalf("load first dirty mark: %v", err)
	}
	time.Sleep(time.Millisecond)
	if err := MarkWikiDocumentDirty(t.Context(), "tenant", "dataset", "document", []string{"chunk-c", "chunk-b"}); err != nil {
		t.Fatalf("second dirty mark: %v", err)
	}
	var second entity.WikiDocumentDirty
	if err := db.First(&second, "document_id = ?", "document").Error; err != nil {
		t.Fatalf("load second dirty mark: %v", err)
	}

	if second.Revision != 2 {
		t.Fatalf("revision = %d, want 2", second.Revision)
	}
	if !second.RunAfter.After(first.RunAfter) {
		t.Fatalf("run_after was not postponed: first=%s second=%s", first.RunAfter, second.RunAfter)
	}
	if got, want := parseDirtyChunkIDs(second.AffectedChunkIDs), []string{"chunk-a", "chunk-b", "chunk-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("affected chunks = %v, want %v", got, want)
	}
	if delay := time.Until(second.RunAfter); delay < wikiDirtyDebounce-time.Second || delay > wikiDirtyDebounce+time.Second {
		t.Fatalf("debounce delay = %s, want about %s", delay, wikiDirtyDebounce)
	}
}

func TestWikiDirtyRevisionPreventsDroppingConcurrentMutation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.WikiDocumentDirty{}); err != nil {
		t.Fatalf("migrate dirty table: %v", err)
	}
	previousDB := kcDB
	kcDB = db
	t.Cleanup(func() { kcDB = previousDB })

	if err := MarkWikiDocumentDirty(t.Context(), "tenant", "dataset", "document", []string{"chunk-a"}); err != nil {
		t.Fatalf("dirty mark: %v", err)
	}
	if err := db.Model(&entity.WikiDocumentDirty{}).Where("document_id = ?", "document").Updates(map[string]any{
		"state": entity.WikiDirtyStateRunning, "claim_owner": "worker",
	}).Error; err != nil {
		t.Fatalf("claim dirty row: %v", err)
	}
	var claimed entity.WikiDocumentDirty
	if err := db.First(&claimed, "document_id = ?", "document").Error; err != nil {
		t.Fatalf("load claimed row: %v", err)
	}

	wikiDirtyCompilerState.Lock()
	previousCompiler := wikiDirtyCompilerState.compiler
	wikiDirtyCompilerState.compiler = func(_ context.Context, _ WikiDirtyRequest) error {
		return MarkWikiDocumentDirty(t.Context(), "tenant", "dataset", "document", []string{"chunk-b"})
	}
	wikiDirtyCompilerState.Unlock()
	t.Cleanup(func() { SetWikiDirtyCompiler(previousCompiler) })

	processWikiDirty(t.Context(), db, "worker", claimed)

	var pending entity.WikiDocumentDirty
	if err := db.First(&pending, "document_id = ?", "document").Error; err != nil {
		t.Fatalf("concurrent mutation was dropped: %v", err)
	}
	if pending.Revision != 2 || pending.State != entity.WikiDirtyStatePending {
		t.Fatalf("pending row = %+v, want revision 2 pending", pending)
	}
}
