package connector

import (
	"context"
	"errors"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
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
