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
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from deepdoc.parser.pdf_parser import RAGFlowPdfParser, PlainParser, VisionParser
from rag.flow.base import ProcessBase, ProcessParamBase


class ParserParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.setups = {
            "pdf": {
                "parse_method": "deepdoc"
            },
            "excel": {},
            "ppt": {},
            "image": {
                "parse_method": "ocr"
            },
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

    def _pdf(self, bin):
        if self._param.setups["pdf"].get("parse_method") == "deepdoc":
            bboxes = RAGFlowPdfParser().parse_into_bboxes(bin)
        elif self._param.setups["pdf"].get("parse_method") == "plain_text":
            lines,_ = PlainParser()(bin)
            bboxes = [{"text": t} for t,_ in lines]
        else:
            assert self._param.setups["pdf"].get("vlm_name")
            vision_model = LLMBundle(self._canvas.tenant_id, LLMType.IMAGE2TEXT, llm_name=self._param.setups["pdf"].get("vlm_name"), lang=self.setups["pdf"].get("lang"))
            lines, _ = VisionParser(vision_model=vision_model)(bin, callback=None)
            bboxes = [{"text": t} for t,_ in lines]

    def _invoke(self, **kwargs):
        pass