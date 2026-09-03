//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package testutil

import (
	"net"
	"testing"
	"time"

	natsengine "ragflow/internal/engine/nats"

	"github.com/nats-io/nats-server/v2/server"
)

// SetupNatsEngine starts an in-process NATS server with JetStream enabled
// on a random port, creates a NatsEngine connected to it, and calls Init()
// to set up the RAGFLOW_TASKS stream. The server is automatically shut down
// when the test completes.
//
// The caller is responsible for calling InitConsumer if a consumer is needed.
func SetupNatsEngine(t *testing.T) *natsengine.NatsEngine {
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
		t.Fatal("embedded NATS server did not become ready within 10s")
	}

	t.Cleanup(func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	addr := ns.Addr().(*net.TCPAddr)
	engine := natsengine.NewNatsEngine("127.0.0.1", addr.Port)
	if err := engine.Init(); err != nil {
		t.Fatalf("NatsEngine.Init against embedded server: %v", err)
	}

	return engine
}
