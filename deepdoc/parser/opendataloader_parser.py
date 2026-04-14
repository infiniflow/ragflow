
from __future__ import annotations

import json
import logging
import re
import shutil
import tempfile
from dataclasses import dataclass
from enum import Enum
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Any, Callable, Iterable, Optional

import pdfplumber
from PIL import Image

try:
    import opendataloader_pdf
except Exception:
    opendataloader_pdf = None

try:
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser
except Exception:
    class RAGFlowPdfParser:
        pass

from deepdoc.parser.utils import extract_pdf_outlines


class OpenDataLoaderContentType(str, Enum):
    IMAGE = "image"
    TABLE = "table"
    TEXT = "text"
    EQUATION = "equation"


@dataclass
class _BBox:
    page_no: int
    x0: float
    y0: float
    x1: float
    y1: float


_TEXT_TYPES = {"heading", "title", "paragraph", "text", "list", "list_item", "caption"}
_TABLE_TYPES = {"table"}
_IMAGE_TYPES = {"image", "picture", "figure"}
_FORMULA_TYPES = {"formula", "equation"}


def _as_float(v) -> Optional[float]:
    try:
        return float(v)
    except Exception:
        return None


def _bbox_from_element(el: dict) -> Optional[_BBox]:
    bb = el.get("bounding box") or el.get("bounding_box") or el.get("bbox")
    pn = el.get("page number")
    if pn is None:
        pn = el.get("page_number")
    if pn is None:
        pn = el.get("page")
    if bb is None or pn is None:
        return None
    if not isinstance(bb, (list, tuple)) or len(bb) < 4:
        return None
    coords = [_as_float(x) for x in bb[:4]]
    if any(c is None for c in coords):
        return None
    try:
        page_no = int(pn)
    except Exception:
        return None
    # OpenDataLoader emits [left, bottom, right, top] in PDF points.
    left, bottom, right, top = coords
    x0, x1 = min(left, right), max(left, right)
    y0, y1 = min(bottom, top), max(bottom, top)
    return _BBox(page_no=page_no, x0=x0, y0=y0, x1=x1, y1=y1)


def _iter_elements(node: Any) -> Iterable[dict]:
    if isinstance(node, dict):
        if "type" in node and ("content" in node or "text" in node or "cells" in node):
            yield node
        for v in node.values():
            yield from _iter_elements(v)
    elif isinstance(node, list):
        for item in node:
            yield from _iter_elements(item)


def _element_text(el: dict) -> str:
    content = el.get("content")
    if isinstance(content, str):
        return content
    text = el.get("text")
    if isinstance(text, str):
        return text
    # tables may expose cells; join row-wise if needed
    cells = el.get("cells")
    if isinstance(cells, list):
        rows: dict[int, list[str]] = {}
        for c in cells:
            if not isinstance(c, dict):
                continue
            row = c.get("row") or c.get("row_index") or 0
            rows.setdefault(int(row), []).append(str(c.get("content") or c.get("text") or ""))
        return "\n".join(" | ".join(v) for _, v in sorted(rows.items()))
    return ""


def _element_html(el: dict) -> str:
    for key in ("html", "html_content"):
        v = el.get(key)
        if isinstance(v, str) and v.strip():
            return v
    return ""


