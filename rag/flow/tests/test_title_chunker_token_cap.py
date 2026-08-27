import asyncio
import importlib
import importlib.util
import sys
import types
from contextlib import contextmanager
from pathlib import Path

"""TDD tests for the TitleChunker token-count CAP (chunk_token_cap).

The TitleChunker (both ``hierarchy`` and ``group`` methods) historically had
no token-size ceiling: a long section with no sub-heading became one giant
chunk. This suite pins the new behaviour:

  * A configurable ``chunk_token_cap`` (default 512, valid 128..8000) is a
    hard ceiling on every *text* chunk's token count.
  * When a built chunk exceeds the cap, it is re-split on sentence boundaries
    (``。!?？；！\\n`` plus the English ``. ``) and the pieces are greedily
    merged into <= cap sub-chunks.
  * A single boundary-less run that still exceeds the cap is hard-split so the
    ceiling always holds (fallback only).
  * Table/image chunks are atomic and never split.
  * ``chunk_token_cap == 0`` disables the ceiling.

Token counting is faked as 1 token == 1 character so the assertions are fully
deterministic (mirrors the stub approach in test_title_chunker_position_int.py,
which also fakes ``num_tokens_from_string``).
"""


@contextmanager
def _load_title_chunker_with_stubs():
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

        # Deterministic tokenizer: 1 token per character. ``truncate`` mirrors
        # token_utils.truncate semantics (prefix bounded by max_len tokens).
        common_token_utils = types.ModuleType("common.token_utils")
        common_token_utils.num_tokens_from_string = lambda text: len(text or "")
        common_token_utils.truncate = lambda text, max_len: (text or "")[:max_len]
        _install("common.token_utils", common_token_utils)

        rag_nlp = types.ModuleType("rag.nlp")
        rag_nlp.naive_merge = lambda *args, **kwargs: []
        rag_nlp.not_bullet = lambda text: False
        rag_nlp.not_title = lambda text: True
        _install("rag.nlp", rag_nlp)

        deepdoc_pkg = types.ModuleType("deepdoc")
        deepdoc_pkg.__path__ = [str(root / "deepdoc")]
        _install("deepdoc", deepdoc_pkg)

        deepdoc_parser_pkg = types.ModuleType("deepdoc.parser")
        deepdoc_parser_pkg.__path__ = [str(root / "deepdoc" / "parser")]
        _install("deepdoc.parser", deepdoc_parser_pkg)

        class _RAGFlowPdfParser:
            @staticmethod
            def remove_tag(text):
                return text

            @staticmethod
            def extract_positions(tag):
                return []

        deepdoc_pdf_parser = types.ModuleType("deepdoc.parser.pdf_parser")
        deepdoc_pdf_parser.RAGFlowPdfParser = _RAGFlowPdfParser
        _install("deepdoc.parser.pdf_parser", deepdoc_pdf_parser)

        deepdoc_parser_utils = types.ModuleType("deepdoc.parser.utils")
        deepdoc_parser_utils.extract_pdf_outlines = lambda *args, **kwargs: []
        _install("deepdoc.parser.utils", deepdoc_parser_utils)

        class ProcessParamBase:
            def __init__(self):
                pass

            def check_valid_value(self, value, msg, allowed):
                if value not in allowed:
                    raise ValueError(msg)

            def check_positive_integer(self, value, msg):
                if not (isinstance(value, int) and value > 0):
                    raise ValueError(msg)

            def check_decimal_float(self, value, msg):
                pass

            def check_nonnegative_number(self, value, msg):
                pass

            def check_empty(self, value, msg):
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

        pdf_chunk_metadata = types.ModuleType("rag.flow.parser.pdf_chunk_metadata")
        pdf_chunk_metadata.PDF_POSITIONS_KEY = "pdf_positions"
        pdf_chunk_metadata.extract_pdf_positions = lambda _item: []
        pdf_chunk_metadata.merge_pdf_positions = lambda _records: []
        pdf_chunk_metadata.finalize_pdf_chunk = lambda chunk: chunk

        async def _noop_restore(*_a, **_k):
            return None

        pdf_chunk_metadata.restore_pdf_text_previews = _noop_restore
        _install("rag.flow.parser.pdf_chunk_metadata", pdf_chunk_metadata)

        common_spec = importlib.util.spec_from_file_location(
            "rag.flow.chunker.title_chunker.common",
            root / "rag" / "flow" / "chunker" / "title_chunker" / "common.py",
        )
        common_module = importlib.util.module_from_spec(common_spec)
        _install("rag.flow.chunker.title_chunker.common", common_module)
        common_spec.loader.exec_module(common_module)

        hierarchy_spec = importlib.util.spec_from_file_location(
            "rag.flow.chunker.title_chunker.hierarchy_chunker",
            root / "rag" / "flow" / "chunker" / "title_chunker" / "hierarchy_chunker.py",
        )
        hierarchy_module = importlib.util.module_from_spec(hierarchy_spec)
        _install("rag.flow.chunker.title_chunker.hierarchy_chunker", hierarchy_module)
        hierarchy_spec.loader.exec_module(hierarchy_module)

        group_spec = importlib.util.spec_from_file_location(
            "rag.flow.chunker.title_chunker.group_chunker",
            root / "rag" / "flow" / "chunker" / "title_chunker" / "group_chunker.py",
        )
        group_module = importlib.util.module_from_spec(group_spec)
        _install("rag.flow.chunker.title_chunker.group_chunker", group_module)
        group_spec.loader.exec_module(group_module)

        yield common_module, hierarchy_module, group_module
    finally:
        for module_name, original in original_modules.items():
            if original is None:
                sys.modules.pop(module_name, None)
            else:
                sys.modules[module_name] = original


