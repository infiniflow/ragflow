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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"

	"ragflow/internal/utility"
)

const (
	azureBlobSource            = "azure_blob"
	azureBlobDefaultBatchSize  = 2
	azureBlobEndpointSuffix    = "blob.core.windows.net"
	azureBlobErrorMessageLimit = 300
)

// azureAssertURLSafe is the SSRF guard used for caller-supplied endpoint URLs.
// It is an indirection over utility.AssertURLSafe so unit tests can substitute
// a stub without touching the shared global guard.
var azureAssertURLSafe = utility.AssertURLSafe

// azureBlobTextExtensions mirrors the extension set used by the OneDrive
// family of file sources in the blob connector.
var azureBlobTextExtensions = map[string]struct{}{
	".pdf":  {},
	".docx": {},
	".doc":  {},
	".xlsx": {},
	".xls":  {},
	".pptx": {},
	".ppt":  {},
	".txt":  {},
	".md":   {},
	".csv":  {},
	".html": {},
	".htm":  {},
	".json": {},
	".xml":  {},
}

var azureBlobImageExtensions = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".webp": {},
	".gif":  {},
	".bmp":  {},
	".tiff": {},
}

// AzureBlobStorageConnector reads blobs from an Azure Blob Storage container.
//
// Authentication supports three mutually exclusive modes selected by
// auth_mode (connection_string, account_key, sas_token). When auth_mode is
// absent, the mode is derived from whichever credential fields are populated,
// mirroring the blob connector's fallback precedence.
type AzureBlobStorageConnector struct {
	batchSize   int
	prefix      string
	allowImages bool

	authMode         string
	containerName    string
	accountName      string
	accountKey       string
	connectionString string
	containerURL     string // sas_token mode base URL (no trailing slash, no SAS query)
	sasToken         string // sas_token mode token (no leading "?")

	containerClient *container.Client

	listBlobs         func(ctx context.Context, prefix, marker string, maxResults int32) ([]azureBlobObject, string, bool, error)
	downloadBlob      func(ctx context.Context, name string) ([]byte, error)
	validateContainer func(ctx context.Context) error
}

// NewAzureBlobStorageConnector creates an Azure Blob connector from
// Python-compatible config.
func NewAzureBlobStorageConnector(config map[string]any) (*AzureBlobStorageConnector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), azureBlobDefaultBatchSize)
	if batchSize <= 0 {
		batchSize = azureBlobDefaultBatchSize
	}
	connector := &AzureBlobStorageConnector{
		batchSize:        batchSize,
		prefix:           normalizeAzureBlobPrefix(stringConfig(config["prefix"])),
		allowImages:      configBoolDefault(config["allow_images"], false),
		authMode:         strings.ToLower(strings.TrimSpace(stringConfig(config["auth_mode"]))),
		containerName:    strings.TrimSpace(stringConfig(credentials["container_name"])),
		accountName:      strings.TrimSpace(stringConfig(credentials["account_name"])),
		accountKey:       stringConfig(credentials["account_key"]),
		connectionString: stringConfig(credentials["connection_string"]),
		containerURL:     strings.TrimRight(strings.TrimSpace(stringConfig(credentials["container_url"])), "/"),
		sasToken:         strings.TrimLeft(stringConfig(credentials["sas_token"]), "?"),
	}
	if connector.authMode == "" {
		switch {
		case connector.connectionString != "":
			connector.authMode = "connection_string"
		case connector.accountName != "" && connector.accountKey != "":
			connector.authMode = "account_key"
		case connector.containerURL != "" && connector.sasToken != "":
			connector.authMode = "sas_token"
		}
	}
	if err := connector.validateCredentials(); err != nil {
		return nil, err
	}
	return connector, nil
}

// Validate validates Azure Blob connector settings and credentials.
func (c *AzureBlobStorageConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("Azure Blob connector is nil")
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "Azure Blob: batch_size must be a positive integer"}
	}
	if err := c.validateCredentials(); err != nil {
		return err
	}
	if err := c.validateURLSafety(); err != nil {
		return err
	}
	if c.validateContainer != nil {
		return c.validateContainer(ctx)
	}
	return c.defaultValidateContainer(ctx)
}

// ValidateConnectorSetting validates Azure Blob settings from an unsaved
// config by constructing a temporary connector from the request.
func (c *AzureBlobStorageConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	connector, err := NewAzureBlobStorageConnector(request)
	if err != nil {
		return err
	}
	return connector.Validate(ctx)
}

// OpenSync opens one Azure Blob sync session.
func (c *AzureBlobStorageConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	session := &azureBlobSyncSession{
		connector: c,
		request:   request,
		batchSize: c.batchSize,
	}
	if request.Resume != nil {
		if err := session.applyResume(ctx, request.Resume); err != nil {
			return nil, err
		}
	}
	return session, nil
}

