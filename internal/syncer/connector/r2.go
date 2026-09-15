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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	defaultR2BatchSize     = 2
	defaultR2SizeThreshold = 20 * 1024 * 1024
	r2ListPageSize         = 1000
	r2Source               = "r2"
)

// R2Connector reads objects from Cloudflare R2 through its S3-compatible API.
type R2Connector struct {
	bucketName        string
	prefix            string
	accountID         string
	accessKeyID       string
	secretKey         string
	allowImages       bool
	batchSize         int
	sizeThreshold     int64
	europeanResidency bool

	client         *s3.Client
	listObjects    func(ctx context.Context, startAfter string, maxKeys int32) ([]r2Object, string, bool, error)
	downloadObject func(ctx context.Context, key string, sizeThreshold int64) ([]byte, error)
}

// NewR2Connector creates a Cloudflare R2 connector from connector config.
func NewR2Connector(config map[string]any) (*R2Connector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultR2BatchSize)
	sizeThreshold := int64(configInt(config["size_threshold"], defaultR2SizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = defaultR2SizeThreshold
	}
	return &R2Connector{
		bucketName:        strings.TrimSpace(stringConfig(config["bucket_name"])),
		prefix:            normalizeR2Prefix(stringConfig(config["prefix"])),
		accountID:         strings.TrimSpace(stringConfig(credentials["account_id"])),
		accessKeyID:       strings.TrimSpace(stringConfig(credentials["r2_access_key_id"])),
		secretKey:         stringConfig(credentials["r2_secret_access_key"]),
		allowImages:       configBoolDefault(config["allow_images"], false),
		europeanResidency: configBoolDefault(config["european_residency"], false),
		batchSize:         batchSize,
		sizeThreshold:     sizeThreshold,
	}, nil
}

// Validate validates Cloudflare R2 connector settings and credentials.
func (c *R2Connector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("Cloudflare R2 connector is nil")
	}
	if c.bucketName == "" {
		return fmt.Errorf("No bucket name was provided in connector settings.")
	}
	if c.accountID == "" || c.accessKeyID == "" || c.secretKey == "" {
		return fmt.Errorf("Cloudflare R2 credentials are required")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if _, err := c.ensureClient(ctx); err != nil {
		return err
	}
	return nil
}

// ValidateConnectorSetting validates Cloudflare R2 settings from an unsaved config.
func (c *R2Connector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if err := c.Validate(ctx); err != nil {
		return err
	}
	client, err := c.ensureClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(c.bucketName)})
	if err != nil {
		return fmt.Errorf("access Cloudflare R2 bucket %q: %w", c.bucketName, err)
	}
	return nil
}

// OpenSync opens one Cloudflare R2 sync session.
func (c *R2Connector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	session := &r2SyncSession{
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

// OpenPrune opens one complete Cloudflare R2 prune snapshot session.
func (c *R2Connector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	return &r2PruneSession{connector: c, batchSize: c.batchSize}, nil
}

// Fetch downloads a Cloudflare R2 object body.
func (c *R2Connector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch r2FetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	return c.download(ctx, fetch.Key)
}

func (c *R2Connector) ensureClient(ctx context.Context) (*s3.Client, error) {
	if c.client != nil {
		return c.client, nil
	}
	cfg, err := awssdkconfig.LoadDefaultConfig(
		ctx,
		awssdkconfig.WithRegion("auto"),
		awssdkconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.accessKeyID, c.secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load Cloudflare R2 config: %w", err)
	}
	c.client = s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(r2Endpoint(c.accountID, c.europeanResidency))
		options.UsePathStyle = true
	})
	return c.client, nil
}

func (c *R2Connector) listObjectPage(ctx context.Context, startAfter string, maxKeys int32) ([]r2Object, string, bool, error) {
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
	objects := make([]r2Object, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, r2ObjectFromS3(object))
	}
	nextStartAfter := ""
	if len(objects) > 0 {
		nextStartAfter = r2SourceID(c.bucketName, objects[len(objects)-1].Key)
	}
	return objects, nextStartAfter, aws.ToBool(output.IsTruncated), nil
}

func (c *R2Connector) download(ctx context.Context, key string) ([]byte, error) {
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
	return readR2Body(output.Body, key, c.sizeThreshold)
}

type r2Object struct {
	Key          string
	LastModified time.Time
	Size         int64
	ETag         string
}

func r2ObjectFromS3(object types.Object) r2Object {
	updatedAt := time.Time{}
	if object.LastModified != nil {
		updatedAt = object.LastModified.UTC()
	}
	return r2Object{
		Key:          aws.ToString(object.Key),
		LastModified: updatedAt,
		Size:         aws.ToInt64(object.Size),
		ETag:         aws.ToString(object.ETag),
	}
}

type r2SyncSession struct {
	connector  *R2Connector
	request    SyncRequest
	batchSize  int
	startAfter string
	done       bool
}

// NextBatch returns the next Cloudflare R2 document batch.
func (s *r2SyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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
			s.startAfter = strings.TrimPrefix(nextStartAfter, r2SourceID(s.connector.bucketName, ""))
		}
		if hasMore && s.startAfter == previousStartAfter {
			return SyncBatch{}, fmt.Errorf("Cloudflare R2 listing did not advance from %q", previousStartAfter)
		}

		documents := make([]SourceDocument, 0, len(objects))
		for _, object := range objects {
			sourceID := r2SourceID(s.connector.bucketName, object.Key)
			if !includeR2Object(s.request, sourceID, object) {
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
		cursor := r2SourceID(s.connector.bucketName, s.startAfter)
		return SyncBatch{
			Documents: documents,
			Checkpoint: &SyncCheckpoint{
				Cursor:    cursor,
				SourceID:  cursor,
				UpdatedAt: &updatedAt,
			},
		}, nil
	}
}

