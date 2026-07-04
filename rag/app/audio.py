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
import logging
import os
import re
import tempfile

from common.constants import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
from rag.nlp import rag_tokenizer, tokenize


def chunk(filename, binary, tenant_id, lang, callback=None, **kwargs):
    doc = {"docnm_kwd": filename, "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename))}
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])
    doc["doc_type_kwd"] = "audio"

    # is it English
    is_english = lang.lower() == "english"  # is_english(sections)
    try:
        _, ext = os.path.splitext(filename)
        if not ext:
            raise RuntimeError("No extension detected.")

        if ext not in [".da", ".wave", ".wav", ".mp3", ".aac", ".flac", ".ogg", ".aiff", ".au", ".midi", ".wma", ".realaudio", ".vqf", ".oggvorbis", ".ape"]:
            raise RuntimeError(f"Extension {ext} is not supported yet.")

        tmp_path = ""
        with tempfile.NamedTemporaryFile(suffix=ext, delete=False) as tmpf:
            tmpf.write(binary)
            tmpf.flush()
            tmp_path = os.path.abspath(tmpf.name)

        callback(0.1, "USE Sequence2Txt LLM to transcription the audio")
        seq2txt_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.SPEECH2TEXT)
        seq2txt_mdl = LLMBundle(tenant_id, seq2txt_model_config, lang=lang)
        ans = seq2txt_mdl.transcription(tmp_path)
        callback(0.8, "Sequence2Txt LLM respond: %s ..." % ans[:32])

        # Whisper-compatible models may return a list of timed segments:
        # [{"start": float, "end": float, "text": str}, ...].
        # Preserve timestamps so playback can seek to the right offset.
        if isinstance(ans, list):
            if ans and isinstance(ans[0], dict):
                chunks = []
                for seg in ans:
                    seg_doc = doc.copy()
                    seg_doc["audio_start_flt"] = float(seg.get("start", 0.0))
                    seg_doc["audio_end_flt"] = float(seg.get("end", 0.0))
                    tokenize(seg_doc, seg.get("text", ""), is_english)
                    chunks.append(seg_doc)
                return chunks
            tokenize(doc, "" if not ans else str(ans), is_english)
            return [doc]

        tokenize(doc, ans if isinstance(ans, str) else str(ans), is_english)
        return [doc]
    except Exception as e:
        callback(prog=-1, msg=str(e))
    finally:
        if tmp_path and os.path.exists(tmp_path):
            try:
                os.unlink(tmp_path)
            except Exception as e:
                logging.exception(f"Failed to remove temporary file: {tmp_path}, exception: {e}")
                pass
    return []
