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
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	defaultGoogleCloudStorageBatchSize     = 32
	defaultGoogleCloudStorageSizeThreshold = 64 * 1024 * 1024
	googleCloudStorageEndpoint             = "https://storage.googleapis.com"
	googleCloudStorageSource               = "google_cloud_storage"
)

// GoogleCloudStorageConnector reads objects from Google Cloud Storage through
// the S3-compatible XML API used by the Python blob connector.
type GoogleCloudStorageConnector struct {
	bucketName    string
	prefix        string
	accessKeyID   string
	secretKey     string
	allowImages   bool
	batchSize     int
	sizeThreshold int64

	client         *s3.Client
	listObjects    func(ctx context.Context, startAfter string, maxKeys int32) ([]googleCloudStorageObject, string, bool, error)
	downloadObject func(ctx context.Context, key string, sizeThreshold int64) ([]byte, error)
}

// NewGoogleCloudStorageConnector creates a Google Cloud Storage connector from
// Python-compatible config.
func NewGoogleCloudStorageConnector(config map[string]any) (*GoogleCloudStorageConnector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultGoogleCloudStorageBatchSize)
	sizeThreshold := int64(configInt(config["size_threshold"], defaultGoogleCloudStorageSizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = defaultGoogleCloudStorageSizeThreshold
	}
	return &GoogleCloudStorageConnector{
		bucketName:    strings.TrimSpace(stringConfig(config["bucket_name"])),
		prefix:        normalizeGoogleCloudStoragePrefix(stringConfig(config["prefix"])),
		accessKeyID:   strings.TrimSpace(stringConfig(credentials["access_key_id"])),
		secretKey:     stringConfig(credentials["secret_access_key"]),
		allowImages:   configBoolDefault(config["allow_images"], false),
		batchSize:     batchSize,
		sizeThreshold: sizeThreshold,
	}, nil
}

// Validate validates Google Cloud Storage connector settings and credentials.
func (c *GoogleCloudStorageConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("Google Cloud Storage connector is nil")
	}
	if c.bucketName == "" {
		return fmt.Errorf("No bucket name was provided in connector settings.")
	}
	if c.accessKeyID == "" || c.secretKey == "" {
		return fmt.Errorf("Google Cloud Storage credentials are required")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if _, err := c.ensureClient(ctx); err != nil {
		return err
	}
	return nil
}

// ValidateConnectorSetting validates Google Cloud Storage settings from an
// unsaved config.
func (c *GoogleCloudStorageConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Google Cloud Storage sync session.
func (c *GoogleCloudStorageConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	session := &googleCloudStorageSyncSession{
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

// OpenPrune opens one complete Google Cloud Storage prune snapshot session.
func (c *GoogleCloudStorageConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	objects, err := c.collectObjects(ctx)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(objects))
	for _, object := range objects {
		if object.Key == "" || strings.HasSuffix(object.Key, "/") {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: googleCloudStorageSourceID(c.bucketName, object.Key)})
	}
	return &googleCloudStoragePruneSession{documents: documents, batchSize: c.batchSize}, nil
}

// Fetch downloads a Google Cloud Storage object body.
func (c *GoogleCloudStorageConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch googleCloudStorageFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	return c.download(ctx, fetch.Key)
}

func (c *GoogleCloudStorageConnector) ensureClient(ctx context.Context) (*s3.Client, error) {
	if c.client != nil {
		return c.client, nil
	}
	cfg, err := awssdkconfig.LoadDefaultConfig(
		ctx,
		awssdkconfig.WithRegion("auto"),
		awssdkconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.accessKeyID, c.secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load Google Cloud Storage config: %w", err)
	}
	c.client = s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(googleCloudStorageEndpoint)
		options.UsePathStyle = true
	})
	return c.client, nil
}

func (c *GoogleCloudStorageConnector) listObjectPage(ctx context.Context, startAfter string, maxKeys int32) ([]googleCloudStorageObject, string, bool, error) {
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
	objects := make([]googleCloudStorageObject, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, googleCloudStorageObjectFromS3(object))
	}
	nextStartAfter := ""
	if len(objects) > 0 {
		nextStartAfter = googleCloudStorageSourceID(c.bucketName, objects[len(objects)-1].Key)
	}
	return objects, nextStartAfter, aws.ToBool(output.IsTruncated), nil
}

func (c *GoogleCloudStorageConnector) collectObjects(ctx context.Context) ([]googleCloudStorageObject, error) {
	var objects []googleCloudStorageObject
	startAfter := ""
	for {
		page, nextStartAfter, hasMore, err := c.listObjectPage(ctx, startAfter, int32(c.batchSize))
		if err != nil {
			return nil, err
		}
		for _, object := range page {
			if object.Key == "" || strings.HasSuffix(object.Key, "/") {
				continue
			}
			objects = append(objects, object)
		}
		if !hasMore {
			break
		}
		startAfter = strings.TrimPrefix(nextStartAfter, googleCloudStorageSourceID(c.bucketName, ""))
		if startAfter == "" {
			break
		}
	}
	sort.SliceStable(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})
	return objects, nil
}

func (c *GoogleCloudStorageConnector) download(ctx context.Context, key string) ([]byte, error) {
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
	return readGoogleCloudStorageBody(output.Body, key, c.sizeThreshold)
}

type googleCloudStorageObject struct {
	Key          string
	LastModified time.Time
	Size         int64
	ETag         string
}

