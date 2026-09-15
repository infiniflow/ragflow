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
import random
import re
from copy import deepcopy

from common.float_utils import normalize_overlapped_percent
from common.token_utils import num_tokens_from_string
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.chunker._sentence_boundary import SENTENCE_BOUNDARY_PATTERN
from rag.flow.chunker.schema import TokenChunkerFromUpstream
from rag.flow.parser.pdf_chunk_metadata import (
    PDF_POSITIONS_KEY,
    extract_pdf_positions,
    finalize_pdf_chunk,
    restore_pdf_text_previews,
)
from rag.nlp import naive_merge
from rag.nlp.delim import DEFAULT_DELIMITER

# _TAG_RE matches parser-emitted coordinate tags of the form
# ``@@<page>\t<left>\t<right>\t<top>\t<bottom>##``. Mirrors Go's
# posTagRemove (internal/ingestion/component/chunker/group.go) so the two
# languages strip tags identically.
_TAG_RE = re.compile(r"@@[\t0-9.-]+?##")


def remove_tag(text):
    """Strip ``@@...##`` coordinate tags from text.

    Used both when measuring the overlap prefix (so the cut lands on the
    tag-free visible text, matching Go's computeOverlapPrefix) and on the
    final chunk text (so coordinate tags never reach embedding/index).
    """
    return _TAG_RE.sub("", text or "")


class TokenChunkerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.delimiter_mode = "delimiter"
        self.chunk_token_size = 512
        self.delimiters = list(DEFAULT_DELIMITER)
        self.overlapped_percent = 0
        self.children_delimiters = []
        self.table_context_size = 0
        self.image_context_size = 0

    def check(self):
        # Backward-compat: "token_size" was removed but is behaviorally identical
        # to "delimiter" at runtime (both route through the same code path), so
        # accept and coerce it instead of rejecting legacy configs / pre-fix
        # frontends. Only genuinely unknown values are rejected.
        if self.delimiter_mode == "token_size":
            self.delimiter_mode = "delimiter"
        self.check_valid_value(self.delimiter_mode, "Delimiter mode abnormal.", ["delimiter", "one"])
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
    # Split text by the compiled delimiter pattern and discard delimiters.
    # No atom-split is performed; empty segments between consecutive delimiters
    # are dropped but whitespace-only segments are preserved (the delimiter is
    # the boundary, not stripped away).
    if not pattern:
        return [text or ""]

    split_texts = re.split("(" + pattern + ")", text or "", flags=re.DOTALL)
    chunks = []
    for i in range(0, len(split_texts), 2):
        chunk = split_texts[i]
        if chunk:
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
        preview_positions = extract_pdf_positions(item)
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
                        PDF_POSITIONS_KEY: deepcopy(preview_positions),
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
                PDF_POSITIONS_KEY: deepcopy(preview_positions),
                "tk_nums": num_tokens_from_string(text or ""),
                "context_above": "",
                "context_below": "",
            }
        )

    return chunks


def _take_sentences(text, need_tokens, from_end=False):
    # Take text from one side until the target token budget is reached.
    texts = re.split(SENTENCE_BOUNDARY_PATTERN, text or "", flags=re.DOTALL)
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


def _overlap_tail_positions(prev_items, overlap_start):
    # Given the source items a previous chunk was built from (each as
    # ``(item_text, pos_group)`` where ``pos_group`` is that item's
    # ``_pdf_positions`` list), return the flattened boxes whose item visible
    # span intersects the overlap tail ``[overlap_start, total)``.
    #
    # PDF positions are per-item (coarse), so an item is included wholesale once
    # any part of it falls in the overlap tail. This keeps the overlap prefix
    # highlighted without over-inflating the box set with the previous chunk's
    # non-overlap (head) coordinates (#18148).
    if not prev_items:
        return []
    # Items are concatenated with a single "\n" separator, matching the merge
    # join at _merge_text_chunks_by_token_size.
    spans = []
    offset = 0
    for item_text, _pos_group in prev_items:
        start = offset
        end = offset + len(item_text)
        spans.append((start, end))
        offset = end + 1
    if not spans:
        return []
    total = spans[-1][1]
    out = []
    for (start, end), (_text, pos_group) in zip(spans, prev_items, strict=True):
        if start < total and end > overlap_start:
            out.extend(pos_group or [])
    return out


