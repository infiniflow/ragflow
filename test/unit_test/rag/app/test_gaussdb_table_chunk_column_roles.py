import sys
import types
from importlib import import_module, reload
from unittest.mock import MagicMock, patch

import pytest

import common.settings as settings


TEST_CSV = b"""row_id,title,content,country,category
1,Earthquake hits Turkey,A 5.8 magnitude earthquake struck Konya,Turkey,Disaster
2,Oil prices surge,Brent crude jumped 4.2 percent,Global,Economy
3,AI regulation proposed,EU unveiled a draft regulation,EU,Technology
"""
FILENAME = "test.csv"
KB_ID = "test_kb_id"
TABLE_NAME = "ragflow_0123456789abcdef0123456789abcdef"


def _install_table_import_stubs():
    names = (
        "deepdoc.parser",
        "deepdoc.parser.utils",
        "deepdoc.parser.figure_parser",
        "deepdoc.vision.ocr",
        "rag.nlp",
        "rag.app.table",
        "rag.app.picture",
        "api.db.services.knowledgebase_service",
        "xpinyin",
    )
    original = {name: sys.modules.get(name) for name in names}

    parser_stub = types.ModuleType("deepdoc.parser")
    parser_stub.__path__ = []
    parser_stub.ExcelParser = type("ExcelParser", (), {})

    utils_stub = types.ModuleType("deepdoc.parser.utils")
    utils_stub.get_text = lambda _filename, binary=None: binary.decode("utf-8") if isinstance(binary, bytes) else str(binary or "")

    figure_stub = types.ModuleType("deepdoc.parser.figure_parser")
    figure_stub.vision_figure_parser_figure_xlsx_wrapper = lambda *_args, **_kwargs: []

    nlp_stub = types.ModuleType("rag.nlp")

    def tokenize_doc(doc, text="", *_args, **_kwargs):
        if isinstance(doc, dict):
            doc["content_with_weight"] = str(text or "")
            doc["content_ltks"] = str(text or "")
            doc["content_sm_ltks"] = str(text or "")
        return doc

    nlp_stub.rag_tokenizer = types.SimpleNamespace(
        tokenize=lambda text, *_args, **_kwargs: str(text),
        fine_grained_tokenize=lambda text, *_args, **_kwargs: str(text),
    )
    nlp_stub.tokenize = tokenize_doc
    nlp_stub.tokenize_table = lambda *_args, **_kwargs: []

    kb_service_stub = types.ModuleType("api.db.services.knowledgebase_service")
    kb_service_stub.KnowledgebaseService = type(
        "KnowledgebaseService",
        (),
        {"update_parser_config": staticmethod(lambda *_args, **_kwargs: None)},
    )

    xpinyin_stub = types.ModuleType("xpinyin")

    class Pinyin:
        def get_pinyins(self, text, splitter="_"):
            return [str(text).replace(" ", splitter)]

    xpinyin_stub.Pinyin = Pinyin

    sys.modules["deepdoc.parser"] = parser_stub
    sys.modules["deepdoc.parser.utils"] = utils_stub
    sys.modules["deepdoc.parser.figure_parser"] = figure_stub
    sys.modules["deepdoc.vision.ocr"] = MagicMock()
    sys.modules["rag.nlp"] = nlp_stub
    sys.modules["rag.app.picture"] = MagicMock()
    sys.modules["api.db.services.knowledgebase_service"] = kb_service_stub
    sys.modules["xpinyin"] = xpinyin_stub
    return original


def _noop_callback(*_args, **_kwargs):
    pass


@pytest.fixture(scope="module")
def table_module():
    original = _install_table_import_stubs()
    try:
        module = import_module("rag.app.table")
        yield reload(module)
    finally:
        for name, previous in original.items():
            if previous is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = previous


@pytest.fixture
def mock_update_kb():
    with patch("rag.app.table.KnowledgebaseService.update_parser_config") as mock:
        yield mock


