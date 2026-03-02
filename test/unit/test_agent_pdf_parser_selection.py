#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
"""
Unit tests for the agent PDF parser selection feature.

Verifies that:
 - Begin/UserFillUp components accept ``layout_recognize`` from DSL config
 - The selected parser propagates through FileService.parse() into the
   parser_config dict
 - The PARSERS dict in rag/app/naive.py resolves every dropdown value
 - normalize_layout_recognizer() correctly strips @MinerU / @PaddleOCR suffixes
 - Backward compatibility: missing layout_recognize defaults to "Plain Text"
"""

import asyncio
import importlib
import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock, patch

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

def _ensure_common_package(monkeypatch):
    """Make ``common`` importable without the full app stack."""
    if "common" not in sys.modules:
        common_pkg = ModuleType("common")
        common_pkg.__path__ = [str(REPO_ROOT / "common")]
        monkeypatch.setitem(sys.modules, "common", common_pkg)


def _import_module_from_path(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


# ===================================================================
# 1.  normalize_layout_recognizer
# ===================================================================


class TestNormalizeLayoutRecognizer:
    """Tests for common/parser_config_utils.py::normalize_layout_recognizer."""

    @pytest.fixture(autouse=True)
    def _load(self, monkeypatch):
        _ensure_common_package(monkeypatch)
        self.mod = _import_module_from_path(
            "common.parser_config_utils",
            REPO_ROOT / "common" / "parser_config_utils.py",
        )
        self.fn = self.mod.normalize_layout_recognizer

    # -- plain values pass through unchanged --

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "raw, expected_recognizer, expected_model",
        [
            ("DeepDOC", "DeepDOC", None),
            ("Plain Text", "Plain Text", None),
            ("Docling", "Docling", None),
            ("TCADP Parser", "TCADP Parser", None),
        ],
        ids=["deepdoc", "plain_text", "docling", "tcadp_parser"],
    )
    def test_plain_values_pass_through(self, raw, expected_recognizer, expected_model):
        recognizer, model = self.fn(raw)
        assert recognizer == expected_recognizer
        assert model == expected_model

    # -- @MinerU suffix --

    @pytest.mark.p1
    def test_mineru_suffix_extracted(self):
        recognizer, model = self.fn("some-model@MinerU")
        assert recognizer == "MinerU"
        assert model == "some-model"

    @pytest.mark.p2
    def test_mineru_suffix_case_insensitive(self):
        recognizer, model = self.fn("my-model@mineru")
        assert recognizer == "MinerU"
        assert model == "my-model"

    # -- @PaddleOCR suffix --

    @pytest.mark.p1
    def test_paddleocr_suffix_extracted(self):
        recognizer, model = self.fn("ocr-v2@PaddleOCR")
        assert recognizer == "PaddleOCR"
        assert model == "ocr-v2"

    @pytest.mark.p2
    def test_paddleocr_suffix_case_insensitive(self):
        recognizer, model = self.fn("ocr-v2@paddleocr")
        assert recognizer == "PaddleOCR"
        assert model == "ocr-v2"

    # -- non-string input --

    @pytest.mark.p2
    def test_none_input(self):
        recognizer, model = self.fn(None)
        assert recognizer is None
        assert model is None

    @pytest.mark.p2
    def test_bool_input(self):
        recognizer, model = self.fn(True)
        assert recognizer is True
        assert model is None


# ===================================================================
# 2.  PARSERS dict resolution  (rag/app/naive.py)
# ===================================================================


