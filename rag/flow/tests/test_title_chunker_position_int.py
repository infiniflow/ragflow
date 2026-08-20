import asyncio
import importlib
import sys
import types
from contextlib import contextmanager
from pathlib import Path

"""Reproduction test for the Book builtin DSL highlight path.

The Book builtin DSL parses PDF with ``parse_method=DeepDOC`` and
``output_format=json``, then chunks with ``TitleChunker`` (method=hierarchy).
This test drives the real TitleChunker chain (extract -> merge -> finalize)
with deepdoc-style JSON items that carry ``positions``, and asserts the
emitted chunks keep a non-empty ``position_int`` covering every source page.

It is a GREEN regression guard: it proves the TitleChunker layer preserves
PDF coordinates for the Book DSL, which localizes the parsing-result "missing
highlight" bug to the parser emission / dataset config rather than the
chunker. Mirrors rag/flow/tests/test_token_chunker.py's
``test_json_delimiter_mode_position_int_survives_full_chain``.
"""


@contextmanager
def _load_title_chunker_with_stubs():
    root = Path(__file__).resolve().parents[3]
    original_modules = {}

    def _install(name: str, module: types.ModuleType):
        original_modules.setdefault(name, sys.modules.get(name))
        sys.modules[name] = module

    try:
        rag_pkg = types.ModuleType("rag")
        rag_pkg.__path__ = [str(root / "rag")]
        _install("rag", rag_pkg)

        rag_flow_pkg = types.ModuleType("rag.flow")
        rag_flow_pkg.__package__ = "rag"
        rag_flow_pkg.__path__ = [str(root / "rag" / "flow")]
        _install("rag.flow", rag_flow_pkg)

        rag_flow_chunker_pkg = types.ModuleType("rag.flow.chunker")
        rag_flow_chunker_pkg.__package__ = "rag.flow"
        rag_flow_chunker_pkg.__path__ = [str(root / "rag" / "flow" / "chunker")]
        _install("rag.flow.chunker", rag_flow_chunker_pkg)

        rag_flow_parser_pkg = types.ModuleType("rag.flow.parser")
        rag_flow_parser_pkg.__package__ = "rag.flow"
        rag_flow_parser_pkg.__path__ = [str(root / "rag" / "flow" / "parser")]
        _install("rag.flow.parser", rag_flow_parser_pkg)

        common_pkg = types.ModuleType("common")
        common_pkg.__path__ = [str(root / "common")]
        _install("common", common_pkg)

        common_float_utils = types.ModuleType("common.float_utils")
        common_float_utils.normalize_overlapped_percent = lambda value: value
        _install("common.float_utils", common_float_utils)

        common_token_utils = types.ModuleType("common.token_utils")
        common_token_utils.num_tokens_from_string = lambda text: 1
        common_token_utils.truncate = lambda text, max_len: (text or "")[:max_len]
        _install("common.token_utils", common_token_utils)

        rag_nlp = types.ModuleType("rag.nlp")
        rag_nlp.naive_merge = lambda *args, **kwargs: []
        rag_nlp.not_bullet = lambda text: False
        rag_nlp.not_title = lambda text: True
        _install("rag.nlp", rag_nlp)

        deepdoc_pkg = types.ModuleType("deepdoc")
        deepdoc_pkg.__path__ = [str(root / "deepdoc")]
        _install("deepdoc", deepdoc_pkg)

        deepdoc_parser_pkg = types.ModuleType("deepdoc.parser")
        deepdoc_parser_pkg.__path__ = [str(root / "deepdoc" / "parser")]
        _install("deepdoc.parser", deepdoc_parser_pkg)

        class _RAGFlowPdfParser:
            @staticmethod
            def remove_tag(text):
                return text

            @staticmethod
            def extract_positions(tag):
                return []

        deepdoc_pdf_parser = types.ModuleType("deepdoc.parser.pdf_parser")
        deepdoc_pdf_parser.RAGFlowPdfParser = _RAGFlowPdfParser
        _install("deepdoc.parser.pdf_parser", deepdoc_pdf_parser)

        deepdoc_parser_utils = types.ModuleType("deepdoc.parser.utils")
        deepdoc_parser_utils.extract_pdf_outlines = lambda *args, **kwargs: []
        _install("deepdoc.parser.utils", deepdoc_parser_utils)

        class ProcessParamBase:
            def __init__(self):
                pass

            def check_valid_value(self, value, msg, allowed):
                if value not in allowed:
                    raise ValueError(msg)

            def check_positive_integer(self, value, msg):
                pass

            def check_decimal_float(self, value, msg):
                pass

            def check_nonnegative_number(self, value, msg):
                pass

        class ProcessBase:
            def __init__(self, _pipeline, _id, param):
                self._pipeline = _pipeline
                self._id = _id
                self._param = param
                self._outputs = {}
                self.callback = lambda *_args, **_kwargs: None

            def set_output(self, key, value):
                self._outputs[key] = value

        rag_flow_base = types.ModuleType("rag.flow.base")
        rag_flow_base.ProcessBase = ProcessBase
        rag_flow_base.ProcessParamBase = ProcessParamBase
        _install("rag.flow.base", rag_flow_base)

        pdf_chunk_metadata = types.ModuleType("rag.flow.parser.pdf_chunk_metadata")
        pdf_chunk_metadata.PDF_POSITIONS_KEY = "pdf_positions"
        pdf_chunk_metadata.extract_pdf_positions = lambda _item: []
        pdf_chunk_metadata.merge_pdf_positions = lambda _records: []
        pdf_chunk_metadata.finalize_pdf_chunk = lambda chunk: chunk
        pdf_chunk_metadata.restore_pdf_text_previews = lambda *_a, **_k: None
        _install("rag.flow.parser.pdf_chunk_metadata", pdf_chunk_metadata)

        common_spec = importlib.util.spec_from_file_location(
            "rag.flow.chunker.title_chunker.common",
            root / "rag" / "flow" / "chunker" / "title_chunker" / "common.py",
        )
        common_module = importlib.util.module_from_spec(common_spec)
        _install("rag.flow.chunker.title_chunker.common", common_module)
        common_spec.loader.exec_module(common_module)

        hierarchy_spec = importlib.util.spec_from_file_location(
            "rag.flow.chunker.title_chunker.hierarchy_chunker",
            root / "rag" / "flow" / "chunker" / "title_chunker" / "hierarchy_chunker.py",
        )
        hierarchy_module = importlib.util.module_from_spec(hierarchy_spec)
        _install("rag.flow.chunker.title_chunker.hierarchy_chunker", hierarchy_module)
        hierarchy_spec.loader.exec_module(hierarchy_module)

        yield common_module, hierarchy_module
    finally:
        for module_name, original in original_modules.items():
            if original is None:
                sys.modules.pop(module_name, None)
            else:
                sys.modules[module_name] = original


