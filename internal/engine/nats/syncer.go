//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/common"
	syncerconnector "ragflow/internal/syncer/connector"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

const (
	// SyncerTaskSubject is the JetStream subject carrying sync_logs task IDs.
	SyncerTaskSubject = "sync.tasks.RAGFLOW"

	syncerStreamName     = "RAGFLOW_SYNC_TASKS"
	syncerConsumerName   = "RAGFLOW_SYNCER_CONSUMER"
	syncCheckpointBucket = "RAGFLOW_SYNC_CHECKPOINTS"
	syncerDeliverSubject = "deliver.syncer.RAGFLOW"
	syncerDeliverGroup   = "RAGFLOW_SYNCER_WORKERS"
	syncerSubjectPattern = "sync.tasks.>"
	syncCheckpointTTL    = 7 * 24 * time.Hour
)

// InitSyncerStream creates the datasource syncer task stream.
func (n *NatsEngine) InitSyncerStream() error {
	n.syncerMu.Lock()
	defer n.syncerMu.Unlock()
	return n.initSyncerStreamLocked()
}

func (n *NatsEngine) initSyncerStreamLocked() error {
	if n.jetStream == nil {
		return fmt.Errorf("syncer: jetStream not initialized")
	}
	if n.syncerStream != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// create  jetStream
	stream, err := ensureStreamConfig(ctx, n.jetStream, jetstream.StreamConfig{
		Name:       syncerStreamName,
		Subjects:   []string{syncerSubjectPattern},
		Retention:  jetstream.WorkQueuePolicy,
		Storage:    jetstream.FileStorage,
		Discard:    jetstream.DiscardNew,
		MaxMsgs:    1024 * 128,
		MaxBytes:   1024 * 1024 * 64,
		Duplicates: 10 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("syncer: create stream: %w", err)
	}
	n.syncerStream = stream
	return nil
}

// InitSyncerConsumer creates the durable push consumer for syncer tasks.
func (n *NatsEngine) InitSyncerConsumer() error {
	n.syncerMu.Lock()
	defer n.syncerMu.Unlock()
	if err := n.initSyncerStreamLocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := syncerPushConsumerConfig()
	consumer, err := n.syncerStream.CreateOrUpdatePushConsumer(ctx, config)
	if err != nil && shouldRecreateSyncerConsumer(ctx, n.syncerStream, config, err) {
		common.Warn("syncer replacing incompatible NATS consumer", zap.String("consumer", syncerConsumerName), zap.Error(err), zap.Bool("delete", true))
		if deleteErr := n.syncerStream.DeleteConsumer(ctx, syncerConsumerName); deleteErr != nil {
			if !strings.Contains(strings.ToLower(deleteErr.Error()), "not found") {
				return fmt.Errorf("syncer: replace existing consumer: %w", deleteErr)
			}
		}
		consumer, err = n.syncerStream.CreateOrUpdatePushConsumer(ctx, config)
	}
	if err != nil {
		return fmt.Errorf("syncer: create push consumer: %w", err)
	}
	n.syncerConsumer = consumer
	return nil
}

// InitSyncCheckpoints creates the KV bucket backing running sync task checkpoints.
func (n *NatsEngine) InitSyncCheckpoints() error {
	n.syncerMu.Lock()
	defer n.syncerMu.Unlock()
	return n.initSyncCheckpointsLocked()
}

func (n *NatsEngine) initSyncCheckpointsLocked() error {
	if n.jetStream == nil {
		return fmt.Errorf("syncer checkpoint: jetStream not initialized")
	}
	if n.syncCheckpointKV != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	kv, err := n.jetStream.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:       syncCheckpointBucket,
		Description:  "running datasource sync checkpoints",
		History:      1,
		TTL:          syncCheckpointTTL,
		MaxValueSize: 1024 * 1024,
		Storage:      jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("syncer checkpoint: create kv: %w", err)
	}
	n.syncCheckpointKV = kv
	return nil
}

func syncerPushConsumerConfig() jetstream.ConsumerConfig {
	return jetstream.ConsumerConfig{
		Name:           syncerConsumerName,
		Durable:        syncerConsumerName,
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxDeliver:     16,
		MaxAckPending:  1024,
		FilterSubject:  syncerSubjectPattern,
		DeliverSubject: syncerDeliverSubject,
		DeliverGroup:   syncerDeliverGroup,
	}
}

