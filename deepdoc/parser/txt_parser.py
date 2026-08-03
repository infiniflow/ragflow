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

from common.token_utils import num_tokens_from_string
from deepdoc.parser.utils import get_text
from rag.nlp.delim import (
    compile_delimiter_pattern,
    normalize_text_newlines,
    parse_delimiter_field,
)


class RAGFlowTxtParser:
    def __call__(self, fnm, binary=None, chunk_token_num=128, delimiter="\n!?;。；！？", keep_delimiters=False):
        txt = get_text(fnm, binary)
        return self.parser_txt(txt, chunk_token_num, delimiter, keep_delimiters)

    @classmethod
    def parser_txt(cls, txt, chunk_token_num=128, delimiter="\n!?;。；！？", keep_delimiters=False):
        if not isinstance(txt, str):
            raise TypeError("txt type should be str!")
        cks = [""]
        tk_nums = [0]

        def add_chunk(t):
            nonlocal cks, tk_nums
            tnum = num_tokens_from_string(t)
            if tk_nums[-1] > chunk_token_num:
                cks.append(t)
                tk_nums.append(tnum)
            else:
                if cks[-1]:
                    cks[-1] += "\n" + t
                else:
                    cks[-1] += t
                tk_nums[-1] += tnum

        txt = normalize_text_newlines(txt)
        parsed_dels = parse_delimiter_field(delimiter)
        dels = compile_delimiter_pattern(parsed_dels)
        logging.debug(
            "RAGFlowTxtParser.parser_txt: delimiter_count=%d, splitting=%s",
            len(parsed_dels),
            bool(dels),
        )
        secs = re.split(r"(%s)" % dels, txt) if dels else [txt]
        for index, sec in enumerate(secs):
            if dels and re.match(f"^{dels}$", sec):
                continue
            if not sec:
                continue
            if keep_delimiters and index + 1 < len(secs) and re.match(f"^{dels}$", secs[index + 1]):
                sec += secs[index + 1]
            add_chunk(sec)

        logging.debug("parser_txt: %d sections -> %d chunks (chunk_token_num=%d)", len(secs), len(cks), chunk_token_num)
        return [[c, ""] for c in cks]
