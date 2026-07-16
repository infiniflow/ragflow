import importlib.util
import asyncio
import sys
import types
from contextlib import contextmanager
from pathlib import Path


@contextmanager
def _load_token_chunker_with_stubs():
    root = Path(__file__).resolve().parents[3]
    original_modules = {}

    def _install(name: str, module: types.ModuleType):
        original_modules.setdefault(name, sys.modules.get(name))
        sys.modules[name] = module

    try:
        rag_pkg = types.ModuleType("rag")
        rag_pkg.__path__ = [str(root / "rag")]
        _install("rag", rag_pkg)

        rag_flow_pkg = types.ModuleType("rag.flow")
        rag_flow_pkg.__package__ = "rag"
        rag_flow_pkg.__path__ = [str(root / "rag" / "flow")]
        _install("rag.flow", rag_flow_pkg)

        rag_flow_chunker_pkg = types.ModuleType("rag.flow.chunker")
        rag_flow_chunker_pkg.__package__ = "rag.flow"
        rag_flow_chunker_pkg.__path__ = [str(root / "rag" / "flow" / "chunker")]
        _install("rag.flow.chunker", rag_flow_chunker_pkg)

        rag_flow_parser_pkg = types.ModuleType("rag.flow.parser")
        rag_flow_parser_pkg.__package__ = "rag.flow"
        rag_flow_parser_pkg.__path__ = [str(root / "rag" / "flow" / "parser")]
        _install("rag.flow.parser", rag_flow_parser_pkg)

        common_pkg = types.ModuleType("common")
        common_pkg.__path__ = [str(root / "common")]
        _install("common", common_pkg)

        common_float_utils = types.ModuleType("common.float_utils")
        common_float_utils.normalize_overlapped_percent = lambda value: value
        _install("common.float_utils", common_float_utils)

        common_token_utils = types.ModuleType("common.token_utils")
        common_token_utils.num_tokens_from_string = lambda text: 1
        _install("common.token_utils", common_token_utils)

        rag_nlp = types.ModuleType("rag.nlp")
        rag_nlp.naive_merge = lambda *_args, **_kwargs: []
        _install("rag.nlp", rag_nlp)

        class ProcessParamBase:
            def __init__(self):
                pass

        class ProcessBase:
            def __init__(self, _pipeline, _id, param):
                self._pipeline = _pipeline
                self._id = _id
                self._param = param
                self._outputs = {}
                self.callback = lambda *_args, **_kwargs: None

            def set_output(self, key, value):
                self._outputs[key] = value

        rag_flow_base = types.ModuleType("rag.flow.base")
        rag_flow_base.ProcessBase = ProcessBase
        rag_flow_base.ProcessParamBase = ProcessParamBase
        _install("rag.flow.base", rag_flow_base)

        rag_flow_parser_pdf_metadata = types.ModuleType("rag.flow.parser.pdf_chunk_metadata")
        rag_flow_parser_pdf_metadata.PDF_POSITIONS_KEY = "pdf_positions"
        rag_flow_parser_pdf_metadata.extract_pdf_positions = lambda _item: []
        rag_flow_parser_pdf_metadata.finalize_pdf_chunk = lambda chunk: chunk

        async def restore_pdf_text_previews(*_args, **_kwargs):
            return None

        rag_flow_parser_pdf_metadata.restore_pdf_text_previews = restore_pdf_text_previews
        _install("rag.flow.parser.pdf_chunk_metadata", rag_flow_parser_pdf_metadata)

        try:
            import pydantic  # noqa: F401

            schema_spec = importlib.util.spec_from_file_location(
                "rag.flow.chunker.schema",
                root / "rag" / "flow" / "chunker" / "schema.py",
            )
            if schema_spec is None or schema_spec.loader is None:
                raise RuntimeError("Failed to locate rag.flow.chunker.schema stub loader.")
            schema_module = importlib.util.module_from_spec(schema_spec)
            _install("rag.flow.chunker.schema", schema_module)
            schema_spec.loader.exec_module(schema_module)
        except Exception:
            schema_module = types.ModuleType("rag.flow.chunker.schema")

            class TokenChunkerFromUpstream:
                def __init__(
                    self,
                    name,
                    file=None,
                    chunks=None,
                    output_format=None,
                    json_result=None,
                    markdown_result=None,
                    text_result=None,
                    html_result=None,
                    _created_time=None,
                    _elapsed_time=None,
                ):
                    self.name = name
                    self.file = file
                    self.chunks = chunks
                    self.output_format = output_format
                    self.json_result = json_result
                    self.json = json_result
                    self.markdown_result = markdown_result
                    self.markdown = markdown_result
                    self.text_result = text_result
                    self.text = text_result
                    self.html_result = html_result
                    self.html = html_result
                    self._created_time = _created_time
                    self._elapsed_time = _elapsed_time

                @classmethod
                def model_validate(cls, data):
                    if isinstance(data, dict):
                        return cls(
                            name=data.get("name", ""),
                            file=data.get("file"),
                            chunks=data.get("chunks"),
                            output_format=data.get("output_format"),
                            json_result=data.get("json_result", data.get("json")),
                            markdown_result=data.get("markdown_result", data.get("markdown")),
                            text_result=data.get("text_result", data.get("text")),
                            html_result=data.get("html_result", data.get("html")),
                            _created_time=data.get("_created_time"),
                            _elapsed_time=data.get("_elapsed_time"),
                        )
                    raise TypeError("TokenChunkerFromUpstream expects a dict payload.")

            schema_module.TokenChunkerFromUpstream = TokenChunkerFromUpstream
            _install("rag.flow.chunker.schema", schema_module)

        token_chunker_spec = importlib.util.spec_from_file_location(
            "rag.flow.chunker.token_chunker",
            root / "rag" / "flow" / "chunker" / "token_chunker.py",
        )
        if token_chunker_spec is None or token_chunker_spec.loader is None:
            raise RuntimeError("Failed to locate rag.flow.chunker.token_chunker stub loader.")
        token_chunker_module = importlib.util.module_from_spec(token_chunker_spec)
        _install("rag.flow.chunker.token_chunker", token_chunker_module)
        token_chunker_spec.loader.exec_module(token_chunker_module)
        yield token_chunker_module
    finally:
        for module_name, original in original_modules.items():
            if original is None:
                sys.modules.pop(module_name, None)
            else:
                sys.modules[module_name] = original


def test_token_chunker_prefers_upstream_chunks_for_json_output_format_chunks():
    # Regression for #16812: when the upstream (e.g. TitleChunker) emits
    # output_format="chunks", TokenChunker must consume from_upstream.chunks and
    # not fall through to the raw parser json_result. Heavy deps are stubbed so
    # the real TokenChunker._invoke runs against the real schema when pydantic is
    # available (see title_chunker/common.py for the same chunks-vs-json branch).
    with _load_token_chunker_with_stubs() as token_chunker_module:
        token_chunker = token_chunker_module.TokenChunker
        param = token_chunker_module.TokenChunkerParam()
        param.delimiter_mode = "one"
        chunker = token_chunker(None, "token_chunker", param)

        kwargs = {
            "name": "token_chunker",
            "output_format": "chunks",
            "chunks": [{"text": "CHAPTER-AWARE"}],
            "json": [{"text": "RAW-PARSER-JSON"}],
        }

        asyncio.run(chunker._invoke(**kwargs))

        assert chunker._outputs["chunks"] == [{"text": "CHAPTER-AWARE"}]
