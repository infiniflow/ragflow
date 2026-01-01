#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import logging
from common.crypto_utils import CryptoUtil


# from common.decorator import singleton

class EncryptedStorageWrapper:
    """Encrypted storage wrapper that wraps existing storage implementations to provide transparent encryption"""

    def __init__(self, storage_impl, algorithm="aes-256-cbc", key=None, iv=None):
        """
        Initialize encrypted storage wrapper
        
        Args:
            storage_impl: Original storage implementation instance
            algorithm: Encryption algorithm, default is aes-256-cbc
            key: Encryption key, uses RAGFLOW_CRYPTO_KEY environment variable if None
            iv: Initialization vector, automatically generated if None
        """
        self.storage_impl = storage_impl
        self.crypto = CryptoUtil(algorithm=algorithm, key=key, iv=iv)
        self.encryption_enabled = True

        # Check if storage implementation has required methods
        # todo: Consider abstracting a storage base class to ensure these methods exist
        required_methods = ["put", "get", "rm", "obj_exist", "health"]
        for method in required_methods:
            if not hasattr(storage_impl, method):
                raise AttributeError(f"Storage implementation missing required method: {method}")

        logging.info(f"EncryptedStorageWrapper initialized with algorithm: {algorithm}")

    def put(self, bucket, fnm, binary, tenant_id=None):
        """
        Encrypt and store data
        
        Args:
            bucket: Bucket name
            fnm: File name
            binary: Original binary data
            tenant_id: Tenant ID (optional)
            
        Returns:
            Storage result
        """
        if not self.encryption_enabled:
            return self.storage_impl.put(bucket, fnm, binary, tenant_id)

        try:
            encrypted_binary = self.crypto.encrypt(binary)

            return self.storage_impl.put(bucket, fnm, encrypted_binary, tenant_id)
        except Exception as e:
            logging.exception(f"Failed to encrypt and store data: {bucket}/{fnm}, error: {str(e)}")
            raise

    def get(self, bucket, fnm, tenant_id=None):
        """
        Retrieve and decrypt data
        
        Args:
            bucket: Bucket name
            fnm: File name
            tenant_id: Tenant ID (optional)
            
        Returns:
            Decrypted binary data
        """
        try:
            # Get encrypted data
            encrypted_binary = self.storage_impl.get(bucket, fnm, tenant_id)

            if encrypted_binary is None:
                return None

            if not self.encryption_enabled:
                return encrypted_binary

            # Decrypt data
            decrypted_binary = self.crypto.decrypt(encrypted_binary)
            return decrypted_binary

        except Exception as e:
            logging.exception(f"Failed to get and decrypt data: {bucket}/{fnm}, error: {str(e)}")
            raise

    def rm(self, bucket, fnm, tenant_id=None):
        """
        Delete data (same as original storage implementation, no decryption needed)
        
        Args:
            bucket: Bucket name
            fnm: File name
            tenant_id: Tenant ID (optional)
            
        Returns:
            Deletion result
        """
        return self.storage_impl.rm(bucket, fnm, tenant_id)

    def obj_exist(self, bucket, fnm, tenant_id=None):
        """
        Check if object exists (same as original storage implementation, no decryption needed)
        
        Args:
            bucket: Bucket name
            fnm: File name
            tenant_id: Tenant ID (optional)
            
        Returns:
            Whether the object exists
        """
        return self.storage_impl.obj_exist(bucket, fnm, tenant_id)

    def health(self):
        """
        Health check (uses the original storage implementation's method)
        
        Returns:
            Health check result
        """
        return self.storage_impl.health()

    def bucket_exists(self, bucket):
        """
        Check if bucket exists (if the original storage implementation has this method)
        
        Args:
            bucket: Bucket name
            
        Returns:
            Whether the bucket exists
        """
        if hasattr(self.storage_impl, "bucket_exists"):
            return self.storage_impl.bucket_exists(bucket)
        return False

    def get_presigned_url(self, bucket, fnm, expires, tenant_id=None):
        """
        Get presigned URL (if the original storage implementation has this method)
        
        Args:
            bucket: Bucket name
            fnm: File name
            expires: Expiration time
            tenant_id: Tenant ID (optional)
            
        Returns:
            Presigned URL
        """
        if hasattr(self.storage_impl, "get_presigned_url"):
            return self.storage_impl.get_presigned_url(bucket, fnm, expires, tenant_id)
        return None

    def scan(self, bucket, fnm, tenant_id=None):
        """
        Scan objects (if the original storage implementation has this method)
        
        Args:
            bucket: Bucket name
            fnm: File name prefix
            tenant_id: Tenant ID (optional)
            
        Returns:
            Scan results
        """
        if hasattr(self.storage_impl, "scan"):
            return self.storage_impl.scan(bucket, fnm, tenant_id)
        return None

    def copy(self, src_bucket, src_path, dest_bucket, dest_path):
        """
        Copy object (if the original storage implementation has this method)
        
        Args:
            src_bucket: Source bucket name
            src_path: Source file path
            dest_bucket: Destination bucket name
            dest_path: Destination file path
            
        Returns:
            Copy result
        """
        if hasattr(self.storage_impl, "copy"):
            return self.storage_impl.copy(src_bucket, src_path, dest_bucket, dest_path)
        return False

    def move(self, src_bucket, src_path, dest_bucket, dest_path):
        """
        Move object (if the original storage implementation has this method)
        
        Args:
            src_bucket: Source bucket name
            src_path: Source file path
            dest_bucket: Destination bucket name
            dest_path: Destination file path
            
        Returns:
            Move result
        """
        if hasattr(self.storage_impl, "move"):
            return self.storage_impl.move(src_bucket, src_path, dest_bucket, dest_path)
        return False

    def remove_bucket(self, bucket):
        """
        Remove bucket (if the original storage implementation has this method)
        
        Args:
            bucket: Bucket name
            
        Returns:
            Remove result
        """
        if hasattr(self.storage_impl, "remove_bucket"):
            return self.storage_impl.remove_bucket(bucket)
        return False

    def enable_encryption(self):
        """Enable encryption"""
        self.encryption_enabled = True
        logging.info("Encryption enabled")

    def disable_encryption(self):
        """Disable encryption"""
        self.encryption_enabled = False
        logging.info("Encryption disabled")


# Create singleton wrapper function
def create_encrypted_storage(storage_impl, algorithm=None, key=None, encryption_enabled=True):
    """
    Create singleton instance of encrypted storage wrapper
    
    Args:
        storage_impl: Original storage implementation instance
        algorithm: Encryption algorithm, uses environment variable RAGFLOW_CRYPTO_ALGORITHM or default if None
        key: Encryption key, uses environment variable RAGFLOW_CRYPTO_KEY if None
        encryption_enabled: Whether to enable encryption functionality
        
    Returns:
        Encrypted storage wrapper instance
    """
    wrapper = EncryptedStorageWrapper(storage_impl, algorithm=algorithm, key=key)

    wrapper.encryption_enabled = encryption_enabled

    if encryption_enabled:
        logging.info("Encryption enabled in storage wrapper")
    else:
        logging.info("Encryption disabled in storage wrapper")

    return wrapper
