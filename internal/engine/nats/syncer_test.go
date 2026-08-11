package nats

import (
	"net"
	"testing"
	"time"

	"ragflow/internal/common"

	"github.com/nats-io/nats-server/v2/server"
)

// TestSyncerTaskStreamPublishesAndFetches verifies the dedicated syncer stream path.
func TestSyncerTaskStreamPublishesAndFetches(t *testing.T) {
	engine := setupSyncerNATSEngine(t)
	if err := engine.InitSyncerStream(); err != nil {
		t.Fatalf("InitSyncerStream: %v", err)
	}
	if err := engine.InitSyncerConsumer(); err != nil {
		t.Fatalf("InitSyncerConsumer: %v", err)
	}
	if err := engine.PublishSyncerTask("task-1"); err != nil {
		t.Fatalf("PublishSyncerTask: %v", err)
	}

	handles, err := engine.FetchSyncerTasks(1)
	if err != nil {
		t.Fatalf("FetchSyncerTasks: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("handles len = %d, want 1", len(handles))
	}
	message := handles[0].GetMessage()
	if message.TaskID != "task-1" || message.TaskType != common.TaskTypeSyncer {
		t.Fatalf("message = %+v", message)
	}
	if err := handles[0].Ack(); err != nil {
		t.Fatalf("Ack: %v", err)
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
