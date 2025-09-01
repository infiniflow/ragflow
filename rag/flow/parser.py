#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

import trio

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from deepdoc.parser import ExcelParser
from deepdoc.parser.pdf_parser import PlainParser, RAGFlowPdfParser, VisionParser
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.llm.cv_model import Base as VLM


class ParserParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.setups = {
            "pdf": {
                "parse_method": "deepdoc",  # deepdoc/plain_text/vlm
                "vlm_name": "",
                "lang": "Chinese",
                "suffix": ["pdf"],
                "output_format": "json",
            },
            "excel": {"output_format": "html"},
            "ppt": {},
            "image": {"parse_method": "ocr"},
            "email": {},
            "text": {},
            "audio": {},
            "video": {},
        }

    def check(self):
        if self.setups["pdf"].get("parse_method") not in ["deepdoc", "plain_text"]:
            assert self.setups["pdf"].get("vlm_name"), "No VLM specified."
            assert self.setups["pdf"].get("lang"), "No language specified."


class Parser(ProcessBase):
    component_name = "Parser"

    def _pdf(self, blob):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on a PDF.")
        conf = self._param.setups["pdf"]
        self.set_output("output_format", conf["output_format"])
        if conf.get("parse_method") == "deepdoc":
            bboxes = RAGFlowPdfParser().parse_into_bboxes(blob, callback=self.callback)
        elif conf.get("parse_method") == "plain_text":
            lines, _ = PlainParser()(blob)
            bboxes = [{"text": t} for t, _ in lines]
        else:
            assert conf.get("vlm_name")
            vision_model = LLMBundle(self._canvas.tenant_id, LLMType.IMAGE2TEXT, llm_name=conf.get("vlm_name"), lang=self.setups["pdf"].get("lang"))
            lines, _ = VisionParser(vision_model=vision_model)(bin, callback=self.callback)
            bboxes = []
            for t, poss in lines:
                pn, x0, x1, top, bott = poss.split(" ")
                bboxes.append({"page_number": int(pn), "x0": int(x0), "x1": int(x1), "top": int(top), "bottom": int(bott), "text": t})

        self.set_output("json", bboxes)
        mkdn = ""
        for b in bboxes:
            if b.get("layout_type", "") == "title":
                mkdn += "\n## "
            if b.get("layout_type", "") == "figure":
                mkdn += "\n![Image]({})".format(VLM.image2base64(b["image"]))
                continue
            mkdn += b.get("text", "") + "\n"
        self.set_output("markdown", mkdn)

    def _excel(self, blob):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on a Excel.")
        conf = self._param.setups["excel"]
        excel_parser = ExcelParser()
        if conf.get("output_format") == "html":
            html = excel_parser.html(blob, 1000000000)
            self.set_output("html", html)
        elif conf.get("output_format") == "json":
            self.set_output("json", [{"text": txt} for txt in excel_parser(blob) if txt])
        elif conf.get("output_format") == "markdown":
            self.set_output("markdown", excel_parser.markdown(blob))

    async def _invoke(self, **kwargs):
        function_map = {
            "pdf": self._pdf,
        }
        for p_type, conf in self._param.setups.items():
            if kwargs.get("name", "").split(".")[-1].lower() not in conf.get("suffix", []):
                continue
            await trio.to_thread.run_sync(function_map[p_type], kwargs["blob"])
            break