def _merge_text_chunks_by_token_size(chunks, chunk_token_size, overlapped_percent):
    # Merge adjacent text chunks when delimiter-based splitting is not active.
    merged = []
    # Parallel to ``merged``: for each merged chunk, the list of source items it
    # was built from, each as ``(item_text, pos_group)``. Tracking items (not
    # just the flattened position list) lets us map a visible-text offset range
    # back to the exact coordinate boxes that belong to it, so the overlap
    # prefix carries only the previous chunk's tail coordinates instead of
    # dropping them (#18148).
    merged_items = []
    prev_text_idx = -1
    threshold = chunk_token_size * (100 - overlapped_percent) / 100.0

    for chunk in chunks:
        if chunk["ck_type"] != "text":
            merged.append(deepcopy(chunk))
            merged_items.append(None)
            prev_text_idx = -1
            continue

        current = deepcopy(chunk)
        current_item = (current["text"], list(current.get(PDF_POSITIONS_KEY) or []))
        should_start_new = prev_text_idx < 0 or merged[prev_text_idx]["tk_nums"] > threshold
        # #17799: an over-budget unit stands alone — never merged into the
        # previous chunk. This matches Python naive_merge and the Go
        # TokenChunker (all three paths stand the over-budget unit alone), so
        # the Python JSON path, Python text path, and Go TokenChunker share one
        # contract.
        if current["tk_nums"] > chunk_token_size:
            should_start_new = True
        if should_start_new:
            if prev_text_idx >= 0 and overlapped_percent > 0 and merged[prev_text_idx]["text"]:
                # Mirror Go computeOverlapPrefix: measure the overlap cut on the
                # tag-free *visible* text, never on the raw text that still
                # carries @@...## coordinate tags. This keeps the overlap prefix
                # aligned with Go and prevents a partial tag from leaking into
                # the next chunk when the cut would land inside a tag.
                visible = remove_tag(merged[prev_text_idx]["text"])
                overlap_start = int(len(visible) * (100 - overlapped_percent) / 100.0)
                if 0 <= overlap_start < len(visible):
                    overlap_text = visible[overlap_start:]
                    # Carry the previous chunk's tail coordinates so the overlap
                    # prefix is highlighted, not just the cur span (#18148).
                    # Only the items intersecting the overlap tail keep their
                    # boxes; the head (non-overlap) boxes are excluded so the
                    # highlight is not over-inflated.
                    overlap_positions = _overlap_tail_positions(merged_items[prev_text_idx], overlap_start)
                else:
                    overlap_text = ""
                    overlap_positions = []
                current["text"] = overlap_text + current["text"]
                current[PDF_POSITIONS_KEY] = overlap_positions + (current.get(PDF_POSITIONS_KEY) or [])
                current_item = (current["text"], list(current.get(PDF_POSITIONS_KEY) or []))
                current["tk_nums"] = num_tokens_from_string(current["text"])
            merged.append(current)
            merged_items.append([current_item])
            prev_text_idx = len(merged) - 1
            continue

        if merged[prev_text_idx]["text"] and current["text"]:
            merged[prev_text_idx]["text"] += "\n" + current["text"]
        else:
            merged[prev_text_idx]["text"] += current["text"]
        merged[prev_text_idx][PDF_POSITIONS_KEY].extend(current.get(PDF_POSITIONS_KEY) or [])
        merged[prev_text_idx]["tk_nums"] += current["tk_nums"]
        merged_items[prev_text_idx].append(current_item)

    return merged


