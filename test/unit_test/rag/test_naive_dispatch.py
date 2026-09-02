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

"""Regression tests for the PDF parser dispatch in :mod:`rag.app.naive`.

Issue #17114: a document whose ``parser_config.layout_recognize`` was a stale
``TenantModel`` UUID (rather than the literal ``"MinerU"`` keyword) was being
dispatched to :func:`by_plaintext`, which tried to resolve the id as an
IMAGE2TEXT vision model and failed with
``Provider <empty> not found for model <id>``.

The fix routes the dispatch to :func:`by_mineru` whenever
``layout_recognize`` does not match any known parser name AND the parser
config carries MinerU-specific options (``mineru_*`` keys).

The dispatch helper ``_dispatch_pdf_parser`` lives in
:mod:`rag.app.naive`, which pulls in a heavy stack (Deepdoc, PaddleOCR,
Docling, etc.). These tests stub the heavy modules so the helper can be
loaded and exercised directly.
"""

import importlib.util
import sys
import types
from pathlib import Path

import pytest

from common.parser_config_utils import (
    MINERU_OPTION_KEYS,
    has_mineru_options,
    normalize_layout_recognizer,
)


# --------------------------------------------------------------------------- #
# has_mineru_options
# --------------------------------------------------------------------------- #


def test_has_mineru_options_recognizes_mineru_keys():
    assert has_mineru_options({"mineru_parse_method": "auto"})
    assert has_mineru_options({"mineru_formula_enable": True})
    assert has_mineru_options({"mineru_table_enable": True})
    assert has_mineru_options({"mineru_lang": "English"})


def test_has_mineru_options_returns_false_without_mineru_keys():
    assert not has_mineru_options({"layout_recognize": "DeepDOC"})
    assert not has_mineru_options({"layout_recognize": "Plain Text"})
    assert not has_mineru_options({})


def test_has_mineru_options_handles_non_dict():
    # Defensive: the dispatch must not crash on unexpected shapes.
    assert not has_mineru_options(None)
    assert not has_mineru_options("mineru_parse_method")
    assert not has_mineru_options(["mineru_parse_method"])


def test_mineru_option_keys_are_exhaustive():
    # Guards against a regression where a new mineru_* option is added in
    # the API but forgotten in the dispatch predicate.
    expected = {"mineru_parse_method", "mineru_formula_enable", "mineru_table_enable", "mineru_lang"}
    assert set(MINERU_OPTION_KEYS) == expected


# --------------------------------------------------------------------------- #
# normalize_layout_recognizer (existing helper, regression pinning)
# --------------------------------------------------------------------------- #


def test_normalize_layout_recognizer_passes_through_known_keywords():
    assert normalize_layout_recognizer("DeepDOC") == ("DeepDOC", None)
    assert normalize_layout_recognizer("MinerU") == ("MinerU", None)
    assert normalize_layout_recognizer("docling") == ("docling", None)


def test_normalize_layout_recognizer_strips_known_provider_suffix():
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@mineru") == ("MinerU", "my-llm@my-instance@my-provider@mineru")
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@paddleocr") == ("PaddleOCR", "my-llm@my-instance@my-provider@paddleocr")
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@opendataloader") == ("OpenDataLoader", "my-llm@my-instance@my-provider@opendataloader")
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@monkeyocr") == ("MonkeyOCR", "my-llm@my-instance@my-provider@monkeyocr")
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@somark") == ("SoMark", "my-llm@my-instance@my-provider@somark")


def test_normalize_layout_recognizer_passes_stale_uuid_through():
    """The dispatcher relies on normalize_layout_recognizer leaving a stale
    TenantModel UUID unchanged so that the by_mineru fallback branch can
    detect it (issue #17114). If this test ever fails because the UUID is
    rewritten to something else, the dispatch recovery in naive.py is
    silently broken."""
    stale_uuid = "06d85f8e819111f1995ef33d60f3a479"
    assert normalize_layout_recognizer(stale_uuid) == (stale_uuid, None)


# --------------------------------------------------------------------------- #
# _dispatch_pdf_parser — actually exercised via stubbed load of rag.app.naive
# --------------------------------------------------------------------------- #
#
# naive.py pulls in a heavy stack (Deepdoc, PaddleOCR, Docling, etc.). The
# fixture below stubs just enough to load the module and exposes
# ``naive_module`` so individual tests can call _dispatch_pdf_parser
# directly and assert on the returned parser, normalized name, and
# parser_model_name.


