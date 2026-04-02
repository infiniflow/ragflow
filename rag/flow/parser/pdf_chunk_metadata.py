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
import io
import logging
import sys
from copy import deepcopy
from functools import partial

import pdfplumber
from PIL import Image

from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from common import settings
from common.misc_utils import get_uuid
from deepdoc.parser.pdf_parser import LOCK_KEY_pdfplumber, RAGFlowPdfParser
from rag.utils.base64_image import image2id

PDF_PREVIEW_GAP = 6
PDF_PREVIEW_CONTEXT = 120
PDF_PREVIEW_ZOOM = 3
PDF_POSITIONS_KEY = "_pdf_positions"


def _extract_raw_positions(item):
    positions = item.get(PDF_POSITIONS_KEY)
    if isinstance(positions, list):
        return deepcopy(positions)

    positions = item.get("positions")
    if isinstance(positions, list):
        return deepcopy(positions)

    position_tag = item.get("position_tag")
    if isinstance(position_tag, str) and position_tag:
        return [[pos[0][-1], *pos[1:]] for pos in RAGFlowPdfParser.extract_positions(position_tag)]

    position_int = item.get("position_int")
    if isinstance(position_int, list):
        return [
            list(pos)
            for pos in position_int
            if isinstance(pos, (list, tuple)) and len(pos) >= 5
        ]

    if item.get("page_number") is not None and all(
        item.get(key) is not None for key in ["x0", "x1", "top", "bottom"]
    ):
        return [[item["page_number"], item["x0"], item["x1"], item["top"], item["bottom"]]]

    return []


def extract_pdf_positions(item):
    # Parser-owned canonical PDF coordinate shape:
    # [[page_number, left, right, top, bottom], ...]
    if not isinstance(item, dict):
        return []

    positions = _extract_raw_positions(item)
    ref_page_number = item.get("page_number")
    ref_page_number = int(ref_page_number) if isinstance(ref_page_number, (int, float)) else None
    if ref_page_number is not None and ref_page_number <= 0:
        ref_page_number += 1

    normalized_positions = []
    for pos in positions:
        if not isinstance(pos, (list, tuple)) or len(pos) < 5:
            continue

        page_number = pos[0][-1] if isinstance(pos[0], list) else pos[0]
        try:
            page_number = int(page_number)
            if ref_page_number is not None and page_number == ref_page_number - 1:
                page_number = ref_page_number
            elif page_number <= 0:
                page_number += 1

            normalized_positions.append(
                [page_number, float(pos[1]), float(pos[2]), float(pos[3]), float(pos[4])]
            )
        except (TypeError, ValueError):
            continue

    return normalized_positions


def normalize_pdf_item_metadata(item):
    if not isinstance(item, dict):
        return item

    positions = extract_pdf_positions(item)
    if positions:
        item[PDF_POSITIONS_KEY] = positions
    else:
        item.pop(PDF_POSITIONS_KEY, None)
    return item


def normalize_pdf_items_metadata(items):
    if not isinstance(items, list):
        return items
    for item in items:
        normalize_pdf_item_metadata(item)
    return items


def merge_pdf_positions(sources):
    merged = []
    seen = set()
    for source in sources or []:
        if isinstance(source, dict):
            positions = extract_pdf_positions(source)
        elif isinstance(source, list):
            positions = source
        else:
            positions = []

        for pos in positions:
            if not isinstance(pos, (list, tuple)) or len(pos) < 5:
                continue
            key = tuple(pos[:5])
            if key in seen:
                continue
            seen.add(key)
            merged.append(list(pos[:5]))

    merged.sort(key=lambda item: (item[0], item[3], item[1]))
    return merged


def build_pdf_position_fields(positions):
    position_int = []
    page_num_int = []
    top_int = []
    for pos in positions or []:
        if not isinstance(pos, (list, tuple)) or len(pos) < 5:
            continue
        try:
            page_no = int(pos[0])
            left = int(pos[1])
            right = int(pos[2])
            top = int(pos[3])
            bottom = int(pos[4])
        except (TypeError, ValueError):
            continue

        position_int.append((page_no, left, right, top, bottom))
        page_num_int.append(page_no)
        top_int.append(top)

    return {
        "position_int": deepcopy(position_int),
        "page_num_int": deepcopy(page_num_int),
        "top_int": deepcopy(top_int),
    }


def finalize_pdf_chunk(chunk):
    if not isinstance(chunk, dict):
        return chunk

    positions = extract_pdf_positions(chunk)
    if positions:
        chunk.update(build_pdf_position_fields(positions))
    chunk.pop(PDF_POSITIONS_KEY, None)
    return chunk


