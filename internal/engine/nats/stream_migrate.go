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
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

// ensureStreamConfig creates the stream, or - when it already exists with an
// older configuration (e.g. created by a previous deployment) - migrates it in
// place via UpdateStream instead of failing with "stream name already in
// use". Only the capacity/dedup/discard fields (MaxBytes, MaxMsgs, Duplicates,
// Discard) are migrated; the server-side current config is the merge base so a
// partial update can never reset fields this helper does not own (Subjects,
// Retention, Storage, ...).
func ensureStreamConfig(ctx context.Context, js jetstream.JetStream, want jetstream.StreamConfig) (jetstream.Stream, error) {
	st, err := js.CreateStream(ctx, want)
	if err == nil {
		return st, nil
	}
	// Match the API error code, not a message substring: the server reports
	// 10058 ("stream name already in use"), which a naive "already exists"
	// contains-check never matches.
	var jsErr jetstream.JetStreamError
	if !errors.As(err, &jsErr) || jsErr.APIError() == nil || jsErr.APIError().ErrorCode != jetstream.JSErrCodeStreamNameInUse {
		return nil, err
	}
	st, err = js.Stream(ctx, want.Name)
	if err != nil {
		return nil, err
	}
	cur := st.CachedInfo().Config
	needUpdate := cur.MaxBytes != want.MaxBytes || cur.MaxMsgs != want.MaxMsgs
	if want.Duplicates != 0 && cur.Duplicates != want.Duplicates {
		needUpdate = true
	}
	if want.Discard != jetstream.DiscardOld && cur.Discard != want.Discard {
		needUpdate = true
	}
	if needUpdate {
		merged := cur
		merged.MaxBytes = want.MaxBytes
		merged.MaxMsgs = want.MaxMsgs
		if want.Duplicates != 0 {
			merged.Duplicates = want.Duplicates
		}
		if want.Discard != jetstream.DiscardOld {
			merged.Discard = want.Discard
		}
		st, err = js.UpdateStream(ctx, merged)
		if err != nil {
			return nil, err
		}
	}
	return st, nil
}