// Close closes the Cloudflare R2 sync session.
func (s *r2SyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed Cloudflare R2 document body.
func (s *r2SyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *r2SyncSession) applyResume(ctx context.Context, checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	prefix := r2SourceID(s.connector.bucketName, "")
	if sourceID == "" || !strings.HasPrefix(sourceID, prefix) {
		return fmt.Errorf("cloudflare r2 sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	anchor := strings.TrimPrefix(sourceID, prefix)
	if anchor == "" {
		return fmt.Errorf("cloudflare r2 sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
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
				return fmt.Errorf("cloudflare r2 resume anchor %q was not found in the current listing: %w", anchor, ErrSyncResumeInvalid)
			}
		}
		if !hasMore {
			break
		}
		startAfter = strings.TrimPrefix(nextStartAfter, prefix)
	}
	return fmt.Errorf("cloudflare r2 resume anchor %q was not found in the current listing: %w", anchor, ErrSyncResumeInvalid)
}

func (c *R2Connector) sourceDocument(sourceID string, object r2Object) (SourceDocument, bool) {
	if object.Key == "" || strings.HasSuffix(object.Key, "/") || (!c.allowImages && object.isImage()) {
		return SourceDocument{}, false
	}
	fileName := path.Base(object.Key)
	fetch := r2FetchReference{Key: object.Key}
	fetchKey, _ := json.Marshal(fetch)
	return SourceDocument{
		SourceID:           sourceID,
		SemanticIdentifier: c.semanticIdentifier(object.Key, fileName),
		Extension:          strings.ToLower(filepath.Ext(fileName)),
		FetchRef:           &FetchReference{Key: string(fetchKey), SizeHint: object.Size},
		UpdatedAt:          object.LastModified,
		SizeBytes:          object.Size,
		Metadata: map[string]any{
			"url": r2ConsoleURL(c.accountID, c.europeanResidency, c.bucketName, object.Key),
		},
		Fingerprint: normalizedR2ETag(object.ETag),
	}, true
}

func (c *R2Connector) semanticIdentifier(key, fileName string) string {
	relativePath := key
	if c.prefix != "" {
		relativePath = strings.TrimPrefix(key, c.prefix)
	}
	if relativePath == "" {
		return fileName
	}
	return strings.ReplaceAll(relativePath, "/", " / ")
}

func (o r2Object) isImage() bool {
	switch strings.ToLower(filepath.Ext(o.Key)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".heif":
		return true
	default:
		return false
	}
}

type r2PruneSession struct {
	connector  *R2Connector
	batchSize  int
	startAfter string
	done       bool
	buffer     []SlimDocument
}

// NextBatch returns the next Cloudflare R2 prune snapshot batch.
func (s *r2PruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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
		page, nextStartAfter, hasMore, err := s.connector.listObjectPage(ctx, s.startAfter, r2ListPageSize)
		if err != nil {
			return PruneBatch{}, err
		}
		previousStartAfter := s.startAfter
		if hasMore {
			next := strings.TrimPrefix(nextStartAfter, r2SourceID(s.connector.bucketName, ""))
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
			doc := SlimDocument{SourceID: r2SourceID(s.connector.bucketName, object.Key)}
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

// Close closes the Cloudflare R2 prune session.
func (s *r2PruneSession) Close() error {
	return nil
}

type r2FetchReference struct {
	Key string `json:"key"`
}

func includeR2Object(request SyncRequest, sourceID string, object r2Object) bool {
	if request.FromBeginning {
		return true
	}
	if object.LastModified.IsZero() {
		return true
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := normalizedR2ETag(object.ETag)
		stored, ok := request.Fingerprints[sourceID]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	return !beforeOrAtWindowStart(object.LastModified, request.WindowStart) && !afterWindowEnd(object.LastModified, request.WindowEnd)
}

func normalizeR2Prefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func r2Endpoint(accountID string, europeanResidency bool) string {
	subdomain := ""
	if europeanResidency {
		subdomain = "eu."
	}
	return fmt.Sprintf("https://%s.%sr2.cloudflarestorage.com", accountID, subdomain)
}

func r2SourceID(bucketName, key string) string {
	return fmt.Sprintf("%s:%s:%s", r2Source, bucketName, key)
}

func normalizedR2ETag(rawETag string) string {
	rawETag = strings.Trim(strings.TrimSpace(rawETag), `"`)
	if rawETag == "" {
		return ""
	}
	return contentFingerprint([]byte(rawETag))
}

func r2ConsoleURL(accountID string, europeanResidency bool, bucketName, key string) string {
	region := "default"
	if europeanResidency {
		region = "eu"
	}
	return fmt.Sprintf("https://dash.cloudflare.com/%s/r2/%s/buckets/%s/objects/%s/details", accountID, region, bucketName, pathEscapeR2Key(key))
}

func pathEscapeR2Key(key string) string {
	parts := strings.Split(key, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func readR2Body(body io.Reader, key string, sizeThreshold int64) ([]byte, error) {
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
