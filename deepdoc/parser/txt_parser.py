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

from deepdoc.parser.utils import get_text
from common.token_utils import num_tokens_from_string


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
            # Enforce chunk_token_num as a HARD cap. The previous implementation
            # checked ``tk_nums[-1] > chunk_token_num`` *after* appending, so
            # every chunk could overshoot by the size of one segment; some
            # pathological inputs (a single very long line with no internal
            # delimiter) produced chunks 100x larger than the configured
            # budget (see issue #17202).
            #
            # The check is now *predictive*: if the current non-empty chunk
            # plus the incoming segment would exceed the budget, start a new
            # chunk. An empty current chunk is just the first-write slot
            # and is always filled before opening a new one. When a single
            # segment is itself larger than the budget, it is emitted as
            # one oversized chunk and a warning is logged so the operator
            # knows the chunker cannot satisfy the budget on this input.
            if cks[-1] != "" and tk_nums[-1] + tnum > chunk_token_num:
                cks.append(t)
                tk_nums.append(tnum)
            else:
                if cks[-1]:
                    cks[-1] += "\n" + t
                else:
                    cks[-1] += t
                tk_nums[-1] += tnum
            # If the segment t was itself larger than the budget, no
            # internal split can satisfy chunk_token_num on this input. The
            # emitted chunk is the best the chunker can do without a
            # splitting heuristic (out of scope for this fix).
            if tnum > chunk_token_num:
                logging.warning(
                    "RAGFlowTxtParser.parser_txt: emitted a single chunk of %d tokens exceeding chunk_token_num=%d; the segment had no internal delimiter.",
                    tnum,
                    chunk_token_num,
                )

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
            add_chunk(sec)

        return [[c, ""] for c in cks]
