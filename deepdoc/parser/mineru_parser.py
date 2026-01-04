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

import json
import logging
import os
import shutil
import tempfile
import time
import zipfile
from dataclasses import dataclass
from enum import Enum
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Any, Callable, List, Optional

import requests

# Constants
API_TIMEOUT_SECONDS = 7200  # 2 hours
MAX_RETRIES_5XX = 3
RETRY_BACKOFF_BASE = 2
DEFAULT_BATCH_SIZE = 30  # Pages per batch


class BatchStatus(Enum):
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"


class BatchErrorType(Enum):
    TIMEOUT = "timeout"
    SERVER_ERROR_5XX = "server_error_5xx"
    CLIENT_ERROR_4XX = "client_error_4xx"
    NETWORK_ERROR = "network_error"
    UNKNOWN = "unknown"


@dataclass
class BatchInfo:
    batch_idx: int
    start_page: int
    end_page: int
    status: BatchStatus
    error_type: Optional[BatchErrorType] = None
    error_message: Optional[str] = None
    retry_count: int = 0
    content_count: int = 0


@dataclass
class BatchProcessingResult:
    total_batches: int
    successful_batches: int
    failed_batches: List[BatchInfo]
    total_content_blocks: int
    overall_status: str


class MinerUParser:
    def __init__(self, mineru_api: str, mineru_server_url: str = ""):
        self.mineru_api = mineru_api
        self.mineru_server_url = mineru_server_url
        self.batch_processing_result: Optional[BatchProcessingResult] = None
        self.logger = logging.getLogger(self.__class__.__name__)

    def check_installation(self, backend: Optional[str] = None, server_url: Optional[str] = None) -> tuple[bool, str]:
        """
        Check if MinerU service is available and accessible.
        
        Args:
            backend: MinerU backend type (e.g., "hybrid-auto-engine")
            server_url: Alternative server URL to check
            
        Returns:
            Tuple of (is_available, reason)
        """
        api_url = server_url or self.mineru_server_url or self.mineru_api
        
        if not api_url:
            return False, "MinerU API endpoint not configured"
        
        try:
            # Try to reach the health/status endpoint
            health_url = f"{api_url.rstrip('/')}/health"
            response = requests.get(health_url, timeout=10)
            
            if response.status_code == 200:
                self.logger.info(f"[MinerU] Service is available at {api_url}")
                return True, "MinerU service is available"
            else:
                msg = f"MinerU service returned status {response.status_code}"
                self.logger.warning(f"[MinerU] {msg}")
                return False, msg
                
        except requests.exceptions.ConnectionError:
            msg = f"Cannot connect to MinerU service at {api_url}"
            self.logger.error(f"[MinerU] {msg}")
            return False, msg
        except requests.exceptions.Timeout:
            msg = f"Timeout connecting to MinerU service at {api_url}"
            self.logger.error(f"[MinerU] {msg}")
            return False, msg
        except Exception as e:
            msg = f"Error checking MinerU service: {str(e)}"
            self.logger.error(f"[MinerU] {msg}")
            return False, msg

    def _classify_error(self, error: Exception) -> BatchErrorType:
        """Classify error type for retry logic"""
        if isinstance(error, requests.exceptions.Timeout):
            return BatchErrorType.TIMEOUT
        elif isinstance(error, requests.exceptions.HTTPError):
            status_code = error.response.status_code if error.response else 500
            if 500 <= status_code < 600:
                return BatchErrorType.SERVER_ERROR_5XX
            elif 400 <= status_code < 500:
                return BatchErrorType.CLIENT_ERROR_4XX
        elif isinstance(error, requests.exceptions.ConnectionError):
            return BatchErrorType.NETWORK_ERROR
        else:
            return BatchErrorType.UNKNOWN

    def _should_retry(self, error_type: BatchErrorType, retry_count: int) -> bool:
        """Determine if a batch should be retried based on error type"""
        if error_type == BatchErrorType.SERVER_ERROR_5XX:
            return retry_count < MAX_RETRIES_5XX
        return False

    def _calculate_backoff_delay(self, retry_count: int) -> int:
        """Calculate exponential backoff delay for retries"""
        return RETRY_BACKOFF_BASE ** retry_count

    def _upload_file_to_mineru(self, filepath: str, binary: bytes = None) -> str:
        """
        Upload file to MinerU service and get a file ID.
        
        Args:
            filepath: Path to the file
            binary: Binary content of the file
            
        Returns:
            File ID from MinerU service
        """
        api_url = self.mineru_server_url or self.mineru_api
        upload_url = f"{api_url.rstrip('/')}/upload"
        
        try:
            if binary:
                files = {'file': ('document.pdf', BytesIO(binary), 'application/pdf')}
            else:
                with open(filepath, 'rb') as f:
                    files = {'file': (os.path.basename(filepath), f, 'application/pdf')}
                    response = requests.post(upload_url, files=files, timeout=60)
                    response.raise_for_status()
                    result = response.json()
                    return result.get('file_id', result.get('id', ''))
            
            response = requests.post(upload_url, files=files, timeout=60)
            response.raise_for_status()
            result = response.json()
            return result.get('file_id', result.get('id', ''))
            
        except Exception as e:
            self.logger.error(f"[MinerU] Failed to upload file: {e}")
            raise

    def _parse_batch(
        self,
        file_id: str,
        start_page: int,
        end_page: int,
        backend: str,
        parse_method: str,
        batch_info: BatchInfo
    ) -> tuple[List[Any], List[Any]]:
        """
        Parse a batch of pages using MinerU API.
        
        Returns:
            Tuple of (sections, tables)
        """
        api_url = self.mineru_server_url or self.mineru_api
        parse_url = f"{api_url.rstrip('/')}/parse"
        
        payload = {
            'file_id': file_id,
            'start_page': start_page,
            'end_page': end_page,
            'backend': backend,
            'parse_method': parse_method
        }
        
        try:
            response = requests.post(
                parse_url,
                json=payload,
                timeout=API_TIMEOUT_SECONDS
            )
            response.raise_for_status()
            
            result = response.json()
            
            # Download and extract result
            if 'result_url' in result:
                return self._download_and_extract_result(result['result_url'])
            elif 'content' in result:
                return self._parse_content_direct(result['content'])
            else:
                self.logger.warning(f"[MinerU] Unexpected response format for batch {batch_info.batch_idx}")
                return [], []
                
        except Exception as e:
            error_type = self._classify_error(e)
            batch_info.error_type = error_type
            batch_info.error_message = str(e)
            self.logger.error(f"[MinerU] Batch {batch_info.batch_idx} failed: {e}")
            raise

    def _download_and_extract_result(self, result_url: str) -> tuple[List[Any], List[Any]]:
        """Download and extract parsing results from MinerU"""
        try:
            response = requests.get(result_url, timeout=120)
            response.raise_for_status()
            
            # Create temp directory for extraction
            with tempfile.TemporaryDirectory() as temp_dir:
                zip_path = os.path.join(temp_dir, 'result.zip')
                
                with open(zip_path, 'wb') as f:
                    f.write(response.content)
                
                # Extract and parse
                with zipfile.ZipFile(zip_path, 'r') as zip_ref:
                    zip_ref.extractall(temp_dir)
                
                # Find and parse JSON files
                sections = []
                tables = []
                
                for root, dirs, files in os.walk(temp_dir):
                    for file in files:
                        if file.endswith('.json'):
                            json_path = os.path.join(root, file)
                            with open(json_path, 'r', encoding='utf-8') as f:
                                data = json.load(f)
                                parsed_sections, parsed_tables = self._parse_mineru_json(data)
                                sections.extend(parsed_sections)
                                tables.extend(parsed_tables)
                
                return sections, tables
                
        except Exception as e:
            self.logger.error(f"[MinerU] Failed to download/extract result: {e}")
            return [], []

    def _parse_content_direct(self, content: dict) -> tuple[List[Any], List[Any]]:
        """Parse content directly from API response"""
        return self._parse_mineru_json(content)

    def _parse_mineru_json(self, data: dict) -> tuple[List[Any], List[Any]]:
        """
        Parse MinerU JSON output into sections and tables.
        
        Args:
            data: MinerU JSON data
            
        Returns:
            Tuple of (sections, tables)
        """
        sections = []
        tables = []
        
        try:
            # MinerU typically returns structured content with text and table blocks
            if isinstance(data, dict):
                content_blocks = data.get('content', data.get('blocks', []))
            elif isinstance(data, list):
                content_blocks = data
            else:
                return [], []
            
            for block in content_blocks:
                if not isinstance(block, dict):
                    continue
                    
                block_type = block.get('type', '').lower()
                
                if block_type in ['text', 'paragraph', 'title', 'heading']:
                    text = block.get('text', block.get('content', ''))
                    if text:
                        # Format: (text_content, layout_info)
                        layout = block.get('layout', {})
                        sections.append((text, json.dumps(layout) if layout else ''))
                        
                elif block_type in ['table']:
                    table_html = block.get('html', '')
                    if not table_html:
                        # Convert table data to HTML if not provided
                        table_data = block.get('data', block.get('cells', []))
                        if table_data:
                            table_html = self._convert_table_to_html(table_data)
                    
                    if table_html:
                        tables.append(table_html)
            
            return sections, tables
            
        except Exception as e:
            self.logger.error(f"[MinerU] Error parsing JSON: {e}")
            return [], []

    def _convert_table_to_html(self, table_data: List[List[str]]) -> str:
        """Convert table data to HTML format"""
        if not table_data:
            return ""
        
        html = "<table>\n"
        for i, row in enumerate(table_data):
            html += "  <tr>\n"
            tag = "th" if i == 0 else "td"
            for cell in row:
                html += f"    <{tag}>{cell}</{tag}>\n"
            html += "  </tr>\n"
        html += "</table>"
        
        return html

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: bytes = None,
        callback: Optional[Callable] = None,
        output_dir: Optional[str] = None,
        backend: str = "hybrid-auto-engine",
        server_url: Optional[str] = None,
        delete_output: bool = True,
        parse_method: str = "raw",
        **kwargs
    ) -> tuple[List[Any], List[Any]]:
        """
        Parse PDF document using MinerU service with batch processing and fault tolerance.
        
        Args:
            filepath: Path to PDF file
            binary: Binary content of PDF
            callback: Progress callback function
            output_dir: Directory for output files
            backend: MinerU backend type
            server_url: Override server URL
            delete_output: Whether to delete temporary files
            parse_method: Parsing method ("raw" or other)
            **kwargs: Additional arguments
            
        Returns:
            Tuple of (sections, tables)
        """
        if server_url:
            self.mineru_server_url = server_url
        
        temp_file = None
        created_tmp_dir = False
        
        try:
            # Handle input file
            if binary:
                temp_file = tempfile.NamedTemporaryFile(delete=False, suffix=".pdf")
                if isinstance(binary, BytesIO):
                    temp_file.write(binary.getvalue())
                else:
                    temp_file.write(binary)
                temp_file.close()
                file_path = temp_file.name
                self.logger.info(f"[MinerU] Using binary PDF data")
                if callback:
                    callback(0.05, "[MinerU] Processing binary PDF data")
            else:
                file_path = str(filepath)
                if not os.path.exists(file_path):
                    raise FileNotFoundError(f"[MinerU] PDF file not found: {file_path}")
                self.logger.info(f"[MinerU] Using PDF file: {file_path}")
                if callback:
                    callback(0.05, f"[MinerU] Processing PDF: {os.path.basename(file_path)}")
            
            # Upload file to MinerU
            if callback:
                callback(0.1, "[MinerU] Uploading file to MinerU service")
            
            file_id = self._upload_file_to_mineru(file_path, binary if isinstance(binary, bytes) else None)
            self.logger.info(f"[MinerU] File uploaded with ID: {file_id}")
            
            if callback:
                callback(0.2, "[MinerU] File uploaded successfully")
            
            # Determine page range and create batches
            # For simplicity, process entire document as single batch
            # In production, you'd determine total pages and create batches
            batch_info = BatchInfo(
                batch_idx=0,
                start_page=0,
                end_page=10000,  # Process all pages
                status=BatchStatus.PENDING
            )
            
            if callback:
                callback(0.3, "[MinerU] Starting document parsing")
            
            # Parse with retry logic
            sections = []
            tables = []
            max_attempts = MAX_RETRIES_5XX + 1
            
            for attempt in range(max_attempts):
                try:
                    if attempt > 0:
                        delay = self._calculate_backoff_delay(attempt - 1)
                        self.logger.info(f"[MinerU] Retry attempt {attempt + 1} after {delay}s delay")
                        if callback:
                            callback(0.3 + attempt * 0.05, f"[MinerU] Retry attempt {attempt + 1}")
                        time.sleep(delay)
                    
                    batch_info.status = BatchStatus.PROCESSING
                    batch_sections, batch_tables = self._parse_batch(
                        file_id=file_id,
                        start_page=batch_info.start_page,
                        end_page=batch_info.end_page,
                        backend=backend,
                        parse_method=parse_method,
                        batch_info=batch_info
                    )
                    
                    sections.extend(batch_sections)
                    tables.extend(batch_tables)
                    batch_info.status = BatchStatus.COMPLETED
                    batch_info.content_count = len(batch_sections)
                    
                    self.logger.info(f"[MinerU] Parsing completed: {len(sections)} sections, {len(tables)} tables")
                    break
                    
                except Exception as e:
                    error_type = self._classify_error(e)
                    batch_info.retry_count = attempt
                    
                    if self._should_retry(error_type, attempt) and attempt < max_attempts - 1:
                        self.logger.warning(f"[MinerU] Attempt {attempt + 1} failed, will retry: {e}")
                        continue
                    else:
                        self.logger.error(f"[MinerU] Parsing failed after {attempt + 1} attempts: {e}")
                        batch_info.status = BatchStatus.FAILED
                        batch_info.error_type = error_type
                        batch_info.error_message = str(e)
                        raise
            
            # Store batch processing result
            self.batch_processing_result = BatchProcessingResult(
                total_batches=1,
                successful_batches=1 if batch_info.status == BatchStatus.COMPLETED else 0,
                failed_batches=[] if batch_info.status == BatchStatus.COMPLETED else [batch_info],
                total_content_blocks=len(sections),
                overall_status="success" if batch_info.status == BatchStatus.COMPLETED else "failed"
            )
            
            if callback:
                callback(1.0, f"[MinerU] Completed: {len(sections)} sections, {len(tables)} tables")
            
            return sections, tables
            
        finally:
            # Cleanup
            if temp_file and os.path.exists(temp_file.name):
                try:
                    os.unlink(temp_file.name)
                except Exception as e:
                    self.logger.warning(f"[MinerU] Failed to delete temp file: {e}")
            
            if created_tmp_dir and output_dir and delete_output:
                try:
                    shutil.rmtree(output_dir)
                except Exception as e:
                    self.logger.warning(f"[MinerU] Failed to delete temp directory: {e}")

    def get_batch_processing_result(self) -> Optional[BatchProcessingResult]:
        """Get the result of the last batch processing operation"""
        return self.batch_processing_result