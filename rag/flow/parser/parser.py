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
import json
import os
import random
from functools import partial

import trio
import numpy as np
from PIL import Image

from common.constants import LLMType
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.llm_service import LLMBundle
from common.misc_utils import get_uuid
from rag.utils.base64_image import image2id
from deepdoc.parser import ExcelParser
from deepdoc.parser.mineru_parser import MinerUParser
from deepdoc.parser.pdf_parser import PlainParser, RAGFlowPdfParser, VisionParser
from deepdoc.parser.tcadp_parser import TCADPParser
from rag.app.naive import Docx
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.parser.schema import ParserFromUpstream
from rag.llm.cv_model import Base as VLM
from common import settings


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
                "markdown",
            ],
            "slides": [
                "json",
            ],
            "image": [
                "text"
            ],
            "email": ["text", "json"],
            "text&markdown": [
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
                "parse_method": "deepdoc",  # deepdoc/plain_text/tcadp_parser/vlm
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
            "text&markdown": {
                "suffix": ["md", "markdown", "mdx", "txt"],
                "output_format": "json",
            },
            "slides": {
                "suffix": [
                    "pptx",
                ],
                "output_format": "json",
            },
            "image": {
                "parse_method": "ocr",
                "llm_id": "",
                "lang": "Chinese",
                "system_prompt": "",
                "suffix": ["jpg", "jpeg", "png", "gif"],
                "output_format": "text",
            },
            "email": {
                "suffix": [
                  "eml", "msg"
                ],
                "fields": ["from", "to", "cc", "bcc", "date", "subject", "body", "attachments", "metadata"],
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
                "output_format": "text",
            },
            "video": {
                "suffix":[
                    "mp4",
                    "avi",
                    "mkv"
                ],
                "output_format": "text",
            },
        }

    def check(self):
        pdf_config = self.setups.get("pdf", {})
        if pdf_config:
            pdf_parse_method = pdf_config.get("parse_method", "")
            self.check_empty(pdf_parse_method, "Parse method abnormal.")

            if pdf_parse_method.lower() not in ["deepdoc", "plain_text", "mineru", "tcadp parser"]:
                self.check_empty(pdf_config.get("lang", ""), "PDF VLM language")

            pdf_output_format = pdf_config.get("output_format", "")
            self.check_valid_value(pdf_output_format, "PDF output format abnormal.", self.allowed_output_format["pdf"])

        spreadsheet_config = self.setups.get("spreadsheet", "")
        if spreadsheet_config:
            spreadsheet_output_format = spreadsheet_config.get("output_format", "")
            self.check_valid_value(spreadsheet_output_format, "Spreadsheet output format abnormal.", self.allowed_output_format["spreadsheet"])

        doc_config = self.setups.get("word", "")
        if doc_config:
            doc_output_format = doc_config.get("output_format", "")
            self.check_valid_value(doc_output_format, "Word processer document output format abnormal.", self.allowed_output_format["word"])

        slides_config = self.setups.get("slides", "")
        if slides_config:
            slides_output_format = slides_config.get("output_format", "")
            self.check_valid_value(slides_output_format, "Slides output format abnormal.", self.allowed_output_format["slides"])

        image_config = self.setups.get("image", "")
        if image_config:
            image_parse_method = image_config.get("parse_method", "")
            if image_parse_method not in ["ocr"]:
                self.check_empty(image_config.get("lang", ""), "Image VLM language")

        text_config = self.setups.get("text&markdown", "")
        if text_config:
            text_output_format = text_config.get("output_format", "")
            self.check_valid_value(text_output_format, "Text output format abnormal.", self.allowed_output_format["text&markdown"])

        audio_config = self.setups.get("audio", "")
        if audio_config:
            self.check_empty(audio_config.get("llm_id"), "Audio VLM")

        video_config = self.setups.get("video", "")
        if video_config:
            self.check_empty(video_config.get("llm_id"), "Video VLM")

        email_config = self.setups.get("email", "")
        if email_config:
            email_output_format = email_config.get("output_format", "")
            self.check_valid_value(email_output_format, "Email output format abnormal.", self.allowed_output_format["email"])

    def get_input_form(self) -> dict[str, dict]:
        return {}