def _real_extract_pdf_positions(item):
    # Faithful mirror of rag/flow/parser/pdf_chunk_metadata.extract_pdf_positions.
    if not isinstance(item, dict):
        return []
    positions = item.get("pdf_positions")
    if isinstance(positions, list):
        return [list(p) for p in positions]
    positions = item.get("positions")
    if isinstance(positions, list):
        return [list(p) for p in positions]
    position_tag = item.get("position_tag")
    if isinstance(position_tag, str) and position_tag:
        return []  # RAGFlowPdfParser.extract_positions is stubbed out here.
    position_int = item.get("position_int")
    if isinstance(position_int, list):
        return [list(p) for p in position_int if isinstance(p, (list, tuple)) and len(p) >= 5]
    return []


def _real_merge_pdf_positions(records):
    # Faithful mirror of rag/flow/parser/pdf_chunk_metadata.merge_pdf_positions.
    merged = []
    for rec in records or []:
        if not isinstance(rec, dict):
            continue
        for pos in rec.get("pdf_positions") or []:
            if isinstance(pos, (list, tuple)) and len(pos) >= 5:
                merged.append([pos[0], pos[1], pos[2], pos[3], pos[4]])
    seen = set()
    out = []
    for pos in merged:
        key = tuple(pos[:5])
        if key not in seen:
            seen.add(key)
            out.append(pos)
    out.sort(key=lambda item: (item[0], item[3], item[1]))
    return out


def _real_finalize_pdf_chunk(chunk):
    # Faithful mirror of rag/flow/parser/pdf_chunk_metadata.finalize_pdf_chunk.
    positions = _real_extract_pdf_positions(chunk)
    if positions:
        chunk["position_int"] = [list(p) for p in positions]
        chunk.pop("pdf_positions", None)
    return chunk


def test_title_chunker_preserves_position_int_from_deepdoc_json():
    # Reproduces the parsing-result highlight verification for the Book builtin
    # DSL (infiniflow/ragflow#18148 follow-up): a deepdoc + json parser output
    # carrying per-item ``positions`` must flow through TitleChunker (method=
    # hierarchy) and reach ``position_int`` on the emitted chunks. This proves
    # the TitleChunker layer is NOT the cause of the missing highlight.
    with _load_title_chunker_with_stubs() as (common_module, hierarchy_module):
        # Install faithful coordinate helpers so the REAL TitleChunker path runs.
        common_module.extract_pdf_positions = _real_extract_pdf_positions
        common_module.merge_pdf_positions = _real_merge_pdf_positions
        common_module.finalize_pdf_chunk = _real_finalize_pdf_chunk

        async def _restore_previews(*_a, **_k):
            return None

        common_module.restore_pdf_text_previews = _restore_previews

        json_result = [
            {"text": "Introduction paragraph on page one.", "doc_type_kwd": "text", "positions": [[1, 10, 200, 50, 80]]},
            {"text": "Body text continues on page one.", "doc_type_kwd": "text", "positions": [[1, 12, 205, 90, 120]]},
            {"text": "Second chapter starts on page two.", "doc_type_kwd": "text", "positions": [[2, 15, 210, 40, 75]]},
        ]

        from_upstream = types.SimpleNamespace(
            output_format="json",
            json_result=json_result,
            markdown_result=None,
            text_result=None,
            html_result=None,
            chunks=None,
            file=None,
            name="book-test",  # not *.pdf -> restore_pdf_text_previews early-returns
        )

        param = common_module.TitleChunkerParam()
        param.method = "hierarchy"
        param.hierarchy = 1
        param.levels = []  # no headings -> all body -> single merged chunk
        param.include_heading_content = False
        param.root_chunk_as_heading = False
        param.chunk_token_cap = 0  # keep this a pure position regression (no re-split)

        process = common_module.ProcessBase(None, "title_chunker", param)
        process._canvas = types.SimpleNamespace(_doc_id="doc", _tenant_id="tenant")
        process._outputs = {}

        chunker = hierarchy_module.HierarchyTitleChunker(process, from_upstream)
        asyncio.run(chunker.invoke())

        chunks = process._outputs.get("chunks", [])
        assert chunks, "TitleChunker produced no chunks"
        pos_int = chunks[0].get("position_int")
        assert pos_int, "position_int missing from TitleChunker output"
        pages = {p[0] for p in pos_int}
        assert pages == {1, 2}, f"expected pages {{1,2}}, got {pages}"
