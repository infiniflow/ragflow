#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""Unit tests for rag.nlp.search.

The primary focus is regression coverage for the ``tag_feas`` RCE fix. The
``TAG_FLD`` value reaching ``_rank_feature_scores`` is ultimately attacker-
controllable via the chunk create/update API and at one point was passed to
``eval()``. These tests pin the replacement ``_extract_tag_feas`` helper to
its defensive contract so a future refactor can't re-introduce a code-eval
sink silently.
"""

import json

import numpy as np
import pytest

from common.constants import PAGERANK_FLD, TAG_FLD
from rag.nlp.search import Dealer

# Critical-priority: these tests guard against re-introducing an RCE sink.
# Matches the precedent set by test/unit_test/rag/prompts/test_generator_sandbox.py,
# which is the closest analogue (also a security-sanitizer regression suite).
pytestmark = pytest.mark.p1


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _mk_fields(**chunks):
    """Build a `search_res.field`-shaped mapping: {chunk_id: {col: value, ...}}."""
    return dict(chunks)


def _mk_search_res(field):
    """Build a minimal SearchResult the scorer will accept."""
    return Dealer.SearchResult(total=len(field), ids=list(field.keys()), field=field)


def _bare_dealer():
    """Dealer instance without running __init__ (avoids pulling in the tokenizer)."""
    return object.__new__(Dealer)


# ---------------------------------------------------------------------------
# _extract_tag_feas — sanitization contract
# ---------------------------------------------------------------------------

class TestExtractTagFeas:
    """The helper must return a sanitized ``{str: number}`` dict or ``None``,
    and must NEVER evaluate attacker-supplied content as code."""

    def test_dict_passthrough(self):
        fields = _mk_fields(c1={TAG_FLD: {"sec": 10, "perf": 2.5}})
        assert Dealer._extract_tag_feas("c1", fields) == {"sec": 10, "perf": 2.5}

    def test_dict_filters_non_numeric_values(self):
        fields = _mk_fields(c1={TAG_FLD: {"ok": 1, "bad": "x", "also_bad": [1, 2]}})
        assert Dealer._extract_tag_feas("c1", fields) == {"ok": 1}

    def test_bool_values_rejected(self):
        # bool is a subclass of int; we explicitly exclude True/False so a
        # poisoned score can't pose as 1/0.
        fields = _mk_fields(c1={TAG_FLD: {"flag_t": True, "flag_f": False, "real": 3}})
        assert Dealer._extract_tag_feas("c1", fields) == {"real": 3}

    def test_json_string_of_dict_parsed(self):
        # OceanBase path returns the JSON column as a raw string.
        fields = _mk_fields(c1={TAG_FLD: json.dumps({"sec": 20})})
        assert Dealer._extract_tag_feas("c1", fields) == {"sec": 20}

    def test_empty_string_returns_none(self):
        fields = _mk_fields(c1={TAG_FLD: ""})
        assert Dealer._extract_tag_feas("c1", fields) is None

    def test_missing_field_returns_none(self):
        fields = _mk_fields(c1={})
        assert Dealer._extract_tag_feas("c1", fields) is None

    def test_empty_dict_returns_empty_dict(self):
        # "{}" is well-formed and semantically means "no tags" — caller treats
        # {} the same as None for the zero-contribution outcome.
        fields = _mk_fields(c1={TAG_FLD: "{}"})
        assert Dealer._extract_tag_feas("c1", fields) == {}

    def test_malformed_json_returns_none(self, caplog):
        fields = _mk_fields(c1={TAG_FLD: "{not json"})
        with caplog.at_level("WARNING"):
            assert Dealer._extract_tag_feas("c1", fields) is None
        assert any("Malformed" in rec.message for rec in caplog.records)

    @pytest.mark.parametrize("payload", [
        json.dumps([1, 2, 3]),     # JSON list
        json.dumps(42),            # JSON number
        json.dumps("hello"),       # JSON string (not a dict)
    ])
    def test_non_dict_json_returns_none(self, payload, caplog):
        fields = _mk_fields(c1={TAG_FLD: payload})
        with caplog.at_level("WARNING"):
            assert Dealer._extract_tag_feas("c1", fields) is None
        assert any("Unexpected" in rec.message for rec in caplog.records)

    def test_non_string_non_dict_returns_none(self, caplog):
        # e.g. a number or list somehow survived the backend read path.
        fields = _mk_fields(c1={TAG_FLD: 42})
        with caplog.at_level("WARNING"):
            assert Dealer._extract_tag_feas("c1", fields) is None

    # ------------------------------------------------------------------
    # Security regressions — these are the tests that pin the fix.
    # ------------------------------------------------------------------

    def test_rce_payload_via_infinity_roundtrip_not_executed(self):
        """Regression for the eval() RCE.

        On the Infinity write path, ``_feas`` values are stored via
        ``json.dumps(v)`` with no type check. An attacker who supplies a bare
        string (not a dict) in the chunk create/update API gets that string
        stored verbatim; on read it round-trips back through ``json.loads`` as
        a ``str``. Historically this string was fed to ``eval()``.

        The helper MUST reject it and MUST NOT evaluate it.
        """
        # If eval() ever runs this, the test will crash loudly by raising.
        payload = 'raise RuntimeError("RCE: eval executed attacker payload")'
        round_tripped = json.loads(json.dumps(payload))
        assert isinstance(round_tripped, str)

        fields = _mk_fields(c1={TAG_FLD: round_tripped})
        assert Dealer._extract_tag_feas("c1", fields) is None

    def test_single_quoted_repr_rejected(self):
        """Previously, ``es_conn.get_fields`` / ``opensearch_conn.get_fields``
        returned ``tag_feas`` as a stringified Python dict (e.g.
        ``"{'sec': 10}"``) — the product of ``str(dict)`` on a native dict
        from ``_source``. That representation is not valid JSON and could
        only be converted back with a dangerous ``eval()`` call.

        The connectors now pass ``tag_feas`` dicts through unchanged, and
        the sanitizer's invariant is that any string-shaped input must be
        parseable as JSON. Single-quoted Python repr is rejected by design
        — the sanitizer deliberately does not fall back to ``eval()`` or
        ``ast.literal_eval()``, since ``tag_feas`` is attacker-reachable
        via the chunk create/update API and any code-evaluating parser
        would restore the RCE this fix closes.
        """
        repr_str = "{'sec': 10}"  # Python repr, not JSON
        fields = _mk_fields(c1={TAG_FLD: repr_str})
        assert Dealer._extract_tag_feas("c1", fields) is None


# ---------------------------------------------------------------------------
# _rank_feature_scores — end-to-end sanity
# ---------------------------------------------------------------------------

class TestRankFeatureScores:
    """Validate the scorer wires ``_extract_tag_feas`` + pagerank correctly."""

    def test_empty_query_returns_pageranks_only(self):
        fields = _mk_fields(
            c1={PAGERANK_FLD: 2, TAG_FLD: {"sec": 5}},
            c2={PAGERANK_FLD: 7, TAG_FLD: {"sec": 9}},
        )
        out = _bare_dealer()._rank_feature_scores({}, _mk_search_res(fields))
        np.testing.assert_array_equal(out, np.array([2.0, 7.0]))

    def test_matching_tag_contributes_score(self):
        # One chunk tagged "sec"=1 vs query {"sec": 1}: cosine-ish score
        # simplifies to 1.0, then the code multiplies by 10 and adds pagerank.
        fields = _mk_fields(c1={PAGERANK_FLD: 0, TAG_FLD: {"sec": 1}})
        out = _bare_dealer()._rank_feature_scores({"sec": 1}, _mk_search_res(fields))
        assert out[0] == pytest.approx(10.0)

    def test_non_matching_tag_scores_zero_plus_pagerank(self):
        fields = _mk_fields(c1={PAGERANK_FLD: 3, TAG_FLD: {"other": 1}})
        out = _bare_dealer()._rank_feature_scores({"sec": 1}, _mk_search_res(fields))
        # no overlap with query → nor=0 → rank_fea=0, plus pagerank=3.
        assert out[0] == pytest.approx(3.0)

    def test_malformed_tag_feas_does_not_break_scorer(self):
        """A poisoned chunk degrades its own score to 0 and leaves siblings
        untouched — the whole search must not fail because of one bad row."""
        fields = _mk_fields(
            c_poisoned={PAGERANK_FLD: 0, TAG_FLD: "__import__('os').system('x')"},
            c_good={PAGERANK_FLD: 0, TAG_FLD: {"sec": 1}},
        )
        out = _bare_dealer()._rank_feature_scores({"sec": 1}, _mk_search_res(fields))
        assert out[0] == pytest.approx(0.0)   # poisoned chunk: no contribution
        assert out[1] == pytest.approx(10.0)  # healthy chunk: scored normally

    def test_json_string_tag_feas_scored_same_as_dict(self):
        """Dict input and JSON-string input should produce identical scores —
        this protects against a future backend change flipping the wire type.
        """
        fields_dict = _mk_fields(c1={PAGERANK_FLD: 0, TAG_FLD: {"sec": 2, "perf": 1}})
        fields_str = _mk_fields(c1={PAGERANK_FLD: 0, TAG_FLD: json.dumps({"sec": 2, "perf": 1})})
        q = {"sec": 2, "perf": 1}
        a = _bare_dealer()._rank_feature_scores(q, _mk_search_res(fields_dict))
        b = _bare_dealer()._rank_feature_scores(q, _mk_search_res(fields_str))
        np.testing.assert_allclose(a, b)
