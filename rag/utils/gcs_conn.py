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

"""
Google Cloud Storage (GCS) Backend for RAGFlow

This module provides GCS support for RAGFlow's object storage layer.
It uses Application Default Credentials (ADC) for authentication and
implements a single-bucket architecture with logical prefix-based separation.

Configuration:
    STORAGE_IMPL=GCS
    RAGFLOW__GCS__BUCKET=your-gcs-bucket-name
    GOOGLE_APPLICATION_CREDENTIALS=/path/to/credentials.json (optional if using Workload Identity)

IAM Requirements:
    - Storage Object User (roles/storage.objectUser)
    - Service Account Token Creator (roles/iam.serviceAccountTokenCreator) for presigned URLs
"""

import logging
import time
from io import BytesIO
from datetime import timedelta
from google.cloud import storage
from google.cloud.exceptions import NotFound, GoogleCloudError
from common.decorator import singleton
from common import settings


@singleton
class RAGFlowGCS:
    """
    Google Cloud Storage backend implementation for RAGFlow.
    
    This class implements the RAGFlow storage interface using GCS.
    It uses a single GCS bucket with prefix-based logical separation
    for different "buckets" (tenants, workspaces, etc.).
    
    Architecture:
        - Single physical GCS bucket (configured in settings)
        - Logical "buckets" mapped to prefixes: {logical_bucket}/{filename}
        - Example: put("rag-data", "file.txt") -> gs://bucket/rag-data/file.txt
    
    Authentication:
        Uses Application Default Credentials (ADC):
        - GOOGLE_APPLICATION_CREDENTIALS environment variable
        - Workload Identity (GKE/Cloud Run)
        - gcloud CLI credentials
    """
    
    def __init__(self):
        """Initialize GCS client and configuration."""
        self.client = None
        self.bucket_name = None
        self.bucket = None
        self.__open__()
    
    def __open__(self):
        """
        Initialize or reinitialize the GCS client connection.
        
        This method:
        1. Closes any existing connection
        2. Loads configuration from settings
        3. Creates a new GCS client using ADC
        4. Gets a reference to the configured bucket
        """
        try:
            if self.client:
                self.__close__()
        except Exception:
            pass
        
        try:
            # Load GCS configuration from settings
            gcs_config = settings.GCS
            self.bucket_name = gcs_config.get('bucket')
            
            if not self.bucket_name:
                raise ValueError("GCS bucket name not configured in settings")
            
            # Initialize GCS client with Application Default Credentials
            # This automatically uses:
            # - GOOGLE_APPLICATION_CREDENTIALS env var
            # - Workload Identity on GKE/Cloud Run
            # - gcloud CLI credentials
            self.client = storage.Client()
            
            # Get bucket reference
            self.bucket = self.client.bucket(self.bucket_name)
            
            logging.info(f"GCS client initialized successfully for bucket: {self.bucket_name}")
            
        except Exception as e:
            logging.exception(f"Failed to connect to GCS bucket {self.bucket_name}: {e}")
            raise
    
    def __close__(self):
        """Close the GCS client connection."""
        if self.client:
            self.client.close()
        self.client = None
        self.bucket = None
    
    def _get_blob_name(self, logical_bucket, filename):
        """
        Convert logical bucket + filename to GCS blob name (prefix).
        
        Args:
            logical_bucket (str): Logical bucket name (used as prefix)
            filename (str): File name within the logical bucket
            
        Returns:
            str: Full blob path in format: {logical_bucket}/{filename}
        """
        return f"{logical_bucket}/{filename}"
    
    def health(self):
        """
        Health check: Verify GCS connection by writing a test object.
        
        Returns:
            bool: True if health check passes
            
        Raises:
            Exception: If health check fails
        """
        logical_bucket = "txtxtxtxt1"
        fnm = "txtxtxtxt1"
        binary = b"_t@@@1"
        
        try:
            blob_name = self._get_blob_name(logical_bucket, fnm)
            blob = self.bucket.blob(blob_name)
            blob.upload_from_string(binary)
            logging.info("GCS health check passed")
            return True
        except Exception as e:
            logging.exception(f"GCS health check failed: {e}")
            raise
    
    def put(self, logical_bucket, fnm, binary, tenant_id=None):
        """
        Upload an object to GCS.
        
        Args:
            logical_bucket (str): Logical bucket name (used as prefix)
            fnm (str): Filename
            binary (bytes): File content as bytes
            tenant_id (str, optional): Tenant ID for logging
            
        Returns:
            True if successful
            
        Raises:
            Exception: If upload fails after retries
        """
        blob_name = self._get_blob_name(logical_bucket, fnm)
        
        for attempt in range(3):
            try:
                blob = self.bucket.blob(blob_name)
                
                if isinstance(binary, bytes):
                    blob.upload_from_string(binary)
                elif isinstance(binary, BytesIO):
                    binary.seek(0)
                    blob.upload_from_file(binary)
                else:
                    # Try to handle as file-like object
                    blob.upload_from_file(BytesIO(binary))
                
                logging.debug(f"Successfully uploaded {blob_name}")
                return True
                
            except Exception as e:
                logging.warning(f"Failed to put {blob_name} (attempt {attempt + 1}/3): {e}")
                if attempt < 2:  # Don't reconnect on last attempt
                    self.__open__()
                    time.sleep(1)
                else:
                    logging.exception(f"Failed to put {logical_bucket}/{fnm} after all retries")
                    raise
        
        return False
    
    def rm(self, logical_bucket, fnm, tenant_id=None):
        """
        Delete an object from GCS.
        
        Args:
            logical_bucket (str): Logical bucket name (used as prefix)
            fnm (str): Filename
            tenant_id (str, optional): Tenant ID for logging
        """
        blob_name = self._get_blob_name(logical_bucket, fnm)
        
        try:
            blob = self.bucket.blob(blob_name)
            blob.delete()
            logging.debug(f"Successfully deleted {blob_name}")
        except NotFound:
            logging.warning(f"Object not found for deletion: {blob_name}")
        except Exception as e:
            logging.exception(f"Failed to remove {logical_bucket}/{fnm}: {e}")
    
    def get(self, logical_bucket, filename, tenant_id=None):
        """
        Download an object from GCS.
        
        Args:
            logical_bucket (str): Logical bucket name (used as prefix)
            filename (str): Filename
            tenant_id (str, optional): Tenant ID for logging
            
        Returns:
            bytes: File content, or None if not found
        """
        blob_name = self._get_blob_name(logical_bucket, filename)
        
        for attempt in range(1):
            try:
                blob = self.bucket.blob(blob_name)
                content = blob.download_as_bytes()
                logging.debug(f"Successfully downloaded {blob_name}")
                return content
                
            except NotFound:
                logging.warning(f"Object not found: {blob_name}")
                return None
            except Exception as e:
                logging.exception(f"Failed to get {logical_bucket}/{filename}: {e}")
                if attempt < 0:  # Retry logic (currently only 1 attempt)
                    self.__open__()
                    time.sleep(1)
        
        return None
    
    def obj_exist(self, logical_bucket, filename, tenant_id=None):
        """
        Check if an object exists in GCS.
        
        Args:
            logical_bucket (str): Logical bucket name (used as prefix)
            filename (str): Filename
            tenant_id (str, optional): Tenant ID for logging
            
        Returns:
            bool: True if object exists, False otherwise
        """
        blob_name = self._get_blob_name(logical_bucket, filename)
        
        try:
            blob = self.bucket.blob(blob_name)
            exists = blob.exists()
            logging.debug(f"Object exists check for {blob_name}: {exists}")
            return exists
        except Exception as e:
            logging.exception(f"Failed to check if object exists {logical_bucket}/{filename}: {e}")
            return False
    
    def bucket_exists(self, logical_bucket):
        """
        Check if the physical GCS bucket exists.
        
        Note: In this implementation, "logical buckets" are just prefixes,
        so this always checks the physical bucket.
        
        Args:
            logical_bucket (str): Logical bucket name (ignored, checks physical bucket)
            
        Returns:
            bool: True if physical bucket exists
        """
        try:
            exists = self.bucket.exists()
            logging.debug(f"Bucket exists check for {self.bucket_name}: {exists}")
            return exists
        except Exception as e:
            logging.exception(f"Failed to check if bucket exists {self.bucket_name}: {e}")
            return False
    
    def get_presigned_url(self, logical_bucket, fnm, expires, tenant_id=None):
        """
        Generate a presigned URL for temporary access to an object.
        
        Args:
            logical_bucket (str): Logical bucket name (used as prefix)
            fnm (str): Filename
            expires (int): Expiration time in seconds
            tenant_id (str, optional): Tenant ID for logging
            
        Returns:
            str: Presigned URL, or None if generation fails
            
        Note:
            Requires Service Account Token Creator role for generating signed URLs
            when running on GCP infrastructure.
        """
        blob_name = self._get_blob_name(logical_bucket, fnm)
        
        for attempt in range(10):
            try:
                blob = self.bucket.blob(blob_name)
                
                # Generate signed URL with specified expiration
                url = blob.generate_signed_url(
                    version="v4",
                    expiration=timedelta(seconds=expires),
                    method="GET"
                )
                
                logging.debug(f"Successfully generated presigned URL for {blob_name}")
                return url
                
            except Exception as e:
                logging.warning(f"Failed to generate presigned URL for {blob_name} (attempt {attempt + 1}/10): {e}")
                if attempt < 9:  # Don't reconnect on last attempt
                    self.__open__()
                    time.sleep(1)
                else:
                    logging.exception(f"Failed to generate presigned URL for {logical_bucket}/{fnm} after all retries")
        
        return None
    
    def remove_bucket(self, logical_bucket):
        """
        Remove all objects with a given prefix (logical bucket).
        
        Args:
            logical_bucket (str): Logical bucket name (prefix to delete)
            
        Warning:
            This deletes ALL objects under the prefix. Use with caution!
        """
        try:
            # List all blobs with the prefix
            prefix = f"{logical_bucket}/"
            blobs = list(self.bucket.list_blobs(prefix=prefix))
            
            logging.info(f"Removing {len(blobs)} objects with prefix {prefix}")
            
            # Delete all blobs
            for blob in blobs:
                try:
                    blob.delete()
                except Exception as e:
                    logging.error(f"Failed to delete {blob.name}: {e}")
            
            logging.info(f"Successfully removed logical bucket {logical_bucket}")
            
        except Exception as e:
            logging.exception(f"Failed to remove logical bucket {logical_bucket}: {e}")
    
    def copy(self, src_bucket, src_path, dest_bucket, dest_path):
        """
        Copy an object from one location to another.
        
        Args:
            src_bucket (str): Source logical bucket
            src_path (str): Source filename
            dest_bucket (str): Destination logical bucket
            dest_path (str): Destination filename
            
        Returns:
            bool: True if copy succeeded, False otherwise
        """
        try:
            src_blob_name = self._get_blob_name(src_bucket, src_path)
            dest_blob_name = self._get_blob_name(dest_bucket, dest_path)
            
            # Check if source exists
            src_blob = self.bucket.blob(src_blob_name)
            if not src_blob.exists():
                logging.error(f"Source object not found: {src_blob_name}")
                return False
            
            # Copy the blob
            dest_blob = self.bucket.blob(dest_blob_name)
            self.bucket.copy_blob(src_blob, self.bucket, dest_blob_name)
            
            logging.info(f"Successfully copied {src_blob_name} to {dest_blob_name}")
            return True
            
        except Exception as e:
            logging.exception(f"Failed to copy {src_bucket}/{src_path} -> {dest_bucket}/{dest_path}: {e}")
            return False
    
    def move(self, src_bucket, src_path, dest_bucket, dest_path):
        """
        Move an object from one location to another (copy + delete).
        
        Args:
            src_bucket (str): Source logical bucket
            src_path (str): Source filename
            dest_bucket (str): Destination logical bucket
            dest_path (str): Destination filename
            
        Returns:
            bool: True if move succeeded, False otherwise
        """
        try:
            # Copy the object
            if self.copy(src_bucket, src_path, dest_bucket, dest_path):
                # Delete the source
                self.rm(src_bucket, src_path)
                logging.info(f"Successfully moved {src_bucket}/{src_path} to {dest_bucket}/{dest_path}")
                return True
            else:
                logging.error(f"Copy failed, move aborted: {src_bucket}/{src_path}")
                return False
                
        except Exception as e:
            logging.exception(f"Failed to move {src_bucket}/{src_path} -> {dest_bucket}/{dest_path}: {e}")
            return False

