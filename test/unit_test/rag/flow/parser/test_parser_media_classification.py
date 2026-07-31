import ast
import copy
import logging
import re
from collections.abc import Callable
from pathlib import Path
from typing import cast

MediaConsumer = Callable[..., list[dict[str, object]]]
MarkdownRenderer = Callable[[list[dict[str, object]], str, str], str]


class _FakeVLM:
    calls: list[object] = []

    @classmethod
    def image2base64(cls, image: object) -> str:
        cls.calls.append(image)
        return "data:image/png;base64,encoded"


def _contains_name(node: ast.AST, name: str) -> bool:
    """Return whether an AST node references the requested variable name."""
    return any(isinstance(child, ast.Name) and child.id == name for child in ast.walk(node))


def _load_mineru_media_consumer() -> MediaConsumer:
    """Isolate the production media and normalization loops for unit testing."""
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "rag" / "flow" / "parser" / "parser.py"
    module_ast = ast.parse(module_path.read_text(encoding="utf-8"), filename=str(module_path))
    parser_class = next(node for node in module_ast.body if isinstance(node, ast.ClassDef) and node.name == "Parser")
    pdf_method = next(node for node in parser_class.body if isinstance(node, ast.FunctionDef) and node.name == "_pdf")

    media_loop = next(node for node in ast.walk(pdf_method) if isinstance(node, ast.For) and _contains_name(node.iter, "mineru_media_blocks"))
    normalization_loop = next(
        node
        for node in ast.walk(pdf_method)
        if isinstance(node, ast.For) and _contains_name(node.iter, "bboxes") and any(isinstance(child, ast.Constant) and child.value == "doc_type_kwd" for child in ast.walk(node))
    )

    consumer = ast.FunctionDef(
        name="consume",
        args=ast.arguments(
            posonlyargs=[],
            args=[ast.arg(arg="mineru_media_blocks")],
            kwonlyargs=[],
            kw_defaults=[],
            defaults=[],
        ),
        body=[
            ast.Assign(targets=[ast.Name(id="bboxes", ctx=ast.Store())], value=ast.List(elts=[], ctx=ast.Load())),
            copy.deepcopy(media_loop),
            ast.Assign(
                targets=[ast.Name(id="conf", ctx=ast.Store())],
                value=ast.Dict(keys=[ast.Constant(value="remove_header_footer")], values=[ast.Constant(value=False)]),
            ),
            ast.Assign(targets=[ast.Name(id="flatten_media_to_text", ctx=ast.Store())], value=ast.Constant(value=False)),
            ast.Assign(targets=[ast.Name(id="normalize_bboxes", ctx=ast.Store())], value=ast.List(elts=[], ctx=ast.Load())),
            copy.deepcopy(normalization_loop),
            ast.Assign(targets=[ast.Name(id="bboxes", ctx=ast.Store())], value=ast.Name(id="normalize_bboxes", ctx=ast.Load())),
            ast.Return(value=ast.Name(id="bboxes", ctx=ast.Load())),
        ],
        decorator_list=[],
        type_params=[],
    )
    function_module = ast.Module(body=[consumer], type_ignores=[])
    ast.fix_missing_locations(function_module)

    namespace: dict[str, object] = {"re": re}
    exec(compile(function_module, str(module_path), "exec"), namespace)
    return cast(MediaConsumer, namespace["consume"])


def _load_pdf_markdown_renderer() -> MarkdownRenderer:
    """Isolate the production PDF markdown loop for unit testing."""
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "rag" / "flow" / "parser" / "parser.py"
    module_ast = ast.parse(module_path.read_text(encoding="utf-8"), filename=str(module_path))
    parser_class = next(node for node in module_ast.body if isinstance(node, ast.ClassDef) and node.name == "Parser")
    pdf_method = next(node for node in parser_class.body if isinstance(node, ast.FunctionDef) and node.name == "_pdf")
    markdown_loop = next(
        node
        for node in ast.walk(pdf_method)
        if isinstance(node, ast.For)
        and isinstance(node.iter, ast.Name)
        and node.iter.id == "bboxes"
        and any(isinstance(child, ast.Constant) and isinstance(child.value, str) and "![Image]" in child.value for child in ast.walk(node))
    )

    renderer = ast.FunctionDef(
        name="render",
        args=ast.arguments(
            posonlyargs=[],
            args=[ast.arg(arg="bboxes"), ast.arg(arg="name"), ast.arg(arg="parse_method")],
            kwonlyargs=[],
            kw_defaults=[],
            defaults=[],
        ),
        body=[
            ast.Assign(targets=[ast.Name(id="mkdn", ctx=ast.Store())], value=ast.Constant(value="")),
            copy.deepcopy(markdown_loop),
            ast.Return(value=ast.Name(id="mkdn", ctx=ast.Load())),
        ],
        decorator_list=[],
        type_params=[],
    )
    function_module = ast.Module(body=[renderer], type_ignores=[])
    ast.fix_missing_locations(function_module)

    namespace: dict[str, object] = {"VLM": _FakeVLM, "logging": logging}
    exec(compile(function_module, str(module_path), "exec"), namespace)
    return cast(MarkdownRenderer, namespace["render"])


def test_mineru_media_consumer_classifies_table_and_figure() -> None:
    consume = _load_mineru_media_consumer()
    table_image = object()
    figure_image = object()

    bboxes = consume(
        [
            ((table_image, "<table><tr><td>value</td></tr></table>"), [(0, 10.0, 20.0, 30.0, 40.0)]),
            ((figure_image, ["Figure caption", "", "Figure footnote"]), []),
        ]
    )

    assert bboxes == [
        {
            "layout_type": "table",
            "doc_type_kwd": "table",
            "text": "<table><tr><td>value</td></tr></table>",
            "image": table_image,
            "positions": [[1, 10.0, 20.0, 30.0, 40.0]],
        },
        {
            "layout_type": "figure",
            "doc_type_kwd": "image",
            "text": "Figure caption\nFigure footnote",
            "image": figure_image,
        },
    ]


def test_pdf_markdown_skips_figures_without_images_and_logs(caplog) -> None:
    render = _load_pdf_markdown_renderer()
    _FakeVLM.calls = []
    bboxes: list[dict[str, object]] = [
        {"layout_type": "figure", "text": "missing image key"},
        {"layout_type": "figure", "image": None, "text": "null image"},
        {"layout_type": "text", "text": "Document body"},
    ]

    with caplog.at_level(logging.WARNING):
        markdown = render(bboxes, "document.pdf", "MinerU")

    warnings = [record.getMessage() for record in caplog.records if "Skipping figure in markdown output" in record.getMessage()]
    assert markdown == "Document body\n"
    assert _FakeVLM.calls == []
    assert len(warnings) == 2
    assert all("document.pdf" in message and "parse_method=MinerU" in message for message in warnings)


def test_pdf_markdown_renders_available_figure_images() -> None:
    render = _load_pdf_markdown_renderer()
    image = object()
    _FakeVLM.calls = []

    markdown = render([{"layout_type": "figure", "image": image}], "document.pdf", "MinerU")

    assert markdown == "\n![Image](data:image/png;base64,encoded)"
    assert _FakeVLM.calls == [image]
