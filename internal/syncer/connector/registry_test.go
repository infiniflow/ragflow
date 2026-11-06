package connector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"strconv"
	"strings"
	"testing"
)

func TestRegistryOpenFromConfig(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterConfigFactory("rss", func(config map[string]any) (Connector, error) {
		return NewRSSConnector(config)
	})

	connector, err := registry.OpenFromConfig("rss", map[string]any{"feed_url": "https://example.com/feed.xml"})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if _, ok := connector.(*RSSConnector); !ok {
		t.Fatalf("connector type = %T, want *RSSConnector", connector)
	}

	_, err = registry.OpenFromConfig("missing", map[string]any{})
	if err == nil || !errors.Is(err, ErrUnsupportedSource) || !strings.Contains(err.Error(), `unsupported connector source "missing"`) {
		t.Fatalf("unsupported source error = %v", err)
	}
}

func TestRegistryOpenUsesTaskFactory(t *testing.T) {
	registry := NewRegistry()
	registry.Register("rss", func(ctx context.Context, taskContext dao.SyncTaskContext) (Connector, error) {
		return NewRSSConnector(map[string]any{"feed_url": "https://example.com/feed.xml"})
	})

	connector, err := registry.Open(context.Background(), dao.SyncTaskContext{
		Connector: entity.Connector{Source: "rss"},
	})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if _, ok := connector.(*RSSConnector); !ok {
		t.Fatalf("connector type = %T, want *RSSConnector", connector)
	}
}

func TestRegistryS3SourceHonorsBucketType(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	credentials := map[string]any{
		"endpoint_url":          "https://objects.example.com",
		"aws_access_key_id":     "key",
		"aws_secret_access_key": "secret",
		"addressing_style":      "path",
	}
	open := func(t *testing.T, config map[string]any) []Connector {
		t.Helper()
		fromConfig, err := registry.OpenFromConfig("s3", config)
		if err != nil {
			t.Fatalf("OpenFromConfig failed: %v", err)
		}
		fromTask, err := registry.Open(context.Background(), dao.SyncTaskContext{
			Connector: entity.Connector{Source: "s3", Config: entity.JSONMap(config)},
		})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		return []Connector{fromConfig, fromTask}
	}

	for _, c := range open(t, map[string]any{"bucket_name": "docs", "bucket_type": "s3_compatible", "credentials": credentials}) {
		compatible, ok := c.(*S3CompatibleConnector)
		if !ok {
			t.Fatalf("s3_compatible connector type = %T, want *S3CompatibleConnector", c)
		}
		if compatible.endpointURL != "https://objects.example.com" || compatible.addressingStyle != "path" {
			t.Fatalf("endpoint_url = %q, addressing_style = %q", compatible.endpointURL, compatible.addressingStyle)
		}
	}
	for _, config := range []map[string]any{
		{"bucket_name": "docs", "bucket_type": "s3", "credentials": credentials},
		{"bucket_name": "docs", "credentials": credentials},
	} {
		for _, c := range open(t, config) {
			if _, ok := c.(*S3Connector); !ok {
				t.Fatalf("bucket_type %v connector type = %T, want *S3Connector", config["bucket_type"], c)
			}
		}
	}
}

// TestRegistryS3SourceCompatibleModeKeepsS3IDs verifies the S3 source keeps the
// "s3" ID namespace in compatible mode, so a checkpoint and documents written
// in AWS mode still match, while the s3_compatible source keeps its own.
func TestRegistryS3SourceCompatibleModeKeepsS3IDs(t *testing.T) {
	keys := []string{"a.txt", "b.txt", "c.txt", "d.txt"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startAfter := r.URL.Query().Get("start-after")
		maxKeys, _ := strconv.Atoi(r.URL.Query().Get("max-keys"))
		var page []string
		for _, key := range keys {
			if key > startAfter && (maxKeys <= 0 || len(page) < maxKeys) {
				page = append(page, key)
			}
		}
		truncated := len(page) > 0 && page[len(page)-1] != keys[len(keys)-1]
		var body strings.Builder
		fmt.Fprintf(&body, `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>bucket</Name><IsTruncated>%t</IsTruncated>`, truncated)
		for _, key := range page {
			fmt.Fprintf(&body, `<Contents><Key>%s</Key><LastModified>2024-01-01T00:00:00.000Z</LastModified><ETag>"%s"</ETag><Size>1</Size></Contents>`, key, key)
		}
		body.WriteString(`</ListBucketResult>`)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, body.String())
	}))
	defer server.Close()
	withConnectorLoopbackTestHook(t)

	registry := NewRegistry()
	RegisterBuiltIns(registry)
	config := map[string]any{
		"bucket_name": "bucket",
		"bucket_type": "s3_compatible",
		"batch_size":  2,
		"credentials": map[string]any{
			"endpoint_url":          server.URL,
			"aws_access_key_id":     "key",
			"aws_secret_access_key": "secret",
			"addressing_style":      "path",
		},
	}
	awsMode, err := NewS3Connector(config)
	if err != nil {
		t.Fatalf("NewS3Connector: %v", err)
	}
	compatibleSource, err := registry.OpenFromConfig("s3_compatible", config)
	if err != nil {
		t.Fatalf("OpenFromConfig(s3_compatible): %v", err)
	}
	if got, want := compatibleSource.(*S3CompatibleConnector).sourceID("a.txt"), s3SourceID(s3CompatibleSource, "bucket", "a.txt"); got != want {
		t.Fatalf("s3_compatible sourceID = %q, want %q", got, want)
	}

	connector, err := registry.OpenFromConfig("s3", config)
	if err != nil {
		t.Fatalf("OpenFromConfig(s3): %v", err)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: awsMode.sourceID("a.txt")},
	})
	if err != nil {
		t.Fatalf("OpenSync from an AWS mode checkpoint: %v", err)
	}
	var got []string
	for {
		batch, err := session.NextBatch(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		for _, document := range batch.Documents {
			got = append(got, document.SourceID)
		}
	}
	want := []string{awsMode.sourceID("b.txt"), awsMode.sourceID("c.txt"), awsMode.sourceID("d.txt")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("source ids = %v, want %v", got, want)
	}
}
