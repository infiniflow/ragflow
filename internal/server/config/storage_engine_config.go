//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// StorageConfig holds all storage-related configurations
type StorageConfig struct {
	Minio MinioConfig `mapstructure:"minio"`
	S3    S3Config    `mapstructure:"s3"`
	OSS   OSSConfig   `mapstructure:"oss"`
	GCS   GCSConfig   `mapstructure:"gcs"`
}

func (c *Config) ParseStorageEngineConfig(v *viper.Viper) error {
	switch c.general.StorageEngine {
	case "minio":
		c.parseMinioConfig(v)
	case "s3":
		c.parseS3Config(v)
	case "oss":
		c.parseOSSConfig(v)
	case "gcs":
		c.parseGCSConfig(v)
	default:
		return fmt.Errorf("invalid storage type: %s", c.general.StorageEngine)
	}

	return nil
}

// MinioConfig holds MinIO storage configuration
type MinioConfig struct {
	Host       string `mapstructure:"host"`        // MinIO server host (e.g., "localhost:9000")
	User       string `mapstructure:"user"`        // Access key
	Password   string `mapstructure:"password"`    // Secret key
	Bucket     string `mapstructure:"bucket"`      // Default bucket (optional)
	PrefixPath string `mapstructure:"prefix_path"` // Path prefix (optional)
	Secure     bool   `mapstructure:"secure"`      // Use HTTPS
	Verify     bool   `mapstructure:"verify"`      // Verify SSL certificates
	Region     string `mapstructure:"region"`      // optional
}

func (c *Config) parseMinioConfig(v *viper.Viper) {
	// Default MinIO config
	c.storageEngine.Minio.Host = "localhost:23817"
	c.storageEngine.Minio.User = "rag_flow"
	c.storageEngine.Minio.Password = "infini_rag_flow"
	c.storageEngine.Minio.Bucket = ""
	c.storageEngine.Minio.PrefixPath = ""
	c.storageEngine.Minio.Secure = false
	c.storageEngine.Minio.Verify = false
	c.storageEngine.Minio.Region = ""

	if !v.IsSet("minio") {
		return
	}
	sub := v.Sub("minio")
	if sub == nil {
		return
	}

	if sub.IsSet("host") {
		c.storageEngine.Minio.Host = sub.GetString("host")
	}

	if sub.IsSet("user") {
		c.storageEngine.Minio.User = sub.GetString("user")
	}

	if sub.IsSet("password") {
		c.storageEngine.Minio.Password = sub.GetString("password")
	}

	if sub.IsSet("bucket") {
		c.storageEngine.Minio.Bucket = sub.GetString("bucket")
	}

	if sub.IsSet("prefix_path") {
		c.storageEngine.Minio.PrefixPath = sub.GetString("prefix_path")
	}

	if sub.IsSet("secure") {
		c.storageEngine.Minio.Secure = sub.GetBool("secure")
	}

	if sub.IsSet("verify") {
		c.storageEngine.Minio.Verify = sub.GetBool("verify")
	}

	if sub.IsSet("region") {
		c.storageEngine.Minio.Region = sub.GetString("region")
	}
}

// S3Config holds AWS S3 storage configuration
type S3Config struct {
	AccessKey        string `mapstructure:"access_key"`        // AWS Access Key ID
	SecretKey        string `mapstructure:"secret_key"`        // AWS Secret Access Key
	Region           string `mapstructure:"region_name"`       // AWS Region
	SessionToken     string `mapstructure:"session_token"`     // AWS Session Token (optional)
	EndpointURL      string `mapstructure:"endpoint_url"`      // Custom endpoint (optional)
	SignatureVersion string `mapstructure:"signature_version"` // Signature version
	AddressingStyle  string `mapstructure:"addressing_style"`  // Addressing style
	Bucket           string `mapstructure:"bucket"`            // Default bucket (optional)
	PrefixPath       string `mapstructure:"prefix_path"`       // Path prefix (optional)
}

