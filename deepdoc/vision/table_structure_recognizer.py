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
import logging
import os
import re
from collections import Counter

import numpy as np
from huggingface_hub import snapshot_download

from common.file_utils import get_project_base_directory
from rag.nlp import rag_tokenizer

from .recognizer import Recognizer


class TableStructureRecognizer(Recognizer):
    labels = [
        "table",
        "table column",
        "table row",
        "table column header",
        "table projected row header",
        "table spanning cell",
    ]

    def __init__(self):
        try:
            super().__init__(self.labels, "tsr", os.path.join(get_project_base_directory(), "rag/res/deepdoc"))
        except Exception:
            super().__init__(
                self.labels,
                "tsr",
                snapshot_download(
                    repo_id="InfiniFlow/deepdoc",
                    local_dir=os.path.join(get_project_base_directory(), "rag/res/deepdoc"),
                    local_dir_use_symlinks=False,
                ),
            )

    def __call__(self, images, thr=0.2):
        table_structure_recognizer_type = os.getenv("TABLE_STRUCTURE_RECOGNIZER_TYPE", "onnx").lower()
        if table_structure_recognizer_type not in ["onnx", "ascend"]:
            raise RuntimeError("Unsupported table structure recognizer type.")

        if table_structure_recognizer_type == "onnx":
            logging.debug("Using Onnx table structure recognizer")
            tbls = super().__call__(images, thr)
        else:  # ascend
            logging.debug("Using Ascend table structure recognizer")
            tbls = self._run_ascend_tsr(images, thr)

        res = []
        # align left&right for rows, align top&bottom for columns
        for tbl in tbls:
            lts = [
                {
                    "label": b["type"],
                    "score": b["score"],
                    "x0": b["bbox"][0],
                    "x1": b["bbox"][2],
                    "top": b["bbox"][1],
                    "bottom": b["bbox"][-1],
                }
                for b in tbl
            ]
            if not lts:
                continue

            left = [b["x0"] for b in lts if b["label"].find("row") > 0 or b["label"].find("header") > 0]
            right = [b["x1"] for b in lts if b["label"].find("row") > 0 or b["label"].find("header") > 0]
            if not left:
                continue
            left = np.mean(left) if len(left) > 4 else np.min(left)
            right = np.mean(right) if len(right) > 4 else np.max(right)
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

    @staticmethod
    def is_caption(bx):
        patt = [r"[图表]+[ 0-9:：]{2,}"]
        if any([re.match(p, bx["text"].strip()) for p in patt]) or bx.get("layout_type", "").find("caption") >= 0:
            return True
        return False

    @staticmethod
    def blockType(b):
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
            (r"^.{1}$", "Sg"),
        ]
        for p, n in patt:
            if re.search(p, b["text"].strip()):
                return n
        tks = [t for t in rag_tokenizer.tokenize(b["text"]).split() if len(t) > 1]
        if len(tks) > 3:
            if len(tks) < 12:
                return "Tx"
            else:
                return "Lx"

        if len(tks) == 1 and rag_tokenizer.tag(tks[0]) == "nr":
            return "Nr"

        return "Ot"

    @staticmethod
    def construct_table(boxes, is_english=False, html=True, **kwargs):
        cap = ""
        i = 0
        while i < len(boxes):
            if TableStructureRecognizer.is_caption(boxes[i]):
                if is_english:
                    cap += " "
                cap += boxes[i]["text"]
                boxes.pop(i)
                i -= 1
            i += 1

        if not boxes:
            return []
        for b in boxes:
            b["btype"] = TableStructureRecognizer.blockType(b)
        max_type = Counter([b["btype"] for b in boxes]).items()
        max_type = max(max_type, key=lambda x: x[1])[0] if max_type else ""
        logging.debug("MAXTYPE: " + max_type)

        rowh = [b["R_bott"] - b["R_top"] for b in boxes if "R" in b]
        rowh = np.min(rowh) if rowh else 0
        boxes = Recognizer.sort_R_firstly(boxes, rowh / 2)
        # for b in boxes:print(b)
        boxes[0]["rn"] = 0
        rows = [[boxes[0]]]
        btm = boxes[0]["bottom"]
        for b in boxes[1:]:
            b["rn"] = len(rows) - 1
            lst_r = rows[-1]
            if lst_r[-1].get("R", "") != b.get("R", "") or (b["top"] >= btm - 3 and lst_r[-1].get("R", "-1") != b.get("R", "-2")):  # new row
                btm = b["bottom"]
                b["rn"] += 1
                rows.append([b])
                continue
            btm = (btm + b["bottom"]) / 2.0
            rows[-1].append(b)

        colwm = [b["C_right"] - b["C_left"] for b in boxes if "C" in b]
        colwm = np.min(colwm) if colwm else 0
        crosspage = len(set([b["page_number"] for b in boxes])) > 1
        if crosspage:
            boxes = Recognizer.sort_X_firstly(boxes, colwm / 2)
        else:
            boxes = Recognizer.sort_C_firstly(boxes, colwm / 2)
        boxes[0]["cn"] = 0
        cols = [[boxes[0]]]
        right = boxes[0]["x1"]
        for b in boxes[1:]:
            b["cn"] = len(cols) - 1
            lst_c = cols[-1]
            if (int(b.get("C", "1")) - int(lst_c[-1].get("C", "1")) == 1 and b["page_number"] == lst_c[-1]["page_number"]) or (
                b["x0"] >= right and lst_c[-1].get("C", "-1") != b.get("C", "-2")
            ):  # new col
                right = b["x1"]
                b["cn"] += 1
                cols.append([b])
                continue
            right = (right + b["x1"]) / 2.0
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
                f = (j > 0 and tbl[ii][j - 1] and tbl[ii][j - 1][0].get("text")) or j == 0
                ff = (j + 1 < len(tbl[ii]) and tbl[ii][j + 1] and tbl[ii][j + 1][0].get("text")) or j + 1 >= len(tbl[ii])
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
                            left = min(left, np.min([bx["x0"] - a["x1"] for a in tbl[i][j - 1]]))
                if j + 1 < len(tbl[0]) and not ff:
                    for i in range(len(tbl)):
                        if tbl[i][j + 1]:
                            right = min(right, np.min([a["x0"] - bx["x1"] for a in tbl[i][j + 1]]))
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
        assert len(cols) == len(tbl[0]), "Column NO. miss matched: %d vs %d" % (len(cols), len(tbl[0]))

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
                f = (i > 0 and tbl[i - 1][jj] and tbl[i - 1][jj][0].get("text")) or i == 0
                ff = (i + 1 < len(tbl) and tbl[i + 1][jj] and tbl[i + 1][jj][0].get("text")) or i + 1 >= len(tbl)
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
                            up = min(up, np.min([bx["top"] - a["bottom"] for a in tbl[i - 1][j]]))
                if i + 1 < len(tbl) and not ff:
                    for j in range(len(tbl[i + 1])):
                        if tbl[i + 1][j]:
                            down = min(down, np.min([a["top"] - bx["bottom"] for a in tbl[i + 1][j]]))
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
                if any([a.get("H") for a in arr]) or (max_type == "Nu" and arr[0]["btype"] != "Nu"):
                    h += 1
            if h / cnt > 0.5:
                hdset.add(i)

        if html:
            return TableStructureRecognizer.__html_table(cap, hdset, TableStructureRecognizer.__cal_spans(boxes, rows, cols, tbl, True))

        return TableStructureRecognizer.__desc_table(cap, hdset, TableStructureRecognizer.__cal_spans(boxes, rows, cols, tbl, False), is_english)

    @staticmethod
    def __html_table(cap, hdset, tbl):
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
                    h = min(np.min([c["bottom"] - c["top"] for c in arr]) / 2, 10)
                    txt = " ".join([c["text"] for c in Recognizer.sort_Y_firstly(arr, h)])
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

    @staticmethod
    def __desc_table(cap, hdr_rowno, tbl, is_english):
        # get text of every colomn in header row to become header text
        clmno = len(tbl[0])
        rowno = len(tbl)
        headers = {}
        hdrset = set()
        lst_hdr = []
        de = "的" if not is_english else " for "
        for r in sorted(list(hdr_rowno)):
            headers[r] = ["" for _ in range(clmno)]
            for i in range(clmno):
                if not tbl[r][i]:
                    continue
                txt = " ".join([a["text"].strip() for a in tbl[r][i]])
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
                        headers[j][k] += (de if headers[j][k] else "") + headers[j - 1][k]
                    else:
                        headers[j][k] = headers[j - 1][k] + (de if headers[j - 1][k] else "") + headers[j][k]

        logging.debug(f">>>>>>>>>>>>>>>>>{cap}：SIZE:{rowno}X{clmno} Header: {hdr_rowno}")
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
            if is_english:
                from_ = " in "
            else:
                from_ = "来自"
            row_txt = [t + f"\t——{from_}“{cap}”" for t in row_txt]
        return row_txt

    @staticmethod
    def __cal_spans(boxes, rows, cols, tbl, html=True):
        # caculate span
        clft = [np.mean([c.get("C_left", c["x0"]) for c in cln]) for cln in cols]
        crgt = [np.mean([c.get("C_right", c["x1"]) for c in cln]) for cln in cols]
        rtop = [np.mean([c.get("R_top", c["top"]) for c in row]) for row in rows]
        rbtm = [np.mean([c.get("R_btm", c["bottom"]) for c in row]) for row in rows]
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

    def _run_ascend_tsr(self, image_list, thr=0.2, batch_size=16):
        import math

        from ais_bench.infer.interface import InferSession

        model_dir = os.path.join(get_project_base_directory(), "rag/res/deepdoc")
        model_file_path = os.path.join(model_dir, "tsr.om")

        if not os.path.exists(model_file_path):
            raise ValueError(f"Model file not found: {model_file_path}")

        device_id = int(os.getenv("ASCEND_LAYOUT_RECOGNIZER_DEVICE_ID", 0))
        session = InferSession(device_id=device_id, model_path=model_file_path)

        images = [np.array(im) if not isinstance(im, np.ndarray) else im for im in image_list]
        results = []

        conf_thr = max(thr, 0.08)

        batch_loop_cnt = math.ceil(float(len(images)) / batch_size)
        for bi in range(batch_loop_cnt):
            s = bi * batch_size
            e = min((bi + 1) * batch_size, len(images))
            batch_images = images[s:e]

            inputs_list = self.preprocess(batch_images)
            for ins in inputs_list:
                feeds = []
                if "image" in ins:
                    feeds.append(ins["image"])
                else:
                    feeds.append(ins[self.input_names[0]])
                output_list = session.infer(feeds=feeds, mode="static")
                bb = self.postprocess(output_list, ins, conf_thr)
                results.append(bb)
        return results
