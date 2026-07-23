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

import re
import logging

from deepdoc.parser.utils import get_text
from common.token_utils import num_tokens_from_string
from rag.nlp import _split_oversized_unit


class RAGFlowTxtParser:
    def __call__(self, fnm, binary=None, chunk_token_num=128, delimiter="\n!?;。；！？"):
        txt = get_text(fnm, binary)
        return self.parser_txt(txt, chunk_token_num, delimiter)

    @classmethod
    def parser_txt(cls, txt, chunk_token_num=128, delimiter="\n!?;。；！？"):
        if not isinstance(txt, str):
            raise TypeError("txt type should be str!")
        cks = [""]
        tk_nums = [0]
        delimiter = delimiter.encode("utf-8").decode("unicode_escape").encode("latin1").decode("utf-8")

        def add_chunk(t):
            nonlocal cks, tk_nums, delimiter
            tnum = num_tokens_from_string(t)

            if cks[-1] == "":
                cks[-1] = t
                tk_nums[-1] = tnum
                return

            merged = cks[-1] + "\n" + t
            merged_tnum = num_tokens_from_string(merged)
            if merged_tnum <= chunk_token_num:
                cks[-1] = merged
                tk_nums[-1] = merged_tnum
                return

            cks.append(t)
            tk_nums.append(tnum)

        dels = []
        s = 0
        for m in re.finditer(r"`([^`]+)`", delimiter, re.I):
            f, m_t = m.span()
            dels.append(m.group(1))
            dels.extend(list(delimiter[s:f]))
            s = m_t
        if s < len(delimiter):
            dels.extend(list(delimiter[s:]))
        dels = [re.escape(d) for d in dels if d]
        dels = [d for d in dels if d]
        dels = "|".join(dels)
        secs = re.split(r"(%s)" % dels, txt)
        for sec in secs:
            if re.match(f"^{dels}$", sec):
                continue
            if not sec:
                continue
            if num_tokens_from_string(sec) <= chunk_token_num:
                add_chunk(sec)
                continue
            pieces = _split_oversized_unit(sec, chunk_token_num, token_count_fn=num_tokens_from_string)
            logging.debug("parser_txt: split oversized section (%d tokens) into %d pieces", num_tokens_from_string(sec), len(pieces))
            for piece in pieces:
                add_chunk(piece)

        logging.debug("parser_txt: %d sections -> %d chunks (chunk_token_num=%d)", len(secs), len(cks), chunk_token_num)
        return [[c, ""] for c in cks]
