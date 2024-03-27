# -*- coding: utf-8 -*-
import math
import json
import re
import os
import numpy as np
from rag.nlp import huqie
from api.utils.file_utils import get_project_base_directory


class Dealer:
    def __init__(self):
        self.stop_words = set(["请问",
                               "您",
                               "你",
                               "我",
                               "他",
                               "是",
                               "的",
                               "就",
                               "有",
                               "于",
                               "及",
                               "即",
                               "在",
                               "为",
                               "最",
                               "有",
                               "从",
                               "以",
                               "了",
                               "将",
                               "与",
                               "吗",
                               "吧",
                               "中",
                               "#",
                               "什么",
                               "怎么",
                               "哪个",
                               "哪些",
                               "啥",
                               "相关"])

        def load_dict(fnm):
            res = {}
            f = open(fnm, "r")
            while True:
                l = f.readline()
                if not l:
                    break
                arr = l.replace("\n", "").split("\t")
                if len(arr) < 2:
                    res[arr[0]] = 0
                else:
                    res[arr[0]] = int(arr[1])

            c = 0
            for _, v in res.items():
                c += v
            if c == 0:
                return set(res.keys())
            return res

        fnm = os.path.join(get_project_base_directory(), "rag/res")
        self.ne, self.df = {}, {}
        try:
            self.ne = json.load(open(os.path.join(fnm, "ner.json"), "r"))
        except Exception as e:
            print("[WARNING] Load ner.json FAIL!")
        try:
            self.df = load_dict(os.path.join(fnm, "term.freq"))
        except Exception as e:
            print("[WARNING] Load term.freq FAIL!")

    def pretoken(self, txt, num=False, stpwd=True):
        patt = [
            r"[~—\t @#%!<>,\.\?\":;'\{\}\[\]_=\(\)\|，。？》•●○↓《；‘’：“”【¥ 】…￥！、·（）×`&\\/「」\\]"
        ]
        rewt = [
        ]
        for p, r in rewt:
            txt = re.sub(p, r, txt)

        res = []
        for t in huqie.qie(txt).split(" "):
            tk = t
            if (stpwd and tk in self.stop_words) or (
                    re.match(r"[0-9]$", tk) and not num):
                continue
            for p in patt:
                if re.match(p, t):
                    tk = "#"
                    break
            tk = re.sub(r"([\+\\-])", r"\\\1", tk)
            if tk != "#" and tk:
                res.append(tk)
        return res

    def tokenMerge(self, tks):
        def oneTerm(t): return len(t) == 1 or re.match(r"[0-9a-z]{1,2}$", t)

        res, i = [], 0
        while i < len(tks):
            j = i
            if i == 0 and oneTerm(tks[i]) and len(
                    tks) > 1 and len(tks[i + 1]) > 1:  # 多 工位
                res.append(" ".join(tks[0:2]))
                i = 2
                continue

            while j < len(
                    tks) and tks[j] and tks[j] not in self.stop_words and oneTerm(tks[j]):
                j += 1
            if j - i > 1:
                if j - i < 5:
                    res.append(" ".join(tks[i:j]))
                    i = j
                else:
                    res.append(" ".join(tks[i:i + 2]))
                    i = i + 2
            else:
                if len(tks[i]) > 0:
                    res.append(tks[i])
                i += 1
        return [t for t in res if t]

    def ner(self, t):
        if not self.ne:
            return ""
        res = self.ne.get(t, "")
        if res:
            return res

    def split(self, txt):
        tks = []
        for t in re.sub(r"[ \t]+", " ", txt).split(" "):
            if tks and re.match(r".*[a-zA-Z]$", tks[-1]) and \
               re.match(r".*[a-zA-Z]$", t) and tks and \
               self.ne.get(t, "") != "func" and self.ne.get(tks[-1], "") != "func":
                tks[-1] = tks[-1] + " " + t
            else:
                tks.append(t)
        return tks

    def weights(self, tks):
        def skill(t):
            if t not in self.sk:
                return 1
            return 6

        def ner(t):
            if re.match(r"[0-9,.]{2,}$", t):
                return 2
            if re.match(r"[a-z]{1,2}$", t):
                return 0.01
            if not self.ne or t not in self.ne:
                return 1
            m = {"toxic": 2, "func": 1, "corp": 3, "loca": 3, "sch": 3, "stock": 3,
                 "firstnm": 1}
            return m[self.ne[t]]

        def postag(t):
            t = huqie.tag(t)
            if t in set(["r", "c", "d"]):
                return 0.3
            if t in set(["ns", "nt"]):
                return 3
            if t in set(["n"]):
                return 2
            if re.match(r"[0-9-]+", t):
                return 2
            return 1

        def freq(t):
            if re.match(r"[0-9. -]{2,}$", t):
                return 3
            s = huqie.freq(t)
            if not s and re.match(r"[a-z. -]+$", t):
                return 300
            if not s:
                s = 0

            if not s and len(t) >= 4:
                s = [tt for tt in huqie.qieqie(t).split(" ") if len(tt) > 1]
                if len(s) > 1:
                    s = np.min([freq(tt) for tt in s]) / 6.
                else:
                    s = 0

            return max(s, 10)

        def df(t):
            if re.match(r"[0-9. -]{2,}$", t):
                return 5
            if t in self.df:
                return self.df[t] + 3
            elif re.match(r"[a-z. -]+$", t):
                return 300
            elif len(t) >= 4:
                s = [tt for tt in huqie.qieqie(t).split(" ") if len(tt) > 1]
                if len(s) > 1:
                    return max(3, np.min([df(tt) for tt in s]) / 6.)

            return 3

        def idf(s, N): return math.log10(10 + ((N - s + 0.5) / (s + 0.5)))

        tw = []
        for tk in tks:
            tt = self.tokenMerge(self.pretoken(tk, True))
            idf1 = np.array([idf(freq(t), 10000000) for t in tt])
            idf2 = np.array([idf(df(t), 1000000000) for t in tt])
            wts = (0.3 * idf1 + 0.7 * idf2) * \
                np.array([ner(t) * postag(t) for t in tt])

            tw.extend(zip(tt, wts))

        S = np.sum([s for _, s in tw])
        return [(t, s / S) for t, s in tw]
