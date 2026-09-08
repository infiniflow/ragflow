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
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	defaultS3BatchSize     = 2
	defaultS3SizeThreshold = 20 * 1024 * 1024
	s3ListPageSize         = 1000
	s3Source               = "s3"
)

var awsRegionLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// S3Connector reads objects from AWS S3.
type S3Connector struct {
	bucketName  string
	prefix      string
	region      string
	authMethod  string
	accessKeyID string
	secretKey   string
	roleARN     string
	allowImages bool
	batchSize   int

	sizeThreshold  int64
	client         *s3.Client
	listObjects    func(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error)
	downloadObject func(ctx context.Context, key string, sizeThreshold int64) ([]byte, error)
}

// NewS3Connector creates an AWS S3 connector from Python-compatible config.
func NewS3Connector(config map[string]any) (*S3Connector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultS3BatchSize)
	sizeThreshold := int64(configInt(config["size_threshold"], defaultS3SizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = defaultS3SizeThreshold
	}
	return &S3Connector{
		bucketName:    strings.TrimSpace(stringConfig(config["bucket_name"])),
		prefix:        normalizeS3Prefix(stringConfig(config["prefix"])),
		region:        strings.TrimSpace(stringConfig(credentials["region"])),
		authMethod:    firstNonEmpty(stringConfig(credentials["authentication_method"]), "access_key"),
		accessKeyID:   strings.TrimSpace(stringConfig(credentials["aws_access_key_id"])),
		secretKey:     stringConfig(credentials["aws_secret_access_key"]),
		roleARN:       strings.TrimSpace(stringConfig(credentials["aws_role_arn"])),
		allowImages:   configBoolDefault(config["allow_images"], false),
		batchSize:     batchSize,
		sizeThreshold: sizeThreshold,
	}, nil
}

// Validate validates AWS S3 connector settings and credentials.
func (c *S3Connector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("S3 connector is nil")
	}
	if c.bucketName == "" {
		return fmt.Errorf("No bucket name was provided in connector settings.")
	}
	if err := validateS3AuthMethod(c.authMethod, c.accessKeyID, c.secretKey, c.roleARN); err != nil {
		return err
	}
	if c.region != "" {
		if err := validateAWSRegion(c.region); err != nil {
			return err
		}
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if _, err := c.ensureClient(ctx); err != nil {
		return err
	}
	return nil
}

// ValidateConnectorSetting validates AWS S3 settings from an unsaved config.
func (c *S3Connector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if err := c.Validate(ctx); err != nil {
		return err
	}
	_, _, _, err := c.listObjectPage(ctx, "", 1)
	if err != nil {
		return fmt.Errorf("access S3 bucket %q: %w", c.bucketName, err)
	}
	return nil
}

// OpenSync opens one AWS S3 sync session.
func (c *S3Connector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	session := &s3SyncSession{
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

// OpenPrune opens one complete AWS S3 prune snapshot session.
func (c *S3Connector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	return &s3PruneSession{connector: c, batchSize: c.batchSize}, nil
}

// Fetch downloads an AWS S3 object body.
func (c *S3Connector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch s3FetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	return c.download(ctx, fetch.Key)
}

func (c *S3Connector) ensureClient(ctx context.Context) (*s3.Client, error) {
	if c.client != nil {
		return c.client, nil
	}
	if c.region != "" {
		if err := validateAWSRegion(c.region); err != nil {
			return nil, err
		}
	}
	authMethod := firstNonEmpty(c.authMethod, "access_key")
	options := make([]func(*awssdkconfig.LoadOptions) error, 0, 2)
	if c.region != "" {
		options = append(options, awssdkconfig.WithRegion(c.region))
	}
	switch authMethod {
	case "access_key":
		options = append(options, awssdkconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.accessKeyID, c.secretKey, "")))
	case "iam_role":
		if c.roleARN == "" {
			return nil, fmt.Errorf("S3 aws_role_arn is required for iam_role authentication")
		}
	case "assume_role":
	default:
		return nil, fmt.Errorf("unsupported S3 authentication_method %q", authMethod)
	}
	cfg, err := awssdkconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("load AWS S3 config: %w", err)
	}
	if authMethod == "iam_role" {
		cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), c.roleARN))
	}
	c.client = s3.NewFromConfig(cfg)
	return c.client, nil
}

func (c *S3Connector) listObjectPage(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error) {
	return listS3ObjectPage(ctx, c.listObjects, c.ensureClient, c.bucketName, c.prefix, s3Source, startAfter, maxKeys)
}

func (c *S3Connector) download(ctx context.Context, key string) ([]byte, error) {
	return downloadS3Object(ctx, c.downloadObject, c.ensureClient, c.bucketName, key, c.sizeThreshold)
}