def _stub(name, **attrs):
    """Stub a module that *replaces* whatever may already be registered
    under the same name (e.g. by test_laws_docx_tables). naive.py's
    fixture needs the same module slots, but with our marker classes —
    a setdefault would leave stale attributes behind."""
    mod = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    sys.modules[name] = mod
    return mod


class _Parser:
    """Marker class so the dispatch can identify by_parser by instance."""

    NAME = ""

    def __init__(self):
        type(self).NAME = self.__class__.__name__


class _ByDeepdoc(_Parser):
    pass


class _ByMineru(_Parser):
    pass


class _ByDocling(_Parser):
    pass


class _ByOpenDataLoader(_Parser):
    pass


class _ByMonkeyocr(_Parser):
    pass


class _ByPlaintext(_Parser):
    pass


@pytest.fixture(scope="module")
def naive_module():
    """Load rag.app.naive with all heavy siblings stubbed.

    Yields the loaded module so tests can call _dispatch_pdf_parser
    directly. Stubs the dispatch's full dependency chain (Deepdoc, the
    LLM stack, PaddleOCR, Docling, ...) so the module can be loaded
    inside a minimal pytest env.
    """
    # Save sys.modules so the fixture restores it on teardown — naive.py's
    # import-time side effects would otherwise leak into the rest of the
    # test suite.
    saved = {name: sys.modules.get(name) for name in list(sys.modules)}

    try:
        # Stub the parser stack used at naive.py import time.
        _stub(
            "deepdoc.parser",
            PdfParser=_Parser,
            DocxParser=_Parser,
            EpubParser=_Parser,
            HtmlParser=_Parser,
            ExcelParser=_Parser,
            JsonParser=_Parser,
            MarkdownElementExtractor=_Parser,
            MarkdownParser=_Parser,
            PdfParser_module=_Parser,
            TxtParser=_Parser,
            RAGFlowPdfParser=_Parser,
            PlainParser=_Parser,
            VisionParser=_Parser,
            DoclingParser=_ByDocling,
            TCADPParser=_Parser,
        )
        _stub("deepdoc.parser.figure_parser", VisionFigureParser=_Parser, vision_figure_parser_docx_wrapper_naive=lambda *a, **k: None, vision_figure_parser_pdf_wrapper=lambda *a, **k: None)
        _stub("deepdoc.parser.pdf_parser", PlainParser=_ByPlaintext, VisionParser=_Parser, RAGFlowPdfParser=_Parser)
        _stub("deepdoc.parser.docling_parser", DoclingParser=_ByDocling)
        _stub("deepdoc.parser.tcadp_parser", TCADPParser=_Parser)
        _stub("deepdoc.parser.utils", extract_pdf_outlines=lambda *a, **k: [])

        _stub("common.parser_config_utils", normalize_layout_recognizer=normalize_layout_recognizer, MINERU_OPTION_KEYS=MINERU_OPTION_KEYS, has_mineru_options=has_mineru_options)
        _stub("common.float_utils", normalize_overlapped_percent=lambda x: x)
        _stub("common.text_utils", normalize_arabic_presentation_forms=lambda x: x)
        _stub("common.token_utils", num_tokens_from_string=lambda s: len((s or "").split()))

        _stub(
            "rag.utils.file_utils",
            extract_embed_file=lambda *a, **k: None,
            extract_links_from_pdf=lambda *a, **k: [],
            extract_links_from_docx=lambda *a, **k: [],
            extract_html=lambda *a, **k: (None, None),
        )

        _stub(
            "rag.nlp",
            num_tokens_from_string=lambda s: len((s or "").split()),
            find_codec=lambda b: "utf-8",
            rag_tokenizer=types.SimpleNamespace(tokenize=lambda s: ((s or "").split(), [])),
            concat_img=lambda *a, **k: None,
            naive_merge=lambda *a, **k: [],
            naive_merge_with_images=lambda *a, **k: [],
            naive_merge_docx=lambda *a, **k: [],
            tokenize_chunks=lambda *a, **k: [],
            tokenize_chunks_with_positions=lambda *a, **k: [],
            doc_tokenize_chunks_with_images=lambda *a, **k: [],
            tokenize_chunks_with_images=lambda *a, **k: [],
            tokenize_table=lambda *a, **k: [],
            append_context2table_image4pdf=lambda *a, **k: [],
        )

        _stub("rag.llm.ocr_model", ensure_mineru_from_env=lambda *a, **k: None, get_first_provider_model_name=lambda *a, **k: None)

        _stub(
            "api.db.joint_services.tenant_model_service",
            resolve_model_config=lambda *a, **k: None,
            ensure_mineru_from_env=lambda *a, **k: None,
            get_tenant_default_model_by_type=lambda *a, **k: None,
            ensure_opendataloader_from_env=lambda *a, **k: None,
            ensure_paddleocr_from_env=lambda *a, **k: None,
            ensure_somark_from_env=lambda *a, **k: None,
            ensure_monkeyocr_from_env=lambda *a, **k: None,
            get_first_provider_model_name=lambda *a, **k: None,
            get_composite_model_name_by_id=lambda *a, **k: (_ for _ in ()).throw(LookupError()),
        )

        _stub("api.db.services.llm_service", LLMBundle=_Parser)

        # Stub the docx stack so the file_format checks at import time succeed.
        _stub("docx", Document=_Parser)
        _stub("docx.opc.pkgreader", _SerializedRelationships=_Parser, _SerializedRelationship=_Parser)
        _stub("docx.table", Table=_Parser)
        _stub("docx.text.paragraph", Paragraph=_Parser)
        _stub("docx.opc.oxml", parse_xml=lambda *a, **k: None)
        _stub("markdown", markdown=lambda *a, **k: "")
        _stub("PIL", Image=_Parser)

        # Load naive.py from source — we can do this even with stubs in place
        # because the module only imports the names listed above.
        repo_root = Path(__file__).resolve().parents[3]
        spec = importlib.util.spec_from_file_location("rag.app.naive_test_module", repo_root / "rag" / "app" / "naive.py")
        module = importlib.util.module_from_spec(spec)
        sys.modules["rag.app.naive_test_module"] = module
        spec.loader.exec_module(module)

        # Replace the dispatch's PARSERS entries with our marker classes
        # so assertions can identify which parser the dispatch selected.
        module.PARSERS["deepdoc"] = _ByDeepdoc
        module.PARSERS["mineru"] = _ByMineru
        module.PARSERS["docling"] = _ByDocling
        module.PARSERS["opendataloader"] = _ByOpenDataLoader
        module.PARSERS["monkeyocr"] = _ByMonkeyocr
        module.PARSERS["plaintext"] = _ByPlaintext

        yield module
    finally:
        # Restore sys.modules exactly — naive.py's import-time side
        # effects would otherwise leak into the rest of the test suite.
        for name in list(sys.modules):
            if name not in saved:
                del sys.modules[name]
        sys.modules.update(saved)