// OpenPrune opens one complete Azure Blob prune snapshot session.
func (c *AzureBlobStorageConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	names, err := c.collectBlobNames(ctx)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(names))
	for _, name := range names {
		if name == "" || !azureBlobSupportedExtension(name, c.allowImages) {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: name})
	}
	return &azureBlobPruneSession{documents: documents, batchSize: c.batchSize}, nil
}

// Fetch downloads an Azure Blob body referenced by a previous sync batch.
func (c *AzureBlobStorageConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch azureBlobFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	return c.download(ctx, fetch.Name)
}

// validateCredentials checks that the selected auth mode has all required
// credential fields populated.
func (c *AzureBlobStorageConnector) validateCredentials() error {
	switch c.authMode {
	case "connection_string":
		if c.connectionString == "" {
			return &ConnectorMissingCredentialError{Message: "Azure Blob: connection_string is required for the connection_string auth mode"}
		}
		if c.containerName == "" {
			return &ConnectorMissingCredentialError{Message: "Azure Blob: container_name is required together with connection_string"}
		}
	case "account_key":
		if c.accountName == "" || c.accountKey == "" {
			return &ConnectorMissingCredentialError{Message: "Azure Blob: account_name and account_key are required for the account_key auth mode"}
		}
		if c.containerName == "" {
			return &ConnectorMissingCredentialError{Message: "Azure Blob: container_name is required together with account_name + account_key"}
		}
	case "sas_token":
		if c.containerURL == "" || c.sasToken == "" {
			return &ConnectorMissingCredentialError{Message: "Azure Blob: container_url and sas_token are required for the sas_token auth mode"}
		}
	default:
		return &ConnectorMissingCredentialError{
			Message: "Azure Blob credentials are incomplete. Provide one of: (a) connection_string + container_name, (b) account_name + account_key + container_name, (c) container_url + sas_token.",
		}
	}
	return nil
}

// validateURLSafety rejects caller-supplied endpoints that are not HTTPS and
// do not resolve to a public host. The connection-string mode is a single
// opaque blob whose endpoint is embedded by Azure, so it is not routed by the
// caller and is intentionally left alone.
func (c *AzureBlobStorageConnector) validateURLSafety() error {
	endpoint := ""
	switch c.authMode {
	case "account_key":
		endpoint = fmt.Sprintf("https://%s.%s", c.accountName, azureBlobEndpointSuffix)
	case "sas_token":
		endpoint = c.containerURL
	}
	if endpoint == "" {
		return nil
	}
	if _, _, err := azureAssertURLSafe(endpoint); err != nil {
		return &ConnectorValidationError{Message: fmt.Sprintf("Azure Blob: unsafe endpoint URL: %v", err)}
	}
	return nil
}

func (c *AzureBlobStorageConnector) ensureClient(ctx context.Context) (*container.Client, error) {
	if c.containerClient != nil {
		return c.containerClient, nil
	}
	var (
		client *container.Client
		err    error
	)
	switch c.authMode {
	case "connection_string":
		client, err = container.NewClientFromConnectionString(c.connectionString, c.containerName, nil)
	case "account_key":
		var cred *azblob.SharedKeyCredential
		cred, err = azblob.NewSharedKeyCredential(c.accountName, c.accountKey)
		if err == nil {
			accountURL := fmt.Sprintf("https://%s.%s/%s", c.accountName, azureBlobEndpointSuffix, c.containerName)
			client, err = container.NewClientWithSharedKeyCredential(accountURL, cred, nil)
		}
	case "sas_token":
		fullURL := c.containerURL + "?" + c.sasToken
		client, err = container.NewClientWithNoCredential(fullURL, nil)
	default:
		return nil, &ConnectorMissingCredentialError{Message: "Azure Blob credentials are incomplete."}
	}
	if err != nil {
		return nil, &ConnectorMissingCredentialError{Message: fmt.Sprintf("Failed to initialise Azure Blob client: %v", err)}
	}
	c.containerClient = client
	return client, nil
}

// defaultValidateContainer issues the container properties call that proves
// both the credential and the container name are valid, classifying errors the
// same way the blob connector does.
func (c *AzureBlobStorageConnector) defaultValidateContainer(ctx context.Context) error {
	client, err := c.ensureClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.GetProperties(ctx, nil); err != nil {
		return c.classifyValidationError(err)
	}
	return nil
}

