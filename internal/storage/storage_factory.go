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

package storage

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// StorageFactory creates storage instances based on configuration
type StorageFactory struct {
	storageType StorageType
	storage     Storage
	config      *StorageConfig
	mu          sync.RWMutex
}

// StorageConfig holds all storage-related configurations
type StorageConfig struct {
	StorageType string           `mapstructure:"storage_type"`
	Minio       *MinioConfig     `mapstructure:"minio"`
	S3          *S3Config        `mapstructure:"s3"`
	OSS         *OSSConfig       `mapstructure:"oss"`
	Azure       *AzureConfig     `mapstructure:"azure"`
	GCS         *GCSConfig       `mapstructure:"gcs"`
	OpenDAL     *OpenDALConfig   `mapstructure:"opendal"`
}

// AzureConfig holds Azure-specific configurations
type AzureConfig struct {
	ContainerURL  string `mapstructure:"container_url"`
	SASToken      string `mapstructure:"sas_token"`
	AccountURL    string `mapstructure:"account_url"`
	ClientID      string `mapstructure:"client_id"`
	Secret        string `mapstructure:"secret"`
	TenantID      string `mapstructure:"tenant_id"`
	ContainerName string `mapstructure:"container_name"`
	AuthorityHost string `mapstructure:"authority_host"`
}

var (
	globalFactory *StorageFactory
	once          sync.Once
)

// GetStorageFactory returns the singleton storage factory instance
func GetStorageFactory() *StorageFactory {
	once.Do(func() {
		globalFactory = &StorageFactory{}
	})
	return globalFactory
}

// InitStorageFactory initializes the storage factory with configuration
func InitStorageFactory(v *viper.Viper) error {
	factory := GetStorageFactory()

	// Get storage type from environment or config
	storageType := os.Getenv("STORAGE_IMPL")
	if storageType == "" {
		storageType = v.GetString("storage_type")
	}
	if storageType == "" {
		storageType = "MINIO" // Default storage type
	}

	storageConfig := &StorageConfig{}
	if err := v.UnmarshalKey("storage", storageConfig); err != nil {
		return fmt.Errorf("failed to unmarshal storage config: %w", err)
	}
	storageConfig.StorageType = storageType

	factory.config = storageConfig

	// Initialize storage based on type
	if err := factory.initStorage(storageType, v); err != nil {
		return err
	}

	zap.L().Info("Storage factory initialized",
		zap.String("storage_type", storageType),
	)

	return nil
}

