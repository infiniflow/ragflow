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
	"database/sql"
	"fmt"
	"regexp"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// OpenDALConfig holds OpenDAL storage configuration
// Note: This is a simplified implementation for MySQL backend only.
// For full OpenDAL support, additional dependencies are required:
//   go get github.com/apache/opendal-go-services/mysql
//   go get github.com/apache/opendal-go/gopkg
type OpenDALConfig struct {
	Scheme           string                 `mapstructure:"scheme"`            // Storage scheme (e.g., "mysql", "fs", "s3")
	MySQLHost        string                 `mapstructure:"mysql_host"`        // MySQL host
	MySQLPort        int                    `mapstructure:"mysql_port"`        // MySQL port
	MySQLUser        string                 `mapstructure:"mysql_user"`        // MySQL user
	MySQLPassword    string                 `mapstructure:"mysql_password"`    // MySQL password
	MySQLDatabase    string                 `mapstructure:"mysql_database"`    // MySQL database
	Table            string                 `mapstructure:"table"`             // Storage table name (for mysql scheme)
	MaxAllowedPacket int                    `mapstructure:"max_allowed_packet"` // Max allowed packet size
	Config           map[string]interface{} `mapstructure:"config"`            // Additional config options
}

// OpenDALStorage implements Storage interface using Apache OpenDAL (MySQL backend)
// This is a simplified implementation that stores files as BLOBs in MySQL
type OpenDALStorage struct {
	db     *sql.DB
	scheme string
	config *OpenDALConfig
	table  string
}

