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
import asyncio
import logging
import random
import re
from copy import deepcopy
from functools import partial
from common.misc_utils import get_uuid
from rag.utils.base64_image import id2image, image2id
from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.splitter.schema import SplitterFromUpstream
from common.float_utils import normalize_overlapped_percent
from rag.nlp import attach_media_context, naive_merge, naive_merge_with_images
from common import settings


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
        self.check_empty(self.delimiters, "Delimiters.")
        self.check_positive_integer(self.chunk_token_size, "Chunk token size.")
        self.check_decimal_float(self.overlapped_percent, "Overlapped percentage: [0, 1)")
        self.check_nonnegative_number(self.table_context_size, "Table context size.")
        self.check_nonnegative_number(self.image_context_size, "Image context size.")

    def get_input_form(self) -> dict[str, dict]:
        return {}


class Splitter(ProcessBase):
    component_name = "Splitter"

    async def _invoke(self, **kwargs):
        try:
            from_upstream = SplitterFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        deli = ""
        for d in self._param.delimiters:
            if len(d) > 1:
                deli += f"`{d}`"
            else:
                deli += d
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

            cks = naive_merge(
                payload,
                self._param.chunk_token_size,
                deli,
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
        if self._param.table_context_size or self._param.image_context_size:
            for ck in json_result:
                if "image" not in ck and ck.get("img_id") and not (isinstance(ck.get("text"), str) and ck.get("text").strip()):
                    ck["image"] = True
            attach_media_context(json_result, self._param.table_context_size, self._param.image_context_size)
            for ck in json_result:
                if ck.get("image") is True:
                    del ck["image"]

        sections, section_images = [], []
        for o in json_result:
            sections.append((o.get("text", ""), o.get("position_tag", "")))
            section_images.append(id2image(o.get("img_id"), partial(settings.STORAGE_IMPL.get, tenant_id=self._canvas._tenant_id)))

        chunks, images = naive_merge_with_images(
            sections,
            section_images,
            self._param.chunk_token_size,
            deli,
            overlapped_percent,
        )
        cks = [
            {
                "text": RAGFlowPdfParser.remove_tag(c),
                "image": img,
                "positions": [[pos[0][-1], *pos[1:]] for pos in RAGFlowPdfParser.extract_positions(c)]
            }
            for c, img in zip(chunks, images) if c.strip()
        ]
        tasks = []
        for d in cks:
            tasks.append(asyncio.create_task(image2id(d, partial(settings.STORAGE_IMPL.put, tenant_id=self._canvas._tenant_id), get_uuid())))
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error(f"error when splitting: {e}")
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise

        if custom_pattern:
            docs = []
            for c in cks:
                split_sec = re.split(r"(%s)" % custom_pattern, c["text"], flags=re.DOTALL)
                if split_sec:
                    c["mom"] = c["text"]
                    for j in range(0, len(split_sec), 2):
                        if not split_sec[j].strip():
                            continue
                        cc = deepcopy(c)
                        cc["text"] = split_sec[j]
                        docs.append(cc)
                else:
                    docs.append(c)
            self.set_output("chunks", docs)
        else:
            self.set_output("chunks",  cks)
        self.callback(1, "Done.")