// initStorage initializes the specific storage implementation
func (f *StorageFactory) initStorage(storageType string, v *viper.Viper) error {
	switch storageType {
	case "MINIO":
		return f.initMinio(v)
	case "AZURE_SPN":
		return f.initAzureSPN(v)
	case "AZURE_SAS":
		return f.initAzureSAS(v)
	case "AWS_S3":
		return f.initS3(v)
	case "OSS":
		return f.initOSS(v)
	case "GCS":
		return f.initGCS(v)
	case "OPENDAL":
		return f.initOpenDAL(v)
	default:
		return fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

func (f *StorageFactory) initMinio(v *viper.Viper) error {
	config := &MinioConfig{}

	// Try to load from minio section first
	if v.IsSet("minio") {
		minioConfig := v.Sub("minio")
		if minioConfig != nil {
			config.Host = minioConfig.GetString("host")
			config.User = minioConfig.GetString("user")
			config.Password = minioConfig.GetString("password")
			config.Secure = minioConfig.GetBool("secure")
			config.Verify = minioConfig.GetBool("verify")
			config.Bucket = minioConfig.GetString("bucket")
			config.PrefixPath = minioConfig.GetString("prefix_path")
		}
	}

	// Apply defaults
	if config.Host == "" {
		config.Host = "localhost:9000"
	}
	if config.User == "" {
		config.User = "minioadmin"
	}
	if config.Password == "" {
		config.Password = "minioadmin"
	}

	storage, err := NewMinioStorage(config)
	if err != nil {
		return fmt.Errorf("failed to create MinIO storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageMinio
	f.storage = storage
	f.config.Minio = config

	return nil
}

func (f *StorageFactory) initAzureSPN(v *viper.Viper) error {
	config := &AzureSPNConfig{}

	if v.IsSet("azure") {
		azureConfig := v.Sub("azure")
		if azureConfig != nil {
			config.AccountURL = azureConfig.GetString("account_url")
			config.ClientID = azureConfig.GetString("client_id")
			config.ClientSecret = azureConfig.GetString("secret")
			config.TenantID = azureConfig.GetString("tenant_id")
			config.ContainerName = azureConfig.GetString("container_name")
			config.AuthorityHost = azureConfig.GetString("authority_host")
		}
	}

	storage, err := NewAzureSPNStorage(config)
	if err != nil {
		return fmt.Errorf("failed to create Azure SPN storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageAzureSpn
	f.storage = storage

	return nil
}

func (f *StorageFactory) initAzureSAS(v *viper.Viper) error {
	config := &AzureSASConfig{}

	if v.IsSet("azure") {
		azureConfig := v.Sub("azure")
		if azureConfig != nil {
			config.ContainerURL = azureConfig.GetString("container_url")
			config.SASToken = azureConfig.GetString("sas_token")
		}
	}

	storage, err := NewAzureSASStorage(config)
	if err != nil {
		return fmt.Errorf("failed to create Azure SAS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageAzureSas
	f.storage = storage

	return nil
}

func (f *StorageFactory) initS3(v *viper.Viper) error {
	config := &S3Config{}

	if v.IsSet("s3") {
		s3Config := v.Sub("s3")
		if s3Config != nil {
			config.AccessKeyID = s3Config.GetString("access_key")
			config.SecretAccessKey = s3Config.GetString("secret_key")
			config.SessionToken = s3Config.GetString("session_token")
			config.Region = s3Config.GetString("region_name")
			config.EndpointURL = s3Config.GetString("endpoint_url")
			config.SignatureVersion = s3Config.GetString("signature_version")
			config.AddressingStyle = s3Config.GetString("addressing_style")
			config.Bucket = s3Config.GetString("bucket")
			config.PrefixPath = s3Config.GetString("prefix_path")
		}
	}

	storage, err := NewS3Storage(config)
	if err != nil {
		return fmt.Errorf("failed to create S3 storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageAWSS3
	f.storage = storage
	f.config.S3 = config

	return nil
}

func (f *StorageFactory) initOSS(v *viper.Viper) error {
	config := &OSSConfig{}

	if v.IsSet("oss") {
		ossConfig := v.Sub("oss")
		if ossConfig != nil {
			config.AccessKeyID = ossConfig.GetString("access_key")
			config.SecretAccessKey = ossConfig.GetString("secret_key")
			config.EndpointURL = ossConfig.GetString("endpoint_url")
			config.Region = ossConfig.GetString("region")
			config.Bucket = ossConfig.GetString("bucket")
			config.PrefixPath = ossConfig.GetString("prefix_path")
			config.SignatureVersion = ossConfig.GetString("signature_version")
			config.AddressingStyle = ossConfig.GetString("addressing_style")
		}
	}

	storage, err := NewOSSStorage(config)
	if err != nil {
		return fmt.Errorf("failed to create OSS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageOSS
	f.storage = storage
	f.config.OSS = config

	return nil
}

func (f *StorageFactory) initGCS(v *viper.Viper) error {
	config := &GCSConfig{}

	if v.IsSet("gcs") {
		gcsConfig := v.Sub("gcs")
		if gcsConfig != nil {
			config.Bucket = gcsConfig.GetString("bucket")
			config.CredentialsFile = gcsConfig.GetString("credentials_file")
			config.ProjectID = gcsConfig.GetString("project_id")
		}
	}

	storage, err := NewGCSStorage(config)
	if err != nil {
		return fmt.Errorf("failed to create GCS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageGCS
	f.storage = storage
	f.config.GCS = config

	return nil
}

func (f *StorageFactory) initOpenDAL(v *viper.Viper) error {
	config := &OpenDALConfig{
		Scheme: "mysql",
		Config: make(map[string]interface{}),
	}

	if v.IsSet("opendal") {
		opendalConfig := v.Sub("opendal")
		if opendalConfig != nil {
			config.Scheme = opendalConfig.GetString("scheme")
			if config.Scheme == "" {
				config.Scheme = "mysql"
			}

			// Load custom config
			if config.Scheme != "mysql" {
				config.Config = opendalConfig.GetStringMap("config")
			}
		}
	}

	// For MySQL scheme, use MySQL config
	if config.Scheme == "mysql" && v.IsSet("mysql") {
		mysqlConfig := v.Sub("mysql")
		if mysqlConfig != nil {
			config.MySQLHost = mysqlConfig.GetString("host")
			config.MySQLPort = mysqlConfig.GetInt("port")
			config.MySQLUser = mysqlConfig.GetString("user")
			config.MySQLPassword = mysqlConfig.GetString("password")
			config.MySQLDatabase = mysqlConfig.GetString("name")
			config.MaxAllowedPacket = mysqlConfig.GetInt("max_allowed_packet")
		}

		if v.IsSet("opendal") {
			opendalConfig := v.Sub("opendal")
			if opendalConfig != nil {
				config.Table = opendalConfig.GetString("config.oss_table")
			}
		}
		if config.Table == "" {
			config.Table = "opendal_storage"
		}
		if config.MaxAllowedPacket == 0 {
			config.MaxAllowedPacket = 134217728 // Default 128MB
		}
	}

	storage, err := NewOpenDALStorage(config)
	if err != nil {
		return fmt.Errorf("failed to create OpenDAL storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageOpenDAL
	f.storage = storage
	f.config.OpenDAL = config

	return nil
}

// GetStorage returns the current storage instance
func (f *StorageFactory) GetStorage() Storage {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.storage
}

// GetStorageType returns the current storage type
func (f *StorageFactory) GetStorageType() StorageType {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.storageType
}

// Create creates a new storage instance based on the storage type
// This is the factory method equivalent to Python's StorageFactory.create()
func (f *StorageFactory) Create(storageType StorageType) (Storage, error) {
	var storage Storage
	var err error

	switch storageType {
	case StorageMinio:
		if f.config.Minio != nil {
			storage, err = NewMinioStorage(f.config.Minio)
		} else {
			return nil, fmt.Errorf("MinIO config not available")
		}
	case StorageAzureSpn:
		if f.config.Azure != nil {
			config := &AzureSPNConfig{
				AccountURL:    f.config.Azure.AccountURL,
				ClientID:      f.config.Azure.ClientID,
				ClientSecret:  f.config.Azure.Secret,
				TenantID:      f.config.Azure.TenantID,
				ContainerName: f.config.Azure.ContainerName,
				AuthorityHost: f.config.Azure.AuthorityHost,
			}
			storage, err = NewAzureSPNStorage(config)
		} else {
			return nil, fmt.Errorf("Azure config not available")
		}
	case StorageAzureSas:
		if f.config.Azure != nil {
			config := &AzureSASConfig{
				ContainerURL: f.config.Azure.ContainerURL,
				SASToken:     f.config.Azure.SASToken,
			}
			storage, err = NewAzureSASStorage(config)
		} else {
			return nil, fmt.Errorf("Azure config not available")
		}
	case StorageAWSS3:
		if f.config.S3 != nil {
			storage, err = NewS3Storage(f.config.S3)
		} else {
			return nil, fmt.Errorf("S3 config not available")
		}
	case StorageOSS:
		if f.config.OSS != nil {
			storage, err = NewOSSStorage(f.config.OSS)
		} else {
			return nil, fmt.Errorf("OSS config not available")
		}
	case StorageGCS:
		if f.config.GCS != nil {
			storage, err = NewGCSStorage(f.config.GCS)
		} else {
			return nil, fmt.Errorf("GCS config not available")
		}
	case StorageOpenDAL:
		if f.config.OpenDAL != nil {
			storage, err = NewOpenDALStorage(f.config.OpenDAL)
		} else {
			return nil, fmt.Errorf("OpenDAL config not available")
		}
	default:
		return nil, fmt.Errorf("unsupported storage type: %v", storageType)
	}

	if err != nil {
		return nil, err
	}

	return storage, nil
}

// SetStorage sets the storage instance (useful for testing)
func (f *StorageFactory) SetStorage(storage Storage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage
}

// StorageTypeMapping returns the storage type mapping (equivalent to Python's storage_mapping)
var StorageTypeMapping = map[StorageType]func(*StorageConfig) (Storage, error){
	StorageMinio: func(config *StorageConfig) (Storage, error) {
		if config.Minio == nil {
			return nil, fmt.Errorf("MinIO config not available")
		}
		return NewMinioStorage(config.Minio)
	},
	StorageAzureSpn: func(config *StorageConfig) (Storage, error) {
		if config.Azure == nil {
			return nil, fmt.Errorf("Azure config not available")
		}
		azureConfig := &AzureSPNConfig{
			AccountURL:    config.Azure.AccountURL,
			ClientID:      config.Azure.ClientID,
			ClientSecret:  config.Azure.Secret,
			TenantID:      config.Azure.TenantID,
			ContainerName: config.Azure.ContainerName,
			AuthorityHost: config.Azure.AuthorityHost,
		}
		return NewAzureSPNStorage(azureConfig)
	},
	StorageAzureSas: func(config *StorageConfig) (Storage, error) {
		if config.Azure == nil {
			return nil, fmt.Errorf("Azure config not available")
		}
		azureConfig := &AzureSASConfig{
			ContainerURL: config.Azure.ContainerURL,
			SASToken:     config.Azure.SASToken,
		}
		return NewAzureSASStorage(azureConfig)
	},
	StorageAWSS3: func(config *StorageConfig) (Storage, error) {
		if config.S3 == nil {
			return nil, fmt.Errorf("S3 config not available")
		}
		return NewS3Storage(config.S3)
	},
	StorageOSS: func(config *StorageConfig) (Storage, error) {
		if config.OSS == nil {
			return nil, fmt.Errorf("OSS config not available")
		}
		return NewOSSStorage(config.OSS)
	},
	StorageOpenDAL: func(config *StorageConfig) (Storage, error) {
		if config.OpenDAL == nil {
			return nil, fmt.Errorf("OpenDAL config not available")
		}
		return NewOpenDALStorage(config.OpenDAL)
	},
	StorageGCS: func(config *StorageConfig) (Storage, error) {
		if config.GCS == nil {
			return nil, fmt.Errorf("GCS config not available")
		}
		return NewGCSStorage(config.GCS)
	},
}


