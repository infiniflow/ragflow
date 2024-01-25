# -*- coding: utf-8 -*-
import fitz
import xgboost as xgb
from io import BytesIO
import torch
import re
import pdfplumber
import logging
from PIL import Image
import numpy as np
from rag.nlp import huqie
from collections import Counter
from copy import deepcopy
from rag.cv.table_recognize import TableTransformer
from rag.cv.ppdetection import PPDet
from huggingface_hub import hf_hub_download
logging.getLogger("pdfminer").setLevel(logging.WARNING)


class HuParser:
    def __init__(self):
        from paddleocr import PaddleOCR
        logging.getLogger("ppocr").setLevel(logging.ERROR)
        self.ocr = PaddleOCR(use_angle_cls=False, lang="ch")
        self.layouter = PPDet()
        self.tbl_det = TableTransformer()

        self.updown_cnt_mdl = xgb.Booster()
        if torch.cuda.is_available():
            self.updown_cnt_mdl.set_param({"device": "cuda"})
        self.updown_cnt_mdl.load_model(hf_hub_download(repo_id="InfiniFlow/text_concat_xgb_v1.0",
                                                       filename="updown_concat_xgb.model"))
        """
        If you have trouble downloading HuggingFace models, -_^ this might help!!

        For Linux:
        export HF_ENDPOINT=https://hf-mirror.com

        For Windows:
        Good luck
        ^_-

        """

    def __char_width(self, c):
        return (c["x1"] - c["x0"]) // len(c["text"])

    def __height(self, c):
        return c["bottom"] - c["top"]

    def _x_dis(self, a, b):
        return min(abs(a["x1"] - b["x0"]), abs(a["x0"] - b["x1"]),
                   abs(a["x0"] + a["x1"] - b["x0"] - b["x1"]) / 2)

    def _y_dis(
            self, a, b):
        return (
            b["top"] + b["bottom"] - a["top"] - a["bottom"]) / 2

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
        tks_down = huqie.qie(down["text"][:LEN]).split(" ")
        tks_up = huqie.qie(up["text"][-LEN:]).split(" ")
        tks_all = up["text"][-LEN:].strip() \
            + (" " if re.match(r"[a-zA-Z0-9]+",
                               up["text"][-1] + down["text"][0]) else "") \
            + down["text"][:LEN].strip()
        tks_all = huqie.qie(tks_all).split(" ")
        fea = [
            up.get("R", -1) == down.get("R", -1),
            y_dis / h,
            down["page_number"] - up["page_number"],
            up["layout_type"] == down["layout_type"],
            up["layout_type"] == "text",
            down["layout_type"] == "text",
            up["layout_type"] == "table",
            down["layout_type"] == "table",
            True if re.search(
                r"([。？！；!?;+)）]|[a-z]\.)$",
                up["text"]) else False,
            True if re.search(r"[，：‘“、0-9（+-]$", up["text"]) else False,
            True if re.search(
                r"(^.?[/,?;:\]，。；：’”？！》】）-])",
                down["text"]) else False,
            True if re.match(r"[\(（][^\(\)（）]+[）\)]$", up["text"]) else False,
            True if re.search(r"[，,][^。.]+$", up["text"]) else False,
            True if re.search(r"[，,][^。.]+$", up["text"]) else False,
            True if re.search(r"[\(（][^\)）]+$", up["text"])
            and re.search(r"[\)）]", down["text"]) else False,
            self._match_proj(down),
            True if re.match(r"[A-Z]", down["text"]) else False,
            True if re.match(r"[A-Z]", up["text"][-1]) else False,
            True if re.match(r"[a-z0-9]", up["text"][-1]) else False,
            True if re.match(r"[0-9.%,-]+$", down["text"]) else False,
            up["text"].strip()[-2:] == down["text"].strip()[-2:] if len(up["text"].strip()
                                                                        ) > 1 and len(
                down["text"].strip()) > 1 else False,
            up["x0"] > down["x1"],
            abs(self.__height(up) - self.__height(down)) / min(self.__height(up),
                                                               self.__height(down)),
            self._x_dis(up, down) / max(w, 0.000001),
            (len(up["text"]) - len(down["text"])) /
            max(len(up["text"]), len(down["text"])),
            len(tks_all) - len(tks_up) - len(tks_down),
            len(tks_down) - len(tks_up),
            tks_down[-1] == tks_up[-1],
            max(down["in_row"], up["in_row"]),
            abs(down["in_row"] - up["in_row"]),
            len(tks_down) == 1 and huqie.tag(tks_down[0]).find("n") >= 0,
            len(tks_up) == 1 and huqie.tag(tks_up[0]).find("n") >= 0
        ]
        return fea

    @staticmethod
    def sort_Y_firstly(arr, threashold):
        # sort using y1 first and then x1
        arr = sorted(arr, key=lambda r: (r["top"], r["x0"]))
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                # restore the order using th
                if abs(arr[j + 1]["top"] - arr[j]["top"]) < threashold \
                        and arr[j + 1]["x0"] < arr[j]["x0"]:
                    tmp = deepcopy(arr[j])
                    arr[j] = deepcopy(arr[j + 1])
                    arr[j + 1] = deepcopy(tmp)
        return arr

    @staticmethod
    def sort_R_firstly(arr, thr=0):
        # sort using y1 first and then x1
        # sorted(arr, key=lambda r: (r["top"], r["x0"]))
        arr = HuParser.sort_Y_firstly(arr, thr)
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                if "R" not in arr[j] or "R" not in arr[j + 1]:
                    continue
                if arr[j + 1]["R"] < arr[j]["R"] \
                        or (
                        arr[j + 1]["R"] == arr[j]["R"]
                        and arr[j + 1]["x0"] < arr[j]["x0"]
                ):
                    tmp = arr[j]
                    arr[j] = arr[j + 1]
                    arr[j + 1] = tmp
        return arr

    @staticmethod
    def sort_X_firstly(arr, threashold, copy=True):
        # sort using y1 first and then x1
        arr = sorted(arr, key=lambda r: (r["x0"], r["top"]))
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                # restore the order using th
                if abs(arr[j + 1]["x0"] - arr[j]["x0"]) < threashold \
                        and arr[j + 1]["top"] < arr[j]["top"]:
                    tmp = deepcopy(arr[j]) if copy else arr[j]
                    arr[j] = deepcopy(arr[j + 1]) if copy else arr[j + 1]
                    arr[j + 1] = deepcopy(tmp) if copy else tmp
        return arr

    @staticmethod
    def sort_C_firstly(arr, thr=0):
        # sort using y1 first and then x1
        # sorted(arr, key=lambda r: (r["x0"], r["top"]))
        arr = HuParser.sort_X_firstly(arr, thr)
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                # restore the order using th
                if "C" not in arr[j] or "C" not in arr[j + 1]:
                    continue
                if arr[j + 1]["C"] < arr[j]["C"] \
                        or (
                        arr[j + 1]["C"] == arr[j]["C"]
                        and arr[j + 1]["top"] < arr[j]["top"]
                ):
                    tmp = arr[j]
                    arr[j] = arr[j + 1]
                    arr[j + 1] = tmp
        return arr

        return sorted(arr, key=lambda r: (r.get("C", r["x0"]), r["top"]))

    def _has_color(self, o):
        if o.get("ncs", "") == "DeviceGray":
            if o["stroking_color"] and o["stroking_color"][0] == 1 and o["non_stroking_color"] and \
                    o["non_stroking_color"][0] == 1:
                if re.match(r"[a-zT_\[\]\(\)-]+", o.get("text", "")):
                    return False
        return True

    def __overlapped_area(self, a, b, ratio=True):
        tp, btm, x0, x1 = a["top"], a["bottom"], a["x0"], a["x1"]
        if b["x0"] > x1 or b["x1"] < x0:
            return 0
        if b["bottom"] < tp or b["top"] > btm:
            return 0
        x0_ = max(b["x0"], x0)
        x1_ = min(b["x1"], x1)
        assert x0_ <= x1_, "Fuckedup! T:{},B:{},X0:{},X1:{} ==> {}".format(
            tp, btm, x0, x1, b)
        tp_ = max(b["top"], tp)
        btm_ = min(b["bottom"], btm)
        assert tp_ <= btm_, "Fuckedup! T:{},B:{},X0:{},X1:{} => {}".format(
            tp, btm, x0, x1, b)
        ov = (btm_ - tp_) * (x1_ - x0_) if x1 - \
            x0 != 0 and btm - tp != 0 else 0
        if ov > 0 and ratio:
            ov /= (x1 - x0) * (btm - tp)
        return ov

    def __find_overlapped_with_threashold(self, box, boxes, thr=0.3):
        if not boxes:
            return
        max_overlaped_i, max_overlaped, _max_overlaped = None, thr, 0
        s, e = 0, len(boxes)
        for i in range(s, e):
            ov = self.__overlapped_area(box, boxes[i])
            _ov = self.__overlapped_area(boxes[i], box)
            if (ov, _ov) < (max_overlaped, _max_overlaped):
                continue
            max_overlaped_i = i
            max_overlaped = ov
            _max_overlaped = _ov

        return max_overlaped_i

    def __find_overlapped(self, box, boxes_sorted_by_y, naive=False):
        if not boxes_sorted_by_y:
            return
        bxs = boxes_sorted_by_y
        s, e, ii = 0, len(bxs), 0
        while s < e and not naive:
            ii = (e + s) // 2
            pv = bxs[ii]
            if box["bottom"] < pv["top"]:
                e = ii
                continue
            if box["top"] > pv["bottom"]:
                s = ii + 1
                continue
            break
        while s < ii:
            if box["top"] > bxs[s]["bottom"]:
                s += 1
            break
        while e - 1 > ii:
            if box["bottom"] < bxs[e - 1]["top"]:
                e -= 1
            break

        max_overlaped_i, max_overlaped = None, 0
        for i in range(s, e):
            ov = self.__overlapped_area(bxs[i], box)
            if ov <= max_overlaped:
                continue
            max_overlaped_i = i
            max_overlaped = ov

        return max_overlaped_i

    def _is_garbage(self, b):
        patt = [r"^•+$", r"(版权归©|免责条款|地址[:：])", r"\.{3,}", "^[0-9]{1,2} / ?[0-9]{1,2}$",
                r"^[0-9]{1,2} of [0-9]{1,2}$", "^http://[^ ]{12,}",
                "(资料|数据)来源[:：]", "[0-9a-z._-]+@[a-z0-9-]+\\.[a-z]{2,3}",
                "\\(cid *: *[0-9]+ *\\)"
                ]
        return any([re.search(p, b["text"]) for p in patt])

    def __layouts_cleanup(self, boxes, layouts, far=2, thr=0.7):
        def notOverlapped(a, b):
            return any([a["x1"] < b["x0"],
                        a["x0"] > b["x1"],
                        a["bottom"] < b["top"],
                        a["top"] > b["bottom"]])

        i = 0
        while i + 1 < len(layouts):
            j = i + 1
            while j < min(i + far, len(layouts)) \
                    and (layouts[i].get("type", "") != layouts[j].get("type", "")
                         or notOverlapped(layouts[i], layouts[j])):
                j += 1
            if j >= min(i + far, len(layouts)):
                i += 1
                continue
            if self.__overlapped_area(layouts[i], layouts[j]) < thr \
                    and self.__overlapped_area(layouts[j], layouts[i]) < thr:
                i += 1
                continue

            if layouts[i].get("score") and layouts[j].get("score"):
                if layouts[i]["score"] > layouts[j]["score"]:
                    layouts.pop(j)
                else:
                    layouts.pop(i)
                continue

            area_i, area_i_1 = 0, 0
            for b in boxes:
                if not notOverlapped(b, layouts[i]):
                    area_i += self.__overlapped_area(b, layouts[i], False)
                if not notOverlapped(b, layouts[j]):
                    area_i_1 += self.__overlapped_area(b, layouts[j], False)

            if area_i > area_i_1:
                layouts.pop(j)
            else:
                layouts.pop(i)

        return layouts

    def __table_paddle(self, images):
        tbls = self.tbl_det([img for img in images], threshold=0.5)
        res = []
        # align left&right for rows, align top&bottom for columns
        for tbl in tbls:
            lts = [{"label": b["type"],
                    "score": b["score"],
                    "x0": b["bbox"][0], "x1": b["bbox"][2],
                    "top": b["bbox"][1], "bottom": b["bbox"][-1]
                    } for b in tbl]
            if not lts:
                continue

            left = [b["x0"] for b in lts if b["label"].find(
                "row") > 0 or b["label"].find("header") > 0]
            right = [b["x1"] for b in lts if b["label"].find(
                "row") > 0 or b["label"].find("header") > 0]
            if not left:
                continue
            left = np.median(left) if len(left) > 4 else np.min(left)
            right = np.median(right) if len(right) > 4 else np.max(right)
            for b in lts:
                if b["label"].find("row") > 0 or b["label"].find("header") > 0:
                    if b["x0"] > left:
                        b["x0"] = left
                    if b["x1"] < right:
                        b["x1"] = right

            top = [b["top"] for b in lts if b["label"] == "table column"]
            bottom = [b["bottom"] for b in lts if b["label"] == "table column"]
            if not top:
                res.append(lts)
                continue
            top = np.median(top) if len(top) > 4 else np.min(top)
            bottom = np.median(bottom) if len(bottom) > 4 else np.max(bottom)
            for b in lts:
                if b["label"] == "table column":
                    if b["top"] > top:
                        b["top"] = top
                    if b["bottom"] < bottom:
                        b["bottom"] = bottom

            res.append(lts)
        return res

    def _table_transformer_job(self, ZM):
        logging.info("Table processing...")
        imgs, pos = [], []
        tbcnt = [0]
        MARGIN = 10
        self.tb_cpns = []
        assert len(self.page_layout) == len(self.page_images)
        for p, tbls in enumerate(self.page_layout):  # for page
            tbls = [f for f in tbls if f["type"] == "table"]
            tbcnt.append(len(tbls))
            if not tbls:
                continue
            for tb in tbls:  # for table
                left, top, right, bott = tb["x0"] - MARGIN, tb["top"] - MARGIN, \
                    tb["x1"] + MARGIN, tb["bottom"] + MARGIN
                left *= ZM
                top *= ZM
                right *= ZM
                bott *= ZM
                pos.append((left, top))
                imgs.append(self.page_images[p].crop((left, top, right, bott)))

        assert len(self.page_images) == len(tbcnt) - 1
        if not imgs:
            return
        recos = self.__table_paddle(imgs)
        tbcnt = np.cumsum(tbcnt)
        for i in range(len(tbcnt) - 1):  # for page
            pg = []
            for j, tb_items in enumerate(
                    recos[tbcnt[i]: tbcnt[i + 1]]):  # for table
                poss = pos[tbcnt[i]: tbcnt[i + 1]]
                for it in tb_items:  # for table components
                    it["x0"] = (it["x0"] + poss[j][0])
                    it["x1"] = (it["x1"] + poss[j][0])
                    it["top"] = (it["top"] + poss[j][1])
                    it["bottom"] = (it["bottom"] + poss[j][1])
                    for n in ["x0", "x1", "top", "bottom"]:
                        it[n] /= ZM
                    it["top"] += self.page_cum_height[i]
                    it["bottom"] += self.page_cum_height[i]
                    it["pn"] = i
                    it["layoutno"] = j
                    pg.append(it)
            self.tb_cpns.extend(pg)

        def gather(kwd, fzy=10, ption=0.6):
            eles = self.sort_Y_firstly(
                [r for r in self.tb_cpns if re.match(kwd, r["label"])], fzy)
            eles = self.__layouts_cleanup(self.boxes, eles, 5, ption)
            return self.sort_Y_firstly(eles, 0)

        # add R,H,C,SP tag to boxes within table layout
        headers = gather(r".*header$")
        rows = gather(r".* (row|header)")
        spans = gather(r".*spanning")
        clmns = sorted([r for r in self.tb_cpns if re.match(
            r"table column$", r["label"])], key=lambda x: (x["pn"], x["layoutno"], x["x0"]))
        clmns = self.__layouts_cleanup(self.boxes, clmns, 5, 0.5)
        for b in self.boxes:
            if b.get("layout_type", "") != "table":
                continue
            ii = self.__find_overlapped_with_threashold(b, rows, thr=0.3)
            if ii is not None:
                b["R"] = ii
                b["R_top"] = rows[ii]["top"]
                b["R_bott"] = rows[ii]["bottom"]

            ii = self.__find_overlapped_with_threashold(b, headers, thr=0.3)
            if ii is not None:
                b["H_top"] = headers[ii]["top"]
                b["H_bott"] = headers[ii]["bottom"]
                b["H_left"] = headers[ii]["x0"]
                b["H_right"] = headers[ii]["x1"]
                b["H"] = ii

            ii = self.__find_overlapped_with_threashold(b, clmns, thr=0.3)
            if ii is not None:
                b["C"] = ii
                b["C_left"] = clmns[ii]["x0"]
                b["C_right"] = clmns[ii]["x1"]

            ii = self.__find_overlapped_with_threashold(b, spans, thr=0.3)
            if ii is not None:
                b["H_top"] = spans[ii]["top"]
                b["H_bott"] = spans[ii]["bottom"]
                b["H_left"] = spans[ii]["x0"]
                b["H_right"] = spans[ii]["x1"]
                b["SP"] = ii

    def __ocr_paddle(self, pagenum, img, chars, ZM=3):
        bxs = self.ocr.ocr(np.array(img), cls=True)[0]
        if not bxs:
            self.boxes.append([])
            return
        bxs = [(line[0], line[1][0]) for line in bxs]
        bxs = self.sort_Y_firstly(
            [{"x0": b[0][0] / ZM, "x1": b[1][0] / ZM,
              "top": b[0][1] / ZM, "text": "", "txt": t,
              "bottom": b[-1][1] / ZM,
              "page_number": pagenum} for b, t in bxs if b[0][0] <= b[1][0] and b[0][1] <= b[-1][1]],
            self.mean_height[-1] / 3
        )

        # merge chars in the same rect
        for c in self.sort_X_firstly(chars, self.mean_width[pagenum - 1] // 4):
            ii = self.__find_overlapped(c, bxs)
            if ii is None:
                self.lefted_chars.append(c)
                continue
            ch = c["bottom"] - c["top"]
            bh = bxs[ii]["bottom"] - bxs[ii]["top"]
            if abs(ch - bh) / max(ch, bh) >= 0.7:
                self.lefted_chars.append(c)
                continue
            bxs[ii]["text"] += c["text"]

        for b in bxs:
            if not b["text"]:
                b["text"] = b["txt"]
            del b["txt"]
        if self.mean_height[-1] == 0:
            self.mean_height[-1] = np.median([b["bottom"] - b["top"]
                                              for b in bxs])

        self.boxes.append(bxs)

    def _layouts_paddle(self, ZM):
        assert len(self.page_images) == len(self.boxes)
        # Tag layout type
        boxes = []
        layouts = self.layouter([np.array(img) for img in self.page_images])
        assert len(self.page_images) == len(layouts)
        for pn, lts in enumerate(layouts):
            bxs = self.boxes[pn]
            lts = [{"type": b["type"],
                    "score": float(b["score"]),
                    "x0": b["bbox"][0] / ZM, "x1": b["bbox"][2] / ZM,
                    "top": b["bbox"][1] / ZM, "bottom": b["bbox"][-1] / ZM,
                    "page_number": pn,
                    } for b in lts]
            lts = self.sort_Y_firstly(lts, self.mean_height[pn] / 2)
            lts = self.__layouts_cleanup(bxs, lts)
            self.page_layout.append(lts)

            # Tag layout type, layouts are ready
            def findLayout(ty):
                nonlocal bxs, lts
                lts_ = [lt for lt in lts if lt["type"] == ty]
                i = 0
                while i < len(bxs):
                    if bxs[i].get("layout_type"):
                        i += 1
                        continue
                    if self._is_garbage(bxs[i]):
                        logging.debug("GARBAGE: " + bxs[i]["text"])
                        bxs.pop(i)
                        continue

                    ii = self.__find_overlapped_with_threashold(bxs[i], lts_,
                                                                thr=0.4)
                    if ii is None:  # belong to nothing
                        bxs[i]["layout_type"] = ""
                        i += 1
                        continue
                    lts_[ii]["visited"] = True
                    if lts_[ii]["type"] in ["footer", "header", "reference"]:
                        if lts_[ii]["type"] not in self.garbages:
                            self.garbages[lts_[ii]["type"]] = []
                        self.garbages[lts_[ii]["type"]].append(bxs[i]["text"])
                        logging.debug("GARBAGE: " + bxs[i]["text"])
                        bxs.pop(i)
                        continue

                    bxs[i]["layoutno"] = f"{ty}-{ii}"
                    bxs[i]["layout_type"] = lts_[ii]["type"]
                    i += 1

            for lt in ["footer", "header", "reference", "figure caption",
                       "table caption", "title", "text", "table", "figure"]:
                findLayout(lt)

            # add box to figure layouts which has not text box
            for i, lt in enumerate(
                    [lt for lt in lts if lt["type"] == "figure"]):
                if lt.get("visited"):
                    continue
                lt = deepcopy(lt)
                del lt["type"]
                lt["text"] = ""
                lt["layout_type"] = "figure"
                lt["layoutno"] = f"figure-{i}"
                bxs.append(lt)

            boxes.extend(bxs)

        self.boxes = boxes

        garbage = set()
        for k in self.garbages.keys():
            self.garbages[k] = Counter(self.garbages[k])
            for g, c in self.garbages[k].items():
                if c > 1:
                    garbage.add(g)

        logging.debug("GARBAGE:" + ",".join(garbage))
        self.boxes = [b for b in self.boxes if b["text"].strip() not in garbage]

        # cumlative Y
        for i in range(len(self.boxes)):
            self.boxes[i]["top"] += \
                self.page_cum_height[self.boxes[i]["page_number"] - 1]
            self.boxes[i]["bottom"] += \
                self.page_cum_height[self.boxes[i]["page_number"] - 1]

    def _text_merge(self):
        # merge adjusted boxes
        bxs = self.boxes

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
            if b.get("layoutno", "0") != b_.get("layoutno", "1"):
                i += 1
                continue

            dis_thr = 1
            dis = b["x1"] - b_["x0"]
            if b.get("layout_type", "") != "text" or b_.get(
                    "layout_type", "") != "text":
                if end_with(b, "，") or start_with(b_, "（，"):
                    dis_thr = -8
                else:
                    i += 1
                    continue

            if abs(self._y_dis(b, b_)) < self.mean_height[bxs[i]["page_number"] - 1] / 5 \
                    and dis >= dis_thr and b["x1"] < b_["x1"]:
                # merge
                bxs[i]["x1"] = b_["x1"]
                bxs[i]["top"] = (b["top"] + b_["top"]) / 2
                bxs[i]["bottom"] = (b["bottom"] + b_["bottom"]) / 2
                bxs[i]["text"] += b_["text"]
                bxs.pop(i + 1)
                continue
            i += 1
        self.boxes = bxs

    def _concat_downward(self):
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

                    if up.get("R", "") != down.get(
                            "R", "") and up["text"][-1] != "，":
                        i += 1
                        continue

                    if re.match(r"[0-9]{2,3}/[0-9]{3}$", up["text"]) \
                            or re.match(r"[0-9]{2,3}/[0-9]{3}$", down["text"]):
                        i += 1
                        continue

                    if not down["text"].strip():
                        i += 1
                        continue

                    if up["x1"] < down["x0"] - 10 * \
                            mw or up["x0"] > down["x1"] + 10 * mw:
                        i += 1
                        continue

                    if i - dp < 5 and up.get("layout_type") == "text":
                        if up.get("layoutno", "1") == down.get(
                                "layoutno", "2"):
                            dfs(down, i + 1)
                            boxes.pop(i)
                            return
                        i += 1
                        continue

                    fea = self._updown_concat_features(up, down)
                    if self.updown_cnt_mdl.predict(
                            xgb.DMatrix([fea]))[0] <= 0.5:
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
                if t["text"] and re.match(
                        r"[0-9\.a-zA-Z]+$", t["text"][-1] + c["text"][-1]):
                    t["text"] += " "
                t["text"] += c["text"]
                t["x0"] = min(t["x0"], c["x0"])
                t["x1"] = max(t["x1"], c["x1"])
                t["page_number"] = min(t["page_number"], c["page_number"])
                t["bottom"] = c["bottom"]
                if not t["layout_type"] \
                        and c["layout_type"]:
                    t["layout_type"] = c["layout_type"]
            boxes.append(t)

        self.boxes = self.sort_Y_firstly(boxes, 0)

    def __filter_forpages(self):
        if not self.boxes:
            return
        to = min(7, len(self.page_images) // 5)
        pg_hits = [0 for _ in range(to)]

        def possible(c):
            if c.get("layout_type", "") == "reference":
                return True
            if c["bottom"] - c["top"] >= 2 * \
                    self.mean_height[c["page_number"] - 1]:
                return False
            if c["text"].find("....") >= 0 \
                    or (c["x1"] - c["x0"] > 250 and re.search(r"[0-9]+$",
                                                              c["text"].strip())):
                return True
            return self.is_caption(c) and re.search(
                r"[0-9]+$", c["text"].strip())

        for c in self.boxes:
            if c["page_number"] >= to:
                break
            if possible(c):
                pg_hits[c["page_number"] - 1] += 1

        st, ed = -1, -1
        for i in range(len(self.boxes)):
            c = self.boxes[i]
            if c["page_number"] >= to:
                break
            if pg_hits[c["page_number"] - 1] >= 3 and possible(c):
                if st < 0:
                    st = i
                else:
                    ed = i
        for _ in range(st, ed + 1):
            self.boxes.pop(st)

    def _blockType(self, b):
        patt = [
            ("^(20|19)[0-9]{2}[年/-][0-9]{1,2}[月/-][0-9]{1,2}日*$", "Dt"),
            (r"^(20|19)[0-9]{2}年$", "Dt"),
            (r"^(20|19)[0-9]{2}[年-][0-9]{1,2}月*$", "Dt"),
            ("^[0-9]{1,2}[月-][0-9]{1,2}日*$", "Dt"),
            (r"^第*[一二三四1-4]季度$", "Dt"),
            (r"^(20|19)[0-9]{2}年*[一二三四1-4]季度$", "Dt"),
            (r"^(20|19)[0-9]{2}[ABCDE]$", "Dt"),
            ("^[0-9.,+%/ -]+$", "Nu"),
            (r"^[0-9A-Z/\._~-]+$", "Ca"),
            (r"^[A-Z]*[a-z' -]+$", "En"),
            (r"^[0-9.,+-]+[0-9A-Za-z/$￥%<>（）()' -]+$", "NE"),
            (r"^.{1}$", "Sg")
        ]
        for p, n in patt:
            if re.search(p, b["text"].strip()):
                return n
        tks = [t for t in huqie.qie(b["text"]).split(" ") if len(t) > 1]
        if len(tks) > 3:
            if len(tks) < 12:
                return "Tx"
            else:
                return "Lx"

        if len(tks) == 1 and huqie.tag(tks[0]) == "nr":
            return "Nr"

        return "Ot"

    def __cal_spans(self, boxes, rows, cols, tbl, html=True):
        # caculate span
        clft = [np.mean([c.get("C_left", c["x0"]) for c in cln])
                for cln in cols]
        crgt = [np.mean([c.get("C_right", c["x1"]) for c in cln])
                for cln in cols]
        rtop = [np.mean([c.get("R_top", c["top"]) for c in row])
                for row in rows]
        rbtm = [np.mean([c.get("R_btm", c["bottom"])
                         for c in row]) for row in rows]
        for b in boxes:
            if "SP" not in b:
                continue
            b["colspan"] = [b["cn"]]
            b["rowspan"] = [b["rn"]]
            # col span
            for j in range(0, len(clft)):
                if j == b["cn"]:
                    continue
                if clft[j] + (crgt[j] - clft[j]) / 2 < b["H_left"]:
                    continue
                if crgt[j] - (crgt[j] - clft[j]) / 2 > b["H_right"]:
                    continue
                b["colspan"].append(j)
            # row span
            for j in range(0, len(rtop)):
                if j == b["rn"]:
                    continue
                if rtop[j] + (rbtm[j] - rtop[j]) / 2 < b["H_top"]:
                    continue
                if rbtm[j] - (rbtm[j] - rtop[j]) / 2 > b["H_bott"]:
                    continue
                b["rowspan"].append(j)

        def join(arr):
            if not arr:
                return ""
            return "".join([t["text"] for t in arr])

        # rm the spaning cells
        for i in range(len(tbl)):
            for j, arr in enumerate(tbl[i]):
                if not arr:
                    continue
                if all(["rowspan" not in a and "colspan" not in a for a in arr]):
                    continue
                rowspan, colspan = [], []
                for a in arr:
                    if isinstance(a.get("rowspan", 0), list):
                        rowspan.extend(a["rowspan"])
                    if isinstance(a.get("colspan", 0), list):
                        colspan.extend(a["colspan"])
                rowspan, colspan = set(rowspan), set(colspan)
                if len(rowspan) < 2 and len(colspan) < 2:
                    for a in arr:
                        if "rowspan" in a:
                            del a["rowspan"]
                        if "colspan" in a:
                            del a["colspan"]
                    continue
                rowspan, colspan = sorted(rowspan), sorted(colspan)
                rowspan = list(range(rowspan[0], rowspan[-1] + 1))
                colspan = list(range(colspan[0], colspan[-1] + 1))
                assert i in rowspan, rowspan
                assert j in colspan, colspan
                arr = []
                for r in rowspan:
                    for c in colspan:
                        arr_txt = join(arr)
                        if tbl[r][c] and join(tbl[r][c]) != arr_txt:
                            arr.extend(tbl[r][c])
                        tbl[r][c] = None if html else arr
                for a in arr:
                    if len(rowspan) > 1:
                        a["rowspan"] = len(rowspan)
                    elif "rowspan" in a:
                        del a["rowspan"]
                    if len(colspan) > 1:
                        a["colspan"] = len(colspan)
                    elif "colspan" in a:
                        del a["colspan"]
                tbl[rowspan[0]][colspan[0]] = arr

        return tbl

    def __construct_table(self, boxes, html=False):
        cap = ""
        i = 0
        while i < len(boxes):
            if self.is_caption(boxes[i]):
                cap += boxes[i]["text"]
                boxes.pop(i)
                i -= 1
            i += 1

        if not boxes:
            return []
        for b in boxes:
            b["btype"] = self._blockType(b)
        max_type = Counter([b["btype"] for b in boxes]).items()
        max_type = max(max_type, key=lambda x: x[1])[0] if max_type else ""
        logging.debug("MAXTYPE: " + max_type)

        rowh = [b["R_bott"] - b["R_top"] for b in boxes if "R" in b]
        rowh = np.min(rowh) if rowh else 0
        # boxes = self.sort_Y_firstly(boxes, rowh/5)
        boxes = self.sort_R_firstly(boxes, rowh / 2)
        boxes[0]["rn"] = 0
        rows = [[boxes[0]]]
        btm = boxes[0]["bottom"]
        for b in boxes[1:]:
            b["rn"] = len(rows) - 1
            lst_r = rows[-1]
            if lst_r[-1].get("R", "") != b.get("R", "") \
                    or (b["top"] >= btm - 3 and lst_r[-1].get("R", "-1") != b.get("R", "-2")
                        ):  # new row
                btm = b["bottom"]
                b["rn"] += 1
                rows.append([b])
                continue
            btm = (btm + b["bottom"]) / 2.
            rows[-1].append(b)

        colwm = [b["C_right"] - b["C_left"] for b in boxes if "C" in b]
        colwm = np.min(colwm) if colwm else 0
        crosspage = len(set([b["page_number"] for b in boxes])) > 1
        if crosspage:
            boxes = self.sort_X_firstly(boxes, colwm / 2, False)
        else:
            boxes = self.sort_C_firstly(boxes, colwm / 2)
        boxes[0]["cn"] = 0
        cols = [[boxes[0]]]
        right = boxes[0]["x1"]
        for b in boxes[1:]:
            b["cn"] = len(cols) - 1
            lst_c = cols[-1]
            if (int(b.get("C", "1")) - int(lst_c[-1].get("C", "1")) == 1 and b["page_number"] == lst_c[-1][
                "page_number"]) \
                    or (b["x0"] >= right and lst_c[-1].get("C", "-1") != b.get("C", "-2")):  # new col
                right = b["x1"]
                b["cn"] += 1
                cols.append([b])
                continue
            right = (right + b["x1"]) / 2.
            cols[-1].append(b)

        tbl = [[[] for _ in range(len(cols))] for _ in range(len(rows))]
        for b in boxes:
            tbl[b["rn"]][b["cn"]].append(b)

        if len(rows) >= 4:
            # remove single in column
            j = 0
            while j < len(tbl[0]):
                e, ii = 0, 0
                for i in range(len(tbl)):
                    if tbl[i][j]:
                        e += 1
                        ii = i
                    if e > 1:
                        break
                if e > 1:
                    j += 1
                    continue
                f = (j > 0 and tbl[ii][j - 1] and tbl[ii]
                     [j - 1][0].get("text")) or j == 0
                ff = (j + 1 < len(tbl[ii]) and tbl[ii][j + 1] and tbl[ii]
                      [j + 1][0].get("text")) or j + 1 >= len(tbl[ii])
                if f and ff:
                    j += 1
                    continue
                bx = tbl[ii][j][0]
                logging.debug("Relocate column single: " + bx["text"])
                # j column only has one value
                left, right = 100000, 100000
                if j > 0 and not f:
                    for i in range(len(tbl)):
                        if tbl[i][j - 1]:
                            left = min(left, np.min(
                                [bx["x0"] - a["x1"] for a in tbl[i][j - 1]]))
                if j + 1 < len(tbl[0]) and not ff:
                    for i in range(len(tbl)):
                        if tbl[i][j + 1]:
                            right = min(right, np.min(
                                [a["x0"] - bx["x1"] for a in tbl[i][j + 1]]))
                assert left < 100000 or right < 100000
                if left < right:
                    for jj in range(j, len(tbl[0])):
                        for i in range(len(tbl)):
                            for a in tbl[i][jj]:
                                a["cn"] -= 1
                    if tbl[ii][j - 1]:
                        tbl[ii][j - 1].extend(tbl[ii][j])
                    else:
                        tbl[ii][j - 1] = tbl[ii][j]
                    for i in range(len(tbl)):
                        tbl[i].pop(j)

                else:
                    for jj in range(j + 1, len(tbl[0])):
                        for i in range(len(tbl)):
                            for a in tbl[i][jj]:
                                a["cn"] -= 1
                    if tbl[ii][j + 1]:
                        tbl[ii][j + 1].extend(tbl[ii][j])
                    else:
                        tbl[ii][j + 1] = tbl[ii][j]
                    for i in range(len(tbl)):
                        tbl[i].pop(j)
                cols.pop(j)
        assert len(cols) == len(tbl[0]), "Column NO. miss matched: %d vs %d" % (
            len(cols), len(tbl[0]))

        if len(cols) >= 4:
            # remove single in row
            i = 0
            while i < len(tbl):
                e, jj = 0, 0
                for j in range(len(tbl[i])):
                    if tbl[i][j]:
                        e += 1
                        jj = j
                    if e > 1:
                        break
                if e > 1:
                    i += 1
                    continue
                f = (i > 0 and tbl[i - 1][jj] and tbl[i - 1]
                     [jj][0].get("text")) or i == 0
                ff = (i + 1 < len(tbl) and tbl[i + 1][jj] and tbl[i + 1]
                      [jj][0].get("text")) or i + 1 >= len(tbl)
                if f and ff:
                    i += 1
                    continue

                bx = tbl[i][jj][0]
                logging.debug("Relocate row single: " + bx["text"])
                # i row only has one value
                up, down = 100000, 100000
                if i > 0 and not f:
                    for j in range(len(tbl[i - 1])):
                        if tbl[i - 1][j]:
                            up = min(up, np.min(
                                [bx["top"] - a["bottom"] for a in tbl[i - 1][j]]))
                if i + 1 < len(tbl) and not ff:
                    for j in range(len(tbl[i + 1])):
                        if tbl[i + 1][j]:
                            down = min(down, np.min(
                                [a["top"] - bx["bottom"] for a in tbl[i + 1][j]]))
                assert up < 100000 or down < 100000
                if up < down:
                    for ii in range(i, len(tbl)):
                        for j in range(len(tbl[ii])):
                            for a in tbl[ii][j]:
                                a["rn"] -= 1
                    if tbl[i - 1][jj]:
                        tbl[i - 1][jj].extend(tbl[i][jj])
                    else:
                        tbl[i - 1][jj] = tbl[i][jj]
                    tbl.pop(i)

                else:
                    for ii in range(i + 1, len(tbl)):
                        for j in range(len(tbl[ii])):
                            for a in tbl[ii][j]:
                                a["rn"] -= 1
                    if tbl[i + 1][jj]:
                        tbl[i + 1][jj].extend(tbl[i][jj])
                    else:
                        tbl[i + 1][jj] = tbl[i][jj]
                    tbl.pop(i)
                rows.pop(i)

        # which rows are headers
        hdset = set([])
        for i in range(len(tbl)):
            cnt, h = 0, 0
            for j, arr in enumerate(tbl[i]):
                if not arr:
                    continue
                cnt += 1
                if max_type == "Nu" and arr[0]["btype"] == "Nu":
                    continue
                if any([a.get("H") for a in arr]) \
                        or (max_type == "Nu" and arr[0]["btype"] != "Nu"):
                    h += 1
            if h / cnt > 0.5:
                hdset.add(i)

        if html:
            return [self.__html_table(cap, hdset,
                                      self.__cal_spans(boxes, rows,
                                                       cols, tbl, True)
                                      )]

        return self.__desc_table(cap, hdset,
                                 self.__cal_spans(boxes, rows, cols, tbl, False))

    def __html_table(self, cap, hdset, tbl):
        # constrcut HTML
        html = "<table>"
        if cap:
            html += f"<caption>{cap}</caption>"
        for i in range(len(tbl)):
            row = "<tr>"
            txts = []
            for j, arr in enumerate(tbl[i]):
                if arr is None:
                    continue
                if not arr:
                    row += "<td></td>" if i not in hdset else "<th></th>"
                    continue
                txt = ""
                if arr:
                    h = min(np.min([c["bottom"] - c["top"] for c in arr]) / 2,
                            self.mean_height[arr[0]["page_number"] - 1] / 2)
                    txt = "".join([c["text"]
                                   for c in self.sort_Y_firstly(arr, h)])
                txts.append(txt)
                sp = ""
                if arr[0].get("colspan"):
                    sp = "colspan={}".format(arr[0]["colspan"])
                if arr[0].get("rowspan"):
                    sp += " rowspan={}".format(arr[0]["rowspan"])
                if i in hdset:
                    row += f"<th {sp} >" + txt + "</th>"
                else:
                    row += f"<td {sp} >" + txt + "</td>"

            if i in hdset:
                if all([t in hdset for t in txts]):
                    continue
                for t in txts:
                    hdset.add(t)

            if row != "<tr>":
                row += "</tr>"
            else:
                row = ""
            html += "\n" + row
        html += "\n</table>"
        return html

    def __desc_table(self, cap, hdr_rowno, tbl):
        # get text of every colomn in header row to become header text
        clmno = len(tbl[0])
        rowno = len(tbl)
        headers = {}
        hdrset = set()
        lst_hdr = []
        for r in sorted(list(hdr_rowno)):
            headers[r] = ["" for _ in range(clmno)]
            for i in range(clmno):
                if not tbl[r][i]:
                    continue
                txt = "".join([a["text"].strip() for a in tbl[r][i]])
                headers[r][i] = txt
                hdrset.add(txt)
            if all([not t for t in headers[r]]):
                del headers[r]
                hdr_rowno.remove(r)
                continue
            for j in range(clmno):
                if headers[r][j]:
                    continue
                if j >= len(lst_hdr):
                    break
                headers[r][j] = lst_hdr[j]
            lst_hdr = headers[r]
        for i in range(rowno):
            if i not in hdr_rowno:
                continue
            for j in range(i + 1, rowno):
                if j not in hdr_rowno:
                    break
                for k in range(clmno):
                    if not headers[j - 1][k]:
                        continue
                    if headers[j][k].find(headers[j - 1][k]) >= 0:
                        continue
                    if len(headers[j][k]) > len(headers[j - 1][k]):
                        headers[j][k] += ("的" if headers[j][k]
                                          else "") + headers[j - 1][k]
                    else:
                        headers[j][k] = headers[j - 1][k] \
                            + ("的" if headers[j - 1][k] else "") \
                            + headers[j][k]

        logging.debug(
            f">>>>>>>>>>>>>>>>>{cap}：SIZE:{rowno}X{clmno} Header: {hdr_rowno}")
        row_txt = []
        for i in range(rowno):
            if i in hdr_rowno:
                continue
            rtxt = []

            def append(delimer):
                nonlocal rtxt, row_txt
                rtxt = delimer.join(rtxt)
                if row_txt and len(row_txt[-1]) + len(rtxt) < 64:
                    row_txt[-1] += "\n" + rtxt
                else:
                    row_txt.append(rtxt)

            r = 0
            if len(headers.items()):
                _arr = [(i - r, r) for r, _ in headers.items() if r < i]
                if _arr:
                    _, r = min(_arr, key=lambda x: x[0])

            if r not in headers and clmno <= 2:
                for j in range(clmno):
                    if not tbl[i][j]:
                        continue
                    txt = "".join([a["text"].strip() for a in tbl[i][j]])
                    if txt:
                        rtxt.append(txt)
                if rtxt:
                    append("：")
                continue

            for j in range(clmno):
                if not tbl[i][j]:
                    continue
                txt = "".join([a["text"].strip() for a in tbl[i][j]])
                if not txt:
                    continue
                ctt = headers[r][j] if r in headers else ""
                if ctt:
                    ctt += "："
                ctt += txt
                if ctt:
                    rtxt.append(ctt)

            if rtxt:
                row_txt.append("; ".join(rtxt))

        if cap:
            row_txt = [t + f"\t——来自“{cap}”" for t in row_txt]
        return row_txt

    @staticmethod
    def is_caption(bx):
        patt = [
            r"[图表]+[ 0-9:：]{2,}"
        ]
        if any([re.match(p, bx["text"].strip()) for p in patt]) \
                or bx["layout_type"].find("caption") >= 0:
            return True
        return False

    def __extract_table_figure(self, need_image, ZM, return_html):
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
            lout_no = str(self.boxes[i]["page_number"]) + \
                "-" + str(self.boxes[i]["layoutno"])
            if self.is_caption(self.boxes[i]) or self.boxes[i]["layout_type"] in ["table caption", "title",
                                                                                  "figure caption", "reference"]:
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
        tbls = sorted([(k, bxs) for k, bxs in tables.items()],
                      key=lambda x: (x[1][0]["top"], x[1][0]["x0"]))

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
            if not self.is_caption(c):
                i += 1
                continue

            # find the nearest layouts
            def nearest(tbls):
                nonlocal c
                mink = ""
                minv = 1000000000
                for k, bxs in tbls.items():
                    for b in bxs[:10]:
                        if b.get("layout_type", "").find("caption") >= 0:
                            continue
                        y_dis = self._y_dis(c, b)
                        x_dis = self._x_dis(
                            c, b) if not x_overlapped(
                            c, b) else 0
                        dis = y_dis * y_dis + x_dis * x_dis
                        if dis < minv:
                            mink = k
                            minv = dis
                return mink, minv

            tk, tv = nearest(tables)
            fk, fv = nearest(figures)
            if min(tv, fv) > 2000:
                i += 1
                continue
            if tv < fv:
                tables[tk].insert(0, c)
                logging.debug(
                    "TABLE:" +
                    self.boxes[i]["text"] +
                    "; Cap: " +
                    tk)
            else:
                figures[fk].insert(0, c)
                logging.debug(
                    "FIGURE:" +
                    self.boxes[i]["text"] +
                    "; Cap: " +
                    tk)
            self.boxes.pop(i)

        res = []

        def cropout(bxs, ltype):
            nonlocal ZM
            pn = set([b["page_number"] - 1 for b in bxs])
            if len(pn) < 2:
                pn = list(pn)[0]
                ht = self.page_cum_height[pn]
                b = {
                    "x0": np.min([b["x0"] for b in bxs]),
                    "top": np.min([b["top"] for b in bxs]) - ht,
                    "x1": np.max([b["x1"] for b in bxs]),
                    "bottom": np.max([b["bottom"] for b in bxs]) - ht
                }
                louts = [l for l in self.page_layout[pn] if l["type"] == ltype]
                ii = self.__find_overlapped(b, louts, naive=True)
                if ii is not None:
                    b = louts[ii]
                else:
                    logging.warn(
                        f"Missing layout match: {pn + 1},%s" %
                        (bxs[0].get(
                            "layoutno", "")))

                left, top, right, bott = b["x0"], b["top"], b["x1"], b["bottom"]
                return self.page_images[pn] \
                    .crop((left * ZM, top * ZM,
                           right * ZM, bott * ZM))
            pn = {}
            for b in bxs:
                p = b["page_number"] - 1
                if p not in pn:
                    pn[p] = []
                pn[p].append(b)
            pn = sorted(pn.items(), key=lambda x: x[0])
            imgs = [cropout(arr, ltype) for p, arr in pn]
            pic = Image.new("RGB",
                            (int(np.max([i.size[0] for i in imgs])),
                             int(np.sum([m.size[1] for m in imgs]))),
                            (245, 245, 245))
            height = 0
            for img in imgs:
                pic.paste(img, (0, int(height)))
                height += img.size[1]
            return pic

        # crop figure out and add caption
        for k, bxs in figures.items():
            txt = "\n".join(
                [b["text"] for b in bxs
                 if not re.match(r"[0-9a-z.\+%-]", b["text"].strip())
                 and len(b["text"].strip()) >= 4
                 ]
            )
            if not txt:
                continue

            res.append(
                (cropout(
                    bxs,
                    "figure"),
                 [txt] if not return_html else [f"<p>{txt}</p>"]))

        for k, bxs in tables.items():
            if not bxs:
                continue
            res.append((cropout(bxs, "table"),
                        self.__construct_table(bxs, html=return_html)))

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
            (r"[⚫•➢✓]", 12)
        ]:
            if re.match(p, line):
                return j
        return

    def _line_tag(self, bx, ZM):
        pn = [bx["page_number"]]
        top = bx["top"] - self.page_cum_height[pn[0] - 1]
        bott = bx["bottom"] - self.page_cum_height[pn[0] - 1]
        while bott * ZM > self.page_images[pn[-1] - 1].size[1]:
            bott -= self.page_images[pn[-1] - 1].size[1] / ZM
            pn.append(pn[-1] + 1)

        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##" \
            .format("-".join([str(p) for p in pn]),
                    bx["x0"], bx["x1"], top, bott)

    def __filterout_scraps(self, boxes, ZM):

        def width(b):
            return b["x1"] - b["x0"]

        def height(b):
            return b["bottom"] - b["top"]

        def usefull(b):
            if b.get("layout_type"):
                return True
            if width(
                    b) > self.page_images[b["page_number"] - 1].size[0] / ZM / 3:
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
            mj = self.proj_match(
                boxes[0]["text"]) or boxes[0].get(
                "layout_type",
                "") == "title"

            def dfs(line, st):
                nonlocal mh, pw, lines, widths
                lines.append(line)
                widths.append(width(line))
                width_mean = np.mean(widths)
                mmj = self.proj_match(
                    line["text"]) or line.get(
                    "layout_type",
                    "") == "title"
                for i in range(st + 1, min(st + 20, len(boxes))):
                    if (boxes[i]["page_number"] - line["page_number"]) > 0:
                        break
                    if not mmj and self._y_dis(
                            line, boxes[i]) >= 3 * mh and height(line) < 1.5 * mh:
                        break

                    if not usefull(boxes[i]):
                        continue
                    if mmj or \
                            (self._x_dis(boxes[i], line) < pw / 10): \
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
            except Exception as e:
                pass
            boxes.pop(0)
            mw = np.mean(widths)
            if mj or mw / pw >= 0.35 or mw > 200:
                res.append("\n".join([c["text"] + self._line_tag(c, ZM) for c in lines]))
            else:
                logging.debug("REMOVED: " +
                              "<<".join([c["text"] for c in lines]))

        return "\n\n".join(res)

    def __images__(self, fnm, zoomin=3, page_from=0, page_to=299):
        self.lefted_chars = []
        self.mean_height = []
        self.mean_width = []
        self.boxes = []
        self.garbages = {}
        self.page_cum_height = [0]
        self.page_layout = []
        try:
            self.pdf = pdfplumber.open(fnm) if isinstance(fnm, str) else pdfplumber.open(BytesIO(fnm))
            self.page_images = [p.to_image(resolution=72 * zoomin).annotated for i, p in
                                enumerate(self.pdf.pages[page_from:page_to])]
            self.page_chars = [[c for c in self.pdf.pages[i].chars if self._has_color(c)] for i in
                               range(len(self.page_images))]
            self.total_page = len(self.pdf.pages)
        except Exception as e:
            self.pdf = fitz.open(fnm) if isinstance(fnm, str) else fitz.open(stream=fnm, filetype="pdf")
            self.page_images = []
            self.page_chars = []
            mat = fitz.Matrix(zoomin, zoomin)
            self.total_page = len(self.pdf)
            for page in self.pdf[page_from:page_to]:
                pix = page.getPixmap(matrix=mat)
                img = Image.frombytes("RGB", [pix.width, pix.height],
                                      pix.samples)
                self.page_images.append(img)
                self.page_chars.append([])

        logging.info("Images converted.")
        for i, img in enumerate(self.page_images):
            chars = self.page_chars[i]
            self.mean_height.append(
                np.median(sorted([c["height"] for c in chars])) if chars else 0
            )
            self.mean_width.append(
                np.median(sorted([c["width"] for c in chars])) if chars else 8
            )
            self.page_cum_height.append(img.size[1] / zoomin)
            # if i > 0:
            #     if not chars:
            #         self.page_cum_height.append(img.size[1] / zoomin)
            #     else:
            #         self.page_cum_height.append(
            #             np.max([c["bottom"] for c in chars]))
            self.__ocr_paddle(i + 1, img, chars, zoomin)

        self.page_cum_height = np.cumsum(self.page_cum_height)
        assert len(self.page_cum_height) == len(self.page_images)+1

    def __call__(self, fnm, need_image=True, zoomin=3, return_html=False):
        self.__images__(fnm, zoomin)
        self._layouts_paddle(zoomin)
        self._table_transformer_job(zoomin)
        self._text_merge()
        self._concat_downward()
        self.__filter_forpages()
        tbls = self.__extract_table_figure(need_image, zoomin, return_html)
        return self.__filterout_scraps(deepcopy(self.boxes), zoomin), tbls

    def remove_tag(self, txt):
        return re.sub(r"@@[\t0-9.-]+?##", "", txt)

    def crop(self, text, ZM=3):
        imgs = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", text):
            pn, left, right, top, bottom = tag.strip(
                "#").strip("@").split("\t")
            left, right, top, bottom = float(left), float(
                right), float(top), float(bottom)
            bottom *= ZM
            pns = [int(p) - 1 for p in pn.split("-")]
            for pn in pns[1:]:
                bottom += self.page_images[pn - 1].size[1]
            imgs.append(
                self.page_images[pns[0]].crop((left * ZM, top * ZM,
                                               right *
                                               ZM, min(
                                                   bottom, self.page_images[pns[0]].size[1])
                                               ))
            )
            bottom -= self.page_images[pns[0]].size[1]
            for pn in pns[1:]:
                imgs.append(
                    self.page_images[pn].crop((left * ZM, 0,
                                               right * ZM,
                                               min(bottom,
                                                   self.page_images[pn].size[1])
                                               ))
                )
                bottom -= self.page_images[pn].size[1]

        if not imgs:
            return
        GAP = 2
        height = 0
        for img in imgs:
            height += img.size[1] + GAP
        height = int(height)
        pic = Image.new("RGB",
                        (int(np.max([i.size[0] for i in imgs])), height),
                        (245, 245, 245))
        height = 0
        for img in imgs:
            pic.paste(img, (0, int(height)))
            height += img.size[1] + GAP
        return pic


if __name__ == "__main__":
    pass
