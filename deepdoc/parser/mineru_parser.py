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
import requests.exceptions
from PIL import Image
from strenum import StrEnum

from deepdoc.parser.pdf_parser import RAGFlowPdfParser

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
# Use a module-level registry key to share a single Lock object without mutating
# sys.path or other global import state. Storing a Lock under a well-known key in
# sys.modules is acceptable, but avoid inserting unrelated paths into sys.path.
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


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
}


class MinerUBackend(StrEnum):
    """MinerU processing backend options."""

    HYBRID_AUTO_ENGINE = "hybrid-auto-engine"  # Hybrid auto engine with automatic optimization (default in MinerU 2.7.0+)
    HYBRID = "hybrid"  # Hybrid backend combining multiple processing strategies
    PIPELINE = "pipeline"  # Traditional multimodel pipeline
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


@dataclass
class MinerUParseOptions:
    """Options for MinerU PDF parsing.

    Notes:
    - `batch_size` default is 30; valid range is [1, 500]. Values outside this range
      will be clamped to a safe default or upper bound by the parser.
    - `start_page` and `end_page` are 0-based. If both provided, `start_page` must
      be <= `end_page` otherwise validation will fail.
    - `strict_mode` is a backend-only option (default True) that requires all
      batches to succeed; it is currently not exposed in the UI. To expose it
      in the frontend, add a boolean field (e.g., `mineru_strict_mode`) to the
      MinerU parser form and propagate it into `parser_config`.
    """

    backend: MinerUBackend = MinerUBackend.HYBRID_AUTO_ENGINE
    lang: Optional[MinerULanguage] = None  # language for OCR (pipeline backend only)
    method: MinerUParseMethod = MinerUParseMethod.AUTO
    server_url: Optional[str] = None
    delete_output: bool = True
    parse_method: str = "raw"
    formula_enable: bool = True
    table_enable: bool = True
    batch_size: int = 30  # Number of pages per batch for large PDFs (clamped to [1,500])
    start_page: Optional[int] = None  # Starting page (0-based, inclusive)
    end_page: Optional[int] = None  # Ending page (0-based, exclusive; matches MinerU API semantics)
    strict_mode: bool = True  # If True (default), all batches must succeed; if False, allow partial success with warnings
    exif_correction: bool = True