@pytest.fixture
def dialog_service(monkeypatch):
    from test.unit_test.api.db.services.test_gaussdb_dialog_sql import _install_settings_import_stubs

    _install_settings_import_stubs(monkeypatch)
    sys.modules.pop("api.db.services.dialog_service", None)
    module = import_module("api.db.services.dialog_service")
    try:
        yield module
    finally:
        sys.modules.pop("api.db.services.dialog_service", None)
        package = sys.modules.get("api.db.services")
        if package is not None and hasattr(package, "dialog_service"):
            delattr(package, "dialog_service")


def _run_chunk(table_module, parser_config, mock_update_kb):
    return table_module.chunk(
        FILENAME,
        binary=TEST_CSV,
        callback=_noop_callback,
        kb_id=KB_ID,
        parser_config=parser_config,
        lang="Chinese",
    )


def _assert_field_map_prompt_closure(dialog_service, chunks, field_map, expected_field_map):
    assert field_map == expected_field_map
    prompt = dialog_service.gaussdb_text_to_sql.build_sql_prompt(TABLE_NAME, field_map, "show table rows")
    for field, path in expected_field_map.items():
        assert path in chunks[0]["chunk_data"]
        assert f"  - {field} ({path}): chunk_data #>> '{{{path}}}'" in prompt


def test_tc_sql_006_chunk_gaussdb_updates_field_map_and_column_names(
    table_module,
    mock_update_kb,
    monkeypatch,
    dialog_service,
):
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True, raising=False)

    chunks = _run_chunk(table_module, {}, mock_update_kb)

    mock_update_kb.assert_called_once_with(
        KB_ID,
        {
            "field_map": {
                "row_id": "row_id",
                "title": "title",
                "content": "content",
                "country": "country",
                "category": "category",
            },
            "table_column_names": ["row_id", "title", "content", "country", "category"],
        },
    )
    _assert_field_map_prompt_closure(
        dialog_service,
        chunks,
        mock_update_kb.call_args.args[1]["field_map"],
        {
            "row_id": "row_id",
            "title": "title",
            "content": "content",
            "country": "country",
            "category": "category",
        },
    )


def test_tc_sql_007_chunk_manual_mode_gaussdb_stores_metadata_in_chunk_data(
    table_module,
    mock_update_kb,
    monkeypatch,
    dialog_service,
):
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True, raising=False)
    parser_config = {
        "table_column_mode": "manual",
        "table_column_roles": {
            "title": "indexing",
            "content": "indexing",
            "row_id": "metadata",
            "country": "metadata",
            "category": "metadata",
        },
    }

    chunks = _run_chunk(table_module, parser_config, mock_update_kb)

    assert chunks[0]["chunk_data"] == {
        "row_id": 1,
        "country": "Turkey",
        "category": "Disaster",
    }
    assert "country_raw" not in chunks[0]
    assert "- country:" not in chunks[0]["content_with_weight"]
    assert "- title: Earthquake hits Turkey" in chunks[0]["content_with_weight"]
    field_map = mock_update_kb.call_args.args[1]["field_map"]
    assert field_map == {
        "row_id": "row_id",
        "country": "country",
        "category": "category",
    }
    _assert_field_map_prompt_closure(
        dialog_service,
        chunks,
        field_map,
        {
            "row_id": "row_id",
            "country": "country",
            "category": "category",
        },
    )


def test_tc_sql_008_chunk_auto_mode_preserves_row_id_text_and_prompt_path(
    table_module,
    mock_update_kb,
    monkeypatch,
    dialog_service,
):
    monkeypatch.setattr(settings, "DOC_ENGINE_GAUSSDB", True, raising=False)

    chunks = _run_chunk(table_module, {}, mock_update_kb)
    field_map = mock_update_kb.call_args.args[1]["field_map"]

    assert "- row_id: 1" in chunks[0]["content_with_weight"].splitlines()
    assert chunks[0]["chunk_data"]["row_id"] == 1
    _assert_field_map_prompt_closure(
        dialog_service,
        chunks,
        field_map,
        {
            "row_id": "row_id",
            "title": "title",
            "content": "content",
            "country": "country",
            "category": "category",
        },
    )
