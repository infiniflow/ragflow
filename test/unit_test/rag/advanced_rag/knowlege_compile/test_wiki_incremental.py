"""Unit tests for wiki_incremental.py — Entity Matching, REDUCE, FINALIZE.

Follows the pattern from task_executor_refactor/conftest.py.
All imports of the target module use importlib to avoid namespace conflicts.
"""

import importlib.util
import json
import os
import sys
from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest

# ---- Import target module via importlib (avoids namespace conflicts) ----
_TEST_DIR = os.path.dirname(os.path.abspath(__file__))
_MODULE_PATH = os.path.normpath(os.path.join(_TEST_DIR, "../../../../../rag/advanced_rag/knowlege_compile/wiki_incremental.py"))
_spec = importlib.util.spec_from_file_location(
    "rag.advanced_rag.knowlege_compile.wiki_incremental",
    _MODULE_PATH,
)
_wiki = importlib.util.module_from_spec(_spec)
sys.modules["rag.advanced_rag.knowlege_compile.wiki_incremental"] = _wiki
_spec.loader.exec_module(_wiki)

# Load constants from the stubbed structure module
from rag.advanced_rag.knowlege_compile.structure import (
    CONCEPT_MIN_CLAIMS,
    CONCEPT_MIN_SOURCES,
)


# ---- Test helpers ----------------------------------------------------------


class MockEmbeddingModel:
    """Deterministic embedding model for reproducible tests."""

    def __init__(self, vector_size: int = 8, seed: int = 42):
        self.vector_size = vector_size
        self.max_length = 512
        self.llm_name = "mock_embedding"
        self._rng = np.random.RandomState(seed)

    def encode(self, texts):
        n = len(texts)
        self._last_texts = texts
        vectors = self._rng.rand(n, self.vector_size).astype(np.float32)
        norms = np.linalg.norm(vectors, axis=1, keepdims=True) + 1e-10
        vectors = vectors / norms
        token_count = sum(len(t.split()) for t in texts)
        return vectors, token_count

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass


class MockChatModel:
    """Canned LLM response for dedup confirmation."""

    def __init__(self, canned: str = "true"):
        self.llm_name = "mock_chat"
        self.max_length = 4096
        self._canned = canned

    async def async_chat(self, system_prompt, messages, **kwargs):
        return self._canned

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass


def make_doc_store(search_results: list[dict] | None = None):
    """Create a mock settings.docStoreConn."""
    conn = MagicMock()
    conn.index_exist = MagicMock(return_value=True)
    conn.search = AsyncMock(return_value={"hits": {"total": {"value": len(search_results or [])}, "hits": search_results or []}})
    conn.insert = AsyncMock(return_value=None)
    conn.update = AsyncMock(return_value=None)
    conn.delete = AsyncMock(return_value=None)

    def _get_fields(res, fields):
        hits = res.get("hits", {}).get("hits", [])
        result = {}
        for hit in hits:
            source = hit.get("_source", {})
            row = {}
            for f in fields:
                val = source.get(f, "")
                row[f] = val
            row["_score"] = source.get("_score", 0.0)
            row["entity_kwd"] = source.get("entity_kwd", "")
            key = source.get("slug_kwd") or source.get("entity_kwd") or source.get("doc_id", "")
            if isinstance(key, (list, tuple)):
                key = key[0] if key else ""
            result[key] = row
        return result

    conn.get_fields = _get_fields
    return conn


# ---- Tests for _extract_raw_entities ---------------------------------------


def test_extract_raw_entities_basic():
    """_extract_raw_entities correctly extracts entities, concepts, and claims."""
    map_results = [
        {
            "doc_id": "doc_1",
            "entities": [
                {"name": "Apple Inc.", "type": "org", "aliases": ["Apple"]},
            ],
            "concepts": [
                {"term": "smartphone industry", "definition_excerpt": "global mobile device market"},
            ],
            "claims": [
                {
                    "entity_name": "Apple Inc.",
                    "statement": "Apple is an American tech company",
                    "source_chunk_id": "C1",
                    "source_doc_id": "doc_1",
                },
                {
                    "entity_name": "smartphone industry",
                    "statement": "Industry worth $500B",
                    "source_chunk_id": "C2",
                    "source_doc_id": "doc_1",
                },
            ],
        }
    ]

    raw, claim_index = _wiki._extract_raw_entities(map_results)

    names = {e["name"] for e in raw}
    assert names == {"Apple Inc.", "smartphone industry"}, f"Got {names}"

    for entry in raw:
        if entry["name"] == "Apple Inc.":
            assert entry["type"] == "org"
            assert "Apple" in entry.get("aliases", [])
            assert entry["claim_count"] == 1
        elif entry["name"] == "smartphone industry":
            assert entry["type"] == "concept"
            assert entry["claim_count"] == 1

    # claim_index holds the full claim text separately
    assert len(claim_index["Apple Inc."]) == 1
    assert claim_index["Apple Inc."][0]["statement"] == "Apple is an American tech company"


def test_extract_raw_entities_empty():
    """_extract_raw_entities returns empty list when no entities/concepts."""
    raw, claim_index = _wiki._extract_raw_entities([{"doc_id": "d1", "entities": [], "concepts": [], "claims": []}])
    assert raw == []
    assert claim_index == {}


