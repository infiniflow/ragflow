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
def test_supplement_merges_text_bboxes_with_embedded_images(monkeypatch):
    module = _load_pdf_chunk_metadata(monkeypatch)
    text_bbox = {"layout_type": "text", "text": "body", "page_number": 1}
    fake_page = SimpleNamespace(
        images=[{"x0": 0, "top": 0, "x1": 12, "bottom": 12}],
        chars=[object()],
        crop=lambda rect: SimpleNamespace(
            to_image=lambda resolution, antialias: SimpleNamespace(original=object())
        ),
    )
    fake_pdf = SimpleNamespace(pages=[fake_page])

    class FakePlumber:
        @staticmethod
        def open(_blob):
            return FakeContext(fake_pdf)

    class FakeContext:
        def __init__(self, pdf):
            self.pdf = pdf

        def __enter__(self):
            return self.pdf

        def __exit__(self, *args):
            return False

    monkeypatch.setattr(module.pdfplumber, "open", FakePlumber.open)
    out = module.supplement_deepdoc_bboxes_with_embedded_images(b"pdf", [text_bbox])
    assert len(out) == 2
    assert out[0]["text"] == "body"
    assert out[1].get("image") is not None


@pytest.mark.p1
def test_supplement_appends_only_missing_embedded_images(monkeypatch):
    module = _load_pdf_chunk_metadata(monkeypatch)
    existing_image = object()
    existing_bbox = {
        "layout_type": "figure",
        "page_number": 1,
        "x0": 0.0,
        "x1": 12.0,
        "top": 0.0,
        "bottom": 12.0,
        "image": existing_image,
        "positions": [[1, 0, 12, 0, 12]],
    }
    fake_page = SimpleNamespace(
        images=[
            {"x0": 0, "top": 0, "x1": 12, "bottom": 12},
            {"x0": 20, "top": 0, "x1": 32, "bottom": 12},
        ],
        chars=[object()],
        crop=lambda rect: SimpleNamespace(
            to_image=lambda resolution, antialias: SimpleNamespace(original=object())
        ),
    )
    fake_pdf = SimpleNamespace(pages=[fake_page])

    class FakePlumber:
        @staticmethod
        def open(_blob):
            return FakeContext(fake_pdf)

    class FakeContext:
        def __init__(self, pdf):
            self.pdf = pdf

        def __enter__(self):
            return self.pdf

        def __exit__(self, *args):
            return False

    monkeypatch.setattr(module.pdfplumber, "open", FakePlumber.open)
    out = module.supplement_deepdoc_bboxes_with_embedded_images(b"pdf", [existing_bbox])
    assert len(out) == 2
    assert out[0]["image"] is existing_image
    assert out[1].get("image") is not None
    assert out[1]["x0"] == 20.0
    assert out[1]["x1"] == 32.0


@pytest.mark.p1
def test_apply_document_vertical_coords_to_bboxes_offsets_supplemented_only(monkeypatch):
    module = _load_pdf_chunk_metadata(monkeypatch)
    deepdoc_box = {"page_number": 2, "top": 50.0, "bottom": 60.0, "text": "body"}
    supplemented = {
        "page_number": 2,
        "top": 10.0,
        "bottom": 20.0,
        "_embedded_supplement": True,
    }
    page_cum_height = [0, 100, 250]
    out = module.apply_document_vertical_coords_to_bboxes([deepdoc_box, supplemented], page_cum_height)
    assert out[0]["top"] == 50.0
    assert out[1]["top"] == 110.0


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
