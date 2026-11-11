//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
	"errors"
	"testing"

	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type fakeJetStreamMsg struct {
	jetstream.Msg
	nakCalled bool
}

func (f *fakeJetStreamMsg) Data() []byte            { return []byte("{}") }
func (f *fakeJetStreamMsg) Headers() natslib.Header { return nil }
func (f *fakeJetStreamMsg) Subject() string         { return "tasks.RAGFLOW" }
func (f *fakeJetStreamMsg) Reply() string           { return "" }
func (f *fakeJetStreamMsg) Nak() error              { f.nakCalled = true; return nil }

func (f *fakeJetStreamMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{}, nil
}

type fakeMessageBatch struct {
	msgs []jetstream.Msg
	err  error
}

func (b *fakeMessageBatch) Messages() <-chan jetstream.Msg {
	ch := make(chan jetstream.Msg, len(b.msgs))
	for _, m := range b.msgs {
		ch <- m
	}
	close(ch)
	return ch
}

func (b *fakeMessageBatch) Error() error { return b.err }

type fakePullConsumer struct {
	jetstream.Consumer
	batch jetstream.MessageBatch
}

func (c *fakePullConsumer) Fetch(int, ...jetstream.FetchOpt) (jetstream.MessageBatch, error) {
	return c.batch, nil
}

// A fetch canceled mid-batch still delivers messages. pullLane drops the
// handles on the error, so fetchOnce must nack them: leaving them in
// AckPending would hold each one until the server's redelivery backoff
// (5s..60s) expires instead of making it available right away.
func TestFetchOnceNacksDeliveredMessagesOnCanceledBatch(t *testing.T) {
	msg := &fakeJetStreamMsg{}
	engine := &NatsEngine{}
	consumer := &fakePullConsumer{batch: &fakeMessageBatch{
		msgs: []jetstream.Msg{msg},
		err:  context.Canceled,
	}}

	handles, err := engine.fetchOnce(context.Background(), consumer, 4)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(handles) != 0 {
		t.Fatalf("handles = %d, want none: the caller drops them on cancellation", len(handles))
	}
	if !msg.nakCalled {
		t.Fatal("delivered message was not nacked; it would strand in AckPending until the redelivery backoff expires")
	}
}

func TestFetchOnceStillNacksOnOtherBatchErrors(t *testing.T) {
	msg := &fakeJetStreamMsg{}
	engine := &NatsEngine{}
	consumer := &fakePullConsumer{batch: &fakeMessageBatch{
		msgs: []jetstream.Msg{msg},
		err:  errors.New("nats: fetch failed"),
	}}

	if _, err := engine.fetchOnce(context.Background(), consumer, 4); err == nil {
		t.Fatal("err = nil, want the batch error")
	}
	if !msg.nakCalled {
		t.Fatal("delivered message was not nacked on a plain batch failure")
	}
}
