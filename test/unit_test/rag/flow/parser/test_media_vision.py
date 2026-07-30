import ast
import re
import sys
from collections import defaultdict
from collections.abc import Callable
from pathlib import Path
from types import ModuleType, SimpleNamespace
from typing import cast

MediaSection = dict[str, object]
MediaVisionFunction = Callable[..., list[MediaSection]]


def _load_media_vision_function(namespace_overrides: dict[str, object]) -> MediaVisionFunction:
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "rag" / "flow" / "parser" / "utils.py"
    module_ast = ast.parse(module_path.read_text(encoding="utf-8"), filename=str(module_path))
    function_node = next(node for node in module_ast.body if isinstance(node, ast.FunctionDef) and node.name == "enhance_media_sections_with_vision")
    function_module = ast.Module(body=[function_node], type_ignores=[])
    ast.fix_missing_locations(function_module)

    namespace: dict[str, object] = {"LLMType": SimpleNamespace(VISION="vision")}
    namespace.update(namespace_overrides)
    exec(compile(function_module, str(module_path), "exec"), namespace)
    return cast(MediaVisionFunction, namespace["enhance_media_sections_with_vision"])


class _FakeVisionFigureParser:
    calls: list[dict[str, object]] = []

    def __init__(self, **kwargs: object) -> None:
        self.calls.append(kwargs)

    def __call__(self, **_kwargs: object) -> list[tuple[tuple[object, str], list[tuple[int, int, int, int, int]]]]:
        return [((object(), "vision description"), [(0, 0, 0, 0, 0)])]


def _load_vision_figure_parser_class() -> type:
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "deepdoc" / "parser" / "figure_parser.py"
    module_ast = ast.parse(module_path.read_text(encoding="utf-8"), filename=str(module_path))
    class_node = next(node for node in module_ast.body if isinstance(node, ast.ClassDef) and node.name == "VisionFigureParser")
    class_module = ast.Module(body=[class_node], type_ignores=[])
    ast.fix_missing_locations(class_module)
    namespace: dict[str, object] = {"ensure_pil_image": lambda image: image}
    exec(compile(class_module, str(module_path), "exec"), namespace)
    return cast(type, namespace["VisionFigureParser"])


def _load_pdf_context_function(monkeypatch) -> Callable[..., list[object]]:
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "rag" / "nlp" / "__init__.py"
    module_ast = ast.parse(module_path.read_text(encoding="utf-8"), filename=str(module_path))
    function_node = next(node for node in module_ast.body if isinstance(node, ast.FunctionDef) and node.name == "append_context2table_image4pdf")
    function_module = ast.Module(body=[function_node], type_ignores=[])
    ast.fix_missing_locations(function_module)

    deepdoc_module = ModuleType("deepdoc")
    parser_module = ModuleType("deepdoc.parser")
    parser_module.PdfParser = type("PdfParser", (), {"extract_positions": staticmethod(lambda _text: [])})
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_module)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_module)
    namespace: dict[str, object] = {
        "defaultdict": defaultdict,
        "num_tokens_from_string": lambda text: len(text.split()),
        "re": re,
    }
    exec(compile(function_module, str(module_path), "exec"), namespace)
    return cast(Callable[..., list[object]], namespace["append_context2table_image4pdf"])


def test_media_vision_enriches_images_without_reprocessing_tables() -> None:
    _FakeVisionFigureParser.calls = []
    enhance_media_sections_with_vision = _load_media_vision_function(
        {
            "resolve_model_config": lambda *_args: object(),
            "get_tenant_default_model_by_type": lambda *_args: object(),
            "LLMBundle": lambda *_args: object(),
            "VisionFigureParser": _FakeVisionFigureParser,
        }
    )
    image = object()
    table_image = object()
    sections: list[MediaSection] = [
        {"doc_type_kwd": "image", "image": image, "text": "figure caption"},
        {"doc_type_kwd": "table", "image": table_image, "text": "<table><tr><td>1</td></tr></table>"},
        {"doc_type_kwd": "text", "text": "body"},
    ]

    result = enhance_media_sections_with_vision(
        sections,
        tenant_id="tenant-1",
        vlm_conf={"llm_id": "vision-model"},
        callback=lambda *_args, **_kwargs: None,
    )

    assert result is sections
    assert sections[0]["text"] == "figure caption\nvision description"
    assert sections[1]["text"] == "<table><tr><td>1</td></tr></table>"
    assert sections[2]["text"] == "body"
    assert len(_FakeVisionFigureParser.calls) == 1
    assert _FakeVisionFigureParser.calls[0]["figures_data"] == [((image, [""]), [(0, 0, 0, 0, 0)])]


def test_vision_figure_parser_preserves_empty_and_mixed_positions() -> None:
    parser_class = _load_vision_figure_parser_class()
    image_without_position = object()
    positioned_image = object()
    positioned = [(3, 10, 20, 30, 40)]
    parser = parser_class(
        vision_model=object(),
        figures_data=[
            ((image_without_position, ["without position"]), []),
            ((positioned_image, ["positioned"]), positioned),
        ],
    )

    assert parser._assemble() == [
        ((image_without_position, ["without position"]), []),
        ((positioned_image, ["positioned"]), positioned),
    ]


def test_pdf_media_context_preserves_items_without_positions(monkeypatch) -> None:
    append_context = _load_pdf_context_function(monkeypatch)
    media_item = ((object(), ["description"]), [])

    assert append_context([], [media_item], table_context_size=32) == [media_item]
    assert append_context([], [media_item], table_context_size=32, return_context=True) == [("", "")]
