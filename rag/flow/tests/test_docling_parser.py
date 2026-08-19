from rag.flow.parser.docling import docling_tables_to_bboxes


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


def test_docling_tables_skips_malformed_entries_without_dropping_valid_ones():
    bboxes = docling_tables_to_bboxes(
        [
            "not a Docling table",
            ((None, "<table></table>"), "not a position list"),
        ]
    )

    assert bboxes == [{"layout_type": "table", "text": "<table></table>"}]


def test_docling_figure_without_an_image_keeps_its_extracted_text():
    bboxes = docling_tables_to_bboxes([((None, ["OCR text from the figure"]), [])])

    assert bboxes == [{"layout_type": "figure", "text": "OCR text from the figure"}]
