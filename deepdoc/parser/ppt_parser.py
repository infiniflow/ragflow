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
from dataclasses import dataclass
from typing import TYPE_CHECKING, Union
from unicodedata import east_asian_width
from pptx import Presentation
import logging
import os
from io import BytesIO


if TYPE_CHECKING:
    import pptx.table
    import pptx.text.text


_TableSpanDict = dict[tuple[int, int], str]


@dataclass
class _ParagraphPart:
    content: str
    is_list: bool = False
    # these 2 available when is_list is True
    indent_level: int = 0
    ordered: bool = False


class RAGFlowPptParser:
    def __init__(self):
        super().__init__()

    def _sanitize_input(self, fnm: Union[str, bytes]) -> BytesIO:
        """统一处理输入，确保返回安全的BytesIO对象"""
        try:
            if isinstance(fnm, str):
                # 处理文件路径：移除空字节并标准化路径
                sanitized_path = fnm.replace('\x00', '')
                sanitized_path = os.path.normpath(sanitized_path)
                with open(sanitized_path, 'rb') as f:
                    return BytesIO(f.read())
            elif isinstance(fnm, bytes):
                return BytesIO(fnm)
            elif hasattr(fnm, 'read'):  # 支持文件类对象
                return BytesIO(fnm.read())
            else:
                raise ValueError("Unsupported input type")
        except Exception as e:
            logging.error(f"Input sanitization failed: {str(e)}")
            raise RuntimeError("Failed to process input file") from e

    @staticmethod
    def __get_bulleted_text(paragraph):
        is_bulleted = bool(paragraph._p.xpath("./a:pPr/a:buChar"))
        if is_bulleted:
            return f"{'  ' * paragraph.level}- {paragraph.text}"
        return paragraph.text

    @staticmethod
    def __get_paragraph_part(
            paragraph: "pptx.text.text._Paragraph"  # noqa: _Paragraph is not exported but used
    ) -> _ParagraphPart:
        if not paragraph.text.strip():
            return _ParagraphPart('')

        xml = paragraph._p  # noqa: pptx does not support the access of order/unorderd list. XML is required
        is_unordered_list = bool(xml.xpath("./a:pPr/a:buChar"))
        is_ordered_list = bool(xml.xpath("./a:pPr/a:buAutoNum"))
        ret = _ParagraphPart(paragraph.text)
        if is_unordered_list or is_ordered_list:
            ret.is_list = True
            ret.ordered = is_ordered_list
            ret.indent_level = paragraph.level
        return ret

    @staticmethod
    def __get_unicode_display_len(s: str) -> int:
        num_wchar = sum(1 for c in s if east_asian_width(c) in ('W', 'F'))
        return len(s) + num_wchar

    @classmethod
    def __format_table_col_width(cls, table: list[list[str]]) -> list[int]:
        """Padding the param `table` and returns the column width (ignore column margin)"""
        num_cols = len(table[0])
        num_rows = len(table)

        raw_width_table = [
            [cls.__get_unicode_display_len(cell) for cell in row]
            for row in table
        ]
        col_actual_width: list[int] = []

        max_formatted_width = max(120 // num_cols - 3, 1)
        for x in range(num_cols):
            col_width = max(row[x] for row in raw_width_table)
            col_width = min(max_formatted_width, col_width)
            col_actual_width.append(col_width)

            for y in range(num_rows):
                if raw_width_table[y][x] < col_width:
                    padding_len = col_width - raw_width_table[y][x]
                    padding_left = padding_len // 2
                    table[y][x] = (' ' * padding_left) + table[y][x] + (' ' * (padding_len - padding_left))
        return col_actual_width

    @classmethod
    def __format_paragraph(cls, has_text_frame, can_escape_list: bool) -> list[str]:
        """
        Format any paragraph from anything with text frame and returns a list of parsed paragraph.

        `has_text_frame` is known that can be table cell or a shape which `shape.has_text_frame == True`.
        `can_escape_list` should be True when the frame block can regard `-` as the beginning of a list item.
        If `can_escape_list` is True, a empty line is inserted between a list item and a normal paragraph.
        """
        formatted = []
        last_is_list = False
        index_stack = [0]    # len of it equals to `indent + 1`
        for paragraph in has_text_frame.text_frame.paragraphs:
            part = cls.__get_paragraph_part(paragraph)
            # quick path of empty content. Empty line do not disturb the index
            if not part.content:
                formatted.append('')
                continue
            # index update for ordered list
            index_stack = index_stack[:part.indent_level + 1]
            index_stack.extend(0 for _ in range(len(index_stack), part.indent_level + 1))
            if not part.ordered:
                index_stack[-1] = 0

            # format this paragraph
            indent = '   ' * part.indent_level
            if part.is_list:
                if part.ordered:
                    index_stack[-1] += 1
                    prefix = f"{index_stack[-1]}. "
                else:
                    prefix = '- '
                last_is_list = True
            else:
                prefix = ''
                if last_is_list and can_escape_list:
                    formatted.append('')
                last_is_list = False
            formatted.append(indent + prefix + part.content)
        if len(formatted) < 2:
            return formatted
        ret = [formatted[0]]
        ret.extend(x for x in formatted[1:] if (ret[-1] != '') or (x != ''))
        return ret

    @classmethod
    def __format_paragraph_with_fallback(cls, has_text_frame, can_escape_list: bool) -> list[str]:
        try:
            return cls.__format_paragraph(has_text_frame, can_escape_list)
        except Exception as ignore:
            return [
                cls.__get_bulleted_text(paragraph)
                for paragraph in has_text_frame.text_frame.paragraphs
                if paragraph.text.strip()
            ]

    def __extract(self, shape):
        if shape.shape_type == 19:  # 表格
            try:
                return self.__format_table(shape.table)
            except Exception as ignore:
                logging.error("err", exc_info=True)
                return "\n".join(
                    "; ".join(cell.text for cell in row.cells)
                    for row in shape.table.rows
                )
        elif shape.has_text_frame:  # 文本框
            return "\n".join(self.__format_paragraph_with_fallback(shape, True))
        elif shape.shape_type == 6:  # 组合形状
            return "\n".join(
                self.__extract(sub_shape)
                for sub_shape in sorted(
                    shape.shapes,
                    key=lambda x: (x.top, x.left)
                )
            )
        return ""

    def __call__(self, fnm, from_page, to_page, callback=None):
        try:
            # 统一输入处理
            ppt_stream = self._sanitize_input(fnm)

            # """
            # convert_ppt_to_txts使用说明：
            # 使用该工具需要在环境变量中按照rag\utils\ppt_to_txts\test\.env.example文件中的例子配置额外的环境变量。
            # 注意访问.env.example中的MINERU_URL需要切换到公司德国代理。
            #
            # 如果环境变量中存在MINERU_URL与VL_URL、VL_API_KEY、VL_MODEL，会使用该工具进行PPT图文提取，并使用VL模型对图片进行描述。
            # 如果环境变量中只存在MINERU_URL，会使用该工具进行PPT图文提取，不会对图片做额外描述。
            # 如果环境变量中不存在MINERU_URL，会使用原来代码中的python-pptx提取方法（下方else:... ...）。
            #
            # 使用convert_ppt_to_txts工具后，ppt中的图片会被储存到"pptx-images"的bucket中。
            # """
            mineru_url = os.getenv('MINERU_URL')
            if mineru_url:
                from rag.utils.ppt_to_txts import convert_ppt_to_txts
                txts = convert_ppt_to_txts(ppt_stream)
            else:
                ppt = Presentation(ppt_stream)
                txts = []
                total_slides = len(ppt.slides)

                # 安全处理页码范围
                from_page = max(0, min(from_page, total_slides - 1))
                to_page = min(max(to_page, from_page + 1), total_slides)

                for i in range(from_page, to_page):
                    try:
                        slide = ppt.slides[i]
                        texts = []
                        for shape in sorted(slide.shapes, key=lambda x: (x.top, x.left)):
                            try:
                                if txt := self.__extract(shape):
                                    texts.append(txt)
                            except Exception as shape_error:
                                logging.warning(f"Shape extraction error on slide {i}: {shape_error}")
                        txts.append("\n\n".join(texts))

                    except Exception as slide_error:
                        logging.error(f"Slide processing failed at index {i}: {slide_error}")
                        txts.append("")  # 保证输出与幻灯片数量一致

            return txts
        except Exception as e:
            logging.critical(f"PPT processing failed: {str(e)}")
            raise RuntimeError(f"PPT parsing error: {str(e)}") from e

    def __format_table(self, table: "pptx.table.Table") -> str:
        """Reformat a table into Markdown format."""
        if not len(table.rows):
            return ''

        contents: list[list[str]] = []
        span_from: dict[tuple[int, int], str] = {}  # span src table of (y, x) -> src_text

        for y, row in enumerate(table.rows):
            formatted_row = []
            for x, cell in enumerate(row.cells):
                formatted_row.append(self.__format_table_cell(y, x, cell, span_from))
            contents.append(formatted_row)
        if span_from:
            logging.warning(f"This pptx may be incorrect: {len(span_from)} unused span items.")
            span_from.clear()

        num_cols = max(len(i) for i in contents)
        for row in contents:
            row_width = len(row)
            if row_width == num_cols:
                continue
            logging.warning(f"This pptx may be incorrect: number of cells in row {row_width} "
                            f"is {len(row)} but the table width is {num_cols}.")
            for _ in range(num_cols - row_width):
                row.append('')

        sep_lines = ['-' * w for w in self.__format_table_col_width(contents)]
        contents.insert(1, sep_lines)

        return '\n'.join(
            ('| ' + ' | '.join(row) + ' |')
            for row in contents
        )

    @classmethod
    def __format_table_cell(
            cls,
            y: int, x: int,
            cell: "pptx.table._Cell",   # noqa: '_Cell' not exported but used
            span: dict[tuple[int, int], str]) -> str:
        """Return content of a cell in html line-break format."""
        if cell.is_spanned:
            if (y, x) in span:
                return span.pop((y, x))
            logging.warning(f"Table cell at row {y}, col {x} has a span flag but cannot find its source.")
        ret = '<br>'.join(cls.__format_paragraph_with_fallback(cell, False)).replace('|', r'\|')
        if (cell.span_width > 1) or (cell.span_height > 1):
            for y_i in range(cell.span_height):
                for x_i in range(cell.span_width):
                    if x_i == y_i == 0:
                        continue
                    span[(y + y_i, x + x_i)] = ret
        return ret