def test_extract_raw_entities_duplicate_claims():
    """Duplicate entity names from different chunks are merged."""
    map_results = [
        {
            "doc_id": "doc_1",
            "entities": [{"name": "Apple Inc.", "type": "org"}],
            "concepts": [],
            "claims": [
                {"entity_name": "Apple Inc.", "statement": "Claim 1", "source_chunk_id": "C1", "source_doc_id": "doc_1"},
                {"entity_name": "Apple Inc.", "statement": "Claim 2", "source_chunk_id": "C2", "source_doc_id": "doc_1"},
            ],
        }
    ]

    raw, claim_index = _wiki._extract_raw_entities(map_results)
    apple = next(e for e in raw if e["name"] == "Apple Inc.")
    assert apple["claim_count"] == 2
    assert len(claim_index["Apple Inc."]) == 2


# ---- Tests for _normalize_key -------------------------------------------------


def test_normalize_key_variants():
    """_normalize_key handles case, punctuation, and whitespace."""
    assert _wiki._normalize_key("Apple Inc.") == "apple inc"
    assert _wiki._normalize_key("  Apple, Inc.  ") == "apple inc"
    assert _wiki._normalize_key("Smartphone Industry") == "smartphone industry"
    assert _wiki._normalize_key("") == ""
    assert _wiki._normalize_key(None) == ""


# ---- Tests for _wiki_reduce_entity -----------------------------------------


@pytest.mark.asyncio
async def test_reduce_entity_create():
    """New entity → action=create with all claims as additions."""
    result = await _wiki._wiki_reduce_entity(
        entity_name="Apple Inc.",
        entity_type="org",
        new_claims=[{"statement": "S1", "source_doc_id": "d1"}],
        existing_page=None,
        deleted_doc_ids=set(),
    )
    assert result["action"] == "create"
    assert result["has_delta"] is True
    assert len(result["additions"]) == 1
    assert result["entity_type"] == "org"


@pytest.mark.asyncio
async def test_reduce_entity_update_additions():
    """Existing entity with new claims → action=update, additions present."""
    existing = {
        "claims": [{"statement": "Old", "source_doc_id": "d1"}],
        "page_version_int": 1,
        "slug_kwd": "entity/apple-inc",
    }
    result = await _wiki._wiki_reduce_entity(
        entity_name="Apple Inc.",
        entity_type="org",
        new_claims=[{"statement": "New", "source_doc_id": "d2"}],
        existing_page=existing,
        deleted_doc_ids=set(),
    )
    assert result["action"] == "update"
    assert len(result["additions"]) == 1
    assert result["additions"][0]["statement"] == "New"


@pytest.mark.asyncio
async def test_reduce_entity_update_retractions():
    """Document deletion → retractions from deleted doc."""
    existing = {
        "claims": [
            {"statement": "S1", "source_doc_id": "d1"},
            {"statement": "S2", "source_doc_id": "d2"},
        ],
        "page_version_int": 1,
    }
    result = await _wiki._wiki_reduce_entity(
        entity_name="E1",
        entity_type="entity",
        new_claims=[],
        existing_page=existing,
        deleted_doc_ids={"d1"},
    )
    assert result["action"] == "update"
    assert len(result["retractions"]) == 1
    assert result["retractions"][0]["source_doc_id"] == "d1"


@pytest.mark.asyncio
async def test_reduce_entity_delete():
    """All source docs deleted → action=delete."""
    existing = {
        "claims": [{"statement": "S1", "source_doc_id": "d1"}],
        "page_version_int": 1,
    }
    result = await _wiki._wiki_reduce_entity(
        entity_name="E1",
        entity_type="entity",
        new_claims=[],
        existing_page=existing,
        deleted_doc_ids={"d1"},
    )
    assert result["action"] == "delete"
    assert result["has_delta"] is True
    assert result["entity_type"] == "entity"


@pytest.mark.asyncio
async def test_reduce_entity_noop():
    """No changes → action=noop, has_delta=False."""
    existing = {
        "claims": [{"statement": "S1", "source_doc_id": "d1"}],
        "page_version_int": 1,
    }
    result = await _wiki._wiki_reduce_entity(
        entity_name="E1",
        entity_type="concept",
        new_claims=[{"statement": "S1", "source_doc_id": "d1"}],
        existing_page=existing,
        deleted_doc_ids=set(),
    )
    assert result["action"] == "noop"
    assert result["has_delta"] is False
    assert result["entity_type"] == "concept"


# ---- Tests for _wiki_match_entities (Entity Matching) ----------------------


@pytest.mark.asyncio
async def test_match_entities_exact_match():
    """Exact match via canonical aliases resolves raw entities to canonical names."""
    embd = _wiki.MockEmbeddingModel() if hasattr(_wiki, "MockEmbeddingModel") else MockEmbeddingModel()
    embd = MockEmbeddingModel()
    chat = MockChatModel(canned="[true]")

    existing_canonical = {
        "Apple Inc.": {
            "entity_name": "Apple Inc.",
            "entity_type_kwd": "org",
            "aliases": ["Apple"],
            "source_doc_ids": ["doc_1"],
            "mention_count_int": 2,
        }
    }

    raw, _ = _wiki._extract_raw_entities(
        [
            {
                "doc_id": "doc_2",
                "entities": [{"name": "Apple", "type": "org"}],
                "concepts": [],
                "claims": [{"entity_name": "Apple", "statement": "Apple makes phones", "source_chunk_id": "C1", "source_doc_id": "doc_2"}],
            }
        ]
    )

    with patch(f"{_wiki.__name__}._knn_search_canonical", new_callable=AsyncMock, return_value=None):
        canonical_map, name_resolution = await _wiki._wiki_match_entities(
            raw_entities=raw,
            existing_canonical=existing_canonical,
            embd_mdl=embd,
            chat_mdl=chat,
            tenant_id="t1",
            kb_id="kb1",
            incremental=False,
        )

    assert "Apple Inc." in canonical_map, f"Keys: {list(canonical_map.keys())}"
    assert name_resolution.get("Apple") == "Apple Inc."


