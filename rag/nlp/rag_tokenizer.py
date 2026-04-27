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
import re

import infinity.rag_tokenizer
from nltk import word_tokenize
from nltk.stem import SnowballStemmer
from nltk.stem import WordNetLemmatizer

logger = logging.getLogger(__name__)

# Map RAGFlow KB language names (lowercase) to NLTK SnowballStemmer language names.
_SNOWBALL_LANGUAGE_MAP = {
    "english": "english",
    "dutch": "dutch",
    "german": "german",
    "french": "french",
    "spanish": "spanish",
    "italian": "italian",
    "portuguese": "portuguese",
    "portuguese br": "portuguese",
    "russian": "russian",
    "arabic": "arabic",
    "danish": "danish",
    "finnish": "finnish",
    "hungarian": "hungarian",
    "norwegian": "norwegian",
    "romanian": "romanian",
    "swedish": "swedish",
    "turkish": "turkish",
}


class RagTokenizer(infinity.rag_tokenizer.RagTokenizer):

    def __init__(self):
        super().__init__()
        # Default: English stemming + lemmatization (matches parent behaviour)
        self._use_lemmatizer = True

    def set_language(self, language: str):
        """Configure stemmer/lemmatizer for the given KB language.

        Args:
            language: RAGFlow knowledge-base language name (e.g. "English",
                      "Dutch", "Chinese").  Case-insensitive.
        """
        lang_key = language.strip().lower()
        snowball_lang = _SNOWBALL_LANGUAGE_MAP.get(lang_key)

        if snowball_lang is not None:
            self.stemmer = SnowballStemmer(snowball_lang)
            if snowball_lang == "english":
                self.lemmatizer = WordNetLemmatizer()
                self._use_lemmatizer = True
            else:
                # WordNet only supports English; disable lemmatizer for other languages.
                self._use_lemmatizer = False
            logger.debug("Tokenizer language set to '%s' (Snowball: %s, lemmatizer: %s)",
                         language, snowball_lang, self._use_lemmatizer)
        else:
            # Unsupported language (Chinese, Japanese, Korean, etc.) – keep
            # parent defaults (English stemmer/lemmatizer).  The tokenize()
            # path for CJK text does not apply stemming anyway.
            logger.debug("Language '%s' has no Snowball stemmer; keeping defaults", language)

    def _normalize_token(self, t: str) -> str:
        """Stem (and optionally lemmatize) a single token.

        Only applies to pure-alphabetic tokens (including hyphens and
        underscores).  Everything else is returned unchanged.
        """
        if re.match(r"[a-zA-Z_-]+$", t):
            if self._use_lemmatizer:
                return self.stemmer.stem(self.lemmatizer.lemmatize(t))
            return self.stemmer.stem(t)
        return t

    def tokenize(self, line: str) -> str:
        from common import settings  # moved from the top of the file to avoid circular import
        if settings.DOC_ENGINE_INFINITY:
            return line

        # --- replicate parent tokenize() but use _normalize_token() ---
        line = re.sub(r"\W+", " ", line)
        line = self._strQ2B(line).lower()
        line = self._tradi2simp(line)

        arr = self._split_by_lang(line)
        res = []
        for L, lang in arr:
            if not lang:
                # Non-Chinese text: tokenize and normalize
                res.extend([self._normalize_token(t) for t in word_tokenize(L)])
                continue
            if len(L) < 2 or re.match(r"[a-z\.-]+$", L) or re.match(r"[0-9\.-]+$", L):
                res.append(L)
                continue

            # Chinese dictionary segmentation (max_forward + max_backward + dfs_ merge)
            tks, s = self._max_forward(L)
            tks1, s1 = self._max_backward(L)

            if tks1 == tks:
                res.append(" ".join(tks))
            else:
                if s1 > s:
                    tks = tks1
                diff = [0 for _ in range(max(len(tks), len(tks1)))]
                for j in range(min(len(tks), len(tks1))):
                    if tks[j] != tks1[j]:
                        diff[j] = 1

                i = 0
                while i < len(tks):
                    s = i
                    while i < len(diff) and diff[i] == 0:
                        i += 1
                    if i > s:
                        res.append(" ".join(tks[s:i]))
                    if i >= len(diff):
                        break
                    s = i
                    while i < len(diff) and diff[i] == 1:
                        i += 1

                    secs = "".join(tks[s:i])
                    tkslist = []
                    self.dfs_(secs, 0, [], tkslist)
                    if not tkslist:
                        res.append(secs)
                    else:
                        stk = self._sort_tokens(tkslist)[0][0]
                        res.append(" ".join(stk))

        return " ".join(res)

    def fine_grained_tokenize(self, tks: str) -> str:
        from common import settings  # moved from the top of the file to avoid circular import
        if settings.DOC_ENGINE_INFINITY:
            return tks

        # --- replicate parent fine_grained_tokenize() exactly ---
        tks = tks.split(" ")
        res = []
        for tk in tks:
            if len(tk) < 3 or re.match(r"[a-z\.-]+$", tk) or re.match(r"[0-9,\.-]+$", tk):
                res.append(tk)
                continue
            tkslist = []
            if len(tk) >= 30:
                res.append(tk)
                continue
            self.dfs_(tk, 0, [], tkslist)
            if not tkslist:
                res.append(tk)
                continue
            stk = self._sort_tokens(tkslist)[0][0]
            if len(stk) == len(tk):
                res.append(tk)
            else:
                res.append(" ".join(stk))
        return " ".join(res)


def is_chinese(s):
    return infinity.rag_tokenizer.is_chinese(s)


def is_number(s):
    return infinity.rag_tokenizer.is_number(s)


def is_alphabet(s):
    return infinity.rag_tokenizer.is_alphabet(s)


def naive_qie(txt):
    return infinity.rag_tokenizer.naive_qie(txt)


tokenizer = RagTokenizer()
tokenize = tokenizer.tokenize
fine_grained_tokenize = tokenizer.fine_grained_tokenize
tag = tokenizer.tag
freq = tokenizer.freq
tradi2simp = tokenizer._tradi2simp
strQ2B = tokenizer._strQ2B
