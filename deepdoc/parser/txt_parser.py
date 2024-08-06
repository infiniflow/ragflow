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

from rag.nlp import find_codec,num_tokens_from_string

class RAGFlowTxtParser:
    def __call__(self, fnm, binary=None, chunk_token_num=128):
        txt = ""
        if binary:
            encoding = find_codec(binary)
            txt = binary.decode(encoding, errors="ignore")
        else:
            with open(fnm, "r") as f:
                while True:
                    l = f.readline()
                    if not l:
                        break
                    txt += l
        return self.parser_txt(txt, chunk_token_num)

    @classmethod
    def parser_txt(cls, txt, chunk_token_num=128):
        if type(txt) != str:
            raise TypeError("txt type should be str!")
        sections = []
        for sec in txt.split("\n"):
            if num_tokens_from_string(sec) > 10 * int(chunk_token_num):
                sections.append((sec[: int(len(sec) / 2)], ""))
                sections.append((sec[int(len(sec) / 2) :], ""))
            else:
                sections.append((sec, ""))
        return sections