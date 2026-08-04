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


def _build_json_chunker(param: dict, monkeypatch_positions=True):
    """Build a TokenChunker (bypassing ComponentBase.__init__) wired for the JSON
    ``delimiter_mode`` path, with heavy deps stubbed.

    Returns ``(chunker, module)`` so callers can monkeypatch the module-global
    ``extract_pdf_positions`` (it is imported as a name, so rebinding the module
    attribute reaches the call sites inside ``_build_json_chunks``).
    """
    with _load_token_chunker_with_stubs() as token_chunker_module:
        token_chunker = token_chunker_module.TokenChunker
        param_obj = token_chunker_module.TokenChunkerParam()
        for key, value in param.items():
            setattr(param_obj, key, value)

        chunker = token_chunker(None, "token_chunker", param_obj)
        chunker._canvas = types.SimpleNamespace(_doc_id=None, _tenant_id="t")
        if monkeypatch_positions:
            # Echo per-item positions so we can assert PDF coordinates survive.
            token_chunker_module.extract_pdf_positions = lambda item: item.get("positions", [])

        yield token_chunker_module, chunker


def test_json_delimiter_mode_drop_delimiter_text():
    # The delimiter is a boundary: its text must never appear inside a chunk.
    for module, chunker in _build_json_chunker({"delimiter_mode": "delimiter", "delimiters": ["`##`"]}):
        kwargs = {
            "name": "token_chunker",
            "output_format": "json",
            "json_result": [{"text": "first part##second part##third part", "doc_type_kwd": "text"}],
        }
        asyncio.run(chunker._invoke(**kwargs))
        chunks = chunker._outputs["chunks"]
        texts = [c["text"] for c in chunks]
        assert texts == ["first part", "second part", "third part"]
        assert all("##" not in t for t in texts)


def test_json_delimiter_mode_newline_join_not_glued():
    # Regression for #17723: JSON flush must join buffered text items with "\\n",
    # never glue them. Two adjacent items "hello" + "world" must stay
    # "hello\\nworld", never become "helloworld".
    for module, chunker in _build_json_chunker({"delimiter_mode": "delimiter", "delimiters": []}):
        kwargs = {
            "name": "token_chunker",
            "output_format": "json",
            "json_result": [
                {"text": "hello", "doc_type_kwd": "text"},
                {"text": "world", "doc_type_kwd": "text"},
            ],
        }
        asyncio.run(chunker._invoke(**kwargs))
        chunks = chunker._outputs["chunks"]
        assert len(chunks) == 1
        assert chunks[0]["text"] == "hello\nworld"


def test_json_delimiter_mode_children_delimiters_applied():
    # Regression for #17723: children_delimiters (secondary split) must run before
    # finalizing the JSON ``delimiter_mode`` path, or they are silently ignored.
    for module, chunker in _build_json_chunker({"delimiter_mode": "delimiter", "delimiters": [], "children_delimiters": ["|"]}):
        kwargs = {
            "name": "token_chunker",
            "output_format": "json",
            "json_result": [{"text": "alpha|beta", "doc_type_kwd": "text"}],
        }
        asyncio.run(chunker._invoke(**kwargs))
        chunks = chunker._outputs["chunks"]
        texts = [c["text"] for c in chunks]
        assert texts == ["alpha", "beta"]


def test_json_delimiter_mode_pdf_positions_retained():
    # PDF coordinates carried on the combined chunk must survive into the output.
    for module, chunker in _build_json_chunker({"delimiter_mode": "delimiter", "delimiters": []}):
        kwargs = {
            "name": "token_chunker",
            "output_format": "json",
            "json_result": [
                {"text": "hello", "doc_type_kwd": "text", "positions": [[1, 0, 10, 0, 5]]},
                {"text": "world", "doc_type_kwd": "text", "positions": [[2, 0, 20, 0, 8]]},
            ],
        }
        asyncio.run(chunker._invoke(**kwargs))
        chunks = chunker._outputs["chunks"]
        assert len(chunks) == 1
        assert chunks[0].get("pdf_positions") == [[1, 0, 10, 0, 5], [2, 0, 20, 0, 8]]


