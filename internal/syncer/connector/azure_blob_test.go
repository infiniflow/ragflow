//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

func TestAzureBlobStorageConnectorDerivesAuthMode(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		want    string
		wantErr bool
	}{
		{
			name: "explicit connection_string",
			config: map[string]any{
				"auth_mode": "connection_string",
				"credentials": map[string]any{
					"connection_string": "DefaultEndpointsProtocol=https;AccountName=acct;AccountKey=key;",
					"container_name":    "container",
				},
			},
			want: "connection_string",
		},
		{
			name: "explicit account_key",
			config: map[string]any{
				"auth_mode": "account_key",
				"credentials": map[string]any{
					"account_name":   "acct",
					"account_key":    "key",
					"container_name": "container",
				},
			},
			want: "account_key",
		},
		{
			name: "explicit sas_token",
			config: map[string]any{
				"auth_mode": "sas_token",
				"credentials": map[string]any{
					"container_url": "https://acct.blob.core.windows.net/container",
					"sas_token":     "?sv=1&sig=abc",
				},
			},
			want: "sas_token",
		},
		{
			name: "fallback connection_string",
			config: map[string]any{
				"credentials": map[string]any{
					"connection_string": "DefaultEndpointsProtocol=https;AccountName=acct;AccountKey=key;",
					"container_name":    "container",
				},
			},
			want: "connection_string",
		},
		{
			name: "fallback account_key",
			config: map[string]any{
				"credentials": map[string]any{
					"account_name":   "acct",
					"account_key":    "key",
					"container_name": "container",
				},
			},
			want: "account_key",
		},
		{
			name: "fallback sas_token",
			config: map[string]any{
				"credentials": map[string]any{
					"container_url": "https://acct.blob.core.windows.net/container",
					"sas_token":     "sv=1&sig=abc",
				},
			},
			want: "sas_token",
		},
		{
			name:    "missing credentials",
			config:  map[string]any{"credentials": map[string]any{}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector, err := NewAzureBlobStorageConnector(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				var missing *ConnectorMissingCredentialError
				if !errors.As(err, &missing) {
					t.Fatalf("error type = %T, want ConnectorMissingCredentialError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAzureBlobStorageConnector: %v", err)
			}
			if connector.authMode != tt.want {
				t.Fatalf("auth mode = %q, want %q", connector.authMode, tt.want)
			}
		})
	}
}

func TestAzureBlobStorageConnectorSasNormalization(t *testing.T) {
	connector, err := NewAzureBlobStorageConnector(map[string]any{
		"auth_mode": "sas_token",
		"credentials": map[string]any{
			"container_url": "https://acct.blob.core.windows.net/container/",
			"sas_token":     "?sv=1&sig=abc",
		},
	})
	if err != nil {
		t.Fatalf("NewAzureBlobStorageConnector: %v", err)
	}
	if connector.containerURL != "https://acct.blob.core.windows.net/container" {
		t.Fatalf("container URL = %q", connector.containerURL)
	}
	if connector.sasToken != "sv=1&sig=abc" {
		t.Fatalf("sas token = %q", connector.sasToken)
	}
}

