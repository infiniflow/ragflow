package connector

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestGoogleCloudStorageConnectorOpenSyncUsesFingerprintAndFetch(t *testing.T) {
	old := mustTime(t, "2026-01-01T00:00:00Z")
	updated := mustTime(t, "2026-01-03T00:00:00Z")
	connector := newTestGoogleCloudStorageConnector(t, []googleCloudStorageObject{
		{Key: "docs/old.txt", LastModified: old, Size: 8, ETag: `"old-etag"`},
		{Key: "docs/new.txt", LastModified: updated, Size: 8, ETag: `"new-etag"`},
	})

	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Fingerprints: map[string]string{
			googleCloudStorageSourceID("bucket", "docs/old.txt"): normalizedGoogleCloudStorageETag(`"old-etag"`),
		},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != googleCloudStorageSourceID("bucket", "docs/new.txt") {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "new.txt" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if doc.Fingerprint != normalizedGoogleCloudStorageETag(`"new-etag"`) {
		t.Fatalf("fingerprint = %q", doc.Fingerprint)
	}
	if doc.FetchRef == nil {
		t.Fatalf("fetch ref is nil")
	}
	fetcher, ok := session.(Fetcher)
	if !ok {
		t.Fatalf("session does not implement Fetcher")
	}
	blob, err := fetcher.Fetch(t.Context(), *doc.FetchRef)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(blob) != "body:new" {
		t.Fatalf("blob = %q", blob)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestGoogleCloudStorageConnectorOpenSyncDefersListingUntilNextBatch(t *testing.T) {
	connector := newTestGoogleCloudStorageConnector(t, []googleCloudStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})
	var listCalls int
	baseListObjects := connector.listObjects
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]googleCloudStorageObject, string, bool, error) {
		listCalls++
		return baseListObjects(ctx, startAfter, maxKeys)
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	if listCalls != 0 {
		t.Fatalf("OpenSync list calls = %d, want 0", listCalls)
	}
	if _, err := session.NextBatch(context.Background()); err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("NextBatch list calls = %d, want 1", listCalls)
	}
}

func TestGoogleCloudStorageConnectorOpenSyncFiltersImagesUnlessAllowed(t *testing.T) {
	objects := []googleCloudStorageObject{
		{Key: "docs/a.png", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
	}
	connector := newTestGoogleCloudStorageConnector(t, objects)
	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != googleCloudStorageSourceID("bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}

	allowed := newTestGoogleCloudStorageConnector(t, objects)
	allowed.allowImages = true
	session, err = allowed.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("allow images OpenSync: %v", err)
	}
	batch, err = session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("allow images NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("allow images documents len = %d, want 2", len(batch.Documents))
	}
}

func TestGoogleCloudStorageConnectorOpenSyncResumesFromCheckpoint(t *testing.T) {
	connector := newTestGoogleCloudStorageConnector(t, []googleCloudStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})

	session, err := connector.OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:   googleCloudStorageSourceID("bucket", "docs/b.txt"),
			SourceID: googleCloudStorageSourceID("bucket", "docs/b.txt"),
		},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != googleCloudStorageSourceID("bucket", "docs/c.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestGoogleCloudStorageConnectorOpenSyncResumeRejectsMissingAnchor(t *testing.T) {
	connector := newTestGoogleCloudStorageConnector(t, []googleCloudStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:   googleCloudStorageSourceID("bucket", "docs/b.txt"),
			SourceID: googleCloudStorageSourceID("bucket", "docs/b.txt"),
		},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestGoogleCloudStorageConnectorOpenSyncResumeRejectsForeignCheckpoint(t *testing.T) {
	connector := newTestGoogleCloudStorageConnector(t, []googleCloudStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor: "other:bucket:docs/a.txt",
		},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestGoogleCloudStorageConnectorOpenPruneReturnsSlimSnapshot(t *testing.T) {
	connector := newTestGoogleCloudStorageConnector(t, []googleCloudStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Key: "docs/folder/", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 0, ETag: "folder"},
	})
	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2", len(batch.Documents))
	}
	if batch.Documents[0].SourceID != googleCloudStorageSourceID("bucket", "docs/a.txt") || batch.Documents[1].SourceID != googleCloudStorageSourceID("bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestGoogleCloudStorageConnectorOpenSyncIncludesMissingFingerprint(t *testing.T) {
	connector := newTestGoogleCloudStorageConnector(t, []googleCloudStorageObject{
		{Key: "docs/no-etag.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1},
	})
	session, err := connector.OpenSync(t.Context(), SyncRequest{
		Fingerprints: map[string]string{"other": "fingerprint"},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
}

func TestReadGoogleCloudStorageBodySizeThreshold(t *testing.T) {
	_, err := readGoogleCloudStorageBody(bytes.NewBufferString("12345"), "large.txt", 4)
	if err == nil {
		t.Fatalf("expected size threshold error")
	}
}

func newTestGoogleCloudStorageConnector(t *testing.T, objects []googleCloudStorageObject) *GoogleCloudStorageConnector {
	t.Helper()
	connector, err := NewGoogleCloudStorageConnector(map[string]any{
		"bucket_name": "bucket",
		"prefix":      "docs",
		"batch_size":  2,
		"credentials": map[string]any{
			"access_key_id":     "access",
			"secret_access_key": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewGoogleCloudStorageConnector: %v", err)
	}
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]googleCloudStorageObject, string, bool, error) {
		var out []googleCloudStorageObject
		for _, object := range objects {
			if startAfter != "" && object.Key <= startAfter {
				continue
			}
			out = append(out, object)
			if maxKeys > 0 && len(out) >= int(maxKeys) {
				break
			}
		}
		hasMore := false
		nextStartAfter := ""
		if len(out) > 0 {
			nextStartAfter = googleCloudStorageSourceID(connector.bucketName, out[len(out)-1].Key)
			for _, object := range objects {
				if object.Key > out[len(out)-1].Key {
					hasMore = true
					break
				}
			}
		}
		return out, nextStartAfter, hasMore, nil
	}
	connector.downloadObject = func(ctx context.Context, key string, sizeThreshold int64) ([]byte, error) {
		if key == "docs/new.txt" {
			return []byte("body:new"), nil
		}
		return []byte("body:" + key), nil
	}
	return connector
}