def _real_extract_pdf_positions(item):
    if not isinstance(item, dict):
        return []
    positions = item.get("pdf_positions")
    if isinstance(positions, list):
        return [list(p) for p in positions]
    positions = item.get("positions")
    if isinstance(positions, list):
        return [list(p) for p in positions]
    return []


def _real_merge_pdf_positions(records):
    merged = []
    for rec in records or []:
        if not isinstance(rec, dict):
            continue
        for pos in rec.get("pdf_positions") or []:
            if isinstance(pos, (list, tuple)) and len(pos) >= 5:
                merged.append([pos[0], pos[1], pos[2], pos[3], pos[4]])
    seen = set()
    out = []
    for pos in merged:
        key = tuple(pos[:5])
        if key not in seen:
            seen.add(key)
            out.append(pos)
    return out


def _real_finalize_pdf_chunk(chunk):
    positions = _real_extract_pdf_positions(chunk)
    if positions:
        chunk["position_int"] = [list(p) for p in positions]
        chunk.pop("pdf_positions", None)
    return chunk


def _run(method, from_upstream, **param_kwargs):
    with _load_title_chunker_with_stubs() as (common_module, hierarchy_module, group_module):
        if param_kwargs.pop("faithful_positions", False):
            common_module.extract_pdf_positions = _real_extract_pdf_positions
            common_module.merge_pdf_positions = _real_merge_pdf_positions
            common_module.finalize_pdf_chunk = _real_finalize_pdf_chunk

        param = common_module.TitleChunkerParam()
        param.method = method
        param.chunk_token_cap = param_kwargs.pop("chunk_token_cap", 512)
        for key, value in param_kwargs.items():
            setattr(param, key, value)

        process = common_module.ProcessBase(None, "title_chunker", param)
        process._canvas = types.SimpleNamespace(_doc_id="doc", _tenant_id="tenant")
        process._outputs = {}

        if method == "hierarchy":
            chunker = hierarchy_module.HierarchyTitleChunker(process, from_upstream)
        else:
            chunker = group_module.GroupTitleChunker(process, from_upstream)
        asyncio.run(chunker.invoke())
        return process._outputs.get("chunks", [])


def _json_upstream(items, output_format="json"):
    return types.SimpleNamespace(
        output_format=output_format,
        json_result=items if output_format == "json" else None,
        markdown_result=None,
        text_result=None,
        html_result=None,
        chunks=None if output_format == "json" else items,
        file=None,
        name="cap-test",
    )


def _char_count(text):
    return len(text or "")


# --------------------------------------------------------------------------- #
# 1. hierarchy: a single oversized chunk is re-split and stays within the cap #
# --------------------------------------------------------------------------- #
def test_hierarchy_oversized_chunk_respects_cap():
    # 12 sentences of 4 chars each ("Saa。") -> 48 tokens, cap 20 -> >=3 chunks.
    body = "。".join(f"S{i:02d}" for i in range(12)) + "。"
    items = [{"text": body, "doc_type_kwd": "text"}]
    chunks = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=20)

    assert len(chunks) > 1, "expected the oversized chunk to be split"
    for ck in chunks:
        assert ck.get("doc_type_kwd", "text") == "text"
        assert _char_count(ck["text"]) <= 20, f"chunk exceeds cap: {ck['text']!r}"
    # Content is preserved (single record -> build_chunks appends one trailing
    # newline, which the sentence split keeps as a boundary; rstrip for compare).
    assert "".join(ck["text"] for ck in chunks).rstrip("\n") == body


