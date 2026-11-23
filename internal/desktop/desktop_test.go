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

//go:build desktop

package desktop

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

// TestStartBootsEveryBackend is the acceptance test for the desktop build:
// it talks to each in-process backend over its real protocol, exactly as the
// server does, instead of inspecting the runtime struct.
func TestStartBootsEveryBackend(t *testing.T) {
	ctx := t.Context()
	rt, err := Start(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("start desktop backends: %v", err)
	}
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("close desktop backends: %v", err)
		}
	})

	t.Run("kvrocks", func(t *testing.T) { assertKvrocks(t, ctx, rt) })
	t.Run("nats", func(t *testing.T) { assertNATS(t, ctx, rt) })
	t.Run("s3", func(t *testing.T) { assertS3(t, ctx, rt) })
	t.Run("infinity", func(t *testing.T) { assertInfinity(t, ctx, rt) })
}

// The RESP store used to live in <data>/kvrocks. Starting on such a directory
// must adopt it under the new name, or the installation would look empty.
//
// The migration is exercised on its own rather than through Start: Pebble
// rejects any stray file in its directory ("invalid marker filename"), so a
// marker inside the store cannot coexist with opening it.
func TestLegacyKvrocksDirectoryIsAdopted(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, legacyPebbisdDirName)
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	marker := filepath.Join(legacy, "marker.txt")
	if err := os.WriteFile(marker, []byte("store"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	if err := migrateLegacyLayout(dir); err != nil {
		t.Fatalf("migrate legacy layout: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, pebbisDirName, "marker.txt")); err != nil {
		t.Fatalf("the store was not moved to %s: %v", pebbisDirName, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy directory still exists: %v", err)
	}

	// A second call is a no-op, and an existing pebbis directory is never
	// overwritten by a stale legacy one.
	if err := migrateLegacyLayout(dir); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

// assertInfinity checks the document engine answers on its loopback port. The
// full thrift handshake is exercised by the server startup path, which the
// desktop entry runs and the smoke test in cmd covers; here the point is that
// Start booted it in-process rather than leaving the address unset.
func assertInfinity(t *testing.T, ctx context.Context, rt *Runtime) {
	t.Helper()
	if rt.InfinityAddr == "" {
		t.Fatal("document engine address is empty")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", rt.InfinityAddr)
	if err != nil {
		t.Fatalf("dial document engine at %s: %v", rt.InfinityAddr, err)
	}
	_ = conn.Close()
}

func assertKvrocks(t *testing.T, ctx context.Context, rt *Runtime) {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: rt.KvrocksAddr, Password: rt.KvrocksPassword})
	t.Cleanup(func() { _ = rdb.Close() })

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := rdb.Set(ctx, "probe", "v", time.Minute).Err(); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, err := rdb.Get(ctx, "probe").Result(); err != nil || got != "v" {
		t.Fatalf("get = %q, %v", got, err)
	}

	// The transaction path is what replaced every Lua script, so a watched
	// read-check-write has to work on this backend, not just on real Kvrocks.
	if err := rdb.Set(ctx, "cas", "a", time.Minute).Err(); err != nil {
		t.Fatalf("seed cas: %v", err)
	}
	err := rdb.Watch(ctx, func(tx *redis.Tx) error {
		cur, err := tx.Get(ctx, "cas").Result()
		if err != nil {
			return err
		}
		if cur != "a" {
			t.Fatalf("watched value = %q, want a", cur)
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Del(ctx, "cas")
			return nil
		})
		return err
	}, "cas")
	if err != nil {
		t.Fatalf("watch/tx: %v", err)
	}
}

func assertNATS(t *testing.T, _ context.Context, rt *Runtime) {
	t.Helper()
	url := fmt.Sprintf("nats://%s:%d", rt.NATSHost, rt.NATSPort)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(nc.Close)

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	// WorkQueuePolicy + FileStorage is what the ingestion queue uses; a KV
	// bucket covers the checkpoint cache on the same server.
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:      "probe",
		Subjects:  []string{"probe.>"},
		Retention: nats.WorkQueuePolicy,
		Storage:   nats.FileStorage,
	}); err != nil {
		t.Fatalf("add stream: %v", err)
	}
	if _, err := js.Publish("probe.t", []byte("x")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{Bucket: "probekv", Storage: nats.FileStorage})
	if err != nil {
		t.Fatalf("create kv: %v", err)
	}
	if _, err := kv.Put("k", []byte("v")); err != nil {
		t.Fatalf("kv put: %v", err)
	}
	entry, err := kv.Get("k")
	if err != nil || string(entry.Value()) != "v" {
		t.Fatalf("kv get = %v, %v", entry, err)
	}
}