def _finalize_json_chunks(chunks):
    # Convert internal chunks into the final token chunker output format.
    docs = []
    for chunk in chunks:
        # Strip parser coordinate tags from the final text so they never
        # reach embedding/index (coordinates already live in the structured
        # PDF_POSITIONS_KEY field). Mirrors Go's removeTag at the chunker
        # output boundary (token.go:544).
        text = remove_tag((chunk.get("context_above") or "") + (chunk.get("text") or "") + (chunk.get("context_below") or ""))
        if not text.strip():
            continue

        # The internal preview coordinates are converted exactly once into the
        # indexed fields consumed downstream.
        doc = {
            "text": text,
            "doc_type_kwd": chunk.get("doc_type_kwd", "text"),
        }
        if chunk.get(PDF_POSITIONS_KEY):
            doc[PDF_POSITIONS_KEY] = deepcopy(chunk[PDF_POSITIONS_KEY])
        if chunk.get("mom"):
            doc["mom"] = chunk["mom"]
        if chunk.get("img_id"):
            doc["img_id"] = chunk["img_id"]
        docs.append(finalize_pdf_chunk(doc))

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

        mom = chunk.get("text", "").removeprefix("\n")
        for text in split_texts:
            if not text.strip():
                continue
            child = deepcopy(chunk)
            child["mom"] = mom
            child["text"] = text
            docs.append(child)

    return docs


