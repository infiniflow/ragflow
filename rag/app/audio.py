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
import copy
import logging
import os
import re
import tempfile

from common.constants import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
from rag.nlp import rag_tokenizer, tokenize

_SUPPORTED_EXTS = {".da", ".wave", ".wav", ".mp3", ".aac", ".flac", ".ogg", ".aiff", ".au", ".midi", ".wma",
                   ".realaudio", ".vqf", ".oggvorbis", ".ape"}

_DEFAULT_WINDOW_SECONDS = 60


def _build_windowed_chunks(base_doc, segments, window_seconds, is_english):
    """Group ASR segments into fixed-duration windows; each window becomes a chunk.

    Each chunk carries:
      start_time_flt  — window start in seconds from file start
      end_time_flt    — window end in seconds from file start
      chunk_order_int — 0-based window index for ordering
    """
    chunks = []
    window_texts = []
    window_start = 0.0
    window_end = 0.0
    order = 0

    for seg in segments:
        seg_start = seg.get("start", 0.0)
        seg_end = seg.get("end", 0.0)
        text = seg.get("text", "").strip()
        if not text:
            continue

        if not window_texts:
            window_start = seg_start

        window_texts.append(text)
        window_end = seg_end

        # Close window when its duration reaches the threshold
        if window_end - window_start >= window_seconds:
            d = copy.deepcopy(base_doc)
            d["start_time_flt"] = window_start
            d["end_time_flt"] = window_end
            d["chunk_order_int"] = order
            d["doc_type_kwd"] = "audio"
            tokenize(d, " ".join(window_texts), is_english)
            chunks.append(d)
            order += 1
            window_texts = []
            window_start = 0.0
            window_end = 0.0

    # Flush the remaining partial window
    if window_texts:
        d = copy.deepcopy(base_doc)
        d["start_time_flt"] = window_start
        d["end_time_flt"] = window_end
        d["chunk_order_int"] = order
        d["doc_type_kwd"] = "audio"
        tokenize(d, " ".join(window_texts), is_english)
        chunks.append(d)

    return chunks


def chunk(filename, binary, tenant_id, lang, callback=None, **kwargs):
    doc = {"docnm_kwd": filename, "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename))}
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])

    is_english = lang.lower() == "english"
    parser_config = kwargs.get("parser_config", {}) or {}
    window_seconds = float(parser_config.get("chunk_duration_seconds", _DEFAULT_WINDOW_SECONDS) or _DEFAULT_WINDOW_SECONDS)

    tmp_path = ""
    try:
        _, ext = os.path.splitext(filename)
        if not ext:
            raise RuntimeError("No extension detected.")
        if ext not in _SUPPORTED_EXTS:
            raise RuntimeError(f"Extension {ext} is not supported yet.")

        with tempfile.NamedTemporaryFile(suffix=ext, delete=False) as tmpf:
            tmpf.write(binary)
            tmpf.flush()
            tmp_path = os.path.abspath(tmpf.name)

        callback(0.1, "USE Sequence2Txt LLM to transcription the audio")
        seq2txt_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.SPEECH2TEXT)
        seq2txt_mdl = LLMBundle(tenant_id, seq2txt_model_config, lang=lang)

        segments = seq2txt_mdl.transcription_with_timestamps(tmp_path)
        callback(0.8, "Sequence2Txt LLM responded with %d segment(s)" % len(segments))

        has_timing = any(s.get("end", 0.0) > 0.0 for s in segments)
        if has_timing:
            chunks = _build_windowed_chunks(doc, segments, window_seconds, is_english)
        else:
            # Provider returned no timestamps — fall back to single chunk (backward-compat)
            full_text = " ".join(s.get("text", "") for s in segments).strip()
            tokenize(doc, full_text, is_english)
            doc["doc_type_kwd"] = "audio"
            chunks = [doc]

        return chunks
    except Exception as e:
        callback(prog=-1, msg=str(e))
    finally:
        if tmp_path and os.path.exists(tmp_path):
            try:
                os.unlink(tmp_path)
            except Exception as exc:
                logging.exception(f"Failed to remove temporary file: {tmp_path}, exception: {exc}")
    return []