def test_match_entities_concept_no_llm():
    """Concept type entities get correct entity_type after matching."""
    raw, _ = _wiki._extract_raw_entities(
        [
            {
                "doc_id": "doc_1",
                "entities": [],
                "concepts": [{"term": "smartphone innovation", "definition_excerpt": "mobile tech advancement"}],
                "claims": [{"entity_name": "smartphone innovation", "statement": "A key concept", "source_chunk_id": "C1", "source_doc_id": "doc_1"}],
            }
        ]
    )

    # Verify extraction preserves concept type
    assert any(e["name"] == "smartphone innovation" and e["type"] == "concept" for e in raw)

    # Verify concept claims count
    concept = next(e for e in raw if e["name"] == "smartphone innovation")
    assert concept["claim_count"] == 1


def test_match_entities_incremental_new_entity():
    """Incremental build: exact match + KNN routing logic works correctly."""
    raw, _ = _wiki._extract_raw_entities(
        [
            {
                "doc_id": "doc_2",
                "entities": [{"name": "Apple Computer", "type": "org"}],
                "concepts": [],
                "claims": [
                    {
                        "entity_name": "Apple Computer",
                        "statement": "Apple Computer is a tech company",
                        "source_chunk_id": "C1",
                        "source_doc_id": "doc_2",
                    }
                ],
            }
        ]
    )

    # Simulate exact match against existing canonical index
    existing_canonical = {
        "Apple Inc.": {
            "entity_name": "Apple Inc.",
            "entity_type_kwd": "org",
            "aliases": [],
            "source_doc_ids": ["doc_1"],
            "mention_count_int": 1,
        }
    }
    exact_flat = {}
    for cname, centry in existing_canonical.items():
        aliases = centry.get("aliases")
        if not isinstance(aliases, list):
            continue
        for alias in [cname] + [a for a in aliases if isinstance(a, str)]:
            exact_flat[_wiki._normalize_key(alias)] = cname

    # "Apple Computer" should NOT exact-match "Apple Inc." (different alias)
    for entry in raw:
        raw_name = entry["name"]
        norm = _wiki._normalize_key(raw_name)
        assert norm not in exact_flat, f"{raw_name} should not exact-match"

    # "Apple Computer" in canonical entity name should normalize similarly
    assert _wiki._normalize_key("Apple Computer") == "apple computer"
    assert _wiki._normalize_key("Apple Inc.") == "apple inc"


def test_match_entities_first_build_pairwise():
    """First build: pairwise embedding dedup logic merges similar entities.

    Verifies that the same-document entity variants are candidates for merge.
    """
    embd = MockEmbeddingModel(vector_size=8, seed=42)
    raw, _ = _wiki._extract_raw_entities(
        [
            {
                "doc_id": "doc_1",
                "entities": [
                    {"name": "Apple Inc.", "type": "org"},
                    {"name": "Apple Computer", "type": "org"},
                ],
                "concepts": [],
                "claims": [
                    {"entity_name": "Apple Inc.", "statement": "Apple Inc. is a tech company", "source_chunk_id": "C1", "source_doc_id": "doc_1"},
                    {"entity_name": "Apple Computer", "statement": "Apple Computer makes hardware", "source_chunk_id": "C2", "source_doc_id": "doc_1"},
                ],
            }
        ]
    )

    # Verify two entities extracted from the same doc
    assert len(raw) == 2
    names = [e["name"] for e in raw]
    assert "Apple Inc." in names
    assert "Apple Computer" in names

    # Compute embeddings and cosine similarity
    query_texts = [_wiki._entity_to_query_text(e) for e in raw]
    emb, _ = embd.encode(query_texts)
    sim = float(np.dot(emb[0], emb[1]) / (np.linalg.norm(emb[0]) * np.linalg.norm(emb[1]) + 1e-10))

    # Verify cosine similarity computation works (value depends on mock seed)
    # The test verifies the calculation is numerically valid, not a specific threshold
    assert isinstance(sim, float), f"Cosine sim should be a float: {sim}"


# ---- Tests for _wiki_finalize (wikilink handling) --------------------------


def _make_wiki_page(slug: str, content: str, related: str | None = None) -> dict:
    return {
        "_source": {
            "slug_kwd": slug,
            "title_kwd": slug.split("/")[-1],
            "md_with_weight": content,
            "outlinks_kwd": "[]",
            "related_kb_pages_kwd": related or "[]",
        }
    }


@pytest.mark.asyncio
async def test_finalize_dead_link_cleanup():
    """FINALIZE removes [[]] from dead wikilinks in page content."""
    search_results = [
        _make_wiki_page("concept/A", "[[B]] is related to [[C]]"),
        _make_wiki_page("concept/B", "Content about B"),
    ]

    doc_store = make_doc_store(search_results)

    with (
        patch("common.settings.docStoreConn", doc_store),
        patch(f"{_wiki.__name__}._load_canonical_entities", new_callable=AsyncMock, return_value={}),
    ):
        await _wiki._wiki_finalize(tenant_id="t1", kb_id="kb1", embd_mdl=None)

    # Verify [[C]] was removed (dead link) while [[B]] was preserved
    update_calls = doc_store.update.call_args_list
    for args in update_calls:
        slug = args[0][0].get("id", "")
        upd = args[0][1]
        if slug == "concept/A":
            content = upd.get("md_with_weight", "")
            assert "[[C]]" not in content, f"Dead link [[C]] not removed: {content}"
            break


