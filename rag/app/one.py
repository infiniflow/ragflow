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
from tika import parser
from io import BytesIO
import re

from deepdoc.parser.utils import get_text
from rag.app import naive
from rag.nlp import rag_tokenizer, tokenize
from deepdoc.parser import PdfParser, ExcelParser, PlainParser, HtmlParser


class Pdf(PdfParser):
    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        callback(msg="OCR is running...")
        self.__images__(
            filename if not binary else binary,
            zoomin,
            from_page,
            to_page,
            callback
        )
        callback(msg="OCR finished")

        from timeit import default_timer as timer
        start = timer()
        self._layouts_rec(zoomin, drop=False)
        callback(0.63, "Layout analysis finished.")
        logging.debug("layouts cost: {}s".format(timer() - start))
        self._table_transformer_job(zoomin)
        callback(0.65, "Table analysis finished.")
        self._text_merge()
        callback(0.67, "Text merging finished")
        tbls = self._extract_table_figure(True, zoomin, True, True)
        self._concat_downward()

        sections = [(b["text"], self.get_position(b, zoomin))
                    for i, b in enumerate(self.boxes)]
        for (img, rows), poss in tbls:
            if not rows:continue
            sections.append((rows if isinstance(rows, str) else rows[0],
                             [(p[0] + 1 - from_page, p[1], p[2], p[3], p[4]) for p in poss]))
        return [(txt, "") for txt, _ in sorted(sections, key=lambda x: (
            x[-1][0][0], x[-1][0][3], x[-1][0][1]))], None


def chunk(filename, binary=None, from_page=0, to_page=100000,
          lang="Chinese", callback=None, **kwargs):
    """
        Supported file formats are docx, pdf, excel, txt.
        One file forms a chunk which maintains original text order.
    """

    eng = lang.lower() == "english"  # is_english(cks)

    if re.search(r"\.docx$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        sections, tbls = naive.Docx()(filename, binary)
        sections = [s for s, _ in sections if s]
        for (_, html), _ in tbls:
            sections.append(html)
        callback(0.8, "Finish parsing.")

    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        pdf_parser = Pdf() if kwargs.get(
            "parser_config", {}).get(
            "layout_recognize", True) else PlainParser()
        sections, _ = pdf_parser(
            filename if not binary else binary, to_page=to_page, callback=callback)
        sections = [s for s, _ in sections if s]

    elif re.search(r"\.xlsx?$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        excel_parser = ExcelParser()
        sections = excel_parser.html(binary, 1000000000)

    elif re.search(r"\.(txt|md|markdown)$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        txt = get_text(filename, binary)
        sections = txt.split("\n")
        sections = [s for s in sections if s]
        callback(0.8, "Finish parsing.")

    elif re.search(r"\.(htm|html)$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        sections = HtmlParser()(filename, binary)
        sections = [s for s in sections if s]
        callback(0.8, "Finish parsing.")

    elif re.search(r"\.doc$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        binary = BytesIO(binary)
        doc_parsed = parser.from_buffer(binary)
        sections = doc_parsed['content'].split('\n')
        sections = [l for l in sections if l]
        callback(0.8, "Finish parsing.")

    else:
        raise NotImplementedError(
            "file type not supported yet(doc, docx, pdf, txt supported)")

    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename))
    }
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])
    tokenize(doc, "\n".join(sections), eng)
    return [doc]


if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass

    chunk(sys.argv[1], from_page=0, to_page=10, callback=dummy)
