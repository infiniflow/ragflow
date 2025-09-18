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
import io
import logging
import random

import trio
import numpy as np
from PIL import Image

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from deepdoc.parser import ExcelParser
from deepdoc.parser.pdf_parser import PlainParser, RAGFlowPdfParser, VisionParser
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.parser.schema import ParserFromUpstream
from rag.llm.cv_model import Base as VLM


class ParserParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.allowed_output_format = {
            "pdf": [
                "json",
                "markdown",
            ],
            "spreadsheet": [
                "json",
                "markdown",
                "html",
            ],
            "word": [
                "json",
            ],
            "ppt": [],
            "image": [
                "text"
            ],
            "email": [],
            "text": [
                "text",
                "json"
            ],
            "audio": [
                "json"
            ],
            "video": [],
        }

        self.setups = {
            "pdf": {
                "parse_method": "deepdoc",  # deepdoc/plain_text/vlm
                "llm_id": "",
                "lang": "Chinese",
                "suffix": [
                    "pdf",
                ],
                "output_format": "json",
            },
            "spreadsheet": {
                "output_format": "html",
                "suffix": [
                    "xls",
                    "xlsx",
                    "csv",
                ],
            },
            "word": {
                "suffix": [
                    "doc",
                    "docx",
                ],
                "output_format": "json",
            },
            "markdown": {
                "suffix": ["md", "markdown"],
                "output_format": "json",
            },
            "ppt": {},
            "image": {
                "parse_method": ["ocr", "vlm"],
                "llm_id": "",
                "lang": "Chinese",
                "suffix": ["jpg", "jpeg", "png", "gif"],
                "output_format": "json",
            },
            "email": {},
            "text": {
                "suffix": [
                    "txt"
                ],
                "output_format": "json",
            },
            "audio": {
                "suffix":[
                    "da",
                    "wave",
                    "wav",
                    "mp3",
                    "aac",
                    "flac",
                    "ogg",
                    "aiff",
                    "au",
                    "midi",
                    "wma",
                    "realaudio",
                    "vqf",
                    "oggvorbis",
                    "ape"
                ],
                "output_format": "json",
            },
            "video": {},
        }

    def check(self):
        pdf_config = self.setups.get("pdf", {})
        if pdf_config:
            pdf_parse_method = pdf_config.get("parse_method", "")
            self.check_valid_value(pdf_parse_method.lower(), "Parse method abnormal.", ["deepdoc", "plain_text", "vlm"])

            if pdf_parse_method not in ["deepdoc", "plain_text"]:
                self.check_empty(pdf_config.get("llm_id"), "VLM")

            pdf_language = pdf_config.get("lang", "")
            self.check_empty(pdf_language, "Language")

            pdf_output_format = pdf_config.get("output_format", "")
            self.check_valid_value(pdf_output_format, "PDF output format abnormal.", self.allowed_output_format["pdf"])

        spreadsheet_config = self.setups.get("spreadsheet", "")
        if spreadsheet_config:
            spreadsheet_output_format = spreadsheet_config.get("output_format", "")
            self.check_valid_value(spreadsheet_output_format, "Spreadsheet output format abnormal.", self.allowed_output_format["spreadsheet"])

        doc_config = self.setups.get("doc", "")
        if doc_config:
            doc_output_format = doc_config.get("output_format", "")
            self.check_valid_value(doc_output_format, "Word processer document output format abnormal.", self.allowed_output_format["doc"])

        image_config = self.setups.get("image", "")
        if image_config:
            image_parse_method = image_config.get("parse_method", "")
            self.check_valid_value(image_parse_method.lower(), "Parse method abnormal.", ["ocr", "vlm"])
            if image_parse_method not in ["ocr"]:
                self.check_empty(image_config.get("llm_id"), "VLM")

            image_language = image_config.get("lang", "")
            self.check_empty(image_language, "Language")

        text_config = self.setups.get("text", "")
        if text_config:
            text_output_format = text_config.get("output_format", "")
            self.check_valid_value(text_output_format, "Text output format abnormal.", self.allowed_output_format["text"])

        audio_config = self.setups.get("audio", "")
        if audio_config:
            self.check_empty(audio_config.get("llm_id"), "VLM")
            audio_language = audio_config.get("lang", "")
            self.check_empty(audio_language, "Language")

    def get_input_form(self) -> dict[str, dict]:
        return {}