func shouldRecreateSyncerConsumer(ctx context.Context, stream jetstream.Stream, desired jetstream.ConsumerConfig, err error) bool {
	var jsErr jetstream.JetStreamError
	if !errors.As(err, &jsErr) || jsErr.APIError() == nil || jsErr.APIError().ErrorCode != jetstream.JSErrCodeConsumerCreate {
		return false
	}

	existing, inspectErr := syncerExistingConsumerConfig(ctx, stream)
	if inspectErr != nil {
		common.Warn("syncer consumer replacement skipped", zap.String("consumer", syncerConsumerName), zap.Error(err), zap.NamedError("inspect_error", inspectErr), zap.Bool("delete", false))
		return false
	}
	if existing == nil {
		common.Warn("syncer consumer replacement skipped", zap.String("consumer", syncerConsumerName), zap.Error(err), zap.Bool("delete", false))
		return false
	}
	return syncerConsumerConfigMismatch(*existing, desired)
}

func syncerExistingConsumerConfig(ctx context.Context, stream jetstream.Stream) (*jetstream.ConsumerConfig, error) {
	lister := stream.ListConsumers(ctx)
	for info := range lister.Info() {
		if info != nil && info.Name == syncerConsumerName {
			config := info.Config
			return &config, nil
		}
	}
	return nil, lister.Err()
}

func syncerConsumerConfigMismatch(existing, desired jetstream.ConsumerConfig) bool {
	return existing.Durable != desired.Durable ||
		existing.Name != desired.Name ||
		existing.AckPolicy != desired.AckPolicy ||
		existing.MaxDeliver != desired.MaxDeliver ||
		existing.MaxAckPending != desired.MaxAckPending ||
		existing.FilterSubject != desired.FilterSubject ||
		existing.DeliverSubject != desired.DeliverSubject ||
		existing.DeliverGroup != desired.DeliverGroup
}

// PublishSyncerTask publishes one sync_logs task wake-up.
func (n *NatsEngine) PublishSyncerTask(taskID string) error {
	if err := n.InitSyncerStream(); err != nil {
		return err
	}

	payload, err := json.Marshal(common.TaskMessage{TaskID: taskID, TaskType: common.TaskTypeSyncer})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// publish to nats
	_, err = n.jetStream.Publish(ctx, SyncerTaskSubject, payload, jetstream.WithMsgID(taskID), jetstream.WithExpectStream(syncerStreamName))
	return err
}

// PublishSyncerTaskWakeup publishes a non-deduplicated wake-up for an existing sync_logs task.
func (n *NatsEngine) PublishSyncerTaskWakeup(taskID string) error {
	if err := n.InitSyncerStream(); err != nil {
		return err
	}

	payload, err := json.Marshal(common.TaskMessage{TaskID: taskID, TaskType: common.TaskTypeSyncer})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msgID := fmt.Sprintf("%s.manual-wakeup.%d", taskID, time.Now().UnixNano())
	_, err = n.jetStream.Publish(ctx, SyncerTaskSubject, payload, jetstream.WithMsgID(msgID), jetstream.WithExpectStream(syncerStreamName))
	return err
}

// SubscribeSyncerTasks starts push delivery for syncer task messages.
func (n *NatsEngine) SubscribeSyncerTasks(ctx context.Context, handler func(common.TaskHandle)) error {
	n.syncerMu.Lock()
	consumer := n.syncerConsumer
	n.syncerMu.Unlock()

	if consumer == nil {
		return fmt.Errorf("syncer: consumer not initialized")
	}

	consumeCtx, err := consumer.Consume(func(msg jetstream.Msg) {
		handler(NewNatsMessageHandle(msg))
	})
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		consumeCtx.Stop()
		<-consumeCtx.Closed()
	}()
	return nil
}

// LoadSyncCheckpoint reads the latest running checkpoint for one sync task.
func (n *NatsEngine) LoadSyncCheckpoint(ctx context.Context, taskID string) (*syncerconnector.SyncCheckpointState, error) {
	kv, err := n.syncCheckpointStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	entry, err := kv.Get(ctx, syncCheckpointKey(taskID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var state syncerconnector.SyncCheckpointState
	if err = json.Unmarshal(entry.Value(), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveSyncCheckpoint writes the latest running checkpoint for one sync task.
func (n *NatsEngine) SaveSyncCheckpoint(ctx context.Context, taskID string, state syncerconnector.SyncCheckpointState) error {
	kv, err := n.syncCheckpointStore()
	if err != nil {
		return err
	}

	// write to json
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = kv.Put(ctx, syncCheckpointKey(taskID), data)
	return err
}

// DeleteSyncCheckpoint removes the running checkpoint for a completed sync task.
func (n *NatsEngine) DeleteSyncCheckpoint(ctx context.Context, taskID string) error {
	kv, err := n.syncCheckpointStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = kv.Delete(ctx, syncCheckpointKey(taskID))
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil
	}
	return err
}

func (n *NatsEngine) syncCheckpointStore() (jetstream.KeyValue, error) {
	n.syncerMu.Lock()
	defer n.syncerMu.Unlock()
	if err := n.initSyncCheckpointsLocked(); err != nil {
		return nil, err
	}
	return n.syncCheckpointKV, nil
}

func syncCheckpointKey(taskID string) string {
	return "task." + taskID
}