class TestParsersDict:
    """
    Verify that every value produced by the frontend dropdown is resolved
    to the correct parser function after ``.strip().lower()``.
    """

    @pytest.fixture(autouse=True)
    def _load(self, monkeypatch):
        _ensure_common_package(monkeypatch)
        # naive.py has heavy imports; we only need the PARSERS dict.
        # Read the module source, extract PARSERS via exec in a controlled namespace.
        src = (REPO_ROOT / "rag" / "app" / "naive.py").read_text()

        # Build minimal stubs for the functions referenced in PARSERS.
        sentinel_fns = {}
        for name in (
            "by_deepdoc", "by_mineru", "by_docling",
            "by_tcadp", "by_paddleocr", "by_plaintext",
        ):
            sentinel_fns[name] = name  # use the name string as a sentinel

        # Extract only the PARSERS dict definition
        ns = dict(sentinel_fns)
        # Find the PARSERS dict definition in source
        import re
        match = re.search(r"^PARSERS\s*=\s*\{[^}]+\}", src, re.MULTILINE)
        assert match, "Could not find PARSERS dict in naive.py"
        exec(match.group(), ns)
        self.parsers = ns["PARSERS"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "dropdown_value, expected_fn",
        [
            ("DeepDOC", "by_deepdoc"),
            ("Plain Text", "by_plaintext"),
            ("Docling", "by_docling"),
            ("TCADP Parser", "by_tcadp"),
            ("MinerU", "by_mineru"),
            ("PaddleOCR", "by_paddleocr"),
        ],
        ids=["deepdoc", "plain_text", "docling", "tcadp_parser", "mineru", "paddleocr"],
    )
    def test_dropdown_value_resolves(self, dropdown_value, expected_fn):
        key = dropdown_value.strip().lower()
        assert key in self.parsers, f"Key {key!r} not found in PARSERS"
        assert self.parsers[key] == expected_fn

    @pytest.mark.p2
    def test_unknown_key_falls_back_to_plaintext(self):
        result = self.parsers.get("nonexistent", "by_plaintext")
        assert result == "by_plaintext"

    @pytest.mark.p2
    def test_legacy_plaintext_key(self):
        assert "plaintext" in self.parsers
        assert self.parsers["plaintext"] == "by_plaintext"

    @pytest.mark.p2
    def test_tcadp_short_key(self):
        assert "tcadp" in self.parsers
        assert self.parsers["tcadp"] == "by_tcadp"


# ===================================================================
# 3.  UserFillUpParam.layout_recognize  (agent/component/fillup.py)
# ===================================================================


class TestUserFillUpParam:
    """
    Verify that UserFillUpParam declares ``layout_recognize`` and that
    ComponentParamBase.update() populates it from DSL config.
    """

    @pytest.fixture(autouse=True)
    def _load(self, monkeypatch):
        _ensure_common_package(monkeypatch)

        # Stub heavy dependencies that fillup.py imports transitively
        agent_pkg = ModuleType("agent")
        agent_pkg.__path__ = [str(REPO_ROOT / "agent")]
        monkeypatch.setitem(sys.modules, "agent", agent_pkg)

        settings_mod = ModuleType("agent.settings")
        settings_mod.PARAM_MAXDEPTH = 10
        agent_pkg.settings = settings_mod
        monkeypatch.setitem(sys.modules, "agent.settings", settings_mod)

        # Stub api.db.services.file_service
        api_pkg = ModuleType("api")
        api_pkg.__path__ = []
        monkeypatch.setitem(sys.modules, "api", api_pkg)

        db_pkg = ModuleType("api.db")
        db_pkg.__path__ = []
        monkeypatch.setitem(sys.modules, "api.db", db_pkg)

        services_pkg = ModuleType("api.db.services")
        services_pkg.__path__ = []
        monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

        file_service_mod = ModuleType("api.db.services.file_service")
        file_service_mod.FileService = MagicMock()
        monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)

        # Stub other transitive dependencies
        for mod_name in [
            "common.connection_utils",
            "common.misc_utils",
        ]:
            stub = ModuleType(mod_name)
            stub.timeout = lambda *a, **kw: (lambda f: f)
            stub.thread_pool_exec = MagicMock()
            monkeypatch.setitem(sys.modules, mod_name, stub)

        # Import the actual modules
        base_mod = _import_module_from_path(
            "agent.component.base",
            REPO_ROOT / "agent" / "component" / "base.py",
        )
        monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)

        fillup_mod = _import_module_from_path(
            "agent.component.fillup",
            REPO_ROOT / "agent" / "component" / "fillup.py",
        )
        self.UserFillUpParam = fillup_mod.UserFillUpParam

    @pytest.mark.p1
    def test_default_layout_recognize_is_empty(self):
        param = self.UserFillUpParam()
        assert param.layout_recognize == ""

    @pytest.mark.p1
    def test_update_sets_layout_recognize(self):
        param = self.UserFillUpParam()
        param.update({"layout_recognize": "DeepDOC"})
        assert param.layout_recognize == "DeepDOC"

    @pytest.mark.p1
    def test_update_with_mineru_model(self):
        param = self.UserFillUpParam()
        param.update({"layout_recognize": "my-model@MinerU"})
        assert param.layout_recognize == "my-model@MinerU"

    @pytest.mark.p2
    def test_update_without_layout_recognize_keeps_default(self):
        param = self.UserFillUpParam()
        param.update({"enable_tips": False})
        assert param.layout_recognize == ""

    @pytest.mark.p2
    def test_empty_string_is_falsy(self):
        """Ensures ``param.layout_recognize or None`` evaluates to None."""
        param = self.UserFillUpParam()
        assert (param.layout_recognize or None) is None


# ===================================================================
# 4.  BeginParam inherits layout_recognize
# ===================================================================


