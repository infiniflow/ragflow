import copy
import random
import re
from io import BytesIO
from docx import Document
import numpy as np
from rag.app import bullets_category, BULLET_PATTERN, is_english, tokenize, remove_contents_table
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
        column_width = np.median([b["x1"] - b["x0"] for b in self.boxes])
        self._concat_downward(concat_between_pages=False)
        self._filter_forpages()
        self._merge_with_same_bullet()
        callback(0.75, "Text merging finished.")
        tbls = self._extract_table_figure(True, zoomin, False)

        callback(0.8, "Text extraction finished")

        return [(b["text"] + self._line_tag(b, zoomin), b.get("layoutno","")) for b in self.boxes]


def chunk(filename, binary=None, from_page=0, to_page=100000, callback=None):
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
        sections, tbls = doc_parser(binary if binary else filename)
        remove_contents_table(sections, eng = is_english(random.choices([t for t,_ in sections], k=200)))
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
        remove_contents_table(sections, eng = is_english(random.choices([t for t,_ in sections], k=200)))
        callback(0.8, "Finish parsing.")
    else: raise NotImplementedError("file type not supported yet(docx, pdf, txt supported)")

    bull = bullets_category([b["text"] for b in random.choices([t for t,_ in sections], k=100)])
    projs = [len(BULLET_PATTERN[bull]) + 1] * len(sections)
    levels = [[]] * len(BULLET_PATTERN[bull]) + 2
    for i, (txt, layout) in enumerate(sections):
        for j, p in enumerate(BULLET_PATTERN[bull]):
            if re.match(p, txt.strip()):
                projs[i] = j
                levels[j].append(i)
                break
        else:
            if re.search(r"(title|head)", layout):
                projs[i] = BULLET_PATTERN[bull]
                levels[BULLET_PATTERN[bull]].append(i)
            else:
                levels[BULLET_PATTERN[bull] + 1].append(i)
    sections = [t for t,_ in sections]

    def binary_search(arr, target):
        if target > arr[-1]: return len(arr) - 1
        if target > arr[0]: return -1
        s, e = 0, len(arr)
        while e - s > 1:
            i = (e + s) // 2
            if target > arr[i]:
                s = i
                continue
            elif target < arr[i]:
                e = i
                continue
            else:
                assert False
        return s

    cks = []
    readed = [False] * len(sections)
    levels = levels[::-1]
    for i, arr in enumerate(levels):
        for j in arr:
            if readed[j]: continue
            readed[j] = True
            cks.append([j])
            if i + 1 == len(levels) - 1: continue
            for ii in range(i + 1, len(levels)):
                jj = binary_search(levels[ii], j)
                if jj < 0: break
                if jj > cks[-1][-1]: cks[-1].pop(-1)
                cks[-1].append(levels[ii][jj])

    # is it English
    eng = is_english(random.choices(sections, k=218))

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
    # wrap up to es documents
    for ck in cks:
        print("\n-".join(ck[::-1]))
        ck = "\n".join(ck[::-1])
        d = copy.deepcopy(doc)
        if pdf_parser:
            d["image"] = pdf_parser.crop(ck)
            ck = pdf_parser.remove_tag(ck)
        tokenize(d, ck, eng)
        res.append(d)
    return res


if __name__ == "__main__":
    import sys
    chunk(sys.argv[1])