func listS3ObjectPage(ctx context.Context, listObjects func(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error), ensureClient func(context.Context) (*s3.Client, error), bucketName, prefix, source, startAfter string, maxKeys int32) ([]s3Object, string, bool, error) {
	if listObjects != nil {
		return listObjects(ctx, startAfter, maxKeys)
	}
	client, err := ensureClient(ctx)
	if err != nil {
		return nil, "", false, err
	}
	input := &s3.ListObjectsV2Input{
		Bucket:     aws.String(bucketName),
		Prefix:     aws.String(prefix),
		StartAfter: aws.String(startAfter),
	}
	if maxKeys > 0 {
		input.MaxKeys = aws.Int32(maxKeys)
	}
	output, err := client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, "", false, err
	}
	objects := make([]s3Object, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, s3ObjectFromAWS(object))
	}
	nextStartAfter := ""
	if len(objects) > 0 {
		nextStartAfter = s3SourceID(source, bucketName, objects[len(objects)-1].Key)
	}
	return objects, nextStartAfter, aws.ToBool(output.IsTruncated), nil
}

func downloadS3Object(ctx context.Context, downloadObject func(ctx context.Context, key string, sizeThreshold int64) ([]byte, error), ensureClient func(context.Context) (*s3.Client, error), bucketName, key string, sizeThreshold int64) ([]byte, error) {
	if downloadObject != nil {
		return downloadObject(ctx, key, sizeThreshold)
	}
	client, err := ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer output.Body.Close()
	return readS3Body(output.Body, key, sizeThreshold)
}

type s3Object struct {
	Key          string
	LastModified time.Time
	Size         int64
	ETag         string
}

func s3ObjectFromAWS(object types.Object) s3Object {
	updatedAt := time.Time{}
	if object.LastModified != nil {
		updatedAt = object.LastModified.UTC()
	}
	return s3Object{
		Key:          aws.ToString(object.Key),
		LastModified: updatedAt,
		Size:         aws.ToInt64(object.Size),
		ETag:         aws.ToString(object.ETag),
	}
}

type s3ObjectStore interface {
	sourceName() string
	listObjectPage(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error)
	sourceID(key string) string
	sourceDocument(sourceID string, object s3Object) (SourceDocument, bool)
	Fetch(ctx context.Context, ref FetchReference) ([]byte, error)
}

type s3SyncSession struct {
	connector  s3ObjectStore
	request    SyncRequest
	batchSize  int
	startAfter string
	done       bool
}

