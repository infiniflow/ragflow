import copy
import re

from api.db import ParserType
from rag.nlp import huqie, tokenize, tokenize_table, add_positions, bullets_category, title_frequency
from deepdoc.parser import PdfParser
from rag.utils import num_tokens_from_string


class Pdf(PdfParser):
    def __init__(self):
        self.model_speciess = ParserType.MANUAL.value
        super().__init__()

    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        from timeit import default_timer as timer
        start = timer()
        callback(msg="OCR is  running...")
        self.__images__(
            filename if not binary else binary,
            zoomin,
            from_page,
            to_page,
            callback
        )
        callback(msg="OCR finished.")
        #for bb in self.boxes:
        #    for b in bb:
        #        print(b)
        print("OCR:", timer()-start)

        def tag(pn, left, right, top, bottom):
            return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##" \
                .format(pn, left, right, top, bottom)

        self._layouts_rec(zoomin)
        callback(0.65, "Layout analysis finished.")
        print("paddle layouts:", timer() - start)
        self._table_transformer_job(zoomin)
        callback(0.67, "Table analysis finished.")
        self._text_merge()
        tbls = self._extract_table_figure(True, zoomin, True, True)
        self._concat_downward()
        self._filter_forpages()
        callback(0.68, "Text merging finished")

        # clean mess
        for b in self.boxes:
            b["text"] = re.sub(r"([\t 　]|\u3000){2,}", " ", b["text"].strip())

        # set pivot using the most frequent type of title,
        # then merge between 2 pivot
        if len(self.boxes)>0 and len(self.outlines)/len(self.boxes) > 0.1:
            max_lvl = max([lvl for _, lvl in self.outlines])
            most_level = max(0, max_lvl-1)
            levels = []
            for b in self.boxes:
                for t,lvl in self.outlines:
                    tks = set([t[i]+t[i+1] for i in range(len(t)-1)])
                    tks_ = set([b["text"][i]+b["text"][i+1] for i in range(min(len(t), len(b["text"])-1))])
                    if len(set(tks & tks_))/max([len(tks), len(tks_), 1]) > 0.8:
                        levels.append(lvl)
                        break
                else:
                    levels.append(max_lvl + 1)
        else:
            bull = bullets_category([b["text"] for b in self.boxes])
            most_level, levels = title_frequency(bull, [(b["text"], b.get("layout_no","")) for b in self.boxes])

        assert len(self.boxes) == len(levels)
        sec_ids = []
        sid = 0
        for i, lvl in enumerate(levels):
            if lvl <= most_level and i > 0 and lvl != levels[i-1]: sid += 1
            sec_ids.append(sid)
            #print(lvl, self.boxes[i]["text"], most_level, sid)

        sections = [(b["text"], sec_ids[i], self.get_position(b, zoomin)) for i, b in enumerate(self.boxes)]
        for (img, rows), poss in tbls:
            sections.append((rows if isinstance(rows, str) else rows[0], -1, [(p[0]+1-from_page, p[1], p[2], p[3], p[4]) for p in poss]))

        chunks = []
        last_sid = -2
        tk_cnt = 0
        for txt, sec_id, poss in sorted(sections, key=lambda x: (x[-1][0][0], x[-1][0][3], x[-1][0][1])):
            poss = "\t".join([tag(*pos) for pos in poss])
            if tk_cnt < 2048 and (sec_id == last_sid or sec_id == -1):
                if chunks:
                    chunks[-1] += "\n" + txt + poss
                    tk_cnt += num_tokens_from_string(txt)
                    continue
            chunks.append(txt + poss)
            tk_cnt = num_tokens_from_string(txt)
            if sec_id >-1: last_sid = sec_id
        return chunks, tbls


def chunk(filename, binary=None, from_page=0, to_page=100000, lang="Chinese", callback=None, **kwargs):
    """
        Only pdf is supported.
    """
    pdf_parser = None

    if re.search(r"\.pdf$", filename, re.IGNORECASE):
        pdf_parser = Pdf()
        cks, tbls = pdf_parser(filename if not binary else binary,
                           from_page=from_page, to_page=to_page, callback=callback)
    else: raise NotImplementedError("file type not supported yet(pdf supported)")
    doc = {
        "docnm_kwd": filename
    }
    doc["title_tks"] = huqie.qie(re.sub(r"\.[a-zA-Z]+$", "", doc["docnm_kwd"]))
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    # is it English
    eng = lang.lower() == "english"#pdf_parser.is_english

    res = tokenize_table(tbls, doc, eng)
    for ck in cks:
        d = copy.deepcopy(doc)
        d["image"], poss = pdf_parser.crop(ck, need_position=True)
        add_positions(d, poss)
        tokenize(d, pdf_parser.remove_tag(ck), eng)
        res.append(d)
    return res



if __name__ == "__main__":
    import sys
    def dummy(prog=None, msg=""):
        pass
    chunk(sys.argv[1], callback=dummy)
