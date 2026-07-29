import ast
from collections.abc import Callable
from pathlib import Path
from types import SimpleNamespace
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
