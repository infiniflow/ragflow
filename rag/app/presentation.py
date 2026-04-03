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
import re
from collections import defaultdict
from io import BytesIO

from PIL import Image
from PyPDF2 import PdfReader as pdf2_read

from deepdoc.parser import PdfParser, PlainParser
from deepdoc.parser.ppt_parser import RAGFlowPptParser
from rag.app.naive import by_plaintext, PARSERS
from common.parser_config_utils import normalize_layout_recognizer
from rag.nlp import rag_tokenizer
from rag.nlp import tokenize


class Pdf(PdfParser):
    def __init__(self):
        super().__init__()

    def __call__(self, filename, binary=None, from_page=0, to_page=100000, zoomin=3, callback=None, **kwargs):
        # 1. OCR
        callback(msg="OCR started")
        self.__images__(filename if not binary else binary, zoomin, from_page, to_page, callback)

        # 2. Layout Analysis
        callback(msg="Layout Analysis")
        self._layouts_rec(zoomin)

        # 3. Table Analysis
        callback(msg="Table Analysis")
        self._table_transformer_job(zoomin)

        # 4. Text Merge
        self._text_merge()

        # 5. Extract Tables (Force HTML)
        tbls = self._extract_table_figure(True, zoomin, True, True)

        # 6. Re-assemble Page Content
        page_items = defaultdict(list)

        # (A) Add text
        for b in self.boxes:
            # b["page_number"] is relative page number，must + from_page
            global_page_num = b["page_number"] + from_page
            if not (from_page < global_page_num <= to_page + from_page):
                continue
            page_items[global_page_num].append({"top": b["top"], "x0": b["x0"], "text": b["text"], "type": "text"})

        # (B) Add table and figure
        for (img, content), positions in tbls:
            if not positions:
                continue

            if isinstance(content, list):
                final_text = "\n".join(content)
            elif isinstance(content, str):
                final_text = content
            else:
                final_text = str(content)

            try:
                pn_index = positions[0][0]
                if isinstance(pn_index, list):
                    pn_index = pn_index[0]

                # pn_index in tbls is absolute page number
                current_page_num = int(pn_index) + 1
            except Exception as e:
                print(f"Error parsing position: {e}")
                continue

            if not (from_page < current_page_num <= to_page + from_page):
                continue

            top = positions[0][3]
            left = positions[0][1]

            page_items[current_page_num].append({"top": top, "x0": left, "text": final_text, "type": "table_or_figure"})

        # 7. Generate result
        res = []
        for i in range(len(self.page_images)):
            current_pn = from_page + i + 1
            items = page_items.get(current_pn, [])
            # Sort by vertical position
            items.sort(key=lambda x: (x["top"], x["x0"]))
            full_page_text = "\n\n".join([item["text"] for item in items])
            if not full_page_text.strip():
                full_page_text = f"[No text or data found in Page {current_pn}]"
            page_img = self.page_images[i]
            res.append((full_page_text, page_img))

        callback(0.9, "Parsing finished")

        return res, []


class PlainPdf(PlainParser):
    def __call__(self, filename, binary=None, from_page=0, to_page=100000, callback=None, **kwargs):
        self.pdf = pdf2_read(filename if not binary else BytesIO(binary))
        page_txt = []
        for page in self.pdf.pages[from_page:to_page]:
            page_txt.append(page.extract_text())
        callback(0.9, "Parsing finished")
        return [(txt, None) for txt in page_txt], []