func (c *Config) parseS3Config(v *viper.Viper) {
	// Default S3 config
	c.storageEngine.S3.AccessKey = ""
	c.storageEngine.S3.SecretKey = ""
	c.storageEngine.S3.Region = ""
	c.storageEngine.S3.SessionToken = ""
	c.storageEngine.S3.EndpointURL = ""
	c.storageEngine.S3.SignatureVersion = "v4"
	c.storageEngine.S3.AddressingStyle = "path"
	c.storageEngine.S3.Bucket = ""
	c.storageEngine.S3.PrefixPath = ""

	if !v.IsSet("s3") {
		return
	}
	sub := v.Sub("s3")
	if sub == nil {
		return
	}

	if sub.IsSet("access_key") {
		c.storageEngine.S3.AccessKey = sub.GetString("access_key")
	}

	if sub.IsSet("secret_key") {
		c.storageEngine.S3.SecretKey = sub.GetString("secret_key")
	}

	if sub.IsSet("region") {
		c.storageEngine.S3.Region = sub.GetString("region")
	}

	if sub.IsSet("session_token") {
		c.storageEngine.S3.SessionToken = sub.GetString("session_token")
	}

	if sub.IsSet("endpoint_url") {
		c.storageEngine.S3.EndpointURL = sub.GetString("endpoint_url")
	}

	if sub.IsSet("signature_version") {
		c.storageEngine.S3.SignatureVersion = sub.GetString("signature_version")
	}

	if sub.IsSet("addressing_style") {
		c.storageEngine.S3.AddressingStyle = sub.GetString("addressing_style")
	}

	if sub.IsSet("bucket") {
		c.storageEngine.S3.Bucket = sub.GetString("bucket")
	}

	if sub.IsSet("prefix_path") {
		c.storageEngine.S3.PrefixPath = sub.GetString("prefix_path")
	}
}

// OSSConfig holds Aliyun OSS storage configuration
// OSS is compatible with S3 API
type OSSConfig struct {
	AccessKey        string `mapstructure:"access_key"`        // OSS Access Key ID
	SecretKey        string `mapstructure:"secret_key"`        // OSS Secret Access Key
	EndpointURL      string `mapstructure:"endpoint_url"`      // OSS Endpoint (e.g., "https://oss-cn-hangzhou.aliyuncs.com")
	Region           string `mapstructure:"region"`            // Region (e.g., "cn-hangzhou")
	Bucket           string `mapstructure:"bucket"`            // Default bucket (optional)
	PrefixPath       string `mapstructure:"prefix_path"`       // Path prefix (optional)
	SignatureVersion string `mapstructure:"signature_version"` // Signature version
	AddressingStyle  string `mapstructure:"addressing_style"`  // Addressing style
}

func (c *Config) parseOSSConfig(v *viper.Viper) {
	// Default OSS config
	c.storageEngine.OSS.AccessKey = ""
	c.storageEngine.OSS.SecretKey = ""
	c.storageEngine.OSS.EndpointURL = ""
	c.storageEngine.OSS.Region = ""
	c.storageEngine.OSS.Bucket = ""
	c.storageEngine.OSS.PrefixPath = ""
	c.storageEngine.OSS.SignatureVersion = "v4"
	c.storageEngine.OSS.AddressingStyle = "path"

	if !v.IsSet("oss") {
		return
	}
	sub := v.Sub("oss")
	if sub == nil {
		return
	}

	if sub.IsSet("access_key") {
		c.storageEngine.OSS.AccessKey = sub.GetString("access_key")
	}

	if sub.IsSet("secret_key") {
		c.storageEngine.OSS.SecretKey = sub.GetString("secret_key")
	}

	if sub.IsSet("endpoint_url") {
		c.storageEngine.OSS.EndpointURL = sub.GetString("endpoint_url")
	}

	if sub.IsSet("region") {
		c.storageEngine.OSS.Region = sub.GetString("region")
	}

	if sub.IsSet("bucket") {
		c.storageEngine.OSS.Bucket = sub.GetString("bucket")
	}

	if sub.IsSet("prefix_path") {
		c.storageEngine.OSS.PrefixPath = sub.GetString("prefix_path")
	}

	if sub.IsSet("signature_version") {
		c.storageEngine.OSS.SignatureVersion = sub.GetString("signature_version")
	}

	if sub.IsSet("addressing_style") {
		c.storageEngine.OSS.AddressingStyle = sub.GetString("addressing_style")
	}

}

