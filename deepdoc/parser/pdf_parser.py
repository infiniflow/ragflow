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

import asyncio
import logging
import math
import os
import random
import re
import sys
import threading
import unicodedata
from collections import Counter, defaultdict
from copy import deepcopy
from io import BytesIO
from timeit import default_timer as timer

import numpy as np
import pdfplumber
import xgboost as xgb
from huggingface_hub import snapshot_download
from PIL import Image
from pypdf import PdfReader as pdf2_read
from sklearn.cluster import KMeans
from sklearn.metrics import silhouette_score

from common.constants import MAXIMUM_PAGE_NUMBER
from common.file_utils import get_project_base_directory
from deepdoc.vision import OCR, AscendLayoutRecognizer, LayoutRecognizer, Recognizer, TableStructureRecognizer
from rag.nlp import rag_tokenizer
from rag.prompts.generator import vision_llm_describe_prompt
from deepdoc.parser.utils import extract_pdf_outlines
from common import settings


from common.misc_utils import thread_pool_exec

LOCK_KEY_pdfplumber = "global_shared_lock_pdfplumber"
if LOCK_KEY_pdfplumber not in sys.modules:
    sys.modules[LOCK_KEY_pdfplumber] = threading.Lock()


class RAGFlowPdfParser:
    def __init__(self, **kwargs):
        """
        If you have trouble downloading HuggingFace models, -_^ this might help!!

        For Linux:
        export HF_ENDPOINT=https://hf-mirror.com

        For Windows:
        Good luck
        ^_-

        """

        self.ocr = OCR()
        self.parallel_limiter = None
        if settings.PARALLEL_DEVICES > 1:
            self.parallel_limiter = [asyncio.Semaphore(1) for _ in range(settings.PARALLEL_DEVICES)]

        layout_recognizer_type = os.getenv("LAYOUT_RECOGNIZER_TYPE", "onnx").lower()
        if layout_recognizer_type not in ["onnx", "ascend"]:
            raise RuntimeError("Unsupported layout recognizer type.")

        if hasattr(self, "model_species"):
            recognizer_domain = "layout." + self.model_species
        else:
            recognizer_domain = "layout"

        if layout_recognizer_type == "ascend":
            logging.debug("Using Ascend LayoutRecognizer")
            self.layouter = AscendLayoutRecognizer(recognizer_domain)
        else:  # onnx
            logging.debug("Using Onnx LayoutRecognizer")
            self.layouter = LayoutRecognizer(recognizer_domain)
        self.tbl_det = TableStructureRecognizer()

        self.updown_cnt_mdl = xgb.Booster()
        # xgboost model is very small; using CPU explicitly
        self.updown_cnt_mdl.set_param({"device": "cpu"})
        logging.info("updown_cnt_mdl initialized on CPU")
        try:
            model_dir = os.path.join(get_project_base_directory(), "rag/res/deepdoc")
            self.updown_cnt_mdl.load_model(os.path.join(model_dir, "updown_concat_xgb.model"))
        except Exception:
            model_dir = snapshot_download(repo_id="InfiniFlow/text_concat_xgb_v1.0", local_dir=os.path.join(get_project_base_directory(), "rag/res/deepdoc"))
            self.updown_cnt_mdl.load_model(os.path.join(model_dir, "updown_concat_xgb.model"))

        self.page_from = 0
        self.column_num = 1

    def __char_width(self, c):
        return (c["x1"] - c["x0"]) // max(len(c["text"]), 1)

    def __height(self, c):
        return c["bottom"] - c["top"]

    def _x_dis(self, a, b):
        return min(abs(a["x1"] - b["x0"]), abs(a["x0"] - b["x1"]), abs(a["x0"] + a["x1"] - b["x0"] - b["x1"]) / 2)

    def _y_dis(self, a, b):
        return (b["top"] + b["bottom"] - a["top"] - a["bottom"]) / 2

    def _match_proj(self, b):
        proj_patt = [
            r"第[零一二三四五六七八九十百]+章",
            r"第[零一二三四五六七八九十百]+[条节]",
            r"[零一二三四五六七八九十百]+[、是 　]",
            r"[\(（][零一二三四五六七八九十百]+[）\)]",
            r"[\(（][0-9]+[）\)]",
            r"[0-9]+(、|\.[　 ]|）|\.[^0-9./a-zA-Z_%><-]{4,})",
            r"[0-9]+\.[0-9.]+(、|\.[ 　])",
            r"[⚫•➢①② ]",
        ]
        return any([re.match(p, b["text"]) for p in proj_patt])

    def _updown_concat_features(self, up, down):
        w = max(self.__char_width(up), self.__char_width(down))
        h = max(self.__height(up), self.__height(down))
        y_dis = self._y_dis(up, down)
        LEN = 6
        tks_down = rag_tokenizer.tokenize(down["text"][:LEN]).split()
        tks_up = rag_tokenizer.tokenize(up["text"][-LEN:]).split()
        tks_all = up["text"][-LEN:].strip() + (" " if re.match(r"[a-zA-Z0-9]+", up["text"][-1] + down["text"][0]) else "") + down["text"][:LEN].strip()
        tks_all = rag_tokenizer.tokenize(tks_all).split()
        fea = [
            up.get("R", -1) == down.get("R", -1),
            y_dis / h,
            down["page_number"] - up["page_number"],
            up["layout_type"] == down["layout_type"],
            up["layout_type"] == "text",
            down["layout_type"] == "text",
            up["layout_type"] == "table",
            down["layout_type"] == "table",
            True if re.search(r"([。？！；!?;+)）]|[a-z]\.)$", up["text"]) else False,
            True if re.search(r"[，：‘“、0-9（+-]$", up["text"]) else False,
            True if re.search(r"(^.?[/,?;:\]，。；：’”？！》】）-])", down["text"]) else False,
            True if re.match(r"[\(（][^\(\)（）]+[）\)]$", up["text"]) else False,
            True if re.search(r"[，,][^。.]+$", up["text"]) else False,
            True if re.search(r"[，,][^。.]+$", up["text"]) else False,
            True if re.search(r"[\(（][^\)）]+$", up["text"]) and re.search(r"[\)）]", down["text"]) else False,
            self._match_proj(down),
            True if re.match(r"[A-Z]", down["text"]) else False,
            True if re.match(r"[A-Z]", up["text"][-1]) else False,
            True if re.match(r"[a-z0-9]", up["text"][-1]) else False,
            True if re.match(r"[0-9.%,-]+$", down["text"]) else False,
            up["text"].strip()[-2:] == down["text"].strip()[-2:] if len(up["text"].strip()) > 1 and len(down["text"].strip()) > 1 else False,
            up["x0"] > down["x1"],
            abs(self.__height(up) - self.__height(down)) / min(self.__height(up), self.__height(down)),
            self._x_dis(up, down) / max(w, 0.000001),
            (len(up["text"]) - len(down["text"])) / max(len(up["text"]), len(down["text"])),
            len(tks_all) - len(tks_up) - len(tks_down),
            len(tks_down) - len(tks_up),
            tks_down[-1] == tks_up[-1] if tks_down and tks_up else False,
            max(down["in_row"], up["in_row"]),
            abs(down["in_row"] - up["in_row"]),
            len(tks_down) == 1 and rag_tokenizer.tag(tks_down[0]).find("n") >= 0,
            len(tks_up) == 1 and rag_tokenizer.tag(tks_up[0]).find("n") >= 0,
        ]
        return fea

    @staticmethod
    def sort_X_by_page(arr, threshold):
        # sort using y1 first and then x1
        arr = sorted(arr, key=lambda r: (r["page_number"], r["x0"], r["top"]))
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                # restore the order using th
                if abs(arr[j + 1]["x0"] - arr[j]["x0"]) < threshold and arr[j + 1]["top"] < arr[j]["top"] and arr[j + 1]["page_number"] == arr[j]["page_number"]:
                    tmp = arr[j]
                    arr[j] = arr[j + 1]
                    arr[j + 1] = tmp
        return arr

    def _has_color(self, o):
        if o.get("ncs", "") == "DeviceGray":
            if o["stroking_color"] and o["stroking_color"][0] == 1 and o["non_stroking_color"] and o["non_stroking_color"][0] == 1:
                if re.match(r"[a-zT_\[\]\(\)-]+", o.get("text", "")):
                    return False
        return True

    # CID pattern regex for unmapped font characters from pdfminer
    _CID_PATTERN = re.compile(r"\(cid\s*:\s*\d+\s*\)")

    _OCR_ALPHABET = None

    @classmethod
    def _ocr_can_represent(cls, text, min_coverage=0.8):
        """True if the OCR recogniser's alphabet covers this text well enough to be worth OCRing."""
        if not text:
            return True
        if cls._OCR_ALPHABET is None:
            res = os.path.join(get_project_base_directory(), "rag/res/deepdoc/ocr.res")
            try:
                with open(res, encoding="utf-8") as f:
                    cls._OCR_ALPHABET = set(f.read())
            except (OSError, UnicodeDecodeError) as e:
                logging.warning("Could not load OCR alphabet from %s: %s; treating all text as representable.", res, e)
                cls._OCR_ALPHABET = set()
        if not cls._OCR_ALPHABET:
            return True  # unknown alphabet: preserve existing behaviour
        letters = [c for c in text if c.strip()]
        if not letters:
            return True
        covered = sum(1 for c in letters if c in cls._OCR_ALPHABET)
        return covered / len(letters) >= min_coverage

    # CJK scripts (Han, Hiragana, Katakana, Hangul) do not separate words with
    # spaces, so a geometric gap between their glyphs must not become one.
    _CJK_PATTERN = re.compile(r"[ᄀ-ᇿ぀-ヿ㄰-㆏㐀-䶿一-鿿가-힯豈-﫿]|[\U00020000-\U0002fa1f]")

    @classmethod
    def _insert_word_spaces(cls, chars, gap_ratio=0.25):
        """Recover missing spaces from character geometry.

        Many PDFs encode no space glyphs and separate words by positioning alone.
        Append a space to a char when the gap to the next exceeds ``gap_ratio`` of
        the mean char width; intra-word kerns fall well below that. CJK is skipped:
        it does not write inter-word spaces, so a gap between CJK glyphs is ordinary
        tracking, not a boundary. ``chars`` is a list of pdfplumber-style dicts and
        is mutated in place.
        """
        widths = [c["width"] for c in chars if c["text"] and c["text"].strip()]
        mean_w = sum(widths) / len(widths) if widths else 0
        if mean_w <= 0:
            return
        for cur, nxt in zip(chars, chars[1:]):
            if (
                cur["text"]
                and nxt["text"]
                and cur["text"].strip()
                and nxt["text"].strip()
                and not cls._CJK_PATTERN.search(cur["text"])
                and not cls._CJK_PATTERN.search(nxt["text"])
                and nxt["x0"] - cur["x1"] > mean_w * gap_ratio
            ):
                cur["text"] += " "

    @staticmethod
    def _is_garbled_char(ch):
        """Check if a single character is garbled (unmappable from PDF font encoding).

        A character is considered garbled if it falls into Unicode Private Use Areas
        or certain replacement/control character ranges that typically indicate
        pdfminer failed to map a CID to a valid Unicode codepoint.
        """
        if not ch:
            return False
        cp = ord(ch)
        if 0xE000 <= cp <= 0xF8FF:
            return True
        if 0xF0000 <= cp <= 0xFFFFF:
            return True
        if 0x100000 <= cp <= 0x10FFFF:
            return True
        if cp == 0xFFFD:
            return True
        if cp < 0x20 and ch not in ("\t", "\n", "\r"):
            return True
        if 0x80 <= cp <= 0x9F:
            return True
        cat = unicodedata.category(ch)
        if cat in ("Cn", "Cs"):
            return True
        return False

    @staticmethod
    def _is_garbled_text(text, threshold=0.5):
        """Check if a text string contains too many garbled characters.

        Examines each character and determines if the overall proportion
        of garbled characters exceeds the given threshold. Also detects
        pdfminer's CID placeholder patterns like '(cid:123)'.
        """
        if not text or not text.strip():
            return False
        if RAGFlowPdfParser._CID_PATTERN.search(text):
            return True
        garbled_count = 0
        total = 0
        for ch in text:
            if ch.isspace():
                continue
            total += 1
            if RAGFlowPdfParser._is_garbled_char(ch):
                garbled_count += 1
        if total == 0:
            return False
        return garbled_count / total >= threshold

    @staticmethod
    def _has_subset_font_prefix(fontname):
        """Check if a font name has a subset prefix (e.g. 'DY1+ZLQDm1-1').

        PDF subset fonts use a 6-letter uppercase tag followed by '+' before
        the actual font name. Some tools use shorter tags (e.g. 'DY1+').
        """
        if not fontname:
            return False
        return bool(re.match(r"^[A-Z0-9]{2,6}\+", fontname))

    @staticmethod
    def _is_garbled_by_font_encoding(page_chars, min_chars=20):
        """Detect garbled text caused by broken font encoding mappings.

        Some PDFs (especially older Chinese standards) embed custom fonts that
        map CJK glyphs to ASCII codepoints. The extracted text appears as
        random ASCII punctuation/symbols instead of actual CJK characters.

        Detection strategy: if a significant proportion of characters come from
        subset-embedded fonts and the page produces overwhelmingly ASCII
        (punctuation, digits, symbols) with virtually no CJK/Hangul/Kana
        characters, the page is likely garbled due to broken font encoding.
        """
        if not page_chars or len(page_chars) < min_chars:
            return False

        subset_font_count = 0
        total_non_space = 0
        ascii_punct_sym = 0
        cjk_like = 0

        for c in page_chars:
            text = c.get("text", "")
            fontname = c.get("fontname", "")
            if not text or text.isspace():
                continue
            total_non_space += 1

            if RAGFlowPdfParser._has_subset_font_prefix(fontname):
                subset_font_count += 1

            cp = ord(text[0])
            if 0x2E80 <= cp <= 0x9FFF or 0xF900 <= cp <= 0xFAFF or 0x20000 <= cp <= 0x2FA1F or 0xAC00 <= cp <= 0xD7AF or 0x3040 <= cp <= 0x30FF:
                cjk_like += 1
            elif 0x21 <= cp <= 0x2F or 0x3A <= cp <= 0x40 or 0x5B <= cp <= 0x60 or 0x7B <= cp <= 0x7E:
                ascii_punct_sym += 1

        if total_non_space < min_chars:
            return False

        subset_ratio = subset_font_count / total_non_space
        if subset_ratio < 0.3:
            return False

        cjk_ratio = cjk_like / total_non_space
        punct_ratio = ascii_punct_sym / total_non_space
        if cjk_ratio < 0.05 and punct_ratio > 0.4:
            return True

        return False

    def _evaluate_table_orientation(self, table_img, sample_ratio=0.3):
        """
        Evaluate the best rotation orientation for a table image.

        Tests 4 rotation angles (0°, 90°, 180°, 270°) and uses OCR
        confidence scores to determine the best orientation.

        Args:
            table_img: PIL Image object of the table region
            sample_ratio: Sampling ratio for quick evaluation

        Returns:
            tuple: (best_angle, best_img, confidence_scores)
                - best_angle: Best rotation angle (0, 90, 180, 270)
                - best_img: Image rotated to best orientation
                - confidence_scores: Dict of scores for each angle
        """

        rotations = [
            (0, "original"),
            (90, "rotate_90"),  # clockwise 90°
            (180, "rotate_180"),  # 180°
            (270, "rotate_270"),  # clockwise 270° (counter-clockwise 90°)
        ]

        results = {}
        best_score = -1
        best_angle = 0
        best_img = table_img
        score_0 = None

        for angle, name in rotations:
            # Rotate image
            if angle == 0:
                rotated_img = table_img
            else:
                # PIL's rotate is counter-clockwise, use negative angle for clockwise
                rotated_img = table_img.rotate(-angle, expand=True)

            # Convert to numpy array for OCR
            img_array = np.array(rotated_img)

            # Perform OCR detection and recognition
            try:
                ocr_results = self.ocr(img_array)

                if ocr_results:
                    # Calculate average confidence
                    scores = [conf for _, (_, conf) in ocr_results]
                    avg_score = sum(scores) / len(scores) if scores else 0
                    total_regions = len(scores)

                    # Combined score: considers both average confidence and number of regions
                    # More regions + higher confidence = better orientation
                    combined_score = avg_score * (1 + 0.1 * min(total_regions, 50) / 50)
                else:
                    avg_score = 0
                    total_regions = 0
                    combined_score = 0

            except Exception as e:
                logging.warning(f"OCR failed for angle {angle}: {e}")
                avg_score = 0
                total_regions = 0
                combined_score = 0

            results[angle] = {"avg_confidence": avg_score, "total_regions": total_regions, "combined_score": combined_score}
            if angle == 0:
                score_0 = combined_score

            logging.debug(f"Table orientation {angle}°: avg_conf={avg_score:.4f}, regions={total_regions}, combined={combined_score:.4f}")

            if combined_score > best_score:
                best_score = combined_score
                best_angle = angle
                best_img = rotated_img

        # Absolute threshold rule:
        # Only choose non-0° if it exceeds 0° by more than 0.2 and 0° score is below 0.8.
        if best_angle != 0 and score_0 is not None:
            if not (best_score - score_0 > 0.2 and score_0 < 0.8):
                best_angle = 0
                best_img = table_img
                best_score = score_0

        results[best_angle] = results.get(best_angle, {"avg_confidence": 0, "total_regions": 0, "combined_score": 0})

        logging.info(f"Best table orientation: {best_angle}° (score={best_score:.4f})")

        return best_angle, best_img, results

    @staticmethod
    def _map_clockwise_rotated_point_to_original(x, y, angle, width, height):
        if angle == 0:
            return x, y
        if angle == 90:
            return y, height - x
        if angle == 180:
            return width - x, height - y
        if angle == 270:
            return width - y, x
        return x, y

    def _table_transformer_job(self, ZM, auto_rotate=None):
        """
        Process table structure recognition.

        When auto_rotate=True, the complete workflow:
        1. Evaluate table orientation and select the best rotation angle
        2. Use rotated image for table structure recognition (TSR)
        3. Re-OCR the rotated image
        4. Match new OCR results with TSR cell coordinates

        Args:
            ZM: Zoom factor
            auto_rotate: Whether to enable auto orientation correction.
                         None means reading TABLE_AUTO_ROTATE from the environment.
        """
        if auto_rotate is None:
            auto_rotate = os.getenv("TABLE_AUTO_ROTATE", "true").lower() in ("true", "1", "yes")

        logging.debug("Table processing...")
        imgs, pos = [], []
        tbcnt = [0]
        MARGIN = 10
        self.tb_cpns = []
        self.table_rotations = {}  # Store rotation info for each table
        self.rotated_table_imgs = {}  # Store rotated table images

        assert len(self.page_layout) == len(self.page_images)

        # Collect layout info for all tables
        table_layouts = []

        table_index = 0
        for p, tbls in enumerate(self.page_layout):  # for page
            tbls = [f for f in tbls if f["type"] == "table"]
            tbcnt.append(len(tbls))
            if not tbls:
                continue
            for page_table_index, tb in enumerate(tbls):  # for table
                left, top, right, bott = tb["x0"] - MARGIN, tb["top"] - MARGIN, tb["x1"] + MARGIN, tb["bottom"] + MARGIN
                left *= ZM
                top *= ZM
                right *= ZM
                bott *= ZM
                layoutno = f"table-{page_table_index}"
                pos.append((left, top, p, table_index, layoutno))

                # Record table layout info
                table_layouts.append({"page": p, "table_index": table_index, "layoutno": layoutno, "layout": tb, "coords": (left, top, right, bott)})

                # Crop table image
                table_img = self.page_images[p].crop((left, top, right, bott))

                if auto_rotate:
                    # Evaluate table orientation
                    logging.debug(f"Evaluating orientation for table {table_index} on page {p}")
                    best_angle, rotated_img, rotation_scores = self._evaluate_table_orientation(table_img)

                    # Store rotation info
                    self.table_rotations[table_index] = {
                        "page": p,
                        "original_pos": (left, top, right, bott),
                        "best_angle": best_angle,
                        "scores": rotation_scores,
                        "rotated_size": rotated_img.size,  # (width, height)
                    }

                    # Store the rotated image
                    self.rotated_table_imgs[table_index] = rotated_img
                    imgs.append(rotated_img)

                else:
                    imgs.append(table_img)
                    self.table_rotations[table_index] = {"page": p, "original_pos": (left, top, right, bott), "best_angle": 0, "scores": {}, "rotated_size": table_img.size}
                    self.rotated_table_imgs[table_index] = table_img

                table_index += 1

        assert len(self.page_images) == len(tbcnt) - 1
        if not imgs:
            return

        # Perform table structure recognition (TSR)
        recos = self.tbl_det(imgs)

        # If tables were rotated, re-OCR the rotated images and replace table boxes
        if auto_rotate:
            self._ocr_rotated_tables(ZM, table_layouts, recos, tbcnt)

        def _map_tsr_component_to_page_space(component, table_pos):
            crop_left, crop_top, page, table_index, _ = table_pos
            rotation_info = self.table_rotations.get(table_index, {})
            angle = rotation_info.get("best_angle", 0)
            original_pos = rotation_info.get("original_pos", (crop_left, crop_top, crop_left, crop_top))
            width = original_pos[2] - original_pos[0]
            height = original_pos[3] - original_pos[1]
            points = [
                (component["x0_rotated"], component["top_rotated"]),
                (component["x1_rotated"], component["top_rotated"]),
                (component["x0_rotated"], component["bottom_rotated"]),
                (component["x1_rotated"], component["bottom_rotated"]),
            ]
            mapped = [self._map_clockwise_rotated_point_to_original(x, y, angle, width, height) for x, y in points]
            xs = [p[0] for p in mapped]
            ys = [p[1] for p in mapped]
            component["x0"] = min(xs) / ZM + crop_left / ZM
            component["x1"] = max(xs) / ZM + crop_left / ZM
            component["top"] = min(ys) / ZM + crop_top / ZM + self.page_cum_height[page]
            component["bottom"] = max(ys) / ZM + crop_top / ZM + self.page_cum_height[page]

        # Process TSR results and align structure boxes with page-cumulative OCR boxes.
        tbcnt = np.cumsum(tbcnt)
        for i in range(len(tbcnt) - 1):  # for page
            pg = []
            for j, tb_items in enumerate(recos[tbcnt[i] : tbcnt[i + 1]]):  # for table
                poss = pos[tbcnt[i] : tbcnt[i + 1]]
                for it in tb_items:  # for table components
                    # TSR coordinates are relative to rotated image, need to record
                    it["x0_rotated"] = it["x0"]
                    it["x1_rotated"] = it["x1"]
                    it["top_rotated"] = it["top"]
                    it["bottom_rotated"] = it["bottom"]

                    it["pn"] = poss[j][2]  # page number
                    it["layoutno"] = poss[j][4]
                    it["table_index"] = poss[j][3]  # table index
                    _map_tsr_component_to_page_space(it, poss[j])
                    pg.append(it)
            self.tb_cpns.extend(pg)

        def _has_gridline(prev_comp, comp):
            try:
                page = prev_comp["pn"]
                cum = self.page_cum_height[page]
                y = int(round((prev_comp["bottom"] + comp["top"]) / 2 * ZM - cum * ZM))
                img = self.page_images[page]
                x0 = int(max(0, prev_comp["x0"] * ZM))
                x1 = int(min(img.width, prev_comp["x1"] * ZM))
                if x1 - x0 < 20 or y < 1 or y >= img.height - 1:
                    return False
                # TSR boxes snap to text, not to the rule itself, so scan a
                # small vertical window around the shared edge.
                w = max(2, int(ZM))
                strip = np.asarray(img.convert("L"), dtype=np.uint8)[max(0, y - w) : y + w + 1, x0:x1]
                dark_frac = (strip < 160).mean(axis=1)
                return bool((dark_frac > 0.6).any())
            except Exception:
                return False

        self.tb_cpns = self._merge_continuation_row_components(self.tb_cpns, self.boxes, _has_gridline)

        def gather(kwd, fzy=10, ption=0.6):
            eles = Recognizer.sort_Y_firstly([r for r in self.tb_cpns if re.match(kwd, r["label"])], fzy)
            eles = Recognizer.layouts_cleanup(self.boxes, eles, 5, ption)
            return Recognizer.sort_Y_firstly(eles, 0)

        # add R,H,C,SP tag to boxes within table layout
        headers = gather(r".*header$")
        rows = gather(r".* (row|header)")
        spans = gather(r".*spanning")
        clmns = sorted([r for r in self.tb_cpns if re.match(r"table column$", r["label"])], key=lambda x: (x["pn"], x["layoutno"], x["x0"]))
        clmns = Recognizer.layouts_cleanup(self.boxes, clmns, 5, 0.5)

        for b in self.boxes:
            if b.get("layout_type", "") != "table":
                continue
            ii = Recognizer.find_overlapped_with_threshold(b, rows, thr=0.3)
            if ii is not None:
                b["R"] = ii
                b["R_top"] = rows[ii]["top"]
                b["R_bott"] = rows[ii]["bottom"]

            ii = Recognizer.find_overlapped_with_threshold(b, headers, thr=0.3)
            if ii is not None:
                b["H_top"] = headers[ii]["top"]
                b["H_bott"] = headers[ii]["bottom"]
                b["H_left"] = headers[ii]["x0"]
                b["H_right"] = headers[ii]["x1"]
                b["H"] = ii

            ii = Recognizer.find_horizontally_tightest_fit(b, clmns)
            if ii is not None:
                b["C"] = ii
                b["C_left"] = clmns[ii]["x0"]
                b["C_right"] = clmns[ii]["x1"]

            ii = Recognizer.find_overlapped_with_threshold(b, spans, thr=0.3)
            if ii is not None:
                b["H_top"] = spans[ii]["top"]
                b["H_bott"] = spans[ii]["bottom"]
                b["H_left"] = spans[ii]["x0"]
                b["H_right"] = spans[ii]["x1"]
                b["SP"] = ii

    # A row boundary with a text gap above this multiple of the table's
    # median OCR line height is treated as a real row boundary; tighter gaps
    # are wrapped-text continuation inside one cell.
    _CONTINUATION_GAP_FACTOR = 1.5

    @staticmethod
    def _merge_continuation_row_components(tb_cpns, boxes, has_separator=None):
        """Merge phantom ``table row`` components from cross-page continuations.

        On a cross-page table continuation whose column holds a long wrapped
        paragraph, the TSR model emits one ``table row`` component per wrapped
        text line instead of one per physical row (issue #18569). Geometry
        alone cannot separate those phantom splits from real adjacent rows,
        because both abut or overlap vertically, so this pass uses the OCR
        text lines inside the table to decide each boundary:

        * a row component that covers no text merges into the next one
          (split-line and padding artifacts);
        * a visible horizontal separator (gridline) between two components
          always keeps them apart;
        * a boundary is kept when a new label-column text run starts below
          it: a real row starts with a new label, while a wrapped label
          continues at the regular line pitch;
        * otherwise the boundary is kept when every content column with text
          below it starts a fresh run there (real rows start new cells), and
          is merged when at least one content column keeps flowing at the
          regular line pitch or when only the label column wraps.

        ``has_separator`` is an optional ``(prev_comp, comp) -> bool``
        predicate reporting a visible horizontal rule between the two
        components; when it is not provided, only the text evidence above is
        used. When in doubt the boundary is kept, so ordinary tables are
        returned unchanged. Components of non-row labels and tables without
        column components are passed through as-is.
        """
        row_pat = re.compile(r".* (row|header)$")
        col_pat = re.compile(r"table column$")

        groups = {}
        for idx, c in enumerate(tb_cpns):
            groups.setdefault((c.get("pn"), c.get("layoutno")), []).append(idx)

        result = []
        for key in sorted(groups, key=lambda k: (k[0] is None, k[0], str(k[1]))):
            comps = [tb_cpns[i] for i in groups[key]]
            row_comps = sorted(
                (c for c in comps if row_pat.match(c["label"])),
                key=lambda c: (c["top"], c["x0"]),
            )
            other_comps = [c for c in comps if not row_pat.match(c["label"])]
            cols = sorted(
                (c for c in comps if col_pat.match(c["label"])),
                key=lambda c: c["x0"],
            )
            if len(row_comps) < 2 or not cols:
                result.extend(comps)
                continue

            pn = key[0]
            x0 = min(c["x0"] for c in row_comps) - 5
            x1 = max(c["x1"] for c in row_comps) + 5
            y0 = min(c["top"] for c in row_comps) - 5
            y1 = max(c["bottom"] for c in row_comps) + 5
            tbl_boxes = [
                b for b in boxes
                if b.get("page_number") == pn + 1
                and b.get("layout_type", "") == "table"
                and b.get("text", "").strip()
                and b["x1"] > x0 and b["x0"] < x1
                and b["bottom"] > y0 and b["top"] < y1
            ]
            if len(tbl_boxes) < 3:
                result.extend(comps)
                continue

            heights = sorted(b["bottom"] - b["top"] for b in tbl_boxes)
            med_h = heights[len(heights) // 2]
            gap_limit = RAGFlowPdfParser._CONTINUATION_GAP_FACTOR * med_h
            label_x1 = cols[0]["x1"]

            def col_of(b):
                cx = (b["x0"] + b["x1"]) / 2
                if cx < label_x1:
                    return 0
                for ci, c in enumerate(cols[1:], start=1):
                    if c["x0"] - 3 <= cx <= c["x1"] + 3:
                        return ci
                return None

            label_boxes = [b for b in tbl_boxes if col_of(b) == 0]
            column_boxes = {}
            for b in tbl_boxes:
                ci = col_of(b)
                if ci is not None:
                    column_boxes.setdefault(ci, []).append(b)

            merged = []
            for comp in row_comps:
                if not merged:
                    merged.append(dict(comp))
                    continue
                acc = merged[-1]
                boundary = acc["bottom"]
                a_texts = [b for b in tbl_boxes if b["top"] < boundary and b["bottom"] > acc["top"]]
                if not a_texts:
                    # Component covering no text: split-line or padding strip.
                    acc["bottom"] = max(acc["bottom"], comp["bottom"])
                    acc["score"] = max(acc.get("score", 0), comp.get("score", 0))
                    continue
                if has_separator is not None and has_separator(acc, comp):
                    merged.append(dict(comp))
                    continue

                b_texts = [b for b in tbl_boxes if boundary - 1 <= b["top"] < comp["bottom"] + 1]

                new_label = False
                for tb in b_texts:
                    if col_of(tb) != 0:
                        continue
                    prev = [ta for ta in label_boxes if tb["top"] - gap_limit <= ta["bottom"] <= tb["top"] + 1]
                    if not prev:
                        new_label = True
                        break
                if new_label:
                    merged.append(dict(comp))
                    continue

                fresh_starts = 0
                tight_continuations = 0
                for ci, cbs in column_boxes.items():
                    if ci == 0:
                        continue
                    below = [b["top"] for b in cbs if boundary - 1 <= b["top"] < comp["bottom"] + 1]
                    if not below:
                        continue
                    above = [b["bottom"] for b in cbs if boundary - 4 * gap_limit < b["bottom"] <= boundary + med_h / 2]
                    if above and min(below) - max(above) <= gap_limit:
                        tight_continuations += 1
                    else:
                        fresh_starts += 1
                # A fresh text run starting in every content column means the
                # boundary is a real row line (new cells). Wrapped paragraphs
                # keep at least one column flowing at the regular line pitch;
                # when only the label column wraps (new label runs were
                # already excluded above), the split is still a phantom one.
                continuation = tight_continuations > 0 or (fresh_starts == 0 and b_texts)
                if continuation:
                    acc["bottom"] = max(acc["bottom"], comp["bottom"])
                    acc["score"] = max(acc.get("score", 0), comp.get("score", 0))
                else:
                    merged.append(dict(comp))

            result.extend(other_comps)
            result.extend(merged)
        return result

    def _ocr_rotated_tables(self, ZM, table_layouts, tsr_results, tbcnt):
        """
        Re-OCR rotated table images and update self.boxes.

        Args:
            ZM: Zoom factor
            table_layouts: List of table layout info
            tsr_results: TSR recognition results
            tbcnt: Cumulative table count per page
        """
        tbcnt = np.cumsum(tbcnt)

        def _table_region(layout, page_index):
            table_x0 = layout["x0"]
            table_top = layout["top"]
            table_x1 = layout["x1"]
            table_bottom = layout["bottom"]
            table_top_cum = table_top + self.page_cum_height[page_index]
            table_bottom_cum = table_bottom + self.page_cum_height[page_index]
            return table_x0, table_top, table_x1, table_bottom, table_top_cum, table_bottom_cum

        def _collect_table_boxes(page_index, table_x0, table_x1, table_top_cum, table_bottom_cum):
            indices = [
                i
                for i, b in enumerate(self.boxes)
                if (
                    b.get("page_number") == page_index + 1
                    and b.get("layout_type") == "table"
                    and b["x0"] >= table_x0 - 5
                    and b["x1"] <= table_x1 + 5
                    and b["top"] >= table_top_cum - 5
                    and b["bottom"] <= table_bottom_cum + 5
                )
            ]
            original_boxes = [self.boxes[i] for i in indices]
            insert_at = indices[0] if indices else len(self.boxes)
            for i in reversed(indices):
                self.boxes.pop(i)
            return original_boxes, insert_at

        def _restore_boxes(original_boxes, insert_at):
            for b in original_boxes:
                self.boxes.insert(insert_at, b)
                insert_at += 1
            return insert_at

        def _insert_ocr_boxes(ocr_results, page_index, crop_left, crop_top, insert_at, table_index, layoutno, best_angle, table_w_px, table_h_px):
            added = 0
            for bbox, (text, conf) in ocr_results:
                if not math.isfinite(conf) or conf < 0.5:
                    continue
                mapped = [self._map_clockwise_rotated_point_to_original(p[0], p[1], best_angle, table_w_px, table_h_px) for p in bbox]
                if not mapped or not all(math.isfinite(value) for point in mapped for value in point):
                    logging.warning("Skipping invalid rotated OCR coordinates on local page %s.", page_index + 1)
                    continue
                x_coords = [p[0] for p in mapped]
                y_coords = [p[1] for p in mapped]
                box_x0 = min(x_coords) / ZM
                box_x1 = max(x_coords) / ZM
                box_top = min(y_coords) / ZM
                box_bottom = max(y_coords) / ZM
                page_width, page_height = self.page_images[page_index].size
                page_x0 = max(0, min(box_x0 + crop_left / ZM, page_width / ZM))
                page_x1 = max(0, min(box_x1 + crop_left / ZM, page_width / ZM))
                page_top = max(0, min(box_top + crop_top / ZM, page_height / ZM))
                page_bottom = max(0, min(box_bottom + crop_top / ZM, page_height / ZM))
                if page_x1 <= page_x0 or page_bottom <= page_top:
                    logging.warning("Skipping empty rotated OCR box on local page %s after clamping.", page_index + 1)
                    continue
                new_box = {
                    "text": text,
                    "x0": page_x0,
                    "x1": page_x1,
                    "top": page_top + self.page_cum_height[page_index],
                    "bottom": page_bottom + self.page_cum_height[page_index],
                    "page_number": page_index + 1,
                    "layout_type": "table",
                    "layoutno": layoutno,
                    "_rotated": True,
                    "_rotation_angle": best_angle,
                    "_table_index": table_index,
                    "_rotated_x0": box_x0,
                    "_rotated_x1": box_x1,
                    "_rotated_top": box_top,
                    "_rotated_bottom": box_bottom,
                }
                self.boxes.insert(insert_at, new_box)
                insert_at += 1
                added += 1
            return added

        for tbl_info in table_layouts:
            table_index = tbl_info["table_index"]
            page = tbl_info["page"]
            layout = tbl_info["layout"]
            layoutno = tbl_info["layoutno"]
            left, top, right, bott = tbl_info["coords"]

            rotation_info = self.table_rotations.get(table_index, {})
            best_angle = rotation_info.get("best_angle", 0)

            # Get the rotated table image
            rotated_img = self.rotated_table_imgs.get(table_index)
            if rotated_img is None:
                continue

            # If no rotation, keep original OCR boxes untouched.
            if best_angle == 0:
                continue

            # Table region is defined by layout's x0, top, x1, bottom (page-local coords)
            table_x0, table_top, table_x1, table_bottom, table_top_cum, table_bottom_cum = _table_region(layout, page)
            original_boxes, insert_at = _collect_table_boxes(page, table_x0, table_x1, table_top_cum, table_bottom_cum)

            logging.info(f"Re-OCR table {table_index} on page {page} with rotation {best_angle}°")

            # Perform OCR on rotated image
            img_array = np.array(rotated_img)
            ocr_results = self.ocr(img_array)

            if not ocr_results:
                logging.warning(f"No OCR results for rotated table {table_index}, restoring originals")
                _restore_boxes(original_boxes, insert_at)
                continue

            # Add new OCR results to self.boxes
            # OCR coordinates are relative to rotated image, map back to original table coords
            table_w_px = right - left
            table_h_px = bott - top
            added = _insert_ocr_boxes(
                ocr_results,
                page,
                left,
                top,
                insert_at,
                table_index,
                layoutno,
                best_angle,
                table_w_px,
                table_h_px,
            )
            if not added:
                logging.warning("No valid rotated OCR boxes for table %s; restoring originals.", table_index)
                _restore_boxes(original_boxes, insert_at)

            logging.info(f"Added {added} OCR results from rotated table {table_index}")

    def __ocr(self, pagenum, img, chars, ZM=3, device_id: int | None = None):
        # start = timer()
        bxs = self.ocr.detect(np.array(img), device_id)
        # logging.info(f"__ocr detecting boxes of an image cost ({timer() - start}s)")

        # start = timer()
        if not bxs:
            self.boxes.append([])
            return
        bxs = [(line[0], line[1][0]) for line in bxs]
        bxs = Recognizer.sort_Y_firstly(
            [
                {"x0": b[0][0] / ZM, "x1": b[1][0] / ZM, "top": b[0][1] / ZM, "text": "", "txt": t, "bottom": b[-1][1] / ZM, "chars": [], "page_number": pagenum}
                for b, t in bxs
                if b[0][0] <= b[1][0] and b[0][1] <= b[-1][1]
            ],
            self.mean_height[pagenum - 1] / 3,
        )

        # merge chars in the same rect
        for c in chars:
            ii = Recognizer.find_overlapped(c, bxs)
            if ii is None:
                self.lefted_chars.append(c)
                continue
            ch = c["bottom"] - c["top"]
            bh = bxs[ii]["bottom"] - bxs[ii]["top"]
            if abs(ch - bh) / max(ch, bh) >= 0.7 and c["text"] != " ":
                self.lefted_chars.append(c)
                continue
            bxs[ii]["chars"].append(c)

        for b in bxs:
            if not b["chars"]:
                del b["chars"]
                continue
            box_chars = b["chars"]
            m_ht = np.mean([c["height"] for c in box_chars])
            garbled_count = 0
            total_count = 0
            for c in Recognizer.sort_Y_firstly(box_chars, m_ht):
                if c["text"] == " " and b["text"]:
                    if re.match(r"[0-9a-zA-Zа-яА-Я,.?;:!%%]", b["text"][-1]):
                        b["text"] += " "
                else:
                    b["text"] += c["text"]
                    for ch in c["text"]:
                        if not ch.isspace():
                            total_count += 1
                            if self._is_garbled_char(ch):
                                garbled_count += 1
            del b["chars"]

            # Strategy 1: PUA / unmapped CID characters. These are genuine garbage,
            # so re-OCR regardless of script.
            if total_count > 0 and garbled_count / total_count >= 0.5:
                logging.info(
                    "Page %d: detected garbled pdfplumber text (garbled=%d/%d), falling back to OCR for box at (%.1f, %.1f)",
                    pagenum,
                    garbled_count,
                    total_count,
                    b["x0"],
                    b["top"],
                )
                b["text"] = ""
                continue

            # Keep a clean text layer the recogniser cannot spell: ocr.res is
            # CJK+Latin, so re-OCRing e.g. a Cyrillic page only produces garbage.
            if total_count > 0 and not self._ocr_can_represent(b["text"]):
                continue

            # Strategy 2: font-encoding garbling — all chars are ASCII
            # punctuation from subset fonts (no CJK output)
            if total_count > 0 and self._is_garbled_by_font_encoding(box_chars, min_chars=5):
                logging.info(
                    "Page %d: detected font-encoding garbled text (%d chars), falling back to OCR for box at (%.1f, %.1f)",
                    pagenum,
                    total_count,
                    b["x0"],
                    b["top"],
                )
                b["text"] = ""

        # logging.info(f"__ocr sorting {len(chars)} chars cost {timer() - start}s")
        # start = timer()
        boxes_to_reg = []
        crop_boxes = []
        for b in bxs:
            if not b["text"]:
                left, right, top, bott = b["x0"] * ZM, b["x1"] * ZM, b["top"] * ZM, b["bottom"] * ZM
                crop_boxes.append(np.array([[left, top], [right, top], [right, bott], [left, bott]], dtype=np.float32))
                boxes_to_reg.append(b)
            del b["txt"]
        if boxes_to_reg:
            crops = self.ocr.get_rotate_crop_images(np.asarray(img), crop_boxes)
            for box, crop in zip(boxes_to_reg, crops):
                box["box_image"] = crop
        texts = self.ocr.recognize_batch([b["box_image"] for b in boxes_to_reg], device_id)
        for i in range(len(boxes_to_reg)):
            boxes_to_reg[i]["text"] = texts[i]
            del boxes_to_reg[i]["box_image"]
        # logging.info(f"__ocr recognize {len(bxs)} boxes cost {timer() - start}s")
        bxs = [b for b in bxs if b["text"]]
        if self.mean_height[pagenum - 1] == 0:
            self.mean_height[pagenum - 1] = np.median([b["bottom"] - b["top"] for b in bxs])
        self.boxes.append(bxs)

    def _layouts_rec(self, ZM, drop=True):
        assert len(self.page_images) == len(self.boxes)
        self.boxes, self.page_layout = self.layouter(self.page_images, self.boxes, ZM, drop=drop)
        # cumlative Y
        for i in range(len(self.boxes)):
            self.boxes[i]["top"] += self.page_cum_height[self.boxes[i]["page_number"] - 1]
            self.boxes[i]["bottom"] += self.page_cum_height[self.boxes[i]["page_number"] - 1]

    def _assign_column(self, boxes, zoomin=3):
        if not boxes:
            return boxes
        if all("col_id" in b for b in boxes):
            return boxes

        by_page = defaultdict(list)
        for b in boxes:
            by_page[b["page_number"]].append(b)

        page_cols = {}

        for pg, bxs in by_page.items():
            if not bxs:
                page_cols[pg] = 1
                continue

            x0s_raw = np.array([b["x0"] for b in bxs], dtype=float)

            min_x0 = np.min(x0s_raw)
            max_x1 = np.max([b["x1"] for b in bxs])
            width = max_x1 - min_x0

            INDENT_TOL = width * 0.12
            x0s = []
            for x in x0s_raw:
                if abs(x - min_x0) < INDENT_TOL:
                    x0s.append([min_x0])
                else:
                    x0s.append([x])
            x0s = np.array(x0s, dtype=float)

            max_try = min(4, len(bxs))
            if max_try < 2:
                max_try = 1
            best_k = 1
            best_score = -1

            for k in range(1, max_try + 1):
                km = KMeans(n_clusters=k, n_init="auto")
                labels = km.fit_predict(x0s)

                centers = np.sort(km.cluster_centers_.flatten())
                if len(centers) > 1:
                    try:
                        score = silhouette_score(x0s, labels)
                    except ValueError:
                        continue
                else:
                    score = 0
                if score > best_score:
                    best_score = score
                    best_k = k

            page_cols[pg] = best_k
            logging.info(f"[Page {pg}] best_score={best_score:.2f}, best_k={best_k}")

        global_cols = Counter(page_cols.values()).most_common(1)[0][0]
        logging.info(f"Global column_num decided by majority: {global_cols}")

        for pg, bxs in by_page.items():
            if not bxs:
                continue
            k = page_cols[pg]
            if len(bxs) < k:
                k = 1
            x0s = np.array([[b["x0"]] for b in bxs], dtype=float)
            km = KMeans(n_clusters=k, n_init="auto")
            labels = km.fit_predict(x0s)

            centers = km.cluster_centers_.flatten()
            order = np.argsort(centers)

            remap = {orig: new for new, orig in enumerate(order)}

            for b, lb in zip(bxs, labels):
                b["col_id"] = remap[lb]

            grouped = defaultdict(list)
            for b in bxs:
                grouped[b["col_id"]].append(b)

        return boxes

    def _text_merge(self, zoomin=3):
        # merge adjusted boxes
        bxs = self._assign_column(self.boxes, zoomin)

        def end_with(b, txt):
            txt = txt.strip()
            tt = b.get("text", "").strip()
            return tt and tt.find(txt) == len(tt) - len(txt)

        def start_with(b, txts):
            tt = b.get("text", "").strip()
            return tt and any([tt.find(t.strip()) == 0 for t in txts])

        # horizontally merge adjacent box with the same layout
        i = 0
        while i < len(bxs) - 1:
            b = bxs[i]
            b_ = bxs[i + 1]

            if b["page_number"] != b_["page_number"] or b.get("col_id") != b_.get("col_id"):
                i += 1
                continue

            if b.get("layoutno", "0") != b_.get("layoutno", "1") or b.get("layout_type", "") in ["table", "figure", "equation"]:
                i += 1
                continue

            if abs(self._y_dis(b, b_)) < self.mean_height[bxs[i]["page_number"] - 1] / 3:
                # merge
                bxs[i]["x1"] = b_["x1"]
                bxs[i]["top"] = (b["top"] + b_["top"]) / 2
                bxs[i]["bottom"] = (b["bottom"] + b_["bottom"]) / 2
                bxs[i]["text"] += b_["text"]
                bxs.pop(i + 1)
                continue
            i += 1
        self.boxes = bxs

    def _naive_vertical_merge(self, zoomin=3):
        # bxs = self._assign_column(self.boxes, zoomin)
        bxs = self.boxes

        grouped = defaultdict(list)
        for b in bxs:
            # grouped[(b["page_number"], b.get("col_id", 0))].append(b)
            grouped[(b["page_number"], "x")].append(b)

        merged_boxes = []
        for (pg, col), bxs in grouped.items():
            bxs = sorted(bxs, key=lambda x: (x["top"], x["x0"]))
            if not bxs:
                continue

            mh = self.mean_height[pg - 1] if self.mean_height else np.median([b["bottom"] - b["top"] for b in bxs]) or 10

            i = 0
            while i + 1 < len(bxs):
                b = bxs[i]
                b_ = bxs[i + 1]

                if b["page_number"] < b_["page_number"] and re.match(r"[0-9  •一—-]+$", b["text"]):
                    bxs.pop(i)
                    continue

                if not b["text"].strip():
                    bxs.pop(i)
                    continue

                if not b["text"].strip() or b.get("layoutno") != b_.get("layoutno"):
                    i += 1
                    continue

                if b_["top"] - b["bottom"] > mh * 1.5:
                    i += 1
                    continue

                overlap = max(0, min(b["x1"], b_["x1"]) - max(b["x0"], b_["x0"]))
                if overlap / max(1, min(b["x1"] - b["x0"], b_["x1"] - b_["x0"])) < 0.3:
                    i += 1
                    continue

                concatting_feats = [
                    b["text"].strip()[-1] in ",;:'\"，、‘“；：-",
                    len(b["text"].strip()) > 1 and b["text"].strip()[-2] in ",;:'\"，‘“、；：",
                    b_["text"].strip() and b_["text"].strip()[0] in "。；？！?”）),，、：",
                ]
                # features for not concating
                feats = [
                    b.get("layoutno", 0) != b_.get("layoutno", 0),
                    b["text"].strip()[-1] in "。？！?",
                    self.is_english and b["text"].strip()[-1] in ".!?",
                    b["page_number"] == b_["page_number"] and b_["top"] - b["bottom"] > self.mean_height[b["page_number"] - 1] * 1.5,
                    b["page_number"] < b_["page_number"] and abs(b["x0"] - b_["x0"]) > self.mean_width[b["page_number"] - 1] * 4,
                ]
                # split features
                detach_feats = [b["x1"] < b_["x0"], b["x0"] > b_["x1"]]
                if (any(feats) and not any(concatting_feats)) or any(detach_feats):
                    logging.debug(
                        "{} {} {} {}".format(
                            b["text"],
                            b_["text"],
                            any(feats),
                            any(concatting_feats),
                        )
                    )
                    i += 1
                    continue

                b["text"] = (b["text"].rstrip() + " " + b_["text"].lstrip()).strip()
                b["bottom"] = b_["bottom"]
                b["x0"] = min(b["x0"], b_["x0"])
                b["x1"] = max(b["x1"], b_["x1"])
                bxs.pop(i + 1)

            merged_boxes.extend(bxs)

        # self.boxes = sorted(merged_boxes, key=lambda x: (x["page_number"], x.get("col_id", 0), x["top"]))
        self.boxes = merged_boxes

    def _final_reading_order_merge(self, zoomin=3):
        if not self.boxes:
            return

        self.boxes = self._assign_column(self.boxes, zoomin=zoomin)

        pages = defaultdict(lambda: defaultdict(list))
        for b in self.boxes:
            pg = b["page_number"]
            col = b.get("col_id", 0)
            pages[pg][col].append(b)

        for pg in pages:
            for col in pages[pg]:
                pages[pg][col].sort(key=lambda x: (x["top"], x["x0"]))

        new_boxes = []
        for pg in sorted(pages.keys()):
            for col in sorted(pages[pg].keys()):
                new_boxes.extend(pages[pg][col])

        self.boxes = new_boxes

    def _concat_downward(self, concat_between_pages=True):
        self.boxes = Recognizer.sort_Y_firstly(self.boxes, 0)
        return

        # count boxes in the same row as a feature
        for i in range(len(self.boxes)):
            mh = self.mean_height[self.boxes[i]["page_number"] - 1]
            self.boxes[i]["in_row"] = 0
            j = max(0, i - 12)
            while j < min(i + 12, len(self.boxes)):
                if j == i:
                    j += 1
                    continue
                ydis = self._y_dis(self.boxes[i], self.boxes[j]) / mh
                if abs(ydis) < 1:
                    self.boxes[i]["in_row"] += 1
                elif ydis > 0:
                    break
                j += 1

        # concat between rows
        boxes = deepcopy(self.boxes)
        blocks = []
        while boxes:
            chunks = []

            def dfs(up, dp):
                chunks.append(up)
                i = dp
                while i < min(dp + 12, len(boxes)):
                    ydis = self._y_dis(up, boxes[i])
                    smpg = up["page_number"] == boxes[i]["page_number"]
                    mh = self.mean_height[up["page_number"] - 1]
                    mw = self.mean_width[up["page_number"] - 1]
                    if smpg and ydis > mh * 4:
                        break
                    if not smpg and ydis > mh * 16:
                        break
                    down = boxes[i]
                    if not concat_between_pages and down["page_number"] > up["page_number"]:
                        break

                    if up.get("R", "") != down.get("R", "") and up["text"][-1] != "，":
                        i += 1
                        continue

                    if re.match(r"[0-9]{2,3}/[0-9]{3}$", up["text"]) or re.match(r"[0-9]{2,3}/[0-9]{3}$", down["text"]) or not down["text"].strip():
                        i += 1
                        continue

                    if not down["text"].strip() or not up["text"].strip():
                        i += 1
                        continue

                    if up["x1"] < down["x0"] - 10 * mw or up["x0"] > down["x1"] + 10 * mw:
                        i += 1
                        continue

                    if i - dp < 5 and up.get("layout_type") == "text":
                        if up.get("layoutno", "1") == down.get("layoutno", "2"):
                            dfs(down, i + 1)
                            boxes.pop(i)
                            return
                        i += 1
                        continue

                    fea = self._updown_concat_features(up, down)
                    if self.updown_cnt_mdl.predict(xgb.DMatrix([fea]))[0] <= 0.5:
                        i += 1
                        continue
                    dfs(down, i + 1)
                    boxes.pop(i)
                    return

            dfs(boxes[0], 1)
            boxes.pop(0)
            if chunks:
                blocks.append(chunks)

        # concat within each block
        boxes = []
        for b in blocks:
            if len(b) == 1:
                boxes.append(b[0])
                continue
            t = b[0]
            for c in b[1:]:
                t["text"] = t["text"].strip()
                c["text"] = c["text"].strip()
                if not c["text"]:
                    continue
                if t["text"] and re.match(r"[0-9\.a-zA-Z]+$", t["text"][-1] + c["text"][-1]):
                    t["text"] += " "
                t["text"] += c["text"]
                t["x0"] = min(t["x0"], c["x0"])
                t["x1"] = max(t["x1"], c["x1"])
                t["page_number"] = min(t["page_number"], c["page_number"])
                t["bottom"] = c["bottom"]
                if not t["layout_type"] and c["layout_type"]:
                    t["layout_type"] = c["layout_type"]
            boxes.append(t)

        self.boxes = Recognizer.sort_Y_firstly(boxes, 0)

    def _filter_forpages(self):
        if not self.boxes:
            return
        findit = False
        i = 0
        while i < len(self.boxes):
            if not re.match(r"(contents|目录|目次|table of contents|致谢|acknowledge)$", re.sub(r"( | |\u3000)+", "", self.boxes[i]["text"].lower())):
                i += 1
                continue
            findit = True
            eng = re.match(r"[0-9a-zA-Z :'.-]{5,}", self.boxes[i]["text"].strip())
            self.boxes.pop(i)
            if i >= len(self.boxes):
                break
            prefix = self.boxes[i]["text"].strip()[:3] if not eng else " ".join(self.boxes[i]["text"].strip().split()[:2])
            while not prefix:
                self.boxes.pop(i)
                if i >= len(self.boxes):
                    break
                prefix = self.boxes[i]["text"].strip()[:3] if not eng else " ".join(self.boxes[i]["text"].strip().split()[:2])
            self.boxes.pop(i)
            if i >= len(self.boxes) or not prefix:
                break
            for j in range(i, min(i + 128, len(self.boxes))):
                if not re.match(prefix, self.boxes[j]["text"]):
                    continue
                for k in range(i, j):
                    self.boxes.pop(i)
                break
        if findit:
            return

        page_dirty = [0] * len(self.page_images)
        for b in self.boxes:
            if re.search(r"(··|··|··)", b["text"]):
                page_dirty[b["page_number"] - 1] += 1
        page_dirty = set([i + 1 for i, t in enumerate(page_dirty) if t > 3])
        if not page_dirty:
            return
        i = 0
        while i < len(self.boxes):
            if self.boxes[i]["page_number"] in page_dirty:
                self.boxes.pop(i)
                continue
            i += 1

    def _merge_with_same_bullet(self):
        i = 0
        while i + 1 < len(self.boxes):
            b = self.boxes[i]
            b_ = self.boxes[i + 1]
            if not b["text"].strip():
                self.boxes.pop(i)
                continue
            if not b_["text"].strip():
                self.boxes.pop(i + 1)
                continue

            if (
                b["text"].strip()[0] != b_["text"].strip()[0]
                or b["text"].strip()[0].lower() in set("qwertyuopasdfghjklzxcvbnm")
                or rag_tokenizer.is_chinese(b["text"].strip()[0])
                or b["top"] > b_["bottom"]
            ):
                i += 1
                continue
            b_["text"] = b["text"] + "\n" + b_["text"]
            b_["x0"] = min(b["x0"], b_["x0"])
            b_["x1"] = max(b["x1"], b_["x1"])
            b_["top"] = b["top"]
            self.boxes.pop(i)

    def _extract_table_figure(self, need_image, ZM, return_html, need_position, separate_tables_figures=False):
        tables = {}
        figures = {}
        # extract figure and table boxes
        i = 0
        lst_lout_no = ""
        nomerge_lout_no = []
        while i < len(self.boxes):
            if "layoutno" not in self.boxes[i]:
                i += 1
                continue
            lout_no = str(self.boxes[i]["page_number"]) + "-" + str(self.boxes[i]["layoutno"])
            if TableStructureRecognizer.is_caption(self.boxes[i]) or self.boxes[i]["layout_type"] in ["table caption", "title", "figure caption", "reference"]:
                nomerge_lout_no.append(lst_lout_no)
            if self.boxes[i]["layout_type"] == "table":
                if re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"]):
                    self.boxes.pop(i)
                    continue
                if lout_no not in tables:
                    tables[lout_no] = []
                tables[lout_no].append(self.boxes[i])
                self.boxes.pop(i)
                lst_lout_no = lout_no
                continue
            if need_image and self.boxes[i]["layout_type"] == "figure":
                if re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"]):
                    self.boxes.pop(i)
                    continue
                if lout_no not in figures:
                    figures[lout_no] = []
                figures[lout_no].append(self.boxes[i])
                self.boxes.pop(i)
                lst_lout_no = lout_no
                continue
            i += 1

        # merge table on different pages
        nomerge_lout_no = set(nomerge_lout_no)
        tbls = sorted([(k, bxs) for k, bxs in tables.items()], key=lambda x: (x[1][0]["top"], x[1][0]["x0"]))

        i = len(tbls) - 1
        while i - 1 >= 0:
            k0, bxs0 = tbls[i - 1]
            k, bxs = tbls[i]
            i -= 1
            if k0 in nomerge_lout_no:
                continue
            if bxs[0]["page_number"] == bxs0[0]["page_number"]:
                continue
            if bxs[0]["page_number"] - bxs0[0]["page_number"] > 1:
                continue
            mh = self.mean_height[bxs[0]["page_number"] - 1]
            if self._y_dis(bxs0[-1], bxs[0]) > mh * 23:
                continue
            tables[k0].extend(tables[k])
            del tables[k]

        def x_overlapped(a, b):
            return not any([a["x1"] < b["x0"], a["x0"] > b["x1"]])

        # find captions and pop out
        i = 0
        while i < len(self.boxes):
            c = self.boxes[i]
            # mh = self.mean_height[c["page_number"]-1]
            if not TableStructureRecognizer.is_caption(c):
                i += 1
                continue

            # find the nearest layouts
            def nearest(tbls):
                nonlocal c
                mink = ""
                minv = 1000000000
                for k, bxs in tbls.items():
                    for b in bxs:
                        if b.get("layout_type", "").find("caption") >= 0:
                            continue
                        y_dis = self._y_dis(c, b)
                        x_dis = self._x_dis(c, b) if not x_overlapped(c, b) else 0
                        dis = y_dis * y_dis + x_dis * x_dis
                        if dis < minv:
                            mink = k
                            minv = dis
                return mink, minv

            tk, tv = nearest(tables)
            fk, fv = nearest(figures)
            # if min(tv, fv) > 2000:
            #    i += 1
            #    continue
            if tv < fv and tk:
                tables[tk].insert(0, c)
                logging.debug("TABLE:" + self.boxes[i]["text"] + "; Cap: " + tk)
            elif fk:
                figures[fk].insert(0, c)
                logging.debug("FIGURE:" + self.boxes[i]["text"] + "; Cap: " + tk)
            self.boxes.pop(i)

        def cropout(bxs, ltype, poss):
            nonlocal ZM
            max_page_index = len(self.page_images) - 1

            def local_page_index(page_number):
                idx = page_number - 1 if page_number > 0 else 0
                if idx > max_page_index and self.page_from:
                    idx = page_number - 1 - self.page_from
                return idx

            pn = set()
            for b in bxs:
                idx = local_page_index(b["page_number"])
                if 0 <= idx <= max_page_index:
                    pn.add(idx)
                else:
                    logging.warning(
                        "Skip out-of-range page_number %s (page_from=%s, pages=%s)",
                        b.get("page_number"),
                        self.page_from,
                        len(self.page_images),
                    )

            if not pn:
                return None

            if len(pn) < 2:
                pn = list(pn)[0]
                ht = self.page_cum_height[pn]
                b = {"x0": np.min([b["x0"] for b in bxs]), "top": np.min([b["top"] for b in bxs]) - ht, "x1": np.max([b["x1"] for b in bxs]), "bottom": np.max([b["bottom"] for b in bxs]) - ht}
                louts = [layout for layout in self.page_layout[pn] if layout["type"] == ltype]
                ii = Recognizer.find_overlapped(b, louts, naive=True)
                if ii is not None:
                    b = louts[ii]
                else:
                    logging.warning(f"Missing layout match: {pn + 1},%s" % (bxs[0].get("layoutno", "")))

                left, top, right, bott = b["x0"], b["top"], b["x1"], b["bottom"]
                if right < left:
                    right = left + 1
                poss.append((pn + self.page_from, left, right, top, bott))
                return self.page_images[pn].crop((left * ZM, top * ZM, right * ZM, bott * ZM))
            pn = {}
            for b in bxs:
                p = local_page_index(b["page_number"])
                if 0 <= p <= max_page_index:
                    if p not in pn:
                        pn[p] = []
                    pn[p].append(b)
            pn = sorted(pn.items(), key=lambda x: x[0])
            imgs = [cropout(arr, ltype, poss) for p, arr in pn]
            imgs = [img for img in imgs if img is not None]
            if not imgs:
                return None
            pic = Image.new("RGB", (int(np.max([i.size[0] for i in imgs])), int(np.sum([m.size[1] for m in imgs]))), (245, 245, 245))
            height = 0
            for img in imgs:
                pic.paste(img, (0, int(height)))
                height += img.size[1]
            return pic

        res = []
        positions = []
        figure_results = []
        figure_positions = []
        # crop figure out and add caption
        for k, bxs in figures.items():
            if not bxs:
                continue
            txt = "\n".join([b["text"] for b in bxs if b.get("text")])

            poss = []

            if separate_tables_figures:
                img = cropout(bxs, "figure", poss)
                if img is None:
                    continue
                figure_results.append((img, [txt] if txt else [""]))
                figure_positions.append(poss)
            else:
                img = cropout(bxs, "figure", poss)
                if img is None:
                    continue
                res.append((img, [txt] if txt else [""]))
                positions.append(poss)

        for k, bxs in tables.items():
            if not bxs:
                continue
            bxs = Recognizer.sort_Y_firstly(bxs, np.mean([(b["bottom"] - b["top"]) / 2 for b in bxs]))

            poss = []

            img = cropout(bxs, "table", poss)
            if img is None:
                continue
            res.append((img, self.tbl_det.construct_table(bxs, html=return_html, is_english=self.is_english)))
            positions.append(poss)

        if separate_tables_figures:
            assert len(positions) + len(figure_positions) == len(res) + len(figure_results)
            if need_position:
                return list(zip(res, positions)), list(zip(figure_results, figure_positions))
            else:
                return res, figure_results
        else:
            assert len(positions) == len(res)
            if need_position:
                return list(zip(res, positions))
            else:
                return res

    def proj_match(self, line):
        if len(line) <= 2:
            return
        if re.match(r"[0-9 ().,%%+/-]+$", line):
            return False
        for p, j in [
            (r"第[零一二三四五六七八九十百]+章", 1),
            (r"第[零一二三四五六七八九十百]+[条节]", 2),
            (r"[零一二三四五六七八九十百]+[、 　]", 3),
            (r"[\(（][零一二三四五六七八九十百]+[）\)]", 4),
            (r"[0-9]+(、|\.[　 ]|\.[^0-9])", 5),
            (r"[0-9]+\.[0-9]+(、|[. 　]|[^0-9])", 6),
            (r"[0-9]+\.[0-9]+\.[0-9]+(、|[ 　]|[^0-9])", 7),
            (r"[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+(、|[ 　]|[^0-9])", 8),
            (r".{,48}[：:?？]$", 9),
            (r"[0-9]+）", 10),
            (r"[\(（][0-9]+[）\)]", 11),
            (r"[零一二三四五六七八九十百]+是", 12),
            (r"[⚫•➢✓]", 12),
        ]:
            if re.match(p, line):
                return j
        return

    def _line_tag(self, bx, ZM):
        """Serialize valid page-local positions without losing per-page bounds."""
        tags = []
        group = []

        def append_tag():
            first, last = group[0], group[-1]
            tags.append("@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format("-".join(str(pos[0]) for pos in group), first[1], first[2], first[3], last[4]))

        for pos in self.get_position(bx, ZM):
            # A single tag has one horizontal interval for every page. Split
            # when different page widths produce different clipped bounds.
            if group and (pos[0] != group[-1][0] + 1 or pos[1:3] != group[-1][1:3]):
                append_tag()
                group = []
            group.append(pos)
        if group:
            append_tag()
        return "".join(tags)

    def __filterout_scraps(self, boxes, ZM):
        def width(b):
            return b["x1"] - b["x0"]

        def height(b):
            return b["bottom"] - b["top"]

        def usefull(b):
            if b.get("layout_type"):
                return True
            if width(b) > self.page_images[b["page_number"] - 1].size[0] / ZM / 3:
                return True
            if b["bottom"] - b["top"] > self.mean_height[b["page_number"] - 1]:
                return True
            return False

        res = []
        while boxes:
            lines = []
            widths = []
            pw = self.page_images[boxes[0]["page_number"] - 1].size[0] / ZM
            mh = self.mean_height[boxes[0]["page_number"] - 1]
            mj = self.proj_match(boxes[0]["text"]) or boxes[0].get("layout_type", "") == "title"

            def dfs(line, st):
                nonlocal mh, pw, lines, widths
                lines.append(line)
                widths.append(width(line))
                mmj = self.proj_match(line["text"]) or line.get("layout_type", "") == "title"
                for i in range(st + 1, min(st + 20, len(boxes))):
                    if (boxes[i]["page_number"] - line["page_number"]) > 0:
                        break
                    if not mmj and self._y_dis(line, boxes[i]) >= 3 * mh and height(line) < 1.5 * mh:
                        break

                    if not usefull(boxes[i]):
                        continue
                    if mmj or (self._x_dis(boxes[i], line) < pw / 10):
                        # and abs(width(boxes[i])-width_mean)/max(width(boxes[i]),width_mean)<0.5):
                        # concat following
                        dfs(boxes[i], i)
                        boxes.pop(i)
                        break

            try:
                if usefull(boxes[0]):
                    dfs(boxes[0], 0)
                else:
                    logging.debug("WASTE: " + boxes[0]["text"])
            except Exception:
                pass
            boxes.pop(0)
            mw = np.mean(widths)
            if mj or mw / pw >= 0.35 or mw > 200:
                res.append("\n".join([c["text"] + self._line_tag(c, ZM) for c in lines]))
            else:
                logging.debug("REMOVED: " + "<<".join([c["text"] for c in lines]))

        return "\n\n".join(res)

    @staticmethod
    def total_page_number(fnm, binary=None):
        try:
            with sys.modules[LOCK_KEY_pdfplumber]:
                pdf = pdfplumber.open(fnm) if not binary else pdfplumber.open(BytesIO(binary))
            total_page = len(pdf.pages)
            pdf.close()
            return total_page
        except Exception:
            logging.exception("total_page_number")

    def __images__(self, fnm, zoomin=3, page_from=0, page_to=MAXIMUM_PAGE_NUMBER, callback=None):
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
                with pdfplumber.open(fnm) if isinstance(fnm, str) else pdfplumber.open(BytesIO(fnm)) as pdf:
                    self.pdf = pdf
                    self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).annotated for i, p in enumerate(self.pdf.pages[page_from:page_to])]

                    try:
                        self.page_chars = [[c for c in page.dedupe_chars().chars if self._has_color(c)] for page in self.pdf.pages[page_from:page_to]]
                    except Exception as e:
                        logging.warning(f"Failed to extract characters for pages {page_from}-{page_to}: {str(e)}")
                        self.page_chars = [[] for _ in range(len(self.page_images))]  # If failed to extract, using empty list instead.

                    # Detect garbled pages and clear their chars so the OCR
                    # path will be used instead. Two detection strategies:
                    # 1) PUA / unmapped CID characters (threshold=0.3)
                    # 2) Font-encoding garbling: subset fonts mapping CJK to ASCII
                    for pi, page_ch in enumerate(self.page_chars):
                        if not page_ch:
                            continue
                        # Strategy 1: PUA / CID garbling
                        sample = page_ch if len(page_ch) <= 200 else page_ch[:200]
                        sample_text = "".join(c.get("text", "") for c in sample)
                        if self._is_garbled_text(sample_text, threshold=0.3):
                            logging.warning(
                                "Page %d: pdfplumber extracted mostly garbled characters (%d chars), clearing to use OCR fallback.",
                                page_from + pi + 1,
                                len(page_ch),
                            )
                            self.page_chars[pi] = []
                            continue
                        # Strategy 2: font-encoding garbling (CJK mapped to ASCII)
                        if self._is_garbled_by_font_encoding(page_ch):
                            logging.warning(
                                "Page %d: detected font-encoding garbled text (subset fonts with no CJK output, %d chars), clearing to use OCR fallback.",
                                page_from + pi + 1,
                                len(page_ch),
                            )
                            self.page_chars[pi] = []

                    self.total_page = len(self.pdf.pages)

        except Exception as e:
            logging.exception(f"RAGFlowPdfParser __images__, exception: {e}")
        logging.info(f"__images__ dedupe_chars cost {timer() - start}s")

        logging.debug("Images converted.")
        self.is_english = [
            re.search(r"[ a-zA-Z0-9,/¸;:'\[\]\(\)!@#$%^&*\"?<>._-]{30,}", "".join(random.choices([c["text"] for c in self.page_chars[i]], k=min(100, len(self.page_chars[i])))))
            for i in range(len(self.page_chars))
        ]
        if sum([1 if e else 0 for e in self.is_english]) > len(self.page_images) / 2:
            self.is_english = True
        else:
            self.is_english = False

        async def __img_ocr(i, id, img, chars, limiter):
            self._insert_word_spaces(chars)

            if limiter:
                async with limiter:
                    await thread_pool_exec(self.__ocr, i + 1, img, chars, zoomin, id)
            else:
                self.__ocr(i + 1, img, chars, zoomin, id)

            if callback and i % 6 == 5:
                callback((i + 1) * 0.6 / len(self.page_images))

        async def __img_ocr_launcher():
            def __ocr_preprocess():
                chars = self.page_chars[i] if not self.is_english else []
                self.mean_height.append(np.median(sorted([c["height"] for c in chars])) if chars else 0)
                self.mean_width.append(np.median(sorted([c["width"] for c in chars])) if chars else 8)
                self.page_cum_height.append(img.size[1] / zoomin)
                return chars

            if self.parallel_limiter:
                tasks = []

                for i, img in enumerate(self.page_images):
                    chars = __ocr_preprocess()

                    semaphore = self.parallel_limiter[i % settings.PARALLEL_DEVICES]

                    async def wrapper(i=i, img=img, chars=chars, semaphore=semaphore):
                        await __img_ocr(
                            i,
                            i % settings.PARALLEL_DEVICES,
                            img,
                            chars,
                            semaphore,
                        )

                    tasks.append(asyncio.create_task(wrapper()))
                    await asyncio.sleep(0)

                try:
                    await asyncio.gather(*tasks, return_exceptions=False)
                except Exception as e:
                    logging.error(f"Error in OCR: {e}")
                    for t in tasks:
                        t.cancel()
                    await asyncio.gather(*tasks, return_exceptions=True)
                    raise

            else:
                for i, img in enumerate(self.page_images):
                    chars = __ocr_preprocess()
                    await __img_ocr(i, 0, img, chars, None)

        start = timer()

        asyncio.run(__img_ocr_launcher())

        logging.info(f"__images__ {len(self.page_images)} pages cost {timer() - start}s")

        if not self.is_english and not any([c for c in self.page_chars]) and self.boxes:
            bxes = [b for bxs in self.boxes for b in bxs]
            self.is_english = re.search(r"[ \na-zA-Z0-9,/¸;:'\[\]\(\)!@#$%^&*\"?<>._-]{30,}", "".join([b["text"] for b in random.choices(bxes, k=min(30, len(bxes)))]))

        logging.debug(f"Is it English: {self.is_english}")

        self.page_cum_height = np.cumsum(self.page_cum_height)
        assert len(self.page_cum_height) == len(self.page_images) + 1
        if len(self.boxes) == 0 and zoomin < 9:
            self.__images__(fnm, zoomin * 3, page_from, page_to, callback)

    def __call__(self, fnm, need_image=True, zoomin=3, return_html=False, auto_rotate_tables=None):
        """
        Parse a PDF file.

        Args:
            fnm: PDF file path or binary content
            need_image: Whether to extract images
            zoomin: Zoom factor
            return_html: Whether to return tables in HTML format
            auto_rotate_tables: Whether to enable auto orientation correction for tables.
                               None: Use TABLE_AUTO_ROTATE env var setting (default: True)
                               True: Enable auto orientation correction
                               False: Disable auto orientation correction
        """
        self.outlines = extract_pdf_outlines(fnm)
        self.__images__(fnm, zoomin)
        self._layouts_rec(zoomin)
        self._table_transformer_job(zoomin, auto_rotate=auto_rotate_tables)
        self._text_merge()
        self._concat_downward()
        self._filter_forpages()
        tbls = self._extract_table_figure(need_image, zoomin, return_html, False)
        return self.__filterout_scraps(deepcopy(self.boxes), zoomin), tbls

    def parse_into_bboxes(self, fnm, callback=None, zoomin=3, from_page=0, to_page=MAXIMUM_PAGE_NUMBER):
        self.outlines = extract_pdf_outlines(fnm)
        batch_size = max(1, int(os.getenv("PDF_PARSER_PAGE_BATCH_SIZE", "50")))
        if isinstance(fnm, str):
            total_pages = self.total_page_number(fnm)
        else:
            total_pages = self.total_page_number(fnm, binary=fnm)

        if total_pages is None:
            effective_to_page = to_page
            logging.warning(
                "parse_into_bboxes: total_page_number returned None; using caller-supplied to_page=%s",
                to_page,
            )
        else:
            effective_to_page = min(to_page, total_pages)

        if effective_to_page - from_page <= batch_size:
            self.__images__(fnm, zoomin, page_from=from_page, page_to=effective_to_page, callback=callback)
            return self._parse_loaded_window_into_bboxes(zoomin, callback=callback)

        logging.info(
            "parse_into_bboxes uses chunk mode: from_page=%s, effective_to_page=%s, batch_size=%s",
            from_page,
            effective_to_page,
            batch_size,
        )
        all_boxes = []
        start = timer()
        for page_from in range(from_page, effective_to_page, batch_size):
            page_to = min(page_from + batch_size, effective_to_page)
            self.__images__(fnm, zoomin, page_from=page_from, page_to=page_to, callback=None)
            chunk_boxes = self._parse_loaded_window_into_bboxes(zoomin)
            all_boxes.extend(self._to_global_boxes(chunk_boxes))
            if callback:
                callback((page_to - from_page) / max(1, effective_to_page - from_page), f"Structured: {page_to}/{effective_to_page} pages")

        logging.info("parse_into_bboxes chunk mode cost %.2fs", timer() - start)
        return all_boxes

    def _parse_loaded_window_into_bboxes(self, zoomin=3, callback=None):
        start = timer()
        self._layouts_rec(zoomin)
        if callback:
            callback(0.63, "Layout analysis ({:.2f}s)".format(timer() - start))

        start = timer()
        self._table_transformer_job(zoomin)
        if callback:
            callback(0.83, "Table analysis ({:.2f}s)".format(timer() - start))

        start = timer()
        self._text_merge()
        self._concat_downward()
        self._naive_vertical_merge(zoomin)
        if callback:
            callback(0.92, "Text merged ({:.2f}s)".format(timer() - start))

        start = timer()
        tbls, figs = self._extract_table_figure(True, zoomin, True, True, True)

        def insert_table_figures(tbls_or_figs, layout_type):
            def min_rectangle_distance(rect1, rect2):
                pn1, left1, right1, top1, bottom1 = rect1
                pn2, left2, right2, top2, bottom2 = rect2
                if right1 >= left2 and right2 >= left1 and bottom1 >= top2 and bottom2 >= top1:
                    return 0
                if right1 < left2:
                    dx = left2 - right1
                elif right2 < left1:
                    dx = left1 - right2
                else:
                    dx = 0
                if bottom1 < top2:
                    dy = top2 - bottom1
                elif bottom2 < top1:
                    dy = top1 - bottom2
                else:
                    dy = 0
                return math.sqrt(dx * dx + dy * dy)

            for (img, txt), poss in tbls_or_figs:
                local_poss = []
                for pn, left, right, top, bott in poss:
                    local_pn = pn - self.page_from
                    if 0 <= local_pn < len(self.page_cum_height) - 1:
                        local_poss.append((local_pn, left, right, top, bott))
                    else:
                        logging.debug(f"Skip out-of-range table/figure position pn={pn}, page_from={self.page_from}")
                if not local_poss:
                    logging.debug("No valid local positions for table/figure; skip insertion.")
                    continue

                if isinstance(txt, list):
                    txt = "\n".join(txt)
                pn, left, right, top, bott = local_poss[0]
                insert_at = len(self.boxes)
                bboxes = [(i, (b["page_number"], b["x0"], b["x1"], b["top"], b["bottom"])) for i, b in enumerate(self.boxes)]
                if bboxes:
                    dists = [
                        (min_rectangle_distance((cand_pn, cand_left, cand_right, cand_top + self.page_cum_height[cand_pn], cand_bott + self.page_cum_height[cand_pn]), rect), i)
                        for i, rect in bboxes
                        for cand_pn, cand_left, cand_right, cand_top, cand_bott in local_poss
                    ]
                    if dists:
                        nearest_bbox_idx = int(np.argmin([dist for dist, _ in dists]))
                        insert_at, _ = bboxes[dists[nearest_bbox_idx][-1]]
                        if self.boxes[insert_at]["bottom"] < top + self.page_cum_height[pn]:
                            insert_at += 1
                else:
                    logging.debug("No text boxes available; append %s block directly.", layout_type)
                self.boxes.insert(
                    insert_at,
                    {
                        "page_number": pn + 1,
                        "x0": left,
                        "x1": right,
                        "top": top + self.page_cum_height[pn],
                        "bottom": bott + self.page_cum_height[pn],
                        "layout_type": layout_type,
                        "text": txt,
                        "image": img,
                        "positions": [[page + 1, int(x0), int(x1), int(y0), int(y1)] for page, x0, x1, y0, y1 in local_poss],
                    },
                )

        for b in self.boxes:
            b["position_tag"] = self._line_tag(b, zoomin)
            b["image"] = self.crop(b["position_tag"], zoomin)
            b["positions"] = [list(pos) for pos in self.get_position(b, zoomin)]

        insert_table_figures(tbls, "table")
        insert_table_figures(figs, "figure")
        if callback:
            callback(1, "Structured ({:.2f}s)".format(timer() - start))
        return deepcopy(self.boxes)

    @staticmethod
    def _offset_position_tag(text, page_offset):
        if not text or page_offset <= 0:
            return text

        def _replace(match):
            pages = [str(int(p) + page_offset) for p in match.group(1).split("-")]
            return f"@@{'-'.join(pages)}\t"

        return re.sub(r"@@([0-9-]+)\t", _replace, text)

    def _to_global_boxes(self, boxes):
        if self.page_from <= 0:
            return boxes

        for box in boxes:
            box["page_number"] = int(box.get("page_number", 1)) + self.page_from
            if isinstance(box.get("position_tag"), str):
                box["position_tag"] = self._offset_position_tag(box["position_tag"], self.page_from)
            if isinstance(box.get("positions"), list):
                box["positions"] = [[int(pos[0]) + self.page_from, *pos[1:]] if isinstance(pos, list) and len(pos) > 0 and isinstance(pos[0], (int, float)) else pos for pos in box["positions"]]
        return boxes

    @staticmethod
    def remove_tag(txt):
        return re.sub(r"@@[\t0-9.-]+?##", "", txt)

    @staticmethod
    def extract_positions(txt):
        poss = []
        for tag in re.findall(r"@@[0-9-]+\t[-0-9.\t]+##", txt):
            try:
                pn, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
                left, right, top, bottom = float(left), float(right), float(top), float(bottom)
                poss.append(([int(p) - 1 for p in pn.split("-")], left, right, top, bottom))
            except ValueError:
                # a '-' can land in a coordinate slot; skip the tag, as ExtractPositions in internal/deepdoc/parser/pdf/util/position.go does
                continue
        return poss

    def crop(self, text, ZM=3, need_position=False):
        poss = self.extract_positions(text)
        if not poss:
            if need_position:
                return None, None
            return

        if not getattr(self, "page_images", None):
            logging.warning("crop called without page images; skipping image generation.")
            if need_position:
                return None, None
            return

        page_count = len(self.page_images)

        normalized = []
        for pns, left, right, top, bottom in poss:
            if not pns:
                logging.warning("Empty page index list in crop; skipping this position.")
                continue
            if any(not (0 <= p < page_count) for p in pns):
                logging.warning("Page indices %s out of range for %s pages during crop; skipping this position.", pns, page_count)
                continue
            if any(b != a + 1 for a, b in zip(pns, pns[1:])):
                logging.warning("Nonconsecutive page indices %s during crop; skipping this position.", pns)
                continue
            left, right = sorted((left, right))
            if len(pns) == 1:
                top, bottom = sorted((top, bottom))
            normalized.append((pns, left, right, top, bottom))

        GAP = 6
        CONTEXT = 120

        def _crop_range(pns, left, right, top, bottom, is_context=False):
            # A tag's top is local to its first page, bottom to its last.
            # Keep all offsets in pixels; comparing these local endpoints or
            # swapping a negative remainder would invent content across pages.
            start = top * ZM
            end = bottom * ZM + sum(self.page_images[pn].height for pn in pns[:-1])
            if end <= start:
                if not is_context:
                    logging.warning(
                        "Skipping empty crop range pages=%s raw=(%s,%s,%s,%s) reason=non_positive_vertical_span",
                        pns,
                        left,
                        right,
                        top,
                        bottom,
                    )
                return []
            pieces = []
            offset = 0
            for pn in pns:
                page_width, page_height = self.page_images[pn].size
                x0 = max(0.0, min(left * ZM, page_width))
                x1 = max(0.0, min(right * ZM, page_width))
                y0 = max(0.0, min(start - offset, page_height))
                y1 = max(0.0, min(end - offset, page_height))
                offset += page_height
                # PIL rounds endpoints, not their difference. A positive float
                # span can still collapse to a zero-pixel crop.
                if round(x1) <= round(x0) or round(y1) <= round(y0):
                    if not is_context:
                        logging.warning(
                            "Skipping empty crop page=%s raw=(%s,%s,%s,%s) clamped=(%s,%s,%s,%s) reason=empty_after_clamp",
                            pn + self.page_from,
                            left,
                            right,
                            top,
                            bottom,
                            x0,
                            y0,
                            x1,
                            y1,
                        )
                    continue
                pieces.append((pn, self.page_images[pn].crop((x0, y0, x1, y1)), (x0 / ZM, y0 / ZM, x1 / ZM, y1 / ZM)))
            return pieces

        body = []
        for pns, left, right, top, bottom in normalized:
            pieces = _crop_range(pns, left, max(left + 10, right), top, bottom)
            if pieces:
                body.append((left, right, pieces))
        if not body:
            if need_position:
                return None, None
            return

        max_width = max(max(right - left for left, right, _pieces in body), GAP)
        first_left, _first_right, first_pieces = body[0]
        first_pn, _first_img, (_fx0, first_top, _fx1, _first_bottom) = first_pieces[0]
        last_left, _last_right, last_pieces = body[-1]
        last_pn, _last_img, (_lx0, _last_top, _lx1, last_bottom) = last_pieces[-1]

        def _context(pn, left, top, bottom):
            return [(img, True) for _page, img, _box in _crop_range([pn], left, left + max_width, top, bottom, is_context=True)]

        head = _context(first_pn, first_left, first_top - CONTEXT, first_top - GAP)
        tail = _context(last_pn, last_left, last_bottom + GAP, last_bottom + CONTEXT)

        positions = []
        body_slices = []
        for _left, _right, pieces in body:
            for pn, img, (x0, y0, x1, y1) in pieces:
                body_slices.append((img, False))
                positions.append((pn + self.page_from, x0, x1, y0, y1))
        imgs = head + body_slices + tail

        height = sum(img.height + GAP for img, _is_context in imgs)
        width = max(img.width for img, _is_context in imgs)
        pic = Image.new("RGB", (width, height), (245, 245, 245))
        height = 0
        for img, is_context in imgs:
            if is_context:
                img = img.convert("RGBA")
                overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
                overlay.putalpha(128)
                img = Image.alpha_composite(img, overlay).convert("RGB")
            pic.paste(img, (0, int(height)))
            height += img.size[1] + GAP

        if need_position:
            return pic, positions
        return pic

    def get_position(self, bx, ZM):
        """Map a cumulative box to bounded, window-local 1-based page slices."""
        poss = []
        pn = bx.get("page_number")
        images = getattr(self, "page_images", None)
        cumulative = getattr(self, "page_cum_height", None)
        if not images or cumulative is None or isinstance(pn, bool) or not isinstance(pn, (int, np.integer)) or not 1 <= pn <= len(images) or len(cumulative) < len(images):
            logging.warning("Skipping position with invalid local page number %s or missing page geometry.", pn)
            return poss
        try:
            coordinates = [float(bx[key]) for key in ("x0", "x1", "top", "bottom")]
            if not math.isfinite(ZM) or ZM <= 0 or not all(math.isfinite(value) for value in coordinates):
                raise ValueError("non-finite coordinates or invalid zoom")
        except (KeyError, TypeError, ValueError, OverflowError):
            logging.warning("Skipping invalid position coordinates on local page %s.", pn)
            return poss

        left, right = sorted(coordinates[:2])
        top, bottom = sorted(coordinates[2:])
        # Validate the local page number before indexing cumulative heights.
        # page_from is added only at the external/global conversion boundary.
        start = (top - cumulative[pn - 1]) * ZM
        end = (bottom - cumulative[pn - 1]) * ZM
        offset = 0
        for index in range(pn - 1, len(images)):
            width, height = images[index].size
            x0, x1 = max(0, min(left * ZM, width)), max(0, min(right * ZM, width))
            y0, y1 = max(0, min(start - offset, height)), max(0, min(end - offset, height))
            if x1 > x0 and y1 > y0:
                poss.append((index + 1, x0 / ZM, x1 / ZM, y0 / ZM, y1 / ZM))
            offset += height
            if offset >= end:
                break
        if not poss:
            logging.warning("Skipping empty position on local page %s after clamping.", pn)
        return poss


