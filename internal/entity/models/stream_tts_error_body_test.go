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

package models

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestReadStreamTTSErrorBody(t *testing.T) {
	t.Run("short body is captured whole", func(t *testing.T) {
		body := strings.NewReader(strings.Repeat("a", 300))
		head, err := readStreamTTSErrorBody(body)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if len(head) != 300 {
			t.Fatalf("len(head) = %d, want 300", len(head))
		}
		if body.Len() != 0 {
			t.Fatalf("remaining bytes = %d, want 0", body.Len())
		}
	})

	t.Run("large body is captured to the head limit and drained to the drain limit", func(t *testing.T) {
		const bodyBytes = 10000
		body := strings.NewReader(strings.Repeat("a", bodyBytes))
		head, err := readStreamTTSErrorBody(body)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if len(head) != streamTTSErrorBodyBytes {
			t.Fatalf("len(head) = %d, want %d", len(head), streamTTSErrorBodyBytes)
		}
		wantRemaining := int64(bodyBytes - streamTTSErrorBodyBytes - streamTTSErrorBodyDrainBytes)
		if int64(body.Len()) != wantRemaining {
			t.Fatalf("remaining bytes = %d, want %d", body.Len(), wantRemaining)
		}
	})

	t.Run("head read failure is returned with the captured bytes", func(t *testing.T) {
		errBoom := errors.New("boom")
		head, err := readStreamTTSErrorBody(iotest.ErrReader(errBoom))
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want %v", err, errBoom)
		}
		if len(head) != 0 {
			t.Fatalf("len(head) = %d, want 0", len(head))
		}
	})

	t.Run("drain read failure is returned with the captured bytes", func(t *testing.T) {
		errBoom := errors.New("boom")
		body := io.MultiReader(
			strings.NewReader(strings.Repeat("a", streamTTSErrorBodyBytes)),
			iotest.ErrReader(errBoom),
		)
		head, err := readStreamTTSErrorBody(body)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want %v", err, errBoom)
		}
		if len(head) != streamTTSErrorBodyBytes {
			t.Fatalf("len(head) = %d, want %d", len(head), streamTTSErrorBodyBytes)
		}
	})
}
