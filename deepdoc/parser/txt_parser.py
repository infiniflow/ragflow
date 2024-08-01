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