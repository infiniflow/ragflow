from __future__ import annotations

"""MonkeyDoc PDF parser (Phase 1 minimal callable pipeline).

This parser mirrors the DeepDoc PDF parser public API but, in Phase 1,
performs only PDF rasterization and returns empty outputs. It enables
incremental wiring and unit testing without invoking any ML components.
"""

import logging
import re
from statistics import median
from io import BytesIO
from typing import Any, List, Tuple
import time

import pdfplumber
from PIL import Image
import gc
import torch  # type: ignore

from monkeydoc.service import MonkeyOCRService
from monkeydoc.logger import get_monkeyocr_logger
import os
from monkeyocr.magic_pdf.model.custom_model import MonkeyOCR  # type: ignore
from pathlib import Path


logger = get_monkeyocr_logger("CEDD OCR")


class MonkeyDocPdfParser:
    """DeepDoc-compatible PDF parser backed by MonkeyOCR (Phase 1).

    Public API compatibility:
    - __call__(fnm, need_image=True, zoomin=3, return_html=False) -> (sections, tbls_or_figs)
    - remove_tag(txt)
    - crop(text, ZM=3, need_position=False)
    """

    def __init__(self) -> None:
        """
        Initialize parser state for the current document lifecycle.
        """

        self.page_images: List[Image.Image] = []

    def _render_pages(self, fnm: str | bytes, zoomin: int = 3, from_page: int = 0, to_page: int = 100000) -> None:
        """Render PDF pages and cache text-layer words.

        Parameters
        ----------
        fnm: str | bytes
            Path to a PDF or in-memory bytes.
        zoomin: int
            Scale factor, DPI will be 72 * zoomin.
        """

        self.page_images = []
        self.page_words: List[List[dict]] = []
        try:
            with (pdfplumber.open(fnm) if isinstance(fnm, str) else pdfplumber.open(BytesIO(fnm))) as pdf:
                for page in pdf.pages[from_page:to_page]:
                    img = page.to_image(resolution=72 * zoomin, antialias=True).annotated
                    self.page_images.append(img)
                    try:
                        words = page.extract_words(x_tolerance=1, y_tolerance=1, keep_blank_chars=False, use_text_flow=True)
                    except Exception:
                        words = []
                    self.page_words.append(words)
        except Exception:
            logger.exception("MonkeyDocPdfParser _render_pages")
            self.page_images = []
            self.page_words = []

    # API-compatible __call__
    def __call__(self, fnm: str | bytes, need_image: bool = True, zoomin: int = 3, return_html: bool = False,
                 omr_enabled: bool = True, omr_min_area: float = 500.0, omr_max_aspect: float = 10.0, from_page: int = 0, to_page: int = 100000,
                 callback=None) -> Tuple[List[Tuple[str, str]], List[Any]]:
        """Parse a PDF and return DeepDoc-compatible outputs.

        Returns
        -------
        sections: list[tuple[str, str]]
            List of text sections with inline position tags. Phase 1 returns [].
        tables_or_figures: list[Any]
            List of table/figure tuples. Phase 1 returns [].
        """

        t0 = time.time()
        logger.info("[CEDD OCR] Rendering pages start")
        self._render_pages(fnm, zoomin=zoomin, from_page=from_page, to_page=to_page)
        try:
            if callback:
                callback(0.15, "CEDD OCR: pages rendered")
        except Exception:
            pass
        logger.info("[CEDD OCR] Rendering pages done: %d pages in %.1f ms", len(self.page_images), (time.time()-t0)*1000)

        mdl = None
        try:
            if MonkeyOCR is not None:
                mdl = MonkeyOCR(str(Path(__file__).resolve().parents[2] / "monkeyocr" / "model_configs.yaml"))
            t1 = time.time()
            logger.info("[CEDD OCR] Layout detection start")
            self.page_layout: List[List[dict]] = MonkeyOCRService.detect_layout(
                self.page_images, zoomin=zoomin, model=mdl
            )
            logger.info("[CEDD OCR] Layout detection done in %.1f ms", (time.time()-t1)*1000)
            try:
                if callback:
                    callback(0.25, "CEDD OCR: layout detected")
            except Exception:
                pass
            # Stats by label
            label_stats = {}
            for blocks in self.page_layout:
                for b in blocks:
                    label_stats[b.get("type", "text")] = label_stats.get(b.get("type", "text"), 0) + 1
            logger.info("[CEDD OCR] Layout counts: %s", label_stats)
            # Rescale layout blocks from pixel coords to PDF units using per-page sizes
            try:
                with (pdfplumber.open(fnm) if isinstance(fnm, str) else pdfplumber.open(BytesIO(fnm))) as pdf:
                    page_sizes = [(p.width, p.height) for p in pdf.pages]
            except Exception:
                page_sizes = [(self.page_images[i].width / max(zoomin, 1), self.page_images[i].height / max(zoomin, 1)) for i in range(len(self.page_images))]
            for idx, blocks in enumerate(self.page_layout):
                img_w, img_h = self.page_images[idx].width, self.page_images[idx].height
                pdf_w, pdf_h = page_sizes[idx] if idx < len(page_sizes) else (img_w / max(zoomin, 1), img_h / max(zoomin, 1))
                sx = (img_w / max(pdf_w, 1e-6))
                sy = (img_h / max(pdf_h, 1e-6))
                for b in blocks:
                    if b.get("unit") == "px":
                        b["x0"], b["x1"] = float(b["x0"]) / sx, float(b["x1"]) / sx
                        b["top"], b["bottom"] = float(b["top"]) / sy, float(b["bottom"]) / sy
                        b.pop("unit", None)
        except Exception:
            logger.exception("MonkeyDocPdfParser detect_layout")
            self.page_layout = [[] for _ in range(len(self.page_images))]

        # Phase 3: Text extraction via text-layer first, with minimal OCR fallback
        self.boxes: List[dict] = []
        try:
            logger.info("[CEDD OCR] Text-layer extraction start")
            text_like_labels = {"text", "title"}
            ocr_fallback_rois: List[tuple[int, int, dict]] = []  # (page_index, layoutno, blk)

            def iou(a, b) -> float:
                ax0, ay0, ax1, ay1 = float(a["x0"]), float(a["top"]), float(a["x1"]), float(a["bottom"]) 
                bx0, by0, bx1, by1 = float(b["x0"]), float(b["top"]), float(b["x1"]), float(b["bottom"]) 
                ix0, iy0 = max(ax0, bx0), max(ay0, by0)
                ix1, iy1 = min(ax1, bx1), min(ay1, by1)
                iw, ih = max(0.0, ix1 - ix0), max(0.0, iy1 - iy0)
                inter = iw * ih
                if inter <= 0:
                    return 0.0
                a_area = max(0.0, (ax1 - ax0) * (ay1 - ay0))
                b_area = max(0.0, (bx1 - bx0) * (by1 - by0))
                return inter / max(min(a_area, b_area), 1e-6)

            total_pages = max(len(self.page_layout), 1)
            for page_index, blocks in enumerate(self.page_layout):
                try:
                    words = self.page_words[page_index]
                except Exception:
                    words = []
                for layoutno, blk in enumerate(blocks):
                    if blk.get("type") not in text_like_labels:
                        continue
                    x0, x1 = float(blk["x0"]), float(blk["x1"])
                    top, bottom = float(blk["top"]), float(blk["bottom"]) 
                    blk_box = {"x0": x0, "x1": x1, "top": top, "bottom": bottom}
                    inside: List[dict] = []
                    for w in words or []:
                        try:
                            wx0 = float(w.get("x0", 0.0))
                            wx1 = float(w.get("x1", 0.0))
                            wt = float(w.get("top", 0.0))
                            wb = float(w.get("bottom", 0.0))
                            cx = 0.5 * (wx0 + wx1)
                            cy = 0.5 * (wt + wb)
                            # Consider a word belonging to a block if its center lies inside the block
                            if x0 <= cx <= x1 and top <= cy <= bottom:
                                inside.append(w)
                        except Exception:
                            continue

                    # If we have text-layer coverage, assemble lines
                    if inside:
                        inside.sort(key=lambda w: (float(w.get("top", 0.0)), float(w.get("x0", 0.0))))
                        # Group by near-equal top to form lines
                        lines: List[List[dict]] = []
                        # Estimate a robust vertical threshold using median word heights
                        try:
                            heights = [max(0.0, float(w.get("bottom", 0.0)) - float(w.get("top", 0.0))) for w in inside if w is not None]
                            med_h = median([h for h in heights if h > 0]) if heights else 0.0
                        except Exception:
                            med_h = 0.0
                        for wd in inside:
                            if not lines:
                                lines.append([wd])
                                continue
                            last_line = lines[-1]
                            y_gap = abs(float(wd.get("top", 0.0)) - float(last_line[0].get("top", 0.0)))
                            # Threshold: min of (median-based) and (current-line-based) caps
                            h_ref = float(last_line[0].get("bottom", 0.0)) - float(last_line[0].get("top", 0.0))
                            thr_med = 0.4 * max(med_h, 1.0)
                            thr_cur = 0.5 * max(h_ref, 1.0)
                            thr = max(1.5, min(thr_med, thr_cur))
                            if y_gap <= thr:
                                last_line.append(wd)
                            else:
                                lines.append([wd])
                        # Join words with spaces per line; then join lines with newline for titles
                        text_lines: List[str] = []
                        for ln in lines:
                            ln.sort(key=lambda w: float(w.get("x0", 0.0)))
                            text_lines.append(" ".join([str(w.get("text", "")).strip() for w in ln if str(w.get("text", "")).strip()]))
                        merged_text = ("\n" if blk.get("type") == "title" else " ").join([t for t in text_lines if t])
                        if merged_text.strip():
                            # Use tight union bounds of the contributing words for better tag alignment
                            try:
                                ux0 = min(float(w.get("x0", x0)) for w in inside)
                                ux1 = max(float(w.get("x1", x1)) for w in inside)
                                utop = min(float(w.get("top", top)) for w in inside)
                                ubot = max(float(w.get("bottom", bottom)) for w in inside)
                                # Small padding to avoid clipping
                                pad = 0.5
                                ux0 = max(0.0, ux0 - pad)
                                ux1 = max(ux0 + 1e-3, ux1 + pad)
                                utop = max(0.0, utop - pad)
                                ubot = max(utop + 1e-3, ubot + pad)
                            except Exception:
                                ux0, ux1, utop, ubot = x0, x1, top, bottom

                            new_box = {
                                "x0": ux0,
                                "x1": ux1,
                                "top": utop,
                                "bottom": ubot,
                                "text": merged_text.strip(),
                                "layout_type": blk.get("type", "text"),
                                "page_number": page_index + 1,
                                "layoutno": layoutno,
                            }

                            # Deduplicate: skip if same text with highly overlapping box already exists on this page
                            is_dup = False
                            try:
                                for ob in self.boxes:
                                    if int(ob.get("page_number", -1)) != new_box["page_number"]:
                                        continue
                                    if str(ob.get("text", "")).strip() != new_box["text"]:
                                        continue
                                    if iou(ob, new_box) >= 0.8:
                                        is_dup = True
                                        break
                            except Exception:
                                is_dup = False
                            if not is_dup:
                                self.boxes.append(new_box)
                            continue

                    # No words matched → schedule OCR fallback
                    ocr_fallback_rois.append((page_index, layoutno, blk))

            try:
                if callback:
                    # progress proportionally with pages processed
                    callback(min(0.55, 0.25 + 0.3 * (page_index + 1) / total_pages), f"CEDD OCR: extracted text for page {page_index + 1}/{total_pages}")
            except Exception:
                pass

            logger.info("[MonkeyDoc] Text-layer extraction done: %d boxes, %d OCR fallbacks", len(self.boxes), len(ocr_fallback_rois))

            # OCR fallback
            if ocr_fallback_rois:
                crops: List[Image.Image] = []
                meta: List[tuple[int, int, dict]] = []
                for page_index, layoutno, blk in ocr_fallback_rois:
                    x0, x1 = float(blk["x0"]), float(blk["x1"])
                    top, bottom = float(blk["top"]), float(blk["bottom"]) 
                    l = max(0, int(x0 * zoomin))
                    r = max(l + 1, int(x1 * zoomin))
                    t = max(0, int(top * zoomin))
                    b = max(t + 1, int(bottom * zoomin))
                    try:
                        if blk.get("type") == "figure":
                            continue # _extract_tables_figures will handle this later
                        else:
                            crops.append(self.page_images[page_index].crop((l, t, r, b)))
                        meta.append((page_index, layoutno, blk))
                    except Exception:
                        continue
                if crops:
                    try:
                        texts = MonkeyOCRService.ocr_text(
                            crops,
                            instruction="Extract the readable text in natural reading order from this region.",
                            model=None,
                        )
                    except Exception:
                        texts = [""] * len(crops)
                    try:
                        if callback:
                            callback(0.65, f"CEDD OCR: OCR fallback processed {len(crops)} regions")
                    except Exception:
                        pass
                    for txt, (page_index, layoutno, blk) in zip(texts, meta):
                        if not isinstance(txt, str):
                            txt = str(txt)
                        txt = txt.strip()
                        if not txt:
                            continue
                        new_box = {
                            "x0": float(blk["x0"]),
                            "x1": float(blk["x1"]),
                            "top": float(blk["top"]),
                            "bottom": float(blk["bottom"]),
                            "text": txt,
                            "layout_type": blk.get("type", "text"),
                            "page_number": page_index + 1,
                            "layoutno": layoutno,
                        }
                        # Deduplicate OCR fallback results as well
                        is_dup = False
                        try:
                            for ob in self.boxes:
                                if int(ob.get("page_number", -1)) != new_box["page_number"]:
                                    continue
                                if str(ob.get("text", "")).strip() != new_box["text"]:
                                    continue
                                if iou(ob, new_box) >= 0.8:
                                    is_dup = True
                                    break
                        except Exception:
                            is_dup = False
                        if not is_dup:
                            self.boxes.append(new_box)
                # Release temporary crops
                del crops, meta, texts
            logger.info("[CEDD OCR] OCR fallback done: total boxes %d", len(self.boxes))
        except Exception:
            logger.exception("MonkeyDocPdfParser text extraction phase")
            self.boxes = []

        # Tables/Figures/OMR extraction
        tbls_or_figs: List[Any] = []
        try:
            logger.info("[CEDD OCR] Tables/Figures/OMR extraction start")
            tbls_or_figs = self._extract_tables_figures(
                need_image=need_image,
                zoomin=zoomin,
                return_html=return_html,
                omr_enabled=omr_enabled,
                omr_min_area=omr_min_area,
                omr_max_aspect=omr_max_aspect,
            )
            logger.info("[CEDD OCR] Tables/Figures/OMR extraction done: %d entries", len(tbls_or_figs))
            try:
                if callback:
                    callback(0.75, "CEDD OCR: tables/figures processed")
            except Exception:
                pass
        except Exception:
            logger.exception("MonkeyDocPdfParser tables/figures extraction")

        # Build sections with inline position tags from all text boxes
        try:
            # Final deduplication using Non-Maximum Suppression on same-text boxes per page
            try:
                by_page_text = {}
                for b in list(self.boxes):
                    key = (int(b.get("page_number", 0)), str(b.get("text", "")).strip())
                    if not key[1]:
                        continue
                    by_page_text.setdefault(key, []).append(b)
                pruned_boxes: List[dict] = []
                for (_, _), arr in by_page_text.items():
                    arr_sorted = sorted(arr, key=lambda d: (float(d.get("bottom", 0.0)) - float(d.get("top", 0.0))) * (float(d.get("x1", 0.0)) - float(d.get("x0", 0.0))), reverse=True)
                    kept: List[dict] = []
                    for cand in arr_sorted:
                        overlapped = False
                        for kept_box in kept:
                            try:
                                if iou(kept_box, cand) >= 0.7:
                                    overlapped = True
                                    break
                            except Exception:
                                continue
                        if not overlapped:
                            kept.append(cand)
                    pruned_boxes.extend(kept)
                self.boxes = pruned_boxes
            except Exception:
                pass

            sections: List[Tuple[str, str]] = self._build_sections_from_boxes(zoomin=zoomin, mdl=mdl)
            from monkeydoc.utils import merge_and_dedup
            sections = merge_and_dedup(sections, y_tol=2.0, min_horiz_overlap=0.1, cover_ratio=0.8)
            try:
                if callback:
                    callback(0.85, f"CEDD OCR: built {len(sections)} sections")
            except Exception:
                pass
        except Exception:
            logger.exception("MonkeyDocPdfParser build sections")
            sections = []

        try:
            if callback:
                callback(0.9, "CEDD OCR: parser returning results")
        except Exception:
            pass
        return sections, tbls_or_figs

    def _line_tag(self, bx: dict) -> str:
        """Generate DeepDoc-compatible inline position tag for a box."""
        try:
            pn = int(bx.get("page_number", 1))
            return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format(
                pn, float(bx["x0"]), float(bx["x1"]), float(bx["top"]), float(bx["bottom"])
            )
        except Exception:
            return ""

    @staticmethod
    def remove_tag(txt: str) -> str:
        """Strip inline position tags from a text line."""
        return re.sub(r"@@[\t0-9.-]+?##", "", txt)

    def crop(self, text: str, ZM: int = 3, need_position: bool = False):  # noqa: N802
        """Reconstruct context strips from inline tags.

        Parameters
        ----------
        text: str
            Text containing one or more inline position tags.
        ZM: int
            Zoom factor to use for cropping; defaults to 3.
        need_position: bool
            If True, also return the list of positions.
        """

        if not self.page_images:
            return (None, None) if need_position else None

        tags = re.findall(r"@@([0-9-]+)\t([0-9.]+)\t([0-9.]+)\t([0-9.]+)\t([0-9.]+)##", text)
        if not tags:
            return (None, None) if need_position else None

        # Parse all tags and group by page
        parsed_rects = []  # list of (pn_idx, left, right, top, bottom)
        for pages_str, left, right, top, bottom in tags:
            try:
                pn_idx = int(str(pages_str).split("-")[0]) - 1
                parsed_rects.append(
                    (pn_idx, float(left), float(right), float(top), float(bottom))
                )
            except Exception:
                continue

        if not parsed_rects:
            return (None, None) if need_position else None

        # Convert to per-page bounding rect using first chunk TL and final chunk BR in reading order
        zm = max(ZM, 1)
        # Sort by reading order (page asc, top asc, left asc)
        parsed_rects.sort(key=lambda x: (x[0], x[3], x[1]))
        per_page_ordered = {}
        for pn_idx, left, right, top, bottom in parsed_rects:
            if pn_idx < 0 or pn_idx >= len(self.page_images):
                continue
            if pn_idx not in per_page_ordered:
                per_page_ordered[pn_idx] = []
            per_page_ordered[pn_idx].append((left, right, top, bottom))
        per_page = {}
        for pn_idx, rects in per_page_ordered.items():
            # Robust union: min left/top, max right/bottom across all rects in this page
            min_left = min(r[0] for r in rects)
            max_right = max(r[1] for r in rects)
            min_top = min(r[2] for r in rects)
            max_bottom = max(r[3] for r in rects)
            l = int(min_left * zm)
            t = int(min_top * zm)
            r = int(max_right * zm)
            b = int(max_bottom * zm)
            if r > l and b > t:
                per_page[pn_idx] = [l, t, r, b]

        if not per_page:
            return (None, None) if need_position else None

        # Crop images per page and compose horizontally (left-to-right by page)
        crops = []
        for pn_idx in sorted(per_page.keys()):
            l, t, r, b = per_page[pn_idx]
            try:
                crops.append(self.page_images[pn_idx].crop((l, t, r, b)))
            except Exception:
                continue

        if not crops:
            return (None, None) if need_position else None

        if len(crops) == 1:
            img = crops[0]
        else:
            # Compose horizontally with white background, height = max height
            widths = [im.width for im in crops]
            heights = [im.height for im in crops]
            total_w = sum(widths)
            max_h = max(heights)
            try:
                from PIL import Image as PILImage
                canvas = PILImage.new("RGB", (total_w, max_h), color=(255, 255, 255))
                x = 0
                for im in crops:
                    y = (max_h - im.height) // 2
                    canvas.paste(im, (x, y))
                    x += im.width
                img = canvas
            except Exception:
                img = crops[0]

        # Positions reflect the chosen per-page bounding rects (first TL to last BR)
        positions = [(pn_idx, l / zm, r / zm, t / zm, b / zm) for pn_idx, (l, t, r, b) in sorted(per_page.items())]
        if need_position:
            return img, positions
        return img

    def _build_sections_from_boxes(self, zoomin: int, mdl) -> List[Tuple[str, str]]:
        """Assemble sections from collected text boxes in per-page reading order.

        This converts every text-bearing block (from text-layer or OCR fallback,
        plus figure/table textual outputs) into a chunk formatted as
        (text + inline_position_tag, "").
        """

        sections: List[Tuple[str, str]] = []
        if not getattr(self, "boxes", None) or not getattr(self, "page_images", None):
            return sections

        # Group boxes by page
        pages = max([int(b.get("page_number", 0)) for b in self.boxes] or [0])
        for pn in range(1, pages + 1):
            page_boxes = [
                b for b in self.boxes
                if int(b.get("page_number", 0)) == pn and str(b.get("text", "")).strip()
            ]
            if not page_boxes:
                continue

            # Determine page size in PDF units
            try:
                img = self.page_images[pn - 1]
                pdf_w = img.width / max(zoomin, 1)
                pdf_h = img.height / max(zoomin, 1)
            except Exception:
                pdf_w, pdf_h = 1000.0, 1000.0

            # Order boxes using LayoutReader if available
            line_boxes = [(float(b["x0"]), float(b["top"]), float(b["x1"]), float(b["bottom"])) for b in page_boxes]
            order = []
            try:
                order = MonkeyOCRService.order_lines(line_boxes, pdf_w, pdf_h, model=mdl)
            except Exception:
                order = []
            if len(order) == len(page_boxes):
                ordered = [page_boxes[i] for i in order]
            else:
                ordered = sorted(page_boxes, key=lambda b: (float(b.get("top", 0.0)), float(b.get("x0", 0.0))))

            # Emit sections
            for b in ordered:
                text = str(b.get("text", "")).strip()
                if not text:
                    continue
                sections.append((text + self._line_tag(b), ""))

        return sections

    def _extract_tables_figures(self, need_image: bool, zoomin: int, return_html: bool,
                                omr_enabled: bool = False, omr_min_area: float = 500.0,
                                omr_max_aspect: float = 10.0) -> List[Any]:
        """Extract tables and figures with crops and captions/HTML.

        Returns list of tuples matching DeepDoc shape:
        - When need_image=True: [((PIL.Image, [caption_or_html]), positions), ...]
        - When no detections: returns []
        """

        if not self.page_layout or not self.page_images:
            return []

        def crop_bbox(pn: int, x0: float, x1: float, top: float, bottom: float):
            # Do not crop or process images. Return positions only.
            return None, [(pn, x0, x1, top, bottom)]

        # Collect candidates
        tables = []  # (page_index, block, caption_text)
        figures = []  # (page_index, block, caption_text)
        omr_candidates = []  # (page_index, block)

        for page_index, blocks in enumerate(self.page_layout):
            # Pre-collect caption blocks for lookup
            captions = [b for b in blocks if b.get("type") in {"table caption", "figure caption"}]

            def find_caption(for_block: dict, kind: str) -> str:
                # choose nearest caption with sufficient horizontal overlap
                target = None
                best = 1e18
                fx0, fx1, ftop, fbot = for_block["x0"], for_block["x1"], for_block["top"], for_block["bottom"]
                for cap in captions:
                    if kind == "table" and cap.get("type") != "table caption":
                        continue
                    if kind == "figure" and cap.get("type") != "figure caption":
                        continue
                    # horizontal overlap ratio
                    overlap = min(fx1, cap["x1"]) - max(fx0, cap["x0"])
                    width = max(cap["x1"] - cap["x0"], 1e-6)
                    if overlap / width < 0.2:
                        continue
                    # vertical distance from block
                    dy = max(0.0, cap["top"] - fbot) if cap["top"] >= fbot else max(0.0, ftop - cap["bottom"])
                    if dy < best:
                        best = dy
                        target = cap
                if not target:
                    return ""
                # Try to use an OCRed text from merged boxes falling within caption region
                for bx in self.boxes:
                    if bx.get("page_number", 0) != page_index + 1:
                        continue
                    if bx["x0"] >= target["x0"] and bx["x1"] <= target["x1"] and bx["top"] >= target["top"] and bx["bottom"] <= target["bottom"]:
                        return bx.get("text", "").strip()
                return ""

            for blk in blocks:
                if blk.get("type") == "table":
                    tables.append((page_index, blk, find_caption(blk, "table")))
                elif blk.get("type") == "figure":
                    figures.append((page_index, blk, find_caption(blk, "figure")))
                # OMR candidates (enabled flag)
                if omr_enabled:
                    bw = blk["x1"] - blk["x0"]
                    bh = blk["bottom"] - blk["top"]
                    area = bw * bh
                    aspect = (bw / max(bh, 1e-6)) if bh > 0 else 0
                    if area > omr_min_area and aspect < omr_max_aspect:
                        if blk.get("type") == "figure":
                            omr_candidates.append((page_index, blk))

        results: List[Any] = []
        positions: List[Any] = []
        
        # NOTE: OMR is handled as part of figure processing below to reuse the same
        # figure crop array: "multiple_choice" => OMR ratings, else => OCR text.

        # Process figures (always OCR handwriting/image region; no saved crops)
        fig_crops: List[Image.Image] = []
        fig_meta: List[tuple[int, dict]] = []
        for page_index, blk, cap in figures:
            x0, x1 = float(blk["x0"]), float(blk["x1"])
            top, bottom = float(blk["top"]), float(blk["bottom"]) 
            l = max(0, int(x0 * zoomin))
            r = max(l + 1, int(x1 * zoomin))
            t = max(0, int(top * zoomin))
            b = max(t + 1, int(bottom * zoomin))
            try:
                fig_crops.append(self.page_images[page_index].crop((l, t, r, b)))
                fig_meta.append((page_index, blk))
            except Exception:
                fig_crops.append(None)  # placeholder
                fig_meta.append((page_index, blk))

        # Classify figures as multiple-choice or text, then run OMR or OCR accordingly.
        fig_texts: List[str] = [""] * len(fig_crops)
        if any(img is not None for img in fig_crops):
            ratings_list = []
            try:
                ratings_list = MonkeyOCRService.omr_ratings_batch([img for img in fig_crops if img is not None])
            except Exception:
                ratings_list = [None] * len([img for img in fig_crops if img is not None])
            # Map ratings back to original index positions
            ratings_iter = iter(ratings_list)
            per_idx_ratings: List[object] = []
            for img in fig_crops:
                per_idx_ratings.append(next(ratings_iter, None) if img is not None else None)

            # Prepare OCR for those without OMR ratings
            ocr_indices: List[int] = [i for i, (img, rt) in enumerate(zip(fig_crops, per_idx_ratings)) if img is not None and (not rt or not any((r > 0) for r in (rt or [])))]
            ocr_imgs: List[Image.Image] = [fig_crops[i] for i in ocr_indices]
            ocr_preds: List[str] = []
            if ocr_imgs:
                try:
                    ocr_preds = MonkeyOCRService.ocr_text(
                        ocr_imgs,
                        instruction="This region may contain handwriting or image text or character. Extract readable text or character only. If there is no text or character, return \"\".",
                    )
                except Exception:
                    ocr_preds = [""] * len(ocr_imgs)
            # Assign captions: OMR caption if ratings present; otherwise OCR text
            ocr_it = iter(ocr_preds)
            for i, img in enumerate(fig_crops):
                if img is None:
                    fig_texts[i] = ""
                    continue
                rt = per_idx_ratings[i]
                if rt and any((r > 0) for r in rt):
                    fig_texts[i] = "".join([f"Item {j+1}: score {r}, \n" for j, r in enumerate(rt)])
                else:
                    fig_texts[i] = next(ocr_it, "")

        for (page_index, blk, cap), txt in zip(figures, fig_texts):
            caption = txt.strip() or (cap or "Figure")
            _, poss = crop_bbox(page_index, blk["x0"], blk["x1"], blk["top"], blk["bottom"])  # positions only
            if need_image:
                results.append((None, [caption] if caption else []))
                positions.append(poss)
            if caption:
                self.boxes.append({
                    "x0": float(blk["x0"]),
                    "x1": float(blk["x1"]),
                    "top": float(blk["top"]),
                    "bottom": float(blk["bottom"]),
                    "text": caption,
                    "layout_type": "text",
                    "page_number": page_index + 1,
                    "layoutno": -3,
                })
        # Release temporary figure crops/preds
        try:
            del fig_crops, fig_meta, fig_texts
        except Exception:
            pass
                    
        # Process tables (also append text to sections)
        table_images: List[Image.Image] = []
        table_pos_list: List[List[tuple]] = []
        for page_index, blk, _cap in tables:
            img, poss = crop_bbox(page_index, blk["x0"], blk["x1"], blk["top"], blk["bottom"])  # poss is list
            table_images.append(img)
            table_pos_list.append(poss)

        table_htmls: List[str] = []
        if need_image and table_images and return_html:
            try:
                table_htmls = MonkeyOCRService.ocr_text(
                    table_images,
                    instruction="This is the image of a table. Please output the table in html format.",
                )
            except Exception:
                table_htmls = [""] * len(table_images)
        else:
            table_htmls = [""] * (len(table_images) if need_image else len(tables))

        ti = 0
        for (page_index, blk, cap) in tables:
            img = table_images[ti] if (need_image and ti < len(table_images)) else None
            html_or_text = table_htmls[ti] if (need_image and return_html and ti < len(table_images)) else (cap or "")
            if not html_or_text:
                html_or_text = "Table"
            if need_image:
                results.append((img, [html_or_text] if html_or_text else []))
                positions.append(table_pos_list[ti])
            # Always append textual representation to sections
            if html_or_text:
                self.boxes.append({
                    "x0": float(blk["x0"]),
                    "x1": float(blk["x1"]),
                    "top": float(blk["top"]),
                    "bottom": float(blk["bottom"]),
                    "text": html_or_text,
                    "layout_type": "text",
                    "page_number": page_index + 1,
                    "layoutno": -2,
                })
            ti += 1

        # Zip results with positions to match DeepDoc return
        if need_image:
            assert len(results) == len(positions)
            return list(zip(results, positions))
        # When images are not needed, return empty list (info appended to sections)
        return []