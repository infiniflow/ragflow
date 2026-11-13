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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
)

type materializeCache struct {
	data []byte
	err  error
	get  int
}

func (c *materializeCache) Put(_ context.Context, format, mime string, data []byte) (entity.MediaRef, error) {
	if c.err != nil && !errors.Is(c.err, media.ErrMediaNotFound) {
		return entity.MediaRef{}, c.err
	}
	c.data = append([]byte(nil), data...)
	sum := sha256.Sum256(data)
	return entity.MediaRef{Format: format, Kind: entity.MediaKindKV, Scope: "run-1", SHA256: hex.EncodeToString(sum[:]), MIME: mime, Size: int64(len(data))}, nil
}

func (c *materializeCache) Get(_ context.Context, ref entity.MediaRef) ([]byte, error) {
	c.get++
	if c.err != nil {
		return nil, c.err
	}
	return media.ValidatePayload(c.data, ref)
}

func cacheMediaRef() entity.MediaRef {
	sum := sha256.Sum256([]byte("hello"))
	return entity.MediaRef{
		Format: "html",
		Kind:   entity.MediaKindKV,
		Scope:  "run-1",
		SHA256: hex.EncodeToString(sum[:]),
		MIME:   "image/png",
		Size:   5,
	}
}

func TestResolveMediaRefReturnsDecodedCacheBytes(t *testing.T) {
	cache := &materializeCache{data: []byte("hello")}
	ctx := media.WithCache(context.Background(), cache)
	data, ref, err := ResolveMediaRef(ctx, map[string]any{"media_ref": cacheMediaRef()})
	if err != nil {
		t.Fatalf("ResolveMediaRef: %v", err)
	}
	if ref == nil || ref.Kind != entity.MediaKindKV || string(data) != "hello" {
		t.Fatalf("ResolveMediaRef = ref %+v, data %q", ref, data)
	}
}

func TestResolveMediaRefRejectsUnsupportedKindAndCorruptBytes(t *testing.T) {
	supported := cacheMediaRef()
	unsupported := supported
	unsupported.Kind = "data-uri"
	ctx := media.WithCache(context.Background(), &materializeCache{data: []byte("hello")})
	if _, _, err := ResolveMediaRef(ctx, map[string]any{"media_ref": unsupported}); err == nil {
		t.Fatal("ResolveMediaRef accepted the removed data-uri source path")
	}

	ctx = media.WithCache(context.Background(), &materializeCache{data: []byte("HELLO")})
	if _, _, err := ResolveMediaRef(ctx, map[string]any{"media_ref": supported}); err == nil {
		t.Fatal("ResolveMediaRef accepted bytes that do not match the reference")
	}
}

func TestMaterializeMediaItemUsesRunCache(t *testing.T) {
	cache := &materializeCache{data: []byte("hello")}
	ctx := media.WithCache(context.Background(), cache)
	ref := cacheMediaRef()
	m := map[string]any{"media_ref": ref}
	if err := MaterializeMediaItem(ctx, m); err != nil {
		t.Fatalf("MaterializeMediaItem: %v", err)
	}
	if cache.get != 1 {
		t.Fatalf("cache Get calls = %d, want 1", cache.get)
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("hello"))
	if m["image"] != want {
		t.Fatalf("image = %v, want %q", m["image"], want)
	}
}

func TestMaterializeMediaItemPropagatesCacheMiss(t *testing.T) {
	cache := &materializeCache{err: media.ErrMediaNotFound}
	ctx := media.WithCache(context.Background(), cache)
	m := map[string]any{"media_ref": cacheMediaRef()}
	if err := MaterializeMediaItem(ctx, m); !errors.Is(err, media.ErrMediaNotFound) {
		t.Fatalf("MaterializeMediaItem error = %v, want cache miss", err)
	}
}

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
	if err := MaterializeMediaItem(context.Background(), m); err == nil {
		t.Fatal("a present but unusable media_ref was treated as absent")
	}
}

func TestMaterializeMediaItemTreatsAbsentMediaRefAsNoop(t *testing.T) {
	var nilRef *entity.MediaRef
	zero := &entity.MediaRef{}
	for _, value := range []any{nil, "", map[string]any{}, nilRef, zero, entity.MediaRef{}} {
		m := map[string]any{"media_ref": value}
		if err := MaterializeMediaItem(context.Background(), m); err != nil {
			t.Errorf("media_ref %#v: %v", value, err)
		}
	}
}

func TestMaterializeMediaItemRequiresRunCacheForPresentRef(t *testing.T) {
	m := map[string]any{"media_ref": map[string]any{
		"format": "html",
		"kind":   "kvrocks",
		"scope":  "run-1",
		"sha256": strings.Repeat("a", 64),
	}}
	if err := MaterializeMediaItem(context.Background(), m); err == nil {
		t.Fatal("a present reference without a run cache was treated as absent")
	}
}
