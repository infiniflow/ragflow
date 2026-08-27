package connector

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestS3ConnectorOpenSyncUsesFingerprintAndFetch(t *testing.T) {
	old := mustTime(t, "2026-01-01T00:00:00Z")
	updated := mustTime(t, "2026-01-03T00:00:00Z")
	connector := newTestS3Connector(t, []s3Object{
		{Key: "docs/old.txt", LastModified: old, Size: 8, ETag: `"old-etag"`},
		{Key: "docs/new.txt", LastModified: updated, Size: 8, ETag: `"new-etag"`},
	})

	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Fingerprints: map[string]string{
			s3SourceID(s3Source, "bucket", "docs/old.txt"): normalizedS3ETag(`"old-etag"`),
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
	if doc.SourceID != s3SourceID(s3Source, "bucket", "docs/new.txt") {
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

func TestS3ConnectorOpenSyncDefersListingUntilNextBatch(t *testing.T) {
	connector := newTestS3Connector(t, []s3Object{
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

func TestS3ConnectorOpenSyncFiltersImagesUnlessAllowed(t *testing.T) {
	objects := []s3Object{
		{Key: "docs/a.png", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
	}
	connector := newTestS3Connector(t, objects)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != s3SourceID(s3Source, "bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}

	allowed := newTestS3Connector(t, objects)
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

func TestS3ConnectorOpenSyncResumesFromCheckpoint(t *testing.T) {
	connector := newTestS3Connector(t, []s3Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Key: "docs/c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})
	checkpointUpdatedAt := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume: &SyncCheckpoint{
			Cursor:    s3SourceID(s3Source, "bucket", "docs/b.txt"),
			SourceID:  s3SourceID(s3Source, "bucket", "docs/b.txt"),
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
	if batch.Documents[0].SourceID != s3SourceID(s3Source, "bucket", "docs/c.txt") {
		t.Fatalf("resumed source id = %q, want docs/c.txt", batch.Documents[0].SourceID)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != s3SourceID(s3Source, "bucket", "docs/c.txt") {
		t.Fatalf("checkpoint = %+v", batch.Checkpoint)
	}
}

func TestS3ConnectorOpenSyncRejectsInvalidResumeAnchors(t *testing.T) {
	connector := newTestS3Connector(t, []s3Object{
		{Key: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Key: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
	})
	checkpoints := []*SyncCheckpoint{
		{SourceID: "other:bucket:docs/a.txt"},
		{SourceID: s3SourceID(s3Source, "bucket", "")},
		{SourceID: s3SourceID(s3Source, "bucket", "docs/missing.txt")},
	}
	for _, checkpoint := range checkpoints {
		_, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: checkpoint})
		if !errors.Is(err, ErrSyncResumeInvalid) {
			t.Fatalf("OpenSync error = %v for checkpoint %+v, want ErrSyncResumeInvalid", err, checkpoint)
		}
	}
}

func TestS3ConnectorOpenSyncIncludesMissingFingerprint(t *testing.T) {
	connector := newTestS3Connector(t, []s3Object{
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

func TestS3ConnectorOpenSyncFiltersByWindow(t *testing.T) {
	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	connector := newTestS3Connector(t, []s3Object{
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
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != s3SourceID(s3Source, "bucket", "docs/within.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestS3ConnectorOpenPruneReturnsSlimSnapshot(t *testing.T) {
	connector := newTestS3Connector(t, []s3Object{
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
	if batch.Documents[0].SourceID != s3SourceID(s3Source, "bucket", "docs/a.txt") || batch.Documents[1].SourceID != s3SourceID(s3Source, "bucket", "docs/b.txt") {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestS3ConnectorOpenPruneStreamsAcrossPages(t *testing.T) {
	connector, err := NewS3Connector(map[string]any{
		"bucket_name": "bucket",
		"batch_size":  2,
		"credentials": map[string]any{
			"aws_access_key_id":     "access",
			"aws_secret_access_key": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewS3Connector: %v", err)
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
			nextStartAfter = s3SourceID(s3Source, connector.bucketName, page[len(page)-1].Key)
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
	if len(first.Documents) != 2 || first.Documents[0].SourceID != s3SourceID(s3Source, "bucket", "docs/a.txt") || first.Documents[1].SourceID != s3SourceID(s3Source, "bucket", "docs/b.txt") {
		t.Fatalf("first batch = %+v", first.Documents)
	}
	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second NextBatch: %v", err)
	}
	if len(second.Documents) != 2 || second.Documents[0].SourceID != s3SourceID(s3Source, "bucket", "docs/c.txt") || second.Documents[1].SourceID != s3SourceID(s3Source, "bucket", "docs/d.txt") {
		t.Fatalf("second batch = %+v", second.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestS3ConnectorOpenPruneErrorsWhenPaginationDoesNotAdvance(t *testing.T) {
	tests := []struct {
		name           string
		nextStartAfter string
	}{
		{name: "empty cursor", nextStartAfter: ""},
		{name: "unchanged cursor", nextStartAfter: s3SourceID(s3Source, "bucket", "docs/a.txt")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connector := newTestS3Connector(t, []s3Object{{Key: "docs/a.txt", Size: 1}})
			connector.listObjects = func(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error) {
				return []s3Object{{Key: "docs/a.txt", Size: 1}}, test.nextStartAfter, true, nil
			}
			session, err := connector.OpenPrune(context.Background(), PruneRequest{})
			if err != nil {
				t.Fatalf("OpenPrune: %v", err)
			}
			batch, err := session.NextBatch(context.Background())
			if err == nil || errors.Is(err, io.EOF) {
				t.Fatalf("NextBatch error = %v, want non-EOF pagination error", err)
			}
			if !strings.Contains(err.Error(), "listing did not advance") {
				t.Fatalf("NextBatch error = %q, want listing did not advance", err)
			}
			if len(batch.Documents) != 0 {
				t.Fatalf("NextBatch returned partial documents = %+v", batch.Documents)
			}
		})
	}
}

func TestS3ConnectorValidate(t *testing.T) {
	ctx := context.Background()
	missingBucket := &S3Connector{authMethod: "access_key", accessKeyID: "access", secretKey: "secret", batchSize: 2}
	if err := missingBucket.Validate(ctx); err == nil {
		t.Fatalf("expected bucket name validation error")
	}
	missingCreds := &S3Connector{bucketName: "bucket", authMethod: "access_key", batchSize: 2}
	if err := missingCreds.Validate(ctx); err == nil {
		t.Fatalf("expected credentials validation error")
	}
	missingRole := &S3Connector{bucketName: "bucket", authMethod: "iam_role", batchSize: 2}
	if err := missingRole.Validate(ctx); err == nil {
		t.Fatalf("expected IAM role validation error")
	}
	badAuth := &S3Connector{bucketName: "bucket", authMethod: "invalid", batchSize: 2}
	if err := badAuth.Validate(ctx); err == nil {
		t.Fatalf("expected unsupported auth method error")
	}
	badBatch := &S3Connector{bucketName: "bucket", authMethod: "access_key", accessKeyID: "access", secretKey: "secret"}
	if err := badBatch.Validate(ctx); err == nil {
		t.Fatalf("expected batch size validation error")
	}
}

func TestS3ConnectorRejectsInvalidRegionBeforeClientCreation(t *testing.T) {
	for _, region := range []string{
		"evil.example#",
		"evil.example.com",
		"https://evil.example.org",
		"region space",
		"-region",
		"region-",
		"Region",
	} {
		connector := newTestS3Connector(t, nil)
		connector.region = region
		connector.client = nil
		if err := connector.Validate(context.Background()); err == nil {
			t.Fatalf("expected invalid region %q to be rejected", region)
		}
		if connector.client != nil {
			t.Fatalf("client was created for invalid region %q", region)
		}
	}
}

func TestS3ConnectorValidateSetting(t *testing.T) {
	connector := newTestS3Connector(t, []s3Object{
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

func TestNewS3ConnectorDefaultsAndConfig(t *testing.T) {
	connector, err := NewS3Connector(map[string]any{
		"bucket_name":    " bucket ",
		"prefix":         "docs",
		"batch_size":     4,
		"size_threshold": 42,
		"allow_images":   true,
		"credentials": map[string]any{
			"authentication_method": "access_key",
			"region":                "us-east-1",
			"aws_access_key_id":     "access",
			"aws_secret_access_key": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewS3Connector: %v", err)
	}
	if connector.bucketName != "bucket" || connector.prefix != "docs/" {
		t.Fatalf("bucket/prefix = %q/%q", connector.bucketName, connector.prefix)
	}
	if connector.batchSize != 4 || connector.sizeThreshold != 42 || !connector.allowImages {
		t.Fatalf("batch/threshold/allow_images = %d/%d/%v", connector.batchSize, connector.sizeThreshold, connector.allowImages)
	}
	if connector.region != "us-east-1" || connector.accessKeyID != "access" || connector.secretKey != "secret" {
		t.Fatalf("credentials = %+v", connector)
	}

	defaults, err := NewS3Connector(map[string]any{"bucket_name": "bucket", "credentials": map[string]any{}})
	if err != nil {
		t.Fatalf("NewS3Connector defaults: %v", err)
	}
	if defaults.batchSize != defaultS3BatchSize || defaults.sizeThreshold != defaultS3SizeThreshold || defaults.authMethod != "access_key" {
		t.Fatalf("defaults = batch %d threshold %d auth %q", defaults.batchSize, defaults.sizeThreshold, defaults.authMethod)
	}
}

func TestS3Helpers(t *testing.T) {
	if got := s3SourceID(s3Source, "bucket", "docs/a.txt"); got != "s3:bucket:docs/a.txt" {
		t.Fatalf("source id = %q", got)
	}
	if got := normalizeS3Prefix("docs"); got != "docs/" {
		t.Fatalf("prefix = %q", got)
	}
	if got := pathEscapeS3Key("docs/a b.txt"); got != "docs/a%20b.txt" {
		t.Fatalf("escaped key = %q", got)
	}
	if got := s3ConsoleURL("us-east-1", "bucket", "docs/a b.txt"); !strings.Contains(got, "https://s3.console.aws.amazon.com/s3/object/bucket?region=us-east-1&prefix=docs/a%20b.txt") {
		t.Fatalf("console url = %q", got)
	}
	if _, err := readS3Body(bytes.NewBufferString("12345"), "large.txt", 4); err == nil {
		t.Fatalf("expected size threshold error")
	}
}

func newTestS3Connector(t *testing.T, objects []s3Object) *S3Connector {
	t.Helper()
	connector, err := NewS3Connector(map[string]any{
		"bucket_name": "bucket",
		"prefix":      "docs",
		"batch_size":  2,
		"credentials": map[string]any{
			"aws_access_key_id":     "access",
			"aws_secret_access_key": "secret",
			"region":                "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("NewS3Connector: %v", err)
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
			nextStartAfter = s3SourceID(s3Source, connector.bucketName, out[len(out)-1].Key)
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
