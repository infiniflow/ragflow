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
	"fmt"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	defaultOCIStorageBatchSize     = 2
	defaultOCIStorageSizeThreshold = 20 * 1024 * 1024
	ociStorageListPageSize         = 1000
	ociStorageSource               = "oci_storage"
)

var ociStorageHostLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// OCIStorageConnector reads objects from Oracle Cloud Infrastructure Object
// Storage through its S3-compatible API.
type OCIStorageConnector struct {
	bucketName    string
	prefix        string
	namespace     string
	region        string
	accessKeyID   string
	secretKey     string
	allowImages   bool
	batchSize     int
	sizeThreshold int64

	client         *s3.Client
	listObjects    func(ctx context.Context, startAfter string, maxKeys int32) ([]ociStorageObject, string, bool, error)
	downloadObject func(ctx context.Context, key string, sizeThreshold int64) ([]byte, error)
}

// NewOCIStorageConnector creates an OCI Object Storage connector from
// Python-compatible config.
func NewOCIStorageConnector(config map[string]any) (*OCIStorageConnector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultOCIStorageBatchSize)
	sizeThreshold := int64(configInt(config["size_threshold"], defaultOCIStorageSizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = defaultOCIStorageSizeThreshold
	}
	return &OCIStorageConnector{
		bucketName:    strings.TrimSpace(stringConfig(config["bucket_name"])),
		prefix:        normalizeOCIStoragePrefix(stringConfig(config["prefix"])),
		namespace:     strings.TrimSpace(stringConfig(credentials["namespace"])),
		region:        strings.TrimSpace(stringConfig(credentials["region"])),
		accessKeyID:   strings.TrimSpace(stringConfig(credentials["access_key_id"])),
		secretKey:     stringConfig(credentials["secret_access_key"]),
		allowImages:   configBoolDefault(config["allow_images"], false),
		batchSize:     batchSize,
		sizeThreshold: sizeThreshold,
	}, nil
}

// Validate validates OCI Object Storage connector settings and credentials.
func (c *OCIStorageConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("OCI Storage connector is nil")
	}
	if c.bucketName == "" {
		return fmt.Errorf("No bucket name was provided in connector settings.")
	}
	if c.namespace == "" || c.region == "" || c.accessKeyID == "" || c.secretKey == "" {
		return fmt.Errorf("OCI Storage credentials are required")
	}
	if err := validateOCIStorageNamespace(c.namespace); err != nil {
		return err
	}
	if err := validateOCIStorageRegion(c.region); err != nil {
		return err
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if _, err := c.ensureClient(ctx); err != nil {
		return err
	}
	return nil
}

// ValidateConnectorSetting validates OCI Object Storage settings from an
// unsaved config.
func (c *OCIStorageConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if err := c.Validate(ctx); err != nil {
		return err
	}
	_, _, _, err := c.listObjectPage(ctx, "", 1)
	if err != nil {
		return fmt.Errorf("access OCI bucket %q: %w", c.bucketName, err)
	}
	return nil
}

// OpenSync opens one OCI Object Storage sync session.
func (c *OCIStorageConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	session := &ociStorageSyncSession{
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

// OpenPrune opens one complete OCI Object Storage prune snapshot session.
func (c *OCIStorageConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	return &ociStoragePruneSession{connector: c, batchSize: c.batchSize}, nil
}

// Fetch downloads an OCI Object Storage object body.
func (c *OCIStorageConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch ociStorageFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	return c.download(ctx, fetch.Key)
}

func (c *OCIStorageConnector) ensureClient(ctx context.Context) (*s3.Client, error) {
	if err := validateOCIStorageNamespace(c.namespace); err != nil {
		return nil, err
	}
	if err := validateOCIStorageRegion(c.region); err != nil {
		return nil, err
	}
	if c.client != nil {
		return c.client, nil
	}
	cfg, err := awssdkconfig.LoadDefaultConfig(
		ctx,
		awssdkconfig.WithRegion(c.region),
		awssdkconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.accessKeyID, c.secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load OCI Storage config: %w", err)
	}
	c.client = s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(ociStorageEndpoint(c.namespace, c.region))
		options.UsePathStyle = true
	})
	return c.client, nil
}

func (c *OCIStorageConnector) listObjectPage(ctx context.Context, startAfter string, maxKeys int32) ([]ociStorageObject, string, bool, error) {
	if c.listObjects != nil {
		return c.listObjects(ctx, startAfter, maxKeys)
	}
	client, err := c.ensureClient(ctx)
	if err != nil {
		return nil, "", false, err
	}
	input := &s3.ListObjectsV2Input{
		Bucket:     aws.String(c.bucketName),
		Prefix:     aws.String(c.prefix),
		StartAfter: aws.String(startAfter),
	}
	if maxKeys > 0 {
		input.MaxKeys = aws.Int32(maxKeys)
	}
	output, err := client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, "", false, err
	}
	objects := make([]ociStorageObject, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, ociStorageObjectFromS3(object))
	}
	nextStartAfter := ""
	if len(objects) > 0 {
		nextStartAfter = ociStorageSourceID(c.bucketName, objects[len(objects)-1].Key)
	}
	return objects, nextStartAfter, aws.ToBool(output.IsTruncated), nil
}