# --------------------------------------------------------------------------- #
# 2. group: same guarantee, different code path                               #
# --------------------------------------------------------------------------- #
def test_group_oversized_chunk_respects_cap():
    body = "。".join(f"S{i:02d}" for i in range(12)) + "。"
    items = [{"text": body, "doc_type_kwd": "text"}]
    chunks = _run("group", _json_upstream(items), levels=[], hierarchy=0, chunk_token_cap=20)

    assert len(chunks) > 1
    for ck in chunks:
        assert _char_count(ck["text"]) <= 20
    assert "".join(ck["text"] for ck in chunks).rstrip("\n") == body


# --------------------------------------------------------------------------- #
# 3. split happens on sentence boundaries, not mid-sentence                   #
# --------------------------------------------------------------------------- #
def test_split_keeps_sentence_boundaries():
    # Every sentence ends with "。"; after greedy merge each chunk must still
    # end on a boundary (no mid-sentence cut while boundaries are available).
    body = "。".join(f"Sentence number {i} ends here" for i in range(10)) + "。"
    items = [{"text": body, "doc_type_kwd": "text"}]
    chunks = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=40)

    assert len(chunks) > 1
    for ck in chunks:
        assert ck["text"].rstrip("\n").endswith("。"), f"chunk cut mid-sentence: {ck['text']!r}"


# --------------------------------------------------------------------------- #
# 4. chunk_token_cap == 0 disables the ceiling                               #
# --------------------------------------------------------------------------- #
def test_cap_zero_disables_splitting():
    body = "。".join(f"S{i:02d}" for i in range(12)) + "。"
    items = [{"text": body, "doc_type_kwd": "text"}]
    chunks = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=0)

    assert len(chunks) == 1, "cap=0 must keep the single-chunk behaviour"
    assert chunks[0]["text"].rstrip("\n") == body


# --------------------------------------------------------------------------- #
# 5. table/image chunks are atomic and never split                            #
# --------------------------------------------------------------------------- #
def test_non_text_chunk_is_atomic():
    big_table = "x" * 200  # 200 tokens, far above any reasonable cap
    items = [{"text": big_table, "doc_type_kwd": "table"}]
    chunks = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=20)

    assert len(chunks) == 1
    assert chunks[0]["doc_type_kwd"] == "table"
    assert chunks[0]["text"] == big_table


# --------------------------------------------------------------------------- #
# 6. pathological single run with no boundary is hard-split (fallback)        #
# --------------------------------------------------------------------------- #
def test_boundaryless_run_is_hard_split():
    # 100 chars, no punctuation -> sentence split yields one giant segment that
    # must be hard-split so the cap still holds.
    body = "x" * 100
    items = [{"text": body, "doc_type_kwd": "text"}]
    chunks = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=20)

    assert len(chunks) > 1
    for ck in chunks:
        assert _char_count(ck["text"]) <= 20
    assert "".join(ck["text"] for ck in chunks).rstrip("\n") == body


# --------------------------------------------------------------------------- #
# 7. PDF positions: every sub-chunk keeps the source coordinates              #
# --------------------------------------------------------------------------- #
def test_position_all_subchunks_keep_coordinates():
    body = "。".join(f"S{i:02d}" for i in range(12)) + "。"
    items = [{"text": body, "doc_type_kwd": "text", "positions": [[1, 10, 200, 50, 80]]}]
    chunks = _run(
        "hierarchy",
        _json_upstream(items),
        levels=[],
        chunk_token_cap=20,
        faithful_positions=True,
    )

    assert len(chunks) > 1
    for ck in chunks:
        assert ck.get("position_int") == [[1, 10, 200, 50, 80]], "every sub-chunk must keep the source coordinates"


