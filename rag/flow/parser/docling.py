#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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


def docling_tables_to_bboxes(tables):
    """Convert Docling tables and figures into flow parser bbox records.

    ``DoclingParser.parse_pdf`` returns tables as ``((image, html_or_caption),
    positions)``.  Flow parser outputs use one-based page numbers, while
    Docling's crop positions are zero-based.
    """
    bboxes = []
    for table in tables or []:
        if not isinstance(table, tuple) or len(table) != 2:
            continue
        media, positions = table
        if not isinstance(media, tuple) or len(media) != 2:
            continue
        image, html_or_caption = media
        box = {"layout_type": "table" if isinstance(html_or_caption, str) else "figure"}
        if isinstance(html_or_caption, str):
            box["text"] = html_or_caption
        elif isinstance(html_or_caption, list):
            box["text"] = html_or_caption[0] if html_or_caption else ""
        else:
            box["text"] = ""
        if image is not None:
            box["image"] = image
        if positions:
            try:
                box["positions"] = [[p[0] + 1, p[1], p[2], p[3], p[4]] for p in positions]
            except (IndexError, TypeError):
                pass
        bboxes.append(box)
    return bboxes
