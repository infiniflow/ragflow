import logging

from rag.flow.parser.docling import docling_tables_to_bboxes, media_records_to_bboxes, order_docling_bboxes


def test_docling_tables_and_figures_are_converted_to_flow_bboxes():
    table_image = object()
    figure_image = object()

    bboxes = docling_tables_to_bboxes(
        [
            ((table_image, "<table><tr><td>Revenue</td></tr></table>"), [(0, 1, 2, 3, 4)]),
            ((figure_image, ["Quarterly revenue chart"]), [(2, 5, 6, 7, 8)]),
        ]
    )

    assert bboxes == [
        {
            "layout_type": "table",
            "text": "<table><tr><td>Revenue</td></tr></table>",
            "image": table_image,
            "positions": [[1, 1, 2, 3, 4]],
        },
        {
            "layout_type": "figure",
            "text": "Quarterly revenue chart",
            "image": figure_image,
            "positions": [[3, 5, 6, 7, 8]],
        },
    ]


def test_docling_tables_skips_malformed_entries_without_dropping_valid_ones(caplog):
    with caplog.at_level(logging.DEBUG, logger="rag.flow.parser.docling"):
        bboxes = docling_tables_to_bboxes(
            [
                "not a Docling table",
                ((None, "<table></table>"), "not a position list"),
            ]
        )

    assert bboxes == [{"layout_type": "table", "text": "<table></table>"}]
    assert "skipped 1 malformed records and 1 invalid position sets" in caplog.text


def test_docling_figure_without_an_image_keeps_its_extracted_text():
    bboxes = docling_tables_to_bboxes([((None, ["OCR text from the figure"]), [])])

    assert bboxes == [{"layout_type": "figure", "text": "OCR text from the figure"}]


def test_media_record_conversion_is_shared_by_parser_backends(caplog):
    with caplog.at_level(logging.DEBUG, logger="rag.flow.parser.docling"):
        bboxes = media_records_to_bboxes([((None, "<table></table>"), [(0, 1, 2, 3, 4)])], "OpenDataLoader")

    assert bboxes == [{"layout_type": "table", "text": "<table></table>", "positions": [[1, 1, 2, 3, 4]]}]
    assert "[OpenDataLoader] Converted 1 media records" in caplog.text


def test_docling_bboxes_are_interleaved_in_stable_coordinate_order():
    unordered = [
        {"text": "text below", "positions": [[1, 0, 10, 80, 90]]},
        {"text": "unpositioned figure", "layout_type": "figure"},
        {"text": "table in the middle", "positions": [[1, 0, 10, 40, 50]]},
        {"text": "text on page two", "positions": [[2, 0, 10, 10, 20]]},
        {"text": "text above", "positions": [[1, 0, 10, 10, 20]]},
        {"text": "another unpositioned figure", "layout_type": "figure"},
    ]

    assert [box["text"] for box in order_docling_bboxes(unordered)] == [
        "text above",
        "table in the middle",
        "text below",
        "text on page two",
        "unpositioned figure",
        "another unpositioned figure",
    ]
