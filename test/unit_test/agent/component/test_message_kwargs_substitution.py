import pytest

from agent.component.message import Message


@pytest.mark.p1
def test_apply_kwargs_substitutes_value_with_regex_escapes():
    # A value containing backslash-digit sequences (e.g. a Windows path) must be
    # inserted verbatim, not interpreted as a regex replacement backreference.
    content = Message._apply_kwargs("path is _v end", {"_v": r"C:\10\report"})
    assert content == r"path is C:\10\report end"


@pytest.mark.p1
def test_apply_kwargs_handles_token_with_regex_metacharacters():
    # The sanitized token is matched literally, so metacharacters in it are not
    # treated as a pattern.
    content = Message._apply_kwargs("value _a[0] here", {"_a[0]": "x"})
    assert content == "value x here"


@pytest.mark.p1
def test_apply_kwargs_replaces_all_occurrences():
    content = Message._apply_kwargs("_a and _a", {"_a": "x"})
    assert content == "x and x"


@pytest.mark.p1
def test_apply_kwargs_skips_none_values():
    content = Message._apply_kwargs("_a stays", {"_a": None})
    assert content == "_a stays"
