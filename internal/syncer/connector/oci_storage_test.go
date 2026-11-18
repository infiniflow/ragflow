package connector

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestOCIStorageConnectorOpenSyncUsesFingerprintAndFetch(t *testing.T) {
	old := mustTime(t, "2026-01-01T00:00:00Z")
	updated := mustTime(t, "2026-01-03T00:00:00Z")
	connector := newTestOCIStorageConnector(t, []ociStorageObject{
		{Key: "docs/old.txt", LastModified: old, Size: 8, ETag: `"old-etag"`},
		{Key: "docs/new.txt", LastModified: updated, Size: 8, ETag: `"new-etag"`},
	})

	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Fingerprints: map[string]string{
			ociStorageSourceID("bucket", "docs/old.txt"): normalizedOCIStorageETag(`"old-etag"`),
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
	if doc.SourceID != ociStorageSourceID("bucket", "docs/new.txt") {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "new.txt" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if doc.Fingerprint != normalizedOCIStorageETag(`"new-etag"`) {
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

func TestOCIStorageConnectorOpenSyncDefersListingUntilNextBatch(t *testing.T) {
	connector := newTestOCIStorageConnector(t, []ociStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})
	var listCalls int
	baseListObjects := connector.listObjects
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]ociStorageObject, string, bool, error) {
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

func TestOCIStorageConnectorOpenSyncFiltersImagesUnlessAllowed(t *testing.T) {
	objects := []ociStorageObject{
		{Key: "docs/a.png", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
	}
	connector := newTestOCIStorageConnector(t, objects)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != ociStorageSourceID("bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}

	allowed := newTestOCIStorageConnector(t, objects)
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

func TestOCIStorageConnectorOpenSyncResumesFromCheckpoint(t *testing.T) {
	connector := newTestOCIStorageConnector(t, []ociStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})

	checkpointUpdatedAt := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:    ociStorageSourceID("bucket", "docs/b.txt"),
			SourceID:  ociStorageSourceID("bucket", "docs/b.txt"),
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
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != ociStorageSourceID("bucket", "docs/c.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestOCIStorageConnectorOpenSyncResumeRejectsInvalidCheckpoints(t *testing.T) {
	connector := newTestOCIStorageConnector(t, []ociStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})

	cases := []*SyncCheckpoint{
		{
			Cursor: "other:bucket:docs/b.txt",
		},
		{
			Cursor: ociStorageSourceID("bucket", "docs/b.txt"),
		},
		{},
	}
	for _, checkpoint := range cases {
		session, err := connector.OpenSync(context.Background(), SyncRequest{
			FromBeginning: true,
			Resume:        checkpoint,
		})
		if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
			t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
		}
	}
}

func TestOCIStorageConnectorOpenSyncIncludesMissingFingerprint(t *testing.T) {
	connector := newTestOCIStorageConnector(t, []ociStorageObject{
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

func TestOCIStorageConnectorOpenSyncFiltersByWindow(t *testing.T) {
	connector := newTestOCIStorageConnector(t, []ociStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "b"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-05T00:00:00Z"), Size: 1, ETag: "c"},
	})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: mustTimePointer(t, "2026-01-02T00:00:00Z"),
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != ociStorageSourceID("bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestOCIStorageConnectorOpenPruneReturnsSlimSnapshot(t *testing.T) {
	connector := newTestOCIStorageConnector(t, []ociStorageObject{
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
	if batch.Documents[0].SourceID != ociStorageSourceID("bucket", "docs/a.txt") || batch.Documents[1].SourceID != ociStorageSourceID("bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestOCIStorageConnectorOpenPruneStreamsAcrossPages(t *testing.T) {
	connector := newTestOCIStorageConnector(t, nil)
	connector.batchSize = 2
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]ociStorageObject, string, bool, error) {
		if startAfter == "" {
			return []ociStorageObject{
				{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
				{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
			}, ociStorageSourceID("bucket", "docs/b.txt"), true, nil
		}
		return []ociStorageObject{
			{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
			{Key: "docs/d.txt", LastModified: mustTime(t, "2026-01-04T00:00:00Z"), Size: 1, ETag: "d"},
		}, "", false, nil
	}

	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 || batch.Documents[0].SourceID != ociStorageSourceID("bucket", "docs/a.txt") || batch.Documents[1].SourceID != ociStorageSourceID("bucket", "docs/b.txt") {
		t.Fatalf("first documents = %+v", batch.Documents)
	}
	batch, err = session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 || batch.Documents[0].SourceID != ociStorageSourceID("bucket", "docs/c.txt") || batch.Documents[1].SourceID != ociStorageSourceID("bucket", "docs/d.txt") {
		t.Fatalf("second documents = %+v", batch.Documents)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestOCIStorageConnectorValidate(t *testing.T) {
	ctx := context.Background()

	missingBucket := &OCIStorageConnector{namespace: "namespace", region: "region", accessKeyID: "access", secretKey: "secret", batchSize: 2}
	if err := missingBucket.Validate(ctx); err == nil {
		t.Fatalf("expected bucket name validation error")
	}

	missingCredentials := &OCIStorageConnector{bucketName: "bucket", batchSize: 2}
	if err := missingCredentials.Validate(ctx); err == nil {
		t.Fatalf("expected credentials validation error")
	}

	badBatch := &OCIStorageConnector{bucketName: "bucket", namespace: "namespace", region: "region", accessKeyID: "access", secretKey: "secret", batchSize: 0}
	if err := badBatch.Validate(ctx); err == nil {
		t.Fatalf("expected batch size validation error")
	}

	badRegion := &OCIStorageConnector{bucketName: "bucket", namespace: "namespace", region: "evil.example#", accessKeyID: "access", secretKey: "secret", batchSize: 2}
	if err := badRegion.Validate(ctx); err == nil {
		t.Fatalf("expected region validation error")
	}
}

func TestOCIStorageConnectorRejectsInvalidNamespaceBeforeClientCreation(t *testing.T) {
	for _, namespace := range []string{
		"namespace.example.com",
		"https://evil.example.org",
		"evil.example.org:443",
		"namespace/tenant",
		"namespace space",
		"-namespace",
		"namespace-",
		"Namespace",
	} {
		connector := newTestOCIStorageConnector(t, nil)
		connector.namespace = namespace
		connector.client = nil
		if _, err := connector.ensureClient(context.Background()); err == nil {
			t.Fatalf("expected invalid namespace %q to be rejected", namespace)
		}
		if connector.client != nil {
			t.Fatalf("client was created for invalid namespace %q", namespace)
		}
	}
}

func TestOCIStorageConnectorRejectsInvalidRegionBeforeClientCreation(t *testing.T) {
	for _, region := range []string{
		"evil.example#",
		"evil.example.com",
		"https://evil.example.org",
		"region/tenant",
		"region space",
		"-region",
		"region-",
		"Region",
	} {
		connector := newTestOCIStorageConnector(t, nil)
		connector.region = region
		connector.client = nil
		if _, err := connector.ensureClient(context.Background()); err == nil {
			t.Fatalf("expected invalid region %q to be rejected", region)
		}
		if connector.client != nil {
			t.Fatalf("client was created for invalid region %q", region)
		}
	}
}

func TestOCIStorageConnectorValidateSetting(t *testing.T) {
	connector := newTestOCIStorageConnector(t, []ociStorageObject{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})
	var listCalls int
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]ociStorageObject, string, bool, error) {
		listCalls++
		return nil, "", false, nil
	}
	if err := connector.ValidateConnectorSetting(context.Background(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting: %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", listCalls)
	}
}

func TestNewOCIStorageConnectorDefaultsAndConfig(t *testing.T) {
	connector, err := NewOCIStorageConnector(map[string]any{
		"bucket_name":    " bucket ",
		"prefix":         "docs",
		"batch_size":     4,
		"size_threshold": 42,
		"allow_images":   true,
		"credentials": map[string]any{
			"namespace":         "mynamespace",
			"region":            "us-ashburn-1",
			"access_key_id":     "access",
			"secret_access_key": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewOCIStorageConnector: %v", err)
	}
	if connector.bucketName != "bucket" {
		t.Fatalf("bucket name = %q", connector.bucketName)
	}
	if connector.prefix != "docs/" {
		t.Fatalf("prefix = %q", connector.prefix)
	}
	if connector.batchSize != 4 {
		t.Fatalf("batch size = %d, want 4", connector.batchSize)
	}
	if connector.sizeThreshold != 42 {
		t.Fatalf("size threshold = %d, want 42", connector.sizeThreshold)
	}
	if !connector.allowImages {
		t.Fatalf("allow images = false, want true")
	}
	if connector.namespace != "mynamespace" || connector.region != "us-ashburn-1" || connector.accessKeyID != "access" || connector.secretKey != "secret" {
		t.Fatalf("credentials = %+v", connector)
	}

	defaults, err := NewOCIStorageConnector(map[string]any{
		"bucket_name": "bucket",
		"credentials": map[string]any{},
	})
	if err != nil {
		t.Fatalf("NewOCIStorageConnector defaults: %v", err)
	}
	if defaults.batchSize != defaultOCIStorageBatchSize || defaults.sizeThreshold != defaultOCIStorageSizeThreshold {
		t.Fatalf("defaults = batch %d threshold %d", defaults.batchSize, defaults.sizeThreshold)
	}
}

func TestOCIStorageHelpers(t *testing.T) {
	if got := ociStorageEndpoint("mynamespace", "us-ashburn-1"); got != "https://mynamespace.compat.objectstorage.us-ashburn-1.oraclecloud.com" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := ociStorageConsoleURL("mynamespace", "us-ashburn-1", "bucket", "docs/a b.txt"); !strings.Contains(got, "https://objectstorage.us-ashburn-1.oraclecloud.com/n/mynamespace/b/bucket/o/docs/a%20b.txt") {
		t.Fatalf("console url = %q", got)
	}
	if _, err := readOCIBody(bytes.NewBufferString("12345"), "large.txt", 4); err == nil {
		t.Fatalf("expected size threshold error")
	}
}

func newTestOCIStorageConnector(t *testing.T, objects []ociStorageObject) *OCIStorageConnector {
	t.Helper()
	connector, err := NewOCIStorageConnector(map[string]any{
		"bucket_name": "bucket",
		"prefix":      "docs",
		"batch_size":  2,
		"credentials": map[string]any{
			"namespace":         "mynamespace",
			"region":            "us-ashburn-1",
			"access_key_id":     "access",
			"secret_access_key": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewOCIStorageConnector: %v", err)
	}
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]ociStorageObject, string, bool, error) {
		var out []ociStorageObject
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
			nextStartAfter = ociStorageSourceID(connector.bucketName, out[len(out)-1].Key)
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

func mustTimePointer(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed := mustTime(t, value)
	return &parsed
}
