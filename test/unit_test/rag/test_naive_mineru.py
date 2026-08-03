import ast
import logging
from collections.abc import Callable
from pathlib import Path
from types import SimpleNamespace
from typing import cast

import pytest


ByMinerU = Callable[..., tuple[object, object, object]]


def _load_by_mineru(namespace_overrides: dict[str, object]) -> ByMinerU:
    repo_root = Path(__file__).resolve().parents[3]
    module_path = repo_root / "rag" / "app" / "naive.py"
    module_ast = ast.parse(module_path.read_text(encoding="utf-8"), filename=str(module_path))
    function_node = next(node for node in module_ast.body if isinstance(node, ast.FunctionDef) and node.name == "by_mineru")
    function_module = ast.Module(body=[function_node], type_ignores=[])
    ast.fix_missing_locations(function_module)

    namespace: dict[str, object] = {
        "MAXIMUM_PAGE_NUMBER": 100000,
        "LLMType": SimpleNamespace(OCR="ocr"),
        "logging": logging,
    }
    namespace.update(namespace_overrides)
    exec(compile(function_module, str(module_path), "exec"), namespace)
    return cast(ByMinerU, namespace["by_mineru"])


class _FakeMinerUParser:
    def __init__(self) -> None:
        self.parse_kwargs: dict[str, object] | None = None
        self.sections = [("Document text", "")]
        self.tables = [(("image", ["caption"]), [(0, 1.0, 2.0, 3.0, 4.0)])]

    def parse_pdf(self, **kwargs: object) -> tuple[object, object]:
        self.parse_kwargs = kwargs
        return self.sections, self.tables


def _build_by_mineru(parser: _FakeMinerUParser, wrapper_calls: list[dict[str, object]]) -> ByMinerU:
    def vision_wrapper(**kwargs: object) -> object:
        wrapper_calls.append(kwargs)
        return [("enriched", kwargs["tbls"])]

    return _load_by_mineru(
        {
            "get_first_provider_model_name": lambda *_args: "mineru-model",
            "ensure_mineru_from_env": lambda *_args: None,
            "resolve_model_config": lambda *_args: object(),
            "LLMBundle": lambda **_kwargs: SimpleNamespace(mdl=parser),
            "vision_figure_parser_pdf_wrapper": vision_wrapper,
        }
    )


def test_by_mineru_raw_keeps_general_vision_enrichment() -> None:
    parser = _FakeMinerUParser()
    wrapper_calls: list[dict[str, object]] = []
    by_mineru = _build_by_mineru(parser, wrapper_calls)

    sections, tables, returned_parser = by_mineru(
        filename="document.pdf",
        binary=b"pdf",
        callback=lambda *_args, **_kwargs: None,
        tenant_id="tenant-1",
        mineru_llm_name="mineru-model",
    )

    assert sections == parser.sections
    assert tables == [("enriched", parser.tables)]
    assert returned_parser is parser
    assert parser.parse_kwargs is not None
    assert "vision_model" not in parser.parse_kwargs
    assert len(wrapper_calls) == 1
    assert wrapper_calls[0]["tenant_id"] == "tenant-1"
    assert wrapper_calls[0]["sections"] == parser.sections


@pytest.mark.parametrize("parse_method", ["manual", "paper"])
def test_by_mineru_defers_non_raw_vision_enrichment_to_chunk_method(parse_method: str) -> None:
    parser = _FakeMinerUParser()
    wrapper_calls: list[dict[str, object]] = []
    by_mineru = _build_by_mineru(parser, wrapper_calls)

    _, tables, _ = by_mineru(
        filename="document.pdf",
        binary=b"pdf",
        callback=lambda *_args, **_kwargs: None,
        tenant_id="tenant-1",
        mineru_llm_name="mineru-model",
        parse_method=parse_method,
    )

    assert tables == parser.tables
    assert wrapper_calls == []
    assert parser.parse_kwargs is not None
    assert "vision_model" not in parser.parse_kwargs