class Parser(ProcessBase):
    component_name = "Parser"

    def _pdf(self, name, blob):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on a PDF.")
        conf = self._param.setups["pdf"]
        self.set_output("output_format", conf["output_format"])

        if conf.get("parse_method").lower() == "deepdoc":
            bboxes = RAGFlowPdfParser().parse_into_bboxes(blob, callback=self.callback)
        elif conf.get("parse_method").lower() == "plain_text":
            lines, _ = PlainParser()(blob)
            bboxes = [{"text": t} for t, _ in lines]
        elif conf.get("parse_method").lower() == "mineru":
            mineru_executable = os.environ.get("MINERU_EXECUTABLE", "mineru")
            mineru_api = os.environ.get("MINERU_APISERVER", "http://host.docker.internal:9987")
            pdf_parser = MinerUParser(mineru_path=mineru_executable, mineru_api=mineru_api)
            ok, reason = pdf_parser.check_installation()
            if not ok:
                raise RuntimeError(f"MinerU not found or server not accessible: {reason}. Please install it via: pip install -U 'mineru[core]'.")

            lines, _ = pdf_parser.parse_pdf(
                filepath=name,
                binary=blob,
                callback=self.callback,
                output_dir=os.environ.get("MINERU_OUTPUT_DIR", ""),
                delete_output=bool(int(os.environ.get("MINERU_DELETE_OUTPUT", 1))),
            )
            bboxes = []
            for t, poss in lines:
                box = {
                    "image": pdf_parser.crop(poss, 1),
                    "positions": [[pos[0][-1], *pos[1:]] for pos in pdf_parser.extract_positions(poss)],
                    "text": t,
                }
                bboxes.append(box)
        elif conf.get("parse_method").lower() == "tcadp parser":
            # ADP is a document parsing tool using Tencent Cloud API
            tcadp_parser = TCADPParser()
            sections, _ = tcadp_parser.parse_pdf(
                filepath=name,
                binary=blob,
                callback=self.callback,
                file_type="PDF",
                file_start_page=1,
                file_end_page=1000
            )
            bboxes = []
            for section, position_tag in sections:
                if position_tag:
                    # Extract position information from TCADP's position tag
                    # Format: @@{page_number}\t{x0}\t{x1}\t{top}\t{bottom}##
                    import re
                    match = re.match(r"@@([0-9-]+)\t([0-9.]+)\t([0-9.]+)\t([0-9.]+)\t([0-9.]+)##", position_tag)
                    if match:
                        pn, x0, x1, top, bott = match.groups()
                        bboxes.append({
                            "page_number": int(pn.split('-')[0]),  # Take the first page number
                            "x0": float(x0),
                            "x1": float(x1),
                            "top": float(top),
                            "bottom": float(bott),
                            "text": section
                        })
                    else:
                        # If no position info, add as text without position
                        bboxes.append({"text": section})
                else:
                    bboxes.append({"text": section})
        else:
            vision_model = LLMBundle(self._canvas._tenant_id, LLMType.IMAGE2TEXT, llm_name=conf.get("parse_method"), lang=self._param.setups["pdf"].get("lang"))
            lines, _ = VisionParser(vision_model=vision_model)(blob, callback=self.callback)
            bboxes = []
            for t, poss in lines:
                for pn, x0, x1, top, bott in RAGFlowPdfParser.extract_positions(poss):
                    bboxes.append({"page_number": int(pn[0]), "x0": float(x0), "x1": float(x1), "top": float(top), "bottom": float(bott), "text": t})

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

    def _spreadsheet(self, name, blob):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on a Spreadsheet.")
        conf = self._param.setups["spreadsheet"]
        self.set_output("output_format", conf["output_format"])
        spreadsheet_parser = ExcelParser()
        if conf.get("output_format") == "html":
            htmls = spreadsheet_parser.html(blob, 1000000000)
            self.set_output("html", htmls[0])
        elif conf.get("output_format") == "json":
            self.set_output("json", [{"text": txt} for txt in spreadsheet_parser(blob) if txt])
        elif conf.get("output_format") == "markdown":
            self.set_output("markdown", spreadsheet_parser.markdown(blob))

    def _word(self, name, blob):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on a Word Processor Document")
        conf = self._param.setups["word"]
        self.set_output("output_format", conf["output_format"])
        docx_parser = Docx()

        if conf.get("output_format") == "json":
            sections, tbls = docx_parser(name, binary=blob)
            sections = [{"text": section[0], "image": section[1]} for section in sections if section]
            sections.extend([{"text": tb, "image": None} for ((_,tb), _) in tbls])
            self.set_output("json", sections)
        elif conf.get("output_format") == "markdown":
            markdown_text = docx_parser.to_markdown(name, binary=blob)
            self.set_output("markdown", markdown_text)

    def _slides(self, name, blob):
        from deepdoc.parser.ppt_parser import RAGFlowPptParser as ppt_parser

        self.callback(random.randint(1, 5) / 100.0, "Start to work on a PowerPoint Document")

        conf = self._param.setups["slides"]
        self.set_output("output_format", conf["output_format"])

        ppt_parser = ppt_parser()
        txts = ppt_parser(blob, 0, 100000, None)

        sections = [{"text": section} for section in txts if section.strip()]

        # json
        assert conf.get("output_format") == "json", "have to be json for ppt"
        if conf.get("output_format") == "json":
            self.set_output("json", sections)

    def _markdown(self, name, blob):
        from functools import reduce

        from rag.app.naive import Markdown as naive_markdown_parser
        from rag.nlp import concat_img

        self.callback(random.randint(1, 5) / 100.0, "Start to work on a markdown.")
        conf = self._param.setups["text&markdown"]
        self.set_output("output_format", conf["output_format"])

        markdown_parser = naive_markdown_parser()
        sections, tables = markdown_parser(name, blob, separate_tables=False)

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
        else:
            self.set_output("text", "\n".join([section_text for section_text, _ in sections]))


    def _image(self, name, blob):
        from deepdoc.vision import OCR

        self.callback(random.randint(1, 5) / 100.0, "Start to work on an image.")
        conf = self._param.setups["image"]
        self.set_output("output_format", conf["output_format"])

        img = Image.open(io.BytesIO(blob)).convert("RGB")

        if conf["parse_method"] == "ocr":
            # use ocr, recognize chars only
            ocr = OCR()
            bxs = ocr(np.array(img))  # return boxes and recognize result
            txt = "\n".join([t[0] for _, t in bxs if t[0]])
        else:
            lang = conf["lang"]
            # use VLM to describe the picture
            cv_model = LLMBundle(self._canvas.get_tenant_id(), LLMType.IMAGE2TEXT, llm_name=conf["parse_method"], lang=lang)
            img_binary = io.BytesIO()
            img.save(img_binary, format="JPEG")
            img_binary.seek(0)

            system_prompt = conf.get("system_prompt")
            if system_prompt:
                txt = cv_model.describe_with_prompt(img_binary.read(), system_prompt)
            else:
                txt = cv_model.describe(img_binary.read())

        self.set_output("text", txt)

    def _audio(self, name, blob):
        import os
        import tempfile

        self.callback(random.randint(1, 5) / 100.0, "Start to work on an audio.")

        conf = self._param.setups["audio"]
        self.set_output("output_format", conf["output_format"])
        _, ext = os.path.splitext(name)
        with tempfile.NamedTemporaryFile(suffix=ext) as tmpf:
            tmpf.write(blob)
            tmpf.flush()
            tmp_path = os.path.abspath(tmpf.name)

            seq2txt_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.SPEECH2TEXT)
            txt = seq2txt_mdl.transcription(tmp_path)

            self.set_output("text", txt)

    def _video(self, name, blob):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on an video.")

        conf = self._param.setups["video"]
        self.set_output("output_format", conf["output_format"])

        cv_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.IMAGE2TEXT, llm_name=conf["llm_id"])
        txt = cv_mdl.chat(system="", history=[], gen_conf={}, video_bytes=blob, filename=name)

        self.set_output("text", txt)

    def _email(self, name, blob):
        self.callback(random.randint(1, 5) / 100.0, "Start to work on an email.")

        email_content = {}
        conf = self._param.setups["email"]
        self.set_output("output_format", conf["output_format"])
        target_fields = conf["fields"]

        _, ext = os.path.splitext(name)
        if ext == ".eml":
            # handle eml file
            from email import policy
            from email.parser import BytesParser

            msg = BytesParser(policy=policy.default).parse(io.BytesIO(blob))
            email_content['metadata'] = {}
            # handle header info
            for header, value in msg.items():
                # get fields like from, to, cc, bcc, date, subject
                if header.lower() in target_fields:
                    email_content[header.lower()] = value
                # get metadata
                elif header.lower() not in ["from", "to", "cc", "bcc", "date", "subject"]:
                    email_content["metadata"][header.lower()] = value
            # get body
            if "body" in target_fields:
                body_text, body_html = [], []
                def _add_content(m, content_type):
                    def _decode_payload(payload, charset, target_list):
                        try:
                            target_list.append(payload.decode(charset))
                        except (UnicodeDecodeError, LookupError):
                            for enc in ["utf-8", "gb2312", "gbk", "gb18030", "latin1"]:
                                try:
                                    target_list.append(payload.decode(enc))
                                    break
                                except UnicodeDecodeError:
                                    continue
                            else:
                                target_list.append(payload.decode("utf-8", errors="ignore"))

                    if content_type == "text/plain":
                        payload = msg.get_payload(decode=True)
                        charset = msg.get_content_charset() or "utf-8"
                        _decode_payload(payload, charset, body_text)
                    elif content_type == "text/html":
                        payload = msg.get_payload(decode=True)
                        charset = msg.get_content_charset() or "utf-8"
                        _decode_payload(payload, charset, body_html)
                    elif "multipart" in content_type:
                        if m.is_multipart():
                            for part in m.iter_parts():
                                _add_content(part, part.get_content_type())

                _add_content(msg, msg.get_content_type())

                email_content["text"] = "\n".join(body_text)
                email_content["text_html"] = "\n".join(body_html)
            # get attachment
            if "attachments" in target_fields:
                attachments = []
                for part in msg.iter_attachments():
                    content_disposition = part.get("Content-Disposition")
                    if content_disposition:
                        dispositions = content_disposition.strip().split(";")
                        if dispositions[0].lower() == "attachment":
                            filename = part.get_filename()
                            payload = part.get_payload(decode=True).decode(part.get_content_charset())
                            attachments.append({
                                "filename": filename,
                                "payload": payload,
                            })
                email_content["attachments"] = attachments
        else:
            # handle msg file
            import extract_msg
            print("handle a msg file.")
            msg = extract_msg.Message(blob)
            # handle header info
            basic_content = {
                "from": msg.sender,
                "to": msg.to,
                "cc": msg.cc,
                "bcc": msg.bcc,
                "date": msg.date,
                "subject": msg.subject,
            }
            email_content.update({k: v for k, v in basic_content.items() if k in target_fields})
            # get metadata
            email_content['metadata'] = {
                'message_id': msg.messageId,
                'in_reply_to': msg.inReplyTo,
            }
            # get body
            if "body" in target_fields:
                email_content["text"] = msg.body[0] if isinstance(msg.body, list) and msg.body else msg.body
                if not email_content["text"] and msg.htmlBody:
                    email_content["text"] = msg.htmlBody[0] if isinstance(msg.htmlBody, list) and msg.htmlBody else msg.htmlBody
            # get attachments
            if "attachments" in target_fields:
                attachments = []
                for t in msg.attachments:
                    attachments.append({
                        "filename": t.name,
                        "payload": t.data.decode("utf-8")
                    })
                email_content["attachments"] = attachments

        if conf["output_format"] == "json":
            self.set_output("json", [email_content])
        else:
            content_txt = ''
            for k, v in email_content.items():
                if isinstance(v, str):
                    # basic info
                    content_txt += f'{k}:{v}' + "\n"
                elif isinstance(v, dict):
                    # metadata
                    content_txt += f'{k}:{json.dumps(v)}' + "\n"
                elif isinstance(v, list):
                    # attachments or others
                    for fb in v:
                        if isinstance(fb, dict):
                            # attachments
                            content_txt += f'{fb["filename"]}:{fb["payload"]}' + "\n"
                        else:
                            # str, usually plain text
                            content_txt += fb
            self.set_output("text", content_txt)

    async def _invoke(self, **kwargs):
        function_map = {
            "pdf": self._pdf,
            "text&markdown": self._markdown,
            "spreadsheet": self._spreadsheet,
            "slides": self._slides,
            "word": self._word,
            "image": self._image,
            "audio": self._audio,
            "video": self._video,
            "email": self._email,
        }
        try:
            from_upstream = ParserFromUpstream.model_validate(kwargs)
        except Exception as e:
            self.set_output("_ERROR", f"Input error: {str(e)}")
            return

        name = from_upstream.name
        if self._canvas._doc_id:
            b, n = File2DocumentService.get_storage_address(doc_id=self._canvas._doc_id)
            blob = settings.STORAGE_IMPL.get(b, n)
        else:
            blob = FileService.get_blob(from_upstream.file["created_by"], from_upstream.file["id"])

        done = False
        for p_type, conf in self._param.setups.items():
            if from_upstream.name.split(".")[-1].lower() not in conf.get("suffix", []):
                continue
            await trio.to_thread.run_sync(function_map[p_type], name, blob)
            done = True
            break

        if not done:
            raise Exception("No suitable for file extension: `.%s`" % from_upstream.name.split(".")[-1].lower())

        outs = self.output()
        async with trio.open_nursery() as nursery:
            for d in outs.get("json", []):
                nursery.start_soon(image2id, d, partial(settings.STORAGE_IMPL.put, tenant_id=self._canvas._tenant_id), get_uuid())
