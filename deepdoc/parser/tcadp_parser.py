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
import json
import logging
import os
import re
import shutil
import tempfile
import time
import traceback
import types
import zipfile
from datetime import datetime
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Any, Callable, Optional

import requests
from tencentcloud.common import credential
from tencentcloud.common.profile.client_profile import ClientProfile
from tencentcloud.common.profile.http_profile import HttpProfile
from tencentcloud.common.exception.tencent_cloud_sdk_exception import TencentCloudSDKException
from tencentcloud.lkeap.v20240522 import lkeap_client, models

from common.config_utils import get_base_config
from deepdoc.parser.pdf_parser import RAGFlowPdfParser


class TencentCloudAPIClient:
    """Tencent Cloud API client using official SDK"""

    def __init__(self, secret_id, secret_key, region):
        self.secret_id = secret_id
        self.secret_key = secret_key
        self.region = region
        self.outlines = []

        # Create credentials
        self.cred = credential.Credential(secret_id, secret_key)

        # Instantiate an http option, optional, can be skipped if no special requirements
        self.httpProfile = HttpProfile()
        self.httpProfile.endpoint = "lkeap.tencentcloudapi.com"

        # Instantiate a client option, optional, can be skipped if no special requirements
        self.clientProfile = ClientProfile()
        self.clientProfile.httpProfile = self.httpProfile

        # Instantiate the client object for the product to be requested, clientProfile is optional
        self.client = lkeap_client.LkeapClient(self.cred, region, self.clientProfile)

    def reconstruct_document_sse(self, file_type, file_url=None, file_base64=None, file_start_page=1, file_end_page=1000, config=None):
        """Call document parsing API using official SDK"""
        try:
            # Instantiate a request object, each interface corresponds to a request object
            req = models.ReconstructDocumentSSERequest()

            # Build request parameters
            params = {
                "FileType": file_type,
                "FileStartPageNumber": file_start_page,
                "FileEndPageNumber": file_end_page,
            }

            # According to Tencent Cloud API documentation, either FileUrl or FileBase64 parameter must be provided, if both are provided only FileUrl will be used
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

            req.from_json_string(json.dumps(params))

            # The returned resp is an instance of ReconstructDocumentSSEResponse, corresponding to the request object
            resp = self.client.ReconstructDocumentSSE(req)
            parser_result = {}

            # Output json format string response
            if isinstance(resp, types.GeneratorType):  # Streaming response
                logging.info("[TCADP] Detected streaming response")
                for event in resp:
                    logging.info(f"[TCADP] Received event: {event}")
                    if event.get('data'):
                        try:
                            data_dict = json.loads(event['data'])
                            logging.info(f"[TCADP] Parsed data: {data_dict}")

                            if data_dict.get('Progress') == "100":
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

                                # Check if there is a download link
                                download_url = data_dict.get("DocumentRecognizeResultUrl")
                                if download_url:
                                    logging.info(f"[TCADP] Got download link: {download_url}")
                                else:
                                    logging.warning("[TCADP] No download link obtained")

                                break  # Found final result, exit loop
                            else:
                                # Print progress information
                                progress = data_dict.get("Progress", "0")
                                logging.info(f"[TCADP] Progress: {progress}%")
                        except json.JSONDecodeError as e:
                            logging.error(f"[TCADP] Failed to parse JSON data: {e}")
                            logging.error(f"[TCADP] Raw data: {event.get('data')}")
                            continue
                    else:
                        logging.info(f"[TCADP] Event without data: {event}")
            else:  # Non-streaming response
                logging.info("[TCADP] Detected non-streaming response")
                if hasattr(resp, 'data') and resp.data:
                    try:
                        data_dict = json.loads(resp.data)
                        parser_result = data_dict
                        logging.info(f"[TCADP] JSON parsing successful: {parser_result}")
                    except json.JSONDecodeError as e:
                        logging.error(f"[TCADP] JSON parsing failed: {e}")
                        return None
                else:
                    logging.error("[TCADP] No data in response")
                    return None

            return parser_result

        except TencentCloudSDKException as err:
            logging.error(f"[TCADP] Tencent Cloud SDK error: {err}")
            return None
        except Exception as e:
            logging.error(f"[TCADP] Unknown error: {e}")
            logging.error(f"[TCADP] Error stack trace: {traceback.format_exc()}")
            return None

    def download_result_file(self, download_url, output_dir):
        """Download parsing result file"""
        if not download_url:
            logging.warning("[TCADP] No downloadable result file")
            return None

        try:
            # Ensure output directory exists
            os.makedirs(output_dir, exist_ok=True)

            # Generate filename
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            filename = f"tcadp_result_{timestamp}.zip"
            file_path = os.path.join(output_dir, filename)

            with requests.get(download_url, stream=True) as response:
                response.raise_for_status()
                with open(file_path, "wb") as f:
                    response.raw.decode_content = True
                    shutil.copyfileobj(response.raw, f)

            logging.info(f"[TCADP] Document parsing result downloaded to: {os.path.basename(file_path)}")
            return file_path

        except Exception as e:
            logging.error(f"[TCADP] Failed to download file: {e}")
            try:
                if "file_path" in locals() and os.path.exists(file_path):
                    os.unlink(file_path)
            except Exception:
                pass
            return None