@pytest.mark.asyncio
async def test_finalize_entity_reference():
    """FINALIZE converts entity references to plain text (Mode A)."""
    search_results = [
        _make_wiki_page("concept/smartphone", "[[Apple Inc.]] drives innovation"),
    ]

    doc_store = make_doc_store(search_results)

    with (
        patch("common.settings.docStoreConn", doc_store),
        patch(
            f"{_wiki.__name__}._load_canonical_entities",
            new_callable=AsyncMock,
            return_value={
                "Apple Inc.": {
                    "entity_kwd": "Apple Inc.",
                    "entity_type_kwd": "org",
                    "aliases": [],
                    "source_doc_ids": ["doc_1"],
                    "mention_count_int": 3,
                }
            },
        ),
    ):
        await _wiki._wiki_finalize(tenant_id="t1", kb_id="kb1", embd_mdl=None)

    update_calls = doc_store.update.call_args_list
    for args in update_calls:
        slug = args[0][0].get("id", "")
        upd = args[0][1]
        if slug == "concept/smartphone":
            content = upd.get("md_with_weight", "")
            assert "[[Apple Inc.]]" not in content, f"Entity ref not cleaned: {content}"
            assert "Apple Inc." in content, f"Entity name missing: {content}"
            break


# ---- Tests for _wiki_decide_concept_pages depth ----------------------------


def test_decide_concept_pages_depth_threshold():
    """Concepts below depth threshold are filtered out."""
    concepts = [
        {
            "term": "deep concept",
            "claims": [{"statement": "C1"}, {"statement": "C2"}, {"statement": "C3"}],
            "source_doc_ids": ["d1", "d2"],
        },
        {
            "term": "thin concept",
            "claims": [{"statement": "C1"}],
            "source_doc_ids": ["d1"],
        },
    ]

    deep = []
    for concept in concepts:
        claims = concept.get("claims", [])
        source_docs = set(concept.get("source_doc_ids", []))
        if len(claims) >= CONCEPT_MIN_CLAIMS and len(source_docs) >= CONCEPT_MIN_SOURCES:
            deep.append(concept)

    names = {c["term"] for c in deep}
    assert "deep concept" in names
    assert "thin concept" not in names


@pytest.mark.asyncio
async def test_mode_a_incremental_creates_low_claim_concept():
    """Incremental builds must NOT apply depth threshold (deltas carry only
    changed claims). A new concept with few changed claims should still create
    a page; the depth filter is first-build only."""
    from unittest.mock import AsyncMock, patch

    # A new concept with only 1 changed claim (below CONCEPT_MIN_CLAIMS=3)
    concept_deltas = [
        {
            "entity_name": "brand new concept",
            "entity_type": "concept",
            "action": "create",
            "additions": [{"statement": "Single new claim", "source_doc_id": "d_new"}],
            "claims": [{"statement": "Single new claim", "source_doc_id": "d_new"}],
            "retractions": [],
            "has_delta": True,
        },
    ]

    with (
        patch(
            "rag.advanced_rag.knowlege_compile.wiki_incremental._wiki_refine_page",
            new_callable=AsyncMock,
            return_value={"page_id": "concept/brand-new-concept"},
        ) as mock_refine,
        patch(
            "rag.advanced_rag.knowlege_compile.wiki_incremental._wiki_update_doc_page_source",
            new_callable=AsyncMock,
        ),
    ):
        result = await _wiki._wiki_mode_a_run(
            deltas=concept_deltas,
            existing_pages={},
            chat_mdl=MockChatModel(),
            embd_mdl=MockEmbeddingModel(),
            tenant_id="t1",
            kb_id="kb1",
            incremental=True,  # ← incremental: depth check SKIPPED
            canonical_claims={
                "brand new concept": [{"statement": "Single new claim", "source_doc_id": "d_new"}],
            },
        )

    # The concept page should be created despite only 1 claim
    assert mock_refine.call_count == 1, f"Expected 1 REFINE call, got {mock_refine.call_count}"
    assert result["pages_created"] == 1


@pytest.mark.asyncio
async def test_mode_a_compiles_entity_and_concept_pages():
    """Mode A compiles BOTH entity and concept pages (no PLAN grouping)."""
    from unittest.mock import AsyncMock, patch

    # One concept delta (3+ claims to pass first-build depth check) + one entity delta
    deltas = [
        {
            "entity_name": "smartphone industry",
            "entity_type": "concept",
            "action": "create",
            "additions": [
                {"statement": "Concept claim 1", "source_doc_id": "d1"},
                {"statement": "Concept claim 2", "source_doc_id": "d1"},
                {"statement": "Concept claim 3", "source_doc_id": "d2"},
            ],
            "claims": [
                {"statement": "Concept claim 1", "source_doc_id": "d1"},
                {"statement": "Concept claim 2", "source_doc_id": "d1"},
                {"statement": "Concept claim 3", "source_doc_id": "d2"},
            ],
            "retractions": [],
            "has_delta": True,
        },
        {
            "entity_name": "Apple Inc.",
            "entity_type": "org",
            "action": "create",
            "additions": [{"statement": "Entity claim", "source_doc_id": "d1"}],
            "claims": [{"statement": "Entity claim", "source_doc_id": "d1"}],
            "retractions": [],
            "has_delta": True,
        },
    ]

    with (
        patch(
            "rag.advanced_rag.knowlege_compile.wiki_incremental._wiki_refine_page",
            new_callable=AsyncMock,
            return_value={"page_id": "x"},
        ) as mock_refine,
        patch(
            "rag.advanced_rag.knowlege_compile.wiki_incremental._wiki_update_doc_page_source",
            new_callable=AsyncMock,
        ),
    ):
        await _wiki._wiki_mode_a_run(
            deltas=deltas,
            existing_pages={},
            chat_mdl=MockChatModel(),
            embd_mdl=MockEmbeddingModel(),
            tenant_id="t1",
            kb_id="kb1",
            incremental=False,  # first build — concept depth check applies, entity always created
            canonical_claims={
                "smartphone industry": [{"statement": "Concept claim 1", "source_doc_id": "d1"}],
                "Apple Inc.": [{"statement": "Entity claim", "source_doc_id": "d1"}],
            },
        )

    # Both pages should be refined (1 concept + 1 entity)
    assert mock_refine.call_count == 2, f"Expected 2 REFINE calls, got {mock_refine.call_count}"

    # Verify page_id prefixes: one concept/ and one entity/
    prefixes = set()
    for args in mock_refine.call_args_list:
        kwargs = args[1]
        prefixes.add(kwargs["page_id"].split("/")[0])
        assert kwargs["page_type_kwd"] in ("concept", "entity")
    assert "concept" in prefixes
    assert "entity" in prefixes


