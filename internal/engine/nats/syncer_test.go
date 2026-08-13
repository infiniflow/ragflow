package nats

import (
	"context"
	"net"
	syncerconnector "ragflow/internal/syncer/connector"
	"testing"
	"time"

	"ragflow/internal/common"

	"github.com/nats-io/nats-server/v2/server"
)

// TestSyncerTaskStreamPublishesAndSubscribes verifies the dedicated syncer stream path.
func TestSyncerTaskStreamPublishesAndSubscribes(t *testing.T) {
	engine := setupSyncerNATSEngine(t)
	if err := engine.InitSyncerStream(); err != nil {
		t.Fatalf("InitSyncerStream: %v", err)
	}
	if err := engine.InitSyncerConsumer(); err != nil {
		t.Fatalf("InitSyncerConsumer: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	received := make(chan common.TaskHandle, 1)
	if err := engine.SubscribeSyncerTasks(ctx, func(handle common.TaskHandle) {
		received <- handle
	}); err != nil {
		t.Fatalf("SubscribeSyncerTasks: %v", err)
	}
	if err := engine.PublishSyncerTask("task-1"); err != nil {
		t.Fatalf("PublishSyncerTask: %v", err)
	}

	var handle common.TaskHandle
	select {
	case handle = <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for syncer task")
	}
	message := handle.GetMessage()
	if message.TaskID != "task-1" || message.TaskType != common.TaskTypeSyncer {
		t.Fatalf("message = %+v", message)
	}
	if err := handle.Ack(); err != nil {
		t.Fatalf("Ack: %v", err)
	}
}

// TestSyncCheckpointKVSaveLoadDelete verifies the syncer checkpoint KV lifecycle.
func TestSyncCheckpointKVSaveLoadDelete(t *testing.T) {
	engine := setupSyncerNATSEngine(t)
	now := time.Date(2026, 8, 11, 1, 2, 3, 0, time.UTC)
	state := syncerconnector.SyncCheckpointState{
		Version:       1,
		TaskID:        "task-1",
		ConnectorID:   "conn-1",
		KBID:          "kb-1",
		WindowEnd:     now,
		NextCommitSeq: 2,
		Checkpoint:    &syncerconnector.SyncCheckpoint{Cursor: "cursor-1", UpdatedAt: &now, SourceID: "source-1"},
	}
	if err := engine.SaveSyncCheckpoint(t.Context(), "task-1", state); err != nil {
		t.Fatalf("SaveSyncCheckpoint: %v", err)
	}

	loaded, err := engine.LoadSyncCheckpoint(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("LoadSyncCheckpoint: %v", err)
	}
	if loaded == nil || loaded.Checkpoint == nil {
		t.Fatalf("loaded checkpoint = %+v, want value", loaded)
	}
	if loaded.TaskID != "task-1" || loaded.Checkpoint.Cursor != "cursor-1" || loaded.Checkpoint.SourceID != "source-1" {
		t.Fatalf("loaded checkpoint = %+v", loaded)
	}

	if err = engine.DeleteSyncCheckpoint(t.Context(), "task-1"); err != nil {
		t.Fatalf("DeleteSyncCheckpoint: %v", err)
	}
	loaded, err = engine.LoadSyncCheckpoint(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("LoadSyncCheckpoint after delete: %v", err)
	}
	if loaded != nil {
		t.Fatalf("loaded checkpoint after delete = %+v, want nil", loaded)
	}
}

func setupSyncerNATSEngine(t *testing.T) *NatsEngine {
	t.Helper()
	opts := &server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("create embedded NATS server: %v", err)
	}
	ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		t.Fatal("embedded NATS server did not become ready")
	}
	t.Cleanup(func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	addr := ns.Addr().(*net.TCPAddr)
	engine := NewNatsEngine("127.0.0.1", addr.Port)
	if err := engine.Init(); err != nil {
		t.Fatalf("NatsEngine.Init: %v", err)
	}
	return engine
}
