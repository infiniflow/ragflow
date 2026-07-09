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

import asyncio
import io
import logging
import os
import re
import tempfile

import numpy as np
from PIL import Image

from api.db.services.llm_service import LLMBundle
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type, get_first_provider_model_name, get_model_config_from_provider_instance, ensure_paddleocr_from_env
from common.constants import LLMType
from common.parser_config_utils import normalize_layout_recognizer
from common.string_utils import clean_markdown_block
from deepdoc.vision import OCR
from rag.nlp import attach_media_context, rag_tokenizer, tokenize

ocr = OCR()

# Gemini supported MIME types
VIDEO_EXTS = [".mp4", ".mov", ".avi", ".flv", ".mpeg", ".mpg", ".webm", ".wmv", ".3gp", ".3gpp", ".mkv"]


def chunk(filename, binary, tenant_id, lang, callback=None, **kwargs):
    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename)),
    }
    eng = lang.lower() == "english"

    parser_config = kwargs.get("parser_config", {}) or {}
    image_ctx = max(0, int(parser_config.get("image_context_size", 0) or 0))

    if any(filename.lower().endswith(ext) for ext in VIDEO_EXTS):
        try:
            doc.update(
                {
                    "doc_type_kwd": "video",
                }
            )
            cv_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.IMAGE2TEXT)
            cv_mdl = LLMBundle(tenant_id, model_config=cv_model_config, lang=lang)
            video_prompt = str(parser_config.get("video_prompt", "") or "")
            ans = asyncio.run(cv_mdl.async_chat(system="", history=[], gen_conf={}, video_bytes=binary, filename=filename, video_prompt=video_prompt))
            callback(0.8, "CV LLM respond: %s ..." % ans[:32])
            ans += "\n" + ans
            tokenize(doc, ans, eng, language=lang)
            return [doc]
        except Exception as e:
            callback(prog=-1, msg=str(e))
    else:
        img = Image.open(io.BytesIO(binary)).convert("RGB")
        doc.update(
            {
                "image": img,
                "doc_type_kwd": "image",
            }
        )

        # Try PaddleOCR if configured as layout_recognize
        txt = _try_paddleocr_image(filename, binary, tenant_id, parser_config, callback)

        if not txt:
            # Fallback to local deepdoc OCR
            bxs = ocr(np.array(img))
            txt = "\n".join([t[0] for _, t in bxs if t[0]])

        callback(0.4, "Finish OCR: (%s ...)" % txt[:12])
        if (eng and len(txt.split()) > 32) or len(txt) > 32:
            tokenize(doc, txt, eng, language=lang)
            callback(0.8, "OCR results is too long to use CV LLM.")
            return attach_media_context([doc], 0, image_ctx)

        try:
            callback(0.4, "Use CV LLM to describe the picture.")
            cv_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.IMAGE2TEXT)
            cv_mdl = LLMBundle(tenant_id, model_config=cv_model_config, lang=lang)
            with io.BytesIO() as img_binary:
                img.save(img_binary, format="JPEG")
                img_binary.seek(0)
                ans = cv_mdl.describe(img_binary.read())
            callback(0.8, "CV LLM respond: %s ..." % ans[:32])
            txt += "\n" + ans
            tokenize(doc, txt, eng, language=lang)
            return attach_media_context([doc], 0, image_ctx)
        except Exception as e:
            callback(prog=-1, msg=str(e))

    return []


def _try_paddleocr_image(filename, binary, tenant_id, parser_config, callback):
    """Try to parse image using PaddleOCR if configured. Returns text or empty string."""
    layout_recognize = parser_config.get("layout_recognize", "")
    if not layout_recognize:
        return ""

    layout_recognizer, parser_model_name = normalize_layout_recognizer(layout_recognize)
    if layout_recognizer != "PaddleOCR":
        return ""

    try:
        paddleocr_llm_name = parser_model_name
        if not paddleocr_llm_name:
            paddleocr_llm_name = get_first_provider_model_name(tenant_id, "PaddleOCR", LLMType.OCR) or ensure_paddleocr_from_env(tenant_id)

        if not paddleocr_llm_name:
            return ""

        ocr_model_config = get_model_config_from_provider_instance(tenant_id, LLMType.OCR, paddleocr_llm_name)
        ocr_model = LLMBundle(tenant_id=tenant_id, model_config=ocr_model_config)
        pdf_parser = ocr_model.mdl

        if not hasattr(pdf_parser, "parse_image"):
            logging.warning("[PaddleOCR] parse_image not available, falling back to local OCR")
            return ""

        callback(0.2, "Using PaddleOCR to parse image...")
        with tempfile.NamedTemporaryFile(suffix=os.path.splitext(filename)[1] or ".png", delete=True) as tmp:
            tmp.write(binary)
            tmp.flush()
            txt = pdf_parser.parse_image(filepath=tmp.name, binary=binary, callback=callback)

        if txt:
            logging.info(f"[PaddleOCR] image parsed successfully: {len(txt)} chars")
            return txt
    except Exception as e:
        logging.warning(f"[PaddleOCR] image parsing failed, falling back to local OCR: {e}")

    return ""


def vision_llm_chunk(binary, vision_model, prompt=None, callback=None):
    """
    A simple wrapper to process image to markdown texts via VLM.

    Returns:
        Simple markdown texts generated by VLM.
    """
    callback = callback or (lambda prog, msg: None)

    img = binary
    txt = ""

    try:
        # Skip tiny crops that fail provider image-size limits.
        if hasattr(img, "size"):
            min_side = 11
            if img.size[0] < min_side or img.size[1] < min_side:
                callback(0.0, f"Skip tiny image for VLM: {img.size[0]}x{img.size[1]}")
                return ""
        with io.BytesIO() as img_binary:
            try:
                img.save(img_binary, format="JPEG")
            except Exception:
                img_binary.seek(0)
                img_binary.truncate()
                img.save(img_binary, format="PNG")

            img_binary.seek(0)
            ans = clean_markdown_block(vision_model.describe_with_prompt(img_binary.read(), prompt))
            txt += "\n" + ans
            return txt

    except Exception as e:
        callback(-1, str(e))

    return ""
