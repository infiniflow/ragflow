import contextvars
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace


def _stub(monkeypatch, name, **attrs):
    """Register a minimal dependency module for isolated Canvas loading."""
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    monkeypatch.setitem(sys.modules, name, module)


def _load_canvas_module(monkeypatch):
    """Load the real Canvas class without its runtime service dependencies."""
    _stub(monkeypatch, "agent.component", component_class=lambda _name: None)
    _stub(monkeypatch, "agent.component.base", ComponentBase=object)
    _stub(monkeypatch, "agent.dsl_migration", normalize_chunker_dsl=lambda dsl: dsl)
    _stub(monkeypatch, "api.db.joint_services.tenant_model_service", get_tenant_default_model_by_type=lambda *_args: None)
    _stub(monkeypatch, "api.db.services.file_service", FileService=object)
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=object)
    _stub(monkeypatch, "api.db.services.task_service", has_canceled=lambda *_args: False)
    _stub(monkeypatch, "common.constants", LLMType=SimpleNamespace(CHAT="chat"))
    _stub(
        monkeypatch,
        "common.llm_request_context",
        set_llm_request_context=lambda **_kwargs: None,
        reset_llm_request_context=lambda _token: None,
    )
    _stub(monkeypatch, "common.exceptions", TaskCanceledException=Exception)
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", hash_str2int=lambda value: hash(value))
    _stub(
        monkeypatch,
        "common.token_utils",
        token_usage_sink=contextvars.ContextVar("token_usage_sink", default=None),
        langfuse_run_attrs=contextvars.ContextVar("langfuse_run_attrs", default=None),
    )
    _stub(monkeypatch, "rag.prompts.generator", chunks_format=lambda *_args: [])
    _stub(monkeypatch, "rag.utils.redis_conn", REDIS_CONN=SimpleNamespace())
    _stub(monkeypatch, "rag.utils.tts_cache", synthesize_with_cache=lambda *_args, **_kwargs: None)

    module_path = Path(__file__).resolve().parents[3] / "agent" / "canvas.py"
    spec = importlib.util.spec_from_file_location("test_canvas_session_reset_module", module_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_start_new_session_discards_inherited_execution_state(monkeypatch):
    """A fresh session starts at Begin instead of resuming a replica's path."""
    module = _load_canvas_module(monkeypatch)
    canvas = module.Canvas.__new__(module.Canvas)
    canvas.history = [{"role": "user", "content": "previous session"}]
    canvas.globals = {"sys.history": [{"role": "user"}]}
    canvas.path = ["begin", "fillup"]

    canvas.start_new_session()

    assert canvas.history == []
    assert canvas.globals["sys.history"] == []
    assert canvas.path == []


def test_start_new_session_normalizes_invalid_system_history(monkeypatch):
    """A fresh session never inherits a malformed system-history value."""
    module = _load_canvas_module(monkeypatch)
    canvas = module.Canvas.__new__(module.Canvas)
    canvas.history = []
    canvas.globals = {"sys.history": None}
    canvas.path = ["fillup"]

    canvas.start_new_session()

    assert canvas.globals["sys.history"] == []
    assert canvas.path == []
