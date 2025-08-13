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
from io import BytesIO
from xpinyin import Pinyin
import numpy as np
import pandas as pd
from collections import Counter

# from openpyxl import load_workbook, Workbook
from dateutil.parser import parse as datetime_parse

from api.db.services.knowledgebase_service import KnowledgebaseService
from deepdoc.parser.utils import get_text
from rag.nlp import rag_tokenizer, tokenize
from deepdoc.parser import ExcelParser


class Excel(ExcelParser):
    def __call__(self, fnm, binary=None, from_page=0, to_page=10000000000, callback=None):
        if not binary:
            wb = Excel._load_excel_to_workbook(fnm)
        else:
            wb = Excel._load_excel_to_workbook(BytesIO(binary))
        total = 0
        for sheetname in wb.sheetnames:
            total += len(list(wb[sheetname].rows))
        res, fails, done = [], [], 0
        rn = 0
        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            rows = list(ws.rows)
            if not rows:
                continue
            headers, header_rows = self._parse_headers(ws, rows)
            if not headers:
                continue
            data = []
            for i, r in enumerate(rows[header_rows:]):
                rn += 1
                if rn - 1 < from_page:
                    continue
                if rn - 1 >= to_page:
                    break
                row_data = self._extract_row_data(ws, r, header_rows + i, len(headers))
                if row_data is None:
                    fails.append(str(i))
                    continue
                if self._is_empty_row(row_data):
                    continue
                data.append(row_data)
                done += 1
            if len(data) == 0:
                continue
            df = pd.DataFrame(data, columns=headers)
            res.append(df)
        callback(0.3, ("Extract records: {}~{}".format(from_page + 1, min(to_page, from_page + rn)) + (f"{len(fails)} failure, line: %s..." % (",".join(fails[:3])) if fails else "")))
        return res

    def _parse_headers(self, ws, rows):
        if len(rows) == 0:
            return [], 0
        has_complex_structure = self._has_complex_header_structure(ws, rows)
        if has_complex_structure:
            return self._parse_multi_level_headers(ws, rows)
        else:
            return self._parse_simple_headers(rows)

    def _has_complex_header_structure(self, ws, rows):
        if len(rows) < 1:
            return False
        merged_ranges = list(ws.merged_cells.ranges)
        # 检查前两行是否涉及合并单元格
        for rng in merged_ranges:
            if rng.min_row <= 2:  # 只要合并区域涉及第1或第2行
                return True
        return False

    def _row_looks_like_header(self, row):
        header_like_cells = 0
        data_like_cells = 0
        non_empty_cells = 0
        for cell in row:
            if cell.value is not None:
                non_empty_cells += 1
                val = str(cell.value).strip()
                if self._looks_like_header(val):
                    header_like_cells += 1
                elif self._looks_like_data(val):
                    data_like_cells += 1
        if non_empty_cells == 0:
            return False
        return header_like_cells >= data_like_cells

    def _parse_simple_headers(self, rows):
        if not rows:
            return [], 0
        header_row = rows[0]
        headers = []
        for cell in header_row:
            if cell.value is not None:
                header_value = str(cell.value).strip()
                if header_value:
                    headers.append(header_value)
            else:
                pass
        final_headers = []
        for i, cell in enumerate(header_row):
            if cell.value is not None:
                header_value = str(cell.value).strip()
                if header_value:
                    final_headers.append(header_value)
                else:
                    final_headers.append(f"Column_{i + 1}")
            else:
                final_headers.append(f"Column_{i + 1}")
        return final_headers, 1

    def _parse_multi_level_headers(self, ws, rows):
        if len(rows) < 2:
            return [], 0
        header_rows = self._detect_header_rows(rows)
        if header_rows == 1:
            return self._parse_simple_headers(rows)
        else:
            return self._build_hierarchical_headers(ws, rows, header_rows), header_rows

    def _detect_header_rows(self, rows):
        if len(rows) < 2:
            return 1
        header_rows = 1
        max_check_rows = min(5, len(rows))
        for i in range(1, max_check_rows):
            row = rows[i]
            if self._row_looks_like_header(row):
                header_rows = i + 1
            else:
                break
        return header_rows

    def _looks_like_header(self, value):
        if len(value) < 1:
            return False
        if any(ord(c) > 127 for c in value):
            return True
        if len([c for c in value if c.isalpha()]) >= 2:
            return True
        if any(c in value for c in ["(", ")", "：", ":", "（", "）", "_", "-"]):
            return True
        return False

    def _looks_like_data(self, value):
        if len(value) == 1 and value.upper() in ["Y", "N", "M", "X", "/", "-"]:
            return True
        if value.replace(".", "").replace("-", "").replace(",", "").isdigit():
            return True
        if value.startswith("0x") and len(value) <= 10:
            return True
        return False

    def _build_hierarchical_headers(self, ws, rows, header_rows):
        headers = []
        max_col = max(len(row) for row in rows[:header_rows]) if header_rows > 0 else 0
        merged_ranges = list(ws.merged_cells.ranges)
        for col_idx in range(max_col):
            header_parts = []
            for row_idx in range(header_rows):
                if col_idx < len(rows[row_idx]):
                    cell_value = rows[row_idx][col_idx].value
                    merged_value = self._get_merged_cell_value(ws, row_idx + 1, col_idx + 1, merged_ranges)
                    if merged_value is not None:
                        cell_value = merged_value
                    if cell_value is not None:
                        cell_value = str(cell_value).strip()
                        if cell_value and cell_value not in header_parts and self._is_valid_header_part(cell_value):
                            header_parts.append(cell_value)
            if header_parts:
                header = "-".join(header_parts)
                headers.append(header)
            else:
                headers.append(f"Column_{col_idx + 1}")
        final_headers = [h for h in headers if h and h != "-"]
        return final_headers

    def _is_valid_header_part(self, value):
        if len(value) == 1 and value.upper() in ["Y", "N", "M", "X"]:
            return False
        if value.replace(".", "").replace("-", "").replace(",", "").isdigit():
            return False
        if value in ["/", "-", "+", "*", "="]:
            return False
        return True

    def _get_merged_cell_value(self, ws, row, col, merged_ranges):
        for merged_range in merged_ranges:
            if merged_range.min_row <= row <= merged_range.max_row and merged_range.min_col <= col <= merged_range.max_col:
                return ws.cell(merged_range.min_row, merged_range.min_col).value
        return None

    def _extract_row_data(self, ws, row, absolute_row_idx, expected_cols):
        row_data = []
        merged_ranges = list(ws.merged_cells.ranges)
        actual_row_num = absolute_row_idx + 1
        for col_idx in range(expected_cols):
            cell_value = None
            actual_col_num = col_idx + 1
            try:
                cell_value = ws.cell(row=actual_row_num, column=actual_col_num).value
            except ValueError:
                if col_idx < len(row):
                    cell_value = row[col_idx].value
            if cell_value is None:
                merged_value = self._get_merged_cell_value(ws, actual_row_num, actual_col_num, merged_ranges)
                if merged_value is not None:
                    cell_value = merged_value
                else:
                    cell_value = self._get_inherited_value(ws, actual_row_num, actual_col_num, merged_ranges)
            row_data.append(cell_value)
        return row_data

    def _get_inherited_value(self, ws, row, col, merged_ranges):
        for merged_range in merged_ranges:
            if merged_range.min_row <= row <= merged_range.max_row and merged_range.min_col <= col <= merged_range.max_col:
                return ws.cell(merged_range.min_row, merged_range.min_col).value
        return None

    def _is_empty_row(self, row_data):
        for val in row_data:
            if val is not None and str(val).strip() != "":
                return False
        return True


