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
import re
import sys
from io import BytesIO
from typing import List, Dict, Any
import base64

import pandas as pd
from openpyxl import Workbook, load_workbook

from rag.nlp import find_codec

# copied from `/openpyxl/cell/cell.py`
ILLEGAL_CHARACTERS_RE = re.compile(r"[\000-\010]|[\013-\014]|[\016-\037]")


class RAGFlowExcelParser:
    @staticmethod
    def _load_excel_to_workbook(file_like_object):
        if isinstance(file_like_object, bytes):
            file_like_object = BytesIO(file_like_object)

        # Read first 4 bytes to determine file type
        file_like_object.seek(0)
        file_head = file_like_object.read(4)
        file_like_object.seek(0)

        if not (file_head.startswith(b"PK\x03\x04") or file_head.startswith(b"\xd0\xcf\x11\xe0")):
            logging.info("Not an Excel file, converting CSV to Excel Workbook")

            try:
                file_like_object.seek(0)
                df = pd.read_csv(file_like_object)
                return RAGFlowExcelParser._dataframe_to_workbook(df)

            except Exception as e_csv:
                raise Exception(f"Failed to parse CSV and convert to Excel Workbook: {e_csv}")

        try:
            return load_workbook(file_like_object, data_only=True)
        except Exception as e:
            logging.info(f"openpyxl load error: {e}, try pandas instead")
            try:
                file_like_object.seek(0)
                try:
                    dfs = pd.read_excel(file_like_object, sheet_name=None)
                    return RAGFlowExcelParser._dataframe_to_workbook(dfs)
                except Exception as ex:
                    logging.info(f"pandas with default engine load error: {ex}, try calamine instead")
                    file_like_object.seek(0)
                    df = pd.read_excel(file_like_object, engine="calamine")
                    return RAGFlowExcelParser._dataframe_to_workbook(df)
            except Exception as e_pandas:
                raise Exception(f"pandas.read_excel error: {e_pandas}, original openpyxl error: {e}")

    @staticmethod
    def _clean_dataframe(df: pd.DataFrame):
        def clean_string(s):
            if isinstance(s, str):
                return ILLEGAL_CHARACTERS_RE.sub(" ", s)
            return s

        return df.apply(lambda col: col.map(clean_string))

    @staticmethod
    def _dataframe_to_workbook(df):
        # if contains multiple sheets use _dataframes_to_workbook
        if isinstance(df, dict) and len(df) > 1:
            return RAGFlowExcelParser._dataframes_to_workbook(df)

        df = RAGFlowExcelParser._clean_dataframe(df)
        wb = Workbook()
        ws = wb.active
        ws.title = "Data"

        for col_num, column_name in enumerate(df.columns, 1):
            ws.cell(row=1, column=col_num, value=column_name)

        for row_num, row in enumerate(df.values, 2):
            for col_num, value in enumerate(row, 1):
                ws.cell(row=row_num, column=col_num, value=value)

        return wb
    
    @staticmethod
    def _dataframes_to_workbook(dfs: dict):
        wb = Workbook()
        default_sheet = wb.active
        wb.remove(default_sheet)
        
        for sheet_name, df in dfs.items():
            df = RAGFlowExcelParser._clean_dataframe(df)
            ws = wb.create_sheet(title=sheet_name)
            for col_num, column_name in enumerate(df.columns, 1):
                ws.cell(row=1, column=col_num, value=column_name)
            for row_num, row in enumerate(df.values, 2):
                for col_num, value in enumerate(row, 1):
                    ws.cell(row=row_num, column=col_num, value=value)
        return wb

    def html(self, fnm, chunk_rows=256):
        from html import escape

        file_like_object = BytesIO(fnm) if not isinstance(fnm, str) else fnm
        wb = RAGFlowExcelParser._load_excel_to_workbook(file_like_object)
        tb_chunks = []

        def _fmt(v):
            if v is None:
                return ""
            return str(v).strip()

        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            try:
                rows = list(ws.rows)
            except Exception as e:
                logging.warning(f"Skip sheet '{sheetname}' due to rows access error: {e}")
                continue

            if not rows:
                continue

            tb_rows_0 = "<tr>"
            for t in list(rows[0]):
                tb_rows_0 += f"<th>{escape(_fmt(t.value))}</th>"
            tb_rows_0 += "</tr>"

            for chunk_i in range((len(rows) - 1) // chunk_rows + 1):
                tb = ""
                tb += f"<table><caption>{sheetname}</caption>"
                tb += tb_rows_0
                for r in list(rows[1 + chunk_i * chunk_rows : min(1 + (chunk_i + 1) * chunk_rows, len(rows))]):
                    tb += "<tr>"
                    for i, c in enumerate(r):
                        if c.value is None:
                            tb += "<td></td>"
                        else:
                            tb += f"<td>{escape(_fmt(c.value))}</td>"
                    tb += "</tr>"
                tb += "</table>\n"
                tb_chunks.append(tb)

        return tb_chunks

    def markdown(self, fnm):
        import pandas as pd

        file_like_object = BytesIO(fnm) if not isinstance(fnm, str) else fnm
        try:
            file_like_object.seek(0)
            df = pd.read_excel(file_like_object)
        except Exception as e:
            logging.warning(f"Parse spreadsheet error: {e}, trying to interpret as CSV file")
            file_like_object.seek(0)
            df = pd.read_csv(file_like_object)
        df = df.replace(r"^\s*$", "", regex=True)
        return df.to_markdown(index=False)

    def extract_images(self, fnm) -> List[Dict[str, Any]]:
        """
        Extract all embedded images from Excel file.
        
        Args:
            fnm: File path or bytes
            
        Returns:
            List of dictionaries containing image information:
            {
                'image_data': base64 encoded image data,
                'format': image format (png, jpeg, etc.),
                'sheet': sheet name,
                'anchor': cell anchor position (e.g., 'A1'),
                'description': alt text/description if available,
                'size': (width, height) in pixels
            }
        """
        file_like_object = BytesIO(fnm) if not isinstance(fnm, str) else fnm
        wb = RAGFlowExcelParser._load_excel_to_workbook(file_like_object)
        
        images = []
        image_index = 0
        
        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            
            # openpyxl stores images in worksheet._images
            if hasattr(ws, '_images') and ws._images:
                for img in ws._images:
                    try:
                        # Get image data
                        img_data = img._data() if hasattr(img, '_data') else img.ref
                        
                        # Encode image to base64
                        if isinstance(img_data, bytes):
                            img_base64 = base64.b64encode(img_data).decode('utf-8')
                        else:
                            img_base64 = img_data
                        
                        # Get image format
                        img_format = getattr(img, 'format', 'png').lower()
                        if img_format == 'emf':
                            img_format = 'png'  # Convert EMF to common format indicator
                        
                        # Get anchor position
                        anchor = 'Unknown'
                        if hasattr(img, 'anchor') and img.anchor:
                            if hasattr(img.anchor, '_from'):
                                # Anchor is a cell reference
                                anchor_cell = img.anchor._from
                                if hasattr(anchor_cell, 'col') and hasattr(anchor_cell, 'row'):
                                    # Convert column number to letter
                                    col_letter = self._number_to_column_letter(anchor_cell.col)
                                    anchor = f"{col_letter}{anchor_cell.row + 1}"
                            elif hasattr(img.anchor, 'col') and hasattr(img.anchor, 'row'):
                                col_letter = self._number_to_column_letter(img.anchor.col)
                                anchor = f"{col_letter}{img.anchor.row + 1}"
                        
                        # Get description/alt text
                        description = getattr(img, 'name', '') or getattr(img, 'description', '') or f'Image_{image_index}'
                        
                        # Get size
                        width = getattr(img, 'width', 0)
                        height = getattr(img, 'height', 0)
                        
                        images.append({
                            'image_data': img_base64,
                            'format': img_format,
                            'sheet': sheetname,
                            'anchor': anchor,
                            'description': description,
                            'size': (width, height),
                            'index': image_index
                        })
                        
                        image_index += 1
                        logging.info(f"Extracted image from sheet '{sheetname}' at {anchor}")
                        
                    except Exception as e:
                        logging.warning(f"Failed to extract image from sheet '{sheetname}': {e}")
                        continue
        
        logging.info(f"Extracted {len(images)} images from Excel file")
        return images
    
    @staticmethod
    def _number_to_column_letter(n):
        """Convert column number to Excel column letter (0 -> A, 1 -> B, etc.)"""
        result = ""
        while n >= 0:
            result = chr(n % 26 + 65) + result
            n = n // 26 - 1
            if n < 0:
                break
        return result

    def __call__(self, fnm):
        file_like_object = BytesIO(fnm) if not isinstance(fnm, str) else fnm
        wb = RAGFlowExcelParser._load_excel_to_workbook(file_like_object)

        res = []
        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            try:
                rows = list(ws.rows)
            except Exception as e:
                logging.warning(f"Skip sheet '{sheetname}' due to rows access error: {e}")
                continue
            if not rows:
                continue
            ti = list(rows[0])
            for r in list(rows[1:]):
                fields = []
                for i, c in enumerate(r):
                    if not c.value:
                        continue
                    t = str(ti[i].value) if i < len(ti) else ""
                    t += ("：" if t else "") + str(c.value)
                    fields.append(t)
                line = "; ".join(fields)
                if sheetname.lower().find("sheet") < 0:
                    line += " ——" + sheetname
                res.append(line)
        return res

    @staticmethod
    def row_number(fnm, binary):
        if fnm.split(".")[-1].lower().find("xls") >= 0:
            wb = RAGFlowExcelParser._load_excel_to_workbook(BytesIO(binary))
            total = 0
            
            for sheetname in wb.sheetnames:
               try:
                   ws = wb[sheetname]
                   total += len(list(ws.rows))
               except Exception as e:
                   logging.warning(f"Skip sheet '{sheetname}' due to rows access error: {e}")
                   continue
            return total

        if fnm.split(".")[-1].lower() in ["csv", "txt"]:
            encoding = find_codec(binary)
            txt = binary.decode(encoding, errors="ignore")
            return len(txt.split("\n"))


if __name__ == "__main__":
    psr = RAGFlowExcelParser()
    psr(sys.argv[1])