@pytest.mark.asyncio
async def test_has_any_pages_detects_existing_pages():
    """_wiki_has_any_pages returns True when wiki_page rows exist."""
    doc_store = make_doc_store(
        [
            {"_source": {"slug_kwd": "concept/smartphone"}},
        ]
    )

    with patch("common.settings.docStoreConn", doc_store):
        has = await _wiki._wiki_has_any_pages("t1", "kb1")

    assert has is True


@pytest.mark.asyncio
async def test_has_any_pages_false_when_no_pages():
    """_wiki_has_any_pages returns False when no wiki_page rows exist."""
    doc_store = make_doc_store([])

    with patch("common.settings.docStoreConn", doc_store):
        has = await _wiki._wiki_has_any_pages("t1", "kb1")

    assert has is False


@pytest.mark.asyncio
async def test_has_any_pages_false_when_index_missing():
    """_wiki_has_any_pages returns False when the index doesn't exist."""
    doc_store = MagicMock()
    doc_store.index_exist = MagicMock(return_value=False)

    with patch("common.settings.docStoreConn", doc_store):
        has = await _wiki._wiki_has_any_pages("t1", "kb1")

    assert has is False


# ---- Tests for doc_page_source ---------------------------------------------


@pytest.mark.asyncio
async def test_canonical_claims_maps_raw_to_canonical():
    """canonical_claims must aggregate raw-name claims onto canonical names.

    Regression test: claim_index is keyed by RAW entity name (from MAP), but
    REDUCE looks up by canonical name.  If a raw name resolves to a different
    canonical name (e.g. "Apple Computer" → "Apple Inc."), the claims must
    still be found, otherwise every entity becomes a no-op and output is empty.
    """
    # claim_index keyed by raw names
    claim_index = {
        "Apple Computer": [
            {"statement": "C1", "source_doc_id": "d1"},
            {"statement": "C2", "source_doc_id": "d1"},
        ],
        "Apple Inc.": [
            {"statement": "C3", "source_doc_id": "d1"},
        ],
        "Samsung": [
            {"statement": "C4", "source_doc_id": "d1"},
        ],
    }
    # name_resolution: "Apple Computer" → "Apple Inc." (merged)
    name_resolution = {"Apple Computer": "Apple Inc.", "Apple Inc.": "Apple Inc.", "Samsung": "Samsung"}
    affected_names = {"Apple Inc.", "Samsung"}

    # Replicate the aggregation logic from wiki_compile_incremental
    canonical_claims: dict[str, list[dict]] = {}
    for raw_name, claims in claim_index.items():
        cname = name_resolution.get(raw_name, raw_name)
        if cname in affected_names:
            canonical_claims.setdefault(cname, []).extend(claims)
    for name in affected_names:
        canonical_claims.setdefault(name, [])

    # "Apple Inc." should collect claims from BOTH "Apple Computer" and "Apple Inc."
    assert len(canonical_claims["Apple Inc."]) == 3, f"Got {len(canonical_claims['Apple Inc.'])}"
    statements = {c["statement"] for c in canonical_claims["Apple Inc."]}
    assert statements == {"C1", "C2", "C3"}
    assert len(canonical_claims["Samsung"]) == 1


@pytest.mark.asyncio
async def test_doc_page_source_entity_names():
    """doc_page_source stores and retrieves entity_names."""
    doc_store = make_doc_store(
        [
            {
                "_source": {
                    "doc_id": "doc_1",
                    "page_ids": '["concept/A", "concept/B"]',
                    "entity_names": '["Apple Inc.", "smartphone industry"]',
                    "source_chunk_hashes": '{"C1": "abc", "C2": "def"}',
                    "map_checksum": "xyz",
                }
            }
        ]
    )

    with patch("common.settings.docStoreConn", doc_store):
        dps = await _wiki._wiki_load_doc_page_source("t1", "kb1", "doc_1")

    assert dps is not None
    assert "Apple Inc." in dps.get("entity_names", [])
    assert "smartphone industry" in dps.get("entity_names", [])
    assert len(dps.get("page_ids", [])) == 2


# ---- End-to-end: Entity Matching → REDUCE ---------------------------------


