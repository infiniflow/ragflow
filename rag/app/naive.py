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
import re
from deepdoc.parser.pdf_parser import PlainParser
from rag.app import laws
from rag.nlp import huqie, is_english, tokenize, naive_merge, tokenize_table, add_positions, tokenize_chunks
from deepdoc.parser import PdfParser, ExcelParser
from rag.settings import cron_logger


class Pdf(PdfParser):
    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        callback(msg="OCR is  running...")
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
        self._layouts_rec(zoomin)
        callback(0.63, "Layout analysis finished.")
        print("paddle layouts:", timer() - start)
        self._table_transformer_job(zoomin)
        callback(0.65, "Table analysis finished.")
        self._text_merge()
        callback(0.67, "Text merging finished")
        tbls = self._extract_table_figure(True, zoomin, True, True)
        #self._naive_vertical_merge()
        self._concat_downward()
        #self._filter_forpages()

        cron_logger.info("paddle layouts:".format(
            (timer() - start) / (self.total_page + 0.1)))
        return [(b["text"], self._line_tag(b, zoomin))
                for b in self.boxes], tbls


def chunk(filename, binary=None, from_page=0, to_page=100000,
          lang="Chinese", callback=None, **kwargs):
    """
        Supported file formats are docx, pdf, excel, txt.
        This method apply the naive ways to chunk files.
        Successive text will be sliced into pieces using 'delimiter'.
        Next, these successive pieces are merge into chunks whose token number is no more than 'Max token number'.
    """

    eng = lang.lower() == "english"  # is_english(cks)
    parser_config = kwargs.get(
        "parser_config", {
            "chunk_token_num": 128, "delimiter": "\n!?。；！？", "layout_recognize": True})
    doc = {
        "docnm_kwd": filename,
        "title_tks": huqie.qie(re.sub(r"\.[a-zA-Z]+$", "", filename))
    }
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    res = []
    pdf_parser = None
    sections = []
    if re.search(r"\.docx?$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        for txt in laws.Docx()(filename, binary):
            sections.append((txt, ""))
        callback(0.8, "Finish parsing.")

    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        pdf_parser = Pdf(
        ) if parser_config.get("layout_recognize", True) else PlainParser()
        sections, tbls = pdf_parser(filename if not binary else binary,
                                    from_page=from_page, to_page=to_page, callback=callback)
        res = tokenize_table(tbls, doc, eng)

    elif re.search(r"\.xlsx?$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        excel_parser = ExcelParser()
        sections = [(excel_parser.html(binary), "")]

    elif re.search(r"\.txt$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        txt = ""
        if binary:
            txt = binary.decode("utf-8")
        else:
            with open(filename, "r") as f:
                while True:
                    l = f.readline()
                    if not l:
                        break
                    txt += l
        sections = txt.split("\n")
        sections = [(l, "") for l in sections if l]
        callback(0.8, "Finish parsing.")

    else:
        raise NotImplementedError(
            "file type not supported yet(docx, pdf, txt supported)")

    chunks = naive_merge(
        sections, parser_config.get(
            "chunk_token_num", 128), parser_config.get(
            "delimiter", "\n!?。；！？"))

    res.extend(tokenize_chunks(chunks, doc, eng, pdf_parser))
    return res


if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass

    chunk(sys.argv[1], from_page=0, to_page=10, callback=dummy)
