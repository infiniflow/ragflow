import requests
import logging
import threading
import sys
from pypdf import PdfReader as pdf2_read
from io import BytesIO
from timeit import default_timer as timer

import pdfplumber

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()

class RemoteMinerUParser:
    def __init__(self):
        self.endpoint = "http://localhost:8000/api/v1/document/parse"

    def __call__(self, filename=None, binary=None, from_page=0, to_page=100000, callback=None):
        try:
            response = requests.post(
                self.endpoint,
                files={"file": (filename, binary)},
            )
            data = response.json()

        except requests.exceptions.RequestException as e:
            raise Exception(f"Request failed: {str(e)}") from e

        self.__images__(filename, 3)

        sections = []
        tables = []

        for block in data:
            block_type = block.get("type")
            block_bbox = block.get("bbox")
            page_num = block.get("pageNum")
            if not block_bbox:
                continue

            # 将 bbox 坐标转换为相对于页面的比例
            x0, y0, x1, y1 = block_bbox
            pos_str = "@@{pn}\t{x0:.1f}\t{x1:.1f}\t{y0:.1f}\t{y1:.1f}##".format(
                pn=page_num, x0=x0, x1=x1, y0=y0, y1=y1
            )
            content = block.get("content")
            if block_type == "table":
                children = block.get("children")
                if children:
                    for child in children:
                        child_bbox = child.get("bbox")
                        x0, y0, x1, y1 = child_bbox
                        tables.append(((None, child.get("content")), [(page_num, x0, x1, y0, y1)]))
                elif content:
                    tables.append(((None, content), [(page_num, x0, x1, y0, y1)]))
            else:
                if content:
                    sections.append((content, pos_str))

        return sections, tables

    def __images__(self, fnm, zoomin=3, page_from=0,
                   page_to=299, callback=None):
        self.lefted_chars = []
        self.mean_height = []
        self.mean_width = []
        self.boxes = []
        self.garbages = {}
        self.page_cum_height = [0]
        self.page_layout = []
        self.page_from = page_from
        start = timer()
        try:
            with sys.modules[LOCK_KEY_pdfplumber]:
                with (pdfplumber.open(fnm) if isinstance(fnm, str) else pdfplumber.open(BytesIO(fnm))) as pdf:
                    self.pdf = pdf
                    self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).annotated for i, p in
                                        enumerate(self.pdf.pages[page_from:page_to])]

                    try:
                        self.page_chars = [[c for c in page.dedupe_chars().chars if self._has_color(c)] for page in self.pdf.pages[page_from:page_to]]
                    except Exception as e:
                        logging.warning(f"Failed to extract characters for pages {page_from}-{page_to}: {str(e)}")
                        self.page_chars = [[] for _ in range(page_to - page_from)]  # If failed to extract, using empty list instead.

                    self.total_page = len(self.pdf.pages)

        except Exception:
            logging.exception("RAGFlowPdfParser __images__")
        logging.info(f"__images__ dedupe_chars cost {timer() - start}s")

        self.outlines = []
        try:
            with (pdf2_read(fnm if isinstance(fnm, str)
                            else BytesIO(fnm))) as pdf:
                self.pdf = pdf

                outlines = self.pdf.outline
                def dfs(arr, depth):
                    for a in arr:
                        if isinstance(a, dict):
                            self.outlines.append((a["/Title"], depth))
                            continue
                        dfs(a, depth + 1)

                dfs(outlines, 0)

        except Exception as e:
            logging.warning(f"Outlines exception: {e}")

        if not self.outlines:
            logging.warning("Miss outlines")

    def crop(self, text_with_tag, need_position=True):
        import re
        # 提取 @@pn	x0	x1	y0	y1## 标签
        matches = re.findall(r"@@([\d\.]+)\t([\d\.]+)\t([\d\.]+)\t([\d\.]+)\t([\d\.]+)##", text_with_tag)

        if not matches and need_position:
            return None, []

        # 返回空图像和位置信息
        from PIL import Image
        dummy_image = Image.new("RGB", (1, 1), color="white")

        positions = []
        for pn, x0, x1, y0, y1 in matches:
            positions.append((
                int(pn),
                float(x0),
                float(x1),
                float(y0),
                float(y1)
            ))

        if need_position:
            return dummy_image, positions
        return dummy_image

    def remove_tag(self, txt):
        import re
        return re.sub(r"@@[\d\t\.]+##", "", txt)