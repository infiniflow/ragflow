import sys
import types

import numpy as np
import pytest

from common.constants import PAGERANK_FLD, TAG_FLD


class _DummyTokenizer:
    def tag(self, *args, **kwargs):
        return []

    def freq(self, *args, **kwargs):
        return 0

    def _tradi2simp(self, text):
        return text

    def _strQ2B(self, text):
        return text


fake_infinity = types.ModuleType("infinity")
fake_infinity_tokenizer = types.ModuleType("infinity.rag_tokenizer")
fake_infinity_tokenizer.RagTokenizer = _DummyTokenizer
fake_infinity_tokenizer.is_chinese = lambda text: False
fake_infinity_tokenizer.is_number = lambda text: False
fake_infinity_tokenizer.is_alphabet = lambda text: True
fake_infinity_tokenizer.naive_qie = lambda text: text.split()
fake_infinity.rag_tokenizer = fake_infinity_tokenizer
sys.modules.setdefault("infinity", fake_infinity)
sys.modules.setdefault("infinity.rag_tokenizer", fake_infinity_tokenizer)

fake_query = types.ModuleType("rag.nlp.query")


class _DummyFulltextQueryer:
    pass


fake_query.FulltextQueryer = _DummyFulltextQueryer
sys.modules.setdefault("rag.nlp.query", fake_query)

fake_settings = types.ModuleType("common.settings")
sys.modules.setdefault("common.settings", fake_settings)

from rag.nlp.search import Dealer


def _make_search_res(tag_feas):
    return Dealer.SearchResult(
        total=1,
        ids=["c1"],
        field={"c1": {TAG_FLD: tag_feas, PAGERANK_FLD: 0}},
    )


def test_rank_feature_scores_parses_python_dict_string():
    dealer = Dealer.__new__(Dealer)
    sres = _make_search_res("{'apple': 2.0}")
    scores = dealer._rank_feature_scores({"apple": 1.0}, sres)
    assert np.isclose(scores[0], 10.0)


def test_rank_feature_scores_parses_json_string():
    dealer = Dealer.__new__(Dealer)
    sres = _make_search_res('{"apple": 2.0}')
    scores = dealer._rank_feature_scores({"apple": 1.0}, sres)
    assert np.isclose(scores[0], 10.0)


def test_rank_feature_scores_handles_dict_value():
    dealer = Dealer.__new__(Dealer)
    sres = _make_search_res({"apple": 2.0})
    scores = dealer._rank_feature_scores({"apple": 1.0}, sres)
    assert np.isclose(scores[0], 10.0)


def test_rank_feature_scores_ignores_invalid_tag_feas_string():
    dealer = Dealer.__new__(Dealer)
    sres = _make_search_res("not a dict")
    scores = dealer._rank_feature_scores({"apple": 1.0}, sres)
    assert np.isclose(scores[0], 0.0)


def test_rank_feature_scores_ignores_executable_tag_feas_string():
    dealer = Dealer.__new__(Dealer)
    sres = _make_search_res('{"apple": (__import__("time").sleep(1) or 1.0)}')
    scores = dealer._rank_feature_scores({"apple": 1.0}, sres)
    assert np.isclose(scores[0], 0.0)


def test_rank_feature_scores_returns_pagerank_when_no_tag_feature():
    dealer = Dealer.__new__(Dealer)
    sres = _make_search_res("{'apple': 2.0}")
    scores = dealer._rank_feature_scores({PAGERANK_FLD: 10}, sres)
    assert np.isclose(scores[0], 0.0)


@pytest.mark.p1
def test_calc_rerank_limit_keeps_reranked_windows_page_aligned():
    for page_size, expected in [(10, 60), (30, 60), (31, 62), (64, 64)]:
        limit = Dealer._rerank_window(page_size, top=1024)
        assert limit == expected
        assert limit <= 64
        assert limit % page_size == 0


@pytest.mark.p1
def test_calc_rerank_limit_avoids_short_block_boundary_pages():
    page_size = 10
    limit = Dealer._rerank_window(page_size, top=1024)
    global_offset = 6 * page_size
    begin = global_offset % limit
    end = begin + page_size

    assert begin == 0
    assert end <= limit


@pytest.mark.p2
def test_calc_rerank_limit_preserves_existing_non_rerank_and_small_top_behavior():
    assert Dealer._rerank_window(page_size=10) == 70
    assert Dealer._rerank_window(page_size=10, top=20) == 20
    assert Dealer._rerank_window(page_size=30, top=50) == 50
    with pytest.raises(ValueError, match=r"_rerank_window.*page_size=100.*top=1024"):
        Dealer._rerank_window(page_size=100, top=1024)
