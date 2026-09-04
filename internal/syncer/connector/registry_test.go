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
