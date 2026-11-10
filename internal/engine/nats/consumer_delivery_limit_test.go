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
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// The redelivery budget must report the limit the broker enforces. A consumer
// created by an older release can carry a different MaxDeliver, and
// ensureConsumer reattaches to it when the MaxWaiting mismatch blocks the
// update, so the local default would be wrong.
func TestDeliveryLimitFollowsExistingConsumer(t *testing.T) {
	host, port := newEmbeddedNatsServer(t)
	engine := NewNatsEngine(host, port)
	if err := engine.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := tierStreams[defaultTier()]
	if _, err := engine.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          spec.consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
		MaxWaiting:    4096,
		FilterSubject: spec.subjects[0],
	}); err != nil {
		t.Fatalf("seed consumer: %v", err)
	}

	if err := engine.InitConsumer(spec.subjects[0]); err != nil {
		t.Fatalf("InitConsumer: %v", err)
	}
	if got := engine.deliveryLimit(defaultTier()); got != 3 {
		t.Fatalf("deliveryLimit = %d, want the existing consumer's 3", got)
	}
}
