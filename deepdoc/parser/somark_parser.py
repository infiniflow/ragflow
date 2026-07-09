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
"""SoMark document parser adapter.

Bridges RAGFlow's PDF parsing pipeline to the SoMark async HTTP API.
Submits a PDF, polls the async task until completion,
then maps SoMark's structured JSON blocks into the (text, layout_type, line_tag)
triples that RAGFlow's downstream chunker expects.
"""

import json
import logging
import os
import random
import re
import sys
import tempfile
import threading
import time
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Callable, Optional

import numpy as np
import pdfplumber
import requests
from PIL import Image
from enum import StrEnum

from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from deepdoc.parser.utils import extract_pdf_outlines

from common.constants import MAXIMUM_PAGE_NUMBER

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


class SoMarkBlockType(StrEnum):
    """All block.type values returned by SoMark JSON output."""

    TEXT = "text"
    TITLE = "title"
    FIGURE = "figure"
    FIGURE_CAPTION = "figure_caption"
    TABLE = "table"
    TABLE_CAPTION = "table_caption"
    HEADER = "header"
    FOOTER = "footer"
    FOOTNOTE = "footnote"
    SIDER = "sider"
    CATE = "cate"
    CATE_ITEM = "cate_item"
    CODE = "code"
    CHOICE = "choice"
    BLANK = "blank"
    QRCODE = "qrcode"
    STAMP = "stamp"
    REFERENCE = "reference"
    EQUATION = "equation"
    CS = "cs"
    CS_EQUATION = "cs_equation"


# Map each SoMark type to RAGFlow's internal layout type.
# Internal types used downstream: text / table / image / equation / code / discarded.
SOMARK_TYPE_TO_RAGFLOW = {
    SoMarkBlockType.TEXT: "text",
    SoMarkBlockType.TITLE: "text",
    SoMarkBlockType.FIGURE: "image",
    SoMarkBlockType.FIGURE_CAPTION: "text",
    SoMarkBlockType.TABLE: "table",
    SoMarkBlockType.TABLE_CAPTION: "text",
    SoMarkBlockType.FOOTNOTE: "text",
    SoMarkBlockType.SIDER: "text",
    SoMarkBlockType.CODE: "code",
    SoMarkBlockType.CHOICE: "text",
    SoMarkBlockType.REFERENCE: "text",
    SoMarkBlockType.EQUATION: "equation",
    SoMarkBlockType.CS: "image",
    SoMarkBlockType.CS_EQUATION: "text",
    SoMarkBlockType.QRCODE: "image",
    SoMarkBlockType.STAMP: "image",
    # header/footer resolved at runtime based on keep_header_footer flag.
    # cate/cate_item/blank are always discarded.
}

# Block types that are always dropped (TOC noise, empty form fields).
ALWAYS_DISCARDED = {
    SoMarkBlockType.CATE,
    SoMarkBlockType.CATE_ITEM,
    SoMarkBlockType.BLANK,
}


class SoMarkAPIError(RuntimeError):
    """Raised when SoMark API returns a non-zero ``code`` or HTTP failure."""


