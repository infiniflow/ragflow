"""Patch pdf_parser.py to add SKIP_OCR env var support.

Usage:
    uv run python3 internal/parser/tools/patch_skip_ocr.py
    SKIP_OCR=1 PYTHONPATH=. uv run python3 internal/parser/tools/dump_aligned_results.py --real
"""
import os, re

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
TARGET = os.path.join(ROOT, 'deepdoc', 'parser', 'pdf_parser.py')

with open(TARGET) as f:
    content = f.read()

# 1. __init__: wrap OCR() with SKIP_OCR check
content = content.replace(
    '        self.ocr = OCR()\n',
    '        self.ocr = OCR() if not os.getenv("SKIP_OCR") else None\n',
)

# 2. __init__: wrap layouter and tbl_det with SKIP_OCR check
# Pattern: lines inside if/else for ascend/onnx layout recognizer
content = content.replace(
    '            self.layouter = AscendLayoutRecognizer(recognizer_domain)\n',
    '            self.layouter = AscendLayoutRecognizer(recognizer_domain) if not os.getenv("SKIP_OCR") else None\n',
)
content = content.replace(
    '            self.layouter = LayoutRecognizer(recognizer_domain)\n',
    '            self.layouter = LayoutRecognizer(recognizer_domain) if not os.getenv("SKIP_OCR") else None\n',
)
content = content.replace(
    '        self.tbl_det = TableStructureRecognizer()\n',
    '        self.tbl_det = TableStructureRecognizer() if not os.getenv("SKIP_OCR") else None\n',
)

# 3. __ocr: add char-based fallback when SKIP_OCR is set
ocr_stub = '''    def __ocr(self, pagenum, img, chars, ZM=3, device_id: int | None = None):
        if os.getenv("SKIP_OCR"):
            # Char-based line grouping (matching Go groupCharsToLines)
            if not chars:
                self.boxes.append([])
                return
            chars_sorted = sorted(chars, key=lambda c: (c["top"], c["x0"]))
            lines, cur = [], []
            for c in chars_sorted:
                if not cur:
                    cur.append(c); continue
                mh = max(c["bottom"] - c["top"], cur[-1]["bottom"] - cur[-1]["top"])
                if abs(c["top"] - cur[-1]["top"]) < mh * 0.5:
                    cur.append(c)
                else:
                    lines.append(cur); cur = [c]
            if cur: lines.append(cur)
            bxs = []
            for line in lines:
                bxs.append({
                    "x0": min(c["x0"] for c in line), "x1": max(c["x1"] for c in line),
                    "top": min(c["top"] for c in line), "bottom": max(c["bottom"] for c in line),
                    "text": "".join(c["text"] for c in line), "page_number": pagenum,
                })
            self.boxes.append(bxs)
            return
'''

old_ocr = re.search(r'    def __ocr\(self.*?\n        start = timer\(\)', content, re.DOTALL)
if old_ocr:
    content = content.replace(old_ocr.group(), ocr_stub + '        start = timer()')

# 4. _layouts_rec: skip when SKIP_OCR is set
content = content.replace(
    '    def _layouts_rec(self, ZM, drop=True):\n        assert len(self.page_images) == len(self.boxes)\n        self.boxes, self.page_layout = self.layouter(self.page_images, self.boxes, ZM, drop=drop)',
    '    def _layouts_rec(self, ZM, drop=True):\n        if os.getenv("SKIP_OCR"):\n            self.boxes = [b for bxs in self.boxes for b in bxs]\n            self.page_layout = [[] for _ in self.boxes]\n            return\n        assert len(self.page_images) == len(self.boxes)\n        self.boxes, self.page_layout = self.layouter(self.page_images, self.boxes, ZM, drop=drop)',
)

# 5. _table_transformer_job: skip when SKIP_OCR is set
content = content.replace(
    '    def _table_transformer_job(self, ZM, auto_rotate=True):',
    '    def _table_transformer_job(self, ZM, auto_rotate=True):\n        if os.getenv("SKIP_OCR"):\n            self.tb_cpns = []\n            self.table_rotations = {}\n            return',
)

with open(TARGET, 'w') as f:
    f.write(content)

print(f'Patched: {TARGET}')
print('Usage: SKIP_OCR=1 PYTHONPATH=. uv run python3 ...')