def _fetch_source_blob(from_upstream, canvas):
    if canvas._doc_id:
        bucket, name = File2DocumentService.get_storage_address(doc_id=canvas._doc_id)
        return settings.STORAGE_IMPL.get(bucket, name)
    if from_upstream.file:
        return FileService.get_blob(from_upstream.file["created_by"], from_upstream.file["id"])
    return None


def _load_pdf_page_images(blob, zoom=PDF_PREVIEW_ZOOM):
    with sys.modules[LOCK_KEY_pdfplumber]:
        with pdfplumber.open(io.BytesIO(blob)) as pdf:
            return [
                page.to_image(resolution=72 * zoom, antialias=True).annotated
                for page in pdf.pages
            ]


def _crop_pdf_preview(page_images, positions, zoom=PDF_PREVIEW_ZOOM):
    if not page_images or not positions:
        return None

    normalized_positions = []
    for pos in sorted(positions, key=lambda item: (item[0], item[3], item[1])):
        if len(pos) < 5:
            continue

        page_idx = int(pos[0]) - 1
        if not (0 <= page_idx < len(page_images)):
            continue

        left, right, top, bottom = map(float, pos[1:5])
        if right <= left or bottom <= top:
            continue
        normalized_positions.append((page_idx, left, right, top, bottom))

    if not normalized_positions:
        return None

    max_width = max(right - left for _, left, right, _, _ in normalized_positions)
    first_page, first_left, _, first_top, _ = normalized_positions[0]
    last_page, last_left, _, _, last_bottom = normalized_positions[-1]
    page_height = lambda idx: page_images[idx].size[1] / zoom

    crop_positions = [
        (
            [first_page],
            first_left,
            first_left + max_width,
            max(0, first_top - PDF_PREVIEW_CONTEXT),
            max(first_top - PDF_PREVIEW_GAP, 0),
        )
    ]
    crop_positions.extend(
        [
            ([page_idx], left, right, top, bottom)
            for page_idx, left, right, top, bottom in normalized_positions
        ]
    )
    crop_positions.append(
        (
            [last_page],
            last_left,
            last_left + max_width,
            min(page_height(last_page), last_bottom + PDF_PREVIEW_GAP),
            min(page_height(last_page), last_bottom + PDF_PREVIEW_CONTEXT),
        )
    )

    imgs = []
    for idx, (pages, left, right, top, bottom) in enumerate(crop_positions):
        page_idx = pages[0]
        effective_right = (
            left + max_width if idx in {0, len(crop_positions) - 1} else max(left + 10, right)
        )
        imgs.append(
            page_images[page_idx].crop(
                (
                    left * zoom,
                    top * zoom,
                    effective_right * zoom,
                    min(bottom * zoom, page_images[page_idx].size[1]),
                )
            )
        )

    canvas_height = int(sum(img.size[1] for img in imgs) + PDF_PREVIEW_GAP * len(imgs))
    canvas_width = int(max(img.size[0] for img in imgs))
    preview = Image.new("RGB", (canvas_width, canvas_height), (245, 245, 245))

    height = 0
    for idx, img in enumerate(imgs):
        if idx in {0, len(imgs) - 1}:
            # Dim the extra context so the highlighted body stays visually distinct.
            img = img.convert("RGBA")
            overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
            overlay.putalpha(128)
            img = Image.alpha_composite(img, overlay).convert("RGB")

        preview.paste(img, (0, height))
        height += img.size[1] + PDF_PREVIEW_GAP

    return preview


async def restore_pdf_text_previews(chunks, from_upstream, canvas):
    if not chunks or not str(from_upstream.name).lower().endswith(".pdf"):
        return

    text_chunks = [
        chunk
        for chunk in chunks
        if chunk.get("doc_type_kwd", "text") == "text" and extract_pdf_positions(chunk)
    ]
    if not text_chunks:
        return

    blob = _fetch_source_blob(from_upstream, canvas)
    if not blob:
        return

    try:
        page_images = _load_pdf_page_images(blob)
    except Exception as e:
        logging.warning(f"Failed to load PDF page images for chunk preview restore: {e}")
        return

    preview_cache = {}
    storage_put = partial(settings.STORAGE_IMPL.put, tenant_id=canvas._tenant_id)
    for chunk in text_chunks:
        preview_positions = extract_pdf_positions(chunk)
        positions_key = tuple(tuple(pos[:5]) for pos in preview_positions)
        if not positions_key:
            continue
        if positions_key in preview_cache:
            chunk["img_id"] = preview_cache[positions_key]
            continue

        preview = _crop_pdf_preview(page_images, preview_positions)
        if not preview:
            continue

        chunk["image"] = preview
        await image2id(chunk, storage_put, get_uuid())
        if chunk.get("img_id"):
            preview_cache[positions_key] = chunk["img_id"]