class TokenChunker(ProcessBase):
    component_name = "TokenChunker"

    async def _invoke(self, **kwargs):
        try:
            from_upstream = TokenChunkerFromUpstream.model_validate(kwargs)
        except Exception as e:  # noqa: BLE001
            self.set_output("_ERROR", f"Input error: {e!s}")
            return

        # Build the primary delimiter regex. If no active custom delimiter exists,
        # the token chunker falls back to token-size based merging.
        delimiter_pattern = _compile_delimiter_pattern(self._param.delimiters)
        custom_pattern = "|".join(re.escape(t) for t in sorted(set(self._param.children_delimiters), key=len, reverse=True))

        self.set_output("output_format", "chunks")
        self.callback(random.randint(1, 5) / 100.0, "Start to split into chunks.")
        overlapped_percent = normalize_overlapped_percent(self._param.overlapped_percent)
        if from_upstream.output_format in ["markdown", "text", "html"]:
            payload = getattr(from_upstream, f"{from_upstream.output_format}_result") or ""
            if self._param.delimiter_mode == "one":
                # Strip parser coordinate tags so they never reach
                # embedding/index (consistent with the JSON merge path).
                self.set_output("chunks", [{"text": remove_tag(payload)}] if payload.strip() else [])
                self.callback(1, "Done.")
                return
            if delimiter_pattern:
                cks = _split_text_by_pattern(payload, delimiter_pattern)
            else:
                cks = naive_merge(
                    payload,
                    self._param.chunk_token_size,
                    "".join(self._param.delimiters),
                    overlapped_percent,
                )
            if custom_pattern:
                docs = []
                for c in cks:
                    if not c.strip():
                        continue
                    for text in _split_text_by_pattern(c, custom_pattern):
                        if not text.strip():
                            continue
                        docs.append({"text": text, "mom": c.removeprefix("\n")})
                self.set_output("chunks", docs)
            else:
                self.set_output("chunks", [{"text": c.strip()} for c in cks if c.strip()])

            self.callback(1, "Done.")
            return

        # json
        json_result = (from_upstream.chunks if from_upstream.output_format == "chunks" else from_upstream.json_result) or []
        if self._param.delimiter_mode == "one":
            sections = []
            for item in json_result:
                text = item.get("text")
                if not isinstance(text, str):
                    text = item.get("content_with_weight")
                if isinstance(text, str) and text.strip():
                    # Strip parser coordinate tags so they never reach
                    # embedding/index (consistent with the JSON merge path).
                    sections.append(remove_tag(text))
            merged_text = "\n".join(sections)
            self.set_output("chunks", [{"text": merged_text}] if merged_text.strip() else [])
            self.callback(1, "Done.")
            return

        # Both branches start from per-item chunks (no pre-split by the
        # delimiter pattern). The delimiter branch splits the buffered text
        # stream while preserving per-segment PDF positions; the no-delimiter
        # branch merges adjacent text items to chunk_token_size (the removed
        # "token_size" behaviour, and a parity match with the Go JSON path).
        text_chunks = _build_json_chunks(json_result, "")

        if delimiter_pattern:
            chunks = []
            text_buffer = []
            text_buffer_pos = []

            def flush_text_buffer():
                if not text_buffer:
                    return
                # Join buffered text items with "\n" so adjacent item text is not
                # glued together (e.g. "hello" + "world" must not become "helloworld").
                # The delimiter is then applied to the combined text; a segment may
                # span across item boundaries (the "\n" glue is not itself a
                # delimiter), so each segment carries only the PDF positions of the
                # item(s) that contributed to it -- never the union of every item
                # (which previously leaked page-N coordinates into page-M chunks and
                # made all segments share one preview image).
                parts = []
                item_ranges = []  # (start, end) of each buffered item in combined_text
                offset = 0
                for text in text_buffer:
                    start = offset
                    parts.append(text)
                    offset += len(text)
                    item_ranges.append((start, offset))
                    parts.append("\n")
                    offset += 1
                combined_text = "".join(parts[:-1])  # drop the trailing glue

                raw = re.split("(" + delimiter_pattern + ")", combined_text, flags=re.DOTALL)
                segments = []  # (text, start, end) within combined_text
                pos = 0
                for i in range(0, len(raw), 2):
                    seg = raw[i]
                    seg_start = pos
                    seg_end = pos + len(seg)
                    if seg:
                        segments.append((seg, seg_start, seg_end))
                    pos = seg_end
                    if i + 1 < len(raw):
                        pos += len(raw[i + 1])

                for text, seg_start, seg_end in segments:
                    if not text.strip():
                        continue
                    seg_pos = []
                    for (istart, iend), item_pos in zip(item_ranges, text_buffer_pos, strict=True):
                        # A segment overlaps an item when their character ranges
                        # intersect; collect that item's coordinates.
                        if seg_start < iend and istart < seg_end:
                            seg_pos.extend(item_pos or [])
                    chunks.append(
                        {
                            "text": text,
                            "doc_type_kwd": "text",
                            "ck_type": "text",
                            PDF_POSITIONS_KEY: deepcopy(seg_pos),
                            "tk_nums": num_tokens_from_string(text),
                        }
                    )
                text_buffer.clear()
                text_buffer_pos.clear()

            for chunk in text_chunks:
                if chunk["ck_type"] == "text":
                    text_buffer.append(chunk["text"])
                    text_buffer_pos.append(chunk.get(PDF_POSITIONS_KEY))
                else:
                    flush_text_buffer()
                    chunks.append(chunk)
            flush_text_buffer()
            # Apply children_delimiters (secondary split) before finalizing.
            if custom_pattern:
                chunks = _split_chunk_docs_by_children(chunks, custom_pattern)
            _attach_context_to_media_chunks(chunks, self._param.table_context_size, self._param.image_context_size)
        else:
            # No active delimiter: merge adjacent text items to chunk_token_size.
            # This runs on the per-item chunks (NOT a single concatenated chunk),
            # so the token cap is actually enforced -- matching the previous
            # "token_size" mode and the Go JSON path. Media chunks break the merge.
            # Media context is attached on the per-item chunks before merging, as
            # the removed "token_size" branch did, to preserve context windows.
            _attach_context_to_media_chunks(text_chunks, self._param.table_context_size, self._param.image_context_size)
            chunks = _merge_text_chunks_by_token_size(text_chunks, self._param.chunk_token_size, overlapped_percent)
            if custom_pattern:
                chunks = _split_chunk_docs_by_children(chunks, custom_pattern)

        await restore_pdf_text_previews(chunks, from_upstream, self._canvas)
        self.set_output("chunks", _finalize_json_chunks(chunks))
        self.callback(1, "Done.")
        return