class MinerUParser(RAGFlowPdfParser):
    def __init__(self, mineru_path: str = "mineru", mineru_api: str = "", mineru_server_url: str = ""):
        self.mineru_api = mineru_api.rstrip("/")
        self.mineru_server_url = mineru_server_url.rstrip("/")
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

    def check_installation(self, backend: str = "pipeline", server_url: Optional[str] = None) -> tuple[bool, str]:
        reason = ""

        valid_backends = ["hybrid-auto-engine", "hybrid", "pipeline", "vlm-http-client", "vlm-transformers", "vlm-vllm-engine", "vlm-mlx-engine", "vlm-vllm-async-engine", "vlm-lmdeploy-engine"]
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

    def _validate_parse_options(self, options: MinerUParseOptions) -> None:
        """Validate parse options for MinerUParser.

        Raises:
            ValueError: If options are invalid (e.g., start_page > end_page).
        """
        # Validate start_page and end_page types and ranges
        if options.start_page is not None and not isinstance(options.start_page, int):
            raise ValueError(f"[MinerU] start_page must be an int or None, got {type(options.start_page).__name__}")
        if options.end_page is not None and not isinstance(options.end_page, int):
            raise ValueError(f"[MinerU] end_page must be an int or None, got {type(options.end_page).__name__}")
        if options.start_page is not None and options.start_page < 0:
            raise ValueError(f"[MinerU] start_page must be >= 0, got {options.start_page}")
        if options.end_page is not None and options.end_page < 0:
            raise ValueError(f"[MinerU] end_page must be >= 0, got {options.end_page}")
        # With our semantics end_page is exclusive (0-based), therefore start_page must be strictly
        # less than end_page when both are provided to refer to a non-empty page range.
        if options.start_page is not None and options.end_page is not None and options.start_page >= options.end_page:
            raise ValueError(f"[MinerU] start_page ({options.start_page}) must be < end_page ({options.end_page})")

        # Validate batch_size and clamp to reasonable bounds
        if not isinstance(options.batch_size, int) or options.batch_size <= 0:
            self.logger.warning(f"[MinerU] invalid batch_size {options.batch_size}, resetting to default 30")
            options.batch_size = 30
        elif options.batch_size > 500:
            self.logger.warning(f"[MinerU] batch_size {options.batch_size} is unusually large; capping to 500")
            options.batch_size = 500

        # strict_mode: ensure boolean and log note because it's currently not exposed in UI
        if not isinstance(options.strict_mode, bool):
            self.logger.warning(f"[MinerU] strict_mode must be boolean, got {type(options.strict_mode).__name__}; coercing to bool")
            options.strict_mode = bool(options.strict_mode)
        # Document strict_mode behaviour in help text to aid frontend developers
        if options.strict_mode:
            self.logger.info("[MinerU] strict_mode is enabled; note: this option currently has no UI control and is backend-only. If you want to expose it in the UI, add a toggle in the mineru form and update translation strings.")

    def _run_mineru(
        self, input_path: Path, output_dir: Path, options: MinerUParseOptions, callback: Optional[Callable] = None
    ) -> Path:
        return self._run_mineru_api(input_path, output_dir, options, callback)

    def _get_total_pages(self, pdf_path: Path) -> int:
        """Return total pages of a PDF using pdfplumber (0 on failure)."""
        try:
            with pdfplumber.open(pdf_path) as pdf:
                return len(pdf.pages)
        except Exception as e:
            self.logger.warning(f"[MinerU] Could not determine total pages for {pdf_path}: {e}")
            return 0

    def _run_mineru_api_single_batch(
        self,
        input_path: Path,
        output_dir: Path,
        options: MinerUParseOptions,
        start_page: int,
        end_page: int,
        callback: Optional[Callable] = None,
    ) -> Path:
        """Run MinerU API for a single page range batch and return the output directory."""
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
            "start_page_id": start_page,
            "end_page_id": end_page,
            "batch_size": options.batch_size,
            "exif_correction": options.exif_correction,
            "strict_mode": options.strict_mode,
        }

        if options.server_url:
            data["server_url"] = options.server_url
        elif self.mineru_server_url:
            data["server_url"] = self.mineru_server_url

        self.logger.info(f"[MinerU] request batch pages={start_page}-{end_page} {data=}")

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
                    if content_type.startswith("application/zip"):
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
                    else:
                        self.logger.warning(f"[MinerU] not zip returned from api: {content_type}")
        except Exception as e:
            raise RuntimeError(f"[MinerU] api failed with exception {e}")
        self.logger.info("[MinerU] Api completed successfully.")
        return Path(output_path)

    def _run_mineru_api(
        self, input_path: Path, output_dir: Path, options: MinerUParseOptions, callback: Optional[Callable] = None
    ) -> Path:
        """Top-level MinerU API runner: supports automatic batching when start/end not set."""
        # Validate options
        try:
            self._validate_parse_options(options)
        except ValueError as ve:
            raise RuntimeError(f"[MinerU] invalid parse options: {ve}")

        pdf_file_path = str(input_path)
        if not os.path.exists(pdf_file_path):
            raise RuntimeError(f"[MinerU] PDF file not exists: {pdf_file_path}")

        # Decision logging: what was provided and configured
        start_set = options.start_page is not None
        end_set = options.end_page is not None
        self.logger.info(f"[MinerU] batching_decision: start_set={start_set}, end_set={end_set}, batch_size={options.batch_size}, strict_mode={options.strict_mode}")

        # If user explicitly set page range, prefer single batch unless the range covers the whole document
        if start_set or end_set:
            s = options.start_page if start_set else 0
            e = options.end_page if end_set else 99999
            try:
                total_pages = self._get_total_pages(input_path)
            except Exception as exc:
                total_pages = 0
                self.logger.warning(f"[MinerU] could not read total pages while evaluating explicit range: {exc}")

            self.logger.info(f"[MinerU] explicit_range_detected: start={s}, end={e}, total_pages={total_pages}")

            # If explicit range appears to be the full document, allow automatic batching
            if s == 0 and total_pages and e >= total_pages:
                self.logger.info(f"[MinerU] Explicit page range covers full document ({s}-{e}) and total_pages={total_pages}; enabling automatic batching")
                # fall through to automatic batching logic below
            else:
                self.logger.info(f"[MinerU] Running single batch for explicit range {s}-{e}")
                return self._run_mineru_api_single_batch(input_path, output_dir, options, s, e, callback=callback)

        # Automatic batching if batch_size positive
        if options.batch_size and options.batch_size > 0:
            total_pages = self._get_total_pages(input_path)
            self.logger.info(f"[MinerU] Auto-batching check: total_pages={total_pages}, batch_size={options.batch_size}")
            if total_pages == 0:
                self.logger.warning("[MinerU] Could not determine total pages; falling back to single batch")
                return self._run_mineru_api_single_batch(input_path, output_dir, options, 0, 99999, callback=callback)
            if total_pages <= options.batch_size:
                self.logger.info(f"[MinerU] total_pages ({total_pages}) <= batch_size ({options.batch_size}); using single batch")
                return self._run_mineru_api_single_batch(input_path, output_dir, options, 0, 99999, callback=callback)

            batches = []
            for batch_start in range(0, total_pages, options.batch_size):
                batch_end = min(batch_start + options.batch_size - 1, total_pages - 1)
                batches.append((batch_start, batch_end))

            merged_outputs = []
            failed_batches = []
            for idx, (bs, be) in enumerate(batches):
                try:
                    self.logger.info(f"[MinerU] processing batch {idx+1}/{len(batches)} pages {bs}-{be}")
                    batch_out_dir = self._run_mineru_api_single_batch(input_path, output_dir, options, bs, be, callback=None)
                    batch_outputs = self._read_output(batch_out_dir, Path(pdf_file_path).stem, method=str(options.method), backend=str(options.backend))
                    # Adjust page indices if present (batch-level -> global)
                    for item in batch_outputs:
                        if isinstance(item, dict) and item.get("page_idx") is not None:
                            item["page_idx"] = item["page_idx"] + bs
                    merged_outputs.extend(batch_outputs)
                    # Cleanup batch outputs if requested
                    if options.delete_output:
                        try:
                            import shutil

                            shutil.rmtree(batch_out_dir)
                        except Exception:
                            pass
                except Exception as e:
                    self.logger.warning(f"[MinerU] batch {idx+1} failed pages {bs}-{be}: {e}")
                    failed_batches.append((bs, be, str(e)))
                    if options.strict_mode:
                        raise RuntimeError(f"[MinerU] Batch {idx+1} failed and strict_mode=True: {e}")

            # Write merged content list to a merged output dir
            pdf_file_name = Path(pdf_file_path).stem.strip()
            merged_output_path = tempfile.mkdtemp(prefix=f"{pdf_file_name}_{options.method}_merged_", dir=str(output_dir))
            merged_json_path = Path(merged_output_path) / f"{pdf_file_name}_content_list.json"
            with open(merged_json_path, "w", encoding="utf-8") as f:
                json.dump(merged_outputs, f, ensure_ascii=False)

            # If some batches failed and strict_mode is True we would have raised before; otherwise log warning
            if failed_batches:
                self.logger.warning(f"[MinerU] {len(failed_batches)} batches failed: {failed_batches}")

            return Path(merged_output_path)

        # Default: single pass over entire document
        return self._run_mineru_api_single_batch(input_path, output_dir, options, 0, 99999, callback=callback)

    def __images__(self, fnm, zoomin: int = 1, page_from=0, page_to=600, callback=None):
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
        allowed_names = {f"{file_stem}_content_list.json", f"{safe_stem}_content_list.json"}
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
            server_url: Optional[str] = None,
            delete_output: bool = True,
            parse_method: str = "raw",
            **kwargs,
    ) -> tuple:
        import shutil

        temp_pdf = None
        created_tmp_dir = False

        parser_cfg = kwargs.get('parser_config', {})
        lang = parser_cfg.get('mineru_lang') or kwargs.get('lang', 'English')
        mineru_lang_code = LANGUAGE_TO_MINERU_MAP.get(lang, 'ch')  # Defaults to Chinese if not matched
        mineru_method_raw_str = parser_cfg.get('mineru_parse_method', 'auto')
        enable_formula = parser_cfg.get('mineru_formula_enable', True)
        enable_table = parser_cfg.get('mineru_table_enable', True)
        start_page = parser_cfg.get('mineru_start_page', None)
        end_page = parser_cfg.get('mineru_end_page', None)
        batch_size = parser_cfg.get('mineru_batch_size', 30)
        strict_mode = parser_cfg.get('mineru_strict_mode', True)
        
        # Handle pages parameter - MinerU only supports single continuous page range
        pages = parser_cfg.get('pages', [])
        if pages and len(pages) > 0:
            if len(pages) == 1:
                # Single page range
                page_range = pages[0]
                if isinstance(page_range, list) and len(page_range) == 2:
                    start_page = page_range[0] - 1  # Convert to 0-based indexing
                    end_page = page_range[1]  # end_page is exclusive in MinerU API
                    self.logger.info(f"[MinerU] Using page range: {page_range} (0-based: {start_page} to {end_page})")
                else:
                    self.logger.warning(f"[MinerU] Invalid page range format: {page_range}")
            else:
                # Multiple page ranges - MinerU doesn't support this, use first range
                self.logger.warning(f"[MinerU] Multiple page ranges not supported, using first range: {pages[0]}")
                page_range = pages[0]
                if isinstance(page_range, list) and len(page_range) == 2:
                    start_page = page_range[0] - 1  # Convert to 0-based indexing
                    end_page = page_range[1]  # end_page is exclusive in MinerU API

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
                backend=MinerUBackend(backend),
                lang=MinerULanguage(mineru_lang_code),
                method=MinerUParseMethod(mineru_method_raw_str),
                server_url=server_url,
                delete_output=delete_output,
                parse_method=parse_method,
                formula_enable=enable_formula,
                table_enable=enable_table,
                batch_size=batch_size,
                start_page=start_page,
                end_page=end_page,
                strict_mode=strict_mode,
            )
            final_out_dir = self._run_mineru(pdf, out_dir, options, callback=callback)
            outputs = self._read_output(final_out_dir, pdf.stem, method=mineru_method_raw_str, backend=backend)
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
