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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// SyncerTaskSubject is the JetStream subject carrying sync_logs task IDs.
	SyncerTaskSubject = "sync.tasks.RAGFLOW"

	syncerStreamName     = "RAGFLOW_SYNC_TASKS"
	syncerConsumerName   = "RAGFLOW_SYNCER_CONSUMER"
	syncerSubjectPattern = "sync.tasks.>"
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
	stream, err := n.jetStream.CreateStream(ctx, jetstream.StreamConfig{
		Name:       syncerStreamName,
		Subjects:   []string{syncerSubjectPattern},
		Retention:  jetstream.WorkQueuePolicy,
		Storage:    jetstream.FileStorage,
		MaxMsgs:    1024 * 128,
		MaxBytes:   1024 * 1024 * 64,
		Duplicates: 10 * time.Minute,
	})
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("syncer: create stream: %w", err)
		}
		stream, err = n.jetStream.Stream(ctx, syncerStreamName)
		if err != nil {
			return fmt.Errorf("syncer: get existing stream: %w", err)
		}
	}
	n.syncerStream = stream
	return nil
}

// InitSyncerConsumer creates the durable pull consumer for syncer tasks.
func (n *NatsEngine) InitSyncerConsumer() error {
	n.syncerMu.Lock()
	defer n.syncerMu.Unlock()
	if err := n.initSyncerStreamLocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	consumer, err := n.syncerStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          syncerConsumerName,
		Durable:       syncerConsumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    16,
		MaxAckPending: 1024 * 128,
		FilterSubject: syncerSubjectPattern,
	})
	if err != nil {
		if strings.Contains(err.Error(), "max waiting can not be updated") {
			consumer, err = n.syncerStream.Consumer(ctx, syncerConsumerName)
			if err != nil {
				return fmt.Errorf("syncer: get existing consumer: %w", err)
			}
		} else {
			return fmt.Errorf("syncer: create consumer: %w", err)
		}
	}
	n.syncerConsumer = consumer
	return nil
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

// FetchSyncerTasks pulls syncer task messages from JetStream.
func (n *NatsEngine) FetchSyncerTasks(batchSize int) ([]common.TaskHandle, error) {
	n.syncerMu.Lock()
	consumer := n.syncerConsumer
	n.syncerMu.Unlock()
	if consumer == nil {
		return nil, fmt.Errorf("syncer: consumer not initialized")
	}
	// fetch task from nats(jetStream)
	messages, err := consumer.Fetch(batchSize, jetstream.FetchMaxWait(1*time.Second))
	if err != nil {
		if errors.Is(err, nats.ErrTimeout) {
			return nil, nil
		}
		return nil, err
	}

	handles := make([]common.TaskHandle, 0, batchSize)
	for msg := range messages.Messages() {
		handles = append(handles, NewNatsMessageHandle(msg))
	}
	if err = messages.Error(); err != nil {
		return handles, err
	}
	return handles, nil
}