# --------------------------------------------------------------------------- #
# 7d. sub-chunk position lists are independent deep copies (no aliasing)       #
# --------------------------------------------------------------------------- #
def test_split_subchunk_positions_not_aliased():
    # _split_text_chunk_by_cap must give every sub-chunk its own deep copy of
    # the source position list: sharing one list object across sub-chunks
    # would let a future in-place mutation silently corrupt all of them.
    body = "。".join(f"S{i:02d}" for i in range(12)) + "。"
    source_positions = [[1, 10, 200, 50, 80], [2, 15, 190, 60, 90]]

    with _load_title_chunker_with_stubs() as (common_module, _hierarchy_module, _group_module):
        key = common_module.PDF_POSITIONS_KEY

        class _Splitter(common_module.BaseTitleChunker):
            # _split_text_chunk_by_cap uses no instance state, so skip the
            # process/from_upstream wiring of the real constructor.
            def __init__(self):
                pass

            def resolve_levels(self, line_records):
                return None

            def build_chunks(self, line_records, resolved):
                return []

        chunker = _Splitter()
        chunk = {"text": body, "doc_type_kwd": "text", key: source_positions}
        subs = common_module.BaseTitleChunker._split_text_chunk_by_cap(chunker, chunk, 20)

    assert len(subs) > 1
    for sub in subs:
        assert sub[key] == source_positions, "every sub-chunk keeps the source coordinates"
        assert sub[key] is not source_positions, "sub-chunk must not alias the source position list"
    # Deep independence: mutating one sub-chunk's nested coordinates must not
    # leak into the source chunk or any other sub-chunk.
    subs[0][key][0][0] = 999
    assert source_positions[0][0] == 1
    assert all(sub[key][0][0] == 1 for sub in subs[1:])


# --------------------------------------------------------------------------- #
# 7b. text output format (no doc_type_kwd) also honours the cap               #
# --------------------------------------------------------------------------- #
def test_hierarchy_text_format_respects_cap():
    body = "。".join(f"S{i:02d}" for i in range(12)) + "。"
    from_upstream = types.SimpleNamespace(
        output_format="text",
        json_result=None,
        markdown_result=None,
        text_result=body,
        html_result=None,
        chunks=None,
        file=None,
        name="cap-test",
    )
    chunks = _run("hierarchy", from_upstream, levels=[], chunk_token_cap=20)

    assert len(chunks) > 1
    for ck in chunks:
        assert _char_count(ck["text"]) <= 20
    assert "".join(ck["text"] for ck in chunks).rstrip("\n") == body


# --------------------------------------------------------------------------- #
# 7c. multiple records merged into one oversized chunk are re-split           #
# --------------------------------------------------------------------------- #
def test_hierarchy_multi_record_merged_respects_cap():
    items = [{"text": f"S{i:02d}。", "doc_type_kwd": "text"} for i in range(6)]
    chunks = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=10)

    assert len(chunks) > 1
    for ck in chunks:
        assert _char_count(ck["text"]) <= 10
    expected_built = "".join(f"S{i:02d}。" + "\n" for i in range(6))
    assert "".join(ck["text"] for ck in chunks) == expected_built


# --------------------------------------------------------------------------- #
# 7d. a chunk already within the cap is left untouched (no spurious split)   #
# --------------------------------------------------------------------------- #
def test_chunk_within_cap_not_split():
    body = "S00。S01。"
    items = [{"text": body, "doc_type_kwd": "text"}]
    chunks = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=512)

    assert len(chunks) == 1
    assert chunks[0]["text"].rstrip("\n") == body


# --------------------------------------------------------------------------- #
# 8. range validation in TitleChunkerParam.check                              #
# --------------------------------------------------------------------------- #
def test_chunk_token_cap_range_validation():
    with _load_title_chunker_with_stubs() as (common_module, _h, _g):
        param = common_module.TitleChunkerParam()
        param.method = "hierarchy"
        param.levels = []

        param.chunk_token_cap = 50
        try:
            param.check()
            raise AssertionError("expected ValueError for cap < 128")
        except ValueError:
            pass

        param.chunk_token_cap = 9000
        try:
            param.check()
            raise AssertionError("expected ValueError for cap > 8000")
        except ValueError:
            pass

        # Valid defaults must not raise.
        param.chunk_token_cap = 512
        param.check()
        param.chunk_token_cap = 128
        param.check()
        param.chunk_token_cap = 8000
        param.check()


# --------------------------------------------------------------------------- #
# 9. hard split is lossless: sub-chunks concatenate back to the original text  #
# --------------------------------------------------------------------------- #
def test_hard_split_is_lossless():
    # A boundary-less run is hard-split; concatenating every sub-chunk must
    # reproduce the original built chunk text exactly (no characters dropped
    # or duplicated across the token-prefix truncation).
    body = "x" * 100
    items = [{"text": body, "doc_type_kwd": "text"}]

    disabled = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=0)
    original = disabled[0]["text"]

    capped = _run("hierarchy", _json_upstream(items), levels=[], chunk_token_cap=20)
    assert len(capped) > 1, "expected the oversized chunk to be hard-split"
    assert "".join(ck["text"] for ck in capped) == original