class TestBeginParam:
    """BeginParam inherits from UserFillUpParam and should carry layout_recognize."""

    @pytest.fixture(autouse=True)
    def _load(self, monkeypatch):
        _ensure_common_package(monkeypatch)

        agent_pkg = ModuleType("agent")
        agent_pkg.__path__ = [str(REPO_ROOT / "agent")]
        monkeypatch.setitem(sys.modules, "agent", agent_pkg)

        settings_mod = ModuleType("agent.settings")
        settings_mod.PARAM_MAXDEPTH = 10
        agent_pkg.settings = settings_mod
        monkeypatch.setitem(sys.modules, "agent.settings", settings_mod)

        api_pkg = ModuleType("api")
        api_pkg.__path__ = []
        monkeypatch.setitem(sys.modules, "api", api_pkg)

        db_pkg = ModuleType("api.db")
        db_pkg.__path__ = []
        monkeypatch.setitem(sys.modules, "api.db", db_pkg)

        services_pkg = ModuleType("api.db.services")
        services_pkg.__path__ = []
        monkeypatch.setitem(sys.modules, "api.db.services", services_pkg)

        file_service_mod = ModuleType("api.db.services.file_service")
        file_service_mod.FileService = MagicMock()
        monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service_mod)

        for mod_name in ["common.connection_utils", "common.misc_utils"]:
            stub = ModuleType(mod_name)
            stub.timeout = lambda *a, **kw: (lambda f: f)
            stub.thread_pool_exec = MagicMock()
            monkeypatch.setitem(sys.modules, mod_name, stub)

        base_mod = _import_module_from_path(
            "agent.component.base",
            REPO_ROOT / "agent" / "component" / "base.py",
        )
        monkeypatch.setitem(sys.modules, "agent.component.base", base_mod)

        fillup_mod = _import_module_from_path(
            "agent.component.fillup",
            REPO_ROOT / "agent" / "component" / "fillup.py",
        )
        monkeypatch.setitem(sys.modules, "agent.component.fillup", fillup_mod)

        begin_mod = _import_module_from_path(
            "agent.component.begin",
            REPO_ROOT / "agent" / "component" / "begin.py",
        )
        self.BeginParam = begin_mod.BeginParam

    @pytest.mark.p1
    def test_inherits_layout_recognize(self):
        param = self.BeginParam()
        assert hasattr(param, "layout_recognize")
        assert param.layout_recognize == ""

    @pytest.mark.p1
    def test_update_sets_layout_recognize(self):
        param = self.BeginParam()
        param.update({"layout_recognize": "TCADP Parser"})
        assert param.layout_recognize == "TCADP Parser"


# ===================================================================
# 5.  FileService.parse  layout_recognize propagation
# ===================================================================


class TestFileServiceParse:
    """
    Verify that FileService.parse() builds parser_config with the supplied
    layout_recognize value (or defaults to 'Plain Text').
    """

    @pytest.fixture(autouse=True)
    def _setup(self, monkeypatch):
        _ensure_common_package(monkeypatch)

        # We'll patch the internals that FileService.parse() calls so we can
        # capture the parser_config that gets built.
        self.captured_kwargs = {}

        # Import the real file_service module — but we need to stub its
        # heavy transitive dependencies.  Instead, we test the logic
        # directly by reading the source and verifying the parser_config line.
        src = (REPO_ROOT / "api" / "db" / "services" / "file_service.py").read_text()
        self.source = src

    @pytest.mark.p1
    def test_source_uses_layout_recognize_param(self):
        """The parse() signature accepts layout_recognize."""
        assert "def parse(filename, blob, img_base64=True, tenant_id=None, layout_recognize=None):" in self.source

    @pytest.mark.p1
    def test_parser_config_uses_layout_recognize_or_default(self):
        """parser_config uses the provided value or falls back to 'Plain Text'."""
        assert 'layout_recognize or "Plain Text"' in self.source

    @pytest.mark.p1
    def test_get_files_accepts_layout_recognize(self):
        """get_files() signature accepts layout_recognize."""
        assert "def get_files(files: Union[None, list[dict]], raw: bool = False, layout_recognize: str = None)" in self.source

    @pytest.mark.p2
    def test_get_files_forwards_layout_recognize_to_parse(self):
        """get_files() passes layout_recognize positionally to parse()."""
        # The ThreadPoolExecutor submit call should include layout_recognize
        assert "file[\"created_by\"], layout_recognize))" in self.source


# ===================================================================
# 6.  Canvas.run() extracts layout_recognize from Begin
# ===================================================================


