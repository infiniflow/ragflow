#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
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

import os
import json
import re
import copy
import pandas as pd

current_file_path = os.path.dirname(os.path.abspath(__file__))
TBL = pd.read_csv(
    os.path.join(current_file_path, "res/schools.csv"), sep="\t", header=0
).fillna("")
TBL["name_en"] = TBL["name_en"].map(lambda x: x.lower().strip())
GOOD_SCH = json.load(open(os.path.join(current_file_path, "res/good_sch.json"), "r",encoding="utf-8"))
GOOD_SCH = set([re.sub(r"[,. &（）()]+", "", c) for c in GOOD_SCH])


def loadRank(fnm):
    global TBL
    TBL["rank"] = 1000000
    with open(fnm, "r", encoding="utf-8") as f:
        while True:
            line = f.readline()
            if not line:
                break
            line = line.strip("\n").split(",")
            try:
                nm, rk = line[0].strip(), int(line[1])
                # assert len(TBL[((TBL.name_cn == nm) | (TBL.name_en == nm))]),f"<{nm}>"
                TBL.loc[((TBL.name_cn == nm) | (TBL.name_en == nm)), "rank"] = rk
            except Exception:
                pass


loadRank(os.path.join(current_file_path, "res/school.rank.csv"))


def split(txt):
    tks = []
    for t in re.sub(r"[ \t]+", " ", txt).split():
        if (
            tks
            and re.match(r".*[a-zA-Z]$", tks[-1])
            and re.match(r"[a-zA-Z]", t)
            and tks
        ):
            tks[-1] = tks[-1] + " " + t
        else:
            tks.append(t)
    return tks


def select(nm):
    global TBL
    if not nm:
        return
    if isinstance(nm, list):
        nm = str(nm[0])
    nm = split(nm)[0]
    nm = str(nm).lower().strip()
    nm = re.sub(r"[(（][^()（）]+[)）]", "", nm.lower())
    nm = re.sub(r"(^the |[,.&（）();；·]+|^(英国|美国|瑞士))", "", nm)
    nm = re.sub(r"大学.*学院", "大学", nm)
    tbl = copy.deepcopy(TBL)
    tbl["hit_alias"] = tbl["alias"].map(lambda x: nm in set(x.split("+")))
    res = tbl[((tbl.name_cn == nm) | (tbl.name_en == nm) | tbl.hit_alias)]
    if res.empty:
        return

    return json.loads(res.to_json(orient="records"))[0]


def is_good(nm):
    global GOOD_SCH
    nm = re.sub(r"[(（][^()（）]+[)）]", "", nm.lower())
    nm = re.sub(r"[''`‘’“”,. &（）();；]+", "", nm)
    return nm in GOOD_SCH
