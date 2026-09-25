package connector

import (
	"context"
	"errors"
	"ragflow/internal/common"
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

func TestRegisterBuiltInsDecryptsStoredCredentials(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	stored, err := common.EncryptConnectorCredentials(map[string]any{
		"credentials":      map[string]any{"github_access_token": "tok-123"},
		"repository_owner": "ada",
	})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	taskContext := dao.SyncTaskContext{Connector: entity.Connector{Source: "github", Config: entity.ConnectorConfig(stored)}}

	connector, err := registry.Open(context.Background(), taskContext)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	github, ok := connector.(*GitHubConnector)
	if !ok || github.token != "tok-123" || github.owner != "ada" {
		t.Fatalf("connector = %#v, want a GitHub connector with the decrypted token", connector)
	}

	t.Setenv(common.EnvRAGFlowConnectorKey, "")
	if _, err := registry.Open(context.Background(), taskContext); err == nil || !strings.Contains(err.Error(), "RAGFLOW_CONNECTOR_KEY is not set") {
		t.Fatalf("Open without key error = %v, want the missing key error", err)
	}
}
