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
	StorageType string      `mapstructure:"type"`
	Minio       MinioConfig `mapstructure:"minio"`
	S3          S3Config    `mapstructure:"s3"`
	OSS         OSSConfig   `mapstructure:"oss"`
	GCS         GCSConfig   `mapstructure:"gcs"`
}

func ParseStorageConfig(storageType string, config *Config, v *viper.Viper) error {
	switch storageType {
	case "minio":
		parseMinioConfig(config, v)
	case "s3":
		parseS3Config(config, v)
	case "oss":
		parseOSSConfig(config, v)
	case "gcs":
		parseGCSConfig(config, v)
	default:
		return fmt.Errorf("invalid storage type: %s", storageType)
	}
	config.StorageEngine.StorageType = storageType
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

func parseMinioConfig(config *Config, v *viper.Viper) {
	// Default MinIO config
	config.StorageEngine.Minio.Host = "localhost:23817"
	config.StorageEngine.Minio.User = "rag_flow"
	config.StorageEngine.Minio.Password = "infini_rag_flow"
	config.StorageEngine.Minio.Bucket = ""
	config.StorageEngine.Minio.PrefixPath = ""
	config.StorageEngine.Minio.Secure = false
	config.StorageEngine.Minio.Verify = false
	config.StorageEngine.Minio.Region = ""

	if !v.IsSet("minio") {
		return
	}
	sub := v.Sub("minio")
	if sub == nil {
		return
	}

	if sub.IsSet("host") {
		config.StorageEngine.Minio.Host = sub.GetString("host")
	}

	if sub.IsSet("user") {
		config.StorageEngine.Minio.User = sub.GetString("user")
	}

	if sub.IsSet("password") {
		config.StorageEngine.Minio.Password = sub.GetString("password")
	}

	if sub.IsSet("bucket") {
		config.StorageEngine.Minio.Bucket = sub.GetString("bucket")
	}

	if sub.IsSet("prefix_path") {
		config.StorageEngine.Minio.PrefixPath = sub.GetString("prefix_path")
	}

	if sub.IsSet("secure") {
		config.StorageEngine.Minio.Secure = sub.GetBool("secure")
	}

	if sub.IsSet("verify") {
		config.StorageEngine.Minio.Verify = sub.GetBool("verify")
	}

	if sub.IsSet("region") {
		config.StorageEngine.Minio.Region = sub.GetString("region")
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

func parseS3Config(config *Config, v *viper.Viper) {
	// Default S3 config
	config.StorageEngine.S3.AccessKey = ""
	config.StorageEngine.S3.SecretKey = ""
	config.StorageEngine.S3.Region = ""
	config.StorageEngine.S3.SessionToken = ""
	config.StorageEngine.S3.EndpointURL = ""
	config.StorageEngine.S3.SignatureVersion = "v4"
	config.StorageEngine.S3.AddressingStyle = "path"
	config.StorageEngine.S3.Bucket = ""
	config.StorageEngine.S3.PrefixPath = ""

	if !v.IsSet("s3") {
		return
	}
	sub := v.Sub("s3")
	if sub == nil {
		return
	}

	if sub.IsSet("access_key") {
		config.StorageEngine.S3.AccessKey = sub.GetString("access_key")
	}

	if sub.IsSet("secret_key") {
		config.StorageEngine.S3.SecretKey = sub.GetString("secret_key")
	}

	if sub.IsSet("region") {
		config.StorageEngine.S3.Region = sub.GetString("region")
	}

	if sub.IsSet("session_token") {
		config.StorageEngine.S3.SessionToken = sub.GetString("session_token")
	}

	if sub.IsSet("endpoint_url") {
		config.StorageEngine.S3.EndpointURL = sub.GetString("endpoint_url")
	}

	if sub.IsSet("signature_version") {
		config.StorageEngine.S3.SignatureVersion = sub.GetString("signature_version")
	}

	if sub.IsSet("addressing_style") {
		config.StorageEngine.S3.AddressingStyle = sub.GetString("addressing_style")
	}

	if sub.IsSet("bucket") {
		config.StorageEngine.S3.Bucket = sub.GetString("bucket")
	}

	if sub.IsSet("prefix_path") {
		config.StorageEngine.S3.PrefixPath = sub.GetString("prefix_path")
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

func parseOSSConfig(config *Config, v *viper.Viper) {
	// Default OSS config
	config.StorageEngine.OSS.AccessKey = ""
	config.StorageEngine.OSS.SecretKey = ""
	config.StorageEngine.OSS.EndpointURL = ""
	config.StorageEngine.OSS.Region = ""
	config.StorageEngine.OSS.Bucket = ""
	config.StorageEngine.OSS.PrefixPath = ""
	config.StorageEngine.OSS.SignatureVersion = "v4"
	config.StorageEngine.OSS.AddressingStyle = "path"

	if !v.IsSet("oss") {
		return
	}
	sub := v.Sub("oss")
	if sub == nil {
		return
	}

	if sub.IsSet("access_key") {
		config.StorageEngine.OSS.AccessKey = sub.GetString("access_key")
	}

	if sub.IsSet("secret_key") {
		config.StorageEngine.OSS.SecretKey = sub.GetString("secret_key")
	}

	if sub.IsSet("endpoint_url") {
		config.StorageEngine.OSS.EndpointURL = sub.GetString("endpoint_url")
	}

	if sub.IsSet("region") {
		config.StorageEngine.OSS.Region = sub.GetString("region")
	}

	if sub.IsSet("bucket") {
		config.StorageEngine.OSS.Bucket = sub.GetString("bucket")
	}

	if sub.IsSet("prefix_path") {
		config.StorageEngine.OSS.PrefixPath = sub.GetString("prefix_path")
	}

	if sub.IsSet("signature_version") {
		config.StorageEngine.OSS.SignatureVersion = sub.GetString("signature_version")
	}

	if sub.IsSet("addressing_style") {
		config.StorageEngine.OSS.AddressingStyle = sub.GetString("addressing_style")
	}

}

type GCSConfig struct {
	Bucket      string `mapstructure:"bucket"`       // Default bucket (optional)
	PrefixPath  string `mapstructure:"prefix_path"`  // Path prefix (optional)
	EndpointURL string `mapstructure:"endpoint_url"` // Custom endpoint (optional)
}

func parseGCSConfig(config *Config, v *viper.Viper) {
	// Default GCS config
	config.StorageEngine.GCS.Bucket = ""
	config.StorageEngine.GCS.PrefixPath = ""
	config.StorageEngine.GCS.EndpointURL = ""

	if !v.IsSet("gcs") {
		return
	}
	sub := v.Sub("gcs")
	if sub == nil {
		return
	}

	if sub.IsSet("bucket") {
		config.StorageEngine.GCS.Bucket = sub.GetString("bucket")
	}

	if sub.IsSet("prefix_path") {
		config.StorageEngine.GCS.PrefixPath = sub.GetString("prefix_path")
	}

	if sub.IsSet("endpoint_url") {
		config.StorageEngine.GCS.EndpointURL = sub.GetString("endpoint_url")
	}
}
