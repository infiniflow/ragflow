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

import importlib
import logging
import sys
import types


def _module(name, **attributes):
    module = types.ModuleType(name)
    for attribute, value in attributes.items():
        setattr(module, attribute, value)
    return module


def test_parser_imports_with_partial_pdf_parser_stub(pdf_parser_stub):
    assert not hasattr(pdf_parser_stub, "PlainParser")

    parser_module = importlib.import_module("rag.flow.parser.parser")

    assert parser_module.Parser.component_name == "Parser"


def test_source_blob_loader_resolves_services_at_runtime(pdf_parser_stub, monkeypatch, caplog):
    parser_module = importlib.import_module("rag.flow.parser.parser")
    calls = []
    caplog.set_level(logging.DEBUG, logger=parser_module.__name__)

    class RuntimeFileService:
        @staticmethod
        def get_blob(created_by, file_id):
            calls.append((created_by, file_id))
            return b"file-blob"

    monkeypatch.setitem(
        sys.modules,
        "api.db.services.file_service",
        _module("api.db.services.file_service", FileService=RuntimeFileService),
    )

    upstream = types.SimpleNamespace(file={"created_by": "tenant-1", "id": "file-1"})
    canvas = types.SimpleNamespace(_doc_id=None)

    assert parser_module._fetch_source_blob(upstream, canvas) == b"file-blob"
    assert calls == [("tenant-1", "file-1")]
    assert "Loading parser source from upstream file storage." in caplog.messages

    class RuntimeFile2DocumentService:
        @staticmethod
        def get_storage_address(doc_id):
            calls.append(("doc", doc_id))
            return "bucket-1", "object-1"

    class RuntimeStorage:
        @staticmethod
        def get(bucket, name):
            calls.append((bucket, name))
            return b"document-blob"

    monkeypatch.setitem(
        sys.modules,
        "api.db.services.file2document_service",
        _module("api.db.services.file2document_service", File2DocumentService=RuntimeFile2DocumentService),
    )
    monkeypatch.setattr(parser_module.settings, "STORAGE_IMPL", RuntimeStorage())
    canvas._doc_id = "doc-1"

    assert parser_module._fetch_source_blob(upstream, canvas) == b"document-blob"
    assert calls[-2:] == [("doc", "doc-1"), ("bucket-1", "object-1")]
    assert "Loading parser source from canvas document storage." in caplog.messages
    for sensitive_value in ("tenant-1", "file-1", "doc-1", "bucket-1", "object-1"):
        assert sensitive_value not in caplog.text


def test_pdf_branch_loads_concrete_parser_at_runtime(pdf_parser_stub, monkeypatch):
    parser_module = importlib.import_module("rag.flow.parser.parser")
    outputs = {}
    parsed_blobs = []

    class RuntimePdfParser:
        def __init__(self):
            self.outlines = []
            self.page_images = []

        def parse_into_bboxes(self, blob, callback):
            parsed_blobs.append(blob)
            return [{"text": "runtime text", "layout_type": "text"}]

    deepdoc_parser = _module("deepdoc.parser")
    deepdoc_parser.__path__ = []
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser)
    monkeypatch.setattr(pdf_parser_stub, "RAGFlowPdfParser", RuntimePdfParser)
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser_stub)
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_service", None)
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_provider_service", None)
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_instance_service", None)

    parser = types.SimpleNamespace(
        _param=types.SimpleNamespace(
            setups={
                "pdf": {
                    "parse_method": "deepdoc",
                    "output_format": "json",
                    "flatten_media_to_text": False,
                    "remove_toc": False,
                    "remove_header_footer": False,
                }
            }
        ),
        _canvas=types.SimpleNamespace(_tenant_id=None, _language=None),
        callback=lambda *_args, **_kwargs: None,
        set_output=lambda key, value: outputs.__setitem__(key, value),
    )

    parser_module.Parser._pdf(parser, "sample.pdf", b"pdf-data", file={"id": "file-1"})

    assert parsed_blobs == [b"pdf-data"]
    assert outputs["json"] == [{"text": "runtime text", "layout_type": "text", "doc_type_kwd": "text"}]