class TCADPParser(RAGFlowPdfParser):
    def __init__(self, secret_id: str = None, secret_key: str = None, region: str = "ap-guangzhou",
                 table_result_type: str = None, markdown_image_response_type: str = None):
        super().__init__()

        # First initialize logger
        self.logger = logging.getLogger(self.__class__.__name__)

        # Log received parameters
        self.logger.info(f"[TCADP] Initializing with parameters - table_result_type: {table_result_type}, markdown_image_response_type: {markdown_image_response_type}")

        # Priority: read configuration from RAGFlow configuration system (service_conf.yaml)
        try:
            tcadp_parser = get_base_config("tcadp_config", {})
            if isinstance(tcadp_parser, dict) and tcadp_parser:
                self.secret_id = secret_id or tcadp_parser.get("secret_id")
                self.secret_key = secret_key or tcadp_parser.get("secret_key")
                self.region = region or tcadp_parser.get("region", "ap-guangzhou")
                # Set table_result_type and markdown_image_response_type from config or parameters
                self.table_result_type = table_result_type if table_result_type is not None else tcadp_parser.get("table_result_type", "1")
                self.markdown_image_response_type = markdown_image_response_type if markdown_image_response_type is not None else tcadp_parser.get("markdown_image_response_type", "1")

            else:
                self.logger.error("[TCADP] Please configure tcadp_config in service_conf.yaml first")
                # If config file is empty, use provided parameters or defaults
                self.secret_id = secret_id
                self.secret_key = secret_key
                self.region = region or "ap-guangzhou"
                self.table_result_type = table_result_type if table_result_type is not None else "1"
                self.markdown_image_response_type = markdown_image_response_type if markdown_image_response_type is not None else "1"

        except ImportError:
            self.logger.info("[TCADP] Configuration module import failed")
            # If config file is not available, use provided parameters or defaults
            self.secret_id = secret_id
            self.secret_key = secret_key
            self.region = region or "ap-guangzhou"
            self.table_result_type = table_result_type if table_result_type is not None else "1"
            self.markdown_image_response_type = markdown_image_response_type if markdown_image_response_type is not None else "1"

        # Log final values
        self.logger.info(f"[TCADP] Final values - table_result_type: {self.table_result_type}, markdown_image_response_type: {self.markdown_image_response_type}")

        if not self.secret_id or not self.secret_key:
            raise ValueError("[TCADP] Please set Tencent Cloud API keys, configure tcadp_config in service_conf.yaml")

    @staticmethod
    def _is_zipinfo_symlink(member: zipfile.ZipInfo) -> bool:
        return (member.external_attr >> 16) & 0o170000 == 0o120000

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
                members = zip_file.infolist()
                for member in members:
                    name = member.filename.replace("\\", "/")
                    if member.is_dir():
                        continue
                    if member.flag_bits & 0x1:
                        raise RuntimeError(f"[TCADP] Encrypted zip entry not supported: {member.filename}")
                    if self._is_zipinfo_symlink(member):
                        raise RuntimeError(f"[TCADP] Symlink zip entry not supported: {member.filename}")
                    if name.startswith("/") or name.startswith("//") or re.match(r"^[A-Za-z]:", name):
                        raise RuntimeError(f"[TCADP] Unsafe zip path (absolute): {member.filename}")
                    parts = [p for p in name.split("/") if p not in ("", ".")]
                    if any(p == ".." for p in parts):
                        raise RuntimeError(f"[TCADP] Unsafe zip path (traversal): {member.filename}")

                    if not (name.endswith(".json") or name.endswith(".md")):
                        continue

                    with zip_file.open(member) as f:
                        if name.endswith(".json"):
                            data = json.load(f)
                            if isinstance(data, list):
                                results.extend(data)
                            else:
                                results.append(data)
                        else:
                            content = f.read().decode("utf-8")
                            results.append({"type": "text", "content": content, "file": name})

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
        file_start_page: Optional[int] = 1,
        file_end_page: Optional[int] = 1000,
        delete_output: Optional[bool] = True,
        max_retries: Optional[int] = 1,
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

                    config = {
                        "TableResultType": self.table_result_type,
                        "MarkdownImageResponseType": self.markdown_image_response_type
                    }

                    self.logger.info(f"[TCADP] API request config - TableResultType: {self.table_result_type}, MarkdownImageResponseType: {self.markdown_image_response_type}")

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
