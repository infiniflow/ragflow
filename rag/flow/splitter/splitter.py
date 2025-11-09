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
from functools import partial

import trio

from common.misc_utils import get_uuid
from rag.utils.base64_image import id2image, image2id
from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.splitter.schema import SplitterFromUpstream
from rag.nlp import naive_merge, naive_merge_with_images
from common import settings


class SplitterParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.chunk_token_size = 512
        self.delimiters = ["\n"]
        self.overlapped_percent = 0

    def check(self):
        self.check_empty(self.delimiters, "Delimiters.")
        self.check_positive_integer(self.chunk_token_size, "Chunk token size.")
        self.check_decimal_float(self.overlapped_percent, "Overlapped percentage: [0, 1)")

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

        self.set_output("output_format", "chunks")
        self.callback(random.randint(1, 5) / 100.0, "Start to split into chunks.")
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
                self._param.overlapped_percent,
            )
            self.set_output("chunks", [{"text": c.strip()} for c in cks if c.strip()])

            self.callback(1, "Done.")
            return

        # json
        sections, section_images = [], []
        for o in from_upstream.json_result or []:
            sections.append((o.get("text", ""), o.get("position_tag", "")))
            section_images.append(id2image(o.get("img_id"), partial(settings.STORAGE_IMPL.get, tenant_id=self._canvas._tenant_id)))

        chunks, images = naive_merge_with_images(
            sections,
            section_images,
            self._param.chunk_token_size,
            deli,
            self._param.overlapped_percent,
        )
        cks = [
            {
                "text": RAGFlowPdfParser.remove_tag(c),
                "image": img,
                "positions": [[pos[0][-1]+1, *pos[1:]] for pos in RAGFlowPdfParser.extract_positions(c)],
            }
            for c, img in zip(chunks, images) if c.strip()
        ]
        async with trio.open_nursery() as nursery:
            for d in cks:
                nursery.start_soon(image2id, d, partial(settings.STORAGE_IMPL.put, tenant_id=self._canvas._tenant_id), get_uuid())
        self.set_output("chunks",  cks)
        self.callback(1, "Done.")
