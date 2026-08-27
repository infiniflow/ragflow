package connector

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestS3CompatibleConnectorOpenSyncUsesFingerprintAndFetch(t *testing.T) {
	old := mustTime(t, "2026-01-01T00:00:00Z")
	updated := mustTime(t, "2026-01-03T00:00:00Z")
	connector := newTestS3CompatibleConnector(t, []s3Object{
		{Key: "docs/old.txt", LastModified: old, Size: 8, ETag: `"old-etag"`},
		{Key: "docs/new.txt", LastModified: updated, Size: 8, ETag: `"new-etag"`},
	})

	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Fingerprints: map[string]string{
			s3SourceID(s3CompatibleSource, "bucket", "docs/old.txt"): normalizedS3ETag(`"old-etag"`),
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
	if doc.SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/new.txt") {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "new.txt" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if doc.Fingerprint != normalizedS3ETag(`"new-etag"`) {
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

func TestS3CompatibleConnectorOpenSyncDefersListingUntilNextBatch(t *testing.T) {
	connector := newTestS3CompatibleConnector(t, []s3Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})
	var listCalls int
	baseListObjects := connector.listObjects
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error) {
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

func TestS3CompatibleConnectorOpenSyncFiltersImagesUnlessAllowed(t *testing.T) {
	objects := []s3Object{
		{Key: "docs/a.png", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
	}
	connector := newTestS3CompatibleConnector(t, objects)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}

	allowed := newTestS3CompatibleConnector(t, objects)
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

func TestS3CompatibleConnectorOpenSyncResumesFromCheckpoint(t *testing.T) {
	connector := newTestS3CompatibleConnector(t, []s3Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})
	checkpointUpdatedAt := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:    s3SourceID(s3CompatibleSource, "bucket", "docs/b.txt"),
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
	if batch.Documents[0].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/c.txt") {
		t.Fatalf("resumed source id = %q, want docs/c.txt", batch.Documents[0].SourceID)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/c.txt") {
		t.Fatalf("checkpoint = %+v", batch.Checkpoint)
	}
}

func TestS3CompatibleConnectorOpenSyncRejectsInvalidResumeAnchors(t *testing.T) {
	connector := newTestS3CompatibleConnector(t, []s3Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
	})
	checkpoints := []*SyncCheckpoint{
		{Cursor: "other:bucket:docs/a.txt"},
		{Cursor: s3SourceID(s3CompatibleSource, "bucket", "")},
		{Cursor: s3SourceID(s3CompatibleSource, "bucket", "docs/missing.txt")},
	}
	for _, checkpoint := range checkpoints {
		_, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: checkpoint})
		if !errors.Is(err, ErrSyncResumeInvalid) {
			t.Fatalf("OpenSync error = %v for checkpoint %+v, want ErrSyncResumeInvalid", err, checkpoint)
		}
	}
}

