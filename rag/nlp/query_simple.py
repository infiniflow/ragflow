#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

import re
from rag.utils.doc_store_conn import MatchTextExpr

from rag.nlp import rag_tokenizer, term_weight


class FulltextQueryer:
    def __init__(self):
        self.tw = term_weight.Dealer()
        self.query_fields = [
            "title_tks^10",
            "title_sm_tks^5",
            "important_kwd^30",
            "important_tks^20",
            "content_ltks^2",
            "content_sm_ltks",
        ]

    @staticmethod
    def rmWWW(txt):
        patts = [
            (
                r"是*(什么样的|哪家|一下|那家|请问|啥样|咋样了|什么时候|何时|何地|何人|是否|是不是|多少|哪里|怎么|哪儿|怎么样|如何|哪些|是啥|啥是|啊|吗|呢|吧|咋|什么|有没有|呀)是*",
                "",
            ),
            (r"(^| )(what|who|how|which|where|why)('re|'s)? ", " "),
            (r"(^| )('s|'re|is|are|were|was|do|does|did|don't|doesn't|didn't|has|have|be|there|you|me|your|my|mine|just|please|may|i|should|would|wouldn't|will|won't|done|go|for|with|so|the|a|an|by|i'm|it's|he's|she's|they|they're|you're|as|by|on|in|at|up|out|down|of) ", " ")
        ]
        for r, p in patts:
            txt = re.sub(r, p, txt, flags=re.IGNORECASE)
        return txt

    def question(self, txt, tbl="qa", min_match:float=0.6):
        # txt = re.sub(
        #     r"[ :\r\n\t,，。？?/`!！&\^%%]+",
        #     " ",
        #     rag_tokenizer.strQ2B(txt.lower()),
        # ).strip()
        txt = re.sub(
            r"[ :\r\n\t,，。？?/`!！&\^%%]+",
            " ",
            txt.lower(),
        ).strip()
        txt = FulltextQueryer.rmWWW(txt)

        if True:
            txt = FulltextQueryer.rmWWW(txt)
            tks = rag_tokenizer.tokenize(txt).split(" ")
            keywords = [t for t in tks if t]

            tks_w = self.tw.weights(tks, preprocess=False)
            tks_w = [(re.sub(r"[ \\\"'^]", "", tk), w) for tk, w in tks_w]
            tks_w = [(re.sub(r"^[a-z0-9]$", "", tk), w) for tk, w in tks_w if tk]
            tks_w = [(re.sub(r"^[\+-]", "", tk), w) for tk, w in tks_w if tk]
            

            q = ["({}^{:.4f}".format(tk, w) + ")" for (tk, w) in tks_w]
            # q = ["({}^{:.4f}".format(tk, w) + " %s)".format(syn) for (tk, w), syn in zip(tks_w, syns)]
            for i in range(1, len(tks_w)):
                q.append(
                    '"%s %s"^%.4f'
                    % (
                        tks_w[i - 1][0],
                        tks_w[i][0],
                        max(tks_w[i - 1][1], tks_w[i][1]) * 2,
                    )
                )
            if not q:
                q.append(txt)
            query = " ".join(q)
            return MatchTextExpr(
                self.query_fields, query, 100
            ), keywords



    # def hybrid_similarity(self, avec, bvecs, atks, btkss, tkweight=0.3, vtweight=0.7):
    #     from sklearn.metrics.pairwise import cosine_similarity as CosineSimilarity
    #     import numpy as np

    #     sims = CosineSimilarity([avec], bvecs)
    #     tksim = self.token_similarity(atks, btkss)
    #     return np.array(sims[0]) * vtweight + np.array(tksim) * tkweight, tksim, sims[0]

    # def token_similarity(self, atks, btkss):
    #     def toDict(tks):
    #         d = {}
    #         if isinstance(tks, str):
    #             tks = tks.split(" ")
    #         for t, c in self.tw.weights(tks, preprocess=False):
    #             if t not in d:
    #                 d[t] = 0
    #             d[t] += c
    #         return d

    #     atks = toDict(atks)
    #     btkss = [toDict(tks) for tks in btkss]
    #     return [self.similarity(atks, btks) for btks in btkss]

    # def similarity(self, qtwt, dtwt):
    #     if isinstance(dtwt, type("")):
    #         dtwt = {t: w for t, w in self.tw.weights(self.tw.split(dtwt), preprocess=False)}
    #     if isinstance(qtwt, type("")):
    #         qtwt = {t: w for t, w in self.tw.weights(self.tw.split(qtwt), preprocess=False)}
    #     s = 1e-9
    #     for k, v in qtwt.items():
    #         if k in dtwt:
    #             s += v  # * dtwt[k]
    #     q = 1e-9
    #     for k, v in qtwt.items():
    #         q += v
    #     return s / q