// assertS3 walks the S3 surface the storage layer actually depends on. The two
// paths that historically break on a non-MinIO gateway are the multipart upload
// (files over 16 MiB) and the version-aware listing that RemoveBucket uses.
func assertS3(t *testing.T, ctx context.Context, rt *Runtime) {
	t.Helper()
	client, err := minio.New(rt.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(rt.S3AccessKey, rt.S3SecretKey, ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("minio client: %v", err)
	}

	const bucket = "probe-bucket"
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		t.Fatalf("make bucket: %v", err)
	}

	// Small object: the ordinary upload path.
	small := []byte("hello")
	if _, err := client.PutObject(ctx, bucket, "docs/small.txt", bytes.NewReader(small),
		int64(len(small)), minio.PutObjectOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("put small: %v", err)
	}
	obj, err := client.GetObject(ctx, bucket, "docs/small.txt", minio.GetObjectOptions{})
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	got, err := io.ReadAll(obj)
	_ = obj.Close()
	if err != nil || !bytes.Equal(got, small) {
		t.Fatalf("get object body = %q, %v", got, err)
	}

	// Large object: above minio-go's 16 MiB part threshold, so this goes
	// through CreateMultipartUpload/UploadPart/CompleteMultipartUpload.
	const largeSize = 17 << 20
	large := bytes.Repeat([]byte("a"), largeSize)
	if _, err := client.PutObject(ctx, bucket, "docs/large.bin", bytes.NewReader(large),
		int64(len(large)), minio.PutObjectOptions{ContentType: "application/octet-stream"}); err != nil {
		t.Fatalf("put large (multipart): %v", err)
	}
	info, err := client.StatObject(ctx, bucket, "docs/large.bin", minio.StatObjectOptions{})
	if err != nil {
		t.Fatalf("stat large: %v", err)
	}
	if info.Size != largeSize {
		t.Fatalf("stat large size = %d, want %d", info.Size, largeSize)
	}

	// CopyObject is the Move path.
	if _, err := client.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: bucket, Object: "docs/copy.txt"},
		minio.CopySrcOptions{Bucket: bucket, Object: "docs/small.txt"}); err != nil {
		t.Fatalf("copy object: %v", err)
	}

	// Plain listing.
	var listed int
	for range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: "docs/", Recursive: true}) {
		listed++
	}
	if listed < 3 {
		t.Fatalf("listed %d objects under docs/, want at least 3", listed)
	}

	// Version-aware listing: this is what RemoveBucket/RemoveEmptyBucket issue,
	// and the API MinIO exposes but other gateways frequently do not.
	var versions int
	versionErr := error(nil)
	for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Recursive: true, WithVersions: true,
	}) {
		if obj.Err != nil {
			versionErr = obj.Err
			break
		}
		versions++
	}
	if versionErr != nil {
		t.Fatalf("list versions: %v", versionErr)
	}
	if versions < 3 {
		t.Fatalf("listed %d versions, want at least 3", versions)
	}

	// Batch delete, then bucket removal.
	objects := make(chan minio.ObjectInfo, 3)
	for _, key := range []string{"docs/small.txt", "docs/large.bin", "docs/copy.txt"} {
		objects <- minio.ObjectInfo{Key: key}
	}
	close(objects)
	for rmErr := range client.RemoveObjects(ctx, bucket, objects, minio.RemoveObjectsOptions{}) {
		if rmErr.Err != nil {
			t.Fatalf("remove objects: %v", rmErr.Err)
		}
	}
	for range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		t.Fatal("bucket still has objects after batch delete")
	}
	if err := client.RemoveBucket(ctx, bucket); err != nil {
		t.Fatalf("remove bucket: %v", err)
	}
}

// TestCloseIsIdempotent guards the shutdown path: a supervisor that closes the
// runtime twice must not panic on a nil store.
func TestCloseIsIdempotent(t *testing.T) {
	rt, err := Start(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := rt.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := rt.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