type GCSConfig struct {
	Bucket      string `mapstructure:"bucket"`       // Default bucket (optional)
	PrefixPath  string `mapstructure:"prefix_path"`  // Path prefix (optional)
	EndpointURL string `mapstructure:"endpoint_url"` // Custom endpoint (optional)
}

func (c *Config) parseGCSConfig(v *viper.Viper) {
	// Default GCS config
	c.storageEngine.GCS.Bucket = ""
	c.storageEngine.GCS.PrefixPath = ""
	c.storageEngine.GCS.EndpointURL = ""

	if !v.IsSet("gcs") {
		return
	}
	sub := v.Sub("gcs")
	if sub == nil {
		return
	}

	if sub.IsSet("bucket") {
		c.storageEngine.GCS.Bucket = sub.GetString("bucket")
	}

	if sub.IsSet("prefix_path") {
		c.storageEngine.GCS.PrefixPath = sub.GetString("prefix_path")
	}

	if sub.IsSet("endpoint_url") {
		c.storageEngine.GCS.EndpointURL = sub.GetString("endpoint_url")
	}
}

func (c *Config) GetMinioConfig() MinioConfig {
	return c.storageEngine.Minio
}

func (m MinioConfig) ExportConfigs() map[string]interface{} {
	var minioConfigs map[string]interface{}
	minioConfigs = make(map[string]interface{})
	minioConfigs["host"] = m.Host
	minioConfigs["user"] = m.User
	minioConfigs["password"] = m.Password
	minioConfigs["bucket"] = m.Bucket
	minioConfigs["prefix_path"] = m.PrefixPath
	minioConfigs["secure"] = m.Secure
	minioConfigs["verify"] = m.Verify
	minioConfigs["region"] = m.Region
	return minioConfigs
}

func (c *Config) GetS3Config() S3Config {
	return c.storageEngine.S3
}

func (s S3Config) ExportConfigs() map[string]interface{} {
	var s3Configs map[string]interface{}
	s3Configs = make(map[string]interface{})
	s3Configs["access_key"] = s.AccessKey
	s3Configs["secret_key"] = s.SecretKey
	s3Configs["region"] = s.Region
	s3Configs["session_token"] = s.SessionToken
	s3Configs["endpoint_url"] = s.EndpointURL
	s3Configs["signature_version"] = s.SignatureVersion
	s3Configs["addressing_style"] = s.AddressingStyle
	s3Configs["bucket"] = s.Bucket
	s3Configs["prefix_path"] = s.PrefixPath
	return s3Configs
}

func (c *Config) GetOSSConfig() OSSConfig {
	return c.storageEngine.OSS
}

func (o OSSConfig) ExportConfigs() map[string]interface{} {
	var ossConfigs map[string]interface{}
	ossConfigs = make(map[string]interface{})
	ossConfigs["access_key"] = o.AccessKey
	ossConfigs["secret_key"] = o.SecretKey
	ossConfigs["endpoint_url"] = o.EndpointURL
	ossConfigs["region"] = o.Region
	ossConfigs["bucket"] = o.Bucket
	ossConfigs["prefix_path"] = o.PrefixPath
	ossConfigs["signature_version"] = o.SignatureVersion
	ossConfigs["addressing_style"] = o.AddressingStyle
	return ossConfigs
}

func (c *Config) GetGCSConfig() GCSConfig {
	return c.storageEngine.GCS
}

func (g GCSConfig) ExportConfigs() map[string]interface{} {
	var gcsConfigs map[string]interface{}
	gcsConfigs = make(map[string]interface{})
	gcsConfigs["bucket"] = g.Bucket
	gcsConfigs["prefix_path"] = g.PrefixPath
	gcsConfigs["endpoint_url"] = g.EndpointURL
	return gcsConfigs
}
