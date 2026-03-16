#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from __future__ import annotations

import base64
import logging
import os
import re
from dataclasses import asdict, dataclass, field, fields
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Any, Callable, ClassVar, Literal, Optional, Union, Tuple, List

import numpy as np
import pdfplumber
import requests
from PIL import Image

try:
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser
except Exception:

    class RAGFlowPdfParser:
        pass


AlgorithmType = Literal["PaddleOCR-VL"]
SectionTuple = tuple[str, ...]
TableTuple = tuple[str, ...]
ParseResult = tuple[list[SectionTuple], list[TableTuple]]


_MARKDOWN_IMAGE_PATTERN = re.compile(
    r"""
        <div[^>]*>\s*
        <img[^>]*/>\s*
        </div>
        |
        <img[^>]*/>
        """,
    re.IGNORECASE | re.VERBOSE | re.DOTALL,
)


def _remove_images_from_markdown(markdown: str) -> str:
    return _MARKDOWN_IMAGE_PATTERN.sub("", markdown)


@dataclass
class PaddleOCRVLConfig:
    """Configuration for PaddleOCR-VL algorithm."""

    use_doc_orientation_classify: Optional[bool] = False
    use_doc_orientation_classify: Optional[bool] = False
    use_doc_unwarping: Optional[bool] = False
    use_layout_detection: Optional[bool] = None
    use_chart_recognition: Optional[bool] = None
    use_seal_recognition: Optional[bool] = None
    use_ocr_for_image_block: Optional[bool] = None
    layout_threshold: Optional[Union[float, dict]] = None
    layout_nms: Optional[bool] = None
    layout_unclip_ratio: Optional[Union[float, Tuple[float, float], dict]] = None
    layout_merge_bboxes_mode: Optional[Union[str, dict]] = None
    layout_shape_mode: Optional[str] = None
    prompt_label: Optional[str] = None
    format_block_content: Optional[bool] = True
    repetition_penalty: Optional[float] = None
    temperature: Optional[float] = None
    top_p: Optional[float] = None
    min_pixels: Optional[int] = None
    max_pixels: Optional[int] = None
    max_new_tokens: Optional[int] = None
    merge_layout_blocks: Optional[bool] = False
    markdown_ignore_labels: Optional[List[str]] = None
    vlm_extra_args: Optional[dict] = None
    restructure_pages: Optional[bool] = False
    merge_tables: Optional[bool] = None
    relevel_titles: Optional[bool] = None


@dataclass
class PaddleOCRConfig:
    """Main configuration for PaddleOCR parser."""

    api_url: str = ""
    access_token: Optional[str] = None
    algorithm: AlgorithmType = "PaddleOCR-VL"
    request_timeout: int = 600
    prettify_markdown: bool = True
    show_formula_number: bool = True
    visualize: bool = False
    additional_params: dict[str, Any] = field(default_factory=dict)
    algorithm_config: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_dict(cls, config: Optional[dict[str, Any]]) -> "PaddleOCRConfig":
        """Create configuration from dictionary."""
        if not config:
            return cls()

        cfg = config.copy()
        algorithm = cfg.get("algorithm", "PaddleOCR-VL")

        # Validate algorithm
        if algorithm not in ("PaddleOCR-VL"):
            raise ValueError(f"Unsupported algorithm: {algorithm}")

        # Extract algorithm-specific configuration
        algorithm_config: dict[str, Any] = {}
        if algorithm == "PaddleOCR-VL":
            algorithm_config = asdict(PaddleOCRVLConfig())
        algorithm_config_user = cfg.get("algorithm_config")
        if isinstance(algorithm_config_user, dict):
            algorithm_config.update({k: v for k, v in algorithm_config_user.items() if v is not None})

        # Remove processed keys
        cfg.pop("algorithm_config", None)

        # Prepare initialization arguments
        field_names = {field.name for field in fields(cls)}
        init_kwargs: dict[str, Any] = {}

        for field_name in field_names:
            if field_name in cfg:
                init_kwargs[field_name] = cfg[field_name]

        init_kwargs["algorithm_config"] = algorithm_config

        return cls(**init_kwargs)

    @classmethod
    def from_kwargs(cls, **kwargs: Any) -> "PaddleOCRConfig":
        """Create configuration from keyword arguments."""
        return cls.from_dict(kwargs)


