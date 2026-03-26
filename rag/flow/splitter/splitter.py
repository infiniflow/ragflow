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
import logging
import random
import re
from copy import deepcopy

from common.float_utils import normalize_overlapped_percent
from common.token_utils import num_tokens_from_string
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.splitter.pdf_splitter import (
    extract_item_positions,
    restore_pdf_text_previews,
)
from rag.flow.splitter.schema import SplitterFromUpstream
from rag.nlp import naive_merge


class SplitterParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.chunk_token_size = 512
        self.delimiters = ["\n"]
        self.overlapped_percent = 0
        self.children_delimiters = []
        self.table_context_size = 0
        self.image_context_size = 0

    def check(self):
        if self.delimiters is None:
            self.delimiters = []
        elif isinstance(self.delimiters, str):
            self.delimiters = [self.delimiters]
        else:
            self.delimiters = [d for d in self.delimiters if isinstance(d, str)]
        self.delimiters = [d for d in self.delimiters if d]

        if self.children_delimiters is None:
            self.children_delimiters = []
        elif isinstance(self.children_delimiters, str):
            self.children_delimiters = [self.children_delimiters]
        else:
            self.children_delimiters = [d for d in self.children_delimiters if isinstance(d, str)]
        self.children_delimiters = [d for d in self.children_delimiters if d]

        self.check_positive_integer(self.chunk_token_size, "Chunk token size.")
        self.check_decimal_float(self.overlapped_percent, "Overlapped percentage: [0, 1)")
        self.check_nonnegative_number(self.table_context_size, "Table context size.")
        self.check_nonnegative_number(self.image_context_size, "Image context size.")

    def get_input_form(self) -> dict[str, dict]:
        return {}


def _compile_delimiter_pattern(delimiters):
    # Build the primary delimiter regex from active delimiters wrapped by backticks.
    raw_delimiters = "".join(delimiter for delimiter in (delimiters or []) if delimiter)
    custom_delimiters = [m.group(1) for m in re.finditer(r"`([^`]+)`", raw_delimiters)]
    if not custom_delimiters:
        return ""
    return "|".join(re.escape(text) for text in sorted(set(custom_delimiters), key=len, reverse=True))


def _split_text_by_pattern(text, pattern):
    # Split text by the compiled delimiter pattern and keep delimiter text in each chunk.
    if not pattern:
        return [text or ""]

    try:
        compiled_pattern = re.compile(r"(%s)" % pattern, flags=re.DOTALL)
    except re.error as e:
        logging.warning(f"Invalid delimiter regex pattern '{pattern}': {e}. Falling back to unsplit text.")
        return [text or ""]

    split_texts = compiled_pattern.split(text or "")
    chunks = []
    for i in range(0, len(split_texts), 2):
        chunk = split_texts[i]
        if not chunk:
            continue
        if i + 1 < len(split_texts):
            chunk += split_texts[i + 1]
        if chunk.strip():
            chunks.append(chunk)
    return chunks


def _build_json_chunks(json_result, delimiter_pattern):
    # Convert upstream JSON items into internal working chunks.
    chunks = []
    for item in json_result:
        doc_type = str(item.get("doc_type_kwd") or "").strip().lower()
        if doc_type == "table":
            ck_type = "table"
        elif doc_type == "image":
            ck_type = "image"
        else:
            ck_type = "text"

        text = item.get("text")
        if not isinstance(text, str):
            text = item.get("content_with_weight")
        if not isinstance(text, str):
            text = ""

        # Keep PDF coordinates as an internal preview field until the final
        # output is assembled. This avoids leaking two public coordinate
        # formats downstream.
        preview_positions = extract_item_positions(item)
        img_id = item.get("img_id")

        if ck_type == "text":
            text_segments = _split_text_by_pattern(text, delimiter_pattern) if delimiter_pattern else [text]
            for segment in text_segments:
                if not segment or not segment.strip():
                    continue
                chunks.append(
                    {
                        "text": segment,
                        "doc_type_kwd": "text",
                        "ck_type": "text",
                        "_preview_positions": deepcopy(preview_positions),
                        "tk_nums": num_tokens_from_string(segment),
                    }
                )
            continue

        chunks.append(
            {
                "text": text or "",
                "doc_type_kwd": ck_type,
                "ck_type": ck_type,
                "img_id": img_id,
                "_preview_positions": deepcopy(preview_positions),
                "tk_nums": num_tokens_from_string(text or ""),
                "context_above": "",
                "context_below": "",
            }
        )

    return chunks