func (c *AzureBlobStorageConnector) classifyValidationError(err error) error {
	msg := truncateAzureBlobMessage(err.Error())
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		switch {
		case respErr.StatusCode == http.StatusUnauthorized ||
			strings.Contains(msg, "AuthenticationFailed") ||
			strings.Contains(msg, "InvalidAuthenticationInfo"):
			return &ConnectorMissingCredentialError{Message: fmt.Sprintf("Azure Blob credential rejected: %s", msg)}
		case respErr.StatusCode == http.StatusForbidden ||
			strings.Contains(msg, "AuthorizationPermissionMismatch") ||
			strings.Contains(msg, "403"):
			return &ConnectorValidationError{Message: fmt.Sprintf("Azure Blob: insufficient permissions on container: %s", msg)}
		case respErr.StatusCode == http.StatusNotFound ||
			strings.Contains(msg, "ContainerNotFound") ||
			strings.Contains(msg, "404"):
			return &ConnectorValidationError{Message: fmt.Sprintf("Azure Blob: container not found: %s", msg)}
		default:
			return &ConnectorValidationError{Message: fmt.Sprintf("Azure Blob validation failed (%s): %s", respErr.ErrorCode, msg)}
		}
	}
	return &ConnectorValidationError{Message: fmt.Sprintf("Azure Blob validation failed: %s", msg)}
}

func (c *AzureBlobStorageConnector) listBlobPage(ctx context.Context, prefix, marker string, maxResults int32) ([]azureBlobObject, string, bool, error) {
	if c.listBlobs != nil {
		return c.listBlobs(ctx, prefix, marker, maxResults)
	}
	client, err := c.ensureClient(ctx)
	if err != nil {
		return nil, "", false, err
	}
	options := &container.ListBlobsFlatOptions{MaxResults: &maxResults}
	if prefix != "" {
		options.Prefix = &prefix
	}
	if marker != "" {
		options.Marker = &marker
	}
	page, err := client.NewListBlobsFlatPager(options).NextPage(ctx)
	if err != nil {
		return nil, "", false, err
	}
	objects := make([]azureBlobObject, 0, len(page.Segment.BlobItems))
	for _, item := range page.Segment.BlobItems {
		if object, ok := azureBlobObjectFromItem(item); ok {
			objects = append(objects, object)
		}
	}
	next := ""
	if page.NextMarker != nil {
		next = *page.NextMarker
	}
	return objects, next, next != "", nil
}

func (c *AzureBlobStorageConnector) collectBlobNames(ctx context.Context) ([]string, error) {
	var names []string
	marker := ""
	for {
		objects, next, hasMore, err := c.listBlobPage(ctx, c.prefix, marker, int32(c.batchSize))
		if err != nil {
			return nil, err
		}
		for _, object := range objects {
			names = append(names, object.Name)
		}
		if !hasMore {
			break
		}
		marker = next
	}
	sort.Strings(names)
	return names, nil
}