// NextBatch returns the next S3 or S3-compatible document batch.
func (s *s3SyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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
			s.startAfter = strings.TrimPrefix(nextStartAfter, s.connector.sourceID(""))
		}
		if hasMore && s.startAfter == previousStartAfter {
			return SyncBatch{}, fmt.Errorf("%s listing did not advance from %q", s.connector.sourceName(), previousStartAfter)
		}

		documents := make([]SourceDocument, 0, len(objects))
		for _, object := range objects {
			sourceID := s.connector.sourceID(object.Key)
			if !includeS3Object(s.request, sourceID, object) {
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

// Close closes the S3 or S3-compatible sync session.
func (s *s3SyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed S3 or S3-compatible object body.
func (s *s3SyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *s3SyncSession) applyResume(ctx context.Context, checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	prefix := s.connector.sourceID("")
	if sourceID == "" || !strings.HasPrefix(sourceID, prefix) {
		return fmt.Errorf("%s sync checkpoint has no source anchor: %w", s.connector.sourceName(), ErrSyncResumeInvalid)
	}
	anchor := strings.TrimPrefix(sourceID, prefix)
	if anchor == "" {
		return fmt.Errorf("%s sync checkpoint has no source anchor: %w", s.connector.sourceName(), ErrSyncResumeInvalid)
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
				return fmt.Errorf("%s resume anchor %q was not found in the current listing: %w", s.connector.sourceName(), anchor, ErrSyncResumeInvalid)
			}
		}
		if !hasMore {
			break
		}
		startAfter = strings.TrimPrefix(nextStartAfter, prefix)
	}
	return fmt.Errorf("%s resume anchor %q was not found in the current listing: %w", s.connector.sourceName(), anchor, ErrSyncResumeInvalid)
}

func (c *S3Connector) sourceName() string {
	return "S3"
}

func (c *S3Connector) sourceID(key string) string {
	return s3SourceID(s3Source, c.bucketName, key)
}

func (c *S3Connector) sourceDocument(sourceID string, object s3Object) (SourceDocument, bool) {
	return newS3SourceDocument(sourceID, object, c.prefix, c.allowImages, map[string]any{
		"url": s3ConsoleURL(c.region, c.bucketName, object.Key),
	})
}

func validateS3AuthMethod(authMethod, accessKeyID, secretKey, roleARN string) error {
	switch authMethod {
	case "access_key":
		if accessKeyID == "" || secretKey == "" {
			return fmt.Errorf("S3 access key credentials are required")
		}
	case "iam_role":
		if roleARN == "" {
			return fmt.Errorf("S3 aws_role_arn is required for iam_role authentication")
		}
	case "assume_role":
	default:
		return fmt.Errorf("unsupported S3 authentication_method %q", authMethod)
	}
	return nil
}

func validateAWSRegion(region string) error {
	if !awsRegionLabelPattern.MatchString(region) {
		return fmt.Errorf("invalid AWS region %q: must be a lowercase DNS label containing only letters, numbers, and hyphens", region)
	}
	return nil
}

type s3PruneSession struct {
	connector  s3ObjectStore
	batchSize  int
	startAfter string
	done       bool
	buffer     []SlimDocument
}

// NextBatch returns the next S3 or S3-compatible prune snapshot batch.
func (s *s3PruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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
		page, nextStartAfter, hasMore, err := s.connector.listObjectPage(ctx, s.startAfter, s3ListPageSize)
		if err != nil {
			return PruneBatch{}, err
		}
		previousStartAfter := s.startAfter
		if hasMore {
			next := strings.TrimPrefix(nextStartAfter, s.connector.sourceID(""))
			if next == "" || next == previousStartAfter {
				return PruneBatch{}, fmt.Errorf("%s listing did not advance from %q", s.connector.sourceName(), previousStartAfter)
			}
			s.startAfter = next
		} else {
			s.done = true
		}
		remaining := s.batchSize - len(documents)
		for _, object := range page {
			if object.Key == "" || strings.HasSuffix(object.Key, "/") {
				continue
			}
			doc := SlimDocument{SourceID: s.connector.sourceID(object.Key)}
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

// Close closes the S3 or S3-compatible prune session.
func (s *s3PruneSession) Close() error {
	return nil
}

type s3FetchReference struct {
	Key string `json:"key"`
}

func includeS3Object(request SyncRequest, sourceID string, object s3Object) bool {
	if request.FromBeginning {
		return true
	}
	if object.LastModified.IsZero() {
		return true
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := normalizedS3ETag(object.ETag)
		stored, ok := request.Fingerprints[sourceID]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	return !beforeOrAtWindowStart(object.LastModified, request.WindowStart) && !afterWindowEnd(object.LastModified, request.WindowEnd)
}

func normalizeS3Prefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

func s3SourceID(source, bucketName, key string) string {
	return fmt.Sprintf("%s:%s:%s", source, bucketName, key)
}

func normalizedS3ETag(rawETag string) string {
	rawETag = strings.Trim(strings.TrimSpace(rawETag), `"`)
	if rawETag == "" {
		return ""
	}
	return contentFingerprint([]byte(rawETag))
}

func s3ConsoleURL(region, bucketName, key string) string {
	return fmt.Sprintf("https://s3.console.aws.amazon.com/s3/object/%s?region=%s&prefix=%s", bucketName, region, pathEscapeS3Key(key))
}

func pathEscapeS3Key(key string) string {
	parts := strings.Split(key, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func readS3Body(body io.Reader, key string, sizeThreshold int64) ([]byte, error) {
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

func newS3SourceDocument(sourceID string, object s3Object, prefix string, allowImages bool, metadata map[string]any) (SourceDocument, bool) {
	if object.Key == "" || strings.HasSuffix(object.Key, "/") || (!allowImages && object.isImage()) {
		return SourceDocument{}, false
	}
	fileName := path.Base(object.Key)
	fetch := s3FetchReference{Key: object.Key}
	fetchKey, _ := json.Marshal(fetch)
	return SourceDocument{
		SourceID:           sourceID,
		SemanticIdentifier: s3SemanticIdentifier(prefix, object.Key, fileName),
		Extension:          strings.ToLower(filepath.Ext(fileName)),
		FetchRef:           &FetchReference{Key: string(fetchKey), SizeHint: object.Size},
		UpdatedAt:          object.LastModified,
		SizeBytes:          object.Size,
		Metadata:           metadata,
		Fingerprint:        normalizedS3ETag(object.ETag),
	}, true
}

func s3SemanticIdentifier(prefix, key, fileName string) string {
	relativePath := key
	if prefix != "" {
		relativePath = strings.TrimPrefix(key, prefix)
	}
	if relativePath == "" {
		return fileName
	}
	return strings.ReplaceAll(relativePath, "/", " / ")
}

func (o s3Object) isImage() bool {
	switch strings.ToLower(filepath.Ext(o.Key)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".heif":
		return true
	default:
		return false
	}
}
