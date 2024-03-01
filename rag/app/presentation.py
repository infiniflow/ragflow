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
import copy
import re
from io import BytesIO
from rag.nlp import tokenize, is_english
from rag.nlp import huqie
from deepdoc.parser import PdfParser, PptParser


class Ppt(PptParser):
    def __call__(self, fnm, from_page, to_page, callback=None):
        txts = super().__call__(fnm, from_page, to_page)

        callback(0.5, "Text extraction finished.")
        import aspose.slides as slides
        import aspose.pydrawing as drawing
        imgs = []
        with slides.Presentation(BytesIO(fnm)) as presentation:
            for i, slide in enumerate(presentation.slides[from_page: to_page]):
                buffered = BytesIO()
                slide.get_thumbnail(0.5, 0.5).save(buffered, drawing.imaging.ImageFormat.jpeg)
                imgs.append(buffered.getvalue())
        assert len(imgs) == len(txts), "Slides text and image do not match: {} vs. {}".format(len(imgs), len(txts))
        callback(0.9, "Image extraction finished")
        self.is_english = is_english(txts)
        return [(txts[i], imgs[i]) for i in range(len(txts))]


class Pdf(PdfParser):
    def __init__(self):
        super().__init__()

    def __garbage(self, txt):
        txt = txt.lower().strip()
        if re.match(r"[0-9\.,%/-]+$", txt): return True
        if len(txt) < 3:return True
        return False

    def __call__(self, filename, binary=None, from_page=0, to_page=100000, zoomin=3, callback=None):
        self.__images__(filename if not binary else binary, zoomin, from_page, to_page)
        callback(0.8, "Page {}~{}: OCR finished".format(from_page, min(to_page, self.total_page)))
        assert len(self.boxes) == len(self.page_images), "{} vs. {}".format(len(self.boxes), len(self.page_images))
        res = []
        #################### More precisely ###################
        # self._layouts_rec(zoomin)
        # self._text_merge()
        # pages = {}
        # for b in self.boxes:
        #     if self.__garbage(b["text"]):continue
        #     if b["page_number"] not in pages: pages[b["page_number"]] = []
        #     pages[b["page_number"]].append(b["text"])
        # for i, lines in pages.items():
        #     res.append(("\n".join(lines), self.page_images[i-1]))
        # return res
        ########################################

        for i in range(len(self.boxes)):
            lines = "\n".join([b["text"] for b in self.boxes[i] if not self.__garbage(b["text"])])
            res.append((lines, self.page_images[i]))
        callback(0.9, "Page {}~{}: Parsing finished".format(from_page, min(to_page, self.total_page)))
        return res


def chunk(filename, binary=None, from_page=0, to_page=100000, callback=None, **kwargs):
    """
    The supported file formats are pdf, pptx.
    Every page will be treated as a chunk. And the thumbnail of every page will be stored.
    PPT file will be parsed by using this method automatically, setting-up for every PPT file is not necessary.
    """
    doc = {
        "docnm_kwd": filename,
        "title_tks": huqie.qie(re.sub(r"\.[a-zA-Z]+$", "", filename))
    }
    doc["title_sm_tks"] = huqie.qieqie(doc["title_tks"])
    res = []
    if re.search(r"\.pptx?$", filename, re.IGNORECASE):
        ppt_parser = Ppt()
        for txt,img in ppt_parser(filename if not binary else binary, from_page, 1000000, callback):
            d = copy.deepcopy(doc)
            d["image"] = img
            tokenize(d, txt, ppt_parser.is_english)
            res.append(d)
        return res
    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        pdf_parser = Pdf()
        for txt,img in pdf_parser(filename if not binary else binary, from_page=from_page, to_page=to_page, callback=callback):
            d = copy.deepcopy(doc)
            d["image"] = img
            tokenize(d, txt, pdf_parser.is_english)
            res.append(d)
        return res

    raise NotImplementedError("file type not supported yet(pptx, pdf supported)")


if __name__== "__main__":
    import sys
    def dummy(a, b):
        pass
    chunk(sys.argv[1], callback=dummy)

