import importlib
import sys
import types


class DummyConnection:
    def __init__(self, *args, **kwargs):
        pass


def stub_module(name, **attrs):
    module = types.ModuleType(name)
    for attr_name, attr_value in attrs.items():
        setattr(module, attr_name, attr_value)
    sys.modules[name] = module
    return module


def stub_settings_import_dependencies():
    memory_utils = importlib.import_module("memory.utils")
    from rag.nlp import is_english
    from rag.nlp import rag_tokenizer

    search = stub_module("rag.nlp.search")
    rag_nlp = stub_module("rag.nlp", search=search, rag_tokenizer=rag_tokenizer, is_english=is_english)
    rag_nlp.__path__ = []
    sys.modules["rag.nlp"] = rag_nlp
    sys.modules["rag.nlp.rag_tokenizer"] = rag_tokenizer

    redis_conn = stub_module(
        "rag.utils.redis_conn",
        REDIS_CONN=types.SimpleNamespace(get_or_create_secret_key=lambda key, value: value, health=lambda: True),
    )
    sys.modules["rag.utils.redis_conn"] = redis_conn

    for module_name in [
        "rag.utils.es_conn",
        "rag.utils.infinity_conn",
        "rag.utils.ob_conn",
        "rag.utils.opensearch_conn",
        "memory.utils.es_conn",
        "memory.utils.infinity_conn",
        "memory.utils.ob_conn",
    ]:
        module = stub_module(
            module_name,
            ESConnection=DummyConnection,
            InfinityConnection=DummyConnection,
            OBConnection=DummyConnection,
            OSConnection=DummyConnection,
        )
        if module_name.startswith("memory.utils."):
            setattr(memory_utils, module_name.rsplit(".", 1)[-1], module)

    for module_name, class_name in [
        ("rag.utils.azure_sas_conn", "RAGFlowAzureSasBlob"),
        ("rag.utils.azure_spn_conn", "RAGFlowAzureSpnBlob"),
        ("rag.utils.gcs_conn", "RAGFlowGCS"),
        ("rag.utils.minio_conn", "RAGFlowMinio"),
        ("rag.utils.opendal_conn", "OpenDALStorage"),
        ("rag.utils.s3_conn", "RAGFlowS3"),
        ("rag.utils.oss_conn", "RAGFlowOSS"),
    ]:
        stub_module(module_name, **{class_name: DummyConnection})
