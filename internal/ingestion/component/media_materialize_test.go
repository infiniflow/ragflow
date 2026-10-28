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

package component

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
)

func TestEncodeMediaDataURIRoundTrip(t *testing.T) {
	data := []byte("hello world")
	uri := encodeMediaDataURI("image/png", data)
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(uri, prefix) {
		t.Fatalf("uri = %q", uri)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, prefix))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != string(data) {
		t.Fatalf("decoded = %q, want %q", decoded, data)
	}
}

func TestMaterializeMediaItemRejectsInvalidMediaRef(t *testing.T) {
	m := map[string]any{"media_ref": map[string]any{"format": "html"}}
	src := media.MediaSourceFunc(func(context.Context) ([]byte, error) { return nil, nil })
	if err := MaterializeMediaItem(context.Background(), m, media.NewMediaResolver(), src); err == nil {
		t.Fatal("a present but unusable media_ref was treated as absent")
	}
}

func TestMaterializeMediaItemTreatsAbsentMediaRefAsNoop(t *testing.T) {
	var nilRef *entity.MediaRef
	zero := &entity.MediaRef{}
	for _, value := range []any{nil, "", map[string]any{}, nilRef, zero, entity.MediaRef{}} {
		m := map[string]any{"media_ref": value}
		src := media.MediaSourceFunc(func(context.Context) ([]byte, error) { return nil, nil })
		if err := MaterializeMediaItem(context.Background(), m, media.NewMediaResolver(), src); err != nil {
			t.Errorf("media_ref %#v: %v", value, err)
		}
	}
}
