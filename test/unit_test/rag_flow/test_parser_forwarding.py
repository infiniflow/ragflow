import json
from types import SimpleNamespace


def test_parser_config_forwarded_to_mineru(monkeypatch, tmp_path):
    # Prepare a parser param with a pdf setup that includes parser_config
    parser_cfg = {"pages": [[5, 15]], "mineru_batch_size": 25}

    # Fake parser to capture kwargs
    captured = {}

    class FakeParser:
        def parse_pdf(self, filepath, binary=None, callback=None, parse_method=None, lang=None, **kwargs):
            captured.update(kwargs)
            # Return empty result consistent with expected interface
            return [], []

        def crop(self, poss, _):
            return None

        def extract_positions(self, poss):
            return []

    class FakeLLMBundle:
        def __init__(self, tenant_id, llm_type, llm_name=None, lang=None):
            self.mdl = FakeParser()

    # Monkeypatch potentially unpackable heavy dependencies (e.g., scholarly, deepdoc) to avoid import-time errors
    import sys
    import types

    monkeypatch.setitem(sys.modules, 'scholarly', types.ModuleType('scholarly'))
    monkeypatch.setitem(sys.modules, 'deepdoc', types.ModuleType('deepdoc'))
    monkeypatch.setitem(sys.modules, 'deepdoc.vision', types.ModuleType('deepdoc.vision'))
    monkeypatch.setitem(sys.modules, 'deepdoc.vision.ocr', types.ModuleType('deepdoc.vision.ocr'))
    monkeypatch.setitem(sys.modules, 'deepdoc.parser', types.ModuleType('deepdoc.parser'))
    monkeypatch.setitem(sys.modules, 'deepdoc.parser.figure_parser', types.ModuleType('deepdoc.parser.figure_parser'))

    # Import the parser module now (after shielding heavy deps) and patch LLMBundle
    import rag.flow.parser.parser as parser_mod
    from agent.component.base import ComponentBase

    monkeypatch.setattr(parser_mod, "LLMBundle", FakeLLMBundle)

    # Monkeypatch ComponentBase.__init__ to avoid Graph type assertion and heavy setup
    original_init = ComponentBase.__init__

    def fake_init(self, canvas, id, param):
        # set minimal required attributes
        self._canvas = SimpleNamespace(_tenant_id="tenant1", task_id="task1")
        self._id = id
        self._param = param
        try:
            self._param.check()
        except Exception:
            pass

    monkeypatch.setattr(ComponentBase, "__init__", fake_init)

    # Prepare Parser param and instance
    param = parser_mod.ParserParam()
    param.setups["pdf"] = {"parse_method": "MinerU", "mineru_llm_name": "fake_mineru", "mineru_parse_method": "raw", "parser_config": parser_cfg}

    parser_obj = parser_mod.Parser(canvas=None, id="parser", param=param)

    # Call the internal _pdf method with a fake PDF blob
    parser_obj._pdf("/tmp/fake.pdf", b"%PDF-1.4")

    # Restore original init
    monkeypatch.setattr(ComponentBase, "__init__", original_init)

    # Assert parser_config was forwarded to the underlying parse_pdf call
    assert "parser_config" in captured, f"parser_config not forwarded, captured: {captured}"
    assert captured["parser_config"]["pages"] == [[5, 15]]
    assert captured["parser_config"]["mineru_batch_size"] == 25