func TestAzureBlobStorageConnectorOpenSyncUsesFetchAndFingerprint(t *testing.T) {
	connector := newTestAzureBlobStorageConnector(t, map[string]any{
		"auth_mode": "account_key",
		"credentials": map[string]any{
			"account_name":   "acct",
			"account_key":    "key",
			"container_name": "container",
		},
	}, []azureBlobObject{
		{Name: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 5, ETag: `"etag-a"`},
		{Name: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 5, ETag: `"etag-b"`},
	})

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "docs/a.txt" || doc.SemanticIdentifier != "docs/a.txt" {
		t.Fatalf("source id = %q, semantic = %q", doc.SourceID, doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if doc.Fingerprint != normalizedAzureBlobETag(`"etag-a"`) {
		t.Fatalf("fingerprint = %q", doc.Fingerprint)
	}
	if doc.Metadata["container"] != "container" || doc.Metadata["etag"] != "etag-a" {
		t.Fatalf("metadata = %+v", doc.Metadata)
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
	if string(blob) != "body:docs/a.txt" {
		t.Fatalf("blob = %q", blob)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestAzureBlobStorageConnectorOpenSyncDefersListing(t *testing.T) {
	connector := newTestAzureBlobStorageConnector(t, map[string]any{
		"auth_mode": "sas_token",
		"credentials": map[string]any{
			"container_url": "https://acct.blob.core.windows.net/container",
			"sas_token":     "sv=1",
		},
	}, []azureBlobObject{{Name: "a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"}})

	var listCalls int
	baseList := connector.listBlobs
	connector.listBlobs = func(ctx context.Context, prefix, marker string, max int32) ([]azureBlobObject, string, bool, error) {
		listCalls++
		return baseList(ctx, prefix, marker, max)
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

func TestAzureBlobStorageConnectorFiltersImagesUnlessAllowed(t *testing.T) {
	config := map[string]any{
		"auth_mode":   "account_key",
		"credentials": map[string]any{"account_name": "acct", "account_key": "key", "container_name": "container"},
	}
	objects := []azureBlobObject{
		{Name: "docs/a.png", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Name: "docs/b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
	}
	connector := newTestAzureBlobStorageConnector(t, config, objects)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "docs/b.txt" {
		t.Fatalf("documents = %+v", batch.Documents)
	}

	allowed := newTestAzureBlobStorageConnector(t, config, objects)
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

func TestAzureBlobStorageConnectorTimeWindowBoundaries(t *testing.T) {
	since := mustTime(t, "2026-01-01T00:00:00Z")
	until := mustTime(t, "2026-01-03T00:00:00Z")
	connector := newTestAzureBlobStorageConnector(t, map[string]any{
		"auth_mode":   "account_key",
		"credentials": map[string]any{"account_name": "acct", "account_key": "key", "container_name": "container"},
	}, []azureBlobObject{
		{Name: "at_since.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Name: "mid.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Name: "until.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
		{Name: "above.txt", LastModified: mustTime(t, "2026-01-04T00:00:00Z"), Size: 1, ETag: "d"},
	})

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &since,
		WindowEnd:   until,
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	got := make([]string, 0, len(batch.Documents))
	for _, doc := range batch.Documents {
		got = append(got, doc.SourceID)
	}
	want := []string{"mid.txt", "until.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("window documents = %v, want %v", got, want)
	}
}

func TestAzureBlobStorageConnectorFingerprintSkipsUnchanged(t *testing.T) {
	connector := newTestAzureBlobStorageConnector(t, map[string]any{
		"auth_mode":   "account_key",
		"credentials": map[string]any{"account_name": "acct", "account_key": "key", "container_name": "container"},
	}, []azureBlobObject{
		{Name: "a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: `"etag-a"`},
		{Name: "b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: `"etag-b"`},
	})

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		Fingerprints: map[string]string{
			"a.txt": normalizedAzureBlobETag(`"etag-a"`),
		},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "b.txt" {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestAzureBlobStorageConnectorResumeSkipsBlobs(t *testing.T) {
	connector := newTestAzureBlobStorageConnector(t, map[string]any{
		"auth_mode":   "account_key",
		"credentials": map[string]any{"account_name": "acct", "account_key": "key", "container_name": "container"},
	}, []azureBlobObject{
		{Name: "a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Name: "b.txt", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Name: "c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: "a.txt", SourceID: "b.txt"},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "c.txt" {
		t.Fatalf("documents = %+v", batch.Documents)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.Cursor != "c.txt" {
		t.Fatalf("checkpoint = %+v", batch.Checkpoint)
	}
}

func TestAzureBlobStorageConnectorResumeRejectsMissingAnchor(t *testing.T) {
	connector := newTestAzureBlobStorageConnector(t, map[string]any{
		"auth_mode":   "account_key",
		"credentials": map[string]any{"account_name": "acct", "account_key": "key", "container_name": "container"},
	}, []azureBlobObject{
		{Name: "a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Name: "c.txt", LastModified: mustTime(t, "2026-01-03T00:00:00Z"), Size: 1, ETag: "c"},
	})

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: "b.txt", SourceID: "b.txt"},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestAzureBlobStorageConnectorResumeRejectsMissingCheckpoint(t *testing.T) {
	connector := newTestAzureBlobStorageConnector(t, map[string]any{
		"auth_mode":   "account_key",
		"credentials": map[string]any{"account_name": "acct", "account_key": "key", "container_name": "container"},
	}, []azureBlobObject{
		{Name: "a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
	})

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestAzureBlobStorageConnectorOpenPruneReturnsSlimSnapshot(t *testing.T) {
	connector := newTestAzureBlobStorageConnector(t, map[string]any{
		"auth_mode":   "account_key",
		"credentials": map[string]any{"account_name": "acct", "account_key": "key", "container_name": "container"},
	}, []azureBlobObject{
		{Name: "docs/a.txt", LastModified: mustTime(t, "2026-01-01T00:00:00Z"), Size: 1, ETag: "a"},
		{Name: "docs/b.md", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "b"},
		{Name: "docs/c.png", LastModified: mustTime(t, "2026-01-02T00:00:00Z"), Size: 1, ETag: "c"},
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
	if batch.Documents[0].SourceID != "docs/a.txt" || batch.Documents[1].SourceID != "docs/b.md" {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestAzureBlobStorageConnectorValidateClassification(t *testing.T) {
	c := &AzureBlobStorageConnector{}
	var missing *ConnectorMissingCredentialError
	var validation *ConnectorValidationError

	err := c.classifyValidationError(&azcore.ResponseError{StatusCode: http.StatusUnauthorized})
	if !errors.As(err, &missing) {
		t.Fatalf("401 error type = %T, want ConnectorMissingCredentialError", err)
	}

	err = c.classifyValidationError(&azcore.ResponseError{StatusCode: http.StatusForbidden})
	if !errors.As(err, &validation) {
		t.Fatalf("403 error type = %T, want ConnectorValidationError", err)
	}

	err = c.classifyValidationError(&azcore.ResponseError{StatusCode: http.StatusNotFound})
	if !errors.As(err, &validation) {
		t.Fatalf("404 error type = %T, want ConnectorValidationError", err)
	}

	err = c.classifyValidationError(errors.New("boom"))
	if !errors.As(err, &validation) {
		t.Fatalf("generic error type = %T, want ConnectorValidationError", err)
	}
}

func TestAzureBlobStorageConnectorValidateRejectsUnsafeURL(t *testing.T) {
	connector, err := NewAzureBlobStorageConnector(map[string]any{
		"auth_mode": "sas_token",
		"credentials": map[string]any{
			"container_url": "http://192.168.1.10/container",
			"sas_token":     "sv=1",
		},
	})
	if err != nil {
		t.Fatalf("NewAzureBlobStorageConnector: %v", err)
	}
	restore := azureAssertURLSafe
	azureAssertURLSafe = func(rawURL string) (string, string, error) {
		return "", "", errors.New("URL resolves to a non-public address (192.168.1.10), which is not allowed")
	}
	defer func() { azureAssertURLSafe = restore }()

	err = connector.Validate(context.Background())
	var validation *ConnectorValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Validate error type = %T, want ConnectorValidationError", err)
	}
}

func TestAzureBlobStorageConnectorValidateConnectorSettingUsesRequest(t *testing.T) {
	// The receiver holds valid credentials, but the request is missing them.
	// ValidateConnectorSetting must reject based on the unsaved request.
	receiver, err := NewAzureBlobStorageConnector(map[string]any{
		"auth_mode": "account_key",
		"credentials": map[string]any{
			"account_name":   "acct",
			"account_key":    "key",
			"container_name": "container",
		},
	})
	if err != nil {
		t.Fatalf("NewAzureBlobStorageConnector: %v", err)
	}
	err = receiver.ValidateConnectorSetting(context.Background(), map[string]any{
		"auth_mode":   "account_key",
		"credentials": map[string]any{"account_name": "acct"}, // missing account_key
	})
	var missing *ConnectorMissingCredentialError
	if !errors.As(err, &missing) {
		t.Fatalf("ValidateConnectorSetting error type = %T, want ConnectorMissingCredentialError", err)
	}
}

// newTestAzureBlobStorageConnector builds a connector with injected listing,
// download and validation hooks so unit tests need no external Blob service.
func newTestAzureBlobStorageConnector(t *testing.T, config map[string]any, objects []azureBlobObject) *AzureBlobStorageConnector {
	t.Helper()
	connector, err := NewAzureBlobStorageConnector(config)
	if err != nil {
		t.Fatalf("NewAzureBlobStorageConnector: %v", err)
	}
	restore := azureAssertURLSafe
	azureAssertURLSafe = func(rawURL string) (string, string, error) { return "host", "1.2.3.4", nil }
	t.Cleanup(func() { azureAssertURLSafe = restore })

	connector.validateContainer = func(ctx context.Context) error { return nil }
	connector.listBlobs = func(ctx context.Context, prefix, marker string, max int32) ([]azureBlobObject, string, bool, error) {
		var filtered []azureBlobObject
		for _, object := range objects {
			if prefix != "" && !strings.HasPrefix(object.Name, prefix) {
				continue
			}
			if marker != "" && object.Name <= marker {
				continue
			}
			filtered = append(filtered, object)
		}
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
		if len(filtered) == 0 {
			return nil, "", false, nil
		}
		end := int(max)
		if end > len(filtered) {
			end = len(filtered)
		}
		page := filtered[:end]
		hasMore := end < len(filtered)
		next := ""
		if hasMore {
			next = page[len(page)-1].Name
		}
		return page, next, hasMore, nil
	}
	connector.downloadBlob = func(ctx context.Context, name string) ([]byte, error) {
		return []byte("body:" + name), nil
	}
	return connector
}
