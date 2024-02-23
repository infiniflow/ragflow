# -*- coding: UTF-8 -*-
import os, json,re,copy
import pandas as pd
current_file_path = os.path.dirname(os.path.abspath(__file__))
TBL = pd.read_csv(os.path.join(current_file_path, "res/schools.csv"), sep="\t", header=0).fillna("")
TBL["name_en"] = TBL["name_en"].map(lambda x: x.lower().strip())
GOOD_SCH = json.load(open(os.path.join(current_file_path, "res/good_sch.json"), "r"))
GOOD_SCH = set([re.sub(r"[,. &（）()]+", "", c) for c in GOOD_SCH])


def loadRank(fnm):
    global TBL
    TBL["rank"] = 1000000
    with open(fnm, "r",encoding='UTF-8') as f:
        while True:
            l = f.readline()
            if not l:break
            l = l.strip("\n").split(",")
            try:
                nm,rk = l[0].strip(),int(l[1])
                #assert len(TBL[((TBL.name_cn == nm) | (TBL.name_en == nm))]),f"<{nm}>"
                TBL.loc[((TBL.name_cn == nm) | (TBL.name_en == nm)), "rank"] = rk
            except Exception as e:
                pass


loadRank(os.path.join(current_file_path, "res/school.rank.csv"))


def split(txt):
    tks = []
    for t in re.sub(r"[ \t]+", " ",txt).split(" "):
        if tks and re.match(r".*[a-zA-Z]$", tks[-1]) and \
           re.match(r"[a-zA-Z]", t) and tks:
            tks[-1] = tks[-1] + " " + t
        else:tks.append(t)
    return tks


def select(nm):
    global TBL
    if not nm:return 
    if isinstance(nm, list):nm = str(nm[0])
    nm = split(nm)[0]
    nm = str(nm).lower().strip()
    nm = re.sub(r"[(（][^()（）]+[)）]", "", nm.lower())
    nm = re.sub(r"(^the |[,.&（）();；·]+|^(英国|美国|瑞士))", "", nm)
    nm = re.sub(r"大学.*学院", "大学", nm)
    tbl = copy.deepcopy(TBL)
    tbl["hit_alias"] = tbl["alias"].map(lambda x:nm in set(x.split("+")))
    res = tbl[((tbl.name_cn == nm) | (tbl.name_en == nm) | (tbl.hit_alias == True))]
    if res.empty:return

    return json.loads(res.to_json(orient="records"))[0]


def is_good(nm):
    global GOOD_SCH
    nm = re.sub(r"[(（][^()（）]+[)）]", "", nm.lower())
    nm = re.sub(r"[''`‘’“”,. &（）();；]+", "", nm)
    return nm in GOOD_SCH

