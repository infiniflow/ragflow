import random
import re
from io import BytesIO
from nltk import word_tokenize
from openpyxl import load_workbook
from rag.parser import is_english
from rag.nlp import huqie, stemmer


class Excel(object):
    def __call__(self, fnm, binary=None, callback=None):
        if not binary:
            wb = load_workbook(fnm)
        else:
            wb = load_workbook(BytesIO(binary))
        total = 0
        for sheetname in wb.sheetnames:
            total += len(list(wb[sheetname].rows))

        res, fails = [], []
        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            rows = list(ws.rows)
            for i, r in enumerate(rows):
                q, a = "", ""
                for cell in r:
                    if not cell.value: continue
                    if not q: q = str(cell.value)
                    elif not a: a = str(cell.value)
                    else: break
                if q and a: res.append((q, a))
                else: fails.append(str(i+1))
                if len(res) % 999 == 0:
                    callback(len(res)*0.6/total, ("Extract Q&A: {}".format(len(res)) + (f"{len(fails)} failure, line: %s..."%(",".join(fails[:3])) if fails else "")))

        callback(0.6, ("Extract Q&A: {}".format(len(res)) + (
            f"{len(fails)} failure, line: %s..." % (",".join(fails[:3])) if fails else "")))
        self.is_english = is_english([rmPrefix(q) for q, _ in random.choices(res, k=30) if len(q)>1])
        return res


def rmPrefix(txt):
    return re.sub(r"^(问题|答案|回答|user|assistant|Q|A|Question|Answer|问|答)[\t:： ]+", "", txt.strip(), flags=re.IGNORECASE)


def beAdoc(d, q, a, eng):
    qprefix = "Question: " if eng else "问题："
    aprefix = "Answer: " if eng else "回答："
    d["content_with_weight"] = "\t".join([qprefix+rmPrefix(q), aprefix+rmPrefix(a)])
    if eng:
        d["content_ltks"] = " ".join([stemmer.stem(w) for w in word_tokenize(q)])
    else:
        d["content_ltks"] = huqie.qie(q)
        d["content_sm_ltks"] = huqie.qieqie(d["content_ltks"])
    return d


def chunk(filename, binary=None, callback=None, **kwargs):

    res = []
    if re.search(r"\.xlsx?$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        excel_parser = Excel()
        for q,a in excel_parser(filename, binary, callback):
            res.append(beAdoc({}, q, a, excel_parser.is_english))
        return res
    elif re.search(r"\.(txt|csv)$", filename, re.IGNORECASE):
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
        lines = txt.split("\n")
        eng = is_english([rmPrefix(l) for l in lines[:100]])
        fails = []
        for i, line in enumerate(lines):
            arr = [l for l in line.split("\t") if len(l) > 1]
            if len(arr) != 2:
                fails.append(str(i))
                continue
            res.append(beAdoc({}, arr[0], arr[1], eng))
            if len(res) % 999 == 0:
                callback(len(res) * 0.6 / len(lines), ("Extract Q&A: {}".format(len(res)) + (
                    f"{len(fails)} failure, line: %s..." % (",".join(fails[:3])) if fails else "")))

        callback(0.6, ("Extract Q&A: {}".format(len(res)) + (
            f"{len(fails)} failure, line: %s..." % (",".join(fails[:3])) if fails else "")))

        return res

    raise NotImplementedError("file type not supported yet(pptx, pdf supported)")


if __name__== "__main__":
    import sys
    def dummy(a, b):
        pass
    chunk(sys.argv[1], callback=dummy)

