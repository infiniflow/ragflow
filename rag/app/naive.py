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
from rag.app import laws
from rag.nlp import huqie, is_english, tokenize, naive_merge, tokenize_table, add_positions
from deepdoc.parser import PdfParser
from rag.settings import cron_logger


class Pdf(PdfParser):
    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        callback(msg="OCR is  running...")
        self.__images__(
            filename if not binary else binary,
            zoomin,
            from_page,
            to_page)
        callback(0.1, "OCR finished")

        from timeit import default_timer as timer
        start = timer()
        self._layouts_rec(zoomin)
        callback(0.5, "Layout analysis finished.")
        print("paddle layouts:", timer() - start)
        self._table_transformer_job(zoomin)
        callback(0.7, "Table analysis finished.")
        self._text_merge()
        self._concat_downward(concat_between_pages=False)
        self._filter_forpages()
        callback(0.77, "Text merging finished")
        tbls = self._extract_table_figure(True, zoomin, False, True)

        cron_logger.info("paddle layouts:".format((timer() - start) / (self.total_page + 0.1)))
        #self._naive_vertical_merge()
        return [(b["text"], self._line_tag(b, zoomin)) for b in self.boxes], tbls


def chunk(filename, binary=None, from_page=0, to_page=100000, lang="Chinese", callback=None, **kwargs):
    """
        Supported file formats are docx, pdf, txt.
        This method apply the naive ways to chunk files.
        Successive text will be sliced into pieces using 'delimiter'.
        Next, these successive pieces are merge into chunks whose token number is no more than 'Max token number'.
    """

    eng = lang.lower() == "english"#is_english(cks)
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
        pdf_parser = Pdf()
        sections, tbls = pdf_parser(filename if not binary else binary,
                              from_page=from_page, to_page=to_page, callback=callback)
        res = tokenize_table(tbls, doc, eng)
    elif re.search(r"\.txt$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        txt = ""
        if binary:
            txt = binary.decode("utf-8")
        else:
            with open(filename, "r") as f:
                while True:
                    l = f.readline()
                    if not l: break
                    txt += l
        sections = txt.split("\n")
        sections = [(l, "") for l in sections if l]
        callback(0.8, "Finish parsing.")
    else:
        raise NotImplementedError("file type not supported yet(docx, pdf, txt supported)")

    parser_config = kwargs.get("parser_config", {"chunk_token_num": 128, "delimiter": "\n!?。；！？"})
    cks = naive_merge(sections, parser_config.get("chunk_token_num", 128), parser_config.get("delimiter", "\n!?。；！？"))

    # wrap up to es documents
    for ck in cks:
        if len(ck.strip()) == 0:continue
        print("--", ck)
        d = copy.deepcopy(doc)
        if pdf_parser:
            d["image"], poss = pdf_parser.crop(ck, need_position=True)
            add_positions(d, poss)
            ck = pdf_parser.remove_tag(ck)
        tokenize(d, ck, eng)
        res.append(d)
    return res


if __name__ == "__main__":
    import sys


    def dummy(a, b):
        pass


    chunk(sys.argv[1], from_page=0, to_page=10, callback=dummy)
