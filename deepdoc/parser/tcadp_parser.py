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
import base64
import hashlib
import hmac
import json
import logging
import os
import shutil
import tempfile
import time
import traceback
import zipfile
from datetime import datetime, timezone
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Any, Callable, Optional

import requests

from api.utils.configs import get_base_config
from deepdoc.parser.pdf_parser import RAGFlowPdfParser


class TencentCloudAPIClient:
    """Tencent Cloud API client without SDK dependency"""

    def __init__(self, secret_id, secret_key, region):
        self.secret_id = secret_id
        self.secret_key = secret_key
        self.region = region
        self.endpoint = "lkeap.tencentcloudapi.com"
        self.service = "lkeap"
        self.version = "2024-05-22"
        self.algorithm = "TC3-HMAC-SHA256"

    def _get_signature(self, method, uri, query_string, headers, payload, timestamp):
        """Generate Tencent Cloud API signature"""

        # 1. Create canonical request - according to Tencent Cloud official specification, only include content-type and host
        canonical_headers = f"content-type:{headers['Content-Type']}\nhost:{headers['Host']}"
        signed_headers = "content-type;host"
        hashed_request_payload = hashlib.sha256(payload.encode("utf-8")).hexdigest()

        canonical_request = f"{method}\n{uri}\n{query_string}\n{canonical_headers}\n\n{signed_headers}\n{hashed_request_payload}"

        # 2. Create string to sign
        date = datetime.fromtimestamp(timestamp, timezone.utc).strftime("%Y-%m-%d")
        credential_scope = f"{date}/{self.service}/tc3_request"

        string_to_sign = f"{self.algorithm}\n{timestamp}\n{credential_scope}\n{hashlib.sha256(canonical_request.encode('utf-8')).hexdigest()}"

        # 3. Calculate signature - fully implemented according to Tencent Cloud official TC3-HMAC-SHA256 algorithm
        def _hmac_sha256(key, msg):
            """HMAC-SHA256 calculation"""
            return hmac.new(key, msg.encode("utf-8"), hashlib.sha256)

        def _get_signature_key(key, date, service):
            """Get signature key - fully implemented according to Tencent Cloud official algorithm"""
            k_date = _hmac_sha256(("TC3" + key).encode("utf-8"), date)
            k_service = _hmac_sha256(k_date.digest(), service)
            k_signing = _hmac_sha256(k_service.digest(), "tc3_request")
            return k_signing.digest()

        signing_key = _get_signature_key(self.secret_key, date, self.service)
        signature = _hmac_sha256(signing_key, string_to_sign).hexdigest()

        # 4. Build Authorization header
        authorization = f"{self.algorithm} Credential={self.secret_id}/{credential_scope}, SignedHeaders={signed_headers}, Signature={signature}"

        return authorization, timestamp

    def reconstruct_document_sse(self, file_type, file_url=None, file_base64=None, file_start_page=1, file_end_page=1000, config=None):
        """Call document parsing API"""
        # Build request parameters
        params = {"FileType": file_type, "FileStartPageNumber": file_start_page, "FileEndPageNumber": file_end_page}
        
        # According to Tencent Cloud API documentation, either FileUrl or FileBase64 must be provided, if both are provided, only FileUrl is used
        if file_url:
            params["FileUrl"] = file_url
            logging.info(f"[TCADP] Using file URL: {file_url}")
        elif file_base64:
            params["FileBase64"] = file_base64
            logging.info(f"[TCADP] Using Base64 data, length: {len(file_base64)} characters")
        else:
            raise ValueError("Must provide either FileUrl or FileBase64 parameter")

        if config:
            params["Config"] = config

        # Build request
        method = "POST"
        uri = "/"
        query_string = ""
        payload = json.dumps(params)

        # Generate timestamp
        timestamp = int(time.time())

        # Build request headers
        headers = {
            "Content-Type": "application/json",
            "Host": self.endpoint,
            "X-TC-Action": "ReconstructDocumentSSE",
            "X-TC-Version": self.version,
            "X-TC-Region": self.region,
            "X-TC-Timestamp": str(timestamp),
            "X-TC-RequestClient": "SDK_PYTHON_3.0.1000",
        }

        # Generate signature
        authorization, _ = self._get_signature(method, uri, query_string, headers, payload, timestamp)
        headers["Authorization"] = authorization

        # Send request
        url = f"https://{self.endpoint}{uri}"
        
        logging.info(f"[TCADP] Sending request to: {url}")

        try:
            # Try non-streaming request first to avoid response content consumption issues
            response = requests.post(url, headers=headers, data=payload, timeout=300)
            logging.info(f"[TCADP] Response status code: {response.status_code}")
            
            if response.status_code != 200:
                logging.error(f"[TCADP] HTTP error: {response.status_code}")
                logging.error(f"[TCADP] Response content: {response.text}")
                return None
                
            response.raise_for_status()

            # Check response content type
            content_type = response.headers.get('content-type', '').lower()
            # Get response content
            response_text = response.text

            # First check if it's an error response
            try:
                result = json.loads(response_text)
                if "Response" in result and "Error" in result["Response"]:
                    error_info = result["Response"]["Error"]
                    error_code = error_info.get("Code", "Unknown")
                    error_message = error_info.get("Message", "Unknown error")
                    logging.error(f"[TCADP] API returned error: {error_code} - {error_message}")
                    
                    # Provide specific error handling suggestions
                    if error_code == "UnsupportedRegion":
                        logging.error("[TCADP] Unsupported region error, please check region configuration")

                    return None
            except json.JSONDecodeError:
                pass
            
            # Check if it's SSE format
            if 'text/event-stream' in content_type or 'data:' in response_text:
                logging.info("[TCADP] Detected SSE format response, using SSE processing")
                # Create mock streaming response object
                
                class MockResponse:
                    def __init__(self, text):
                        self.text = text
                        self.headers = response.headers
                        self._text = text
                    
                    def iter_lines(self, decode_unicode=True):
                        for line in self._text.split('\n'):
                            yield line
                
                mock_response = MockResponse(response_text)
                result = self._handle_sse_response(mock_response)
            else:
                logging.info("[TCADP] Detected JSON format response, parsing directly")
                try:
                    result = json.loads(response_text)
                    logging.info(f"[TCADP] JSON parsing successful: {result}")
                except json.JSONDecodeError as e:
                    logging.error(f"[TCADP] JSON parsing failed: {e}")
                    return None
            
            if not result:
                logging.error("[TCADP] All response processing methods failed")
            return result

        except requests.exceptions.Timeout as e:
            logging.error(f"[TCADP] Request timeout: {e}")
            return None
        except requests.exceptions.RequestException as e:
            logging.error(f"[TCADP] Request failed: {e}")
            if hasattr(e, "response") and e.response is not None:
                logging.error(f"[TCADP] Error response status code: {e.response.status_code}")
                logging.error(f"[TCADP] Error response content: {e.response.text}")
            return None
        except Exception as e:
            logging.error(f"[TCADP] Unknown error: {e}")
            return None

    def _handle_sse_response(self, response):
        """Handle SSE streaming response"""
        parser_result = {}
        line_count = 0
        data_count = 0
        all_lines = []

        logging.info("[TCADP] Starting to process SSE streaming response")

        try:
            # Process SSE streaming response
            for line in response.iter_lines(decode_unicode=True):
                line_count += 1
                all_lines.append(line)
                
                if line.strip():

                    # Parse SSE data
                    if line.startswith("data:"):
                        data_count += 1
                        data_str = line[5:]  # Remove 'data:' prefix

                        try:
                            data_dict = json.loads(data_str)

                            # Print progress information
                            if data_dict.get("ResponseType") == "PROGRESS":
                                progress = data_dict.get("Progress", "0")
                                logging.info(f"[TCADP] Progress: {progress}%")

                            # Save final result
                            if data_dict.get("Progress") == "100":
                                parser_result = data_dict
                                logging.info("[TCADP] Document parsing completed!")
                                logging.info(f"[TCADP] Task ID: {data_dict.get('TaskId')}")
                                logging.info(f"[TCADP] Success pages: {data_dict.get('SuccessPageNum')}")
                                logging.info(f"[TCADP] Failed pages: {data_dict.get('FailPageNum')}")

                                # Print failed page information
                                failed_pages = data_dict.get("FailedPages", [])
                                if failed_pages:
                                    logging.warning("[TCADP] Failed parsing pages:")
                                    for page in failed_pages:
                                        logging.warning(f"[TCADP]   Page number: {page.get('PageNumber')}, Error: {page.get('ErrorMsg')}")
                                
                                # Check if there's a download link
                                download_url = data_dict.get("DocumentRecognizeResultUrl")
                                if download_url:
                                    logging.info(f"[TCADP] Got download link: {download_url}")
                                else:
                                    logging.warning("[TCADP] No download link obtained")
                                
                                break  # Found final result, exit loop

                        except json.JSONDecodeError as e:
                            logging.error(f"[TCADP] Failed to parse JSON data: {e}")
                            logging.error(f"[TCADP] Raw data: {data_str}")
                            continue
                    else:
                        # Handle non-data lines (such as event types, comments, etc.)
                        logging.info(f"[TCADP] Non-data line: {line}")
                        
                        # Try to parse JSON directly (might be non-SSE format response)
                        if line.strip().startswith('{') and line.strip().endswith('}'):
                            try:
                                data_dict = json.loads(line.strip())
                                logging.info(f"[TCADP] Direct JSON parsing successful: {data_dict}")
                                
                                # Check if it's an error response
                                if "Response" in data_dict and "Error" in data_dict["Response"]:
                                    error_info = data_dict["Response"]["Error"]
                                    error_code = error_info.get("Code", "Unknown")
                                    error_message = error_info.get("Message", "Unknown error")
                                    logging.error(f"[TCADP] API returned error: {error_code} - {error_message}")
                                    # For error responses, don't set parser_result, let upper layer handle
                                    break
                                
                                # Check if it's a valid response
                                if "TaskId" in data_dict or "DocumentRecognizeResultUrl" in data_dict:
                                    parser_result = data_dict
                                    logging.info("[TCADP] Found valid response data")
                                    break
                                    
                            except json.JSONDecodeError:
                                pass

            logging.info(f"[TCADP] SSE processing completed: total lines={line_count}, data lines={data_count}")
            
            # Print all received lines for debugging
            logging.info(f"[TCADP] All received lines: {all_lines}")
            
            if not parser_result:
                logging.warning("[TCADP] No parsing result obtained, response format may be incorrect or parsing incomplete")
                # Try to return the last valid data dictionary
                if data_count > 0:
                    logging.warning("[TCADP] Attempting to use last data as result")
                    # Logic can be added here to save the last valid data

        except Exception as e:
            logging.error(f"[TCADP] Error occurred while processing SSE response: {e}")
            logging.error(f"[TCADP] Error stack trace: {traceback.format_exc()}")

        return parser_result

    def download_result_file(self, download_url, output_dir):
        """Download parsing result file"""
        if not download_url:
            logging.warning("[TCADP] No downloadable result file")
            return None

        try:
            response = requests.get(download_url)
            response.raise_for_status()

            # Ensure output directory exists
            os.makedirs(output_dir, exist_ok=True)

            # Generate filename
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            filename = f"adp_result_{timestamp}.zip"
            file_path = os.path.join(output_dir, filename)

            # Save file
            with open(file_path, "wb") as f:
                f.write(response.content)

            logging.info(f"[TCADP] Document parsing result downloaded to: {os.path.basename(file_path)}")
            return file_path

        except requests.exceptions.RequestException as e:
            logging.error(f"[TCADP] Failed to download file: {e}")
            return None


