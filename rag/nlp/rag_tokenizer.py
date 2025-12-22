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

from common import settings

try:
    import infinity.rag_tokenizer
    HAS_INFINITY_TOKENIZER = True
except ImportError:
    HAS_INFINITY_TOKENIZER = False
    class DummyRagTokenizer:
        def tokenize(self, line: str) -> str:
            return line
        def fine_grained_tokenize(self, tks: str) -> str:
            return tks
        def tag(self, txt):
            return []
        def freq(self, txt):
            return {}
        def _tradi2simp(self, txt):
            return txt
        def _strQ2B(self, txt):
            return txt

if HAS_INFINITY_TOKENIZER:
    class RagTokenizer(infinity.rag_tokenizer.RagTokenizer):
        def tokenize(self, line: str) -> str:
            if settings.DOC_ENGINE_INFINITY:
                return line
            else:
                return super().tokenize(line)
        def fine_grained_tokenize(self, tks: str) -> str:
            if settings.DOC_ENGINE_INFINITY:
                return tks
            else:
                return super().fine_grained_tokenize(tks)
else:
    class RagTokenizer(DummyRagTokenizer):
        def tokenize(self, line: str) -> str:
            return line
        def fine_grained_tokenize(self, tks: str) -> str:
            return tks

def is_chinese(s):
    if HAS_INFINITY_TOKENIZER:
        return infinity.rag_tokenizer.is_chinese(s)
    return any('\u4e00' <= char <= '\u9fff' for char in s)

def is_number(s):
    if HAS_INFINITY_TOKENIZER:
        return infinity.rag_tokenizer.is_number(s)
    try:
        float(s)
        return True
    except ValueError:
        return False

def is_alphabet(s):
    if HAS_INFINITY_TOKENIZER:
        return infinity.rag_tokenizer.is_alphabet(s)
    return s.isalpha()

def naive_qie(txt):
    if HAS_INFINITY_TOKENIZER:
        return infinity.rag_tokenizer.naive_qie(txt)
    return txt

tokenizer = RagTokenizer()
tokenize = tokenizer.tokenize
fine_grained_tokenize = tokenizer.fine_grained_tokenize
tag = tokenizer.tag
freq = tokenizer.freq
tradi2simp = tokenizer._tradi2simp
strQ2B = tokenizer._strQ2B
