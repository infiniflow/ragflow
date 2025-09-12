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

import logging
import re
import json
import os
import pandas as pd
from rag.nlp import rag_tokenizer
from . import regions


current_file_path = os.path.dirname(os.path.abspath(__file__))
GOODS = pd.read_csv(
    os.path.join(current_file_path, "res/corp_baike_len.csv"), sep="\t", header=0
).fillna(0)
GOODS["cid"] = GOODS["cid"].astype(str)
GOODS = GOODS.set_index(["cid"])
CORP_TKS = json.load(
    open(os.path.join(current_file_path, "res/corp.tks.freq.json"), "r",encoding="utf-8")
)
GOOD_CORP = json.load(open(os.path.join(current_file_path, "res/good_corp.json"), "r",encoding="utf-8"))
CORP_TAG = json.load(open(os.path.join(current_file_path, "res/corp_tag.json"), "r",encoding="utf-8"))


def baike(cid, default_v=0):
    global GOODS
    try:
        return GOODS.loc[str(cid), "len"]
    except Exception:
        pass
    return default_v


def corpNorm(nm, add_region=True):
    global CORP_TKS
    if not nm or not isinstance(nm, str):
        return ""
    nm = rag_tokenizer.tradi2simp(rag_tokenizer.strQ2B(nm)).lower()
    nm = re.sub(r"&amp;", "&", nm)
    nm = re.sub(r"[\(\)（）\+'\"\t \*\\【】-]+", " ", nm)
    nm = re.sub(
        r"([—-]+.*| +co\..*|corp\..*| +inc\..*| +ltd.*)", "", nm, count=10000, flags=re.IGNORECASE
    )
    nm = re.sub(
        r"(计算机|技术|(技术|科技|网络)*有限公司|公司|有限|研发中心|中国|总部)$",
        "",
        nm,
        count=10000,
        flags=re.IGNORECASE,
    )
    if not nm or (len(nm) < 5 and not regions.isName(nm[0:2])):
        return nm

    tks = rag_tokenizer.tokenize(nm).split()
    reg = [t for i, t in enumerate(tks) if regions.isName(t) and (t != "中国" or i > 0)]
    nm = ""
    for t in tks:
        if regions.isName(t) or t in CORP_TKS:
            continue
        if re.match(r"[0-9a-zA-Z\\,.]+", t) and re.match(r".*[0-9a-zA-Z\,.]+$", nm):
            nm += " "
        nm += t

    r = re.search(r"^([^a-z0-9 \(\)&]{2,})[a-z ]{4,}$", nm.strip())
    if r:
        nm = r.group(1)
    r = re.search(r"^([a-z ]{3,})[^a-z0-9 \(\)&]{2,}$", nm.strip())
    if r:
        nm = r.group(1)
    return nm.strip() + (("" if not reg else "(%s)" % reg[0]) if add_region else "")


def rmNoise(n):
    n = re.sub(r"[\(（][^()（）]+[)）]", "", n)
    n = re.sub(r"[,. &（）()]+", "", n)
    return n


GOOD_CORP = set([corpNorm(rmNoise(c), False) for c in GOOD_CORP])
for c, v in CORP_TAG.items():
    cc = corpNorm(rmNoise(c), False)
    if not cc:
        logging.debug(c)
CORP_TAG = {corpNorm(rmNoise(c), False): v for c, v in CORP_TAG.items()}


def is_good(nm):
    global GOOD_CORP
    if nm.find("外派") >= 0:
        return False
    nm = rmNoise(nm)
    nm = corpNorm(nm, False)
    for n in GOOD_CORP:
        if re.match(r"[0-9a-zA-Z]+$", n):
            if n == nm:
                return True
        elif nm.find(n) >= 0:
            return True
    return False


def corp_tag(nm):
    global CORP_TAG
    nm = rmNoise(nm)
    nm = corpNorm(nm, False)
    for n in CORP_TAG.keys():
        if re.match(r"[0-9a-zA-Z., ]+$", n):
            if n == nm:
                return CORP_TAG[n]
        elif nm.find(n) >= 0:
            if len(n) < 3 and len(nm) / len(n) >= 2:
                continue
            return CORP_TAG[n]
    return []