# --------------------------------------------------------------------------- #
# 10. tokenizer failure still enforces the cap via character-count fallback    #
# --------------------------------------------------------------------------- #
def test_tokenizer_failure_falls_back_to_char_count():
    # Simulate an unavailable tokenizer: num_tokens_from_string returns 0 for
    # every text. The cap must still apply (via the character-count fallback)
    # instead of silently leaving a giant chunk intact.
    body = "x" * 100
    items = [{"text": body, "doc_type_kwd": "text"}]

    with _load_title_chunker_with_stubs() as (common_module, hierarchy_module, _group_module):
        # Override the stubbed tokenizer to always report 0 tokens.
        common_module.num_tokens_from_string = lambda text: 0

        param = common_module.TitleChunkerParam()
        param.method = "hierarchy"
        param.chunk_token_cap = 20
        process = common_module.ProcessBase(None, "title_chunker", param)
        process._canvas = types.SimpleNamespace(_doc_id="doc", _tenant_id="tenant")
        process._outputs = {}

        chunker = hierarchy_module.HierarchyTitleChunker(process, _json_upstream(items))
        asyncio.run(chunker.invoke())
        chunks = process._outputs.get("chunks", [])

    assert len(chunks) > 1, "cap must still apply when the tokenizer reports 0"
    for ck in chunks:
        # The character-count fallback must still honour the ceiling, not just
        # split somewhere: every emitted piece stays within the cap.
        assert len(ck["text"]) <= 20, f"fallback chunk exceeds cap: {ck['text']!r}"
    assert "".join(ck["text"] for ck in chunks) == "x" * 100 + "\n"


# --------------------------------------------------------------------------- #
# 11. hard split is lossless with the REAL tiktoken-backed truncate           #
# --------------------------------------------------------------------------- #
def test_hard_split_lossless_with_real_truncate():
    # _hard_split_by_tokens advances with rest[len(head):], which is only
    # lossless if head is a true prefix of rest. The real truncate
    # (encoder.decode(encoder.encode(s)[:n])) can decode a mid-multibyte token
    # boundary as U+FFFD, which is NOT a prefix. This exercises the hard-split
    # path with the real tokenizer on multibyte, boundary-less text and asserts
    # the concatenated sub-chunks reproduce the original text exactly (no
    # character dropped at the split boundary).
    import importlib.util

    root = Path(__file__).resolve().parents[3]
    spec = importlib.util.spec_from_file_location("ragflow_real_token_utils", str(root / "common" / "token_utils.py"))
    real_token_utils = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(real_token_utils)

    # Multibyte, no sentence boundary -> forces the hard-split fallback path.
    body = "中文混合English文本再混入一些标点符号\uff0c" * 30
    items = [{"text": body, "doc_type_kwd": "text"}]

    with _load_title_chunker_with_stubs() as (common_module, hierarchy_module, _g):
        # Swap the 1-token-per-char stub for the REAL tiktoken-backed truncate
        # AND the real token counter, so the cap is enforced and verified in
        # real token units (not characters).
        common_module.truncate = real_token_utils.truncate
        common_module.num_tokens_from_string = real_token_utils.num_tokens_from_string

        param = common_module.TitleChunkerParam()
        param.method = "hierarchy"
        param.chunk_token_cap = 20
        process = common_module.ProcessBase(None, "title_chunker", param)
        process._canvas = types.SimpleNamespace(_doc_id="doc", _tenant_id="tenant")
        process._outputs = {}
        chunker = hierarchy_module.HierarchyTitleChunker(process, _json_upstream(items))
        asyncio.run(chunker.invoke())
        chunks = process._outputs.get("chunks", [])

    assert len(chunks) > 1, "expected the oversized chunk to be hard-split"
    for ck in chunks:
        # Every emitted piece must honour the 20-token cap under the real
        # tokenizer, not just be a lossless concatenation.
        assert real_token_utils.num_tokens_from_string(ck["text"]) <= 20, f"hard-split chunk exceeds token cap: {ck['text']!r}"
    assert "".join(ck["text"] for ck in chunks) == body + "\n"