def _take_sentences(text, need_tokens, from_end=False):
    # Take text from one side until the target token budget is reached.
    split_pat = r"([。!?？；！\n]|\. )"
    texts = re.split(split_pat, text or "", flags=re.DOTALL)
    sentences = []
    for i in range(0, len(texts), 2):
        sentences.append(texts[i] + (texts[i + 1] if i + 1 < len(texts) else ""))
    iterator = reversed(sentences) if from_end else sentences
    collected = ""
    for sentence in iterator:
        collected = sentence + collected if from_end else collected + sentence
        if num_tokens_from_string(collected) >= need_tokens:
            break
    return collected


def _attach_context_to_media_chunks(chunks, table_context_size, image_context_size):
    # Add surrounding text to table/image chunks when context windows are enabled.
    for i, chunk in enumerate(chunks):
        if chunk["ck_type"] not in {"table", "image"}:
            continue

        context_size = image_context_size if chunk["ck_type"] == "image" else table_context_size
        if context_size <= 0:
            continue

        remain_above = context_size
        remain_below = context_size
        parts_above = []
        parts_below = []

        prev = i - 1
        while prev >= 0 and remain_above > 0:
            prev_chunk = chunks[prev]
            if prev_chunk["ck_type"] == "text":
                if prev_chunk["tk_nums"] >= remain_above:
                    parts_above.insert(0, _take_sentences(prev_chunk["text"], remain_above, from_end=True))
                    remain_above = 0
                    break
                parts_above.insert(0, prev_chunk["text"])
                remain_above -= prev_chunk["tk_nums"]
            prev -= 1

        after = i + 1
        while after < len(chunks) and remain_below > 0:
            after_chunk = chunks[after]
            if after_chunk["ck_type"] == "text":
                if after_chunk["tk_nums"] >= remain_below:
                    parts_below.append(_take_sentences(after_chunk["text"], remain_below))
                    remain_below = 0
                    break
                parts_below.append(after_chunk["text"])
                remain_below -= after_chunk["tk_nums"]
            after += 1

        chunk["context_above"] = "".join(parts_above)
        chunk["context_below"] = "".join(parts_below)


def _merge_text_chunks_by_token_size(chunks, chunk_token_size, overlapped_percent):
    # Merge adjacent text chunks when delimiter-based splitting is not active.
    merged = []
    prev_text_idx = -1
    threshold = chunk_token_size * (100 - overlapped_percent) / 100.0

    for chunk in chunks:
        if chunk["ck_type"] != "text":
            merged.append(deepcopy(chunk))
            prev_text_idx = -1
            continue

        current = deepcopy(chunk)
        should_start_new = prev_text_idx < 0 or merged[prev_text_idx]["tk_nums"] > threshold
        if should_start_new:
            if prev_text_idx >= 0 and overlapped_percent > 0 and merged[prev_text_idx]["text"]:
                overlapped = merged[prev_text_idx]["text"]
                overlap_start = int(len(overlapped) * (100 - overlapped_percent) / 100.0)
                current["text"] = overlapped[overlap_start:] + current["text"]
                current["tk_nums"] = num_tokens_from_string(current["text"])
            merged.append(current)
            prev_text_idx = len(merged) - 1
            continue

        if merged[prev_text_idx]["text"] and current["text"]:
            merged[prev_text_idx]["text"] += "\n" + current["text"]
        else:
            merged[prev_text_idx]["text"] += current["text"]
        merged[prev_text_idx]["_preview_positions"].extend(current.get("_preview_positions") or [])
        merged[prev_text_idx]["tk_nums"] += current["tk_nums"]

    return merged


