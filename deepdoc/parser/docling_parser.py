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
from __future__ import annotations

import logging
import re
from dataclasses import dataclass
from enum import Enum
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Any, Callable, Iterable, Optional

import pdfplumber
from PIL import Image

try:
    from docling.document_converter import DocumentConverter, PdfFormatOption
    from docling.datamodel.base_models import InputFormat
    from docling.datamodel.pipeline_options import (
        PdfPipelineOptions,
        TableFormerMode,
        TableStructureOptions,
        EasyOcrOptions,
    )
    from docling.backend.pypdfium2_backend import PyPdfiumDocumentBackend
except Exception:
    DocumentConverter = None
    PdfFormatOption = None
    InputFormat = None
    PdfPipelineOptions = None
    TableFormerMode = None
    TableStructureOptions = None
    EasyOcrOptions = None
    PyPdfiumDocumentBackend = None  

try:
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser
except Exception:
    class RAGFlowPdfParser:  
        pass


class DoclingContentType(str, Enum):
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


class DoclingParser(RAGFlowPdfParser):
    def __init__(self):
        """
        Initialize DoclingParser with RAGFlow-compatible configuration.
        
        Note: Advanced features like Heron layout model and VLM enhancements
        are available in docling library but not yet integrated into RAGFlow.
        """
        self.logger = logging.getLogger(self.__class__.__name__)
        self.page_images: list[Image.Image] = []
        self.page_from = 0
        self.page_to = 10_000
        self.outlines = []
        self._converter = None
    
    def _get_converter(self):
        """
        Get or create a DocumentConverter with optimized configuration.
        Uses caching to avoid recreating the converter.
        
        Returns:
            DocumentConverter instance or None if not available
        """
        if self._converter is not None:
            return self._converter
            
        if DocumentConverter is None:
            self.logger.warning("[Docling] DocumentConverter not available")
            return None
            
        try:
            # Configure pipeline options for better accuracy
            pipeline_options = None
            if PdfPipelineOptions is not None:
                table_structure_options = None
                if TableStructureOptions is not None and TableFormerMode is not None:
                    # Use TableFormer for better table extraction
                    table_structure_options = TableStructureOptions(
                        mode=TableFormerMode.ACCURATE,
                        do_cell_matching=True,
                    )
                
                pipeline_options = PdfPipelineOptions(
                    do_table_structure=True,
                    do_ocr=True,
                    table_structure_options=table_structure_options,
                )
                
                # Enable OCR options if available
                if EasyOcrOptions is not None:
                    pipeline_options.ocr_options = EasyOcrOptions(
                        force_full_page_ocr=False,
                    )
            
            # Configure format options
            format_options = {}
            if PdfFormatOption is not None and pipeline_options is not None:
                format_options = {
                    InputFormat.PDF: PdfFormatOption(
                        pipeline_options=pipeline_options,
                        backend=PyPdfiumDocumentBackend if PyPdfiumDocumentBackend is not None else None,
                    )
                }
            
            # Create converter with configuration
            if format_options:
                self._converter = DocumentConverter(format_options=format_options)
            else:
                self._converter = DocumentConverter()
                
            self.logger.info("[Docling] DocumentConverter initialized successfully")
            return self._converter
            
        except Exception as e:
            self.logger.error(f"[Docling] Failed to initialize DocumentConverter: {e}", exc_info=True)
            return None
        
    def check_installation(self) -> bool:
        """Check if Docling is properly installed and can be initialized."""
        if DocumentConverter is None:
            self.logger.warning("[Docling] 'docling' is not importable, please: pip install docling")
            return False
        try:
            converter = self._get_converter()
            return converter is not None
        except Exception as e:
            self.logger.error(f"[Docling] init DocumentConverter failed: {e}", exc_info=True)
            return False

    def __images__(self, fnm, zoomin: int = 1, page_from=0, page_to=600, callback=None):
        """
        Render PDF pages as images for cropping and visualization.
        
        Args:
            fnm: File path or binary data
            zoomin: Resolution multiplier for image rendering
            page_from: Start page index
            page_to: End page index
            callback: Optional callback for progress updates
        """
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
                self.logger.info(f"[Docling] Rendered {len(self.page_images)} pages as images")
        except Exception as e:
            self.page_images = []
            self.logger.error(f"[Docling] Failed to render PDF pages: {e}", exc_info=True)
        finally:
            if bytes_io:
                bytes_io.close()

    def _make_line_tag(self,bbox: _BBox) -> str:
        if bbox is None:
            return ""
        x0,x1, top, bott = bbox.x0, bbox.x1, bbox.y0, bbox.y1
        if hasattr(self, "page_images") and self.page_images and len(self.page_images) >= bbox.page_no:
            _, page_height = self.page_images[bbox.page_no-1].size
            top, bott = page_height-top ,page_height-bott
        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format(
            bbox.page_no, x0,x1, top, bott
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
            if 0 < ii < len(poss)-1:
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

    def _iter_doc_items(self, doc) -> Iterable[tuple[str, Any, Optional[_BBox]]]:
        for t in getattr(doc, "texts", []):
            parent=getattr(t, "parent", "")
            ref=getattr(parent,"cref","")
            label=getattr(t, "label", "")
            if (label in ("section_header","text",) and ref in ("#/body",)) or label in ("list_item",):
                text = getattr(t, "text", "") or ""
                bbox = None
                if getattr(t, "prov", None):
                    pn = getattr(t.prov[0], "page_no", None)
                    bb = getattr(t.prov[0], "bbox", None)
                    bb = [getattr(bb, "l", None),getattr(bb, "t", None),getattr(bb, "r", None),getattr(bb, "b", None)]
                    if pn and bb and len(bb) == 4:
                        bbox = _BBox(page_no=int(pn), x0=bb[0], y0=bb[1], x1=bb[2], y1=bb[3])
                yield (DoclingContentType.TEXT.value, text, bbox)

        for item in getattr(doc, "texts", []):
            if getattr(item, "label", "") in ("FORMULA",):
                text = getattr(item, "text", "") or ""
                bbox = None
                if getattr(item, "prov", None):
                    pn = getattr(item.prov, "page_no", None)
                    bb = getattr(item.prov, "bbox", None)
                    bb = [getattr(bb, "l", None),getattr(bb, "t", None),getattr(bb, "r", None),getattr(bb, "b", None)]
                    if pn and bb and len(bb) == 4:
                        bbox = _BBox(int(pn), bb[0], bb[1], bb[2], bb[3])
                yield (DoclingContentType.EQUATION.value, text, bbox)

    def _transfer_to_sections(self, doc, parse_method: str) -> list[tuple[str, str]]:
        sections: list[tuple[str, str]] = []
        for typ, payload, bbox in self._iter_doc_items(doc):
            if typ == DoclingContentType.TEXT.value:
                section = payload.strip()
                if not section:
                    continue
            elif typ == DoclingContentType.EQUATION.value:
                section = payload.strip()
            else:
                continue
            
            tag = self._make_line_tag(bbox) if isinstance(bbox,_BBox) else ""
            if parse_method == "manual":
                sections.append((section, typ, tag))
            elif parse_method == "paper":
                sections.append((section + tag, typ))
            else:
                sections.append((section, tag))
        return sections

    def cropout_docling_table(self, page_no: int, bbox: tuple[float, float, float, float], zoomin: int = 1):
        """
        Crop a table or image region from the rendered PDF pages.
        
        Args:
            page_no: Page number (1-indexed)
            bbox: Bounding box (left, top, right, bottom)
            zoomin: Zoom factor (currently not used)
            
        Returns:
            Tuple of (cropped_image, position_list)
        """
        if not getattr(self, "page_images", None):
            self.logger.warning("[Docling] No page images available for cropping")
            return None, ""

        idx = (page_no - 1) - getattr(self, "page_from", 0)
        if idx < 0 or idx >= len(self.page_images):
            self.logger.warning(f"[Docling] Page index {idx} out of range for cropping")
            return None, ""

        try:
            page_img = self.page_images[idx]
            W, H = page_img.size
            left, top, right, bott = bbox

            x0 = float(left)
            y0 = float(H-top)
            x1 = float(right)
            y1 = float(H-bott)

            x0, y0 = max(0.0, min(x0, W - 1)), max(0.0, min(y0, H - 1))
            x1, y1 = max(x0 + 1.0, min(x1, W)), max(y0 + 1.0, min(y1, H))

            crop = page_img.crop((int(x0), int(y0), int(x1), int(y1))).convert("RGB")
            pos = (page_no-1 if page_no>0 else 0, x0, x1, y0, y1)
            return crop, [pos]
        except Exception as e:
            self.logger.error(f"[Docling] Failed to crop region from page {page_no}: {e}", exc_info=True)
            return None, ""

    def _transfer_to_tables(self, doc):
        """
        Extract tables and pictures from the Docling document.
        
        Args:
            doc: Docling document object
            
        Returns:
            List of table/image tuples with positions
        """
        tables = []
        
        # Extract tables
        for tab in getattr(doc, "tables", []):
            img = None
            positions = ""
            
            # Extract table image if page images are available
            if getattr(tab, "prov", None):
                try:
                    pn = getattr(tab.prov[0], "page_no", None)
                    bb = getattr(tab.prov[0], "bbox", None)
                    if pn is not None and bb is not None:
                        left = getattr(bb, "l", None)
                        top = getattr(bb, "t", None)
                        right = getattr(bb, "r", None)
                        bott = getattr(bb, "b", None)
                        if None not in (left, top, right, bott):
                            img, positions = self.cropout_docling_table(int(pn), (float(left), float(top), float(right), float(bott)))
                except Exception as e:
                    self.logger.warning(f"[Docling] Failed to extract table image: {e}")
                    
            # Extract table HTML
            html = ""
            try:
                html = tab.export_to_html(doc=doc)
            except Exception as e:
                self.logger.warning(f"[Docling] Failed to export table to HTML: {e}")
                
            tables.append(((img, html), positions if positions else ""))
            
        # Extract pictures
        for pic in getattr(doc, "pictures", []):
            img = None
            positions = ""
            
            # Extract picture image
            if getattr(pic, "prov", None):
                try:
                    pn = getattr(pic.prov[0], "page_no", None)
                    bb = getattr(pic.prov[0], "bbox", None)
                    if pn is not None and bb is not None:
                        left = getattr(bb, "l", None)
                        top = getattr(bb, "t", None)
                        right = getattr(bb, "r", None)
                        bott = getattr(bb, "b", None)
                        if None not in (left, top, right, bott):
                            img, positions = self.cropout_docling_table(int(pn), (float(left), float(top), float(right), float(bott)))
                except Exception as e:
                    self.logger.warning(f"[Docling] Failed to extract picture image: {e}")
                    
            # Extract picture caption
            captions = ""
            try:
                captions = pic.caption_text(doc=doc)
            except Exception as e:
                self.logger.warning(f"[Docling] Failed to extract picture caption: {e}")
                
            tables.append(((img, [captions]), positions if positions else ""))
            
        return tables

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes | None = None,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None, 
        lang: Optional[str] = None,        
        method: str = "auto",             
        delete_output: bool = True,
        parse_method: str = "raw"
    ):
        """
        Parse a PDF file using Docling with advanced configuration.
        
        Args:
            filepath: Path to the PDF file
            binary: Optional binary data of the PDF
            callback: Optional callback for progress updates
            output_dir: Directory for temporary files
            lang: Language hint (currently not used by Docling)
            method: Parsing method (currently not used)
            delete_output: Whether to delete temporary files
            parse_method: RAGFlow parsing method ("raw", "manual", "paper")
            
        Returns:
            Tuple of (sections, tables) compatible with RAGFlow pipeline
            
        Raises:
            RuntimeError: If Docling is not available
            FileNotFoundError: If the PDF file is not found
        """
        if not self.check_installation():
            raise RuntimeError("Docling not available, please install `docling`")

        # Handle binary input
        tmp_pdf = None
        if binary is not None:
            try:
                tmpdir = Path(output_dir) if output_dir else Path.cwd() / ".docling_tmp"
                tmpdir.mkdir(parents=True, exist_ok=True)
                name = Path(filepath).name or "input.pdf"
                tmp_pdf = tmpdir / name
                with open(tmp_pdf, "wb") as f:
                    if isinstance(binary, (bytes, bytearray)):
                        f.write(binary)
                    else:
                        f.write(binary.getbuffer())
                src_path = tmp_pdf
            except Exception as e:
                self.logger.error(f"[Docling] Failed to write temporary PDF: {e}", exc_info=True)
                raise RuntimeError(f"Failed to write temporary PDF: {e}") from e
        else:
            src_path = Path(filepath)
            if not src_path.exists():
                raise FileNotFoundError(f"PDF not found: {src_path}")

        if callback:
            callback(0.1, f"[Docling] Converting: {src_path}")

        # Render pages as images for cropping
        try:
            self.__images__(str(src_path), zoomin=1)
        except Exception as e:
            self.logger.warning(f"[Docling] render pages failed: {e}", exc_info=True)

        # Convert document using cached converter
        try:
            conv = self._get_converter()
            if conv is None:
                raise RuntimeError("Failed to initialize DocumentConverter")
                
            if callback:
                callback(0.3, "[Docling] Starting conversion...")
                
            conv_res = conv.convert(str(src_path))
            doc = conv_res.document
            
            if callback:
                callback(0.7, f"[Docling] Parsed doc: {getattr(doc, 'num_pages', 'n/a')} pages")
        except Exception as e:
            self.logger.error(f"[Docling] Document conversion failed: {e}", exc_info=True)
            raise RuntimeError(f"Document conversion failed: {e}") from e

        # Extract sections and tables
        try:
            sections = self._transfer_to_sections(doc, parse_method=parse_method)
            tables = self._transfer_to_tables(doc)
            
            if callback:
                callback(0.95, f"[Docling] Sections: {len(sections)}, Tables: {len(tables)}")
        except Exception as e:
            self.logger.error(f"[Docling] Failed to extract content: {e}", exc_info=True)
            raise RuntimeError(f"Failed to extract content: {e}") from e

        # Clean up temporary files
        if tmp_pdf is not None and delete_output:
            try:
                tmp_pdf.unlink(missing_ok=True)
            except Exception as e:
                self.logger.warning(f"[Docling] Failed to delete temporary file: {e}")

        if callback:
            callback(1.0, "[Docling] Done.")
            
        return sections, tables


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    parser = DoclingParser()
    print("Docling available:", parser.check_installation())
    sections, tables = parser.parse_pdf(filepath="test_docling/toc.pdf", binary=None)
    print(len(sections), len(tables))