def test_pdf_parse_method_resolves_tenant_model_services_only_for_unknown_reference(pdf_parser_stub, monkeypatch, caplog):
    parser_module = importlib.import_module("rag.flow.parser.parser")
    calls = []
    caplog.set_level(logging.DEBUG, logger=parser_module.__name__)

    class RuntimeTenantModelService:
        @staticmethod
        def get_by_id(model_id):
            calls.append(("model", model_id))
            return True, types.SimpleNamespace(model_name="ocr-model", provider_id="provider-1", instance_id="instance-1")

    class RuntimeTenantModelProviderService:
        @staticmethod
        def get_by_id(provider_id):
            calls.append(("provider", provider_id))
            return True, types.SimpleNamespace(provider_name="MinerU")

    class RuntimeTenantModelInstanceService:
        @staticmethod
        def get_by_id(instance_id):
            calls.append(("instance", instance_id))
            return True, types.SimpleNamespace(instance_name="runtime")

    monkeypatch.setitem(
        sys.modules,
        "api.db.services.tenant_model_service",
        _module("api.db.services.tenant_model_service", TenantModelService=RuntimeTenantModelService),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.db.services.tenant_model_provider_service",
        _module("api.db.services.tenant_model_provider_service", TenantModelProviderService=RuntimeTenantModelProviderService),
    )
    monkeypatch.setitem(
        sys.modules,
        "api.db.services.tenant_model_instance_service",
        _module("api.db.services.tenant_model_instance_service", TenantModelInstanceService=RuntimeTenantModelInstanceService),
    )

    assert parser_module._resolve_pdf_parse_method("tenant-model-1") == ("MinerU", "ocr-model@runtime@MinerU")
    assert calls == [
        ("model", "tenant-model-1"),
        ("provider", "provider-1"),
        ("instance", "instance-1"),
    ]
    assert "Resolved PDF parser tenant model to provider parser MinerU." in caplog.messages
    assert "tenant-model-1" not in caplog.text


def test_known_pdf_provider_reference_skips_tenant_services(pdf_parser_stub, monkeypatch, caplog):
    parser_module = importlib.import_module("rag.flow.parser.parser")
    raw_model_reference = "private-model@runtime@MinerU"
    caplog.set_level(logging.DEBUG, logger=parser_module.__name__)

    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_service", None)
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_provider_service", None)
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_instance_service", None)

    assert parser_module._resolve_pdf_parse_method(raw_model_reference) == ("MinerU", raw_model_reference)
    assert "Using configured PDF provider parser MinerU without tenant-model lookup." in caplog.messages
    assert raw_model_reference not in caplog.text


def test_pdf_parse_method_logs_safe_fallback_without_model_reference(pdf_parser_stub, monkeypatch, caplog):
    parser_module = importlib.import_module("rag.flow.parser.parser")
    raw_model_reference = "private-tenant-model-reference"
    caplog.set_level(logging.DEBUG, logger=parser_module.__name__)

    class MissingTenantModelService:
        @staticmethod
        def get_by_id(_model_id):
            return False, None

    monkeypatch.setitem(
        sys.modules,
        "api.db.services.tenant_model_service",
        _module("api.db.services.tenant_model_service", TenantModelService=MissingTenantModelService),
    )
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_provider_service", None)
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_model_instance_service", None)

    assert parser_module._resolve_pdf_parse_method(raw_model_reference) == (raw_model_reference, None)
    assert "PDF parser reference did not resolve to a tenant model; using the configured method." in caplog.messages
    assert raw_model_reference not in caplog.text


def test_pdf_position_tag_loads_concrete_parser_at_runtime(pdf_parser_stub, monkeypatch):
    metadata = importlib.import_module("rag.flow.parser.pdf_chunk_metadata")
    monkeypatch.delattr(pdf_parser_stub, "RAGFlowPdfParser")

    assert metadata.extract_pdf_positions({"positions": [[1, 10, 20, 30, 40]]}) == [[1, 10.0, 20.0, 30.0, 40.0]]

    extracted_tags = []

    class RuntimePdfParser:
        @staticmethod
        def extract_positions(value):
            extracted_tags.append(value)
            return [([0], 10.0, 20.0, 30.0, 40.0)]

    monkeypatch.setattr(pdf_parser_stub, "RAGFlowPdfParser", RuntimePdfParser, raising=False)
    position_tag = "@@1\t10\t20\t30\t40##"

    assert metadata.extract_pdf_positions({"position_tag": position_tag}) == [[1, 10.0, 20.0, 30.0, 40.0]]
    assert extracted_tags == [position_tag]
