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
import sys
import tempfile
import threading
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

# Constants
MAX_PAGE_NUMBER = 99999  # Maximum page number for MinerU API (effectively unlimited)
CROP_GAP_PIXELS = 6  # Gap between cropped image segments
CROP_CONTEXT_LINES = 120  # Number of pixels for context before/after crop

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
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
    """Options for MinerU PDF parsing."""

    backend: MinerUBackend = MinerUBackend.HYBRID_AUTO_ENGINE
    lang: Optional[MinerULanguage] = None  # language for OCR (pipeline backend only)
    method: MinerUParseMethod = MinerUParseMethod.AUTO
    server_url: Optional[str] = None
    delete_output: bool = True
    parse_method: str = "raw"
    formula_enable: bool = True
    table_enable: bool = True
    batch_size: int = 30  # Number of pages per batch for large PDFs
    start_page: Optional[int] = None  # Starting page (0-based, for manual pagination)
    end_page: Optional[int] = None  # Ending page (0-based, for manual pagination)


class MinerUParser(RAGFlowPdfParser):
    def __init__(self, mineru_path: str = "mineru", mineru_api: str = "", mineru_server_url: str = ""):
        self.mineru_api = mineru_api.rstrip("/")
        self.mineru_server_url = mineru_server_url.rstrip("/")
        self.outlines = []
        self.logger = logging.getLogger(self.__class__.__name__)

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
                os.makedirs(os.path.dirname(full_path), exist_ok=True)
                if member.is_dir():
                    os.makedirs(full_path, exist_ok=True)
                else:
                    os.makedirs(os.path.dirname(full_path), exist_ok=True)
                    with open(full_path, "wb") as f:
                        f.write(zip_ref.read(filename))

    @staticmethod
    def _is_http_endpoint_valid(url, timeout=5):
        try:
            response = requests.head(url, timeout=timeout, allow_redirects=True)
            return response.status_code in [200, 301, 302, 307, 308]
        except Exception:
            return False

    def check_installation(self, backend: str = "hybrid-auto-engine", server_url: Optional[str] = None) -> tuple[bool, str]:
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

    def _get_total_pages(self, pdf_path: Path) -> int:
        """Get total number of pages in a PDF file using pypdf.
        
        Args:
            pdf_path: Path to the PDF file
            
        Returns:
            Total number of pages, or 0 if unable to determine
            
        Note:
            This method uses pypdf which only reads the PDF structure,
            not the full content, so it's memory-efficient even for large PDFs.
        """
        try:
            from pypdf import PdfReader
            # Use a context manager to ensure file is closed properly
            with open(pdf_path, 'rb') as f:
                try:
                    reader = PdfReader(f)
                    total_pages = len(reader.pages)
                    self.logger.info(f"[MinerU] PDF has {total_pages} pages: {pdf_path}")
                    return total_pages
                except MemoryError as e:
                    self.logger.error(f"[MinerU] Memory error while reading PDF structure: {e}")
                    return 0
                except Exception as e:
                    self.logger.warning(f"[MinerU] Failed to get page count from PDF: {e}")
                    return 0
        except IOError as e:
            self.logger.error(f"[MinerU] Failed to open PDF file {pdf_path}: {e}")
            return 0
        except ImportError as e:
            self.logger.error(f"[MinerU] pypdf library not available: {e}")
            return 0
        except Exception as e:
            self.logger.warning(f"[MinerU] Unexpected error getting total pages: {e}")
            return 0

    def _run_mineru(
        self, input_path: Path, output_dir: Path, options: MinerUParseOptions, callback: Optional[Callable] = None
    ) -> Path:
        return self._run_mineru_api(input_path, output_dir, options, callback)

    def _run_mineru_api(
        self, input_path: Path, output_dir: Path, options: MinerUParseOptions, callback: Optional[Callable] = None
    ) -> Path:
        """Run MinerU API with batch processing support.
        
        If start_page/end_page are None and batch_size > 0, automatically batch the PDF.
        Otherwise, process with specified pages or entire document.
        """
        pdf_file_path = str(input_path)

        if not os.path.exists(pdf_file_path):
            raise RuntimeError(f"[MinerU] PDF file not exists: {pdf_file_path}")

        pdf_file_name = Path(pdf_file_path).stem.strip()
        
        # Determine if we need to batch process
        need_batching = (options.start_page is None and options.end_page is None and options.batch_size > 0)
        
        if need_batching:
            # Get total pages for automatic batching
            total_pages = self._get_total_pages(input_path)
            if total_pages == 0:
                self.logger.warning("[MinerU] Could not determine total pages, processing without batching")
                need_batching = False
            elif total_pages <= options.batch_size:
                self.logger.info(f"[MinerU] PDF has {total_pages} pages, batch_size={options.batch_size}, no batching needed")
                need_batching = False
        
        if not need_batching:
            # Process entire document or specified page range (no batching)
            return self._run_mineru_api_single_batch(
                input_path, output_dir, options, callback,
                start_page=options.start_page if options.start_page is not None else 0,
                end_page=options.end_page if options.end_page is not None else MAX_PAGE_NUMBER
            )
        
        # Batch processing: split into multiple API calls
        self.logger.info(f"[MinerU] Batch processing enabled: total_pages={total_pages}, batch_size={options.batch_size}")
        
        batches = []
        for batch_start in range(0, total_pages, options.batch_size):
            batch_end = min(batch_start + options.batch_size - 1, total_pages - 1)
            batches.append((batch_start, batch_end))
        
        self.logger.info(f"[MinerU] Processing {len(batches)} batches: {batches}")
        
        # Create a single output directory for merged results
        output_path = tempfile.mkdtemp(prefix=f"{pdf_file_name}_{options.method}_merged_", dir=str(output_dir))
        merged_content_list = []
        
        for batch_idx, (batch_start, batch_end) in enumerate(batches):
            self.logger.info(f"[MinerU] Processing batch {batch_idx + 1}/{len(batches)}: pages {batch_start}-{batch_end}")
            
            if callback:
                progress = 0.20 + (batch_idx / len(batches)) * 0.50  # Progress from 20% to 70%
                callback(progress, f"[MinerU] Processing batch {batch_idx + 1}/{len(batches)}: pages {batch_start}-{batch_end}")
            
            try:
                # Process this batch
                batch_output_path = self._run_mineru_api_single_batch(
                    input_path, output_dir, options, None,  # No callback for individual batches
                    start_page=batch_start,
                    end_page=batch_end
                )
                
                # Read the batch results
                batch_content_list = self._read_output(batch_output_path, pdf_file_name, 
                                                       method=str(options.method), backend=str(options.backend))
                
                # Adjust page indices in batch results to reflect global page numbers
                for item in batch_content_list:
                    if 'page_idx' in item:
                        item['page_idx'] += batch_start
                
                merged_content_list.extend(batch_content_list)
                
                # Clean up batch output if needed
                if options.delete_output:
                    try:
                        import shutil
                        shutil.rmtree(batch_output_path)
                    except Exception as cleanup_err:
                        self.logger.warning(f"[MinerU] Failed to clean up batch output {batch_output_path}: {cleanup_err}")
                        
            except Exception as batch_err:
                self.logger.error(f"[MinerU] Batch {batch_idx + 1} failed (pages {batch_start}-{batch_end}): {batch_err}")
                # Try to continue with other batches or re-raise if it's the first batch
                if batch_idx == 0:
                    raise RuntimeError(f"[MinerU] First batch failed, aborting: {batch_err}")
                else:
                    self.logger.warning(f"[MinerU] Continuing with remaining batches after batch {batch_idx + 1} failure")
        
        if not merged_content_list:
            raise RuntimeError("[MinerU] No content extracted from any batch")
        
        # Write merged content_list to output directory
        merged_json_path = Path(output_path) / f"{pdf_file_name}_content_list.json"
        with open(merged_json_path, 'w', encoding='utf-8') as f:
            json.dump(merged_content_list, f, ensure_ascii=False, indent=2)
        
        self.logger.info(f"[MinerU] Batch processing completed: {len(batches)} batches, {len(merged_content_list)} total blocks")
        
        if callback:
            callback(0.75, f"[MinerU] Batch processing completed: {len(merged_content_list)} blocks")
        
        return Path(output_path)

    def _run_mineru_api_single_batch(
        self, input_path: Path, output_dir: Path, options: MinerUParseOptions, callback: Optional[Callable] = None,
        start_page: int = 0, end_page: int = MAX_PAGE_NUMBER
    ) -> Path:
        """Process a single batch (or entire document) via MinerU API.
        
        Args:
            input_path: Path to the PDF file
            output_dir: Directory to store output
            options: MinerU parsing options
            callback: Optional progress callback
            start_page: Starting page (0-based, inclusive)
            end_page: Ending page (0-based, inclusive)
            
        Returns:
            Path to the output directory containing results
        """
        pdf_file_path = str(input_path)
        pdf_file_name = Path(pdf_file_path).stem.strip()
        
        # Create unique output path for this batch
        batch_suffix = f"_p{start_page}-{end_page}" if start_page > 0 or end_page < 99999 else ""
        output_path = tempfile.mkdtemp(prefix=f"{pdf_file_name}_{options.method}{batch_suffix}_", dir=str(output_dir))
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
        }

        if options.server_url:
            data["server_url"] = options.server_url
        elif self.mineru_server_url:
            data["server_url"] = self.mineru_server_url

        self.logger.info(f"[MinerU] request {data=}")
        self.logger.info(f"[MinerU] request {options=}")

        headers = {"Accept": "application/json"}
        
        # Open file in context manager to ensure proper cleanup
        try:
            with open(pdf_file_path, "rb") as pdf_file:
                files = {"files": (pdf_file_name + ".pdf", pdf_file, "application/pdf")}
                
                self.logger.info(f"[MinerU] invoke api: {self.mineru_api}/file_parse backend={options.backend} server_url={data.get('server_url')} pages={start_page}-{end_page}")
                if callback:
                    callback(0.20, f"[MinerU] invoke api: {self.mineru_api}/file_parse pages={start_page}-{end_page}")
                response = requests.post(url=f"{self.mineru_api}/file_parse", files=files, data=data, headers=headers,
                                          timeout=1800)

            response.raise_for_status()
            if response.headers.get("Content-Type") == "application/zip":
                self.logger.info(f"[MinerU] zip file returned, saving to {output_zip_path}...")

                if callback:
                    callback(0.30, f"[MinerU] zip file returned, saving to {output_zip_path}...")

                with open(output_zip_path, "wb") as f:
                    f.write(response.content)

                self.logger.info(f"[MinerU] Unzip to {output_path}...")
                self._extract_zip_no_root(output_zip_path, output_path, pdf_file_name + "/")

                if callback:
                    callback(0.40, f"[MinerU] Unzip to {output_path}...")
            else:
                self.logger.warning(f"[MinerU] not zip returned from api: {response.headers.get('Content-Type')}")
                raise RuntimeError(f"[MinerU] Unexpected response type: {response.headers.get('Content-Type')}")
        except requests.exceptions.Timeout as e:
            raise RuntimeError(f"[MinerU] API request timed out after 1800 seconds: {e}")
        except requests.exceptions.RequestException as e:
            raise RuntimeError(f"[MinerU] API request failed: {e}")
        except IOError as e:
            raise RuntimeError(f"[MinerU] File I/O error: {e}")
        except Exception as e:
            raise RuntimeError(f"[MinerU] API failed with exception: {e}")
        
        self.logger.info("[MinerU] Api completed successfully.")
        return Path(output_path)

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

    @staticmethod
    def _validate_crop_coordinates(left: float, right: float, top: float, bottom: float, tolerance: float = 1e-6) -> bool:
        """Validate crop coordinates are valid.
        
        Args:
            left, right, top, bottom: Coordinate values
            tolerance: Tolerance for floating-point comparison
            
        Returns:
            True if coordinates are valid, False otherwise
        """
        if left < 0 or right < 0 or top < 0 or bottom < 0:
            return False
        # Use tolerance for floating-point comparison
        if left >= right - tolerance or top >= bottom - tolerance:
            return False
        return True

    @staticmethod
    def _clamp_coordinates_to_image(left: float, top: float, right: float, bottom: float, 
                                     img_width: int, img_height: int) -> tuple[int, int, int, int]:
        """Clamp crop coordinates to image bounds.
        
        Args:
            left, top, right, bottom: Coordinate values (may be outside image)
            img_width, img_height: Image dimensions
            
        Returns:
            Tuple of (x0, y0, x1, y1) clamped to valid image coordinates
        """
        x0 = max(0, min(int(left), img_width - 1))
        y0 = max(0, min(int(top), img_height - 1))
        x1 = max(x0 + 1, min(int(right), img_width))
        y1 = max(y0 + 1, min(int(bottom), img_height))
        return x0, y0, x1, y1

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
            
            # Validate coordinates
            if not self._validate_crop_coordinates(left, right, top, bottom):
                self.logger.warning(f"[MinerU] Invalid crop coordinates: left={left}, right={right}, top={top}, bottom={bottom}; skipping.")
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

        try:
            max_width = max(np.max([right - left for (_, left, right, _, _) in poss]), CROP_GAP_PIXELS)
        except (ValueError, TypeError) as e:
            self.logger.error(f"[MinerU] Failed to calculate max_width: {e}")
            if need_position:
                return None, None
            return
             
        pos = poss[0]
        first_page_idx = pos[0][0]
        
        # Validate first_page_idx before insertion
        if not (0 <= first_page_idx < page_count):
            self.logger.warning(f"[MinerU] First page index {first_page_idx} out of range; using fallback.")
            first_page_idx = 0
            
        poss.insert(0, ([first_page_idx], pos[1], pos[2], max(0, pos[3] - CROP_CONTEXT_LINES), max(pos[3] - CROP_GAP_PIXELS, 0)))
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
                min(last_page_height, pos[4] + CROP_GAP_PIXELS),
                min(last_page_height, pos[4] + CROP_CONTEXT_LINES),
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
            img_width, img_height = img0.size
            
            # Clamp coordinates to image bounds using helper
            x0, y0, x1, y1 = self._clamp_coordinates_to_image(left, top, right, bottom, img_width, img_height)
            
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
                page_width, page_height = page.size
                
                # Clamp coordinates to image bounds using helper
                x0, y0, x1, y1 = self._clamp_coordinates_to_image(left, 0, right, bottom, page_width, page_height)
                
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
            height += img.size[1] + CROP_GAP_PIXELS
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
            height += img.size[1] + CROP_GAP_PIXELS

        if need_position:
            return pic, positions
        return pic

    @staticmethod
    def extract_positions(txt: str):
        poss = []
        for tag in re.findall(r"@@[0-9-]+	[0-9.	]+##", txt):
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
            sanitized = re.sub(r"[/\\.]{2,}|[/\\]", "", name)
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

        try:
            with open(json_file, "r", encoding="utf-8") as f:
                data = json.load(f)
        except json.JSONDecodeError as e:
            raise RuntimeError(f"[MinerU] Failed to parse JSON output file {json_file}: {e}")
        except IOError as e:
            raise RuntimeError(f"[MinerU] Failed to read output file {json_file}: {e}")
        
        if not isinstance(data, list):
            raise RuntimeError(f"[MinerU] Expected list in output file, got {type(data)}")

        for item in data:
            if not isinstance(item, dict):
                self.logger.warning(f"[MinerU] Unexpected item type in output: {type(item)}")
                continue
                
            for key in ("img_path", "table_img_path", "equation_img_path"):
                if key in item and item[key]:
                    try:
                        # Resolve relative paths
                        img_path = Path(item[key])
                        if not img_path.is_absolute():
                            img_path = subdir / item[key]
                        item[key] = str(img_path.resolve())
                        
                        # Check if referenced file exists and log warning if missing
                        if not img_path.exists():
                            self.logger.warning(f"[MinerU] Referenced file does not exist: {img_path}")
                    except Exception as e:
                        self.logger.warning(f"[MinerU] Failed to resolve path for {key}='{item[key]}': {e}")
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

        # Validate inputs
        if not filepath and not binary:
            raise ValueError("[MinerU] Either filepath or binary must be provided")
        
        if backend not in [b.value for b in MinerUBackend]:
            self.logger.warning(f"[MinerU] Unknown backend '{backend}', using 'hybrid-auto-engine'")
            backend = "hybrid-auto-engine"

        temp_pdf = None
        created_tmp_dir = False

        parser_cfg = kwargs.get('parser_config', {})
        if not isinstance(parser_cfg, dict):
            self.logger.warning(f"[MinerU] parser_config is not a dict (type: {type(parser_cfg)}), using empty dict")
            parser_cfg = {}
            
        lang = parser_cfg.get('mineru_lang') or kwargs.get('lang', 'English')
        mineru_lang_code = LANGUAGE_TO_MINERU_MAP.get(lang, 'ch')  # Defaults to Chinese if not matched
        mineru_method_raw_str = parser_cfg.get('mineru_parse_method', 'auto')
        
        # Validate parse method
        if mineru_method_raw_str not in [m.value for m in MinerUParseMethod]:
            self.logger.warning(f"[MinerU] Invalid parse method '{mineru_method_raw_str}', using 'auto'")
            mineru_method_raw_str = 'auto'
            
        enable_formula = parser_cfg.get('mineru_formula_enable', True)
        enable_table = parser_cfg.get('mineru_table_enable', True)
        
        # Batch processing configuration with validation
        batch_size = parser_cfg.get('mineru_batch_size', 30)  # Default 30 pages per batch
        try:
            batch_size = max(1, int(batch_size))  # Ensure at least 1
        except (ValueError, TypeError):
            self.logger.warning(f"[MinerU] Invalid batch_size '{batch_size}', using default 30")
            batch_size = 30
            
        start_page = parser_cfg.get('mineru_start_page', None)  # Manual pagination (0-based)
        end_page = parser_cfg.get('mineru_end_page', None)  # Manual pagination (0-based)
        
        # Validate page numbers if specified
        if start_page is not None:
            try:
                start_page = max(0, int(start_page))
            except (ValueError, TypeError):
                self.logger.warning(f"[MinerU] Invalid start_page '{start_page}', ignoring")
                start_page = None
                
        if end_page is not None:
            try:
                end_page = max(0, int(end_page))
            except (ValueError, TypeError):
                self.logger.warning(f"[MinerU] Invalid end_page '{end_page}', ignoring")
                end_page = None
        
        # Validate page range
        if start_page is not None and end_page is not None and start_page > end_page:
                self.logger.warning(f"[MinerU] start_page ({start_page}) > end_page ({end_page}), swapping")
                start_page, end_page = end_page, start_page

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