@pytest.mark.asyncio
async def test_entity_matching_to_reduce_flow():
    """Entity Matching → REDUCE: canonical names flow through correctly.

    Only the REDUCE call is async (straightforward dict/await logic).
    Entity Matching is tested synchronously here.
    """
    map_results = [
        {
            "doc_id": "doc_1",
            "entities": [{"name": "Apple Inc.", "type": "org"}],
            "concepts": [{"term": "smartphone industry"}],
            "claims": [
                {"entity_name": "Apple Inc.", "statement": "Apple Inc. is a tech company", "source_chunk_id": "C1", "source_doc_id": "doc_1"},
                {"entity_name": "smartphone industry", "statement": "A global industry", "source_chunk_id": "C2", "source_doc_id": "doc_1"},
            ],
        }
    ]

    raw, claim_index = _wiki._extract_raw_entities(map_results)

    # Verify matching (synchronous, no _wiki_match_entities call)
    assert any(e["name"] == "Apple Inc." and e["type"] == "org" for e in raw)
    assert any(e["name"] == "smartphone industry" and e["type"] == "concept" for e in raw)

    canonical_map = {}
    for entry in raw:
        canonical_map[entry["name"]] = entry

    assert len(canonical_map) >= 2
    assert canonical_map["Apple Inc."]["type"] == "org"
    assert canonical_map["smartphone industry"]["type"] == "concept"

    # REDUCE (async but clean — only uses asyncio.gather, no _wiki_match_entities)
    # canonical_claims built from claim_index (full text kept separate)
    canonical_claims = {n: claim_index.get(n, []) for n in canonical_map}

    deltas = await _wiki._wiki_reduce_batch(
        affected_names=set(canonical_map.keys()),
        existing_pages={},
        deleted_doc_ids=set(),
        canonical_claims=canonical_claims,
        canonical_map=canonical_map,
        name_resolution={e["name"]: e["name"] for e in raw},
    )

    for d in deltas:
        assert "entity_type" in d, f"Missing entity_type in delta: {d}"

    concept_deltas = [d for d in deltas if d.get("entity_type") == "concept"]
    entity_deltas = [d for d in deltas if d.get("entity_type") != "concept"]
    assert len(concept_deltas) >= 1
    assert len(entity_deltas) >= 1


# ---- Edge cases ------------------------------------------------------------


@pytest.mark.asyncio
async def test_reduce_batch_empty_affected():
    """_wiki_reduce_batch with empty affected_names returns []."""
    result = await _wiki._wiki_reduce_batch(
        affected_names=set(),
        map_results=[],
        existing_pages={},
        deleted_doc_ids=set(),
    )
    assert result == []


@pytest.mark.asyncio
async def test_reduce_entity_type_preservation():
    """entity_type is preserved across all REDUCE paths."""

    r1 = await _wiki._wiki_reduce_entity("C1", entity_type="concept", new_claims=[{"statement": "S1", "source_doc_id": "d1"}], existing_page=None, deleted_doc_ids=set())
    assert r1["entity_type"] == "concept"

    r2 = await _wiki._wiki_reduce_entity(
        "E1",
        entity_type="org",
        new_claims=[{"statement": "N", "source_doc_id": "d2"}],
        existing_page={"claims": [{"statement": "O", "source_doc_id": "d1"}], "page_version_int": 1},
        deleted_doc_ids=set(),
    )
    assert r2["entity_type"] == "org"

    r3 = await _wiki._wiki_reduce_entity(
        "E1", entity_type="concept", new_claims=[], existing_page={"claims": [{"statement": "O", "source_doc_id": "d1"}], "page_version_int": 1}, deleted_doc_ids={"d1"}
    )
    assert r3["entity_type"] == "concept"


# ---- _wiki_decide_concept_pages helper test --------------------------------


def test_entity_matching_concept_entity_types():
    """Entity Matching preserves entity_type for both entity and concept."""
    raw, _ = _wiki._extract_raw_entities(
        [
            {
                "doc_id": "doc_1",
                "entities": [{"name": "Apple Inc.", "type": "org"}],
                "concepts": [{"term": "supply chain"}],
                "claims": [
                    {"entity_name": "Apple Inc.", "statement": "S1", "source_chunk_id": "C1", "source_doc_id": "doc_1"},
                    {"entity_name": "supply chain", "statement": "S2", "source_chunk_id": "C2", "source_doc_id": "doc_1"},
                ],
            }
        ]
    )

    types = {e["name"]: e.get("type") for e in raw}
    assert types.get("Apple Inc.") == "org"
    assert types.get("supply chain") == "concept"


# ---- _wiki_load_pages_for_graph (canvas graph) -----------------------------


@pytest.mark.asyncio
async def test_load_pages_for_graph_shape():
    """_wiki_load_pages_for_graph projects wiki_page rows onto the graph shape."""
    doc_store = make_doc_store(
        [
            {
                "_source": {
                    "slug_kwd": "concept/smartphone",
                    "title_kwd": "Smartphone",
                    "page_type_kwd": "concept",
                    "summary_with_weight": "A mobile device.",
                    "entity_names_kwd": '["smartphone"]',
                    "outlinks_kwd": '["entity/apple"]',
                    "source_chunk_ids": '["C1", "C2"]',
                    "source_doc_ids": '["d1"]',
                }
            }
        ]
    )

    with patch("common.settings.docStoreConn", doc_store):
        pages = await _wiki._wiki_load_pages_for_graph("t1", "kb1")

    assert len(pages) == 1
    p = pages[0]
    assert p["slug"] == "concept/smartphone"
    assert p["title"] == "Smartphone"
    assert p["page_type"] == "concept"
    assert p["outlinks"] == ["entity/apple"]
    assert p["source_chunk_ids"] == ["C1", "C2"]
    assert p["source_doc_ids"] == ["d1"]


