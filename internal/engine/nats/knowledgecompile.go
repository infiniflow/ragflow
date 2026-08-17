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

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Knowledge-compile (§11) subjects, stream, queue group and KV bucket. The
// subject prefix is shared by the stream's Subjects filter and the consumer's
// FilterSubject, so every published event must sit under knowledge.compile.events.>.
const (
	knowledgeCompileStreamName    = "RAGFLOW_KNOWLEDGE_COMPILE_EVENTS"
	knowledgeCompileSubjectPrefix = "knowledge.compile.events.>"
	knowledgeCompileQueueGroup    = "knowledge_compile_events_q"
	knowledgeCompileKVBucket      = "knowledge_compile_leases"
)

// knowledgeCompileLeaseValue is the CAS-protected payload stored under lock:<dataset_id>.
type knowledgeCompileLeaseValue struct {
	Holder string `json:"holder"`
	Expiry int64  `json:"expiry"` // unix nanos; lease is stale when Expiry < now
}

// InitKnowledgeCompileStream creates the dedicated RAGFLOW_KNOWLEDGE_COMPILE_EVENTS stream,
// isolated from the task queue (RAGFLOW_TASKS).
func (n *NatsEngine) InitKnowledgeCompileStream() error {
	if n.jetStream == nil {
		return fmt.Errorf("knowledgecompile: jetStream not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := jetstream.StreamConfig{
		Name:      knowledgeCompileStreamName,
		Subjects:  []string{knowledgeCompileSubjectPrefix},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
		MaxMsgs:   1024 * 1024,
		MaxBytes:  1024 * 1024 * 64,
	}
	if _, err := n.jetStream.CreateStream(ctx, cfg); err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("knowledgecompile: create stream: %w", err)
	}
	return nil
}

// PublishKnowledgeCompile publishes a wake-up payload on the notify subject via
// core NATS. The subject (notify.kc.workers) is intentionally outside the
// knowledge.compile.events stream, so it must go through core NATS rather than
// the JetStream Publish API, which errors with "no stream matches subject".
func (n *NatsEngine) PublishKnowledgeCompile(subject string, payload []byte) error {
	if n.nc == nil {
		return fmt.Errorf("knowledgecompile: nats not initialized")
	}
	return n.nc.Publish(subject, payload)
}

// InitKnowledgeCompileConsumer creates the competing-consumer (queue group) for the knowledge-compile stream.
func (n *NatsEngine) InitKnowledgeCompileConsumer() error {
	if n.jetStream == nil {
		return fmt.Errorf("knowledgecompile: jetStream not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := n.jetStream.Stream(ctx, knowledgeCompileStreamName)
	if err != nil {
		return fmt.Errorf("knowledgecompile: stream not found (call InitKnowledgeCompileStream first): %w", err)
	}
	n.knowledgeCompileStream = stream
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          "KNOWLEDGE_COMPILE_CONSUMER",
		Durable:       "knowledge_compile_durable",
		DeliverGroup:  knowledgeCompileQueueGroup,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    16,
		MaxAckPending: 1024 * 128,
		FilterSubject: knowledgeCompileSubjectPrefix,
	})
	if err != nil {
		if strings.Contains(err.Error(), "max waiting can not be updated") {
			cons, err = stream.Consumer(ctx, "KNOWLEDGE_COMPILE_CONSUMER")
			if err != nil {
				return fmt.Errorf("knowledgecompile: get existing consumer: %w", err)
			}
		} else {
			return fmt.Errorf("knowledgecompile: create consumer: %w", err)
		}
	}
	n.knowledgeCompileConsumer = cons
	return nil
}

// FetchKnowledgeCompileMessages pulls up to batchSize messages from the knowledge-compile consumer. Because
// the consumer filters by subject only (never payload), the returned batch is
// a mix of datasets; the caller keeps the KB it is processing and Naks the rest.
func (n *NatsEngine) FetchKnowledgeCompileMessages(batchSize int) ([]common.RawMessage, error) {
	if n.knowledgeCompileConsumer == nil {
		return nil, fmt.Errorf("knowledgecompile: consumer not initialized (call InitKnowledgeCompileConsumer first)")
	}
	messages, err := n.knowledgeCompileConsumer.Fetch(batchSize, jetstream.FetchMaxWait(1*time.Second))
	if err != nil {
		// A max-wait timeout with zero messages is the normal "nothing to do"
		// condition for a polling fetch, not an error: surface it as an empty
		// batch so the caller can idle without logging or backoff-pacing.
		if errors.Is(err, nats.ErrTimeout) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]common.RawMessage, 0, 8)
	for msg := range messages.Messages() {
		out = append(out, &natsRawHandle{msg: msg})
	}
	return out, nil
}

// natsRawHandle adapts a jetstream.Msg to common.RawMessage.
type natsRawHandle struct {
	msg jetstream.Msg
}

