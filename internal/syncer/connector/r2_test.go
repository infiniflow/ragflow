package connector

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestR2ConnectorOpenSyncUsesFingerprintAndFetch(t *testing.T) {
	old := mustTime(t, "2026-01-01T00:00:00Z")
	updated := mustTime(t, "2026-01-03T00:00:00Z")
	connector := newTestR2Connector(t, []r2Object{
		{Key: "docs/old.txt", LastModified: old, Size: 8, ETag: `"old-etag"`},
		{Key: "docs/new.txt", LastModified: updated, Size: 8, ETag: `"new-etag"`},
	})

	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Fingerprints: map[string]string{
			r2SourceID("bucket", "docs/old.txt"): normalizedR2ETag(`"old-etag"`),
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
	if doc.SourceID != r2SourceID("bucket", "docs/new.txt") {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "new.txt" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if doc.Fingerprint != normalizedR2ETag(`"new-etag"`) {
		t.Fatalf("fingerprint = %q", doc.Fingerprint)
	}
	if doc.FetchRef == nil {
		t.Fatalf("fetch ref is nil")
	}
	fetcher, ok := session.(Fetcher)
	if !ok {
		t.Fatalf("session does not implement Fetcher")
	}
	blob, err := fetcher.Fetch(context.Background(), *doc.FetchRef)
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

func TestR2ConnectorOpenSyncDefersListingUntilNextBatch(t *testing.T) {
	connector := newTestR2Connector(t, []r2Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})
	var listCalls int
	baseListObjects := connector.listObjects
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]r2Object, string, bool, error) {
		listCalls++
		return baseListObjects(ctx, startAfter, maxKeys)
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
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

func TestR2ConnectorOpenSyncFiltersImagesUnlessAllowed(t *testing.T) {
	objects := []r2Object{
		{Key: "docs/a.png", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
	}
	connector := newTestR2Connector(t, objects)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != r2SourceID("bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}

	allowed := newTestR2Connector(t, objects)
	allowed.allowImages = true
	session, err = allowed.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
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

func TestR2ConnectorOpenSyncResumesFromCheckpoint(t *testing.T) {
	objects := []r2Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	}
	connector := newTestR2Connector(t, objects)

	checkpointUpdatedAt := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:    r2SourceID("bucket", "docs/b.txt"),
			SourceID:  r2SourceID("bucket", "docs/b.txt"),
			UpdatedAt: &checkpointUpdatedAt,
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
	if batch.Documents[0].SourceID != r2SourceID("bucket", "docs/c.txt") {
		t.Fatalf("resumed source id = %q, want docs/c.txt", batch.Documents[0].SourceID)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.Cursor != r2SourceID("bucket", "docs/c.txt") {
		t.Fatalf("checkpoint = %+v", batch.Checkpoint)
	}
}

func TestR2ConnectorOpenSyncResumeRejectsForeignCheckpoint(t *testing.T) {
	connector := newTestR2Connector(t, []r2Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor: "other:bucket:docs/a.txt",
		},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestR2ConnectorOpenSyncResumeRejectsMissingAnchor(t *testing.T) {
	connector := newTestR2Connector(t, []r2Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:   r2SourceID("bucket", "docs/b.txt"),
			SourceID: r2SourceID("bucket", "docs/b.txt"),
		},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestR2ConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector := newTestR2Connector(t, []r2Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestR2ConnectorOpenPruneReturnsSlimSnapshot(t *testing.T) {
	connector := newTestR2Connector(t, []r2Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Key: "docs/folder/", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 0, ETag: "folder"},
	})
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
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
	if batch.Documents[0].SourceID != r2SourceID("bucket", "docs/a.txt") || batch.Documents[1].SourceID != r2SourceID("bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestR2ConnectorOpenSyncIncludesMissingFingerprint(t *testing.T) {
	connector := newTestR2Connector(t, []r2Object{
		{Key: "docs/no-etag.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1},
	})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
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

func TestR2ConnectorValidate(t *testing.T) {
	ctx := context.Background()
	_, err := NewR2Connector(map[string]any{"credentials": map[string]any{}})
	if err != nil {
		t.Fatalf("NewR2Connector: %v", err)
	}

	missingBucket := &R2Connector{accountID: "account", accessKeyID: "access", secretKey: "secret", batchSize: 2}
	if err := missingBucket.Validate(ctx); err == nil {
		t.Fatalf("expected bucket name validation error")
	}

	missingCreds := &R2Connector{bucketName: "bucket", batchSize: 2}
	if err := missingCreds.Validate(ctx); err == nil {
		t.Fatalf("expected credentials validation error")
	}

	badBatch := &R2Connector{bucketName: "bucket", accountID: "account", accessKeyID: "access", secretKey: "secret", batchSize: 0}
	if err := badBatch.Validate(ctx); err == nil {
		t.Fatalf("expected batch size validation error")
	}
}

func TestR2Endpoint(t *testing.T) {
	if got := r2Endpoint("myaccount", false); got != "https://myaccount.r2.cloudflarestorage.com" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := r2Endpoint("myaccount", true); got != "https://myaccount.eu.r2.cloudflarestorage.com" {
		t.Fatalf("eu endpoint = %q", got)
	}
}

func TestNewR2ConnectorDefaultsAndConfig(t *testing.T) {
	connector, err := NewR2Connector(map[string]any{
		"bucket_name": " bucket ",
		"prefix":      "docs",
		"credentials": map[string]any{
			"account_id":           "myaccount",
			"r2_access_key_id":     "access",
			"r2_secret_access_key": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewR2Connector: %v", err)
	}
	if connector.bucketName != "bucket" {
		t.Fatalf("bucket name = %q", connector.bucketName)
	}
	if connector.prefix != "docs/" {
		t.Fatalf("prefix = %q", connector.prefix)
	}
	if connector.batchSize != defaultR2BatchSize {
		t.Fatalf("batch size = %d, want %d", connector.batchSize, defaultR2BatchSize)
	}
	if connector.sizeThreshold != defaultR2SizeThreshold {
		t.Fatalf("size threshold = %d, want %d", connector.sizeThreshold, defaultR2SizeThreshold)
	}
	if connector.accountID != "myaccount" || connector.accessKeyID != "access" || connector.secretKey != "secret" {
		t.Fatalf("credentials = %+v", connector)
	}
	if got := r2ConsoleURL(connector.accountID, connector.europeanResidency, connector.bucketName, "docs/a b.txt"); !strings.Contains(got, "https://dash.cloudflare.com/myaccount/r2/default/buckets/bucket/objects/docs/a%20b.txt/details") {
		t.Fatalf("console url = %q", got)
	}
}

func TestReadR2BodySizeThreshold(t *testing.T) {
	_, err := readR2Body(bytes.NewBufferString("12345"), "large.txt", 4)
	if err == nil {
		t.Fatalf("expected size threshold error")
	}
}

func newTestR2Connector(t *testing.T, objects []r2Object) *R2Connector {
	t.Helper()
	connector, err := NewR2Connector(map[string]any{
		"bucket_name": "bucket",
		"prefix":      "docs",
		"batch_size":  2,
		"credentials": map[string]any{
			"account_id":           "myaccount",
			"r2_access_key_id":     "access",
			"r2_secret_access_key": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewR2Connector: %v", err)
	}
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]r2Object, string, bool, error) {
		var out []r2Object
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
			nextStartAfter = r2SourceID(connector.bucketName, out[len(out)-1].Key)
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