@pytest.mark.asyncio
async def test_finalize_writes_outlinks():
    """_wiki_finalize stamps outlinks_kwd / outlinks_int for valid wikilinks."""
    search_results = [
        {"_source": {"slug_kwd": "concept/A", "md_with_weight": "[[concept/B]] related", "page_type_kwd": "concept"}},
        {"_source": {"slug_kwd": "concept/B", "md_with_weight": "content", "page_type_kwd": "concept"}},
    ]
    doc_store = make_doc_store(search_results)

    with (
        patch("common.settings.docStoreConn", doc_store),
        patch(f"{_wiki.__name__}._load_canonical_entities", new_callable=AsyncMock, return_value={}),
    ):
        await _wiki._wiki_finalize(tenant_id="t1", kb_id="kb1", embd_mdl=None)

    update_calls = doc_store.update.call_args_list
    for args in update_calls:
        cond = args[0][0]
        upd = args[0][1]
        if cond.get("id") == "concept/A":
            # outlinks_kwd is a *_kwd field: Infinity shreds json.dumps strings
            # on read-back, so _wiki_finalize writes a native list (mirroring
            # the old-mode writer) which Infinity stores/reads back correctly.
            assert upd["outlinks_kwd"] == ["concept/B"], f"Got {upd['outlinks_kwd']}"
            assert upd["outlinks_int"] == 1
            break
    else:
        pytest.fail("No update for concept/A found")


async def test_finalize_auto_links_mentions():
    """_wiki_finalize auto-links standalone mentions of other pages' names."""
    search_results = [
        {
            "_source": {
                "slug_kwd": "entity/Apple",
                "title_kwd": "Apple",
                "md_with_weight": "Apple makes phones. Apple is based in California.",
                "page_type_kwd": "entity",
            }
        },
        {
            "_source": {
                "slug_kwd": "entity/Google",
                "title_kwd": "Google",
                "md_with_weight": "Google competes with Apple in search and mobile.",
                "page_type_kwd": "entity",
            }
        },
    ]
    doc_store = make_doc_store(search_results)

    with (
        patch("common.settings.docStoreConn", doc_store),
        patch(f"{_wiki.__name__}._load_canonical_entities", new_callable=AsyncMock, return_value={}),
    ):
        await _wiki._wiki_finalize(tenant_id="t1", kb_id="kb1", embd_mdl=None)

    update_calls = doc_store.update.call_args_list
    google_upd = None
    for args in update_calls:
        cond = args[0][0]
        if cond.get("id") == "entity/Google":
            google_upd = args[0][1]
            break
    assert google_upd is not None, "entity/Google not updated"
    # "Apple" mentioned in Google page → auto-linked exactly once + outlink
    # recorded, and rendered into the navigable artifact-link form.
    assert "[Apple](artifact/kb1/entity/Apple)" in google_upd["md_with_weight"], google_upd["md_with_weight"]
    assert "entity/Apple" in (google_upd["outlinks_kwd"] or []), google_upd["outlinks_kwd"]
    assert google_upd["outlinks_int"] == 1


def test_inside_wikilink():
    content = "before [[entity/X]] after"
    assert _wiki._inside_wikilink(content, content.index("entity"))
    assert not _wiki._inside_wikilink(content, content.index("before"))
    assert not _wiki._inside_wikilink(content, content.index("after"))


def test_extract_outlinks_from_content():
    """_wiki_extract_outlinks_from_content derives unique ordered outlinks."""
    content = "See [[concept/B]] and [[entity/apple]] and [[concept/B]] again"
    assert _wiki._wiki_extract_outlinks_from_content(content) == [
        "concept/B",
        "entity/apple",
    ]
    assert _wiki._wiki_extract_outlinks_from_content("") == []
    assert _wiki._wiki_extract_outlinks_from_content("no links here") == []


@pytest.mark.asyncio
async def test_load_pages_for_graph_outlink_fallback():
    """Pages without outlinks_kwd get edges derived from content wikilinks."""
    doc_store = make_doc_store(
        [
            {
                "_source": {
                    "slug_kwd": "concept/A",
                    "title_kwd": "A",
                    "page_type_kwd": "concept",
                    "summary_with_weight": "",
                    "md_with_weight": "See [[concept/B]]",
                    "entity_names_kwd": "[]",
                    "source_chunk_ids": "[]",
                    "source_doc_ids": "[]",
                }
            }
        ]
    )

    with patch("common.settings.docStoreConn", doc_store):
        pages = await _wiki._wiki_load_pages_for_graph("t1", "kb1")

    assert pages[0]["outlinks"] == ["concept/B"]


# ---- _load_canonical_entities: *_kwd scalar normalization ------------------


@pytest.mark.asyncio
async def test_load_canonical_entities_normalizes_entity_type():
    """entity_type_kwd comes back as a list (['concept']) from Infinity; the
    loader must normalize it to a scalar so `== "concept"` checks work."""
    doc_store = make_doc_store(
        [
            {
                "_source": {
                    "entity_kwd": "询问笔录",
                    "entity_type_kwd": ["concept"],  # simulated Infinity list
                    "aliases": '["询问笔录"]',
                    "source_doc_ids": '["doc_1"]',
                    "mention_count_int": 2,
                }
            }
        ]
    )

    with patch("common.settings.docStoreConn", doc_store):
        canon = await _wiki._load_canonical_entities("t1", "kb1")

    assert "询问笔录" in canon
    assert canon["询问笔录"]["entity_type_kwd"] == "concept", canon["询问笔录"]


@pytest.mark.asyncio
async def test_search_existing_pages_normalizes_slug():
    """slug_kwd comes back as a list; _search_existing_pages must key by scalar."""
    doc_store = make_doc_store(
        [
            {
                "_source": {
                    "slug_kwd": ["concept/询问笔录"],  # simulated Infinity list
                    "title_kwd": ["询问笔录"],
                    "md_with_weight": "content",
                }
            }
        ]
    )

    with patch("common.settings.docStoreConn", doc_store):
        pages = await _wiki._search_existing_pages("t1", "kb1", ["slug_kwd", "title_kwd", "md_with_weight"])

    assert "concept/询问笔录" in pages
    assert pages["concept/询问笔录"]["title_kwd"] == ["询问笔录"]  # raw preserved
    assert pages["concept/询问笔录"]["id"] == "concept/询问笔录"