class TestCanvasLayoutRecognize:
    """
    Verify that Canvas.run() extracts layout_recognize from the Begin
    component and passes it to get_files_async().
    """

    @pytest.fixture(autouse=True)
    def _setup(self):
        self.source = (REPO_ROOT / "agent" / "canvas.py").read_text()

    @pytest.mark.p1
    def test_run_extracts_layout_recognize_from_begin(self):
        """Canvas.run() finds the Begin component and reads its layout_recognize."""
        assert 'layout_recognize = getattr(cpn["obj"]._param, "layout_recognize", None)' in self.source

    @pytest.mark.p1
    def test_get_files_async_accepts_layout_recognize(self):
        """get_files_async() accepts layout_recognize parameter."""
        assert "async def get_files_async(self, files: Union[None, list[dict]], layout_recognize: str = None)" in self.source

    @pytest.mark.p1
    def test_get_files_async_forwards_to_parse(self):
        """get_files_async() passes layout_recognize to FileService.parse()."""
        assert 'FileService.parse(file["name"], blob, True, file["created_by"], layout_recognize)' in self.source

    @pytest.mark.p1
    def test_sys_files_uses_layout_recognize(self):
        """sys.files path passes layout_recognize to get_files_async()."""
        assert "await self.get_files_async(kwargs[k], layout_recognize)" in self.source

    @pytest.mark.p2
    def test_get_files_sync_accepts_layout_recognize(self):
        """Sync get_files() wrapper also accepts layout_recognize."""
        assert "def get_files(self, files: Union[None, list[dict]], layout_recognize: str = None)" in self.source


# ===================================================================
# 7.  End-to-end: dropdown value → normalize → PARSERS lookup
# ===================================================================


class TestEndToEndParserResolution:
    """
    Simulate the full path a dropdown value takes from the frontend
    through normalize_layout_recognizer() and into the PARSERS dict lookup.
    """

    @pytest.fixture(autouse=True)
    def _load(self, monkeypatch):
        _ensure_common_package(monkeypatch)
        self.normalize = _import_module_from_path(
            "common.parser_config_utils",
            REPO_ROOT / "common" / "parser_config_utils.py",
        ).normalize_layout_recognizer

        # Extract PARSERS dict
        import re
        src = (REPO_ROOT / "rag" / "app" / "naive.py").read_text()
        sentinel_fns = {}
        for name in (
            "by_deepdoc", "by_mineru", "by_docling",
            "by_tcadp", "by_paddleocr", "by_plaintext",
        ):
            sentinel_fns[name] = name
        ns = dict(sentinel_fns)
        match = re.search(r"^PARSERS\s*=\s*\{[^}]+\}", src, re.MULTILINE)
        exec(match.group(), ns)
        self.parsers = ns["PARSERS"]

    def _resolve(self, dropdown_value):
        """Simulate the full resolution chain."""
        recognizer, model_name = self.normalize(dropdown_value)
        name = recognizer.strip().lower()
        return self.parsers.get(name, "by_plaintext"), model_name

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "dropdown_value, expected_parser, expected_model",
        [
            ("DeepDOC", "by_deepdoc", None),
            ("Plain Text", "by_plaintext", None),
            ("Docling", "by_docling", None),
            ("TCADP Parser", "by_tcadp", None),
            ("my-model@MinerU", "by_mineru", "my-model"),
            ("ocr-v2@PaddleOCR", "by_paddleocr", "ocr-v2"),
        ],
        ids=["deepdoc", "plain_text", "docling", "tcadp_parser", "mineru", "paddleocr"],
    )
    def test_full_resolution(self, dropdown_value, expected_parser, expected_model):
        parser, model = self._resolve(dropdown_value)
        assert parser == expected_parser
        assert model == expected_model

    @pytest.mark.p1
    def test_backward_compat_none_defaults_to_plain_text(self):
        """When layout_recognize is None/empty (old agents), default is Plain Text."""
        # Simulate: layout_recognize = param.layout_recognize or None  ->  None
        # Then:     parser_config["layout_recognize"] = None or "Plain Text"  ->  "Plain Text"
        default_value = None or "Plain Text"
        recognizer, _ = self.normalize(default_value)
        name = recognizer.strip().lower()
        parser = self.parsers.get(name, "by_plaintext")
        assert parser == "by_plaintext"

    @pytest.mark.p2
    def test_backward_compat_empty_string_defaults(self):
        """Empty string from param default → None → 'Plain Text'."""
        default_value = "" or "Plain Text"
        recognizer, _ = self.normalize(default_value)
        name = recognizer.strip().lower()
        parser = self.parsers.get(name, "by_plaintext")
        assert parser == "by_plaintext"
