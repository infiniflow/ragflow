import copy
import re
from io import BytesIO
from docx import Document
import numpy as np
from rag.app import callback__, bullets_category, BULLET_PATTERN
from rag.nlp import huqie
from rag.parser.pdf_parser import HuParser


class Docx(object):
    def __init__(self):
        pass

    def __clean(self, line):
        line = re.sub(r"\u3000", " ", line).strip()
        return line

    def __call__(self, filename, binary=None):
        self.doc = Document(
            filename) if not binary else Document(BytesIO(binary))
        lines = [self.__clean(p.text) for p in self.doc.paragraphs]
        return [l for l in lines if l]


class Pdf(HuParser):
    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        self.__images__(
            filename if not binary else binary,
            zoomin,
            from_page,
            to_page)
        callback__((min(to_page, self.total_page) - from_page) / self.total_page / 2,
                   "Page {}~{}: OCR finished".format(from_page, min(to_page, self.total_page)), callback)

        from timeit import default_timer as timer
        start = timer()
        self._layouts_paddle(zoomin)
        callback__((min(to_page, self.total_page) - from_page) / self.total_page / 2,
                   "Page {}~{}: Layout analysis finished".format(from_page, min(to_page, self.total_page)), callback)
        print("paddle layouts:", timer()-start)
        bxs = self.sort_Y_firstly(self.boxes, np.median(self.mean_height) / 3)
        # is it English
        eng = 0
        for b in bxs:
            if re.match(r"[a-zA-Z]", b["text"].strip()):
                eng += 1
        if eng / len(bxs) > 0.8:
            eng = True
        else:
            eng = False
        # Merge vertically
        i = 0
        while i + 1 < len(bxs):
            b = bxs[i]
            b_ = bxs[i + 1]
            if b["page_number"] < b_["page_number"] and re.match(r"[0-9  •一—-]+$", b["text"]):
                bxs.pop(i)
                continue
            concatting_feats = [
                b["text"].strip()[-1] in ",;:'\"，、‘“；：",
                len(b["text"].strip())>1 and b["text"].strip()[-2] in ",;:'\"，‘“、；：",
                b["text"].strip()[0] in "。；？！?”）),，、：",
            ]
            # features for not concating
            feats = [
                b.get("layoutno",0) != b.get("layoutno",0),
                b["text"].strip()[-1] in "。？！?",
                eng and b["text"].strip()[-1] in ".!?",
                b["page_number"] == b_["page_number"] and b_["top"] - \
                b["bottom"] > self.mean_height[b["page_number"] - 1] * 1.5,
                b["page_number"] < b_["page_number"] and abs(
                    b["x0"] - b_["x0"]) > self.mean_width[b["page_number"] - 1] * 4
            ]
            if any(feats) and not any(concatting_feats):
                i += 1
                continue
            # merge up and down
            b["bottom"] = b_["bottom"]
            b["text"] += b_["text"]
            b["x0"] = min(b["x0"], b_["x0"])
            b["x1"] = max(b["x1"], b_["x1"])
            bxs.pop(i + 1)

        callback__((min(to_page, self.total_page) - from_page) / self.total_page / 2,
                   "Page {}~{}: Text extraction finished".format(from_page, min(to_page, self.total_page)), callback)

        return [b["text"] + self._line_tag(b, zoomin) for b in bxs]


def chunk(filename, binary=None, from_page=0, to_page=100000, callback=None):
    doc = {
        "docnm_kwd": filename,
        "title_tks": huqie.qie(re.sub(r"\.[a-zA-Z]+$", "", filename))
    }
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    pdf_parser = None
    sections = []
    if re.search(r"\.docx?$", filename, re.IGNORECASE):
        for txt in Docx()(filename, binary):
            sections.append(txt)
    if re.search(r"\.pdf$", filename, re.IGNORECASE):
        pdf_parser = Pdf()
        for txt in pdf_parser(filename if not binary else binary,
                         from_page=from_page, to_page=to_page, callback=callback):
            sections.append(txt)
    if re.search(r"\.txt$", filename, re.IGNORECASE):
        txt = ""
        if binary:txt = binary.decode("utf-8")
        else:
            with open(filename, "r") as f:
                while True:
                    l = f.readline()
                    if not l:break
                    txt += l
            sections = txt.split("\n")
        sections = [l for l in sections if l]

    # is it English
    eng = 0
    for sec in sections:
        if re.match(r"[a-zA-Z]", sec.strip()):
            eng += 1
    if eng / len(sections) > 0.8:
        eng = True
    else:
        eng = False
    # Remove 'Contents' part
    i = 0
    while i < len(sections):
        if not re.match(r"(Contents|目录|目次)$", re.sub(r"( | |\u3000)+", "", sections[i].split("@@")[0])):
            i += 1
            continue
        sections.pop(i)
        if i >= len(sections): break
        prefix = sections[i].strip()[:3] if not eng else " ".join(sections[i].strip().split(" ")[:2])
        while not prefix:
            sections.pop(i)
            if i >= len(sections): break
            prefix = sections[i].strip()[:3] if not eng else " ".join(sections[i].strip().split(" ")[:2])
        sections.pop(i)
        if i >= len(sections) or not prefix: break
        for j in range(i, min(i+128, len(sections))):
            if not re.match(prefix, sections[j]):
                continue
            for k in range(i, j):sections.pop(i)
            break

    bull = bullets_category(sections)
    projs = [len(BULLET_PATTERN[bull])] * len(sections)
    for i, sec in enumerate(sections):
        for j,p in enumerate(BULLET_PATTERN[bull]):
            if re.match(p, sec.strip()):
                projs[i] = j
                break
    readed = [0] * len(sections)
    cks = []
    for pr in range(len(BULLET_PATTERN[bull])-1, 1, -1):
        for i in range(len(sections)):
            if readed[i] or projs[i] < pr:
                continue
            # find father and grand-father and grand...father
            p = projs[i]
            readed[i] = 1
            ck = [sections[i]]
            for j in range(i-1, -1, -1):
                if projs[j] >= p:continue
                ck.append(sections[j])
                readed[j] = 1
                p = projs[j]
                if p == 0: break
            cks.append(ck[::-1])

    res = []
    # wrap up to es documents
    for ck in cks:
        print("\n-".join(ck))
        ck = "\n".join(ck)
        d = copy.deepcopy(doc)
        if pdf_parser:
            d["image"] = pdf_parser.crop(ck)
            ck = pdf_parser.remove_tag(ck)
        d["content_ltks"] = huqie.qie(ck)
        d["content_sm_ltks"] = huqie.qieqie(d["content_ltks"])
        res.append(d)
    return res


if __name__ == "__main__":
    import sys
    chunk(sys.argv[1])
