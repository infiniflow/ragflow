#
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
"""VLM enhancement for naive DeepDOC PDF chunking (built-in KB parsers)."""

from __future__ import annotations


def naive_deepdoc_vision_available(tenant_id, parser_config=None) -> bool:
    """Return whether tenant VLM config (explicit or default) can run figure enhancement."""
    if not tenant_id:
        return False
    parser_config = parser_config or {}
    vlm_conf = parser_config.get("vlm") or {}
    from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type, resolve_model_config
    from common.constants import LLMType

    try:
        resolve_model_config(tenant_id, LLMType.VISION, vlm_conf["llm_id"])
        return True
    except Exception:
        try:
            get_tenant_default_model_by_type(tenant_id, LLMType.VISION)
            return True
        except Exception:
            return False


def _bbox_positions_for_naive_tables(box):
    positions = []
    for pos in box.get("positions") or []:
        if not isinstance(pos, (list, tuple)) or len(pos) < 5:
            continue
        try:
            page_number = int(pos[0])
            positions.append((page_number - 1, float(pos[1]), float(pos[2]), float(pos[3]), float(pos[4])))
        except (TypeError, ValueError):
            continue
    return positions


def _apply_document_vertical_coords(box, page_cum_height):
    """Convert page-local top/bottom from pdfplumber into DeepDOC cumulative coordinates."""
    if not box.get("_embedded_supplement"):
        return box
    pn = box.get("page_number")
    if pn is None or page_cum_height is None:
        return box
    try:
        idx = int(pn) - 1
    except (TypeError, ValueError):
        return box
    if idx < 0 or idx >= len(page_cum_height) - 1:
        return box
    offset = float(page_cum_height[idx])
    updated = dict(box)
    for key in ("top", "bottom"):
        if updated.get(key) is not None:
            updated[key] = float(updated[key]) + offset
    updated.pop("_embedded_supplement", None)
    return updated


def merge_vlm_enhanced_bboxes_into_naive_pdf(sections, tables, bboxes, pdf_parser, zoomin=3):
    """Merge VLM-enhanced bbox text back into naive PDF sections and tables."""
    sections = list(sections or [])
    tables = list(tables or [])
    sections_was_empty = not sections

    for box in bboxes:
        if box.get("image") is None:
            continue

        enhanced_text = (box.get("text") or "").strip()
        try:
            tag = box.get("position_tag") or pdf_parser._line_tag(box, zoomin)
        except Exception:
            tag = ""

        matched = False
        for i, (sec_text, sec_tag) in enumerate(sections):
            if tag and sec_tag and str(tag).strip() == str(sec_tag).strip():
                sections[i] = (enhanced_text or sec_text, sec_tag)
                matched = True
                break
            st = (sec_text or "").strip()
            if st and enhanced_text and (enhanced_text.startswith(st) or st in enhanced_text):
                sections[i] = (enhanced_text, sec_tag)
                matched = True
                break

        if matched:
            continue

        if sections_was_empty:
            continue

        poss = _bbox_positions_for_naive_tables(box)
        desc = [enhanced_text] if enhanced_text else ["figure"]
        tables.append(((box["image"], desc), poss))

    if not sections:
        for box in bboxes:
            txt = (box.get("text") or "").strip()
            if not txt and box.get("image") is None:
                continue
            try:
                tag = box.get("position_tag") or pdf_parser._line_tag(box, zoomin)
            except Exception:
                tag = ""
            sections.append((txt or " ", tag))

    return sections, tables


def enhance_naive_deepdoc_pdf_media(
    sections,
    tables,
    binary,
    pdf_parser,
    from_page=0,
    to_page=100000,
    callback=None,
    lang="English",
    **kwargs,
):
    """Run VLM figure enhancement for naive DeepDOC PDF chunks and merge results."""
    tenant_id = kwargs.get("tenant_id")
    if not tenant_id or not binary:
        return sections, tables

    parser_config = kwargs.get("parser_config") or {}
    if not naive_deepdoc_vision_available(tenant_id, parser_config):
        return sections, tables

    from deepdoc.parser.pdf_parser import RAGFlowPdfParser
    from rag.flow.parser.pdf_chunk_metadata import supplement_deepdoc_bboxes_with_embedded_images
    from rag.flow.parser.utils import enhance_media_sections_with_vision

    vlm_conf = parser_config.get("vlm")

    zoomin = 3
    bbox_parser = RAGFlowPdfParser()
    bboxes = bbox_parser.parse_into_bboxes(
        binary,
        callback=callback,
        zoomin=zoomin,
        from_page=from_page,
        to_page=to_page,
    )
    bboxes = supplement_deepdoc_bboxes_with_embedded_images(
        binary,
        bboxes,
        from_page=from_page,
        to_page=to_page,
    )
    page_cum_height = getattr(bbox_parser, "page_cum_height", None)
    bboxes = [_apply_document_vertical_coords(box, page_cum_height) for box in bboxes]

    for box in bboxes:
        if box.get("image") is not None:
            box["doc_type_kwd"] = "image"

    if not bboxes:
        return sections, tables

    enhance_media_sections_with_vision(
        bboxes,
        tenant_id,
        vlm_conf,
        callback=callback,
        lang=lang,
    )

    return merge_vlm_enhanced_bboxes_into_naive_pdf(sections, tables, bboxes, bbox_parser, zoomin=zoomin)
