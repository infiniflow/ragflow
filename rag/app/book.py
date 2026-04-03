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
import re
from io import BytesIO

from deepdoc.parser.utils import get_text
from rag.app import naive
from rag.app.naive import by_plaintext, PARSERS
from common.parser_config_utils import normalize_layout_recognizer
from rag.nlp import bullets_category, is_english, remove_contents_table, hierarchical_merge, make_colon_as_title, naive_merge, random_choices, tokenize_table, tokenize_chunks, attach_media_context
from rag.nlp import rag_tokenizer
from deepdoc.parser import PdfParser, HtmlParser
from deepdoc.parser.figure_parser import vision_figure_parser_docx_wrapper
from PIL import Image


class Pdf(PdfParser):
    def __call__(self, filename, binary=None, from_page=0, to_page=100000, zoomin=3, callback=None):
        from timeit import default_timer as timer

        start = timer()
        callback(msg="OCR started")
        self.__images__(filename if not binary else binary, zoomin, from_page, to_page, callback)
        callback(msg="OCR finished ({:.2f}s)".format(timer() - start))

        start = timer()
        self._layouts_rec(zoomin)
        callback(0.67, "Layout analysis ({:.2f}s)".format(timer() - start))
        logging.debug("layouts: {}".format(timer() - start))

        start = timer()
        self._table_transformer_job(zoomin)
        callback(0.68, "Table analysis ({:.2f}s)".format(timer() - start))

        start = timer()
        self._text_merge()
        tbls = self._extract_table_figure(True, zoomin, True, True)
        self._naive_vertical_merge()
        self._filter_forpages()
        self._merge_with_same_bullet()
        callback(0.8, "Text extraction ({:.2f}s)".format(timer() - start))

        return [(b["text"] + self._line_tag(b, zoomin), b.get("layoutno", "")) for b in self.boxes], tbls


def chunk(filename, binary=None, from_page=0, to_page=100000, lang="Chinese", callback=None, **kwargs):
    """
    Supported file formats are docx, pdf, txt.
    Since a book is long and not all the parts are useful, if it's a PDF,
    please set up the page ranges for every book in order eliminate negative effects and save elapsed computing time.
    """
    parser_config = kwargs.get("parser_config", {"chunk_token_num": 512, "delimiter": "\n!?。；！？", "layout_recognize": "DeepDOC"})
    doc = {"docnm_kwd": filename, "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename))}
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])
    pdf_parser = None
    sections, tbls = [], []
    if re.search(r"\.docx$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        doc_parser = naive.Docx()
        # TODO: table of contents need to be removed
        main_sections = doc_parser(filename, binary=binary, from_page=from_page, to_page=to_page)

        sections = []
        tbls = []
        for text, image, html in main_sections:
            sections.append((text, image))
            tbls.append(((None, html), ""))

        remove_contents_table(sections, eng=is_english(random_choices([t for t, _ in sections], k=200)))

        tbls = vision_figure_parser_docx_wrapper(sections=sections, tbls=tbls, callback=callback, **kwargs)
        # tbls = [((None, lns), None) for lns in tbls]
        sections = [(item[0], item[1] if item[1] is not None else "") for item in sections if not isinstance(item[1], Image.Image)]
        callback(0.8, "Finish parsing.")

    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        layout_recognizer, parser_model_name = normalize_layout_recognizer(parser_config.get("layout_recognize", "DeepDOC"))

        if isinstance(layout_recognizer, bool):
            layout_recognizer = "DeepDOC" if layout_recognizer else "Plain Text"

        name = layout_recognizer.strip().lower()
        parser = PARSERS.get(name, by_plaintext)
        callback(0.1, "Start to parse.")

        sections, tables, pdf_parser = parser(
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

        if not sections and not tables:
            return []

        if name in ["tcadp", "docling", "mineru", "paddleocr"]:
            parser_config["chunk_token_num"] = 0

        callback(0.8, "Finish parsing.")
    elif re.search(r"\.txt$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        txt = get_text(filename, binary)
        sections = txt.split("\n")
        sections = [(line, "") for line in sections if line]
        remove_contents_table(sections, eng=is_english(random_choices([t for t, _ in sections], k=200)))
        callback(0.8, "Finish parsing.")

    elif re.search(r"\.(htm|html)$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        sections = HtmlParser()(filename, binary)
        sections = [(line, "") for line in sections if line]
        remove_contents_table(sections, eng=is_english(random_choices([t for t, _ in sections], k=200)))
        callback(0.8, "Finish parsing.")

    elif re.search(r"\.doc$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        try:
            from tika import parser as tika_parser
        except Exception as e:
            callback(0.8, f"tika not available: {e}. Unsupported .doc parsing.")
            logging.warning(f"tika not available: {e}. Unsupported .doc parsing for {filename}.")
            return []

        binary = BytesIO(binary)
        doc_parsed = tika_parser.from_buffer(binary)
        if doc_parsed.get("content", None) is not None:
            sections = doc_parsed["content"].split("\n")
            sections = [(line, "") for line in sections if line]
            remove_contents_table(sections, eng=is_english(random_choices([t for t, _ in sections], k=200)))
            callback(0.8, "Finish parsing.")

    else:
        raise NotImplementedError("file type not supported yet(doc, docx, pdf, txt supported)")

    make_colon_as_title(sections)
    bull = bullets_category([t for t in random_choices([t for t, _ in sections], k=100)])
    if bull >= 0:
        chunks = ["\n".join(ck) for ck in hierarchical_merge(bull, sections, 5)]
    else:
        sections = [s.split("@") for s, _ in sections]
        sections = [(pr[0], "@" + pr[1]) if len(pr) == 2 else (pr[0], "") for pr in sections]
        chunks = naive_merge(sections, parser_config.get("chunk_token_num", 256), parser_config.get("delimiter", "\n。；！？"))

    # is it English
    # is_english(random_choices([t for t, _ in sections], k=218))
    eng = lang.lower() == "english"

    res = tokenize_table(tbls, doc, eng)
    res.extend(tokenize_chunks(chunks, doc, eng, pdf_parser))
    table_ctx = max(0, int(parser_config.get("table_context_size", 0) or 0))
    image_ctx = max(0, int(parser_config.get("image_context_size", 0) or 0))
    if table_ctx or image_ctx:
        attach_media_context(res, table_ctx, image_ctx)

    return res


if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass

    chunk(sys.argv[1], from_page=1, to_page=10, callback=dummy)
