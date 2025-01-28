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
import sys

sys.path.insert(
    0,
    os.path.abspath(
        os.path.join(
            os.path.dirname(
                os.path.abspath(__file__)),
            '../../')))

from deepdoc.vision.seeit import draw_box
from deepdoc.vision import LayoutRecognizer, TableStructureRecognizer, OCR, init_in_out
import argparse
import re
import numpy as np


def main(args):
    images, outputs = init_in_out(args)
    if args.mode.lower() == "layout":
        detr = LayoutRecognizer("layout")
        layouts = detr.forward(images, thr=float(args.threshold))
    if args.mode.lower() == "tsr":
        detr = TableStructureRecognizer()
        ocr = OCR()
        layouts = detr(images, thr=float(args.threshold))
    for i, lyt in enumerate(layouts):
        if args.mode.lower() == "tsr":
            #lyt = [t for t in lyt if t["type"] == "table column"]
            html = get_table_html(images[i], lyt, ocr)
            with open(outputs[i] + ".html", "w+", encoding='utf-8') as f:
                f.write(html)
            lyt = [{
                "type": t["label"],
                "bbox": [t["x0"], t["top"], t["x1"], t["bottom"]],
                "score": t["score"]
            } for t in lyt]
        img = draw_box(images[i], lyt, detr.labels, float(args.threshold))
        img.save(outputs[i], quality=95)
        logging.info("save result to: " + outputs[i])


def get_table_html(img, tb_cpns, ocr):
    boxes = ocr(np.array(img))
    boxes = LayoutRecognizer.sort_Y_firstly(
        [{"x0": b[0][0], "x1": b[1][0],
          "top": b[0][1], "text": t[0],
          "bottom": b[-1][1],
          "layout_type": "table",
          "page_number": 0} for b, t in boxes if b[0][0] <= b[1][0] and b[0][1] <= b[-1][1]],
        np.mean([b[-1][1] - b[0][1] for b, _ in boxes]) / 3
    )

    def gather(kwd, fzy=10, ption=0.6):
        nonlocal boxes
        eles = LayoutRecognizer.sort_Y_firstly(
            [r for r in tb_cpns if re.match(kwd, r["label"])], fzy)
        eles = LayoutRecognizer.layouts_cleanup(boxes, eles, 5, ption)
        return LayoutRecognizer.sort_Y_firstly(eles, 0)

    headers = gather(r".*header$")
    rows = gather(r".* (row|header)")
    spans = gather(r".*spanning")
    clmns = sorted([r for r in tb_cpns if re.match(
        r"table column$", r["label"])], key=lambda x: x["x0"])
    clmns = LayoutRecognizer.layouts_cleanup(boxes, clmns, 5, 0.5)

    for b in boxes:
        ii = LayoutRecognizer.find_overlapped_with_threashold(b, rows, thr=0.3)
        if ii is not None:
            b["R"] = ii
            b["R_top"] = rows[ii]["top"]
            b["R_bott"] = rows[ii]["bottom"]

        ii = LayoutRecognizer.find_overlapped_with_threashold(b, headers, thr=0.3)
        if ii is not None:
            b["H_top"] = headers[ii]["top"]
            b["H_bott"] = headers[ii]["bottom"]
            b["H_left"] = headers[ii]["x0"]
            b["H_right"] = headers[ii]["x1"]
            b["H"] = ii

        ii = LayoutRecognizer.find_horizontally_tightest_fit(b, clmns)
        if ii is not None:
            b["C"] = ii
            b["C_left"] = clmns[ii]["x0"]
            b["C_right"] = clmns[ii]["x1"]

        ii = LayoutRecognizer.find_overlapped_with_threashold(b, spans, thr=0.3)
        if ii is not None:
            b["H_top"] = spans[ii]["top"]
            b["H_bott"] = spans[ii]["bottom"]
            b["H_left"] = spans[ii]["x0"]
            b["H_right"] = spans[ii]["x1"]
            b["SP"] = ii

    html = """
    <html>
    <head>
    <style>
    ._table_1nkzy_11 {
      margin: auto;
      width: 70%%;
      padding: 10px;
    }
    ._table_1nkzy_11 p {
      margin-bottom: 50px;
      border: 1px solid #e1e1e1;
    }

    caption {
      color: #6ac1ca;
      font-size: 20px;
      height: 50px;
      line-height: 50px;
      font-weight: 600;
      margin-bottom: 10px;
    }

    ._table_1nkzy_11 table {
      width: 100%%;
      border-collapse: collapse;
    }

    th {
      color: #fff;
      background-color: #6ac1ca;
    }

    td:hover {
      background: #c1e8e8;
    }

    tr:nth-child(even) {
      background-color: #f2f2f2;
    }

    ._table_1nkzy_11 th,
    ._table_1nkzy_11 td {
      text-align: center;
      border: 1px solid #ddd;
      padding: 8px;
    }
    </style>
    </head>
    <body>
    %s
    </body>
    </html>
""" % TableStructureRecognizer.construct_table(boxes, html=True)
    return html


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('--inputs',
                        help="Directory where to store images or PDFs, or a file path to a single image or PDF",
                        required=True)
    parser.add_argument('--output_dir', help="Directory where to store the output images. Default: './layouts_outputs'",
                        default="./layouts_outputs")
    parser.add_argument(
        '--threshold',
        help="A threshold to filter out detections. Default: 0.5",
        default=0.5)
    parser.add_argument('--mode', help="Task mode: layout recognition or table structure recognition", choices=["layout", "tsr"],
                        default="layout")
    args = parser.parse_args()
    main(args)