def _finalize_json_chunks(chunks):
    # Convert internal chunks back to the splitter output format.
    docs = []
    for chunk in chunks:
        text = (chunk.get("context_above") or "") + (chunk.get("text") or "") + (chunk.get("context_below") or "")
        if not text.strip():
            continue

        # The internal preview coordinates are converted exactly once into the
        # indexed fields consumed downstream.
        position_int = []
        page_num_int = []
        top_int = []
        for pos in chunk.get("_preview_positions") or []:
            if not isinstance(pos, (list, tuple)) or len(pos) < 5:
                continue
            try:
                page_no = int(pos[0])
                left = int(pos[1])
                right = int(pos[2])
                top = int(pos[3])
                bottom = int(pos[4])
                position_int.append(
                    (
                        page_no,
                        left,
                        right,
                        top,
                        bottom,
                    )
                )
                page_num_int.append(page_no)
                top_int.append(top)
            except (TypeError, ValueError):
                continue

        doc = {
            "text": text,
            "position_int": deepcopy(position_int),
            "page_num_int": deepcopy(page_num_int),
            "top_int": deepcopy(top_int),
            "doc_type_kwd": chunk.get("doc_type_kwd", "text"),
        }
        if chunk.get("mom"):
            doc["mom"] = chunk["mom"]
        if chunk.get("img_id"):
            doc["img_id"] = chunk["img_id"]
        docs.append(doc)

    return docs


def _split_chunk_docs_by_children(chunks, pattern):
    # Apply the secondary children_delimiters split to text chunks only.
    if not pattern:
        return chunks

    docs = []
    for chunk in chunks:
        if chunk.get("doc_type_kwd", "text") != "text":
            docs.append(chunk)
            continue

        split_texts = _split_text_by_pattern(chunk.get("text", ""), pattern)
        if not split_texts:
            docs.append(chunk)
            continue

        mom = chunk.get("text", "")
        for text in split_texts:
            if not text.strip():
                continue
            child = deepcopy(chunk)
            child["mom"] = mom
            child["text"] = text
            docs.append(child)

    return docs


def _split_plain_payload(payload, delimiter_pattern, chunk_token_size, overlapped_percent):
    # Plain text uses delimiter splitting first and token-size splitting as fallback.
    if delimiter_pattern:
        return _split_text_by_pattern(payload, delimiter_pattern)
    return naive_merge(payload, chunk_token_size, "", overlapped_percent)


class Splitter(ProcessBase):
    component_name = "Splitter"

    async def _invoke(self, **kwargs):
        try:
            from_upstream = SplitterFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        # Build the primary delimiter regex. If no active custom delimiter exists,
        # the splitter falls back to token-size based merging.
        delimiter_pattern = _compile_delimiter_pattern(self._param.delimiters)
        custom_pattern = "|".join(re.escape(t) for t in sorted(set(self._param.children_delimiters), key=len, reverse=True))

        self.set_output("output_format", "chunks")
        self.callback(random.randint(1, 5) / 100.0, "Start to split into chunks.")
        overlapped_percent = normalize_overlapped_percent(self._param.overlapped_percent)
        if from_upstream.output_format in ["markdown", "text", "html"]:
            if from_upstream.output_format == "markdown":
                payload = from_upstream.markdown_result
            elif from_upstream.output_format == "text":
                payload = from_upstream.text_result
            else:  # == "html"
                payload = from_upstream.html_result

            if not payload:
                payload = ""

            cks = _split_plain_payload(
                payload,
                delimiter_pattern,
                self._param.chunk_token_size,
                overlapped_percent,
            )
            if custom_pattern:
                docs = []
                for c in cks:
                    if not c.strip():
                        continue
                    split_sec = re.split(r"(%s)" % custom_pattern, c, flags=re.DOTALL)
                    if split_sec:
                        for j in range(0, len(split_sec), 2):
                            if not split_sec[j].strip():
                                continue
                            docs.append({
                                "text": split_sec[j],
                                "mom": c
                            })
                    else:
                        docs.append({"text": c})
                self.set_output("chunks", docs)
            else:
                self.set_output("chunks", [{"text": c.strip()} for c in cks if c.strip()])

            self.callback(1, "Done.")
            return

        # json
        json_result = from_upstream.json_result or []
        # Structured JSON input is normalized first, then optionally enriched with
        # media context, and finally merged only when delimiter splitting is inactive.
        chunks = _build_json_chunks(json_result, delimiter_pattern)
        _attach_context_to_media_chunks(chunks, self._param.table_context_size, self._param.image_context_size)
        if not delimiter_pattern:
            chunks = _merge_text_chunks_by_token_size(chunks, self._param.chunk_token_size, overlapped_percent)

        if custom_pattern:
            chunks = _split_chunk_docs_by_children(chunks, custom_pattern)

        await restore_pdf_text_previews(chunks, from_upstream, self._canvas)
        cks = _finalize_json_chunks(chunks)
        self.set_output("chunks", cks)
        self.callback(1, "Done.")