def trans_datatime(s):
    try:
        return datetime_parse(s.strip()).strftime("%Y-%m-%d %H:%M:%S")
    except Exception:
        pass


def trans_bool(s):
    if re.match(r"(true|yes|是|\*|✓|✔|☑|✅|√)$", str(s).strip(), flags=re.IGNORECASE):
        return "yes"
    if re.match(r"(false|no|否|⍻|×)$", str(s).strip(), flags=re.IGNORECASE):
        return "no"


def column_data_type(arr):
    arr = list(arr)
    counts = {"int": 0, "float": 0, "text": 0, "datetime": 0, "bool": 0}
    trans = {t: f for f, t in [(int, "int"), (float, "float"), (trans_datatime, "datetime"), (trans_bool, "bool"), (str, "text")]}
    float_flag = False
    for a in arr:
        if a is None:
            continue
        if re.match(r"[+-]?[0-9]+$", str(a).replace("%%", "")) and not str(a).replace("%%", "").startswith("0"):
            counts["int"] += 1
            if int(str(a)) > 2**63 - 1:
                float_flag = True
                break
        elif re.match(r"[+-]?[0-9.]{,19}$", str(a).replace("%%", "")) and not str(a).replace("%%", "").startswith("0"):
            counts["float"] += 1
        elif re.match(r"(true|yes|是|\*|✓|✔|☑|✅|√|false|no|否|⍻|×)$", str(a), flags=re.IGNORECASE):
            counts["bool"] += 1
        elif trans_datatime(str(a)):
            counts["datetime"] += 1
        else:
            counts["text"] += 1
    if float_flag:
        ty = "float"
    else:
        counts = sorted(counts.items(), key=lambda x: x[1] * -1)
        ty = counts[0][0]
    for i in range(len(arr)):
        if arr[i] is None:
            continue
        try:
            arr[i] = trans[ty](str(arr[i]))
        except Exception:
            arr[i] = None
    # if ty == "text":
    #    if len(arr) > 128 and uni / len(arr) < 0.1:
    #        ty = "keyword"
    return arr, ty


