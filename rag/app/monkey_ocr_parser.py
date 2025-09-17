#!/usr/bin/env python3
"""
MonkeyOCR Parser for RAGFlow Integration
Integrates CEDD OCR service with RAGFlow document processing
Follows exact flow from cedd_parse.py
"""

import logging
import os
import tempfile
import time
from pathlib import Path
from typing import Dict, Any, Optional, List
import sys

from monkeydoc.parser import MonkeyDocPdfParser  # type: ignore

logger = logging.getLogger(__name__)


def chunk(filename, binary=None, from_page=0, to_page=100000, lang="Chinese", callback=None, **kwargs):
    """
    MonkeyOCR chunk function for RAGFlow integration.
    Follows exact cedd_parse.py flow with 'full' mode.

    Args:
        filename (str): File name
        binary (bytes): File content
        from_page (int): Start page
        to_page (int): End page
        lang (str): Language
        callback (function): Progress callback
        **kwargs: Additional arguments

    Returns:
        list: List of document chunks
    """

    def safe_callback(progress, message):
        if callback:
            callback(progress, message)

    try:

        safe_callback(0.1, "Starting CEDD OCR processing with cedd_parse flow...")
        logger.info("✅ Step 1: Starting CEDD OCR processing")
        import re
        if re.search(r"\.pdf$", filename, re.IGNORECASE) or re.search(r"\.png$", filename, re.IGNORECASE) or re.search(r"\.jpg$", filename, re.IGNORECASE) or re.search(r"\.jpeg$", filename, re.IGNORECASE):
            parser_config = kwargs.get("parser_config", {})
            logger.info(f"⚙️ Parser config: {parser_config}")
            safe_callback(0.1, "CEDD OCR: rendering and layout/OCR...")
            try:
                # Parse with DeepDoc-compatible MonkeyDoc
                pdf_parser = MonkeyDocPdfParser()
                # Determine table HTML flag and whether to return images
                mcfg = parser_config.get("monkeyocr", {}) if isinstance(parser_config, dict) else {}
                return_html = bool(mcfg.get("table_html", True))
                # Default to sections-only (no images) to avoid unnecessary crops
                need_image = bool(mcfg.get("return_images", False))
                # Read MonkeyOCR config flags
                omr_cfg = mcfg.get("omr", {}) if isinstance(mcfg, dict) else {}
                omr_enabled = bool(omr_cfg.get("enabled", True))
                omr_min_area = float(omr_cfg.get("min_area", 200.0))
                omr_max_aspect = float(omr_cfg.get("max_aspect", 10.0))
                chunk_token_num = int(parser_config.get("chunk_token_num", 256))
                sections, tbls = pdf_parser(
                    filename if not binary else binary,
                    need_image=need_image,
                    zoomin=3,
                    return_html=return_html,
                    omr_enabled=omr_enabled,
                    omr_min_area=omr_min_area,
                    omr_max_aspect=omr_max_aspect,
                    from_page=from_page,
                    to_page=to_page,
                    callback=safe_callback
                )
                from monkeydoc.utils import pack_by_token_limit
                sections = pack_by_token_limit(sections, chunk_token_num=chunk_token_num)
                safe_callback(0.7, "CEDD OCR: building chunks...")
                from rag.nlp import tokenize_chunks, tokenize_table, rag_tokenizer
                import re

                # Base doc metadata
                base_doc = {
                    "docnm_kwd": filename,
                    "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename)),
                    "doc_type_kwd": "monkeyocr",
                }
                eng = lang.lower() == "english"

                # Tokenize sections with position tags; pass parser to crop/remove_tag
                docs_text = tokenize_chunks([s[0] if isinstance(s, tuple) else s for s in sections], base_doc, eng, pdf_parser=pdf_parser)

                # Tokenize tables/figures
                docs_tbls = tokenize_table(tbls, base_doc, eng)

                docs = (docs_text or []) + (docs_tbls or [])
                safe_callback(1.0, f"CEDD OCR: {len(docs)} chunks ready")
                return docs
            except Exception as e:
                logger.warning(f"MonkeyDoc path failed, falling back to enhanced markdown path: {e}")
        else:
            raise NotImplementedError(f"Unsupported file format: {filename}")
    except Exception as e:
        error_msg = f"CEDD OCR processing failed: {str(e)}"
        logger.error(f"❌ {error_msg}")
        logger.exception(f"Exception details for {filename}:")
        safe_callback(-1, error_msg)
        return []
    finally:
        logger.info(f"🏁 CEDD OCR chunk function finished for file: {filename}")

if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass

    chunk(sys.argv[1], from_page=0, to_page=10, callback=dummy)