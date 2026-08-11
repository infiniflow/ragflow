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

import logging
import json
import re
from collections import defaultdict

from common.query_base import QueryBase
from common.doc_store.doc_store_base import MatchTextExpr
from rag.nlp import rag_tokenizer, term_weight, synonym
from rag.utils.redis_conn import REDIS_CONN


class FulltextQueryer(QueryBase):
    def __init__(self):
        self.tw = term_weight.Dealer()
        self.syn = synonym.Dealer(redis=REDIS_CONN.REDIS if REDIS_CONN.is_alive() else None)
        self._entity_tokens_debug: list = []
        self.query_fields = [
            "title_tks^10",
            "title_sm_tks^5",
            "important_kwd^30",
            "important_tks^20",
            "question_tks^20",
            "content_ltks^2",
            "content_sm_ltks",
        ]

    # Words that start with a capital letter in English but are function words /
    # interrogatives (never proper nouns). They are excluded from entity boosting so
    # the query's real entities (person/place names) dominate BM25 instead.
    _ENTITY_STOP = frozenset(
        "what how when where which who why whose whom the this that these those a an and or of in on to for with at by as is are be was were it its not no yes but if then".split()
    )

    # Function words that carry no retrieval signal. They are never boosted as
    # content words. This leaves the query's real content tokens (e.g. "height")
    # boosted, while "the/of/in" stay flat. References/footnote chunks that only
    # repeat the entity name but lack any content token are thereby de-ranked
    # below the chunk that actually carries the requested attribute value.
    _FUNCTION_WORDS = _ENTITY_STOP | frozenset(
        "there here one two three be been being do does did have has had would could should may might must "
        "can will shall about above after against again all am among any are at because before below between "
        "both but by can did do does doing down during each few for from further had has have having he her "
        "here hers herself him himself his how i if into is it its itself just me more most my myself no nor "
        "not now of off on once only or other our ours ourselves out over own same she should so some such "
        "than that the their theirs them themselves then there these they this those through to too under "
        "until up very was we were what when where which while who whom why will with you your yours "
        "yourself yourselves".split()
    )

    def _extract_entity_tokens(self, text):
        """Extract proper-noun tokens (uppercase-initial sequences) from a raw query
        for BM25 boosting. A query like "standing heights of Usain Bolt and Tom
        Daley" carries two entities; weighting them above the ambiguous attribute
        word "height"/"tall" stops BM25 from recalling "tallest building/waterfall"
        instead of the person's infobox chunk. Returns a set of lowercased tokens.

        Must be called with the query BEFORE it is lowercased (``original_query``),
        so only genuinely capitalized words are treated as entities. If none are
        found (e.g. all-lowercase or CJK queries) this returns an empty set and the
        caller leaves weights untouched."""
        if not text:
            return set()
        tokens = set()
        for seq in re.findall(r"[A-Z][a-zA-Z]{2,}(?: [A-Z][a-zA-Z]{2,})*", text):
            if seq.lower() in self._ENTITY_STOP:
                continue
            for tk in rag_tokenizer.tokenize(seq).split():
                if tk:
                    tokens.add(tk)
        return tokens

    def question(self, txt, tbl="qa", min_match: float = 0.6):
        original_query = txt
        txt = self.add_space_between_eng_zh(txt)

        # Strip Infinity ESCAPABLE characters from the query.
        #
        # Infinity's search_lexer.l defines ESCAPABLE characters [\x20()^"'~*?:\\]
        # If these characters appear unescaped in a query, Infinity's lexer will
        # interpret them as special tokens, causing parsing errors.
        txt = re.sub(
            r"[ :|\r\n\t,，。？?/`!！&^%%()\[\]{}<>*~'\"\\]+",
            " ",
            rag_tokenizer.tradi2simp(rag_tokenizer.strQ2B(txt.lower())),
        ).strip()
        otxt = txt
        txt = self.rmWWW(txt)

        if not self.is_chinese(txt):
            txt = self.rmWWW(txt)
            tks = rag_tokenizer.tokenize(txt).split()
            keywords = [t for t in tks if t]
            tks_w = self.tw.weights(tks, preprocess=False)
            tks_w = [(re.sub(r"[ \\\"'^]", "", tk), w) for tk, w in tks_w]
            tks_w = [(re.sub(r"^[\+-]", "", tk), w) for tk, w in tks_w if tk]
            tks_w = [(tk.strip(), w) for tk, w in tks_w if tk.strip()]
            # Boost query terms so the retrieval lands on the right document AND the
            # right attribute inside it:
            #  - proper nouns (entity names, e.g. "usain"/"bolt") get x3 so the search
            #    anchors to the entity's document (an ambiguous attribute word like
            #    "height"/"tall" would otherwise drag in "tallest building");
            #  - other content words (the requested attribute, e.g. "height") get x5.
            #    A document's references/footnotes repeat the entity name many times but
            #    carry none of these content words, so boosting them de-ranks the
            #    footnotes below the chunk that actually holds the attribute value.
            #  - function words ("the/of/in") stay flat.
            _entity_tokens = self._extract_entity_tokens(original_query)
            if _entity_tokens:
                tks_w = [
                    (
                        tk,
                        w * 3.0 if tk in _entity_tokens else (w * 5.0 if tk not in self._FUNCTION_WORDS else w),
                    )
                    for tk, w in tks_w
                ]
            self._entity_tokens_debug = sorted(_entity_tokens)
            syns = []
            for tk, w in tks_w[:256]:
                # Strip single quotes from synonym terms to avoid Infinity lexer TokenError
                # (e.g. WordNet returns "cat-o'-nine-tails" for "cat")
                syn = [rag_tokenizer.tokenize(s).replace("'", "") for s in self.syn.lookup(tk)]
                keywords.extend(syn)
                syn = ['"{}"^{:.4f}'.format(s, w / 4.0) for s in syn if s.strip()]
                syns.append(" ".join(syn))

            q = ["({}^{:.4f}".format(tk, w) + " {})".format(syn) for (tk, w), syn in zip(tks_w, syns) if tk and not re.match(r"[.^+\(\)-]", tk)]
            for i in range(1, len(tks_w)):
                left, right = tks_w[i - 1][0].strip(), tks_w[i][0].strip()
                if not left or not right:
                    continue
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
            if self._entity_tokens_debug:
                logging.info("[EntityBoost] qs=%r keywords=%r", query, keywords)
            return MatchTextExpr(self.query_fields, query, 100, {"original_query": original_query}), keywords

        def need_fine_grained_tokenize(tk):
            if len(tk) < 3:
                return False
            if re.match(r"[0-9a-z\.\+#_\*-]+$", tk):
                return False
            return True

        txt = self.rmWWW(txt)
        qs, keywords = [], []
        for tt in self.tw.split(txt)[:256]:  # .split():
            if not tt:
                continue
            keywords.append(tt)
            twts = self.tw.weights([tt])
            syns = self.syn.lookup(tt)
            if syns and len(keywords) < 32:
                keywords.extend(syns)
            logging.debug(json.dumps(twts, ensure_ascii=False))
            tms = []
            for tk, w in sorted(twts, key=lambda x: x[1] * -1):
                sm = rag_tokenizer.fine_grained_tokenize(tk).split() if need_fine_grained_tokenize(tk) else []
                sm = [
                    re.sub(
                        r"[ ,\./;'\[\]\\`~!@#$%\^&\*\(\)=\+_<>\?:\"\{\}\|，。；‘’【】、！￥……（）——《》？：“”-]+",
                        "",
                        m,
                    )
                    for m in sm
                ]
                sm = [self.sub_special_char(m) for m in sm if len(m) > 1]
                sm = [m for m in sm if len(m) > 1]

                if len(keywords) < 32:
                    keywords.append(re.sub(r"[ \\\"']+", "", tk))
                    keywords.extend(sm)

                tk_syns = self.syn.lookup(tk)
                tk_syns = [self.sub_special_char(s) for s in tk_syns]
                if len(keywords) < 32:
                    keywords.extend([s for s in tk_syns if s])
                tk_syns = [rag_tokenizer.fine_grained_tokenize(s) for s in tk_syns if s]
                tk_syns = [f'"{s}"' if s.find(" ") > 0 else s for s in tk_syns]

                if len(keywords) >= 32:
                    break

                tk = self.sub_special_char(tk)
                if tk.find(" ") > 0:
                    tk = '"%s"' % tk
                if tk_syns:
                    tk = f"({tk} OR (%s)^0.2)" % " ".join(tk_syns)
                if sm:
                    tk = f'{tk} OR "%s" OR ("%s"~2)^0.5' % (" ".join(sm), " ".join(sm))
                if tk.strip():
                    tms.append((tk, w))

            tms = " ".join([f"({t})^{w}" for t, w in tms])

            if len(twts) > 1:
                tms += ' ("%s"~2)^1.5' % rag_tokenizer.tokenize(tt)

            syns = " OR ".join(['"%s"' % rag_tokenizer.tokenize(self.sub_special_char(s)) for s in syns])
            if syns and tms:
                tms = f"({tms})^5 OR ({syns})^0.7"

            qs.append(tms)

        if qs:
            query = " OR ".join([f"({t})" for t in qs if t])
            if not query:
                query = otxt
            return MatchTextExpr(self.query_fields, query, 100, {"minimum_should_match": min_match, "original_query": original_query}), keywords
        return None, keywords

    def hybrid_similarity(self, avec, bvecs, atks, btkss, tkweight=0.3, vtweight=0.7):
        from sklearn.metrics.pairwise import cosine_similarity
        import numpy as np

        sims = cosine_similarity([avec], bvecs)
        tksim = self.token_similarity(atks, btkss)
        if np.sum(sims[0]) == 0:
            return np.array(tksim), tksim, sims[0]
        return np.array(sims[0]) * vtweight + np.array(tksim) * tkweight, tksim, sims[0]

    def token_similarity(self, atks, btkss):
        def to_dict(tks):
            if isinstance(tks, str):
                tks = tks.split()
            d = defaultdict(int)
            wts = self.tw.weights(tks, preprocess=False)
            for i, (t, c) in enumerate(wts):
                d[t] += c * 0.4
                if i + 1 < len(wts):
                    _t, _c = wts[i + 1]
                    d[t + _t] += max(c, _c) * 0.6
            return d

        atks = to_dict(atks)
        btkss = [to_dict(tks) for tks in btkss]
        return [self.similarity(atks, btks) for btks in btkss]

    def similarity(self, qtwt, dtwt):
        if isinstance(dtwt, type("")):
            dtwt = {t: w for t, w in self.tw.weights(self.tw.split(dtwt), preprocess=False)}
        if isinstance(qtwt, type("")):
            qtwt = {t: w for t, w in self.tw.weights(self.tw.split(qtwt), preprocess=False)}
        s = 1e-9
        for k, v in qtwt.items():
            if k in dtwt:
                s += v  # * dtwt[k]
        q = 1e-9
        for k, v in qtwt.items():
            q += v  # * v
        return s / q  # math.sqrt(3. * (s / q / math.log10( len(dtwt.keys()) + 512 )))

    def paragraph(self, content_tks: str, keywords: list = [], keywords_topn=30):
        if isinstance(content_tks, str):
            content_tks = [c.strip() for c in content_tks.split() if c.strip()]
        tks_w = self.tw.weights(content_tks, preprocess=False)

        origin_keywords = keywords.copy()
        keywords = [f'"{k.strip()}"' for k in keywords]
        for tk, w in sorted(tks_w, key=lambda x: x[1] * -1)[:keywords_topn]:
            tk_syns = self.syn.lookup(tk)
            tk_syns = [self.sub_special_char(s) for s in tk_syns]
            tk_syns = [rag_tokenizer.fine_grained_tokenize(s) for s in tk_syns if s]
            tk_syns = [f'"{s}"' if s.find(" ") > 0 else s for s in tk_syns]
            tk = self.sub_special_char(tk)
            if tk.find(" ") > 0:
                tk = '"%s"' % tk
            if tk_syns:
                tk = f"({tk} OR (%s)^0.2)" % " ".join(tk_syns)
            if tk:
                keywords.append(f"{tk}^{w}")

        return MatchTextExpr(self.query_fields, " ".join(keywords), 100, {"minimum_should_match": min(3, round(len(keywords) / 10)), "original_query": " ".join(origin_keywords)})
