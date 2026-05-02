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
import re
import shutil
import sys
import tempfile
import threading
import time
import zipfile
from dataclasses import dataclass
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Any, Callable, Optional

import numpy as np
import pdfplumber
import requests
from PIL import Image
from strenum import StrEnum

from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from deepdoc.parser.utils import extract_pdf_outlines

from common.constants import MAXIMUM_PAGE_NUMBER

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


OFFICIAL_V4_MAX_FILE_BYTES = 200 * 1024 * 1024
OFFICIAL_V4_MAX_PAGES = 200


class MinerUContentType(StrEnum):
    IMAGE = "image"
    TABLE = "table"
    TEXT = "text"
    EQUATION = "equation"
    CODE = "code"
    LIST = "list"
    DISCARDED = "discarded"


# Mapping from language names to MinerU language codes
LANGUAGE_TO_MINERU_MAP = {
    'English': 'en',
    'Chinese': 'ch',
    'Traditional Chinese': 'chinese_cht',
    'Russian': 'east_slavic',
    'Ukrainian': 'east_slavic',
    'Indonesian': 'latin',
    'Spanish': 'latin',
    'Vietnamese': 'latin',
    'Japanese': 'japan',
    'Korean': 'korean',
    'Portuguese BR': 'latin',
    'German': 'latin',
    'French': 'latin',
    'Italian': 'latin',
    'Tamil': 'ta',
    'Telugu': 'te',
    'Kannada': 'ka',
    'Thai': 'th',
    'Greek': 'el',
    'Hindi': 'devanagari',
    'Bulgarian': 'cyrillic',
    'Turkish': 'latin',
}


class MinerUBackend(StrEnum):
    """MinerU processing backend options."""

    PIPELINE = "pipeline"  # Traditional multimodel pipeline (default)
    VLM_TRANSFORMERS = "vlm-transformers"  # Vision-language model using HuggingFace Transformers
    VLM_MLX_ENGINE = "vlm-mlx-engine"  # Faster, requires Apple Silicon and macOS 13.5+
    VLM_VLLM_ENGINE = "vlm-vllm-engine"  # Local vLLM engine, requires local GPU
    VLM_VLLM_ASYNC_ENGINE = "vlm-vllm-async-engine"  # Asynchronous vLLM engine, new in MinerU API
    VLM_LMDEPLOY_ENGINE = "vlm-lmdeploy-engine"  # LMDeploy engine
    VLM_HTTP_CLIENT = "vlm-http-client"  # HTTP client for remote vLLM server (CPU only)


class MinerULanguage(StrEnum):
    """MinerU supported languages for OCR (pipeline backend only)."""

    CH = "ch"  # Chinese
    CH_SERVER = "ch_server"  # Chinese (server)
    CH_LITE = "ch_lite"  # Chinese (lite)
    EN = "en"  # English
    KOREAN = "korean"  # Korean
    JAPAN = "japan"  # Japanese
    CHINESE_CHT = "chinese_cht"  # Chinese Traditional
    TA = "ta"  # Tamil
    TE = "te"  # Telugu
    KA = "ka"  # Kannada
    TH = "th"  # Thai
    EL = "el"  # Greek
    LATIN = "latin"  # Latin
    ARABIC = "arabic"  # Arabic
    EAST_SLAVIC = "east_slavic"  # East Slavic
    CYRILLIC = "cyrillic"  # Cyrillic
    DEVANAGARI = "devanagari"  # Devanagari


class MinerUParseMethod(StrEnum):
    """MinerU PDF parsing methods (pipeline backend only)."""

    AUTO = "auto"  # Automatically determine the method based on the file type
    TXT = "txt"  # Use text extraction method
    OCR = "ocr"  # Use OCR method for image-based PDFs


class MinerUAccessMode(StrEnum):
    """MinerU service access mode."""

    SELF_HOSTED = "self_hosted"
    OFFICIAL_V4 = "official_v4"


@dataclass
class MinerUParseOptions:
    """Options for MinerU PDF parsing."""

    access_mode: MinerUAccessMode = MinerUAccessMode.SELF_HOSTED
    backend: MinerUBackend = MinerUBackend.PIPELINE
    lang: Optional[MinerULanguage] = None  # language for OCR (pipeline backend only)
    method: MinerUParseMethod = MinerUParseMethod.AUTO
    server_url: Optional[str] = None
    delete_output: bool = True
    parse_method: str = "raw"
    formula_enable: bool = True
    table_enable: bool = True
    api_base_url: str = "https://mineru.net"
    api_token: str = ""
    model_version: str = "vlm"
    poll_interval: int = 3
    poll_timeout: int = 300