class OpenDataLoaderParser(RAGFlowPdfParser):
    def __init__(self):
        self.logger = logging.getLogger(self.__class__.__name__)
        self.page_images: list[Image.Image] = []
        self.page_from = 0
        self.page_to = 10_000
        self.outlines = []

    def check_installation(self) -> bool:
        if opendataloader_pdf is None:
            self.logger.warning(
                "[OpenDataLoader] 'opendataloader_pdf' is not importable. "
                "Install with: pip install opendataloader-pdf (requires Java 11+)."
            )
            return False
        if not hasattr(opendataloader_pdf, "convert"):
            self.logger.warning("[OpenDataLoader] installed package lacks convert() entry point.")
            return False
        if not shutil.which("java"):
            self.logger.warning("[OpenDataLoader] Java runtime not found on PATH (Java 11+ required).")
            return False
        return True

    def __images__(self, fnm, zoomin: int = 1, page_from=0, page_to=600, callback=None):
        self.page_from = page_from
        self.page_to = page_to
        bytes_io = None
        try:
            if not isinstance(fnm, (str, PathLike)):
                bytes_io = BytesIO(fnm)
            opener = pdfplumber.open(fnm) if isinstance(fnm, (str, PathLike)) else pdfplumber.open(bytes_io)
            with opener as pdf:
                pages = pdf.pages[page_from:page_to]
                self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).original for p in pages]
        except Exception as e:
            self.page_images = []
            self.logger.exception(e)
        finally:
            if bytes_io:
                bytes_io.close()

    def _make_line_tag(self, bbox: _BBox) -> str:
        if bbox is None:
            return ""
        x0, x1, top, bott = bbox.x0, bbox.x1, bbox.y0, bbox.y1
        if self.page_images and len(self.page_images) >= bbox.page_no:
            _, page_height = self.page_images[bbox.page_no - 1].size
            top, bott = page_height - top, page_height - bott
        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format(
            bbox.page_no, x0, x1, top, bott
        )

    @staticmethod
    def extract_positions(txt: str) -> list[tuple[list[int], float, float, float, float]]:
        poss = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", txt):
            pn, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
            left, right, top, bottom = float(left), float(right), float(top), float(bottom)
            poss.append(([int(p) - 1 for p in pn.split("-")], left, right, top, bottom))
        return poss

    def crop(self, text: str, ZM: int = 1, need_position: bool = False):
        imgs = []
        poss = self.extract_positions(text)
        if not poss:
            return (None, None) if need_position else None
        GAP = 6
        pos = poss[0]
        poss.insert(0, ([pos[0][0]], pos[1], pos[2], max(0, pos[3] - 120), max(pos[3] - GAP, 0)))
        pos = poss[-1]
        poss.append(([pos[0][-1]], pos[1], pos[2], min(self.page_images[pos[0][-1]].size[1], pos[4] + GAP), min(self.page_images[pos[0][-1]].size[1], pos[4] + 120)))
        positions = []
        for ii, (pns, left, right, top, bottom) in enumerate(poss):
            if bottom <= top:
                bottom = top + 4
            img0 = self.page_images[pns[0]]
            x0, y0, x1, y1 = int(left), int(top), int(right), int(min(bottom, img0.size[1]))
            crop0 = img0.crop((x0, y0, x1, y1))
            imgs.append(crop0)
            if 0 < ii < len(poss) - 1:
                positions.append((pns[0] + self.page_from, x0, x1, y0, y1))
            remain_bottom = bottom - img0.size[1]
            for pn in pns[1:]:
                if remain_bottom <= 0:
                    break
                page = self.page_images[pn]
                x0, y0, x1, y1 = int(left), 0, int(right), int(min(remain_bottom, page.size[1]))
                cimgp = page.crop((x0, y0, x1, y1))
                imgs.append(cimgp)
                if 0 < ii < len(poss) - 1:
                    positions.append((pn + self.page_from, x0, x1, y0, y1))
                remain_bottom -= page.size[1]
        if not imgs:
            return (None, None) if need_position else None
        height = sum(i.size[1] + GAP for i in imgs)
        width = max(i.size[0] for i in imgs)
        pic = Image.new("RGB", (width, int(height)), (245, 245, 245))
        h = 0
        for ii, img in enumerate(imgs):
            if ii == 0 or ii + 1 == len(imgs):
                img = img.convert("RGBA")
                overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
                overlay.putalpha(128)
                img = Image.alpha_composite(img, overlay).convert("RGB")
            pic.paste(img, (0, int(h)))
            h += img.size[1] + GAP
        return (pic, positions) if need_position else pic

    def _cropout_region(self, bbox: _BBox, zoomin: int = 1):
        if not self.page_images:
            return None, ""
        idx = (bbox.page_no - 1) - self.page_from
        if idx < 0 or idx >= len(self.page_images):
            return None, ""
        page_img = self.page_images[idx]
        W, H = page_img.size
        x0 = max(0.0, min(float(bbox.x0), W - 1))
        y0 = max(0.0, min(float(H - bbox.y1), H - 1))
        x1 = max(x0 + 1.0, min(float(bbox.x1), W))
        y1 = max(y0 + 1.0, min(float(H - bbox.y0), H))
        try:
            crop = page_img.crop((int(x0), int(y0), int(x1), int(y1))).convert("RGB")
        except Exception:
            return None, ""
        pos = (bbox.page_no - 1 if bbox.page_no > 0 else 0, x0, x1, y0, y1)
        return crop, [pos]

    def _classify(self, el_type: str) -> str:
        t = (el_type or "").lower()
        if t in _TABLE_TYPES:
            return OpenDataLoaderContentType.TABLE.value
        if t in _IMAGE_TYPES:
            return OpenDataLoaderContentType.IMAGE.value
        if t in _FORMULA_TYPES:
            return OpenDataLoaderContentType.EQUATION.value
        return OpenDataLoaderContentType.TEXT.value

    def _transfer_from_json(self, root: Any, parse_method: str):
        sections: list[tuple[str, ...]] = []
        tables: list = []
        for el in _iter_elements(root):
            el_type = self._classify(el.get("type", ""))
            bbox = _bbox_from_element(el)
            tag = self._make_line_tag(bbox) if bbox else ""

            if el_type == OpenDataLoaderContentType.TABLE.value:
                html = _element_html(el) or _element_text(el)
                img = None
                positions = ""
                if bbox:
                    img, positions = self._cropout_region(bbox)
                tables.append(((img, html), positions if positions else ""))
                continue

            if el_type == OpenDataLoaderContentType.IMAGE.value:
                img = None
                positions = ""
                if bbox:
                    img, positions = self._cropout_region(bbox)
                caption = _element_text(el)
                tables.append(((img, [caption] if caption else [""]), positions if positions else ""))
                continue

            text = _element_text(el).strip()
            if not text:
                continue
            if parse_method in {"manual", "pipeline"}:
                sections.append((text, el_type, tag))
            elif parse_method == "paper":
                sections.append((text + tag, el_type))
            else:
                sections.append((text, tag))
        return sections, tables

    @staticmethod
    def _sections_from_markdown(md: str, parse_method: str) -> list[tuple[str, ...]]:
        txt = (md or "").strip()
        if not txt:
            return []
        if parse_method in {"manual", "pipeline"}:
            return [(txt, OpenDataLoaderContentType.TEXT.value, "")]
        if parse_method == "paper":
            return [(txt, OpenDataLoaderContentType.TEXT.value)]
        return [(txt, "")]

    @staticmethod
    def _read_outputs(out_dir: Path) -> tuple[Optional[dict], Optional[str]]:
        json_doc: Optional[dict] = None
        md_text: Optional[str] = None
        for p in sorted(out_dir.rglob("*.json")):
            try:
                with open(p, "r", encoding="utf-8") as f:
                    json_doc = json.load(f)
                break
            except Exception:
                continue
        for p in sorted(out_dir.rglob("*.md")):
            try:
                md_text = p.read_text(encoding="utf-8")
                break
            except Exception:
                continue
        return json_doc, md_text

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes | None = None,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None,
        delete_output: bool = True,
        parse_method: str = "raw",
        hybrid: Optional[str] = None,
        image_output: Optional[str] = None,
        sanitize: Optional[bool] = None,
    ):
        self.outlines = extract_pdf_outlines(binary if binary is not None else filepath)

        if not self.check_installation():
            raise RuntimeError("OpenDataLoader not available, please install `opendataloader-pdf`")

        workdir: Optional[Path] = None
        cleanup_workdir = False
        if output_dir:
            workdir = Path(output_dir)
            workdir.mkdir(parents=True, exist_ok=True)
        else:
            workdir = Path(tempfile.mkdtemp(prefix="opendataloader_"))
            cleanup_workdir = True

        src_path: Path
        tmp_input: Optional[Path] = None
        if binary is not None:
            name = Path(filepath).name or "input.pdf"
            tmp_input = workdir / f"_in_{name}"
            with open(tmp_input, "wb") as f:
                if isinstance(binary, (bytes, bytearray)):
                    f.write(binary)
                else:
                    f.write(binary.getbuffer())
            src_path = tmp_input
        else:
            src_path = Path(filepath)
            if not src_path.exists():
                raise FileNotFoundError(f"PDF not found: {src_path}")

        if callback:
            callback(0.1, f"[OpenDataLoader] Converting: {src_path.name}")

        try:
            self.__images__(str(src_path), zoomin=1)
        except Exception as e:
            self.logger.warning(f"[OpenDataLoader] render pages failed: {e}")

        out_sub = workdir / "out"
        out_sub.mkdir(parents=True, exist_ok=True)
        convert_kwargs: dict[str, Any] = {
            "input_path": [str(src_path)],
            "output_dir": str(out_sub),
            "format": "markdown,json",
        }
        if hybrid:
            convert_kwargs["hybrid"] = hybrid
        if image_output:
            convert_kwargs["image_output"] = image_output
        if sanitize is not None:
            convert_kwargs["sanitize"] = sanitize

        try:
            opendataloader_pdf.convert(**convert_kwargs)
        except Exception as e:
            if cleanup_workdir:
                shutil.rmtree(workdir, ignore_errors=True)
            raise RuntimeError(f"[OpenDataLoader] convert failed: {e}") from e

        if callback:
            callback(0.7, "[OpenDataLoader] Reading outputs")

        json_doc, md_text = self._read_outputs(out_sub)

        sections: list[tuple[str, ...]] = []
        tables: list = []
        if json_doc is not None:
            sections, tables = self._transfer_from_json(json_doc, parse_method=parse_method)
        if not sections and md_text:
            sections = self._sections_from_markdown(md_text, parse_method=parse_method)

        if callback:
            callback(0.95, f"[OpenDataLoader] Sections: {len(sections)}, Tables: {len(tables)}")

        if delete_output:
            if tmp_input is not None:
                try:
                    tmp_input.unlink(missing_ok=True)
                except Exception:
                    pass
            if cleanup_workdir:
                shutil.rmtree(workdir, ignore_errors=True)
            else:
                shutil.rmtree(out_sub, ignore_errors=True)

        if callback:
            callback(1.0, "[OpenDataLoader] Done.")
        return sections, tables


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    parser = OpenDataLoaderParser()
    print("OpenDataLoader available:", parser.check_installation())
