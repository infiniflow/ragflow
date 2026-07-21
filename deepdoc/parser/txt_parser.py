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

from deepdoc.parser.utils import get_text
from common.token_utils import num_tokens_from_string


def _split_oversized_text(text, chunk_token_num):
    """Break a single unit that exceeds ``chunk_token_num`` tokens into pieces
    that each fit the budget. Whitespace is used as the primary break (mirrors
    ``RAGFlowHtmlParser._split_oversized_block``); a single run of non-whitespace
    longer than the budget falls back to fixed-size character windows.
    """
    if num_tokens_from_string(text or "") <= chunk_token_num:
        return [text]
    pieces = []
    current = ""
    current_tokens = 0
    token_cache = {}

    def atom_tokens(atom):
        if atom.isspace():
            return 0
        if atom not in token_cache:
            token_cache[atom] = num_tokens_from_string(atom)
        return token_cache[atom]

    for atom in re.findall(r"\s+|\S+", text or ""):
        a_tokens = atom_tokens(atom)
        if a_tokens > chunk_token_num and not atom.isspace():
            if current:
                pieces.append(current)
                current = ""
                current_tokens = 0
            for i in range(0, len(atom), chunk_token_num):
                pieces.append(atom[i : i + chunk_token_num])
            continue
        if current and current_tokens + a_tokens > chunk_token_num:
            pieces.append(current)
            current = ""
            current_tokens = 0
        current += atom
        current_tokens += a_tokens
    if current:
        pieces.append(current)
    return pieces


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

            if tk_nums[-1] + tnum <= chunk_token_num:
                cks[-1] += "\n" + t
                tk_nums[-1] += tnum
                return

            cks.append(t)
            tk_nums.append(tnum)

        dels = []
        s = 0
        for m in re.finditer(r"`([^`]+)`", delimiter, re.I):
            f, t = m.span()
            dels.append(m.group(1))
            dels.extend(list(delimiter[s:f]))
            s = t
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
            for piece in _split_oversized_text(sec, chunk_token_num):
                add_chunk(piece)

        return [[c, ""] for c in cks]