class MinerUParser(RAGFlowPdfParser):
    def __init__(
        self,
        mineru_path: str = "mineru",
        mineru_api: str = "",
        mineru_server_url: str = "",
        access_mode: str = "self_hosted",
        api_base_url: str = "https://mineru.net",
        api_token: str = "",
        model_version: str = "vlm",
        poll_interval: int = 3,
        poll_timeout: int = 300,
    ):
        self.mineru_api = mineru_api.rstrip("/")
        self.mineru_server_url = mineru_server_url.rstrip("/")
        self.mineru_access_mode = access_mode or MinerUAccessMode.SELF_HOSTED
        self.mineru_api_base_url = (api_base_url or "https://mineru.net").rstrip("/")
        self.mineru_api_token = (api_token or "").strip()
        self.mineru_model_version = model_version or "vlm"
        self.mineru_poll_interval = poll_interval
        self.mineru_poll_timeout = poll_timeout
        self.outlines = []
        self.logger = logging.getLogger(self.__class__.__name__)

    @staticmethod
    def _is_zipinfo_symlink(member: zipfile.ZipInfo) -> bool:
        return (member.external_attr >> 16) & 0o170000 == 0o120000

    def _extract_zip_no_root(self, zip_path, extract_to, root_dir):
        self.logger.info(f"[MinerU] Extract zip: zip_path={zip_path}, extract_to={extract_to}, root_hint={root_dir}")
        base_dir = Path(extract_to).resolve()
        with zipfile.ZipFile(zip_path, "r") as zip_ref:
            members = zip_ref.infolist()
            if not root_dir:
                if members and members[0].filename.endswith("/"):
                    root_dir = members[0].filename
                else:
                    root_dir = None
            if root_dir:
                root_dir = root_dir.replace("\\", "/")
                if not root_dir.endswith("/"):
                    root_dir += "/"

            for member in members:
                if member.flag_bits & 0x1:
                    raise RuntimeError(f"[MinerU] Encrypted zip entry not supported: {member.filename}")
                if self._is_zipinfo_symlink(member):
                    raise RuntimeError(f"[MinerU] Symlink zip entry not supported: {member.filename}")

                name = member.filename.replace("\\", "/")
                if root_dir and name == root_dir:
                    self.logger.info("[MinerU] Ignore root folder...")
                    continue
                if root_dir and name.startswith(root_dir):
                    name = name[len(root_dir) :]
                if not name:
                    continue
                if name.startswith("/") or name.startswith("//") or re.match(r"^[A-Za-z]:", name):
                    raise RuntimeError(f"[MinerU] Unsafe zip path (absolute): {member.filename}")

                parts = [p for p in name.split("/") if p not in ("", ".")]
                if any(p == ".." for p in parts):
                    raise RuntimeError(f"[MinerU] Unsafe zip path (traversal): {member.filename}")

                rel_path = os.path.join(*parts) if parts else ""
                dest_path = (Path(extract_to) / rel_path).resolve(strict=False)
                if dest_path != base_dir and base_dir not in dest_path.parents:
                    raise RuntimeError(f"[MinerU] Unsafe zip path (escape): {member.filename}")

                if member.is_dir():
                    os.makedirs(dest_path, exist_ok=True)
                    continue

                os.makedirs(dest_path.parent, exist_ok=True)
                with zip_ref.open(member) as src, open(dest_path, "wb") as dst:
                    shutil.copyfileobj(src, dst)

    @staticmethod
    def _is_http_endpoint_valid(url, timeout=5):
        try:
            response = requests.head(url, timeout=timeout, allow_redirects=True)
            return response.status_code in [200, 301, 302, 307, 308]
        except Exception:
            return False

    @staticmethod
    def _is_http_endpoint_reachable(url, timeout=10, headers=None):
        try:
            response = requests.get(url, timeout=timeout, allow_redirects=True, headers=headers)
            return response.ok, response.status_code
        except Exception:
            return False, None

    def check_installation(
        self,
        backend: str = "pipeline",
        server_url: Optional[str] = None,
        access_mode: Optional[str] = None,
        api_base_url: Optional[str] = None,
        api_token: Optional[str] = None,
    ) -> tuple[bool, str]:
        reason = ""

        resolved_mode = access_mode or self.mineru_access_mode
        try:
            mode = MinerUAccessMode(resolved_mode)
        except ValueError:
            reason = f"[MinerU] Invalid access mode '{resolved_mode}'."
            self.logger.warning(reason)
            return False, reason

        if mode == MinerUAccessMode.OFFICIAL_V4:
            resolved_base_url = (api_base_url or self.mineru_api_base_url or "").rstrip("/")
            resolved_token = (api_token or self.mineru_api_token or "").strip()
            if not resolved_base_url:
                reason = "[MinerU] MINERU_API_BASE_URL not configured for official_v4."
                self.logger.warning(reason)
                return False, reason
            if not resolved_token:
                reason = "[MinerU] MINERU_API_TOKEN not configured for official_v4."
                self.logger.warning(reason)
                return False, reason

            probe_url = f"{resolved_base_url}/api/v4/extract/task"
            headers = {
                "Authorization": f"Bearer {resolved_token}",
                "Content-Type": "application/json",
                "Accept": "application/json",
            }
            probe_payload = {
                "url": "https://cdn-mineru.openxlab.org.cn/demo/example.pdf",
                "model_version": self.mineru_model_version or "vlm",
            }
            try:
                response = requests.post(probe_url, headers=headers, json=probe_payload, timeout=30)
            except requests.RequestException as exc:
                reason = f"[MinerU] Official v4 endpoint not reachable: {probe_url}"
                self.logger.warning(f"{reason}. exception={exc}")
                return False, reason

            status_code = response.status_code
            if status_code in (401, 403):
                reason = "[MinerU] Official v4 token invalid or unauthorized."
                self.logger.warning(f"{reason} status={status_code} url={probe_url}")
                return False, reason
            if status_code == 404:
                reason = f"[MinerU] Official v4 endpoint not found: {probe_url}"
                self.logger.warning(reason)
                return False, reason
            if status_code >= 500:
                reason = f"[MinerU] Official v4 endpoint not reachable: {probe_url}"
                self.logger.warning(f"{reason} status={status_code}")
                return False, reason
            if status_code >= 400:
                reason = f"[MinerU] Official v4 test request rejected: HTTP {status_code}"
                self.logger.warning(f"{reason} url={probe_url} body={response.text[:256]}")
                return False, reason

            probe_body = {}
            if response.content:
                try:
                    probe_body = response.json()
                except ValueError:
                    reason = "[MinerU] Official v4 test request returned invalid JSON response."
                    self.logger.warning(f"{reason} status={status_code} url={probe_url}")
                    return False, reason

            probe_code = probe_body.get("code")
            if probe_code != 0:
                reason = (
                    "[MinerU] Official v4 test request rejected: missing application code"
                    if probe_code is None
                    else f"[MinerU] Official v4 test request rejected: code={probe_code}"
                )
                self.logger.warning(
                    f"{reason} msg={probe_body.get('msg')} trace_id={probe_body.get('trace_id')} url={probe_url}"
                )
                return False, reason

            self.logger.info(
                f"[MinerU] official_v4 test request accepted status={status_code} code={probe_code} url={probe_url}"
            )
            return True, ""

        valid_backends = ["pipeline", "vlm-http-client", "vlm-transformers", "vlm-vllm-engine", "vlm-mlx-engine", "vlm-vllm-async-engine", "vlm-lmdeploy-engine"]
        if backend not in valid_backends:
            reason = f"[MinerU] Invalid backend '{backend}'. Valid backends are: {valid_backends}"
            self.logger.warning(reason)
            return False, reason

        if not self.mineru_api:
            reason = "[MinerU] MINERU_APISERVER not configured."
            self.logger.warning(reason)
            return False, reason

        api_openapi = f"{self.mineru_api}/openapi.json"
        try:
            api_ok = self._is_http_endpoint_valid(api_openapi)
            self.logger.info(f"[MinerU] API openapi.json reachable={api_ok} url={api_openapi}")
            if not api_ok:
                reason = f"[MinerU] MinerU API not accessible: {api_openapi}"
                return False, reason
        except Exception as exc:
            reason = f"[MinerU] MinerU API check failed: {exc}"
            self.logger.warning(reason)
            return False, reason

        if backend == "vlm-http-client":
            resolved_server = server_url or self.mineru_server_url
            if not resolved_server:
                reason = "[MinerU] MINERU_SERVER_URL required for vlm-http-client backend."
                self.logger.warning(reason)
                return False, reason
            try:
                server_ok = self._is_http_endpoint_valid(resolved_server)
                self.logger.info(f"[MinerU] vlm-http-client server check reachable={server_ok} url={resolved_server}")
            except Exception as exc:
                self.logger.warning(f"[MinerU] vlm-http-client server probe failed: {resolved_server}: {exc}")

        return True, reason

    def _run_mineru(
        self, input_path: Path, output_dir: Path, options: MinerUParseOptions, callback: Optional[Callable] = None
    ) -> Path:
        if options.access_mode == MinerUAccessMode.OFFICIAL_V4:
            return self._run_mineru_official_v4(input_path, output_dir, options, callback)
        return self._run_mineru_api(input_path, output_dir, options, callback)

    def _run_mineru_api(
        self, input_path: Path, output_dir: Path, options: MinerUParseOptions, callback: Optional[Callable] = None
    ) -> Path:
        pdf_file_path = str(input_path)

        if not os.path.exists(pdf_file_path):
            raise RuntimeError(f"[MinerU] PDF file not exists: {pdf_file_path}")

        pdf_file_name = Path(pdf_file_path).stem.strip()
        output_path = tempfile.mkdtemp(prefix=f"{pdf_file_name}_{options.method}_", dir=str(output_dir))
        output_zip_path = os.path.join(str(output_dir), f"{Path(output_path).name}.zip")

        data = {
            "output_dir": "./output",
            "lang_list": options.lang,
            "backend": options.backend,
            "parse_method": options.method,
            "formula_enable": options.formula_enable,
            "table_enable": options.table_enable,
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

        if options.server_url:
            data["server_url"] = options.server_url
        elif self.mineru_server_url:
            data["server_url"] = self.mineru_server_url

        self.logger.info(f"[MinerU] request {data=}")
        self.logger.info(f"[MinerU] request {options=}")

        headers = {"Accept": "application/json"}
        try:
            self.logger.info(f"[MinerU] invoke api: {self.mineru_api}/file_parse backend={options.backend} server_url={data.get('server_url')}")
            if callback:
                callback(0.20, f"[MinerU] invoke api: {self.mineru_api}/file_parse")
            with open(pdf_file_path, "rb") as pdf_file:
                files = {"files": (pdf_file_name + ".pdf", pdf_file, "application/pdf")}
                with requests.post(
                    url=f"{self.mineru_api}/file_parse",
                    files=files,
                    data=data,
                    headers=headers,
                    timeout=1800,
                    stream=True,
                ) as response:
                    response.raise_for_status()
                    content_type = response.headers.get("Content-Type", "")
                    if not content_type.startswith("application/zip"):
                        raise RuntimeError(f"[MinerU] not zip returned from api: {content_type}")
                    self.logger.info(f"[MinerU] zip file returned, saving to {output_zip_path}...")
                    if callback:
                        callback(0.30, f"[MinerU] zip file returned, saving to {output_zip_path}...")
                    with open(output_zip_path, "wb") as f:
                        response.raw.decode_content = True
                        shutil.copyfileobj(response.raw, f)
                    self.logger.info(f"[MinerU] Unzip to {output_path}...")
                    self._extract_zip_no_root(output_zip_path, output_path, pdf_file_name + "/")
                    if callback:
                        callback(0.40, f"[MinerU] Unzip to {output_path}...")
            self.logger.info("[MinerU] Api completed successfully.")
            return Path(output_path)
        except requests.RequestException as e:
            raise RuntimeError(f"[MinerU] api failed with exception {e}")

    @staticmethod
    def _build_official_headers(api_token: str):
        return {
            "Authorization": f"Bearer {api_token}",
            "Content-Type": "application/json",
            "Accept": "application/json",
        }

    @staticmethod
    def _pick_extract_result(extract_results: list[dict], file_name: str) -> dict:
        if not extract_results:
            return {}
        if len(extract_results) == 1:
            return extract_results[0]
        normalized = Path(file_name).name
        for item in extract_results:
            result_name = Path(str(item.get("file_name", ""))).name
            if result_name == normalized:
                return item
        return extract_results[0]

    def _poll_official_batch_result(
        self,
        batch_id: str,
        upload_name: str,
        options: MinerUParseOptions,
        callback: Optional[Callable] = None,
    ) -> str:
        poll_url = f"{options.api_base_url.rstrip('/')}/api/v4/extract-results/batch/{batch_id}"
        headers = self._build_official_headers(options.api_token)
        start_time = time.time()
        deadline = time.time() + max(int(options.poll_timeout), 30)
        interval = max(int(options.poll_interval), 1)

        while True:
            response = requests.get(poll_url, headers=headers, timeout=30)
            response.raise_for_status()
            payload = response.json() if response.content else {}

            if payload.get("code", -1) != 0:
                raise RuntimeError(f"[MinerU] official_v4 poll failed: {payload.get('msg', 'unknown error')}")

            data = payload.get("data") or {}
            extract_results = data.get("extract_result") or []
            result = self._pick_extract_result(extract_results, upload_name)
            state = str(result.get("state", "")).lower()

            if state == "done":
                full_zip_url = result.get("full_zip_url", "")
                if not full_zip_url:
                    raise RuntimeError("[MinerU] official_v4 done but full_zip_url missing")
                return full_zip_url

            if state == "failed":
                err_msg = result.get("err_msg") or payload.get("msg") or "unknown error"
                raise RuntimeError(f"[MinerU] official_v4 parse failed: {err_msg}")

            if callback:
                if state == "running":
                    progress_info = result.get("extract_progress") or {}
                    extracted_pages = progress_info.get("extracted_pages") or 0
                    total_pages = progress_info.get("total_pages") or 0
                    ratio = min(max(extracted_pages / total_pages, 0.0), 1.0) if total_pages else 0.0
                    callback(0.55 + 0.25 * ratio, f"[MinerU] official_v4 running {extracted_pages}/{total_pages}")
                else:
                    callback(0.50, f"[MinerU] official_v4 state: {state or 'pending'}")
            else:
                if state == "running":
                    progress_info = result.get("extract_progress") or {}
                    extracted_pages = progress_info.get("extracted_pages") or 0
                    total_pages = progress_info.get("total_pages") or 0
                    ratio = min(max(extracted_pages / total_pages, 0.0), 1.0) if total_pages else 0.0
                    self.logger.info(
                        f"[MinerU] official_v4 running {extracted_pages}/{total_pages} "
                        f"ratio={ratio:.2f} batch_id={batch_id}"
                    )
                else:
                    self.logger.info(
                        f"[MinerU] official_v4 poll state={state or 'pending'} batch_id={batch_id}"
                    )

            if time.time() >= deadline:
                elapsed = int(time.time() - start_time)
                self.logger.warning(
                    f"[MinerU] official_v4 polling timeout after {options.poll_timeout}s "
                    f"(elapsed={elapsed}s) batch_id={batch_id}"
                )
                raise TimeoutError(f"[MinerU] official_v4 polling timeout after {options.poll_timeout}s")
            time.sleep(interval)

    def _run_mineru_official_v4(
        self,
        input_path: Path,
        output_dir: Path,
        options: MinerUParseOptions,
        callback: Optional[Callable] = None,
    ) -> Path:
        pdf_file_path = str(input_path)
        if not os.path.exists(pdf_file_path):
            raise RuntimeError(f"[MinerU] PDF file not exists: {pdf_file_path}")

        if not options.api_token:
            raise RuntimeError("[MinerU] official_v4 requires api_token")
        if not options.api_base_url:
            raise RuntimeError("[MinerU] official_v4 requires api_base_url")

        file_size = os.path.getsize(pdf_file_path)
        if file_size > OFFICIAL_V4_MAX_FILE_BYTES:
            err_msg = (
                "[MinerU] official_v4 only supports PDFs up to "
                f"{int(OFFICIAL_V4_MAX_FILE_BYTES / 1024 / 1024)} MB, got {file_size} bytes"
            )
            if callback:
                callback(-1, err_msg)
            raise RuntimeError(err_msg)

        page_count = len(self.page_images) if getattr(self, "page_images", None) else None
        if page_count is None:
            with pdfplumber.open(pdf_file_path) as pdf:
                page_count = len(pdf.pages)
        if page_count > OFFICIAL_V4_MAX_PAGES:
            err_msg = (
                "[MinerU] official_v4 only supports PDFs up to "
                f"{OFFICIAL_V4_MAX_PAGES} pages, got {page_count} pages"
            )
            if callback:
                callback(-1, err_msg)
            raise RuntimeError(err_msg)

        pdf_file_name = Path(pdf_file_path).stem.strip()
        upload_name = f"{pdf_file_name}.pdf"
        output_path = tempfile.mkdtemp(prefix=f"{pdf_file_name}_{options.model_version}_", dir=str(output_dir))
        output_zip_path = os.path.join(str(output_dir), f"{Path(output_path).name}.zip")

        create_url = f"{options.api_base_url.rstrip('/')}/api/v4/file-urls/batch"
        headers = self._build_official_headers(options.api_token)
        create_payload = {
            "files": [{"name": upload_name}],
            "model_version": options.model_version,
            "enable_formula": options.formula_enable,
            "enable_table": options.table_enable,
            "language": options.lang.value if options.lang else None,
        }
        if options.method == MinerUParseMethod.OCR:
            create_payload["files"][0]["is_ocr"] = True
        elif options.method == MinerUParseMethod.TXT:
            create_payload["files"][0]["is_ocr"] = False

        create_payload = {k: v for k, v in create_payload.items() if v is not None}
        self.logger.info(f"[MinerU] official_v4 create batch: {create_url}")
        if callback:
            callback(0.20, "[MinerU] official_v4 create upload task")

        response = requests.post(create_url, headers=headers, json=create_payload, timeout=30)
        response.raise_for_status()
        body = response.json() if response.content else {}
        if body.get("code", -1) != 0:
            raise RuntimeError(f"[MinerU] official_v4 create batch failed: {body.get('msg', 'unknown error')}")

        batch_data = body.get("data") or {}
        batch_id = batch_data.get("batch_id")
        file_urls = batch_data.get("file_urls") or []
        if not batch_id or not file_urls:
            raise RuntimeError("[MinerU] official_v4 create batch returned incomplete data")

        if callback:
            callback(0.30, "[MinerU] official_v4 uploading file")
        with open(pdf_file_path, "rb") as pdf_file:
            upload_resp = requests.put(file_urls[0], data=pdf_file, timeout=1800)
        if not upload_resp.ok:
            raise RuntimeError(f"[MinerU] official_v4 upload failed: HTTP {upload_resp.status_code}")

        if callback:
            callback(0.40, "[MinerU] official_v4 polling result")
        full_zip_url = self._poll_official_batch_result(batch_id, upload_name, options, callback)

        if callback:
            callback(0.75, "[MinerU] official_v4 downloading parse zip")
        with requests.get(full_zip_url, timeout=1800, stream=True) as zip_resp:
            zip_resp.raise_for_status()
            with open(output_zip_path, "wb") as f:
                zip_resp.raw.decode_content = True
                shutil.copyfileobj(zip_resp.raw, f)

        self.logger.info(f"[MinerU] official_v4 unzip to {output_path}...")
        self._extract_zip_no_root(output_zip_path, output_path, None)
        if callback:
            callback(0.80, f"[MinerU] official_v4 unzip to {output_path}")

        return Path(output_path)

    def __images__(self, fnm, zoomin: int = 1, page_from=0, page_to=MAXIMUM_PAGE_NUMBER, callback=None):
        self.page_from = page_from
        self.page_to = page_to
        try:
            with pdfplumber.open(fnm) if isinstance(fnm, (str, PathLike)) else pdfplumber.open(BytesIO(fnm)) as pdf:
                self.pdf = pdf
                self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).original for _, p in
                                    enumerate(self.pdf.pages[page_from:page_to])]
        except Exception as e:
            self.page_images = None
            self.total_page = 0
            self.logger.exception(e)

    def _line_tag(self, bx):
        pn = [bx["page_idx"] + 1]
        positions = bx.get("bbox", (0, 0, 0, 0))
        x0, top, x1, bott = positions
        # Normalize flipped coordinates (MinerU may report inverted bbox for flipped images)
        if x0 > x1:
            x0, x1 = x1, x0
        if top > bott:
            top, bott = bott, top

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
            self.logger.warning(
                f"[MinerU] Last page index {last_page_idx} out of range for {page_count} pages; skipping crop.")
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
                    self.logger.warning(
                        f"[MinerU] Page index {pn}-1 out of range for {page_count} pages during crop; skipping height accumulation.")

            if not (0 <= pns[0] < page_count):
                self.logger.warning(
                    f"[MinerU] Base page index {pns[0]} out of range for {page_count} pages during crop; skipping this segment.")
                continue

            img0 = self.page_images[pns[0]]
            x0, y0, x1, y1 = int(left), int(top), int(right), int(min(bottom, img0.size[1]))
            if x0 > x1:
                x0, x1 = x1, x0
            if y0 > y1:
                y0, y1 = y1, y0
            if x1 <= x0 or y1 <= y0:
                continue
            crop0 = img0.crop((x0, y0, x1, y1))
            imgs.append(crop0)
            if 0 < ii < len(poss) - 1:
                positions.append((pns[0] + self.page_from, x0, x1, y0, y1))

            bottom -= img0.size[1]
            for pn in pns[1:]:
                if not (0 <= pn < page_count):
                    self.logger.warning(
                        f"[MinerU] Page index {pn} out of range for {page_count} pages during crop; skipping this page.")
                    continue
                page = self.page_images[pn]
                x0, y0, x1, y1 = int(left), 0, int(right), int(min(bottom, page.size[1]))
                if x0 > x1:
                    x0, x1 = x1, x0
                if y0 > y1:
                    y0, y1 = y1, y0
                if x1 <= x0 or y1 <= y0:
                    bottom -= page.size[1]
                    continue
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

    def _read_output(self, output_dir: Path, file_stem: str, method: str = "auto", backend: str = "pipeline") -> list[
        dict[str, Any]]:
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
        content_names = (f"{file_stem}_content_list.json", f"{safe_stem}_content_list.json")
        allowed_names = set(content_names)
        self.logger.info(f"[MinerU] Expected output files: {', '.join(sorted(allowed_names))}")
        self.logger.info(f"[MinerU] Searching output in: {output_dir}")

        jf = output_dir / f"{file_stem}_content_list.json"
        self.logger.info(f"[MinerU] Trying original path: {jf}")
        attempted.append(jf)
        if jf.exists():
            subdir = output_dir
            json_file = jf
        else:
            alt = output_dir / f"{safe_stem}_content_list.json"
            self.logger.info(f"[MinerU] Trying sanitized filename: {alt}")
            attempted.append(alt)
            if alt.exists():
                subdir = output_dir
                json_file = alt
            else:
                nested_alt = output_dir / safe_stem / f"{safe_stem}_content_list.json"
                self.logger.info(f"[MinerU] Trying sanitized nested path: {nested_alt}")
                attempted.append(nested_alt)
                if nested_alt.exists():
                    subdir = nested_alt.parent
                    json_file = nested_alt

        if not json_file:
            parse_subdir = None
            if backend.startswith("pipeline"):
                parse_subdir = method
            elif backend.startswith("hybrid"):
                parse_subdir = f"hybrid_{method}"
            elif backend.startswith("vlm"):
                parse_subdir = "vlm"

            if parse_subdir:
                for content_name in content_names:
                    for candidate in output_dir.glob(f"**/{parse_subdir}/{content_name}"):
                        self.logger.info(f"[MinerU] Trying parse-method path: {candidate}")
                        attempted.append(candidate)
                        if candidate.exists():
                            subdir = candidate.parent
                            json_file = candidate
                            break
                    if json_file:
                        break

        if not json_file:
            direct_candidates = []
            for glob_pattern in ("*_content_list.json", "content_list.json"):
                for candidate in sorted(output_dir.glob(glob_pattern), key=lambda x: len(str(x))):
                    self.logger.info(f"[MinerU] Trying generic content_list path: {candidate}")
                    attempted.append(candidate)
                    if candidate.exists() and candidate.parent == output_dir:
                        direct_candidates.append(candidate)

            # Avoid binding to another job's output by only accepting a single direct candidate.
            if len(direct_candidates) == 1:
                json_file = direct_candidates[0]
                subdir = json_file.parent
            elif len(direct_candidates) > 1:
                raise FileNotFoundError(
                    f"[MinerU] Ambiguous output file candidates for '{file_stem}': "
                    + ", ".join(str(p) for p in direct_candidates[:10])
                )

        if not json_file:
            raise FileNotFoundError(f"[MinerU] Missing output file, tried: {', '.join(str(p) for p in attempted)}")

        with open(json_file, "r", encoding="utf-8") as f:
            data = json.load(f)

        for item in data:
            for key in ("img_path", "table_img_path", "equation_img_path"):
                if key in item and item[key]:
                    item[key] = str((subdir / item[key]).resolve())
        return data

    def _transfer_to_sections(self, outputs: list[dict[str, Any]], parse_method: str = None):
        sections = []
        for output in outputs:
            match output["type"]:
                case MinerUContentType.TEXT:
                    section = output.get("text", "")
                case MinerUContentType.TABLE:
                    section = output.get("table_body", "") + "\n".join(output.get("table_caption", [])) + "\n".join(
                        output.get("table_footnote", []))
                    if not section.strip():
                        section = "FAILED TO PARSE TABLE"
                case MinerUContentType.IMAGE:
                    section = "".join(output.get("image_caption", [])) + "\n" + "".join(
                        output.get("image_footnote", []))
                case MinerUContentType.EQUATION:
                    section = output.get("text", "")
                case MinerUContentType.CODE:
                    section = output.get("code_body", "") + "\n".join(output.get("code_caption", []))
                case MinerUContentType.LIST:
                    section = "\n".join(output.get("list_items", []))
                case MinerUContentType.DISCARDED:
                    continue  # Skip discarded blocks entirely

            if section and parse_method in {"manual", "pipeline"}:
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
            server_url: Optional[str] = None,
            delete_output: bool = True,
            parse_method: str = "raw",
            **kwargs,
    ) -> tuple:
        import shutil

        self.outlines = extract_pdf_outlines(binary if binary is not None else filepath)
        temp_pdf = None
        created_tmp_dir = False

        parser_cfg = kwargs.get('parser_config', {})
        access_mode_raw = kwargs.get('mineru_access_mode', self.mineru_access_mode or MinerUAccessMode.SELF_HOSTED)
        try:
            access_mode = MinerUAccessMode(access_mode_raw)
        except ValueError:
            self.logger.warning(
                f"[MinerU] Invalid mineru_access_mode='{access_mode_raw}', fallback to self_hosted."
            )
            access_mode = MinerUAccessMode.SELF_HOSTED

        def _safe_int(value, default: int) -> int:
            try:
                return int(value)
            except (TypeError, ValueError):
                return default

        lang = parser_cfg.get('mineru_lang') or kwargs.get('lang', 'English')
        mineru_lang_code = LANGUAGE_TO_MINERU_MAP.get(lang, 'ch')  # Defaults to Chinese if not matched
        mineru_method_raw_str = parser_cfg.get('mineru_parse_method', 'auto')
        enable_formula = parser_cfg.get('mineru_formula_enable', True)
        enable_table = parser_cfg.get('mineru_table_enable', True)
        model_version = kwargs.get('mineru_model_version', self.mineru_model_version or 'vlm')
        api_base_url = kwargs.get('mineru_api_base_url', self.mineru_api_base_url or 'https://mineru.net')
        api_token = kwargs.get('mineru_api_token', self.mineru_api_token or '')
        poll_interval = _safe_int(kwargs.get('mineru_poll_interval', self.mineru_poll_interval), 3)
        poll_timeout = _safe_int(kwargs.get('mineru_poll_timeout', self.mineru_poll_timeout), 300)

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

        self.logger.info(f"[MinerU] Output directory: {out_dir} backend={backend} api={self.mineru_api} server_url={server_url or self.mineru_server_url}")
        if callback:
            callback(0.15, f"[MinerU] Output directory: {out_dir}")

        self.__images__(pdf, zoomin=1)

        try:
            options = MinerUParseOptions(
                access_mode=access_mode,
                backend=MinerUBackend(backend),
                lang=MinerULanguage(mineru_lang_code),
                method=MinerUParseMethod(mineru_method_raw_str),
                server_url=server_url,
                delete_output=delete_output,
                parse_method=parse_method,
                formula_enable=enable_formula,
                table_enable=enable_table,
                api_base_url=api_base_url,
                api_token=api_token,
                model_version=model_version,
                poll_interval=poll_interval,
                poll_timeout=poll_timeout,
            )
            final_out_dir = self._run_mineru(pdf, out_dir, options, callback=callback)
            outputs = self._read_output(final_out_dir, pdf.stem, method=mineru_method_raw_str, backend=backend)
            self.logger.info(f"[MinerU] Parsed {len(outputs)} blocks from PDF.")
            if callback:
                callback(0.85, f"[MinerU] Parsed {len(outputs)} blocks from PDF.")

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