class PaddleOCRParser(RAGFlowPdfParser):
    """Parser for PDF documents using PaddleOCR API."""

    _ZOOMIN = 2

    _COMMON_FIELD_MAPPING: ClassVar[dict[str, str]] = {
        "prettify_markdown": "prettifyMarkdown",
        "show_formula_number": "showFormulaNumber",
        "visualize": "visualize",
    }

    _ALGORITHM_FIELD_MAPPINGS: ClassVar[dict[str, dict[str, str]]] = {
        "PaddleOCR-VL": {
            "use_doc_orientation_classify": "useDocOrientationClassify",
            "use_doc_unwarping": "useDocUnwarping",
            "use_layout_detection": "useLayoutDetection",
            "use_chart_recognition": "useChartRecognition",
            "use_seal_recognition": "useSealRecognition",
            "use_ocr_for_image_block": "useOcrForImageBlock",
            "layout_threshold": "layoutThreshold",
            "layout_nms": "layoutNms",
            "layout_unclip_ratio": "layoutUnclipRatio",
            "layout_merge_bboxes_mode": "layoutMergeBboxesMode",
            "layout_shape_mode": "layoutShapeMode",
            "prompt_label": "promptLabel",
            "format_block_content": "formatBlockContent",
            "repetition_penalty": "repetitionPenalty",
            "temperature": "temperature",
            "top_p": "topP",
            "min_pixels": "minPixels",
            "max_pixels": "maxPixels",
            "max_new_tokens": "maxNewTokens",
            "merge_layout_blocks": "mergeLayoutBlocks",
            "markdown_ignore_labels": "markdownIgnoreLabels",
            "vlm_extra_args": "vlmExtraArgs",
            "restructure_pages": "restructurePages",
            "merge_tables": "mergeTables",
            "relevel_titles": "relevelTitles",
        },
    }

    def __init__(
        self,
        api_url: Optional[str] = None,
        access_token: Optional[str] = None,
        algorithm: AlgorithmType = "PaddleOCR-VL",
        *,
        request_timeout: int = 600,
    ):
        """Initialize PaddleOCR parser."""
        super().__init__()

        self.api_url = api_url.rstrip("/") if api_url else os.getenv("PADDLEOCR_API_URL", "")
        self.access_token = access_token or os.getenv("PADDLEOCR_ACCESS_TOKEN")
        self.algorithm = algorithm
        self.request_timeout = request_timeout
        self.logger = logging.getLogger(self.__class__.__name__)

        # Force PDF file type
        self.file_type = 0

        # Initialize page images for cropping
        self.page_images: list[Image.Image] = []
        self.page_from = 0

    # Public methods
    def check_installation(self) -> tuple[bool, str]:
        """Check if the parser is properly installed and configured."""
        if not self.api_url:
            return False, "[PaddleOCR] API URL not configured"

        # TODO [@Bobholamovic]: Check URL availability and token validity

        return True, ""

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes | None = None,
        callback: Optional[Callable[[float, str], None]] = None,
        *,
        parse_method: str = "raw",
        api_url: Optional[str] = None,
        access_token: Optional[str] = None,
        algorithm: Optional[AlgorithmType] = None,
        request_timeout: Optional[int] = None,
        prettify_markdown: Optional[bool] = None,
        show_formula_number: Optional[bool] = None,
        visualize: Optional[bool] = None,
        additional_params: Optional[dict[str, Any]] = None,
        algorithm_config: Optional[dict[str, Any]] = None,
        **kwargs: Any,
    ) -> ParseResult:
        """Parse PDF document using PaddleOCR API."""
        # Create configuration - pass all kwargs to capture VL config parameters
        config_dict = {
            "api_url": api_url if api_url is not None else self.api_url,
            "access_token": access_token if access_token is not None else self.access_token,
            "algorithm": algorithm if algorithm is not None else self.algorithm,
            "request_timeout": request_timeout if request_timeout is not None else self.request_timeout,
        }
        if prettify_markdown is not None:
            config_dict["prettify_markdown"] = prettify_markdown
        if show_formula_number is not None:
            config_dict["show_formula_number"] = show_formula_number
        if visualize is not None:
            config_dict["visualize"] = visualize
        if additional_params is not None:
            config_dict["additional_params"] = additional_params
        if algorithm_config is not None:
            config_dict["algorithm_config"] = algorithm_config

        cfg = PaddleOCRConfig.from_dict(config_dict)

        if not cfg.api_url:
            raise RuntimeError("[PaddleOCR] API URL missing")

        # Prepare file data and generate page images for cropping
        data_bytes = self._prepare_file_data(filepath, binary)

        # Generate page images for cropping functionality
        input_source = filepath if binary is None else binary
        try:
            self.__images__(input_source, callback=callback)
        except Exception as e:
            self.logger.warning(f"[PaddleOCR] Failed to generate page images for cropping: {e}")

        # Build and send request
        result = self._send_request(data_bytes, cfg, callback)

        # Process response
        sections = self._transfer_to_sections(result, algorithm=cfg.algorithm, parse_method=parse_method)
        if callback:
            callback(0.9, f"[PaddleOCR] done, sections: {len(sections)}")

        tables = self._transfer_to_tables(result)
        if callback:
            callback(1.0, f"[PaddleOCR] done, tables: {len(tables)}")

        return sections, tables

    def _prepare_file_data(self, filepath: str | PathLike[str], binary: BytesIO | bytes | None) -> bytes:
        """Prepare file data for API request."""
        source_path = Path(filepath)

        if binary is not None:
            if isinstance(binary, (bytes, bytearray)):
                return binary
            return binary.getbuffer().tobytes()

        if not source_path.exists():
            raise FileNotFoundError(f"[PaddleOCR] file not found: {source_path}")

        return source_path.read_bytes()

    def _build_payload(self, data: bytes, file_type: int, config: PaddleOCRConfig) -> dict[str, Any]:
        """Build payload for API request."""
        payload: dict[str, Any] = {
            "file": base64.b64encode(data).decode("ascii"),
            "fileType": file_type,
        }

        # Add common parameters
        for param_key, param_value in [
            ("prettify_markdown", config.prettify_markdown),
            ("show_formula_number", config.show_formula_number),
            ("visualize", config.visualize),
        ]:
            if param_value is not None:
                api_param = self._COMMON_FIELD_MAPPING[param_key]
                payload[api_param] = param_value

        # Add algorithm-specific parameters
        algorithm_mapping = self._ALGORITHM_FIELD_MAPPINGS.get(config.algorithm, {})
        for param_key, param_value in config.algorithm_config.items():
            if param_value is not None and param_key in algorithm_mapping:
                api_param = algorithm_mapping[param_key]
                payload[api_param] = param_value

        # Add any additional parameters
        if config.additional_params:
            payload.update(config.additional_params)

        return payload

    def _send_request(self, data: bytes, config: PaddleOCRConfig, callback: Optional[Callable[[float, str], None]]) -> dict[str, Any]:
        """Send request to PaddleOCR API and parse response."""
        # Build payload
        payload = self._build_payload(data, self.file_type, config)

        # Prepare headers
        headers = {"Content-Type": "application/json", "Client-Platform": "ragflow"}
        if config.access_token:
            headers["Authorization"] = f"token {config.access_token}"

        self.logger.info("[PaddleOCR] invoking API")
        if callback:
            callback(0.1, "[PaddleOCR] submitting request")

        # Send request
        try:
            resp = requests.post(config.api_url, json=payload, headers=headers, timeout=self.request_timeout)
            resp.raise_for_status()
        except Exception as exc:
            if callback:
                callback(-1, f"[PaddleOCR] request failed: {exc}")
            raise RuntimeError(f"[PaddleOCR] request failed: {exc}")

        # Parse response
        try:
            response_data = resp.json()
        except Exception as exc:
            raise RuntimeError(f"[PaddleOCR] response is not JSON: {exc}") from exc

        if callback:
            callback(0.8, "[PaddleOCR] response received")

        # Validate response format
        if response_data.get("errorCode") != 0 or not isinstance(response_data.get("result"), dict):
            if callback:
                callback(-1, "[PaddleOCR] invalid response format")
            raise RuntimeError("[PaddleOCR] invalid response format")

        return response_data["result"]

    def _transfer_to_sections(self, result: dict[str, Any], algorithm: AlgorithmType, parse_method: str) -> list[SectionTuple]:
        """Convert API response to section tuples."""
        sections: list[SectionTuple] = []

        if algorithm in ("PaddleOCR-VL",):
            layout_parsing_results = result.get("layoutParsingResults", [])

            for page_idx, layout_result in enumerate(layout_parsing_results):
                pruned_result = layout_result.get("prunedResult", {})
                parsing_res_list = pruned_result.get("parsing_res_list", [])

                for block in parsing_res_list:
                    block_content = block.get("block_content", "").strip()
                    if not block_content:
                        continue

                    # Remove images
                    block_content = _remove_images_from_markdown(block_content)

                    label = block.get("block_label", "")
                    block_bbox = block.get("block_bbox", [0, 0, 0, 0])

                    tag = f"@@{page_idx + 1}\t{block_bbox[0] // self._ZOOMIN}\t{block_bbox[2] // self._ZOOMIN}\t{block_bbox[1] // self._ZOOMIN}\t{block_bbox[3] // self._ZOOMIN}##"

                    if parse_method == "manual":
                        sections.append((block_content, label, tag))
                    elif parse_method == "paper":
                        sections.append((block_content + tag, label))
                    else:
                        sections.append((block_content, tag))

        return sections

    def _transfer_to_tables(self, result: dict[str, Any]) -> list[TableTuple]:
        """Convert API response to table tuples."""
        return []

    def __images__(self, fnm, page_from=0, page_to=100, callback=None):
        """Generate page images from PDF for cropping."""
        self.page_from = page_from
        self.page_to = page_to
        try:
            with pdfplumber.open(fnm) if isinstance(fnm, (str, PathLike)) else pdfplumber.open(BytesIO(fnm)) as pdf:
                self.pdf = pdf
                self.page_images = [p.to_image(resolution=72, antialias=True).original for i, p in enumerate(self.pdf.pages[page_from:page_to])]
        except Exception as e:
            self.page_images = None
            self.logger.exception(e)

    @staticmethod
    def extract_positions(txt: str):
        """Extract position information from text tags."""
        poss = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", txt):
            pn, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
            left, right, top, bottom = float(left), float(right), float(top), float(bottom)
            poss.append(([int(p) - 1 for p in pn.split("-")], left, right, top, bottom))
        return poss

    def crop(self, text: str, need_position: bool = False):
        """Crop images from PDF based on position tags in text."""
        imgs = []
        poss = self.extract_positions(text)

        if not poss:
            if need_position:
                return None, None
            return

        if not getattr(self, "page_images", None):
            self.logger.warning("[PaddleOCR] crop called without page images; skipping image generation.")
            if need_position:
                return None, None
            return

        page_count = len(self.page_images)

        filtered_poss = []
        for pns, left, right, top, bottom in poss:
            if not pns:
                self.logger.warning("[PaddleOCR] Empty page index list in crop; skipping this position.")
                continue
            valid_pns = [p for p in pns if 0 <= p < page_count]
            if not valid_pns:
                self.logger.warning(f"[PaddleOCR] All page indices {pns} out of range for {page_count} pages; skipping.")
                continue
            filtered_poss.append((valid_pns, left, right, top, bottom))

        poss = filtered_poss
        if not poss:
            self.logger.warning("[PaddleOCR] No valid positions after filtering; skip cropping.")
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
            self.logger.warning(f"[PaddleOCR] Last page index {last_page_idx} out of range for {page_count} pages; skipping crop.")
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
                    self.logger.warning(f"[PaddleOCR] Page index {pn}-1 out of range for {page_count} pages during crop; skipping height accumulation.")

            if not (0 <= pns[0] < page_count):
                self.logger.warning(f"[PaddleOCR] Base page index {pns[0]} out of range for {page_count} pages during crop; skipping this segment.")
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
                    self.logger.warning(f"[PaddleOCR] Page index {pn} out of range for {page_count} pages during crop; skipping this page.")
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


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    parser = PaddleOCRParser(api_url=os.getenv("PADDLEOCR_API_URL", ""), algorithm=os.getenv("PADDLEOCR_ALGORITHM", "PaddleOCR-VL"))
    ok, reason = parser.check_installation()
    print("PaddleOCR available:", ok, reason)