class Parser(ProcessBase):
    component_name = "Parser"

    def _pdf(self, from_upstream: ParserFromUpstream):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on a PDF.")

        blob = from_upstream.blob
        conf = self._param.setups["pdf"]
        self.set_output("output_format", conf["output_format"])

        if conf.get("parse_method") == "deepdoc":
            bboxes = RAGFlowPdfParser().parse_into_bboxes(blob, callback=self.callback)
        elif conf.get("parse_method") == "plain_text":
            lines, _ = PlainParser()(blob)
            bboxes = [{"text": t} for t, _ in lines]
        else:
            assert conf.get("llm_id")
            vision_model = LLMBundle(self._canvas._tenant_id, LLMType.IMAGE2TEXT, llm_name=conf.get("llm_id"), lang=self._param.setups["pdf"].get("lang"))
            lines, _ = VisionParser(vision_model=vision_model)(blob, callback=self.callback)
            bboxes = []
            for t, poss in lines:
                pn, x0, x1, top, bott = poss.split(" ")
                bboxes.append({"page_number": int(pn), "x0": float(x0), "x1": float(x1), "top": float(top), "bottom": float(bott), "text": t})

        if conf.get("output_format") == "json":
            self.set_output("json", bboxes)
        if conf.get("output_format") == "markdown":
            mkdn = ""
            for b in bboxes:
                if b.get("layout_type", "") == "title":
                    mkdn += "\n## "
                if b.get("layout_type", "") == "figure":
                    mkdn += "\n![Image]({})".format(VLM.image2base64(b["image"]))
                    continue
                mkdn += b.get("text", "") + "\n"
            self.set_output("markdown", mkdn)

    def _spreadsheet(self, from_upstream: ParserFromUpstream):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on a Spreadsheet.")

        blob = from_upstream.blob
        conf = self._param.setups["spreadsheet"]
        self.set_output("output_format", conf["output_format"])

        print("spreadsheet {conf=}", flush=True)
        spreadsheet_parser = ExcelParser()
        if conf.get("output_format") == "html":
            html = spreadsheet_parser.html(blob, 1000000000)
            self.set_output("html", html)
        elif conf.get("output_format") == "json":
            self.set_output("json", [{"text": txt} for txt in spreadsheet_parser(blob) if txt])
        elif conf.get("output_format") == "markdown":
            self.set_output("markdown", spreadsheet_parser.markdown(blob))

    def _word(self, from_upstream: ParserFromUpstream):
        from tika import parser as  word_parser

        self.callback(random.randint(1, 5) / 100.0, "Start to work on a Word Processor Document")

        blob = from_upstream.blob
        name = from_upstream.name
        conf = self._param.setups["word"]
        self.set_output("output_format", conf["output_format"])

        print("word {conf=}", flush=True)
        doc_parsed = word_parser.from_buffer(blob)

        sections = []
        if doc_parsed.get("content"):
            sections = doc_parsed["content"].split("\n")
            sections = [{"text": section} for section in sections if section]
        else:
            logging.warning(f"tika.parser got empty content from {name}.")

        # json
        assert conf.get("output_format") == "json", "have to be json for doc"
        if conf.get("output_format") == "json":
            self.set_output("json", sections)

    def _markdown(self, from_upstream: ParserFromUpstream):
        from functools import reduce

        from rag.app.naive import Markdown as naive_markdown_parser
        from rag.nlp import concat_img

        self.callback(random.randint(1, 5) / 100.0, "Start to work on a markdown.")

        blob = from_upstream.blob
        name = from_upstream.name
        conf = self._param.setups["markdown"]
        self.set_output("output_format", conf["output_format"])

        markdown_parser = naive_markdown_parser()
        sections, tables = markdown_parser(name, blob, separate_tables=False)

        # json
        assert conf.get("output_format") == "json", "have to be json for doc"
        if conf.get("output_format") == "json":
            json_results = []

            for section_text, _ in sections:
                json_result = {
                    "text": section_text,
                }

                images = markdown_parser.get_pictures(section_text) if section_text else None
                if images:
                    # If multiple images found, combine them using concat_img
                    combined_image = reduce(concat_img, images) if len(images) > 1 else images[0]
                    json_result["image"] = combined_image

                json_results.append(json_result)

            self.set_output("json", json_results)

    def _text(self, from_upstream: ParserFromUpstream):
        from deepdoc.parser.utils import get_text

        self.callback(random.randint(1, 5) / 100.0, "Start to work on a text.")

        blob = from_upstream.blob
        name = from_upstream.name
        conf = self._param.setups["text"]
        self.set_output("output_format", conf["output_format"])

        # parse binary to text
        text_content = get_text(name, binary=blob)

        if conf.get("output_format") == "json":
            result = [{"text": text_content}]
            self.set_output("json", result)
        else:
            result = text_content
            self.set_output("text", result)

    def _image(self, from_upstream: ParserFromUpstream):
        from deepdoc.vision import OCR

        self.callback(random.randint(1, 5) / 100.0, "Start to work on an image.")

        blob = from_upstream.blob
        conf = self._param.setups["image"]
        self.set_output("output_format", conf["output_format"])

        img = Image.open(io.BytesIO(blob)).convert("RGB")
        lang = conf["lang"]

        if conf["parse_method"] == "ocr":
            # use ocr, recognize chars only
            ocr = OCR()
            bxs = ocr(np.array(img))  # return boxes and recognize result
            txt = "\n".join([t[0] for _, t in bxs if t[0]])

        else:
            # use VLM to describe the picture
            cv_model = LLMBundle(self._canvas.get_tenant_id(), LLMType.IMAGE2TEXT, llm_name=conf["llm_id"],lang=lang)
            img_binary = io.BytesIO()
            img.save(img_binary, format="JPEG")
            img_binary.seek(0)
            txt = cv_model.describe(img_binary.read())

        self.set_output("text", txt)

    def _audio(self, from_upstream: ParserFromUpstream):
        import os
        import tempfile

        self.callback(random.randint(1, 5) / 100.0, "Start to work on an audio.")

        blob = from_upstream.blob
        name = from_upstream.name
        conf = self._param.setups["audio"]
        self.set_output("output_format", conf["output_format"])

        lang = conf["lang"]
        _, ext = os.path.splitext(name)
        tmp_path = ""
        with tempfile.NamedTemporaryFile(suffix=ext) as tmpf:
            tmpf.write(blob)
            tmpf.flush()
            tmp_path = os.path.abspath(tmpf.name)

            seq2txt_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.SPEECH2TEXT, lang=lang)
            txt = seq2txt_mdl.transcription(tmp_path)

            self.set_output("text", txt)

    async def _invoke(self, **kwargs):
        function_map = {
            "pdf": self._pdf,
            "markdown": self._markdown,
            "spreadsheet": self._spreadsheet,
            "word": self._word,
            "text": self._text,
            "image": self._image,
            "audio": self._audio,
        }
        try:
            from_upstream = ParserFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        for p_type, conf in self._param.setups.items():
            if from_upstream.name.split(".")[-1].lower() not in conf.get("suffix", []):
                continue
            await trio.to_thread.run_sync(function_map[p_type], from_upstream)
            break