def chunk(filename, binary=None, from_page=0, to_page=100000, lang="Chinese", callback=None, parser_config=None, **kwargs):
    """
    The supported file formats are pdf, ppt, pptx.
    Every page will be treated as a chunk. And the thumbnail of every page will be stored.
    PPT file will be parsed by using this method automatically, setting-up for every PPT file is not necessary.
    """
    if parser_config is None:
        parser_config = {}
    eng = lang.lower() == "english"
    doc = {"docnm_kwd": filename, "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename))}
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])
    res = []
    if re.search(r"\.pptx?$", filename, re.IGNORECASE):
        try:
            ppt_parser = RAGFlowPptParser()
            for pn, txt in enumerate(ppt_parser(filename if not binary else binary, from_page, 1000000, callback)):
                d = copy.deepcopy(doc)
                pn += from_page
                d["doc_type_kwd"] = "image"
                d["page_num_int"] = [pn + 1]
                d["top_int"] = [0]
                d["position_int"] = [(pn + 1, 0, 0, 0, 0)]
                tokenize(d, txt, eng)
                res.append(d)
            return res
        except Exception as e:
            logging.warning(f"python-pptx parsing failed for {filename}: {e}, trying tika as fallback")
            if callback:
                callback(0.1, "python-pptx failed, trying tika as fallback")
            
            try:
                from tika import parser as tika_parser
            except Exception as tika_error:
                error_msg = f"tika not available: {tika_error}. Unsupported .ppt/.pptx parsing."
                if callback:
                    callback(0.8, error_msg)
                logging.warning(f"{error_msg} for {filename}.")
                raise NotImplementedError(error_msg)
            
            if binary:
                binary_data = binary
            else:
                with open(filename, 'rb') as f:
                    binary_data = f.read()
            doc_parsed = tika_parser.from_buffer(BytesIO(binary_data))
            
            if doc_parsed.get("content", None) is not None:
                sections = doc_parsed["content"].split("\n")
                sections = [s for s in sections if s.strip()]
                
                for pn, txt in enumerate(sections):
                    d = copy.deepcopy(doc)
                    pn += from_page
                    d["doc_type_kwd"] = "text"
                    d["page_num_int"] = [pn + 1]
                    d["top_int"] = [0]
                    d["position_int"] = [(pn + 1, 0, 0, 0, 0)]
                    tokenize(d, txt, eng)
                    res.append(d)
                
                if callback:
                    callback(0.8, "Finish parsing with tika.")
                return res
            else:
                error_msg = f"tika.parser got empty content from {filename}."
                if callback:
                    callback(0.8, error_msg)
                logging.warning(error_msg)
                raise NotImplementedError(error_msg)
    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        layout_recognizer, parser_model_name = normalize_layout_recognizer(parser_config.get("layout_recognize", "DeepDOC"))

        if isinstance(layout_recognizer, bool):
            layout_recognizer = "DeepDOC" if layout_recognizer else "Plain Text"

        name = layout_recognizer.strip().lower()
        parser = PARSERS.get(name, by_plaintext)
        callback(0.1, "Start to parse.")

        sections, _, _ = parser(
            filename=filename,
            binary=binary,
            from_page=from_page,
            to_page=to_page,
            lang=lang,
            callback=callback,
            pdf_cls=Pdf,
            layout_recognizer=layout_recognizer,
            mineru_llm_name=parser_model_name,
            paddleocr_llm_name=parser_model_name,
            **kwargs,
        )

        if not sections:
            return []

        if name in ["tcadp", "docling", "mineru", "paddleocr"]:
            parser_config["chunk_token_num"] = 0

        callback(0.8, "Finish parsing.")

        for pn, (txt, img) in enumerate(sections):
            d = copy.deepcopy(doc)
            pn += from_page
            if not isinstance(img, Image.Image):
                img = None
            d["image"] = img
            d["page_num_int"] = [pn + 1]
            d["top_int"] = [0]
            d["position_int"] = [(pn + 1, 0, img.size[0] if img else 0, 0, img.size[1] if img else 0)]
            tokenize(d, txt, eng)
            res.append(d)
        return res

    raise NotImplementedError("file type not supported yet(ppt, pptx, pdf supported)")


if __name__ == "__main__":
    import sys

    def dummy(a, b):
        pass

    chunk(sys.argv[1], callback=dummy)
