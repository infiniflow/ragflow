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

_BACKTICK_PAT = re.compile(r"`([^`]+)`", re.I)

class RAGFlowTxtParser:
    def __call__(self, fnm:str, binary:bytes|None=None, chunk_token_num:int=128, delimiter:str="\n!?;。；！？"):
        txt = get_text(fnm, binary)
        return self.parser_txt(txt, chunk_token_num, delimiter)

    @classmethod
    def parser_txt(cls, txt:str, chunk_token_num:int=128, delimiter:str="\n!?;。；！？"):
        if not isinstance(txt, str):
            raise TypeError("txt type should be str!")
        cks: list[str] = [""]
        tk_nums: list[int] = [0]
        delimiter = delimiter.encode('utf-8').decode('unicode_escape').encode('latin1').decode('utf-8')

        dels_list: list[str] = []
        pos = 0
        for m in _BACKTICK_PAT.finditer(delimiter):
            start, end = m.span()
            dels_list.append(m.group(1))
            dels_list.extend(delimiter[pos: start])
            pos = end
        if pos < len(delimiter):
            dels_list.extend(delimiter[pos:])

        dels = "|".join(re.escape(d) for d in dels_list if d)
        only_del_pat = re.compile(f"^({dels})$")
        secs = re.split(f"({dels})", txt)

        for sec in secs:
            if only_del_pat.match(sec):
                continue

            tnum = num_tokens_from_string(sec)
            if tk_nums[-1] > chunk_token_num:
                cks.append(sec)
                tk_nums.append(tnum)
            else:
                cks[-1] += sec
                tk_nums[-1] += tnum

        return [[c, ""] for c in cks]
