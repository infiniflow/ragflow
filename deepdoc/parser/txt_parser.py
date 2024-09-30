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
import itertools

from deepdoc.parser.utils import get_text
from rag.nlp import num_tokens_from_string


class RAGFlowTxtParser:
    def __call__(self, fnm, binary=None, chunk_token_num=128, delimiter="\n!?;。；！？"):
        txt = get_text(fnm, binary)
        return self.parser_txt(txt, chunk_token_num, delimiter)

    @classmethod
    def parser_txt(cls, txt, chunk_token_num=128, delimiter="\n!?;。；！？"):
        if not isinstance(txt, str):
            raise TypeError("txt type should be str!")

        groups = itertools.groupby(txt, key=lambda x: x not in delimiter)
        strs = ["".join(v) for k, v in groups if k]
        cks, ck, num = [], "", 0
        for i, s in enumerate(strs):
            ck += s
            num += num_tokens_from_string(s)
            if num > chunk_token_num or i == len(strs) - 1:
                cks.append(ck)
                ck, num = "", 0

        return [[c, ""] for c in cks]
