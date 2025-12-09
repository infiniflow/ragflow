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
import platform
import re
import subprocess
import sys
import tempfile
import threading
import time
import zipfile
from io import BytesIO
from os import PathLike
from pathlib import Path
from queue import Empty, Queue
from typing import Any, Callable, Optional

import numpy as np
import pdfplumber
import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry
from PIL import Image
from strenum import StrEnum

from deepdoc.parser.pdf_parser import RAGFlowPdfParser

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


class ProgressTracker:
    """
    Progress tracker: parse mineru output for progress and implement intelligent stall-based timeout.
    
    Features:
    1. Parse tqdm progress bar output (e.g., "100%|████| 659/659")
    2. Intelligent stall-based timeout (reset timer when progress updates)
    3. Report progress to RagFlow callback
    
    Design principles:
    - Standalone helper class, does not modify MinerUParser core logic
    - Parsing failure does not affect functionality, just no progress display
    - Compatible with upstream code changes
    """
    
    # Match tqdm progress bar format: "100%|████████████| 659/659" or "Page 45/659"
    TQDM_PATTERN = re.compile(r'(\d+)%\|[^\|]*\|\s*(\d+)/(\d+)')
    PAGE_PATTERN = re.compile(r'[Pp]age\s*(\d+)\s*/\s*(\d+)')
    STEP_PATTERN = re.compile(r'(\d+)/(\d+)\s*\[')  # "45/659 [00:30<01:00"
    
    def __init__(
        self, 
        callback: Optional[Callable] = None,
        stall_timeout: int = 300,  # 5 minutes without progress triggers timeout
        progress_base: float = 0.25,  # Base progress value
        progress_range: float = 0.45,  # Progress range (0.25 ~ 0.70)
    ):
        self.callback = callback
        self.stall_timeout = stall_timeout
        self.progress_base = progress_base
        self.progress_range = progress_range
        
        self.last_progress_time = time.time()
        self.last_progress_value = 0.0
        self.total_pages = 0
        self.current_page = 0
        self.logger = logging.getLogger(self.__class__.__name__)
    
    def parse_line(self, line: str) -> Optional[float]:
        """
        Parse a line of output and extract progress information.
        Returns progress value 0.0 ~ 1.0, or None if cannot parse.
        """
        line = line.strip()
        if not line:
            return None
        
        # Try to match tqdm format: "100%|████| 659/659"
        if match := self.TQDM_PATTERN.search(line):
            percent = int(match.group(1))
            current = int(match.group(2))
            total = int(match.group(3))
            self.current_page = current
            self.total_pages = total
            return percent / 100.0
        
        # Try to match "Page 45/659" format
        if match := self.PAGE_PATTERN.search(line):
            current = int(match.group(1))
            total = int(match.group(2))
            self.current_page = current
            self.total_pages = total
            return current / total if total > 0 else None
        
        # Try to match "45/659 [" format
        if match := self.STEP_PATTERN.search(line):
            current = int(match.group(1))
            total = int(match.group(2))
            self.current_page = current
            self.total_pages = total
            return current / total if total > 0 else None
        
        return None
    
    def on_output(self, line: str, prefix: str = "STDOUT") -> None:
        """
        Process a line of output. Parse progress and update state.
        """
        progress = self.parse_line(line)
        
        if progress is not None and progress > self.last_progress_value:
            self.last_progress_value = progress
            self.last_progress_time = time.time()  # Reset timeout timer!
            
            # Calculate actual progress value (map to progress_base ~ progress_base+progress_range)
            actual_progress = self.progress_base + (progress * self.progress_range)
            
            if self.callback:
                msg = f"[MinerU] Processing page {self.current_page}/{self.total_pages}" if self.total_pages else f"[MinerU] Progress: {progress:.0%}"
                try:
                    self.callback(actual_progress, msg)
                except Exception as e:
                    self.logger.warning(f"[MinerU] Progress callback failed: {e}")
    
    def is_stalled(self) -> bool:
        """Check if stalled (no progress update for a long time)"""
        return (time.time() - self.last_progress_time) > self.stall_timeout
    
    def get_stall_info(self) -> str:
        """Get stall information for error reporting"""
        stall_duration = int(time.time() - self.last_progress_time)
        return (
            f"No progress for {stall_duration}s "
            f"(stalled at {self.last_progress_value:.0%}, "
            f"page {self.current_page}/{self.total_pages or '?'})"
        )
    
    def reset(self) -> None:
        """Reset state (for retry)"""
        self.last_progress_time = time.time()
        self.last_progress_value = 0.0
        self.current_page = 0


class MinerUContentType(StrEnum):
    IMAGE = "image"
    TABLE = "table"
    TEXT = "text"
    EQUATION = "equation"
    CODE = "code"
    LIST = "list"
    DISCARDED = "discarded"
    HEADER = "header"
    PAGE_NUMBER = "page_number"