func (c *AzureBlobStorageConnector) download(ctx context.Context, name string) ([]byte, error) {
	if c.downloadBlob != nil {
		return c.downloadBlob(ctx, name)
	}
	client, err := c.ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	response, err := client.NewBlockBlobClient(name).DownloadStream(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}

func (c *AzureBlobStorageConnector) sourceDocument(name string, object azureBlobObject) (SourceDocument, bool) {
	if name == "" || !azureBlobSupportedExtension(name, c.allowImages) {
		return SourceDocument{}, false
	}
	fetch := azureBlobFetchReference{Name: name}
	fetchKey, _ := json.Marshal(fetch)
	return SourceDocument{
		SourceID:           name,
		SemanticIdentifier: name,
		Extension:          azureBlobExtension(name),
		FetchRef:           &FetchReference{Key: string(fetchKey), SizeHint: object.Size},
		UpdatedAt:          object.LastModified,
		SizeBytes:          object.Size,
		Metadata: map[string]any{
			"container": c.containerName,
			"etag":      azureBlobETag(object.ETag),
			"prefix":    c.prefix,
		},
		Fingerprint: normalizedAzureBlobETag(object.ETag),
	}, true
}

// azureBlobObject is the normalized minimal blob snapshot used by the
// connector and its tests.
type azureBlobObject struct {
	Name         string
	LastModified time.Time
	Size         int64
	ETag         string
}

func azureBlobObjectFromItem(item *container.BlobItem) (azureBlobObject, bool) {
	if item == nil || item.Name == nil || item.Properties == nil {
		return azureBlobObject{}, false
	}
	object := azureBlobObject{Name: *item.Name}
	if item.Properties.LastModified != nil {
		object.LastModified = item.Properties.LastModified.UTC()
	}
	if item.Properties.ContentLength != nil {
		object.Size = *item.Properties.ContentLength
	}
	if item.Properties.ETag != nil {
		object.ETag = string(*item.Properties.ETag)
	}
	return object, true
}

type azureBlobSyncSession struct {
	connector *AzureBlobStorageConnector
	request   SyncRequest
	batchSize int
	marker    string
	skipUntil string
	done      bool
}

// NextBatch returns the next Azure Blob document batch.
func (s *azureBlobSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	for {
		if s.done {
			return SyncBatch{}, io.EOF
		}
		objects, next, hasMore, err := s.connector.listBlobPage(ctx, s.connector.prefix, s.marker, int32(s.batchSize))
		if err != nil {
			return SyncBatch{}, err
		}
		if !hasMore {
			s.done = true
		}
		s.marker = next

		documents := make([]SourceDocument, 0, len(objects))
		for _, object := range objects {
			if s.skipUntil != "" && object.Name <= s.skipUntil {
				continue
			}
			if !includeAzureBlobObject(s.request, object) {
				continue
			}
			if document, ok := s.connector.sourceDocument(object.Name, object); ok {
				documents = append(documents, document)
			}
		}
		if len(documents) == 0 {
			continue
		}
		last := documents[len(documents)-1]
		updatedAt := last.UpdatedAt
		return SyncBatch{
			Documents: documents,
			Checkpoint: &SyncCheckpoint{
				Cursor:    last.SourceID,
				SourceID:  last.SourceID,
				UpdatedAt: &updatedAt,
			},
		}, nil
	}
}

// Close closes the Azure Blob sync session.
func (s *azureBlobSyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed Azure Blob document body.
func (s *azureBlobSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

// applyResume configures the session to skip every blob at or before the
// checkpoint anchor. The anchor must still exist in the current container
// listing; a missing anchor means the saved progress no longer describes the
// same source state.
func (s *azureBlobSyncSession) applyResume(ctx context.Context, checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	anchor := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if anchor == "" {
		return fmt.Errorf("azure blob sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}

	marker := ""
	for {
		objects, next, hasMore, err := s.connector.listBlobPage(ctx, s.connector.prefix, marker, int32(s.batchSize))
		if err != nil {
			return err
		}
		for _, object := range objects {
			if object.Name == anchor {
				s.skipUntil = anchor
				return nil
			}
			if object.Name > anchor {
				return fmt.Errorf("azure blob resume anchor %q was not found in the current listing: %w", anchor, ErrSyncResumeInvalid)
			}
		}
		if !hasMore {
			break
		}
		marker = next
	}
	return fmt.Errorf("azure blob resume anchor %q was not found in the current listing: %w", anchor, ErrSyncResumeInvalid)
}

type azureBlobPruneSession struct {
	documents  []SlimDocument
	batchSize  int
	batchIndex int
}

// NextBatch returns the next Azure Blob prune snapshot batch.
func (s *azureBlobPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.batchIndex >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	end := s.batchIndex + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	documents := s.documents[s.batchIndex:end]
	s.batchIndex = end
	return PruneBatch{Documents: documents}, nil
}

// Close closes the Azure Blob prune session.
func (s *azureBlobPruneSession) Close() error {
	return nil
}

type azureBlobFetchReference struct {
	Name string `json:"name"`
}

func includeAzureBlobObject(request SyncRequest, object azureBlobObject) bool {
	if request.FromBeginning {
		return true
	}
	if object.LastModified.IsZero() {
		return true
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := normalizedAzureBlobETag(object.ETag)
		stored, ok := request.Fingerprints[object.Name]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	return !beforeOrAtWindowStart(object.LastModified, request.WindowStart) && !afterWindowEnd(object.LastModified, request.WindowEnd)
}

func azureBlobExtension(name string) string {
	index := strings.LastIndex(name, ".")
	if index < 0 {
		return ""
	}
	return "." + strings.ToLower(name[index+1:])
}

func azureBlobSupportedExtension(name string, allowImages bool) bool {
	extension := azureBlobExtension(name)
	if _, ok := azureBlobTextExtensions[extension]; ok {
		return true
	}
	if allowImages {
		_, ok := azureBlobImageExtensions[extension]
		return ok
	}
	return false
}

func normalizeAzureBlobPrefix(prefix string) string {
	return strings.TrimLeft(strings.TrimSpace(prefix), "/")
}

func normalizedAzureBlobETag(rawETag string) string {
	rawETag = strings.Trim(strings.TrimSpace(rawETag), `"`)
	if rawETag == "" {
		return ""
	}
	return contentFingerprint([]byte(rawETag))
}

// azureBlobETag returns the raw ETag with surrounding quotes stripped, matching
// the metadata emitted by the blob connector.
func azureBlobETag(rawETag string) string {
	return strings.Trim(strings.TrimSpace(rawETag), `"`)
}

func truncateAzureBlobMessage(msg string) string {
	if len(msg) > azureBlobErrorMessageLimit {
		return msg[:azureBlobErrorMessageLimit]
	}
	return msg
}