def chunk(filename, binary=None, from_page=0, to_page=10000000000, lang="Chinese", callback=None, **kwargs):
    """
    Excel and csv(txt) format files are supported.
    For csv or txt file, the delimiter between columns is TAB.
    The first line must be column headers.
    Column headers must be meaningful terms inorder to make our NLP model understanding.
    It's good to enumerate some synonyms using slash '/' to separate, and even better to
    enumerate values using brackets like 'gender/sex(male, female)'.
    Here are some examples for headers:
        1. supplier/vendor\tcolor(yellow, red, brown)\tgender/sex(male, female)\tsize(M,L,XL,XXL)
        2. 姓名/名字\t电话/手机/微信\t最高学历（高中，职高，硕士，本科，博士，初中，中技，中专，专科，专升本，MPA，MBA，EMBA）

    Every row in table will be treated as a chunk.
    """

    if re.search(r"\.xlsx?$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        excel_parser = Excel()
        dfs = excel_parser(filename, binary, from_page=from_page, to_page=to_page, callback=callback)
    elif re.search(r"\.(txt|csv)$", filename, re.IGNORECASE):
        callback(0.1, "Start to parse.")
        txt = get_text(filename, binary)
        lines = txt.split("\n")
        fails = []
        headers = lines[0].split(kwargs.get("delimiter", "\t"))
        rows = []
        for i, line in enumerate(lines[1:]):
            if i < from_page:
                continue
            if i >= to_page:
                break
            row = [field for field in line.split(kwargs.get("delimiter", "\t"))]
            if len(row) != len(headers):
                fails.append(str(i))
                continue
            rows.append(row)

        callback(0.3, ("Extract records: {}~{}".format(from_page, min(len(lines), to_page)) + (f"{len(fails)} failure, line: %s..." % (",".join(fails[:3])) if fails else "")))

        dfs = [pd.DataFrame(np.array(rows), columns=headers)]

    else:
        raise NotImplementedError("file type not supported yet(excel, text, csv supported)")

    res = []
    PY = Pinyin()
    fieds_map = {"text": "_tks", "int": "_long", "keyword": "_kwd", "float": "_flt", "datetime": "_dt", "bool": "_kwd"}
    for df in dfs:
        for n in ["id", "_id", "index", "idx"]:
            if n in df.columns:
                del df[n]
        clmns = df.columns.values
        if len(clmns) != len(set(clmns)):
            col_counts = Counter(clmns)
            duplicates = [col for col, count in col_counts.items() if count > 1]
            if duplicates:
                raise ValueError(f"Duplicate column names detected: {duplicates}\nFrom: {clmns}")

        txts = list(copy.deepcopy(clmns))
        py_clmns = [PY.get_pinyins(re.sub(r"(/.*|（[^（）]+?）|\([^()]+?\))", "", str(n)), "_")[0] for n in clmns]
        clmn_tys = []
        for j in range(len(clmns)):
            cln, ty = column_data_type(df[clmns[j]])
            clmn_tys.append(ty)
            df[clmns[j]] = cln
            if ty == "text":
                txts.extend([str(c) for c in cln if c])
        clmns_map = [(py_clmns[i].lower() + fieds_map[clmn_tys[i]], str(clmns[i]).replace("_", " ")) for i in range(len(clmns))]

        eng = lang.lower() == "english"  # is_english(txts)
        for ii, row in df.iterrows():
            d = {"docnm_kwd": filename, "title_tks": rag_tokenizer.tokenize(re.sub(r"\.[a-zA-Z]+$", "", filename))}
            row_txt = []
            for j in range(len(clmns)):
                if row[clmns[j]] is None:
                    continue
                if not str(row[clmns[j]]):
                    continue
                if not isinstance(row[clmns[j]], pd.Series) and pd.isna(row[clmns[j]]):
                    continue
                fld = clmns_map[j][0]
                d[fld] = row[clmns[j]] if clmn_tys[j] != "text" else rag_tokenizer.tokenize(row[clmns[j]])
                row_txt.append("{}:{}".format(clmns[j], row[clmns[j]]))
            if not row_txt:
                continue
            tokenize(d, "; ".join(row_txt), eng)
            res.append(d)

        KnowledgebaseService.update_parser_config(kwargs["kb_id"], {"field_map": {k: v for k, v in clmns_map}})
    callback(0.35, "")

    return res


if __name__ == "__main__":
    import sys

    def dummy(prog=None, msg=""):
        pass

    chunk(sys.argv[1], callback=dummy)