class SoMarkParser(RAGFlowPdfParser):
    """Parse a PDF via SoMark's async HTTP API and convert blocks to RAGFlow sections."""

    SUBMIT_PATH = "/parse/async"
    CHECK_PATH = "/parse/async_check"
    USAGE_PATH = "/usage"

    # /usage quota check only works in SaaS; private deployments fall back
    # to a generic HEAD health check.
    SAAS_BASE_URL = "https://somark.tech/api/v1"
    USAGE_REQUEST_TIMEOUT = 10  # /usage request timeout

    # SoMark error codes
    QPS_LIMIT_CODE = 1124  # rate limited; retry with backoff when hit during submission
    INVALID_API_KEY_CODE = 1107  # returned by /usage check when API key is invalid

    # Submission phase: retry "concurrency slots full" rejections within a fixed budget
    SUBMIT_BUDGET_SECONDS = 10 * 60  # total submission retry budget (10 min)
    SUBMIT_BACKOFF_BASE_SECONDS = 1.0  # initial backoff interval
    SUBMIT_BACKOFF_MAX_SECONDS = 10.0  # max single backoff interval
    SUBMIT_BACKOFF_JITTER_SECONDS = 0.5  # jitter to avoid thundering herd from concurrent callers
    SUBMIT_REQUEST_TIMEOUT = 60  # single submit request timeout

    # Polling phase: keep querying task status until success / failure / budget exhausted
    POLL_BUDGET_SECONDS = 10 * 60  # max time to wait for a single task
    POLL_INTERVAL_BASE_SECONDS = 2.0  # initial polling interval
    POLL_INTERVAL_MAX_SECONDS = 10.0  # max polling interval for long-running tasks
    POLL_INTERVAL_GROWTH = 1.5  # multiplier applied after each poll
    POLL_REQUEST_TIMEOUT = 30  # single poll request timeout

    def __init__(
        self,
        base_url: str,
        api_key: str = "",
        *,
        image_format: str = "url",
        formula_format: str = "latex",
        table_format: str = "html",
        cs_format: str = "image",
        enable_text_cross_page: bool = False,
        enable_table_cross_page: bool = False,
        enable_title_level_recognition: bool = False,
        enable_inline_image: bool = False,
        enable_table_image: bool = True,
        enable_image_understanding: bool = True,
        keep_header_footer: bool = False,
    ):
        self.base_url = base_url.strip().rstrip("/")
        # Intentionally NOT stripping: caller may want to pass raw key as-is
        # (e.g. for verification where whitespace would also be reported back).
        self.api_key = api_key
        self.element_formats = {
            "image": image_format.strip().lower(),
            "formula": formula_format.strip().lower(),
            "table": table_format.strip().lower(),
            "cs": cs_format.strip().lower(),
        }
        self.feature_config = {
            "enable_text_cross_page": bool(enable_text_cross_page),
            "enable_table_cross_page": bool(enable_table_cross_page),
            "enable_title_level_recognition": bool(enable_title_level_recognition),
            "enable_inline_image": bool(enable_inline_image),
            "enable_table_image": bool(enable_table_image),
            "enable_image_understanding": bool(enable_image_understanding),
            "keep_header_footer": bool(keep_header_footer),
        }
        self.outlines: list = []
        self.logger = logging.getLogger(self.__class__.__name__)

    # ---------------------------------------------------------------------
    # Reachability check
    # ---------------------------------------------------------------------
    @staticmethod
    def _is_http_endpoint_valid(url: str, timeout: int = 5) -> bool:
        try:
            response = requests.head(url, timeout=timeout, allow_redirects=True)
            return response.status_code < 500
        except Exception:
            return False

    def check_installation(self) -> tuple[bool, str]:
        if not self.base_url:
            return False, "[SoMark] SOMARK_BASE_URL not configured."
        if not self.base_url.startswith(("http://", "https://")):
            return False, "[SoMark] SOMARK_BASE_URL must start with http:// or https://."

        # SaaS deployment: hit /usage to verify API key validity and remaining quota.
        if self.base_url == self.SAAS_BASE_URL:
            return self._check_saas_usage()

        # Private deployment: use a cheap HEAD health check.
        if not self._is_http_endpoint_valid(self.base_url):
            return False, f"[SoMark] server unreachable: {self.base_url}"
        return True, ""

    def _check_saas_usage(self) -> tuple[bool, str]:
        """Verify api_key and remaining quota against the hosted SoMark service.

        Treats two specific business outcomes as user-facing errors:
          - ``code == 1107``: invalid API key.
          - ``code == 0`` but both ``remaining_paid_pages`` and
            ``remaining_free_pages_this_month`` are 0: out of parse quota.
        """
        url = f"{self.base_url}{self.USAGE_PATH}"
        data = self._auth_field()
        try:
            resp = requests.post(url, data=data, timeout=self.USAGE_REQUEST_TIMEOUT)
        except requests.RequestException as exc:
            return False, f"[SoMark] usage check failed: {exc}"

        if resp.status_code >= 500:
            return False, (f"[SoMark] usage HTTP {resp.status_code}: {resp.text[:200]}")

        try:
            body = resp.json()
        except ValueError:
            return False, (f"[SoMark] usage non-JSON response ({resp.status_code}): {resp.text[:200]}")

        code = body.get("code")
        message = body.get("message") or ""

        if code == self.INVALID_API_KEY_CODE:
            return False, f"[SoMark] {message or 'Invalid API key'}"

        if code != 0:
            return False, f"[SoMark] usage error code={code} message={message}"

        usage = body.get("data") or {}
        paid = usage.get("remaining_paid_pages") or 0
        free = usage.get("remaining_free_pages_this_month") or 0
        if paid == 0 and free == 0:
            return False, ("[SoMark] insufficient parse pages (remaining_paid_pages=0, remaining_free_pages_this_month=0)")

        return True, ""

    # ---------------------------------------------------------------------
    # HTTP helpers
    # ---------------------------------------------------------------------
    def _auth_field(self) -> dict:
        """Return the api_key multipart field if configured; empty dict otherwise.

        SoMark's hosted API requires ``api_key``. Local deployments reject the
        field outright, so we omit it entirely when blank.
        """
        return {"api_key": self.api_key} if self.api_key else {}

    def _submit_task(self, pdf_path: Path, callback: Optional[Callable] = None) -> str:
        url = f"{self.base_url}{self.SUBMIT_PATH}"
        data = {
            "output_formats": ["json"],
            "element_formats": json.dumps(self.element_formats, ensure_ascii=False),
            "feature_config": json.dumps(self.feature_config, ensure_ascii=False),
        }
        data.update(self._auth_field())

        safe_keys = [k for k in data if k != "api_key"]
        self.logger.info(f"[SoMark] submit {url} keys={safe_keys} has_api_key={bool(self.api_key)}")
        if callback:
            callback(0.20, f"[SoMark] submitting task to {url}")

        deadline = time.monotonic() + self.SUBMIT_BUDGET_SECONDS
        attempt = 0

        while True:
            try:
                with open(pdf_path, "rb") as fh:
                    files = {"file": (pdf_path.name, fh, "application/pdf")}
                    # multipart fields with list values must be sent as repeated tuples
                    form_data = []
                    for key, value in data.items():
                        if isinstance(value, list):
                            for v in value:
                                form_data.append((key, str(v)))
                        else:
                            form_data.append((key, str(value)))
                    resp = requests.post(
                        url,
                        files=files,
                        data=form_data,
                        timeout=self.SUBMIT_REQUEST_TIMEOUT,
                    )
            except requests.RequestException as exc:
                raise SoMarkAPIError(f"[SoMark] submit failed: {exc}") from exc

            # Inline parsing so the QPS-limit code can be distinguished from
            # other business errors before raising.
            if resp.status_code >= 500:
                raise SoMarkAPIError(f"[SoMark] submit HTTP {resp.status_code}: {resp.text[:200]}")
            try:
                body = resp.json()
            except ValueError as exc:
                raise SoMarkAPIError(f"[SoMark] submit non-JSON response ({resp.status_code}): {resp.text[:200]}") from exc

            code = body.get("code")
            if code == 0:
                task_id = (body.get("data") or {}).get("task_id")
                if not task_id:
                    raise SoMarkAPIError(f"[SoMark] submit returned no task_id: {body}")
                self.logger.info(f"[SoMark] task submitted, task_id={task_id} attempts={attempt + 1}")
                return task_id

            # QPS / concurrency rejection: exponential backoff within budget.
            if code == self.QPS_LIMIT_CODE:
                backoff = min(
                    self.SUBMIT_BACKOFF_BASE_SECONDS * (2**attempt),
                    self.SUBMIT_BACKOFF_MAX_SECONDS,
                )
                wait = backoff + random.random() * self.SUBMIT_BACKOFF_JITTER_SECONDS
                if time.monotonic() + wait > deadline:
                    raise SoMarkAPIError("[SoMark] submit blocked by QPS limit; retry budget exhausted")
                self.logger.info(
                    "[SoMark] submit hit QPS limit, retrying in %.2fs (attempt=%d)",
                    wait,
                    attempt + 1,
                )
                if callback:
                    callback(
                        0.20,
                        f"[SoMark] busy (QPS limit), backing off {wait:.2f}s before retry",
                    )
                time.sleep(wait)
                attempt += 1
                continue

            # Any other non-zero code: not retryable.
            raise SoMarkAPIError(f"[SoMark] submit business error code={code} message={body.get('message')}")

    def _poll_task(self, task_id: str, callback: Optional[Callable] = None) -> dict:
        url = f"{self.base_url}{self.CHECK_PATH}"
        deadline = time.monotonic() + self.POLL_BUDGET_SECONDS
        interval = self.POLL_INTERVAL_BASE_SECONDS
        started_at = time.monotonic()
        attempt = 0

        while time.monotonic() < deadline:
            # Sleep first: the task was just submitted, an immediate poll is wasteful.
            time.sleep(interval)
            attempt += 1

            data = {"task_id": task_id}
            data.update(self._auth_field())
            try:
                resp = requests.post(url, data=data, timeout=self.POLL_REQUEST_TIMEOUT)
            except requests.RequestException as exc:
                raise SoMarkAPIError(f"[SoMark] poll request failed: {exc}") from exc

            body = self._parse_json_body(resp, "poll")
            payload = body.get("data") or {}
            status = payload.get("status")
            elapsed = time.monotonic() - started_at

            if status == "SUCCESS":
                self.logger.info(f"[SoMark] task {task_id} completed after {attempt} poll(s) in {elapsed:.1f}s")
                result = payload.get("result")
                if not result:
                    raise SoMarkAPIError(f"[SoMark] SUCCESS but no result: {body}")
                return result
            if status == "FAILED":
                raise SoMarkAPIError(f"[SoMark] task {task_id} FAILED: {body.get('message')}")

            if callback and attempt % 5 == 0:
                callback(
                    0.40,
                    f"[SoMark] still {status}, polled {attempt} time(s) (elapsed={elapsed:.1f}s, next in {interval:.1f}s)",
                )

            interval = min(
                interval * self.POLL_INTERVAL_GROWTH,
                self.POLL_INTERVAL_MAX_SECONDS,
            )

        raise SoMarkAPIError(f"[SoMark] task {task_id} timed out after {self.POLL_BUDGET_SECONDS}s while waiting")

    @staticmethod
    def _parse_json_body(resp: requests.Response, stage: str) -> dict:
        if resp.status_code >= 500:
            raise SoMarkAPIError(f"[SoMark] {stage} HTTP {resp.status_code}: {resp.text[:200]}")
        try:
            body = resp.json()
        except ValueError as exc:
            raise SoMarkAPIError(f"[SoMark] {stage} non-JSON response ({resp.status_code}): {resp.text[:200]}") from exc

        code = body.get("code")
        if code != 0:
            raise SoMarkAPIError(f"[SoMark] {stage} business error code={code} message={body.get('message')}")
        return body

    # ---------------------------------------------------------------------
    # Page image rendering
    # ---------------------------------------------------------------------
    def __images__(
        self,
        fnm,
        zoomin: int = 1,
        page_from: int = 0,
        page_to: int = MAXIMUM_PAGE_NUMBER,
        callback=None,
    ):
        self.page_from = page_from
        self.page_to = page_to
        try:
            ctx = pdfplumber.open(fnm) if isinstance(fnm, (str, PathLike)) else pdfplumber.open(BytesIO(fnm))
            with ctx as pdf:
                self.pdf = pdf
                self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).original for _, p in enumerate(self.pdf.pages[page_from:page_to])]
        except Exception as exc:
            self.page_images = None
            self.total_page = 0
            self.logger.exception(exc)

    # ---------------------------------------------------------------------
    # Position tagging (compatible with RAGFlow's extract_positions/crop)
    # ---------------------------------------------------------------------
    def _line_tag(self, bx: dict) -> str:
        """Build a ``@@page\\tx0\\tx1\\ty0\\ty1##`` tag.

        bx requires keys: ``page_idx`` (0-based), ``bbox`` ([x1,y1,x2,y2] in
        SoMark's reported pixel coordinates), ``page_size`` ({h,w}).
        """
        page_idx = bx.get("page_idx", 0)
        pn = [page_idx + 1]
        bbox = bx.get("bbox") or (0, 0, 0, 0)
        if len(bbox) != 4:
            bbox = (0, 0, 0, 0)
        x0, top, x1, bott = bbox
        page_size = bx.get("page_size") or {}
        src_w = page_size.get("w") or 1
        src_h = page_size.get("h") or 1

        if x0 > x1:
            x0, x1 = x1, x0
        if top > bott:
            top, bott = bott, top

        if hasattr(self, "page_images") and self.page_images and len(self.page_images) > page_idx:
            page_width, page_height = self.page_images[page_idx].size
            x0 = (x0 / src_w) * page_width
            x1 = (x1 / src_w) * page_width
            top = (top / src_h) * page_height
            bott = (bott / src_h) * page_height

        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format("-".join(str(p) for p in pn), x0, x1, top, bott)

    @staticmethod
    def extract_positions(txt: str):
        poss = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", txt):
            pn, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
            left, right, top, bottom = (
                float(left),
                float(right),
                float(top),
                float(bottom),
            )
            poss.append(([int(p) - 1 for p in pn.split("-")], left, right, top, bottom))
        return poss

    def crop(self, text, ZM=1, need_position=False):
        """Image crop based on tags."""
        imgs = []
        poss = self.extract_positions(text)
        if not poss:
            return (None, None) if need_position else None
        if not getattr(self, "page_images", None):
            self.logger.warning("[SoMark] crop called without page images; skip.")
            return (None, None) if need_position else None

        page_count = len(self.page_images)
        filtered = []
        for pns, left, right, top, bottom in poss:
            valid_pns = [p for p in pns if 0 <= p < page_count]
            if valid_pns:
                filtered.append((valid_pns, left, right, top, bottom))
        if not filtered:
            return (None, None) if need_position else None
        poss = filtered

        max_width = max(np.max([right - left for (_, left, right, _, _) in poss]), 6)
        GAP = 6
        first = poss[0]
        poss.insert(
            0,
            (
                [first[0][0]],
                first[1],
                first[2],
                max(0, first[3] - 120),
                max(first[3] - GAP, 0),
            ),
        )
        last = poss[-1]
        last_pn = last[0][-1]
        if not (0 <= last_pn < page_count):
            return (None, None) if need_position else None
        last_h = self.page_images[last_pn].size[1]
        poss.append(
            (
                [last_pn],
                last[1],
                last[2],
                min(last_h, last[4] + GAP),
                min(last_h, last[4] + 120),
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
            if not (0 <= pns[0] < page_count):
                continue
            base_img = self.page_images[pns[0]]
            x0, y0, x1, y1 = (
                int(left),
                int(top),
                int(right),
                int(min(bottom, base_img.size[1])),
            )
            if x0 > x1:
                x0, x1 = x1, x0
            if y0 > y1:
                y0, y1 = y1, y0
            if x1 <= x0 or y1 <= y0:
                continue
            imgs.append(base_img.crop((x0, y0, x1, y1)))
            if 0 < ii < len(poss) - 1:
                positions.append((pns[0] + self.page_from, x0, x1, y0, y1))
            bottom -= base_img.size[1]
            for pn in pns[1:]:
                if not (0 <= pn < page_count):
                    continue
                page = self.page_images[pn]
                x0, y0, x1, y1 = (
                    int(left),
                    0,
                    int(right),
                    int(min(bottom, page.size[1])),
                )
                if x0 > x1:
                    x0, x1 = x1, x0
                if y0 > y1:
                    y0, y1 = y1, y0
                if x1 <= x0 or y1 <= y0:
                    bottom -= page.size[1]
                    continue
                imgs.append(page.crop((x0, y0, x1, y1)))
                if 0 < ii < len(poss) - 1:
                    positions.append((pn + self.page_from, x0, x1, y0, y1))
                bottom -= page.size[1]

        if not imgs:
            return (None, None) if need_position else None

        height = sum(img.size[1] + GAP for img in imgs)
        width = int(np.max([i.size[0] for i in imgs]))
        pic = Image.new("RGB", (width, int(height)), (245, 245, 245))
        offset = 0
        for ii, img in enumerate(imgs):
            if ii == 0 or ii + 1 == len(imgs):
                img = img.convert("RGBA")
                overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
                overlay.putalpha(128)
                img = Image.alpha_composite(img, overlay).convert("RGB")
            pic.paste(img, (0, int(offset)))
            offset += img.size[1] + GAP

        return (pic, positions) if need_position else pic

    # ---------------------------------------------------------------------
    # SoMark JSON -> RAGFlow sections
    # ---------------------------------------------------------------------
    def _resolve_internal_type(self, block_type: str) -> Optional[str]:
        """Resolve the RAGFlow internal layout type, or ``None`` to discard.

        Header/footer obey ``keep_header_footer``; cate/cate_item/blank always drop.
        Unknown SoMark types fall back to ``text`` to avoid silent loss.
        """
        if block_type in ALWAYS_DISCARDED:
            return None
        if block_type == SoMarkBlockType.HEADER or block_type == SoMarkBlockType.FOOTER:
            return "text" if self.feature_config.get("keep_header_footer") else None
        return SOMARK_TYPE_TO_RAGFLOW.get(block_type, "text")

    @staticmethod
    def _block_text(block: dict, internal_type: str) -> str:
        """Extract the textual payload for a block.

        ``image``-typed blocks contribute no text (image only); everything else
        falls back to ``content``. For ``title`` blocks with title_level we prepend
        markdown-style hashes so downstream tokenization preserves hierarchy.
        """
        if internal_type == "image":
            return ""
        content = (block.get("content") or "").strip()
        if block.get("type") == SoMarkBlockType.TITLE.value:
            level = block.get("title_level")
            if isinstance(level, int) and 1 <= level <= 6:
                content = ("#" * level) + " " + content
        return content

    def _transfer_to_sections(self, pages: list[dict], parse_method: Optional[str] = None) -> list[tuple]:
        # MinerU contract: manual/pipeline callers (the rag/flow DAG) want typed
        # 3-tuples ``(text, layout_type, line_tag)`` so the consumer can set
        # box["layout_type"] and crop via the separate position field; every other
        # caller (naive.py standard chunking) wants 2-tuples ``(text, line_tag)``
        # that naive_merge consumes directly.
        typed = parse_method in {"manual", "pipeline"}
        sections: list[tuple] = []
        image_seq = 0
        for page in pages or []:
            page_idx = page.get("page_num", 0)
            page_size = page.get("page_size") or {}
            for block in page.get("blocks") or []:
                btype = block.get("type")
                internal = self._resolve_internal_type(btype)
                if internal is None:
                    continue
                # Inject page_idx so _line_tag can compute coords.
                bbox = block.get("bbox")
                tag_input = {
                    "page_idx": page_idx,
                    "bbox": bbox,
                    "page_size": page_size,
                }
                if internal == "image":
                    # Align with MinerU: the figure is recovered by cropping the
                    # locally rendered page region via crop(), not from img_url.
                    if not bbox or len(bbox) != 4:
                        continue  # no geometry -> nothing to crop
                    line_tag = self._line_tag(tag_input)
                    image_seq += 1
                    caption = (block.get("content") or "").strip()
                    label = caption or f"{btype} {image_seq}"
                    if typed:
                        # 3-tuple: layout_type + a real (separate) position field;
                        # keep the caption in text so figure understanding can be
                        # embedded and retrieved while crop() still uses line_tag.
                        sections.append((label, internal, line_tag))
                    else:
                        # 2-tuple (naive.py): the chunk id is hash(content + doc_id),
                        # so an empty-text image chunk would collide across every
                        # figure and all but one would be dropped on upsert. Give it a
                        # non-empty, unique caption (SoMark image-understanding text if
                        # present, else "<type> <seq>") so each figure gets a distinct
                        # id. The tag is appended so tokenize_chunks() -> crop() can
                        # still recover the figure; remove_tag() then strips it, leaving
                        # the caption as the chunk text.
                        sections.append((f"{label}{line_tag}", ""))
                    continue
                text = self._block_text(block, internal)
                if not text:
                    continue
                line_tag = self._line_tag(tag_input)
                if typed:
                    sections.append((text, internal, line_tag))
                else:
                    sections.append((text, line_tag))

        return sections

    def _transfer_to_tables(self, pages: list[dict]) -> list:
        # Tables are inlined as HTML in section text; no separate table extraction.
        return []

    # ---------------------------------------------------------------------
    # Public entry point
    # ---------------------------------------------------------------------
    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes | None = None,
        callback: Optional[Callable] = None,
        parse_method: Optional[str] = None,
        **kwargs,
    ) -> tuple:
        self.outlines = extract_pdf_outlines(binary if binary is not None else filepath)

        # Normalize input to a real PDF file on disk.
        temp_pdf: Optional[Path] = None
        if binary:
            tmp_dir = Path(tempfile.mkdtemp(prefix="somark_bin_pdf_"))
            file_name = Path(filepath).stem.replace(" ", "") + ".pdf"
            temp_pdf = tmp_dir / file_name
            with open(temp_pdf, "wb") as f:
                f.write(binary.getvalue() if isinstance(binary, BytesIO) else binary)
            pdf_path = temp_pdf
        else:
            pdf_path = Path(filepath)
            if not pdf_path.exists():
                if callback:
                    callback(-1, f"[SoMark] PDF not found: {pdf_path}")
                raise FileNotFoundError(f"[SoMark] PDF not found: {pdf_path}")

        if callback:
            callback(0.10, f"[SoMark] using {pdf_path.name}")

        # Render page images locally so _line_tag/crop can map bbox to pixels.
        self.__images__(pdf_path, zoomin=1)

        try:
            ok, reason = self.check_installation()
            if not ok:
                raise SoMarkAPIError(reason)

            task_id = self._submit_task(pdf_path, callback=callback)
            result = self._poll_task(task_id, callback=callback)

            outputs = (result or {}).get("outputs") or {}
            json_payload = outputs.get("json") or {}
            pages = json_payload.get("pages") or []
            if not pages:
                self.logger.warning("[SoMark] empty pages in response; nothing to chunk")

            if callback:
                callback(
                    0.75,
                    f"[SoMark] parsed {sum(len(p.get('blocks') or []) for p in pages)} blocks across {len(pages)} pages",
                )

            sections = self._transfer_to_sections(pages, parse_method)
            tables = self._transfer_to_tables(pages)
            return sections, tables
        finally:
            if temp_pdf and temp_pdf.exists():
                try:
                    temp_pdf.unlink()
                    temp_pdf.parent.rmdir()
                except Exception:
                    pass


if __name__ == "__main__":
    parser = SoMarkParser(
        base_url=os.environ.get("SOMARK_BASE_URL", "https://somark.tech/api/v1"),
        api_key=os.environ.get("SOMARK_API_KEY", ""),
    )
    ok, reason = parser.check_installation()
    print("SoMark available:", ok, reason)
