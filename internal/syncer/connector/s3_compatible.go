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
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	defaultS3CompatibleRegion = "us-east-1"
	s3CompatibleSource        = "s3_compatible"
)

// S3CompatibleConnector reads objects from any S3-compatible object store.
type S3CompatibleConnector struct {
	bucketName      string
	prefix          string
	endpointURL     string
	region          string
	accessKeyID     string
	secretKey       string
	addressingStyle string
	allowImages     bool
	batchSize       int

	sizeThreshold  int64
	client         *s3.Client
	listObjects    func(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error)
	downloadObject func(ctx context.Context, key string, sizeThreshold int64) ([]byte, error)
}

// NewS3CompatibleConnector creates an S3-compatible connector from
// Python-compatible config.
func NewS3CompatibleConnector(config map[string]any) (*S3CompatibleConnector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultS3BatchSize)
	sizeThreshold := int64(configInt(config["size_threshold"], defaultS3SizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = defaultS3SizeThreshold
	}
	addressingStyle := firstNonEmpty(stringConfig(credentials["addressing_style"]), "virtual")
	return &S3CompatibleConnector{
		bucketName:      strings.TrimSpace(stringConfig(config["bucket_name"])),
		prefix:          normalizeS3Prefix(stringConfig(config["prefix"])),
		endpointURL:     strings.TrimSpace(stringConfig(credentials["endpoint_url"])),
		region:          firstNonEmpty(stringConfig(credentials["region"]), defaultS3CompatibleRegion),
		accessKeyID:     strings.TrimSpace(stringConfig(credentials["aws_access_key_id"])),
		secretKey:       stringConfig(credentials["aws_secret_access_key"]),
		addressingStyle: addressingStyle,
		allowImages:     configBoolDefault(config["allow_images"], false),
		batchSize:       batchSize,
		sizeThreshold:   sizeThreshold,
	}, nil
}

// Validate validates S3-compatible connector settings and credentials.
func (c *S3CompatibleConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("S3-compatible connector is nil")
	}
	if c.bucketName == "" {
		return fmt.Errorf("No bucket name was provided in connector settings.")
	}
	if err := validateS3CompatibleEndpoint(c.endpointURL); err != nil {
		return err
	}
	if c.accessKeyID == "" || c.secretKey == "" {
		return fmt.Errorf("S3-compatible access key credentials are required")
	}
	if err := validateS3CompatibleAddressingStyle(c.addressingStyle); err != nil {
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

// ValidateConnectorSetting validates S3-compatible settings from an unsaved
// config.
func (c *S3CompatibleConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if err := c.Validate(ctx); err != nil {
		return err
	}
	_, _, _, err := c.listObjectPage(ctx, "", 1)
	if err != nil {
		return fmt.Errorf("access S3-compatible bucket %q: %w", c.bucketName, err)
	}
	return nil
}

// OpenSync opens one S3-compatible sync session.
func (c *S3CompatibleConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
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

// OpenPrune opens one complete S3-compatible prune snapshot session.
func (c *S3CompatibleConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	return &s3PruneSession{connector: c, batchSize: c.batchSize}, nil
}

// Fetch downloads an S3-compatible object body.
func (c *S3CompatibleConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch s3FetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	return c.download(ctx, fetch.Key)
}

func (c *S3CompatibleConnector) ensureClient(ctx context.Context) (*s3.Client, error) {
	if c.client != nil {
		return c.client, nil
	}
	if err := validateS3CompatibleEndpoint(c.endpointURL); err != nil {
		return nil, err
	}
	if err := validateS3CompatibleAddressingStyle(c.addressingStyle); err != nil {
		return nil, err
	}
	region := firstNonEmpty(c.region, defaultS3CompatibleRegion)
	cfg, err := awssdkconfig.LoadDefaultConfig(
		ctx,
		awssdkconfig.WithRegion(region),
		awssdkconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.accessKeyID, c.secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load S3-compatible config: %w", err)
	}
	c.client = s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(c.endpointURL)
		options.UsePathStyle = c.addressingStyle == "path"
	})
	return c.client, nil
}

func (c *S3CompatibleConnector) listObjectPage(ctx context.Context, startAfter string, maxKeys int32) ([]s3Object, string, bool, error) {
	return listS3ObjectPage(ctx, c.listObjects, c.ensureClient, c.bucketName, c.prefix, s3CompatibleSource, startAfter, maxKeys)
}

func (c *S3CompatibleConnector) download(ctx context.Context, key string) ([]byte, error) {
	return downloadS3Object(ctx, c.downloadObject, c.ensureClient, c.bucketName, key, c.sizeThreshold)
}

func (c *S3CompatibleConnector) sourceName() string {
	return "S3-compatible"
}

func (c *S3CompatibleConnector) sourceID(key string) string {
	return s3SourceID(s3CompatibleSource, c.bucketName, key)
}

func (c *S3CompatibleConnector) sourceDocument(sourceID string, object s3Object) (SourceDocument, bool) {
	return newS3SourceDocument(sourceID, object, c.prefix, c.allowImages, nil)
}

func validateS3CompatibleAddressingStyle(addressingStyle string) error {
	if addressingStyle != "virtual" && addressingStyle != "path" {
		return fmt.Errorf("unsupported S3-compatible addressing_style %q", addressingStyle)
	}
	return nil
}

func validateS3CompatibleEndpoint(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("S3-compatible endpoint_url is required")
	}
	if strings.ContainsAny(raw, " \t\r\n") || strings.Contains(raw, "#") {
		return fmt.Errorf("invalid S3-compatible endpoint_url %q", raw)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid S3-compatible endpoint_url %q: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid S3-compatible endpoint_url %q: scheme must be http or https", raw)
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return fmt.Errorf("invalid S3-compatible endpoint_url %q: host is required", raw)
	}
	if parsed.User != nil {
		return fmt.Errorf("invalid S3-compatible endpoint_url %q: userinfo is not allowed", raw)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("invalid S3-compatible endpoint_url %q: fragment is not allowed", raw)
	}
	return nil
}