func TestS3CompatibleConnectorOpenSyncIncludesMissingFingerprint(t *testing.T) {
	connector := newTestS3CompatibleConnector(t, []s3Object{
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

func TestS3CompatibleConnectorOpenSyncFiltersByWindow(t *testing.T) {
	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	connector := newTestS3CompatibleConnector(t, []s3Object{
		{Key: "docs/before.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "before"},
		{Key: "docs/within.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "within"},
		{Key: "docs/after.txt", LastModified: mustTime(t, "2026-01-05T00:00:00Z"), Size: 1, ETag: "after"},
	})
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/within.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestS3CompatibleConnectorOpenPruneReturnsSlimSnapshot(t *testing.T) {
	connector := newTestS3CompatibleConnector(t, []s3Object{
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
	if batch.Documents[0].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/a.txt") || batch.Documents[1].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestS3CompatibleConnectorOpenPruneStreamsAcrossPages(t *testing.T) {
	connector, err := NewS3CompatibleConnector(map[string]any{
		"bucket_name": "bucket",
		"batch_size":  2,
		"credentials": map[string]any{
			"endpoint_url":          "https://s3.example.com",
			"aws_access_key_id":     "access",
			"aws_secret_access_key": "secret",
			"addressing_style":      "path",
		},
	})
	if err != nil {
		t.Fatalf("NewS3CompatibleConnector: %v", err)
	}
	pages := [][]s3Object{
		{
			{Key: "docs/a.txt", Size: 1},
			{Key: "docs/b.txt", Size: 1},
		},
		{
			{Key: "docs/c.txt", Size: 1},
			{Key: "docs/d.txt", Size: 1},
		},
	}
	call := 0
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error) {
		if call >= len(pages) {
			return nil, "", false, nil
		}
		page := pages[call]
		call++
		nextStartAfter := ""
		if len(page) > 0 {
			nextStartAfter = s3SourceID(s3CompatibleSource, connector.bucketName, page[len(page)-1].Key)
		}
		return page, nextStartAfter, call < len(pages), nil
	}
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch: %v", err)
	}
	if len(first.Documents) != 2 || first.Documents[0].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/a.txt") || first.Documents[1].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/b.txt") {
		t.Fatalf("first batch = %+v", first.Documents)
	}
	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second NextBatch: %v", err)
	}
	if len(second.Documents) != 2 || second.Documents[0].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/c.txt") || second.Documents[1].SourceID != s3SourceID(s3CompatibleSource, "bucket", "docs/d.txt") {
		t.Fatalf("second batch = %+v", second.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestS3CompatibleConnectorValidate(t *testing.T) {
	ctx := context.Background()
	missingBucket := &S3CompatibleConnector{endpointURL: "https://s3.example.com", accessKeyID: "access", secretKey: "secret", addressingStyle: "virtual", batchSize: 2}
	if err := missingBucket.Validate(ctx); err == nil {
		t.Fatalf("expected bucket name validation error")
	}
	missingEndpoint := &S3CompatibleConnector{bucketName: "bucket", accessKeyID: "access", secretKey: "secret", addressingStyle: "virtual", batchSize: 2}
	if err := missingEndpoint.Validate(ctx); err == nil {
		t.Fatalf("expected endpoint validation error")
	}
	missingCreds := &S3CompatibleConnector{bucketName: "bucket", endpointURL: "https://s3.example.com", addressingStyle: "virtual", batchSize: 2}
	if err := missingCreds.Validate(ctx); err == nil {
		t.Fatalf("expected credentials validation error")
	}
	badStyle := &S3CompatibleConnector{bucketName: "bucket", endpointURL: "https://s3.example.com", accessKeyID: "access", secretKey: "secret", addressingStyle: "invalid", batchSize: 2}
	if err := badStyle.Validate(ctx); err == nil {
		t.Fatalf("expected addressing style validation error")
	}
	badBatch := &S3CompatibleConnector{bucketName: "bucket", endpointURL: "https://s3.example.com", accessKeyID: "access", secretKey: "secret", addressingStyle: "virtual"}
	if err := badBatch.Validate(ctx); err == nil {
		t.Fatalf("expected batch size validation error")
	}
}

func TestS3CompatibleConnectorRejectsInvalidEndpointBeforeClientCreation(t *testing.T) {
	for _, endpoint := range []string{
		"https://evil.example#",
		"evil.example#",
		"ftp://evil.example",
		"https://user:pass@evil.example",
		"https://evil.example#fragment",
		"https://",
		"https://evil.example/pa th",
	} {
		connector := newTestS3CompatibleConnector(t, nil)
		connector.endpointURL = endpoint
		connector.client = nil
		if err := connector.Validate(context.Background()); err == nil {
			t.Fatalf("expected invalid endpoint %q to be rejected", endpoint)
		}
		if connector.client != nil {
			t.Fatalf("client was created for invalid endpoint %q", endpoint)
		}
	}
}

func TestS3CompatibleConnectorValidateSetting(t *testing.T) {
	connector := newTestS3CompatibleConnector(t, []s3Object{
		{Key: "docs/a.txt", Size: 1},
	})
	var listCalls int
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error) {
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

func TestNewS3CompatibleConnectorDefaultsAndConfig(t *testing.T) {
	connector, err := NewS3CompatibleConnector(map[string]any{
		"bucket_name":    " bucket ",
		"prefix":         "docs",
		"batch_size":     4,
		"size_threshold": 42,
		"allow_images":   true,
		"credentials": map[string]any{
			"endpoint_url":          " https://s3.example.com ",
			"aws_access_key_id":     "access",
			"aws_secret_access_key": "secret",
			"addressing_style":      "path",
		},
	})
	if err != nil {
		t.Fatalf("NewS3CompatibleConnector: %v", err)
	}
	if connector.bucketName != "bucket" || connector.prefix != "docs/" {
		t.Fatalf("bucket/prefix = %q/%q", connector.bucketName, connector.prefix)
	}
	if connector.batchSize != 4 || connector.sizeThreshold != 42 || !connector.allowImages {
		t.Fatalf("batch/threshold/allow_images = %d/%d/%v", connector.batchSize, connector.sizeThreshold, connector.allowImages)
	}
	if connector.endpointURL != "https://s3.example.com" || connector.addressingStyle != "path" || connector.region != defaultS3CompatibleRegion {
		t.Fatalf("endpoint/style/region = %q/%q/%q", connector.endpointURL, connector.addressingStyle, connector.region)
	}

	defaults, err := NewS3CompatibleConnector(map[string]any{"bucket_name": "bucket", "credentials": map[string]any{}})
	if err != nil {
		t.Fatalf("NewS3CompatibleConnector defaults: %v", err)
	}
	if defaults.batchSize != defaultS3BatchSize || defaults.sizeThreshold != defaultS3SizeThreshold || defaults.region != defaultS3CompatibleRegion || defaults.addressingStyle != "virtual" {
		t.Fatalf("defaults = batch %d threshold %d region %q style %q", defaults.batchSize, defaults.sizeThreshold, defaults.region, defaults.addressingStyle)
	}
}

func TestS3CompatibleHelpers(t *testing.T) {
	if got := s3SourceID(s3CompatibleSource, "bucket", "docs/a.txt"); got != "s3_compatible:bucket:docs/a.txt" {
		t.Fatalf("source id = %q", got)
	}
	if got := normalizeS3Prefix("docs"); got != "docs/" {
		t.Fatalf("prefix = %q", got)
	}
	if got := pathEscapeS3Key("docs/a b.txt"); got != "docs/a%20b.txt" {
		t.Fatalf("escaped key = %q", got)
	}
	if got := strings.TrimSpace(s3ConsoleURL("us-east-1", "bucket", "docs/a b.txt")); !strings.HasPrefix(got, "https://s3.console.aws.amazon.com/s3/object/bucket?region=us-east-1&prefix=docs/a%20b.txt") {
		t.Fatalf("console url = %q", got)
	}
	if _, err := readS3Body(bytes.NewBufferString("12345"), "large.txt", 4); err == nil {
		t.Fatalf("expected size threshold error")
	}
}

func newTestS3CompatibleConnector(t *testing.T, objects []s3Object) *S3CompatibleConnector {
	t.Helper()
	connector, err := NewS3CompatibleConnector(map[string]any{
		"bucket_name": "bucket",
		"prefix":      "docs",
		"batch_size":  2,
		"credentials": map[string]any{
			"endpoint_url":          "https://s3.example.com",
			"aws_access_key_id":     "access",
			"aws_secret_access_key": "secret",
			"addressing_style":      "virtual",
		},
	})
	if err != nil {
		t.Fatalf("NewS3CompatibleConnector: %v", err)
	}
	connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error) {
		var out []s3Object
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
			nextStartAfter = s3SourceID(s3CompatibleSource, connector.bucketName, out[len(out)-1].Key)
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