class PlainParser:
    def __call__(self, filename, from_page=0, to_page=MAXIMUM_PAGE_NUMBER, **kwargs):
        lines = []
        try:
            self.pdf = pdf2_read(filename if isinstance(filename, str) else BytesIO(filename))
            for page in self.pdf.pages[from_page:to_page]:
                lines.extend([t for t in page.extract_text().split("\n")])
        except Exception:
            logging.exception("Outlines exception")
        self.outlines = extract_pdf_outlines(filename)

        return [(line, "") for line in lines], []

    def crop(self, ck, need_position):
        raise NotImplementedError

    @staticmethod
    def remove_tag(txt):
        raise NotImplementedError


class VisionParser(RAGFlowPdfParser):
    def __init__(self, vision_model, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.vision_model = vision_model
        self.outlines = []

    def __images__(self, fnm, zoomin=3, page_from=0, page_to=MAXIMUM_PAGE_NUMBER, callback=None):
        try:
            with sys.modules[LOCK_KEY_pdfplumber]:
                self.pdf = pdfplumber.open(fnm) if isinstance(fnm, str) else pdfplumber.open(BytesIO(fnm))
                self.page_images = [p.to_image(resolution=72 * zoomin).annotated for i, p in enumerate(self.pdf.pages[page_from:page_to])]
                self.total_page = len(self.pdf.pages)
        except Exception:
            self.page_images = None
            self.total_page = 0
            logging.exception("VisionParser __images__")

    def __call__(self, filename, from_page=0, to_page=MAXIMUM_PAGE_NUMBER, **kwargs):
        callback = kwargs.get("callback") or (lambda prog, msg: None)
        zoomin = kwargs.get("zoomin", 3)
        self.__images__(fnm=filename, zoomin=zoomin, page_from=from_page, page_to=to_page, callback=callback)

        total_pdf_pages = self.total_page

        start_page = max(0, from_page)
        end_page = min(to_page, total_pdf_pages)

        all_docs = []

        # If `__images__` failed (pdfplumber exception path), `self.page_images`
        # is None and the loop below silently returns no chunks. Surface the
        # failure through the callback so the user sees a useful message in
        # the task log instead of a generic "No chunk built from …".
        if self.page_images is None:
            callback(-1, "VisionParser could not rasterize the PDF page batch; check server logs for the pdfplumber exception.")
            return all_docs, []

        for idx, img_binary in enumerate(self.page_images):
            pdf_page_num = from_page + idx  # 0-based
            if pdf_page_num < start_page or pdf_page_num >= end_page:
                continue

            from rag.app.picture import vision_llm_chunk as picture_vision_llm_chunk

            text = picture_vision_llm_chunk(
                binary=img_binary,
                vision_model=self.vision_model,
                prompt=vision_llm_describe_prompt(page=pdf_page_num + 1),
                callback=callback,
            )

            if kwargs.get("callback"):
                kwargs["callback"](idx * 1.0 / len(self.page_images), f"Processed: {idx + 1}/{len(self.page_images)}")

            if text:
                width, height = self.page_images[idx].size
                all_docs.append((text, f"@@{pdf_page_num + 1}\t{0.0:.1f}\t{width / zoomin:.1f}\t{0.0:.1f}\t{height / zoomin:.1f}##"))

        # If the vision model produced no output for any page in the batch,
        # log a clear warning so the user can distinguish "no images on these
        # pages" from "vision model returned nothing for the configured
        # layout_recognizer". This is the silent-failure mode behind #17173
        # (qwen-VL returning empty for page batches 2+ while the first batch
        # succeeded).
        if not all_docs and self.page_images:
            callback(
                1.0,
                f"VisionParser processed {len(self.page_images)} page(s) "
                f"(pages {from_page + 1}-{min(to_page, total_pdf_pages)}) but the vision model returned no text. "
                f"Check the configured layout_recognizer model configuration.",
            )
        return all_docs, []


if __name__ == "__main__":
    pass