def test_json_delimiter_mode_pdf_positions_per_segment_not_broadcast():
    # Regression for #3 (PDF coordinate leak): when consecutive text items from
    # different pages are buffered and then split by a custom delimiter, each
    # output segment must carry only the PDF positions of the item(s) that
    # contributed to it -- not the union of every buffered item. The old code
    # broadcast ``combined_pos`` to every split chunk, so a page-1 segment also
    # claimed page-2 coordinates and all segments shared one PDF preview image.
    for module, chunker in _build_json_chunker({"delimiter_mode": "delimiter", "delimiters": ["`。`"]}):
        # Mirror production's preview-cache behaviour: a chunk's preview image is
        # keyed by its position set, so chunks sharing positions share one image.
        async def _restore_previews(chunks, from_upstream, canvas):
            preview_cache = {}
            for chunk in chunks:
                positions = chunk.get("pdf_positions") or []
                key = tuple(tuple(p[:5]) for p in positions)
                if key in preview_cache:
                    chunk["img_id"] = preview_cache[key]
                else:
                    new_id = "img-%d" % len(preview_cache)
                    chunk["img_id"] = new_id
                    preview_cache[key] = new_id

        module.restore_pdf_text_previews = _restore_previews

        kwargs = {
            "name": "doc.pdf",
            "output_format": "json",
            "json_result": [
                {"text": "第一章。第二段", "doc_type_kwd": "text", "positions": [[1, 0, 10, 0, 5]]},
                {"text": "第三章。第四章", "doc_type_kwd": "text", "positions": [[2, 0, 20, 0, 8]]},
            ],
        }
        asyncio.run(chunker._invoke(**kwargs))
        chunks = chunker._outputs["chunks"]
        texts = [c["text"] for c in chunks]
        # Custom "。" splits the buffered text into three segments; the "\n" join
        # between the two items is NOT a split point, so the middle segment spans
        # both pages.
        assert texts == ["第一章", "第二段\n第三章", "第四章"], texts

        positions = [c.get("pdf_positions") for c in chunks]
        # Page-1-only segment must NOT carry page-2 coordinates.
        assert positions[0] == [[1, 0, 10, 0, 5]], positions
        # Spanning segment legitimately carries both pages.
        assert positions[1] == [[1, 0, 10, 0, 5], [2, 0, 20, 0, 8]], positions
        # Page-2-only segment must NOT carry page-1 coordinates.
        assert positions[2] == [[2, 0, 20, 0, 8]], positions

        # Previews must not be shared: distinct position sets -> distinct images.
        img_ids = [c.get("img_id") for c in chunks]
        assert len(set(img_ids)) == len(img_ids), img_ids


def test_json_delimiter_mode_consecutive_delimiter_keeps_boundary():
    # Regression for #17723: "A####B" with pattern "##" must yield ["A", "B"],
    # both boundary-adjacent segments preserved (the bug collapsed it to "A##B").
    for module, chunker in _build_json_chunker({"delimiter_mode": "delimiter", "delimiters": ["`##`"]}):
        kwargs = {
            "name": "token_chunker",
            "output_format": "json",
            "json_result": [{"text": "A####B", "doc_type_kwd": "text"}],
        }
        asyncio.run(chunker._invoke(**kwargs))
        chunks = chunker._outputs["chunks"]
        texts = [c["text"] for c in chunks]
        assert texts == ["A", "B"]
        assert all("##" not in t for t in texts)


def test_text_delimiter_mode_token_size_zero_or_one_no_atom_split():
    # token_size=0/1 must not atom-split delimiter segments into 1-token chunks;
    # the delimiter path produces delimiter-boundary chunks regardless of cap.
    for module, chunker in _build_json_chunker({"delimiter_mode": "token_size", "delimiters": ["`|`"]}):
        # text path: delimiter_mode is token_size but a custom delimiter is
        # present, so the delimiter branch (_split_text_by_pattern) is used.
        kwargs = {
            "name": "token_chunker",
            "output_format": "text",
            "text": "aaa|bbb|ccc",
        }
        for chunk_token_size in (0, 1):
            setattr(chunker._param, "chunk_token_size", chunk_token_size)
            asyncio.run(chunker._invoke(**kwargs))
            chunks = chunker._outputs["chunks"]
            texts = [c["text"] for c in chunks]
            assert texts == ["aaa", "bbb", "ccc"], f"token_size={chunk_token_size} atom-split a delimiter segment: {texts}"
