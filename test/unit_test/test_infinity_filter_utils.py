from common.doc_store.infinity_filter_utils import build_fulltext_filter


def test_build_fulltext_filter_escapes_single_quotes():
    assert build_fulltext_filter("content", "maintainer's note") == (
        "filter_fulltext('content', 'maintainer''s note')"
    )


def test_build_fulltext_filter_preserves_plain_text():
    assert build_fulltext_filter("content", "plain text") == "filter_fulltext('content', 'plain text')"