func googleCloudStorageObjectFromS3(object types.Object) googleCloudStorageObject {
	updatedAt := time.Time{}
	if object.LastModified != nil {
		updatedAt = object.LastModified.UTC()
	}
	return googleCloudStorageObject{
		Key:          aws.ToString(object.Key),
		LastModified: updatedAt,
		Size:         aws.ToInt64(object.Size),
		ETag:         aws.ToString(object.ETag),
	}
}

type googleCloudStorageSyncSession struct {
	connector  *GoogleCloudStorageConnector
	request    SyncRequest
	batchSize  int
	startAfter string
	done       bool
}

// NextBatch returns the next Google Cloud Storage document batch.
func (s *googleCloudStorageSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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
			s.startAfter = strings.TrimPrefix(nextStartAfter, googleCloudStorageSourceID(s.connector.bucketName, ""))
		}
		if hasMore && s.startAfter == previousStartAfter {
			return SyncBatch{}, fmt.Errorf("Google Cloud Storage listing did not advance from %q", previousStartAfter)
		}

		documents := make([]SourceDocument, 0, len(objects))
		for _, object := range objects {
			sourceID := googleCloudStorageSourceID(s.connector.bucketName, object.Key)
			if !includeGoogleCloudStorageObject(s.request, sourceID, object) {
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

// Close closes the Google Cloud Storage sync session.
func (s *googleCloudStorageSyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed Google Cloud Storage document body.
func (s *googleCloudStorageSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *googleCloudStorageSyncSession) applyResume(ctx context.Context, checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	prefix := googleCloudStorageSourceID(s.connector.bucketName, "")
	if sourceID == "" || !strings.HasPrefix(sourceID, prefix) {
		return fmt.Errorf("google cloud storage sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	anchor := strings.TrimPrefix(sourceID, prefix)
	if anchor == "" {
		return fmt.Errorf("google cloud storage sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
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
				return fmt.Errorf("google cloud storage resume anchor %q was not found in the current listing: %w", anchor, ErrSyncResumeInvalid)
			}
		}
		if !hasMore {
			break
		}
		startAfter = strings.TrimPrefix(nextStartAfter, prefix)
	}
	return fmt.Errorf("google cloud storage resume anchor %q was not found in the current listing: %w", anchor, ErrSyncResumeInvalid)
}

func (c *GoogleCloudStorageConnector) sourceDocument(sourceID string, object googleCloudStorageObject) (SourceDocument, bool) {
	if object.Key == "" || strings.HasSuffix(object.Key, "/") || (!c.allowImages && object.isImage()) {
		return SourceDocument{}, false
	}
	fileName := path.Base(object.Key)
	fetch := googleCloudStorageFetchReference{Key: object.Key}
	fetchKey, _ := json.Marshal(fetch)
	return SourceDocument{
		SourceID:           sourceID,
		SemanticIdentifier: c.semanticIdentifier(object.Key, fileName),
		Extension:          strings.ToLower(filepath.Ext(fileName)),
		FetchRef:           &FetchReference{Key: string(fetchKey), SizeHint: object.Size},
		UpdatedAt:          object.LastModified,
		SizeBytes:          object.Size,
		Metadata: map[string]any{
			"url": googleCloudStorageConsoleURL(c.bucketName, object.Key),
		},
		Fingerprint: normalizedGoogleCloudStorageETag(object.ETag),
	}, true
}

func (c *GoogleCloudStorageConnector) semanticIdentifier(key, fileName string) string {
	relativePath := key
	if c.prefix != "" {
		relativePath = strings.TrimPrefix(key, c.prefix)
	}
	if relativePath == "" {
		return fileName
	}
	return strings.ReplaceAll(relativePath, "/", " / ")
}

func (o googleCloudStorageObject) isImage() bool {
	switch strings.ToLower(filepath.Ext(o.Key)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".heif":
		return true
	default:
		return false
	}
}

type googleCloudStoragePruneSession struct {
	documents  []SlimDocument
	batchSize  int
	batchIndex int
}

// NextBatch returns the next Google Cloud Storage prune snapshot batch.
func (s *googleCloudStoragePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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

// Close closes the Google Cloud Storage prune session.
func (s *googleCloudStoragePruneSession) Close() error {
	return nil
}

type googleCloudStorageFetchReference struct {
	Key string `json:"key"`
}

func includeGoogleCloudStorageObject(request SyncRequest, sourceID string, object googleCloudStorageObject) bool {
	if request.FromBeginning {
		return true
	}
	if object.LastModified.IsZero() {
		return true
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := normalizedGoogleCloudStorageETag(object.ETag)
		stored, ok := request.Fingerprints[sourceID]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	return !beforeOrAtWindowStart(object.LastModified, request.WindowStart) && !afterWindowEnd(object.LastModified, request.WindowEnd)
}

func normalizeGoogleCloudStoragePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func googleCloudStorageSourceID(bucketName, key string) string {
	return fmt.Sprintf("%s:%s:%s", googleCloudStorageSource, bucketName, key)
}

func normalizedGoogleCloudStorageETag(rawETag string) string {
	rawETag = strings.Trim(strings.TrimSpace(rawETag), `"`)
	if rawETag == "" {
		return ""
	}
	return contentFingerprint([]byte(rawETag))
}

func googleCloudStorageConsoleURL(bucketName, key string) string {
	return fmt.Sprintf("https://console.cloud.google.com/storage/browser/_details/%s/%s", bucketName, pathEscapeGoogleCloudStorageKey(key))
}

func pathEscapeGoogleCloudStorageKey(key string) string {
	parts := strings.Split(key, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func readGoogleCloudStorageBody(body io.Reader, key string, sizeThreshold int64) ([]byte, error) {
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
