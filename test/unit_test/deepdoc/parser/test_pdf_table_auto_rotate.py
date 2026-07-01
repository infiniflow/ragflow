import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest import mock

import pytest

pytestmark = pytest.mark.p2


def _load_pdf_parser_module():
    repo_root = Path(__file__).resolve().parents[4]

    numpy_mod = ModuleType("numpy")

    def _cumsum(values):
        total = 0
        out = []
        for value in values:
            total += value
            out.append(total)
        return out

    numpy_mod.cumsum = _cumsum
    sys.modules["numpy"] = numpy_mod

    for name in [
        "pdfplumber",
        "xgboost",
        "huggingface_hub",
        "PIL",
        "PIL.Image",
        "pypdf",
        "sklearn",
        "sklearn.cluster",
        "sklearn.metrics",
    ]:
        sys.modules[name] = ModuleType(name)

    sys.modules["huggingface_hub"].snapshot_download = lambda *args, **kwargs: ""
    sys.modules["pypdf"].PdfReader = object
    sys.modules["sklearn.cluster"].KMeans = object
    sys.modules["sklearn.metrics"].silhouette_score = lambda *args, **kwargs: 0

    common_pkg = ModuleType("common")
    common_pkg.settings = SimpleNamespace(PARALLEL_DEVICES=1)
    sys.modules["common"] = common_pkg

    common_constants = ModuleType("common.constants")
    common_constants.MAXIMUM_PAGE_NUMBER = 1024
    sys.modules["common.constants"] = common_constants

    common_file_utils = ModuleType("common.file_utils")
    common_file_utils.get_project_base_directory = lambda: str(repo_root)
    sys.modules["common.file_utils"] = common_file_utils

    common_misc_utils = ModuleType("common.misc_utils")
    common_misc_utils.thread_pool_exec = lambda *args, **kwargs: None
    sys.modules["common.misc_utils"] = common_misc_utils

    common_token_utils = ModuleType("common.token_utils")
    sys.modules["common.token_utils"] = common_token_utils

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_pkg.__path__ = [str(repo_root / "deepdoc")]
    sys.modules["deepdoc"] = deepdoc_pkg

    parser_pkg = ModuleType("deepdoc.parser")
    parser_pkg.__path__ = [str(repo_root / "deepdoc" / "parser")]
    sys.modules["deepdoc.parser"] = parser_pkg

    vision_mod = ModuleType("deepdoc.vision")

    class _Recognizer:
        @staticmethod
        def sort_Y_firstly(items, *_args, **_kwargs):
            return items

        @staticmethod
        def layouts_cleanup(_boxes, items, *_args, **_kwargs):
            return items

        @staticmethod
        def find_overlapped_with_threshold(*_args, **_kwargs):
            return None

        @staticmethod
        def find_horizontally_tightest_fit(*_args, **_kwargs):
            return None

    vision_mod.OCR = object
    vision_mod.AscendLayoutRecognizer = object
    vision_mod.LayoutRecognizer = object
    vision_mod.Recognizer = _Recognizer
    vision_mod.TableStructureRecognizer = object
    sys.modules["deepdoc.vision"] = vision_mod

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = [str(repo_root / "rag")]
    sys.modules["rag"] = rag_pkg

    rag_nlp = ModuleType("rag.nlp")
    rag_nlp.rag_tokenizer = SimpleNamespace(tokenize=lambda text: text)
    sys.modules["rag.nlp"] = rag_nlp

    rag_prompts = ModuleType("rag.prompts")
    rag_prompts.__path__ = [str(repo_root / "rag" / "prompts")]
    sys.modules["rag.prompts"] = rag_prompts

    rag_prompts_generator = ModuleType("rag.prompts.generator")
    rag_prompts_generator.vision_llm_describe_prompt = lambda *args, **kwargs: ""
    sys.modules["rag.prompts.generator"] = rag_prompts_generator

    parser_utils = ModuleType("deepdoc.parser.utils")
    parser_utils.extract_pdf_outlines = lambda *_args, **_kwargs: []
    sys.modules["deepdoc.parser.utils"] = parser_utils

    module_name = "test_pdf_table_auto_rotate_unit_module"
    module_path = repo_root / "deepdoc" / "parser" / "pdf_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


class _FakeImage:
    size = (100, 60)

    def crop(self, _coords):
        return self


def test_table_transformer_job_respects_env_when_auto_rotate_omitted(monkeypatch):
    with mock.patch.dict(sys.modules, {}, clear=False):
        module = _load_pdf_parser_module()

    parser = object.__new__(module.RAGFlowPdfParser)
    parser.page_layout = [[{"type": "table", "x0": 0, "top": 0, "x1": 10, "bottom": 10}]]
    parser.page_images = [_FakeImage()]
    parser.boxes = []
    parser.tbl_det = lambda imgs: [[] for _ in imgs]
    parser._ocr_rotated_tables = lambda *_args, **_kwargs: None

    def _unexpected_orientation_eval(_table_img):
        raise AssertionError("orientation evaluation should be skipped when TABLE_AUTO_ROTATE=false")

    parser._evaluate_table_orientation = _unexpected_orientation_eval
    monkeypatch.setenv("TABLE_AUTO_ROTATE", "false")

    parser._table_transformer_job(1)
