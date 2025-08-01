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

import copy
import re
import os
import tempfile
from io import BytesIO

from PIL import Image

from rag.nlp import tokenize
from rag.nlp import rag_tokenizer
from deepdoc.parser import PdfParser, PptParser, PlainParser
from PyPDF2 import PdfReader as pdf2_read
from rag.utils.mineru_parse import MinerUPdf


class Ppt(PptParser):
    def __call__(self, fnm, from_page, to_page, callback=None):
        txts = super().__call__(fnm, from_page, to_page, callback)

        if callback:
            callback(0.5, "Text extraction finished.")

        try:
            import subprocess

            with tempfile.TemporaryDirectory() as tmp_dir:
                # 写入临时文件
                ppt_stream = self._sanitize_input(fnm)
                ppt_path = os.path.join(tmp_dir, "temp.pptx")
                with open(ppt_path, 'wb') as f:
                    f.write(ppt_stream.getvalue())

                # 调用LibreOffice转换
                output_dir = os.path.join(tmp_dir, "images")
                os.makedirs(output_dir, exist_ok=True)

                cmd = [
                    'libreoffice', '--headless', '--convert-to', 'jpg',
                    '--outdir', output_dir, ppt_path
                ]
                subprocess.run(cmd, check=True, capture_output=True)

                # 加载图片并处理异常
                imgs = []
                img_files = sorted(
                    [f for f in os.listdir(output_dir) if f.endswith('.jpg')],
                    key=lambda x: int(x.split('_')[-1].split('.')[0])
                )

                for i in range(len(txts)):
                    try:
                        if i < len(img_files):
                            img_path = os.path.join(output_dir, img_files[i])
                            img = Image.open(img_path)
                            imgs.append(img if img else Image.new('RGB', (800, 600), 'white'))
                        else:
                            imgs.append(Image.new('RGB', (800, 600), 'white'))
                    except Exception:
                        imgs.append(Image.new('RGB', (800, 600), 'white'))

                if callback:
                    callback(0.9, "Image extraction finished")

                return list(zip(txts, imgs))

        except Exception as e:
            return [(txt, Image.new('RGB', (800, 600), 'white')) for txt in txts]


class Pdf(PdfParser):
    def __init__(self):
        super().__init__()

    def __garbage(self, txt):
        txt = txt.lower().strip()
        if re.match(r"[0-9\.,%/-]+$", txt):
            return True
        if len(txt) < 3:
            return True
        return False

    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, zoomin=3, callback=None):
        from timeit import default_timer as timer
        start = timer()
        callback(msg="OCR started")
        self.__images__(filename if not binary else binary,
                        zoomin, from_page, to_page, callback)
        callback(msg="Page {}~{}: OCR finished ({:.2f}s)".format(from_page, min(to_page, self.total_page), timer() - start))
        assert len(self.boxes) == len(self.page_images), "{} vs. {}".format(
            len(self.boxes), len(self.page_images))
        res = []
        for i in range(len(self.boxes)):
            lines = "\n".join([b["text"] for b in self.boxes[i]
                              if not self.__garbage(b["text"])])
            res.append((lines, self.page_images[i]))
        callback(0.9, "Page {}~{}: Parsing finished".format(
            from_page, min(to_page, self.total_page)))
        return res


class PlainPdf(PlainParser):
    def __call__(self, filename, binary=None, from_page=0,
                 to_page=100000, callback=None, **kwargs):
        self.pdf = pdf2_read(filename if not binary else BytesIO(binary))
        page_txt = []
        for page in self.pdf.pages[from_page: to_page]:
            page_txt.append(page.extract_text())
        callback(0.9, "Parsing finished")
        return [(txt, None) for txt in page_txt]


def chunk(filename, binary=None, from_page=0, to_page=100000,
          lang="Chinese", callback=None, **kwargs):
    """
    The supported file formats are pdf, pptx.
    Every page will be treated as a chunk. And the thumbnail of every page will be stored.
    PPT file will be parsed by using this method automatically, setting-up for every PPT file is not necessary.
    """
    eng = lang.lower() == "english"
    doc = {
        "docnm_kwd": filename,
        "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename))
    }
    doc["title_sm_tks"] = rag_tokenizer.fine_grained_tokenize(doc["title_tks"])
    res = []
    if re.search(r"\.pptx?$", filename, re.IGNORECASE):
        ppt_parser = Ppt()
        for pn, (txt, img) in enumerate(ppt_parser(
                filename if not binary else binary, from_page, 1000000, callback)):
            d = copy.deepcopy(doc)
            pn += from_page
            d["image"] = img
            d["page_num_int"] = [pn + 1]
            d["top_int"] = [0]
            d["position_int"] = [(pn + 1, 0, img.size[0], 0, img.size[1])]
            tokenize(d, txt, eng)
            res.append(d)
        return res
    elif re.search(r"\.pdf$", filename, re.IGNORECASE):
        if kwargs.get("parser_config", {}).get("layout_recognize", "DeepDOC") == "Plain Text":
            pdf_parser = PlainParser()
        elif kwargs.get("parser_config", {}).get("layout_recognize", "DeepDOC") == "MinerU":
            pdf_parser = MinerUPdf()
        else:
            pdf_parser = Pdf()
        for pn, (txt, img) in enumerate(pdf_parser(filename, binary,
                                                   from_page=from_page, to_page=to_page, callback=callback)):
            d = copy.deepcopy(doc)
            pn += from_page
            if img:
                d["image"] = img
            d["page_num_int"] = [pn + 1]
            d["top_int"] = [0]
            d["position_int"] = [(pn + 1, 0, img.size[0] if img else 0, 0, img.size[1] if img else 0)]
            tokenize(d, txt, eng)
            res.append(d)
        return res

    raise NotImplementedError(
        "file type not supported yet(pptx, pdf supported)")


if __name__ == "__main__":
    import sys

    def dummy(a, b):
        pass
    chunk(sys.argv[1], callback=dummy)