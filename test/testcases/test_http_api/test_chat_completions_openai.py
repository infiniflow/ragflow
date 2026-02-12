import importlib.util
from pathlib import Path


_TARGET = (
    Path(__file__).resolve().parent
    / "test_session_management"
    / "test_session_management_chat_completions_openai.py"
)
_SPEC = importlib.util.spec_from_file_location(
    "test_session_management_chat_completions_openai",
    _TARGET,
)
_MODULE = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(_MODULE)
globals().update({k: v for k, v in _MODULE.__dict__.items() if not k.startswith("_")})