class TCADPParser(RAGFlowPdfParser):
    def __init__(self, secret_id: str = None, secret_key: str = None, region: str = "ap-guangzhou"):
        super().__init__()
        
        # First initialize logger
        self.logger = logging.getLogger(self.__class__.__name__)
        
        # Priority: read configuration from RAGFlow configuration system (service_conf.yaml)
        try:
            tcadp_parser = get_base_config("tcadp_config", {})
            if isinstance(tcadp_parser, dict) and tcadp_parser:
                self.secret_id = secret_id or tcadp_parser.get("secret_id")
                self.secret_key = secret_key or tcadp_parser.get("secret_key")
                self.region = region or tcadp_parser.get("region", "ap-guangzhou")
                self.logger.info("[TCADP] Configuration read from service_conf.yaml")
            else:
                self.logger.error("[TCADP] Please configure tcadp_config in service_conf.yaml first")

        except ImportError:
            self.logger.info("[TCADP] Configuration module import failed")

        if not self.secret_id or not self.secret_key:
            raise ValueError("[TCADP] Please set Tencent Cloud API keys, configure tcadp_config in service_conf.yaml")

    def check_installation(self) -> bool:
        """Check if Tencent Cloud API configuration is correct"""
        try:
            # Check necessary configuration parameters
            if not self.secret_id or not self.secret_key:
                self.logger.error("[TCADP] Tencent Cloud API configuration incomplete")
                return False

            # Try to create client to verify configuration
            TencentCloudAPIClient(self.secret_id, self.secret_key, self.region)
            self.logger.info("[TCADP] Tencent Cloud API configuration check passed")
            return True
        except Exception as e:
            self.logger.error(f"[TCADP] Tencent Cloud API configuration check failed: {e}")
            return False

    def _file_to_base64(self, file_path: str, binary: bytes = None) -> str:
        """Convert file to Base64 format"""
        
        if binary:
            # If binary data is directly available, convert directly
            return base64.b64encode(binary).decode('utf-8')
        else:
            # Read from file path and convert
            with open(file_path, 'rb') as f:
                file_data = f.read()
                return base64.b64encode(file_data).decode('utf-8')

    def _extract_content_from_zip(self, zip_path: str) -> list[dict[str, Any]]:
        """Extract parsing results from downloaded ZIP file"""
        results = []

        try:
            with zipfile.ZipFile(zip_path, "r") as zip_file:
                # Find JSON result files
                json_files = [f for f in zip_file.namelist() if f.endswith(".json")]

                for json_file in json_files:
                    with zip_file.open(json_file) as f:
                        data = json.load(f)
                        if isinstance(data, list):
                            results.extend(data)
                        else:
                            results.append(data)

                # Find Markdown files
                md_files = [f for f in zip_file.namelist() if f.endswith(".md")]
                for md_file in md_files:
                    with zip_file.open(md_file) as f:
                        content = f.read().decode("utf-8")
                        results.append({"type": "text", "content": content, "file": md_file})

        except Exception as e:
            self.logger.error(f"[TCADP] Failed to extract ZIP file content: {e}")

        return results

    def _parse_content_to_sections(self, content_data: list[dict[str, Any]]) -> list[tuple[str, str]]:
        """Convert parsing results to sections format"""
        sections = []

        for item in content_data:
            content_type = item.get("type", "text")
            content = item.get("content", "")

            if not content:
                continue

            # Process based on content type
            if content_type == "text" or content_type == "paragraph":
                section_text = content
            elif content_type == "table":
                # Handle table content
                table_data = item.get("table_data", {})
                if isinstance(table_data, dict):
                    # Convert table data to text
                    rows = table_data.get("rows", [])
                    section_text = "\n".join([" | ".join(row) for row in rows])
                else:
                    section_text = str(table_data)
            elif content_type == "image":
                # Handle image content
                caption = item.get("caption", "")
                section_text = f"[Image] {caption}" if caption else "[Image]"
            elif content_type == "equation":
                # Handle equation content
                section_text = f"$${content}$$"
            else:
                section_text = content

            if section_text.strip():
                # Generate position tag (simplified version)
                position_tag = "@@1\t0.0\t1000.0\t0.0\t100.0##"
                sections.append((section_text, position_tag))

        return sections

    def _parse_content_to_tables(self, content_data: list[dict[str, Any]]) -> list:
        """Convert parsing results to tables format"""
        tables = []

        for item in content_data:
            if item.get("type") == "table":
                table_data = item.get("table_data", {})
                if isinstance(table_data, dict):
                    rows = table_data.get("rows", [])
                    if rows:
                        # Convert to table format
                        table_html = "<table>\n"
                        for i, row in enumerate(rows):
                            table_html += "  <tr>\n"
                            for cell in row:
                                tag = "th" if i == 0 else "td"
                                table_html += f"    <{tag}>{cell}</{tag}>\n"
                            table_html += "  </tr>\n"
                        table_html += "</table>"
                        tables.append(table_html)

        return tables

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None,
        file_type: str = "PDF",
        file_start_page: int = 1,
        file_end_page: int = 1000,
        config: Optional[dict] = None,
        delete_output: bool = True,
        max_retries: int = 1,
    ) -> tuple:
        """Parse PDF document"""

        temp_file = None
        created_tmp_dir = False

        try:
            # Handle input file
            if binary:
                temp_file = tempfile.NamedTemporaryFile(delete=False, suffix=".pdf")
                temp_file.write(binary)
                temp_file.close()
                file_path = temp_file.name
                self.logger.info(f"[TCADP] Received binary PDF -> {os.path.basename(file_path)}")
                if callback:
                    callback(0.1, f"[TCADP] Received binary PDF -> {os.path.basename(file_path)}")
            else:
                file_path = str(filepath)
                if not os.path.exists(file_path):
                    if callback:
                        callback(-1, f"[TCADP] PDF file does not exist: {file_path}")
                    raise FileNotFoundError(f"[TCADP] PDF file does not exist: {file_path}")

            # Convert file to Base64 format
            if callback:
                callback(0.2, "[TCADP] Converting file to Base64 format")
            
            file_base64 = self._file_to_base64(file_path, binary)
            if callback:
                callback(0.25, f"[TCADP] File converted to Base64, size: {len(file_base64)} characters")

            # Create Tencent Cloud API client
            client = TencentCloudAPIClient(self.secret_id, self.secret_key, self.region)

            # Call document parsing API (with retry mechanism)
            if callback:
                callback(0.3, "[TCADP] Starting to call Tencent Cloud document parsing API")

            result = None
            for attempt in range(max_retries):
                try:
                    if attempt > 0:
                        self.logger.info(f"[TCADP] Retry attempt {attempt + 1}")
                        if callback:
                            callback(0.3 + attempt * 0.1, f"[TCADP] Retry attempt {attempt + 1}")
                        time.sleep(2 ** attempt)  # Exponential backoff
                    
                    result = client.reconstruct_document_sse(
                        file_type=file_type, 
                        file_base64=file_base64, 
                        file_start_page=file_start_page, 
                        file_end_page=file_end_page, 
                        config=config
                    )
                    
                    if result:
                        self.logger.info(f"[TCADP] Attempt {attempt + 1} successful")
                        break
                    else:
                        self.logger.warning(f"[TCADP] Attempt {attempt + 1} failed, result is None")
                        
                except Exception as e:
                    self.logger.error(f"[TCADP] Attempt {attempt + 1} exception: {e}")
                    if attempt == max_retries - 1:
                        raise

            if not result:
                error_msg = f"[TCADP] Document parsing failed, retried {max_retries} times"
                self.logger.error(error_msg)
                if callback:
                    callback(-1, error_msg)
                raise RuntimeError(error_msg)

            # Get download link
            download_url = result.get("DocumentRecognizeResultUrl")
            if not download_url:
                if callback:
                    callback(-1, "[TCADP] No parsing result download link obtained")
                raise RuntimeError("[TCADP] No parsing result download link obtained")

            if callback:
                callback(0.6, f"[TCADP] Parsing result download link: {download_url}")

            # Set output directory
            if output_dir:
                out_dir = Path(output_dir)
                out_dir.mkdir(parents=True, exist_ok=True)
            else:
                out_dir = Path(tempfile.mkdtemp(prefix="adp_pdf_"))
                created_tmp_dir = True

            # Download result file
            zip_path = client.download_result_file(download_url, str(out_dir))
            if not zip_path:
                if callback:
                    callback(-1, "[TCADP] Failed to download parsing result")
                raise RuntimeError("[TCADP] Failed to download parsing result")

            if callback:
                # Shorten file path display, only show filename
                zip_filename = os.path.basename(zip_path)
                callback(0.8, f"[TCADP] Parsing result downloaded: {zip_filename}")

            # Extract ZIP file content
            content_data = self._extract_content_from_zip(zip_path)
            self.logger.info(f"[TCADP] Extracted {len(content_data)} content blocks")

            if callback:
                callback(0.9, f"[TCADP] Extracted {len(content_data)} content blocks")

            # Convert to sections and tables format
            sections = self._parse_content_to_sections(content_data)
            tables = self._parse_content_to_tables(content_data)

            self.logger.info(f"[TCADP] Parsing completed: {len(sections)} sections, {len(tables)} tables")

            if callback:
                callback(1.0, f"[TCADP] Parsing completed: {len(sections)} sections, {len(tables)} tables")

            return sections, tables

        finally:
            # Clean up temporary files
            if temp_file and os.path.exists(temp_file.name):
                try:
                    os.unlink(temp_file.name)
                except Exception:
                    pass

            if delete_output and created_tmp_dir and out_dir.exists():
                try:
                    shutil.rmtree(out_dir)
                except Exception:
                    pass


if __name__ == "__main__":
    # Test ADP parser
    parser = TCADPParser()
    print("ADP available:", parser.check_installation())

    # Test parsing
    filepath = ""
    if filepath and os.path.exists(filepath):
        with open(filepath, "rb") as file:
            sections, tables = parser.parse_pdf(filepath=filepath, binary=file.read())
            print(f"Parsing result: {len(sections)} sections, {len(tables)} tables")
            for i, (section, tag) in enumerate(sections[:3]):  # Only print first 3
                print(f"Section {i + 1}: {section[:100]}...")