func (c *OCIStorageConnector) download(ctx context.Context, key string) ([]byte, error) {
	if c.downloadObject != nil {
		return c.downloadObject(ctx, key, c.sizeThreshold)
	}
	client, err := c.ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer output.Body.Close()
	return readOCIBody(output.Body, key, c.sizeThreshold)
}

type ociStorageObject struct {
	Key          string
	LastModified time.Time
	Size         int64
	ETag         string
}

func ociStorageObjectFromS3(object types.Object) ociStorageObject {
	updatedAt := time.Time{}
	if object.LastModified != nil {
		updatedAt = object.LastModified.UTC()
	}
	return ociStorageObject{
		Key:          aws.ToString(object.Key),
		LastModified: updatedAt,
		Size:         aws.ToInt64(object.Size),
		ETag:         aws.ToString(object.ETag),
	}
}

type ociStorageSyncSession struct {
	connector  *OCIStorageConnector
	request    SyncRequest
	batchSize  int
	startAfter string
	done       bool
}

// NextBatch returns the next OCI Object Storage document batch.
func (s *ociStorageSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	for {
		if s.done {
			return SyncBatch{}, io.EOF
		}
		previousStartAfter := s.startAfter
		objects, nextStartAfter, hasMore, err := s.connector.listObjectPage(ctx, s.startAfter, int32(s.batchSize))
		if err != nil {
			return SyncBatch{}, err
		}
		if !hasMore {
			s.done = true
		}
		if nextStartAfter != "" {
			s.startAfter = strings.TrimPrefix(nextStartAfter, ociStorageSourceID(s.connector.bucketName, ""))
		}
		if hasMore && s.startAfter == previousStartAfter {
			return SyncBatch{}, fmt.Errorf("OCI Storage listing did not advance from %q", previousStartAfter)
		}

		documents := make([]SourceDocument, 0, len(objects))
		for _, object := range objects {
			sourceID := ociStorageSourceID(s.connector.bucketName, object.Key)
			if !includeOCIStorageObject(s.request, sourceID, object) {
				continue
			}
			document, ok := s.connector.sourceDocument(sourceID, object)
			if ok {
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

// Close closes the OCI Object Storage sync session.
func (s *ociStorageSyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed OCI Object Storage document body.
func (s *ociStorageSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *ociStorageSyncSession) applyResume(ctx context.Context, checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	prefix := ociStorageSourceID(s.connector.bucketName, "")
	if sourceID == "" || !strings.HasPrefix(sourceID, prefix) {
		return fmt.Errorf("oci storage sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	anchor := strings.TrimPrefix(sourceID, prefix)
	if anchor == "" {
		return fmt.Errorf("oci storage sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}

	startAfter := ""
	for {
		objects, nextStartAfter, hasMore, err := s.connector.listObjectPage(ctx, startAfter, int32(s.batchSize))
		if err != nil {
			return err
		}
		for _, object := range objects {
			if object.Key == anchor {
				s.startAfter = anchor
				return nil
			}
			if object.Key > anchor {
				return fmt.Errorf("oci storage resume anchor %q was not found in the current listing: %w", anchor, ErrSyncResumeInvalid)
			}
		}
		if !hasMore {
			break
		}
		startAfter = strings.TrimPrefix(nextStartAfter, prefix)
	}
	return fmt.Errorf("oci storage resume anchor %q was not found in the current listing: %w", anchor, ErrSyncResumeInvalid)
}

func (c *OCIStorageConnector) sourceDocument(sourceID string, object ociStorageObject) (SourceDocument, bool) {
	if object.Key == "" || strings.HasSuffix(object.Key, "/") || (!c.allowImages && object.isImage()) {
		return SourceDocument{}, false
	}
	fileName := path.Base(object.Key)
	fetch := ociStorageFetchReference{Key: object.Key}
	fetchKey, _ := json.Marshal(fetch)
	return SourceDocument{
		SourceID:           sourceID,
		SemanticIdentifier: c.semanticIdentifier(object.Key, fileName),
		Extension:          strings.ToLower(filepath.Ext(fileName)),
		FetchRef:           &FetchReference{Key: string(fetchKey), SizeHint: object.Size},
		UpdatedAt:          object.LastModified,
		SizeBytes:          object.Size,
		Metadata: map[string]any{
			"url": ociStorageConsoleURL(c.namespace, c.region, c.bucketName, object.Key),
		},
		Fingerprint: normalizedOCIStorageETag(object.ETag),
	}, true
}

func (c *OCIStorageConnector) semanticIdentifier(key, fileName string) string {
	relativePath := key
	if c.prefix != "" {
		relativePath = strings.TrimPrefix(key, c.prefix)
	}
	if relativePath == "" {
		return fileName
	}
	return strings.ReplaceAll(relativePath, "/", " / ")
}

func (o ociStorageObject) isImage() bool {
	switch strings.ToLower(filepath.Ext(o.Key)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".heif":
		return true
	default:
		return false
	}
}

type ociStoragePruneSession struct {
	connector  *OCIStorageConnector
	batchSize  int
	startAfter string
	done       bool
	buffer     []SlimDocument
}

// NextBatch returns the next OCI Object Storage prune snapshot batch.
func (s *ociStoragePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	if len(s.buffer) > 0 {
		n := s.batchSize
		if n > len(s.buffer) {
			n = len(s.buffer)
		}
		documents = append(documents, s.buffer[:n]...)
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize && !s.done {
		page, nextStartAfter, hasMore, err := s.connector.listObjectPage(ctx, s.startAfter, ociStorageListPageSize)
		if err != nil {
			return PruneBatch{}, err
		}
		previousStartAfter := s.startAfter
		if hasMore {
			next := strings.TrimPrefix(nextStartAfter, ociStorageSourceID(s.connector.bucketName, ""))
			if next == "" || next == previousStartAfter {
				s.done = true
			} else {
				s.startAfter = next
			}
		} else {
			s.done = true
		}
		remaining := s.batchSize - len(documents)
		for _, object := range page {
			if object.Key == "" || strings.HasSuffix(object.Key, "/") {
				continue
			}
			doc := SlimDocument{SourceID: ociStorageSourceID(s.connector.bucketName, object.Key)}
			if remaining > 0 {
				documents = append(documents, doc)
				remaining--
			} else {
				s.buffer = append(s.buffer, doc)
			}
		}
		if remaining <= 0 {
			break
		}
	}
	if len(documents) == 0 && len(s.buffer) == 0 && s.done {
		return PruneBatch{}, io.EOF
	}
	return PruneBatch{Documents: documents}, nil
}

// Close closes the OCI Object Storage prune session.
func (s *ociStoragePruneSession) Close() error {
	return nil
}

type ociStorageFetchReference struct {
	Key string `json:"key"`
}

func includeOCIStorageObject(request SyncRequest, sourceID string, object ociStorageObject) bool {
	if request.FromBeginning {
		return true
	}
	if object.LastModified.IsZero() {
		return true
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := normalizedOCIStorageETag(object.ETag)
		stored, ok := request.Fingerprints[sourceID]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	return !beforeOrAtWindowStart(object.LastModified, request.WindowStart) && !afterWindowEnd(object.LastModified, request.WindowEnd)
}

func normalizeOCIStoragePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func validateOCIStorageNamespace(namespace string) error {
	if !ociStorageHostLabelPattern.MatchString(namespace) {
		return fmt.Errorf("invalid OCI Storage namespace %q: must be a lowercase DNS label containing only letters, numbers, and hyphens", namespace)
	}
	return nil
}

func validateOCIStorageRegion(region string) error {
	if !ociStorageHostLabelPattern.MatchString(region) {
		return fmt.Errorf("invalid OCI Storage region %q: must be a lowercase DNS label containing only letters, numbers, and hyphens", region)
	}
	return nil
}

func ociStorageEndpoint(namespace, region string) string {
	return fmt.Sprintf("https://%s.compat.objectstorage.%s.oraclecloud.com", namespace, region)
}

func ociStorageSourceID(bucketName, key string) string {
	return fmt.Sprintf("%s:%s:%s", ociStorageSource, bucketName, key)
}

func normalizedOCIStorageETag(rawETag string) string {
	rawETag = strings.Trim(strings.TrimSpace(rawETag), `"`)
	if rawETag == "" {
		return ""
	}
	return contentFingerprint([]byte(rawETag))
}

func ociStorageConsoleURL(namespace, region, bucketName, key string) string {
	return fmt.Sprintf("https://objectstorage.%s.oraclecloud.com/n/%s/b/%s/o/%s", region, namespace, bucketName, pathEscapeOCIStorageKey(key))
}

func pathEscapeOCIStorageKey(key string) string {
	parts := strings.Split(key, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func readOCIBody(body io.Reader, key string, sizeThreshold int64) ([]byte, error) {
	limited := io.LimitReader(body, sizeThreshold+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > sizeThreshold {
		return nil, fmt.Errorf("%s exceeds size threshold of %d", key, sizeThreshold)
	}
	return data, nil
}
