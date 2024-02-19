import copy
import re
from rag.app import laws
from rag.parser import is_english, tokenize, naive_merge
from rag.nlp import huqie
from rag.parser.pdf_parser import HuParser
from rag.settings import cron_logger


class Pdf(HuParser):
    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        self.__images__(
            filename if not binary else binary,
            zoomin,
            from_page,
            to_page)
        callback(0.1, "OCR finished")

        from timeit import default_timer as timer
        start = timer()
        self._layouts_paddle(zoomin)
        callback(0.77, "Layout analysis finished")
        cron_logger.info("paddle layouts:".format((timer() - start) / (self.total_page + 0.1)))
        self._naive_vertical_merge()
        return [(b["text"], self._line_tag(b, zoomin)) for b in self.boxes]


def chunk(filename, binary=None, from_page=0, to_page=100000, callback=None, **kwargs):
    """
        Supported file formats are docx, pdf, txt.
        This method apply the naive ways to chunk files.
        Successive text will be sliced into pieces using 'delimiter'.
        Next, these successive pieces are merge into chunks whose token number is no more than 'Max token number'.
    """
    doc = {
        "docnm_kwd": filename,
        "title_tks": huqie.qie(re.sub(r"\.[a-zA-Z]+$", "", filename))
    }
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    pdf_parser = None
    sections = []
    if re.search(r"\.docx?$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        for txt in laws.Docx()(filename, binary):
            sections.append((txt, ""))
        callback(0.8, "Finish parsing.")
    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        pdf_parser = Pdf()
        sections = pdf_parser(filename if not binary else binary,
                              from_page=from_page, to_page=to_page, callback=callback)
    elif re.search(r"\.txt$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        txt = ""
        if binary:
            txt = binary.decode("utf-8")
        else:
            with open(filename, "r") as f:
                while True:
                    l = f.readline()
                    if not l: break
                    txt += l
        sections = txt.split("\n")
        sections = [(l, "") for l in sections if l]
        callback(0.8, "Finish parsing.")
    else:
        raise NotImplementedError("file type not supported yet(docx, pdf, txt supported)")

    parser_config = kwargs.get("parser_config", {"chunk_token_num": 128, "delimiter": "\n!?。；！？"})
    cks = naive_merge(sections, parser_config["chunk_token_num"], parser_config["delimiter"])
    eng = is_english(cks)
    res = []
    # wrap up to es documents
    for ck in cks:
        print("--", ck)
        d = copy.deepcopy(doc)
        if pdf_parser:
            d["image"] = pdf_parser.crop(ck)
            ck = pdf_parser.remove_tag(ck)
        tokenize(d, ck, eng)
        res.append(d)
    return res


if __name__ == "__main__":
    import sys


    def dummy(a, b):
        pass


    chunk(sys.argv[1], from_page=0, to_page=10, callback=dummy)
