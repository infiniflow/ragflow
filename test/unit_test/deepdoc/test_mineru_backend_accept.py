import importlib.util
import importlib.machinery
from pathlib import Path

# Provide a minimal dummy RAGFlowPdfParser to avoid importing heavy submodules during tests
import types, sys
pdf_parser_module = types.ModuleType("deepdoc.parser.pdf_parser")
class DummyRAGFlowPdfParser:
    pass
pdf_parser_module.RAGFlowPdfParser = DummyRAGFlowPdfParser
sys.modules["deepdoc.parser.pdf_parser"] = pdf_parser_module

spec = importlib.util.spec_from_file_location(
    "deepdoc.parser.mineru_parser",
    Path(__file__).resolve().parent.parent.parent.parent / "deepdoc" / "parser" / "mineru_parser.py",
)
module = importlib.util.module_from_spec(spec)
loader = spec.loader
assert loader is not None
loader.exec_module(module)
MinerUParser = module.MinerUParser
MinerUBackend = module.MinerUBackend


def test_mineru_backend_enum_has_hybrid():
    assert hasattr(MinerUBackend, 'HYBRID_AUTO_ENGINE')
    assert MinerUBackend.HYBRID_AUTO_ENGINE.value == 'hybrid-auto-engine'


def test_check_installation_accepts_hybrid():
    parser = MinerUParser()
    ok, reason = parser.check_installation(backend='hybrid-auto-engine')
    # Should not raise and should return a boolean; if no server configured, ok may be False but not an invalid-backend error
    assert isinstance(ok, bool)
    assert not (reason.startswith('[MinerU] Invalid backend'))
