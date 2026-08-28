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

package nats

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"ragflow/internal/common"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// newEmbeddedNatsServer starts an in-process JetStream-enabled NATS server on
// a random port and returns its host/port.
func newEmbeddedNatsServer(t *testing.T) (string, int) {
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
	return "127.0.0.1", addr.Port
}

// TestPublishTaskDedup: publishing the same task twice within the stream's
// Duplicates window must collapse to a single stored message, so a startup
// reconciliation republish racing a residual original message can never
// double-execute a task.
func TestPublishTaskDedup(t *testing.T) {
	host, port := newEmbeddedNatsServer(t)
	engine := NewNatsEngine(host, port)
	if err := engine.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	payload, err := json.Marshal(common.TaskMessage{TaskID: "task-dedup-1", TaskType: common.TaskTypeIngestionTask})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err = engine.PublishTask("tasks.RAGFLOW", payload); err != nil {
		t.Fatalf("first PublishTask: %v", err)
	}
	if err = engine.PublishTask("tasks.RAGFLOW", payload); err != nil {
		t.Fatalf("duplicate PublishTask: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := engine.stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("stream holds %d messages after duplicate publish, want 1 (MsgID dedup not applied)", info.State.Msgs)
	}

	// A payload without a task_id must degrade to a plain publish, not fail.
	if err = engine.PublishTask("tasks.RAGFLOW", []byte("not-json")); err != nil {
		t.Fatalf("non-TaskMessage PublishTask: %v", err)
	}
}

// TestInitMigratesLegacyStreamConfig: a stream created by an older deployment
// (no Duplicates, 1MB MaxBytes) must be migrated in place by Init instead of
// being left stale behind an "already exists" error. The server-side config is
// the merge base: fields the helper does not own (Subjects, Retention) must
// survive the update.
func TestInitMigratesLegacyStreamConfig(t *testing.T) {
	host, port := newEmbeddedNatsServer(t)

	// Pre-create the stream with the legacy (pre-migration) configuration.
	nc, err := nats.Connect("nats://" + net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("connect legacy stream creator: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream context: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	legacy, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:      "RAGFLOW_TASKS",
		Subjects:  []string{"tasks.>"},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
		MaxMsgs:   1024 * 128,
		MaxBytes:  1024 * 1024,
	})
	if err != nil {
		t.Fatalf("create legacy stream: %v", err)
	}
	legacyInfo, err := legacy.Info(ctx)
	if err != nil {
		t.Fatalf("legacy stream info: %v", err)
	}
	// Precondition: the legacy stream differs from the wanted config (server
	// fills a 2m default Duplicates when unspecified).
	if legacyInfo.Config.MaxBytes != int64(1024*1024) {
		t.Fatalf("precondition: legacy MaxBytes = %d, want 1MB", legacyInfo.Config.MaxBytes)
	}
	if legacyInfo.Config.Duplicates == 10*time.Minute {
		t.Fatalf("precondition: legacy Duplicates already migrated (%v)", legacyInfo.Config.Duplicates)
	}

	engine := NewNatsEngine(host, port)
	if err := engine.Init(); err != nil {
		t.Fatalf("Init over legacy stream: %v", err)
	}

	info, err := engine.stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream info after migration: %v", err)
	}
	if got := info.Config.MaxBytes; got != int64(1024*1024*64) {
		t.Fatalf("MaxBytes after migration = %d, want %d", got, int64(1024*1024*64))
	}
	if got := info.Config.Duplicates; got != 10*time.Minute {
		t.Fatalf("Duplicates after migration = %v, want 10m", got)
	}
	// Non-owned fields must not be reset by the partial update.
	if got := info.Config.Retention; got != jetstream.WorkQueuePolicy {
		t.Fatalf("Retention after migration = %v, want WorkQueuePolicy (must not be reset)", got)
	}
	if len(info.Config.Subjects) != 1 || info.Config.Subjects[0] != "tasks.>" {
		t.Fatalf("Subjects after migration = %v, want [tasks.>] (must not be reset)", info.Config.Subjects)
	}
}

// TestInitConsumerSetsAckWaitAndBackOff: the consumer must carry an explicit
// redelivery schedule (AckWait + BackOff) so unacked messages are retried on
// a paced backoff instead of the broker default.
func TestInitConsumerSetsAckWaitAndBackOff(t *testing.T) {
	host, port := newEmbeddedNatsServer(t)
	engine := NewNatsEngine(host, port)
	if err := engine.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := engine.InitConsumer("tasks.>"); err != nil {
		t.Fatalf("InitConsumer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := engine.consumer.Info(ctx)
	if err != nil {
		t.Fatalf("consumer info: %v", err)
	}
	wantBackOff := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second}
	if len(info.Config.BackOff) != len(wantBackOff) {
		t.Fatalf("BackOff = %v, want %v", info.Config.BackOff, wantBackOff)
	}
	for i, d := range wantBackOff {
		if info.Config.BackOff[i] != d {
			t.Fatalf("BackOff[%d] = %v, want %v", i, info.Config.BackOff[i], d)
		}
	}
	// The server normalizes AckWait to BackOff[0] when BackOff is present;
	// anything else means the schedule was not persisted as configured.
	if got := info.Config.AckWait; got != wantBackOff[0] {
		t.Fatalf("AckWait = %v, want %v (server-normalized to BackOff[0])", got, wantBackOff[0])
	}
}