class MinerUParser(RAGFlowPdfParser):
    """MinerU PDF Parser with multiple backend support.
    
    Backends:
    - pipeline: Local MinerU with traditional pipeline (requires GPU)
    - vlm-http-client: MinerU CLI calling external vLLM server
    - vlm-direct: NEW! Direct vLLM API call from RagFlow (bypasses mineru-api)
    - api: External mineru-api service (legacy, prone to timeout issues)
    """
    
    # Timeout settings for different operations
    API_CONNECT_TIMEOUT = 30  # seconds to establish connection
    API_READ_TIMEOUT_PER_PAGE = 60  # seconds per page for processing
    API_MAX_TIMEOUT = 1800  # maximum total timeout
    HEARTBEAT_INTERVAL = 30  # seconds between progress updates
    
    def __init__(self, mineru_path: str = "mineru", mineru_api: str = "http://host.docker.internal:9987", mineru_server_url: str = ""):
        self.mineru_path = Path(mineru_path)
        self.mineru_api = mineru_api.rstrip("/")
        self.mineru_server_url = mineru_server_url.rstrip("/")
        self.using_api = False
        self.using_vlm_direct = False  # New flag for direct vLLM mode
        self.outlines = []
        self.logger = logging.getLogger(self.__class__.__name__)
        self._session = None  # Reusable HTTP session

    def _extract_zip_no_root(self, zip_path, extract_to, root_dir):
        self.logger.info(f"[MinerU] Extract zip: zip_path={zip_path}, extract_to={extract_to}, root_hint={root_dir}")
        with zipfile.ZipFile(zip_path, "r") as zip_ref:
            if not root_dir:
                files = zip_ref.namelist()
                if files and files[0].endswith("/"):
                    root_dir = files[0]
                else:
                    root_dir = None

            if not root_dir or not root_dir.endswith("/"):
                self.logger.info(f"[MinerU] No root directory found, extracting all (root_hint={root_dir})")
                zip_ref.extractall(extract_to)
                return

            root_len = len(root_dir)
            for member in zip_ref.infolist():
                filename = member.filename
                if filename == root_dir:
                    self.logger.info("[MinerU] Ignore root folder...")
                    continue

                path = filename
                if path.startswith(root_dir):
                    path = path[root_len:]

                full_path = os.path.join(extract_to, path)
                if member.is_dir():
                    os.makedirs(full_path, exist_ok=True)
                else:
                    os.makedirs(os.path.dirname(full_path), exist_ok=True)
                    with open(full_path, "wb") as f:
                        f.write(zip_ref.read(filename))

    def _get_http_session(self) -> requests.Session:
        """Get or create a reusable HTTP session with retry logic."""
        if self._session is None:
            self._session = requests.Session()
            retry_strategy = Retry(
                total=3,
                backoff_factor=0.5,
                status_forcelist=[500, 502, 503, 504],
                allowed_methods=["HEAD", "GET", "POST"],
            )
            adapter = HTTPAdapter(max_retries=retry_strategy)
            self._session.mount("http://", adapter)
            self._session.mount("https://", adapter)
        return self._session

    def _is_http_endpoint_valid(self, url, timeout=5):
        try:
            session = self._get_http_session()
            response = session.head(url, timeout=timeout, allow_redirects=True)
            return response.status_code in [200, 301, 302, 307, 308]
        except Exception:
            return False

    def check_installation(self, backend: str = "pipeline", server_url: Optional[str] = None) -> tuple[bool, str]:
        reason = ""

        valid_backends = ["pipeline", "vlm-http-client", "vlm-transformers", "vlm-vllm-engine", "vlm-direct"]
        if backend not in valid_backends:
            reason = f"[MinerU] Invalid backend '{backend}'. Valid backends are: {valid_backends}"
            self.logger.warning(reason)
            return False, reason

        subprocess_kwargs = {
            "capture_output": True,
            "text": True,
            "check": True,
            "encoding": "utf-8",
            "errors": "ignore",
        }

        if platform.system() == "Windows":
            subprocess_kwargs["creationflags"] = getattr(subprocess, "CREATE_NO_WINDOW", 0)

        if server_url is None:
            server_url = self.mineru_server_url

        # NEW: vlm-direct mode - direct connection to vLLM server, bypassing mineru-api
        if backend == "vlm-direct" and server_url:
            try:
                # Check if vLLM server is accessible (OpenAI-compatible API)
                health_endpoints = ["/health", "/v1/models", "/openapi.json"]
                server_accessible = False
                for endpoint in health_endpoints:
                    if self._is_http_endpoint_valid(server_url.rstrip("/") + endpoint):
                        server_accessible = True
                        break
                
                if server_accessible:
                    self.logger.info(f"[MinerU] vlm-direct server accessible: {server_url}")
                    self.using_api = False
                    self.using_vlm_direct = True
                    return True, reason
                else:
                    reason = f"[MinerU] vlm-direct server not accessible: {server_url}"
                    self.logger.warning(reason)
                    return False, reason
            except Exception as e:
                reason = f"[MinerU] vlm-direct server check failed: {server_url}: {e}"
                self.logger.warning(reason)
                return False, reason

        if backend == "vlm-http-client" and server_url:
            try:
                server_accessible = self._is_http_endpoint_valid(server_url + "/openapi.json")
                self.logger.info(f"[MinerU] vlm-http-client server check: {server_accessible}")
                if server_accessible:
                    self.using_api = False  # We are using http client, not API
                    return True, reason
                else:
                    reason = f"[MinerU] vlm-http-client server not accessible: {server_url}"
                    self.logger.warning(f"[MinerU] vlm-http-client server not accessible: {server_url}")
                    return False, reason
            except Exception as e:
                self.logger.warning(f"[MinerU] vlm-http-client server check failed: {e}")
                try:
                    response = requests.get(server_url, timeout=5)
                    self.logger.info(f"[MinerU] vlm-http-client server connection check: success with status {response.status_code}")
                    self.using_api = False
                    return True, reason
                except Exception as e:
                    reason = f"[MinerU] vlm-http-client server connection check failed: {server_url}: {e}"
                    self.logger.warning(f"[MinerU] vlm-http-client server connection check failed: {server_url}: {e}")
                    return False, reason

        try:
            result = subprocess.run([str(self.mineru_path), "--version"], **subprocess_kwargs)
            version_info = result.stdout.strip()
            if version_info:
                self.logger.info(f"[MinerU] Detected version: {version_info}")
            else:
                self.logger.info("[MinerU] Detected MinerU, but version info is empty.")
            return True, reason
        except subprocess.CalledProcessError as e:
            self.logger.warning(f"[MinerU] Execution failed (exit code {e.returncode}).")
        except FileNotFoundError:
            self.logger.warning("[MinerU] MinerU not found. Please install it via: pip install -U 'mineru[core]'")
        except Exception as e:
            self.logger.error(f"[MinerU] Unexpected error during installation check: {e}")

        # If executable check fails, try API check
        try:
            if self.mineru_api:
                # check openapi.json
                openapi_exists = self._is_http_endpoint_valid(self.mineru_api + "/openapi.json")
                if not openapi_exists:
                    reason = "[MinerU] Failed to detect vaild MinerU API server"
                    return openapi_exists, reason
                self.logger.info(f"[MinerU] Detected {self.mineru_api}/openapi.json: {openapi_exists}")
                self.using_api = openapi_exists
                return openapi_exists, reason
            else:
                self.logger.info("[MinerU] api not exists.")
        except Exception as e:
            reason = f"[MinerU] Unexpected error during api check: {e}"
            self.logger.error(f"[MinerU] Unexpected error during api check: {e}")
        return False, reason

    def _run_mineru(
        self, input_path: Path, output_dir: Path, method: str = "auto", backend: str = "pipeline", lang: Optional[str] = None, server_url: Optional[str] = None, callback: Optional[Callable] = None
    ):
        if self.using_vlm_direct:
            self._run_mineru_vlm_direct(input_path, output_dir, server_url, callback)
        elif self.using_api:
            self._run_mineru_api(input_path, output_dir, method, backend, lang, callback)
        else:
            self._run_mineru_executable(input_path, output_dir, method, backend, lang, server_url, callback)

    def _run_mineru_api(self, input_path: Path, output_dir: Path, method: str = "auto", backend: str = "pipeline", lang: Optional[str] = None, callback: Optional[Callable] = None):
        output_zip_path = os.path.join(str(output_dir), "output.zip")

        pdf_file_path = str(input_path)

        if not os.path.exists(pdf_file_path):
            raise RuntimeError(f"[MinerU] PDF file not exists: {pdf_file_path}")

        pdf_file_name = Path(pdf_file_path).stem.strip()
        output_path = os.path.join(str(output_dir), pdf_file_name, method)
        os.makedirs(output_path, exist_ok=True)

        files = {"files": (pdf_file_name + ".pdf", open(pdf_file_path, "rb"), "application/pdf")}

        data = {
            "output_dir": "./output",
            "lang_list": lang,
            "backend": backend,
            "parse_method": method,
            "formula_enable": True,
            "table_enable": True,
            "server_url": None,
            "return_md": True,
            "return_middle_json": True,
            "return_model_output": True,
            "return_content_list": True,
            "return_images": True,
            "response_format_zip": True,
            "start_page_id": 0,
            "end_page_id": 99999,
        }

        headers = {"Accept": "application/json"}
        
        # Calculate dynamic timeout based on expected page count
        # Estimate ~60s per page, with minimum 5 minutes
        estimated_pages = data.get("end_page_id", 100) - data.get("start_page_id", 0)
        dynamic_timeout = max(300, min(estimated_pages * self.API_READ_TIMEOUT_PER_PAGE, self.API_MAX_TIMEOUT))
        
        self.logger.info(f"[MinerU] invoke api: {self.mineru_api}/file_parse (timeout={dynamic_timeout}s)")
        if callback:
            callback(0.20, f"[MinerU] invoke api: {self.mineru_api}/file_parse")
        
        # Use session with retry logic and streaming response
        session = self._get_http_session()
        last_heartbeat = time.time()
        
        try:
            # Stream response to allow progress monitoring
            with session.post(
                url=f"{self.mineru_api}/file_parse",
                files=files,
                data=data,
                headers=headers,
                timeout=(self.API_CONNECT_TIMEOUT, dynamic_timeout),
                stream=True
            ) as response:
                response.raise_for_status()
                
                if response.headers.get("Content-Type") == "application/zip":
                    self.logger.info(f"[MinerU] zip file returned, saving to {output_zip_path}...")
                    if callback:
                        callback(0.30, f"[MinerU] zip file returned, saving to {output_zip_path}...")
                    
                    # Stream write with progress updates
                    total_size = int(response.headers.get('content-length', 0))
                    downloaded = 0
                    with open(output_zip_path, "wb") as f:
                        for chunk in response.iter_content(chunk_size=1024 * 1024):  # 1MB chunks
                            if chunk:
                                f.write(chunk)
                                downloaded += len(chunk)
                                
                                # Send heartbeat every 30 seconds during download
                                now = time.time()
                                if callback and now - last_heartbeat >= self.HEARTBEAT_INTERVAL:
                                    progress = 0.30 + (0.10 * downloaded / total_size) if total_size else 0.35
                                    callback(progress, f"[MinerU] Downloading... {downloaded // (1024*1024)}MB")
                                    last_heartbeat = now
                    
                    self.logger.info(f"[MinerU] Unzip to {output_path}...")
                    self._extract_zip_no_root(output_zip_path, output_path, pdf_file_name + "/")
                    
                    if callback:
                        callback(0.40, f"[MinerU] Unzip to {output_path}...")
                else:
                    self.logger.warning("[MinerU] not zip returned from api: %s" % response.headers.get("Content-Type"))
                    
        except requests.exceptions.Timeout as e:
            raise RuntimeError(f"[MinerU] API timeout after {dynamic_timeout}s. Consider using vlm-direct mode. Error: {e}")
        except requests.exceptions.ConnectionError as e:
            raise RuntimeError(f"[MinerU] API connection error. Is mineru-api running? Error: {e}")
        except Exception as e:
            raise RuntimeError(f"[MinerU] API failed with exception: {e}")
        
        self.logger.info("[MinerU] API completed successfully.")

    def _run_mineru_vlm_direct(
        self, input_path: Path, output_dir: Path, server_url: Optional[str] = None, callback: Optional[Callable] = None
    ):
        """
        Direct vLLM integration mode - bypasses mineru-api and calls vLLM server directly.
        
        This method uses mineru's internal vlm-http-client backend to connect directly to
        a vLLM server, eliminating the mineru-api middleware that can cause deadlocks.
        
        Requires: mineru[core] package installed
        """
        if not server_url:
            server_url = self.mineru_server_url
            
        if not server_url:
            raise RuntimeError("[MinerU] vlm-direct mode requires MINERU_SERVER_URL to be set")
        
        self.logger.info(f"[MinerU] Starting vlm-direct mode with server: {server_url}")
        if callback:
            callback(0.15, f"[MinerU] Connecting to vLLM server: {server_url}")
        
        try:
            # Try to import mineru's vlm_analyze module
            from mineru.backend.vlm.vlm_analyze import doc_analyze
            from mineru.data.data_reader_writer import FileBasedDataWriter
        except ImportError as e:
            self.logger.warning(f"[MinerU] mineru package not available, falling back to executable mode: {e}")
            # Fallback to executable mode with vlm-http-client backend
            self._run_mineru_executable(
                input_path, output_dir, 
                method="auto", 
                backend="vlm-http-client", 
                server_url=server_url, 
                callback=callback
            )
            return
        
        pdf_file_name = input_path.stem.replace(" ", "")
        local_md_dir = output_dir / pdf_file_name / "vlm"
        local_image_dir = local_md_dir / "images"
        local_md_dir.mkdir(parents=True, exist_ok=True)
        local_image_dir.mkdir(parents=True, exist_ok=True)
        
        self.logger.info(f"[MinerU] Output directory: {local_md_dir}")
        if callback:
            callback(0.20, f"[MinerU] Output directory: {local_md_dir}")
        
        # Read PDF bytes
        with open(input_path, "rb") as f:
            pdf_bytes = f.read()
        
        # Create image writer for mineru
        image_writer = FileBasedDataWriter(str(local_image_dir))
        
        try:
            if callback:
                callback(0.25, "[MinerU] Starting VLM processing (direct mode)...")
            
            # Call mineru's doc_analyze with http-client backend
            middle_json, results = doc_analyze(
                pdf_bytes=pdf_bytes,
                image_writer=image_writer,
                backend="http-client",
                server_url=server_url,
                max_concurrency=10,
                http_timeout=self.API_READ_TIMEOUT_PER_PAGE,
                max_retries=3,
                retry_backoff_factor=0.5,
            )
            
            if callback:
                callback(0.60, "[MinerU] VLM processing completed, writing output...")
            
            # Write content_list.json (required by _read_output)
            content_list = self._convert_middle_json_to_content_list(middle_json)
            content_list_path = local_md_dir / f"{pdf_file_name}_content_list.json"
            with open(content_list_path, "w", encoding="utf-8") as f:
                json.dump(content_list, f, ensure_ascii=False, indent=2)
            
            self.logger.info(f"[MinerU] vlm-direct completed, output: {content_list_path}")
            if callback:
                callback(0.70, f"[MinerU] vlm-direct completed successfully")
                
        except Exception as e:
            self.logger.error(f"[MinerU] vlm-direct failed: {e}")
            raise RuntimeError(f"[MinerU] vlm-direct mode failed: {e}")
    
    def _convert_middle_json_to_content_list(self, middle_json: dict) -> list:
        """Convert mineru's middle_json format to content_list format."""
        content_list = []
        pdf_info = middle_json.get("pdf_info", [])
        
        for page_idx, page_info in enumerate(pdf_info):
            preproc_blocks = page_info.get("preproc_blocks", [])
            for block in preproc_blocks:
                block_type = block.get("type", "text")
                
                item = {
                    "type": block_type,
                    "page_idx": page_idx,
                    "bbox": block.get("bbox", [0, 0, 0, 0]),
                }
                
                if block_type == "text":
                    lines = block.get("lines", [])
                    text = " ".join(
                        span.get("content", "") 
                        for line in lines 
                        for span in line.get("spans", [])
                    )
                    item["text"] = text
                elif block_type == "table":
                    item["table_body"] = block.get("html", "")
                    item["table_caption"] = block.get("caption", [])
                    item["table_footnote"] = block.get("footnote", [])
                elif block_type == "image":
                    item["image_caption"] = block.get("caption", [])
                    item["image_footnote"] = block.get("footnote", [])
                    item["img_path"] = block.get("img_path", "")
                elif block_type == "equation":
                    item["text"] = block.get("latex", "")
                
                content_list.append(item)
        
        return content_list

    def _run_mineru_executable(
        self, input_path: Path, output_dir: Path, method: str = "auto", backend: str = "pipeline", lang: Optional[str] = None, server_url: Optional[str] = None, callback: Optional[Callable] = None
    ):
        cmd = [str(self.mineru_path), "-p", str(input_path), "-o", str(output_dir), "-m", method]
        if backend:
            cmd.extend(["-b", backend])
        if lang:
            cmd.extend(["-l", lang])
        if server_url and backend in ("vlm-http-client", "vlm-direct"):
            cmd.extend(["-u", server_url])

        self.logger.info(f"[MinerU] Running command: {' '.join(cmd)}")
        if callback:
            callback(0.20, f"[MinerU] Starting {backend} backend...")

        subprocess_kwargs = {
            "stdout": subprocess.PIPE,
            "stderr": subprocess.PIPE,
            "text": True,
            "encoding": "utf-8",
            "errors": "ignore",
            "bufsize": 1,
        }

        if platform.system() == "Windows":
            subprocess_kwargs["creationflags"] = getattr(subprocess, "CREATE_NO_WINDOW", 0)

        process = subprocess.Popen(cmd, **subprocess_kwargs)
        stdout_queue, stderr_queue = Queue(), Queue()
        
        # Create progress tracker (intelligent timeout + progress parsing)
        tracker = ProgressTracker(
            callback=callback,
            stall_timeout=300,  # 5 minutes without progress triggers warning
            progress_base=0.25,
            progress_range=0.45,
        )

        def enqueue_output(pipe, queue, prefix):
            for line in iter(pipe.readline, ""):
                if line.strip():
                    queue.put((prefix, line.strip()))
            pipe.close()

        threading.Thread(target=enqueue_output, args=(process.stdout, stdout_queue, "STDOUT"), daemon=True).start()
        threading.Thread(target=enqueue_output, args=(process.stderr, stderr_queue, "STDERR"), daemon=True).start()

        while process.poll() is None:
            for q in (stdout_queue, stderr_queue):
                try:
                    while True:
                        prefix, line = q.get_nowait()
                        if prefix == "STDOUT":
                            self.logger.info(f"[MinerU] {line}")
                            # Parse progress and update callback
                            tracker.on_output(line, prefix)
                        else:
                            self.logger.warning(f"[MinerU] {line}")
                except Empty:
                    pass
            
            # Check if stalled (progress-based intelligent timeout)
            if tracker.is_stalled():
                stall_info = tracker.get_stall_info()
                self.logger.warning(f"[MinerU] Process appears stalled: {stall_info}")
                if callback:
                    callback(tracker.progress_base + tracker.last_progress_value * tracker.progress_range, 
                             f"[MinerU] Warning: {stall_info}")
                # Note: only warning here, not forcibly terminating the process
                # because some PDFs do take a long time to process certain pages
                tracker.last_progress_time = time.time()  # Reset to avoid repeated warnings
            
            time.sleep(0.1)

        return_code = process.wait()
        if return_code != 0:
            raise RuntimeError(f"[MinerU] Process failed with exit code {return_code}")
        self.logger.info("[MinerU] Command completed successfully.")

    def __images__(self, fnm, zoomin: int = 1, page_from=0, page_to=600, callback=None):
        self.page_from = page_from
        self.page_to = page_to
        try:
            with pdfplumber.open(fnm) if isinstance(fnm, (str, PathLike)) else pdfplumber.open(BytesIO(fnm)) as pdf:
                self.pdf = pdf
                self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).original for _, p in enumerate(self.pdf.pages[page_from:page_to])]
        except Exception as e:
            self.page_images = None
            self.total_page = 0
            self.logger.exception(e)

    def _line_tag(self, bx):
        pn = [bx["page_idx"] + 1]
        positions = bx.get("bbox", (0, 0, 0, 0))
        x0, top, x1, bott = positions

        if hasattr(self, "page_images") and self.page_images and len(self.page_images) > bx["page_idx"]:
            page_width, page_height = self.page_images[bx["page_idx"]].size
            x0 = (x0 / 1000.0) * page_width
            x1 = (x1 / 1000.0) * page_width
            top = (top / 1000.0) * page_height
            bott = (bott / 1000.0) * page_height

        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format("-".join([str(p) for p in pn]), x0, x1, top, bott)

    def crop(self, text, ZM=1, need_position=False):
        imgs = []
        poss = self.extract_positions(text)
        if not poss:
            if need_position:
                return None, None
            return

        if not getattr(self, "page_images", None):
            self.logger.warning("[MinerU] crop called without page images; skipping image generation.")
            if need_position:
                return None, None
            return

        page_count = len(self.page_images)

        filtered_poss = []
        for pns, left, right, top, bottom in poss:
            if not pns:
                self.logger.warning("[MinerU] Empty page index list in crop; skipping this position.")
                continue
            valid_pns = [p for p in pns if 0 <= p < page_count]
            if not valid_pns:
                self.logger.warning(f"[MinerU] All page indices {pns} out of range for {page_count} pages; skipping.")
                continue
            filtered_poss.append((valid_pns, left, right, top, bottom))

        poss = filtered_poss
        if not poss:
            self.logger.warning("[MinerU] No valid positions after filtering; skip cropping.")
            if need_position:
                return None, None
            return

        max_width = max(np.max([right - left for (_, left, right, _, _) in poss]), 6)
        GAP = 6
        pos = poss[0]
        first_page_idx = pos[0][0]
        poss.insert(0, ([first_page_idx], pos[1], pos[2], max(0, pos[3] - 120), max(pos[3] - GAP, 0)))
        pos = poss[-1]
        last_page_idx = pos[0][-1]
        if not (0 <= last_page_idx < page_count):
            self.logger.warning(f"[MinerU] Last page index {last_page_idx} out of range for {page_count} pages; skipping crop.")
            if need_position:
                return None, None
            return
        last_page_height = self.page_images[last_page_idx].size[1]
        poss.append(
            (
                [last_page_idx],
                pos[1],
                pos[2],
                min(last_page_height, pos[4] + GAP),
                min(last_page_height, pos[4] + 120),
            )
        )

        positions = []
        for ii, (pns, left, right, top, bottom) in enumerate(poss):
            right = left + max_width

            if bottom <= top:
                bottom = top + 2

            for pn in pns[1:]:
                if 0 <= pn - 1 < page_count:
                    bottom += self.page_images[pn - 1].size[1]
                else:
                    self.logger.warning(f"[MinerU] Page index {pn}-1 out of range for {page_count} pages during crop; skipping height accumulation.")

            if not (0 <= pns[0] < page_count):
                self.logger.warning(f"[MinerU] Base page index {pns[0]} out of range for {page_count} pages during crop; skipping this segment.")
                continue

            img0 = self.page_images[pns[0]]
            x0, y0, x1, y1 = int(left), int(top), int(right), int(min(bottom, img0.size[1]))
            crop0 = img0.crop((x0, y0, x1, y1))
            imgs.append(crop0)
            if 0 < ii < len(poss) - 1:
                positions.append((pns[0] + self.page_from, x0, x1, y0, y1))

            bottom -= img0.size[1]
            for pn in pns[1:]:
                if not (0 <= pn < page_count):
                    self.logger.warning(f"[MinerU] Page index {pn} out of range for {page_count} pages during crop; skipping this page.")
                    continue
                page = self.page_images[pn]
                x0, y0, x1, y1 = int(left), 0, int(right), int(min(bottom, page.size[1]))
                cimgp = page.crop((x0, y0, x1, y1))
                imgs.append(cimgp)
                if 0 < ii < len(poss) - 1:
                    positions.append((pn + self.page_from, x0, x1, y0, y1))
                bottom -= page.size[1]

        if not imgs:
            if need_position:
                return None, None
            return

        height = 0
        for img in imgs:
            height += img.size[1] + GAP
        height = int(height)
        width = int(np.max([i.size[0] for i in imgs]))
        pic = Image.new("RGB", (width, height), (245, 245, 245))
        height = 0
        for ii, img in enumerate(imgs):
            if ii == 0 or ii + 1 == len(imgs):
                img = img.convert("RGBA")
                overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
                overlay.putalpha(128)
                img = Image.alpha_composite(img, overlay).convert("RGB")
            pic.paste(img, (0, int(height)))
            height += img.size[1] + GAP

        if need_position:
            return pic, positions
        return pic

    @staticmethod
    def extract_positions(txt: str):
        poss = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", txt):
            pn, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
            left, right, top, bottom = float(left), float(right), float(top), float(bottom)
            poss.append(([int(p) - 1 for p in pn.split("-")], left, right, top, bottom))
        return poss

    def _bbox_to_pixels(self, bbox, page_size):
        x0, y0, x1, y1 = bbox
        pw, ph = page_size
        maxv = max(bbox)
        # 经验：MinerU bbox 常为 0~1000 归一化；否则认为已是像素
        if maxv <= 1.5:
            sx, sy = pw, ph
        elif maxv <= 1200:
            sx, sy = pw / 1000.0, ph / 1000.0
        else:
            sx, sy = 1.0, 1.0
        return (
            int(x0 * sx),
            int(y0 * sy),
            int(x1 * sx),
            int(y1 * sy),
        )

    def _generate_missing_images(self, outputs: list[dict[str, Any]], subdir: Path, file_stem: str):
        if not getattr(self, "page_images", None):
            return
        if not subdir:
            return
        img_root = subdir / "generated_images"
        img_root.mkdir(parents=True, exist_ok=True)
        text_types = {
            "text", "list", "code", "header", "equation",
            MinerUContentType.TEXT, MinerUContentType.LIST, MinerUContentType.CODE, 
            MinerUContentType.HEADER, MinerUContentType.EQUATION
        }
        generated = 0
        for idx, item in enumerate(outputs):
            if item.get("type") not in text_types:
                continue
            if item.get("img_path"):
                continue
            
            bbox = item.get("bbox")
            if not bbox or len(bbox) != 4:
                continue
            
            page_idx = int(item.get("page_idx", 0))
            if page_idx < 0 or page_idx >= len(self.page_images):
                continue
                
            x0, y0, x1, y1 = self._bbox_to_pixels(bbox, self.page_images[page_idx].size)
            
            # guard invalid bbox
            if x1 - x0 < 2 or y1 - y0 < 2:
                continue
                
            try:
                crop = self.page_images[page_idx].crop((x0, y0, x1, y1))
                fname = f"{file_stem}_gen_{idx}.jpg"
                out_path = img_root / fname
                crop.save(out_path, format="JPEG", quality=80)
                item["img_path"] = str(out_path.resolve())
                generated += 1
            except Exception as e:
                self.logger.debug(f"[MinerU] skip image gen idx={idx} page={page_idx}: {e}")
                continue
                
        if generated:
            self.logger.info(f"[MinerU] generated {generated} fallback images for text blocks")

    def _read_output(self, output_dir: Path, file_stem: str, method: str = "auto", backend: str = "pipeline") -> list[dict[str, Any]]:
        candidates = []
        seen = set()

        def add_candidate_path(p: Path):
            if p not in seen:
                seen.add(p)
                candidates.append(p)

        if backend.startswith("vlm-"):
            add_candidate_path(output_dir / file_stem / "vlm")
            if method:
                add_candidate_path(output_dir / file_stem / method)
            add_candidate_path(output_dir / file_stem / "auto")
        else:
            if method:
                add_candidate_path(output_dir / file_stem / method)
            add_candidate_path(output_dir / file_stem / "vlm")
            add_candidate_path(output_dir / file_stem / "auto")

        json_file = None
        subdir = None
        attempted = []

        # mirror MinerU's sanitize_filename to align ZIP naming
        def _sanitize_filename(name: str) -> str:
            sanitized = re.sub(r"[/\\\.]{2,}|[/\\]", "", name)
            sanitized = re.sub(r"[^\w.-]", "_", sanitized, flags=re.UNICODE)
            if sanitized.startswith("."):
                sanitized = "_" + sanitized[1:]
            return sanitized or "unnamed"

        safe_stem = _sanitize_filename(file_stem)
        allowed_names = {f"{file_stem}_content_list.json", f"{safe_stem}_content_list.json"}
        self.logger.info(f"[MinerU] Expected output files: {', '.join(sorted(allowed_names))}")
        self.logger.info(f"[MinerU] Searching output candidates: {', '.join(str(c) for c in candidates)}")

        for sub in candidates:
            jf = sub / f"{file_stem}_content_list.json"
            self.logger.info(f"[MinerU] Trying original path: {jf}")
            attempted.append(jf)
            if jf.exists():
                subdir = sub
                json_file = jf
                break

            # MinerU API sanitizes non-ASCII filenames inside the ZIP root and file names.
            alt = sub / f"{safe_stem}_content_list.json"
            self.logger.info(f"[MinerU] Trying sanitized filename: {alt}")
            attempted.append(alt)
            if alt.exists():
                subdir = sub
                json_file = alt
                break

            nested_alt = sub / safe_stem / f"{safe_stem}_content_list.json"
            self.logger.info(f"[MinerU] Trying sanitized nested path: {nested_alt}")
            attempted.append(nested_alt)
            if nested_alt.exists():
                subdir = nested_alt.parent
                json_file = nested_alt
                break

        if not json_file:
            raise FileNotFoundError(f"[MinerU] Missing output file, tried: {', '.join(str(p) for p in attempted)}")

        with open(json_file, "r", encoding="utf-8") as f:
            data = json.load(f)

        for item in data:
            for key in ("img_path", "table_img_path", "equation_img_path"):
                if key in item and item[key]:
                    item[key] = str((subdir / item[key]).resolve())

        # MinerU(vlm-http-client) 不会为纯文本生成图片，这里兜底用本地页图裁剪生成，方便后续引用/MinIO 存图
        try:
            self._generate_missing_images(data, subdir, file_stem)
        except Exception as e:
            self.logger.warning(f"[MinerU] generate missing images failed: {e}")

        return data

    def _transfer_to_sections(self, outputs: list[dict[str, Any]], parse_method: str = None):
        sections = []
        for output in outputs:
            match output["type"]:
                case MinerUContentType.TEXT:
                    section = output["text"]
                case MinerUContentType.TABLE:
                    section = output.get("table_body", "") + "\n".join(output.get("table_caption", [])) + "\n".join(output.get("table_footnote", []))
                    if not section.strip():
                        section = "FAILED TO PARSE TABLE"
                case MinerUContentType.IMAGE:
                    section = "".join(output.get("image_caption", [])) + "\n" + "".join(output.get("image_footnote", []))
                case MinerUContentType.EQUATION:
                    section = output["text"]
                case MinerUContentType.CODE:
                    section = output["code_body"] + "\n".join(output.get("code_caption", []))
                case MinerUContentType.LIST:
                    section = "\n".join(output.get("list_items", []))
                case MinerUContentType.DISCARDED:
                    pass

            if section and parse_method == "manual":
                sections.append((section, output["type"], self._line_tag(output)))
            elif section and parse_method == "paper":
                sections.append((section + self._line_tag(output), output["type"]))
            else:
                sections.append((section, self._line_tag(output)))
        return sections

    def _transfer_to_tables(self, outputs: list[dict[str, Any]]):
        return []

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None,
        backend: str = "pipeline",
        lang: Optional[str] = None,
        method: str = "auto",
        server_url: Optional[str] = None,
        delete_output: bool = True,
        parse_method: str = "raw",
    ) -> tuple:
        import shutil

        temp_pdf = None
        created_tmp_dir = False

        # remove spaces, or mineru crash, and _read_output fail too
        file_path = Path(filepath)
        pdf_file_name = file_path.stem.replace(" ", "") + ".pdf"
        pdf_file_path_valid = os.path.join(file_path.parent, pdf_file_name)

        if binary:
            temp_dir = Path(tempfile.mkdtemp(prefix="mineru_bin_pdf_"))
            temp_pdf = temp_dir / pdf_file_name
            with open(temp_pdf, "wb") as f:
                f.write(binary)
            pdf = temp_pdf
            self.logger.info(f"[MinerU] Received binary PDF -> {temp_pdf}")
            if callback:
                callback(0.15, f"[MinerU] Received binary PDF -> {temp_pdf}")
        else:
            if pdf_file_path_valid != filepath:
                self.logger.info(f"[MinerU] Remove all space in file name: {pdf_file_path_valid}")
                shutil.move(filepath, pdf_file_path_valid)
            pdf = Path(pdf_file_path_valid)
            if not pdf.exists():
                if callback:
                    callback(-1, f"[MinerU] PDF not found: {pdf}")
                raise FileNotFoundError(f"[MinerU] PDF not found: {pdf}")

        if output_dir:
            out_dir = Path(output_dir)
            out_dir.mkdir(parents=True, exist_ok=True)
        else:
            out_dir = Path(tempfile.mkdtemp(prefix="mineru_pdf_"))
            created_tmp_dir = True

        self.logger.info(f"[MinerU] Output directory: {out_dir}")
        if callback:
            callback(0.15, f"[MinerU] Output directory: {out_dir}")

        self.__images__(pdf, zoomin=1)

        try:
            self._run_mineru(pdf, out_dir, method=method, backend=backend, lang=lang, server_url=server_url, callback=callback)
            outputs = self._read_output(out_dir, pdf.stem, method=method, backend=backend)
            self.logger.info(f"[MinerU] Parsed {len(outputs)} blocks from PDF.")
            if callback:
                callback(0.75, f"[MinerU] Parsed {len(outputs)} blocks from PDF.")

            return self._transfer_to_sections(outputs, parse_method), self._transfer_to_tables(outputs)
        finally:
            if temp_pdf and temp_pdf.exists():
                try:
                    temp_pdf.unlink()
                    temp_pdf.parent.rmdir()
                except Exception:
                    pass
            if delete_output and created_tmp_dir and out_dir.exists():
                try:
                    shutil.rmtree(out_dir)
                except Exception:
                    pass


if __name__ == "__main__":
    parser = MinerUParser("mineru")
    ok, reason = parser.check_installation()
    print("MinerU available:", ok)

    filepath = ""
    with open(filepath, "rb") as file:
        outputs = parser.parse_pdf(filepath=filepath, binary=file.read())
        for output in outputs:
            print(output)
