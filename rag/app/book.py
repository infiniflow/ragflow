import copy
import random
import re
import numpy as np
from rag.parser import bullets_category, BULLET_PATTERN, is_english, tokenize, remove_contents_table, \
    hierarchical_merge, make_colon_as_title, naive_merge, random_choices
from rag.nlp import huqie
from rag.parser.docx_parser import HuDocxParser
from rag.parser.pdf_parser import HuParser


class Pdf(HuParser):
    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        self.__images__(
            filename if not binary else binary,
            zoomin,
            from_page,
            to_page)
        callback(0.1, "OCR finished")

        from timeit import default_timer as timer
        start = timer()
        self._layouts_paddle(zoomin)
        callback(0.47, "Layout analysis finished")
        print("paddle layouts:", timer() - start)
        self._table_transformer_job(zoomin)
        callback(0.68, "Table analysis finished")
        self._text_merge()
        self._concat_downward(concat_between_pages=False)
        self._filter_forpages()
        self._merge_with_same_bullet()
        callback(0.75, "Text merging finished.")
        tbls = self._extract_table_figure(True, zoomin, False)

        callback(0.8, "Text extraction finished")

        return [(b["text"] + self._line_tag(b, zoomin), b.get("layoutno","")) for b in self.boxes], tbls


def chunk(filename, binary=None, from_page=0, to_page=100000, callback=None, **kwargs):
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
    sections,tbls = [], []
    if re.search(r"\.docx?$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        doc_parser = HuDocxParser()
        # TODO: table of contents need to be removed
        sections, tbls = doc_parser(binary if binary else filename, from_page=from_page, to_page=to_page)
        remove_contents_table(sections, eng=is_english(random_choices([t for t,_ in sections], k=200)))
        callback(0.8, "Finish parsing.")
    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        pdf_parser = Pdf()
        sections,tbls = pdf_parser(filename if not binary else binary,
                         from_page=from_page, to_page=to_page, callback=callback)
    elif re.search(r"\.txt$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        txt = ""
        if binary:txt = binary.decode("utf-8")
        else:
            with open(filename, "r") as f:
                while True:
                    l = f.readline()
                    if not l:break
                    txt += l
        sections = txt.split("\n")
        sections = [(l,"") for l in sections if l]
        remove_contents_table(sections, eng = is_english(random_choices([t for t,_ in sections], k=200)))
        callback(0.8, "Finish parsing.")
    else: raise NotImplementedError("file type not supported yet(docx, pdf, txt supported)")

    make_colon_as_title(sections)
    bull = bullets_category([t for t in random_choices([t for t,_ in sections], k=100)])
    if bull >= 0: cks = hierarchical_merge(bull, sections, 3)
    else: cks = naive_merge(sections, kwargs.get("chunk_token_num", 256), kwargs.get("delimer", "\n。；！？"))

    sections = [t for t, _ in sections]
    # is it English
    eng = is_english(random_choices(sections, k=218))

    res = []
    # add tables
    for img, rows in tbls:
        bs = 10
        de = ";" if eng else "；"
        for i in range(0, len(rows), bs):
            d = copy.deepcopy(doc)
            r = de.join(rows[i:i + bs])
            r = re.sub(r"\t——(来自| in ).*”%s" % de, "", r)
            tokenize(d, r, eng)
            d["image"] = img
            res.append(d)
            print("TABLE", d["content_with_weight"])
    # wrap up to es documents
    for ck in cks:
        d = copy.deepcopy(doc)
        ck = "\n".join(ck)
        if pdf_parser:
            d["image"] = pdf_parser.crop(ck)
            ck = pdf_parser.remove_tag(ck)
        tokenize(d, ck, eng)
        res.append(d)
    return res


if __name__ == "__main__":
    import sys
    def dummy(a, b):
        pass
    chunk(sys.argv[1], from_page=1, to_page=10, callback=dummy)