def _dispatch(naive_module, layout_recognize, parser_config=None):
    """Invoke _dispatch_pdf_parser with the given layout_recognize and
    parser_config extras. Returns the (parser, name, layout_recognizer,
    opendataloader_llm_name, parser_model_name) tuple."""
    cfg = dict(parser_config or {})
    cfg["layout_recognize"] = layout_recognize
    return naive_module._dispatch_pdf_parser(cfg)


# CodeRabbit review #3: don't fall back to MinerU for known keywords.
def test_dispatch_does_not_fall_back_to_mineru_for_plain_text_with_mineru_options(naive_module):
    """layout_recognize='Plain Text' is a known keyword — the dispatcher
    must keep routing to by_plaintext even if mineru_* options are set."""
    parser, name, _lr, _op, _model = _dispatch(
        naive_module,
        "Plain Text",
        {"mineru_lang": "English"},
    )
    assert name == "plaintext"
    assert parser is _ByPlaintext


def test_dispatch_does_not_fall_back_to_mineru_for_plaintext_with_mineru_options(naive_module):
    parser, name, _lr, _op, _model = _dispatch(
        naive_module,
        "plaintext",
        {"mineru_lang": "English"},
    )
    assert name == "plaintext"
    assert parser is _ByPlaintext


def test_dispatch_does_not_fall_back_to_mineru_for_deepdoc_with_mineru_options(naive_module):
    parser, name, _lr, _op, _model = _dispatch(
        naive_module,
        "DeepDOC",
        {"mineru_lang": "English"},
    )
    assert name == "deepdoc"
    assert parser is _ByDeepdoc


