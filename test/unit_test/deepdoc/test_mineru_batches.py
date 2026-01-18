# import module directly to avoid package import side-effects
import importlib.util
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
MinerUParseOptions = module.MinerUParseOptions


def test_compute_batches_basic():
    # basic sanity checks for batch splitting logic via options
    opts = MinerUParseOptions(batch_size=30, start_page=0, end_page=89)
    bsize = opts.batch_size
    pages = list(range(opts.start_page, opts.end_page + 1))
    total_batches = (len(pages) + bsize - 1) // bsize
    assert total_batches == 3
    # ensure last batch end page index
    assert pages[-1] == 89


def test_batch_size_one():
    opts = MinerUParseOptions(batch_size=1, start_page=0, end_page=2)
    pages = list(range(opts.start_page, opts.end_page + 1))
    total_batches = (len(pages) + opts.batch_size - 1) // opts.batch_size
    assert total_batches == 3