func (h *natsRawHandle) Data() []byte { return h.msg.Data() }
func (h *natsRawHandle) Ack() error   { return h.msg.Ack() }
func (h *natsRawHandle) Nak() error   { return h.msg.Nak() }

// InitKnowledgeCompileLeases creates the KV bucket backing per-KB leases.
func (n *NatsEngine) InitKnowledgeCompileLeases() error {
	if n.jetStream == nil {
		return fmt.Errorf("knowledgecompile: jetStream not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	kv, err := n.jetStream.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: knowledgeCompileKVBucket})
	if err != nil {
		return fmt.Errorf("knowledgecompile: create kv: %w", err)
	}
	n.kv = kv
	return nil
}

// AcquireKnowledgeCompileLease attempts to take the lease for key (CAS). It succeeds when the
// key is absent or its previous holder's TTL has expired. Returns the new
// revision and acquired=true on success.
func (n *NatsEngine) AcquireKnowledgeCompileLease(key, holder string, ttl time.Duration) (uint64, bool, error) {
	if n.kv == nil {
		return 0, false, fmt.Errorf("knowledgecompile: kv not initialized (call InitKnowledgeCompileLeases first)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	val, _ := json.Marshal(knowledgeCompileLeaseValue{Holder: holder, Expiry: time.Now().Add(ttl).UnixNano()})

	existing, err := n.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			rev, cerr := n.kv.Create(ctx, key, val)
			if cerr != nil {
				return 0, false, nil // lost the race
			}
			return rev, true, nil
		}
		return 0, false, err
	}
	var lv knowledgeCompileLeaseValue
	_ = json.Unmarshal(existing.Value(), &lv)
	if lv.Expiry < time.Now().UnixNano() {
		// Stale lease: CAS-overwrite with our revision.
		nrev, uerr := n.kv.Update(ctx, key, val, existing.Revision())
		if uerr != nil {
			return 0, false, nil
		}
		return nrev, true, nil
	}
	return 0, false, nil
}

// HeartbeatKnowledgeCompileLease refreshes the TTL of a held lease. revision must match the
// current holder; a revision mismatch (returning ok=false) means the lease was
// taken over by another instance and the caller must abort.
func (n *NatsEngine) HeartbeatKnowledgeCompileLease(key, holder string, ttl time.Duration, revision uint64) (uint64, bool, error) {
	if n.kv == nil {
		return 0, false, fmt.Errorf("knowledgecompile: kv not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	val, _ := json.Marshal(knowledgeCompileLeaseValue{Holder: holder, Expiry: time.Now().Add(ttl).UnixNano()})
	nrev, err := n.kv.Update(ctx, key, val, revision)
	if err != nil {
		return 0, false, nil
	}
	return nrev, true, nil
}

// ReleaseKnowledgeCompileLease deletes the lease only if we still own it (revision match),
// so we never release a lease another instance has since acquired.
func (n *NatsEngine) ReleaseKnowledgeCompileLease(key, holder string, revision uint64) error {
	if n.kv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := n.kv.Delete(ctx, key, jetstream.LastRevision(revision))
	if err != nil && strings.Contains(err.Error(), "wrong") {
		return nil // someone else owns it now; nothing to release
	}
	return err
}

// knowledgeCompileNotifySubject is the Option E wake-up channel: workers
// subscribe here and publishers push a {dataset_id} payload after appending to
// a KB's MySQL backlog. It is intentionally outside the knowledge.compile.events
// stream because it carries no routing payload — MySQL is the scheduling truth.
const knowledgeCompileNotifySubject = "notify.kc.workers"

// SubscribeNotify opens a core NATS subscription to the wake-up subject and
// streams dataset ids to the returned channel. The subscription is torn down
// when ctx is cancelled. A full channel is dropped (not blocked) so a slow
// worker can never stall publishers.
func (n *NatsEngine) SubscribeNotify(ctx context.Context) (<-chan string, error) {
	if n.nc == nil {
		return nil, fmt.Errorf("knowledgecompile: nats not initialized")
	}
	ch := make(chan string, 256)
	// done guards the send so the callback never writes to a closed channel:
	// Sub.Unsubscribe does not wait for an in-flight dispatcher callback, so we
	// must not close(ch) while a callback may still run. The reader selects on
	// ctx.Done as well, so leaving ch open is safe and avoids the panic.
	done := make(chan struct{})
	sub, err := n.nc.Subscribe(knowledgeCompileNotifySubject, func(m *nats.Msg) {
		var p struct {
			DatasetID string `json:"dataset_id"`
		}
		if err := json.Unmarshal(m.Data, &p); err != nil || p.DatasetID == "" {
			return
		}
		select {
		case ch <- p.DatasetID:
		case <-done:
		}
	})
	if err != nil {
		return nil, fmt.Errorf("knowledgecompile: subscribe notify: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
		close(done)
	}()
	return ch, nil
}
