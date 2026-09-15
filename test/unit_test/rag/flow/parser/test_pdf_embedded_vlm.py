#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import Mock

import pytest

REPO_ROOT = Path(__file__).resolve().parents[5]


def _load_pdf_chunk_metadata(monkeypatch):
    for package_name in ("api", "api.db", "api.db.services", "common", "rag", "rag.utils"):
        pkg = ModuleType(package_name)
        pkg.__path__ = []
        monkeypatch.setitem(sys.modules, package_name, pkg)
    common_settings = ModuleType("common.settings")
    common_settings.settings = SimpleNamespace(STORAGE_IMPL=Mock())
    monkeypatch.setitem(sys.modules, "common.settings", common_settings)
    monkeypatch.setitem(sys.modules, "common.misc_utils", ModuleType("common.misc_utils"))
    sys.modules["common.misc_utils"].get_uuid = lambda: "id"
    monkeypatch.setitem(sys.modules, "rag.utils.base64_image", ModuleType("rag.utils.base64_image"))
    sys.modules["rag.utils.base64_image"].image2id = Mock()

    module_path = REPO_ROOT / "rag/flow/parser/pdf_chunk_metadata.py"
    spec = importlib.util.spec_from_file_location("test_pdf_chunk_metadata_isolated", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p1
def test_supplement_embedded_images_for_image_only_pdf(monkeypatch):
    module = _load_pdf_chunk_metadata(monkeypatch)
    blob = REPO_ROOT / "test/unit_test/rag/flow/parser/fixtures/image_only.pdf"
    data = blob.read_bytes()
    out = module.supplement_deepdoc_bboxes_with_embedded_images(data, [])
    assert out, "expected embedded image boxes for image-only PDF"
    assert out[0].get("image") is not None


@pytest.mark.p1
def test_enhance_media_runs_for_title_block_with_image(monkeypatch):
    utils_path = REPO_ROOT / "rag/flow/parser/utils.py"
    for package_name in (
        "api",
        "api.db",
        "api.db.services",
        "api.db.joint_services",
        "common",
        "deepdoc",
        "deepdoc.parser",
        "rag",
    ):
        pkg = ModuleType(package_name)
        pkg.__path__ = []
        monkeypatch.setitem(sys.modules, package_name, pkg)

    llm_service = ModuleType("llm_service")
    llm_service.LLMBundle = Mock()
    llm_service.resolve_llm_setting = Mock(return_value={})
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service)

    tenant_model_service = ModuleType("tenant_model_service")
    tenant_model_service.get_tenant_default_model_by_type = Mock()
    tenant_model_service.resolve_model_config = Mock(return_value={"llm_name": "vision"})
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_service)

    monkeypatch.setitem(sys.modules, "common.constants", SimpleNamespace(LLMType=SimpleNamespace(VISION="vision")))

    figure_parser = ModuleType("figure_parser")
    parser_instance = Mock(return_value=[((None, "giraffe description"), None)])
    figure_parser.VisionFigureParser = Mock(return_value=parser_instance)
    monkeypatch.setitem(sys.modules, "deepdoc.parser.figure_parser", figure_parser)

    nlp = ModuleType("rag.nlp")
    nlp.is_english = Mock(return_value=False)
    nlp.random_choices = Mock(return_value=[])
    nlp.remove_contents_table = Mock()
    monkeypatch.setitem(sys.modules, "rag.nlp", nlp)

    spec = importlib.util.spec_from_file_location("test_flow_utils_isolated", utils_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)

    sections = [
        {
            "text": "长颈鹿说明：",
            "image": object(),
            "layout_type": "title",
            "doc_type_kwd": "text",
        }
    ]
    result = module.enhance_media_sections_with_vision(sections, "tenant-id", {"llm_id": "vision-model"})
    assert "giraffe description" in result[0]["text"]
