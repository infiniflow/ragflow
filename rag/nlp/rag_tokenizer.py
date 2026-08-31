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
import unicodedata

import infinity.rag_tokenizer

# Languages whose tokens are indexed with diacritics folded to ASCII.
# SPLIT_CHAR only captures ASCII letter runs, so accented words are
# otherwise fragmented before indexing ('škola' -> 'š kola'). Neither
# language has a Snowball stemmer, so stemming is disabled for them.
DIACRITIC_FOLDING_LANGUAGES = {"slovak", "czech"}


def fold_diacritics(text: str) -> str:
    """Strip combining marks from latin text: 'škola' -> 'skola'."""
    if not text or text.isascii():
        return text
    return "".join(c for c in unicodedata.normalize("NFD", text) if not unicodedata.combining(c))


class RagTokenizer(infinity.rag_tokenizer.RagTokenizer):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._fold_diacritics = False

    def set_language(self, language: str):
        lang_key = (language or "English").strip().lower()
        self._fold_diacritics = lang_key in DIACRITIC_FOLDING_LANGUAGES
        logging.debug("rag_tokenizer language=%s fold_diacritics=%s", lang_key, self._fold_diacritics)
        if self._fold_diacritics:
            # Force a no-stemming state rather than inheriting whatever stemmer
            # a previously processed dataset left on this shared instance.
            self._use_lemmatizer = False
            return
        super().set_language(language)

    def fold_text(self, text: str) -> str:
        """Fold diacritics iff the current language folds them.

        tokenize() folds on its own; this is for query preprocessing that
        inspects raw text before tokenization (see FulltextQueryer).
        """
        return fold_diacritics(text) if self._fold_diacritics else text

    def _normalize_token(self, t: str) -> str:
        if self._fold_diacritics:
            return t
        return super()._normalize_token(t)

    def tokenize(self, line: str) -> str:
        from common import settings  # moved from the top of the file to avoid circular import

        if settings.DOC_ENGINE_INFINITY:
            return line
        else:
            return super().tokenize(self.fold_text(line))

    def fine_grained_tokenize(self, tks: str) -> str:
        from common import settings  # moved from the top of the file to avoid circular import

        if settings.DOC_ENGINE_INFINITY:
            return tks
        else:
            return super().fine_grained_tokenize(tks)


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
