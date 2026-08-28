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
import logging


logger = logging.getLogger(__name__)


def media_records_to_bboxes(media_records, source):
    """Convert parser media records into flow parser bbox records.

    Parser backends return media as ``((image, html_or_caption), positions)``.
    Flow parser outputs use one-based page numbers, while parser crop positions
    are zero-based.

    Args:
        media_records: Tables and figures returned by a parser backend.
        source: Parser name included in count-only diagnostic logs.
    """
    bboxes = []
    skipped_records = 0
    skipped_positions = 0
    for table in media_records or []:
        if not isinstance(table, tuple) or len(table) != 2:
            skipped_records += 1
            continue
        media, positions = table
        if not isinstance(media, tuple) or len(media) != 2:
            skipped_records += 1
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
                skipped_positions += 1
        bboxes.append(box)
    logger.debug(
        "[%s] Converted %d media records; skipped %d malformed records and %d invalid position sets.",
        source,
        len(bboxes),
        skipped_records,
        skipped_positions,
    )
    return bboxes


def docling_tables_to_bboxes(tables):
    """Convert Docling tables and figures into flow parser bbox records."""
    return media_records_to_bboxes(tables, "Docling")


def order_docling_bboxes(bboxes):
    """Return Docling bboxes in stable page-coordinate reading order.

    Docling returns text and media in separate collections. Coordinates are the
    shared ordering signal; records without usable coordinates remain at the
    end in their original relative order.
    """

    def reading_order_key(index_and_box):
        index, box = index_and_box
        positions = box.get("positions") if isinstance(box, dict) else None
        if isinstance(positions, list):
            for position in positions:
                if not isinstance(position, (list, tuple)) or len(position) < 5:
                    continue
                try:
                    return (0, int(position[0]), float(position[3]), float(position[1]), index)
                except (TypeError, ValueError):
                    continue
        return (1, 0, 0, 0, index)

    return [box for _, box in sorted(enumerate(bboxes), key=reading_order_key)]