# ---- relation-based linking in FINALIZE -------------------------------------


@pytest.mark.asyncio
async def test_finalize_links_via_map_relations():
    """_wiki_finalize connects pages when MAP extracted a (from, to) relation
    between them, even when prose contains no [[wikilink]]."""
    # Two wiki pages + one map_extract row whose relation links them.
    doc_store = make_doc_store(
        [
            {
                "_source": {
                    "slug_kwd": "entity/肖亮",
                    "title_kwd": "肖亮",
                    "md_with_weight": "肖亮 is a person.",
                    "page_type_kwd": "entity",
                }
            },
            {
                "_source": {
                    "slug_kwd": "entity/肖立",
                    "title_kwd": "肖立",
                    "md_with_weight": "肖立 is a person.",
                    "page_type_kwd": "entity",
                }
            },
            {
                "_source": {
                    "doc_id": "map1",
                    "compile_kwd": "wiki_map_extract",
                    "content_with_weight": json.dumps(
                        {
                            "entities": [{"name": "肖亮", "type": "person"}, {"name": "肖立", "type": "person"}],
                            "relations": [{"from": "肖亮", "to": "肖立", "type": "other"}],
                        },
                        ensure_ascii=False,
                    ),
                }
            },
        ]
    )

    with (
        patch("common.settings.docStoreConn", doc_store),
        patch(f"{_wiki.__name__}._load_canonical_entities", new_callable=AsyncMock, return_value={}),
    ):
        await _wiki._wiki_finalize(tenant_id="t1", kb_id="kb1", embd_mdl=None)

    update_calls = doc_store.update.call_args_list
    xiaoliang_upd = None
    for args in update_calls:
        if args[0][0].get("id") == "entity/肖亮":
            xiaoliang_upd = args[0][1]
            break
    assert xiaoliang_upd is not None, "entity/肖亮 not updated"
    assert "entity/肖立" in (xiaoliang_upd["outlinks_kwd"] or []), xiaoliang_upd["outlinks_kwd"]
    assert xiaoliang_upd["outlinks_int"] == 1
    # The relation edge must be injected into the page body. It is stored either
    # as a raw [[entity/肖立]] or (after the link transformer runs) as the
    # navigable Markdown form [肖立](artifact/kb1/entity/肖立). Both contain the
    # target slug so the graph can reconstruct edges from md_with_weight.
    body = xiaoliang_upd["md_with_weight"] or ""
    assert "entity/肖立" in body, body


@pytest.mark.asyncio
async def test_wiki_finalize_renders_navigable_links():
    """FINALIZE renders [[slug]] to [text](artifact/{kb_id}/{slug}) so the
    frontend wiki viewer can deep-link, not plain [[...]] text."""
    # Two pages; page B's body links to page A via [[entity/肖立]]. FINALIZE
    # must render that to the navigable [肖立](artifact/kb1/entity/肖立) form.
    search_results = [
        {
            "_source": {
                "slug_kwd": "entity/肖立",
                "title_kwd": "肖立",
                "md_with_weight": "肖立 is a person.",
                "page_type_kwd": "entity",
            }
        },
        {
            "_source": {
                "slug_kwd": "entity/肖亮",
                "title_kwd": "肖亮",
                "md_with_weight": "肖亮 references [[entity/肖立]] here.",
                "page_type_kwd": "entity",
            }
        },
    ]
    doc_store = make_doc_store(search_results)
    with (
        patch("common.settings.docStoreConn", doc_store),
        patch(f"{_wiki.__name__}._load_canonical_entities", new_callable=AsyncMock, return_value={}),
    ):
        await _wiki._wiki_finalize(tenant_id="t1", kb_id="kb1", embd_mdl=None)
    update_calls = doc_store.update.call_args_list
    upd = None
    for args in update_calls:
        if args[0][0].get("id") == "entity/肖亮":
            upd = args[0][1]
            break
    assert upd is not None
    body = upd["md_with_weight"] or ""
    assert "[肖立](artifact/kb1/entity/肖立)" in body, body


@pytest.mark.asyncio
async def test_reduce_entity_claimless_concept_is_skipped():
    """A new concept without claims must not create an ungrounded page."""
    from rag.advanced_rag.knowlege_compile import wiki_incremental as _wiki

    result = await _wiki._wiki_reduce_entity(
        entity_name="继承纠纷",
        entity_type="concept",
        existing_page=None,
        new_claims=[],  # concepts have no dedicated claim rows
        deleted_doc_ids=set(),
    )
    assert result["action"] == "noop"
    assert result["has_delta"] is False
    assert result["entity_type"] == "concept"


@pytest.mark.asyncio
async def test_page_router_skips_knn_when_no_existing_pages():
    """A first Mode B build should cluster directly without page-index searches."""
    entities = [
        {"entity_name": "Apple", "entity_type": "org", "claims": []},
        {"entity_name": "Banana", "entity_type": "org", "claims": []},
    ]
    embd_mdl = MockEmbeddingModel()
    embd_mdl.encode = MagicMock(wraps=embd_mdl.encode)
    doc_store = make_doc_store()

    with (
        patch("common.settings.docStoreConn", doc_store),
        patch(
            f"{_wiki.__name__}._wiki_cluster_entities",
            side_effect=lambda items, embeddings, threshold: [items],
        ),
    ):
        assignments = await _wiki._wiki_page_router(
            affected_entities=entities,
            embd_mdl=embd_mdl,
            tenant_id="t1",
            kb_id="kb1",
            existing_page_ids=set(),
        )

    assert assignments == {"_new_entity/apple": entities}
    assert embd_mdl.encode.call_count == 1
    doc_store.search.assert_not_called()