# Issue #17114: the actual dispatch recovery path.
def test_dispatch_falls_back_to_mineru_only_for_unknown_layout_recognize(naive_module):
    """A stale TenantModel UUID with mineru_* options must route to
    by_mineru so the operator's MinerU intent is honored (issue #17114)."""
    stale_uuid = "06d85f8e819111f1995ef33d60f3a479"
    parser, name, _lr, _op, _model = _dispatch(
        naive_module,
        stale_uuid,
        {"mineru_lang": "English"},
    )
    assert name == "mineru"
    # The fallback branch reassigns parser=by_mineru directly (not via
    # PARSERS["mineru"]), so identify it by the function name.
    assert parser.__name__ == "by_mineru"


# CodeRabbit review #4: layout_recognize_override preserves parser_model_name.
def test_dispatch_uses_resolved_layout_recognize_via_override(naive_module):
    """The chunk() call site resolves a valid TenantModel UUID via
    get_composite_model_name_by_id() into "<model>@<instance>@<provider>"
    before reaching the dispatch. The dispatch must honor that resolved
    value via the layout_recognize_override argument so the configured
    MinerU model name is preserved as parser_model_name (CodeRabbit
    review #4)."""
    stale_uuid = "06d85f8e819111f1995ef33d60f3a479"
    resolved = "my-llm@my-instance@my-provider@mineru"

    # If we passed layout_recognizer="MinerU" (the post-normalize form)
    # instead of layout_recognize_raw, parser_model_name would be None.
    # Passing the full composite name preserves the model so by_mineru can
    # bind to the operator's configured OCR model.
    parser, name, _lr, _op, model = naive_module._dispatch_pdf_parser(
        {"layout_recognize": stale_uuid},
        layout_recognize_override=resolved,
    )
    assert name == "mineru"
    # PARSERS["mineru"] resolves to the by_mineru dispatch, which the
    # fixture exposes as the _ByMineru marker class.
    assert parser.__name__ == "_ByMineru"
    assert model == resolved


# CodeRabbit review #4 follow-up: layout_recognize_override threads the
# opendataloader_llm_name through too.
def test_dispatch_uses_resolved_layout_recognize_via_override_for_opendataloader(naive_module):
    resolved = "my-llm@my-instance@my-provider@opendataloader"

    parser, name, _lr, op_name, _model = naive_module._dispatch_pdf_parser(
        {"layout_recognize": "06d85f8e819111f1995ef33d60f3a479"},
        layout_recognize_override=resolved,
    )
    assert name == "opendataloader"
    assert parser is _ByOpenDataLoader
    assert op_name == resolved


def test_dispatch_uses_resolved_layout_recognize_via_override_for_monkeyocr(naive_module):
    resolved = "my-llm@my-instance@my-provider@monkeyocr"

    parser, name, _lr, _op, model = naive_module._dispatch_pdf_parser(
        {"layout_recognize": "06d85f8e819111f1995ef33d60f3a479"},
        layout_recognize_override=resolved,
    )
    assert name == "monkeyocr"
    assert parser is _ByMonkeyocr
    assert model == resolved


def test_merge_excel_items_keeps_sheets_separate(naive_module):
    items = [
        ("a b", (0, 2, 2, 1, 2)),
        ("c d", (0, 3, 3, 1, 2)),
        ("e f", (1, 2, 2, 1, 3)),
    ]
    merged = naive_module._merge_excel_items(items, chunk_token_num=10)
    assert [pos for _, pos in merged] == [(0, 2, 3, 1, 2), (1, 2, 2, 1, 3)]
    assert merged[0][0] == "a b\nc d"


def test_merge_excel_items_splits_on_token_budget(naive_module):
    items = [
        ("one two", (0, 2, 2, 1, 1)),
        ("three four", (0, 3, 3, 1, 1)),
    ]
    merged = naive_module._merge_excel_items(items, chunk_token_num=2)
    assert [pos for _, pos in merged] == [(0, 2, 2, 1, 1), (0, 3, 3, 1, 1)]


def test_merge_excel_items_passthrough_when_budget_disabled(naive_module):
    items = [("a", (0, 2, 2, 1, 1)), ("b", (0, 3, 3, 1, 1))]
    assert naive_module._merge_excel_items(items, chunk_token_num=0) == items
    assert naive_module._merge_excel_items([], chunk_token_num=128) == []
