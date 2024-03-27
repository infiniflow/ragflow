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
from io import BytesIO

from rag.nlp import bullets_category, is_english, tokenize, remove_contents_table, \
    hierarchical_merge, make_colon_as_title, naive_merge, random_choices, tokenize_table, add_positions, tokenize_chunks
from rag.nlp import huqie
from deepdoc.parser import PdfParser, DocxParser, PlainParser


class Pdf(PdfParser):
    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        callback(msg="OCR is  running...")
        self.__images__(
            filename if not binary else binary,
            zoomin,
            from_page,
            to_page,
            callback)
        callback(msg="OCR finished")

        from timeit import default_timer as timer
        start = timer()
        self._layouts_rec(zoomin)
        callback(0.67, "Layout analysis finished")
        print("paddle layouts:", timer() - start)
        self._table_transformer_job(zoomin)
        callback(0.68, "Table analysis finished")
        self._text_merge()
        tbls = self._extract_table_figure(True, zoomin, True, True)
        self._naive_vertical_merge()
        self._filter_forpages()
        self._merge_with_same_bullet()
        callback(0.75, "Text merging finished.")

        callback(0.8, "Text extraction finished")

        return [(b["text"] + self._line_tag(b, zoomin), b.get("layoutno", ""))
                for b in self.boxes], tbls


def chunk(filename, binary=None, from_page=0, to_page=100000,
          lang="Chinese", callback=None, **kwargs):
    """
        Supported file formats are docx, pdf, txt.
        Since a book is long and not all the parts are useful, if it's a PDF,
        please setup the page ranges for every book in order eliminate negative effects and save elapsed computing time.
    """
    doc = {
        "docnm_kwd": filename,
        "title_tks": huqie.qie(re.sub(r"\.[a-zA-Z]+$", "", filename))
    }
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    pdf_parser = None
    sections, tbls = [], []
    if re.search(r"\.docx?$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        doc_parser = DocxParser()
        # TODO: table of contents need to be removed
        sections, tbls = doc_parser(
            binary if binary else filename, from_page=from_page, to_page=to_page)
        remove_contents_table(sections, eng=is_english(
            random_choices([t for t, _ in sections], k=200)))
        callback(0.8, "Finish parsing.")

    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        pdf_parser = Pdf() if kwargs.get(
            "parser_config", {}).get(
            "layout_recognize", True) else PlainParser()
        sections, tbls = pdf_parser(filename if not binary else binary,
                                    from_page=from_page, to_page=to_page, callback=callback)

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
        remove_contents_table(sections, eng=is_english(
            random_choices([t for t, _ in sections], k=200)))
        callback(0.8, "Finish parsing.")

    else:
        raise NotImplementedError(
            "file type not supported yet(docx, pdf, txt supported)")

    make_colon_as_title(sections)
    bull = bullets_category(
        [t for t in random_choices([t for t, _ in sections], k=100)])
    if bull >= 0:
        chunks = ["\n".join(ck)
                  for ck in hierarchical_merge(bull, sections, 3)]
    else:
        sections = [s.split("@") for s, _ in sections]
        sections = [(pr[0], "@" + pr[1]) for pr in sections if len(pr) == 2]
        chunks = naive_merge(
            sections, kwargs.get(
                "chunk_token_num", 256), kwargs.get(
                "delimer", "\n。；！？"))

    # is it English
    # is_english(random_choices([t for t, _ in sections], k=218))
    eng = lang.lower() == "english"

    res = tokenize_table(tbls, doc, eng)
    res.extend(tokenize_chunks(chunks, doc, eng, pdf_parser))

    return res


if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass
    chunk(sys.argv[1], from_page=1, to_page=10, callback=dummy)