// NewOpenDALStorage creates a new OpenDAL storage instance (MySQL backend)
func NewOpenDALStorage(config *OpenDALConfig) (*OpenDALStorage, error) {
	storage := &OpenDALStorage{
		scheme: config.Scheme,
		config: config,
		table:  config.Table,
	}

	if storage.table == "" {
		storage.table = "opendal_storage"
	}

	if storage.scheme == "" {
		storage.scheme = "mysql"
	}

	if err := storage.connect(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (o *OpenDALStorage) connect() error {
	if o.scheme != "mysql" {
		return fmt.Errorf("only MySQL backend is supported in this simplified implementation, got: %s", o.scheme)
	}

	// Validate table name to prevent SQL injection
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(o.table) {
		return fmt.Errorf("invalid table name: %s", o.table)
	}

	// Build DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		o.config.MySQLUser,
		o.config.MySQLPassword,
		o.config.MySQLHost,
		o.config.MySQLPort,
		o.config.MySQLDatabase,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping MySQL: %w", err)
	}

	o.db = db

	// Initialize table
	if err := o.initTable(); err != nil {
		db.Close()
		return err
	}

	zap.L().Info("OpenDAL MySQL storage initialized", zap.String("table", o.table))
	return nil
}

func (o *OpenDALStorage) initTable() error {
	// Set max_allowed_packet
	maxPacket := o.config.MaxAllowedPacket
	if maxPacket == 0 {
		maxPacket = 134217728 // Default 128MB
	}

	_, err := o.db.Exec(fmt.Sprintf("SET GLOBAL max_allowed_packet=%d", maxPacket))
	if err != nil {
		zap.L().Warn("Failed to set max_allowed_packet", zap.Error(err))
	}

	// Create table if not exists
	createTableSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		`+"`key`"+` VARCHAR(255) PRIMARY KEY,
		value LONGBLOB,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	)`, o.table)

	_, err = o.db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

func (o *OpenDALStorage) reconnect() {
	if err := o.connect(); err != nil {
		zap.L().Error("Failed to reconnect to OpenDAL", zap.Error(err))
	}
}

func (o *OpenDALStorage) getKey(bucket, fnm string) string {
	return fmt.Sprintf("%s/%s", bucket, fnm)
}

// Health checks OpenDAL storage availability
func (o *OpenDALStorage) Health() bool {
	if o.db == nil {
		return false
	}

	if err := o.db.Ping(); err != nil {
		zap.L().Error("Health check failed", zap.Error(err))
		return false
	}

	// Try to write a test record
	key := o.getKey("health_check_bucket", "health_check")
	data := []byte("_t@@@1")

	query := fmt.Sprintf("INSERT INTO %s (`key`, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)", o.table)
	_, err := o.db.Exec(query, key, data)
	if err != nil {
		zap.L().Error("Health check write failed", zap.Error(err))
		return false
	}

	return true
}

// Put uploads an object to OpenDAL storage
func (o *OpenDALStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	key := o.getKey(bucket, fnm)

	query := fmt.Sprintf("INSERT INTO %s (`key`, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)", o.table)
	_, err := o.db.Exec(query, key, binary)
	if err != nil {
		zap.L().Error("Failed to put object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// Get retrieves an object from OpenDAL storage
func (o *OpenDALStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	key := o.getKey(bucket, fnm)

	query := fmt.Sprintf("SELECT value FROM %s WHERE `key` = ?", o.table)
	var data []byte
	err := o.db.QueryRow(query, key).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		zap.L().Error("Failed to get object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return nil, err
	}

	return data, nil
}

// Rm removes an object from OpenDAL storage
func (o *OpenDALStorage) Rm(bucket, fnm string, tenantID ...string) error {
	key := o.getKey(bucket, fnm)

	query := fmt.Sprintf("DELETE FROM %s WHERE `key` = ?", o.table)
	_, err := o.db.Exec(query, key)
	if err != nil {
		zap.L().Error("Failed to remove object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in OpenDAL storage
func (o *OpenDALStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	key := o.getKey(bucket, fnm)

	query := fmt.Sprintf("SELECT 1 FROM %s WHERE `key` = ? LIMIT 1", o.table)
	var exists int
	err := o.db.QueryRow(query, key).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false
		}
		return false
	}

	return true
}

// GetPresignedURL generates a presigned URL for accessing an object
// Note: MySQL backend doesn't support presigned URLs
func (o *OpenDALStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	return "", fmt.Errorf("presigned URLs not supported for OpenDAL MySQL backend")
}

// BucketExists checks if a bucket exists
// For MySQL backend, we check if any keys with the bucket prefix exist
func (o *OpenDALStorage) BucketExists(bucket string) bool {
	prefix := bucket + "/"

	query := fmt.Sprintf("SELECT 1 FROM %s WHERE `key` LIKE ? LIMIT 1", o.table)
	var exists int
	err := o.db.QueryRow(query, prefix+"%").Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false
		}
		return false
	}

	return true
}

// RemoveBucket removes a bucket and all its objects
func (o *OpenDALStorage) RemoveBucket(bucket string) error {
	prefix := bucket + "/"

	query := fmt.Sprintf("DELETE FROM %s WHERE `key` LIKE ?", o.table)
	_, err := o.db.Exec(query, prefix+"%")
	if err != nil {
		zap.L().Error("Failed to remove bucket", zap.String("bucket", bucket), zap.Error(err))
		return err
	}

	return nil
}

// Copy copies an object from source to destination
func (o *OpenDALStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	srcKey := o.getKey(srcBucket, srcPath)
	destKey := o.getKey(destBucket, destPath)

	// Read source
	data, err := o.Get(srcBucket, srcPath)
	if err != nil {
		zap.L().Error("Failed to read source object", zap.String("src", srcKey), zap.Error(err))
		return false
	}
	if data == nil {
		zap.L().Error("Source object not found", zap.String("src", srcKey))
		return false
	}

	// Write to destination
	if err := o.Put(destBucket, destPath, data); err != nil {
		zap.L().Error("Failed to write destination object", zap.String("dest", destKey), zap.Error(err))
		return false
	}

	return true
}

// Move moves an object from source to destination
func (o *OpenDALStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if o.Copy(srcBucket, srcPath, destBucket, destPath) {
		if err := o.Rm(srcBucket, srcPath); err != nil {
			zap.L().Error("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			return false
		}
		return true
	}
	return false
}

// Scan lists objects in a path (OpenDAL-specific method)
func (o *OpenDALStorage) Scan(bucket, fnm string, tenantID ...string) ([]string, error) {
	prefix := o.getKey(bucket, fnm)
	if !regexp.MustCompile(`^[a-zA-Z0-9_/]+$`).MatchString(prefix) {
		return nil, fmt.Errorf("invalid path: %s", prefix)
	}

	query := fmt.Sprintf("SELECT `key` FROM %s WHERE `key` LIKE ?", o.table)
	rows, err := o.db.Query(query, prefix+"%")
	if err != nil {
		zap.L().Error("Failed to scan objects", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue
		}
		result = append(result, key)
	}

	return result, nil
}
