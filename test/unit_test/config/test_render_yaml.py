from docker.scripts.render_yaml import replace_env, render_node


# -----------------------
# Tests for replace_env
# -----------------------

def test_replace_env_basic(monkeypatch):
    # Environment variable is set
    monkeypatch.setenv("FOO", "bar")
    assert replace_env("${FOO}") == "bar"
    assert replace_env("${FOO:-default}") == "bar"

    # Environment variable not set, use default
    monkeypatch.delenv("FOO", raising=False)
    assert replace_env("${FOO:-default}") == "default"


def test_replace_env_empty_string(monkeypatch):
    # Environment variable set to empty string, should fallback to default
    monkeypatch.setenv("FOO", "")
    assert replace_env("${FOO:-default}") == "default"


def test_replace_env_no_default(monkeypatch):
    monkeypatch.delenv("FOO", raising=False)
    # No default, should replace with empty string
    assert replace_env("${FOO}") == ""


def test_replace_env_quotes(monkeypatch):
    monkeypatch.delenv("FOO", raising=False)
    assert replace_env('${FOO:-"quoted"}') == "quoted"
    assert replace_env("${FOO:-'quoted2'}") == "quoted2"


def test_replace_env_non_string():
    # Non-string values should be returned as-is
    assert replace_env(123) == 123
    assert replace_env(None) is None
    assert replace_env(True) is True
    assert replace_env([1, 2]) == [1, 2]
    assert replace_env({"a": 1}) == {"a": 1}


# -----------------------
# Tests for render_node
# -----------------------

def test_render_node_scalar(monkeypatch):
    monkeypatch.setenv("FOO", "bar")
    node = "${FOO}"
    assert render_node(node) == "bar"


def test_render_node_list(monkeypatch):
    monkeypatch.setenv("FOO", "x")
    monkeypatch.delenv("BAR", raising=False)
    node = ["${FOO}", "${BAR:-y}", 123]
    expected = ["x", "y", 123]
    assert render_node(node) == expected


def test_render_node_dict(monkeypatch):
    monkeypatch.setenv("FOO", "a")
    monkeypatch.delenv("BAR", raising=False)
    node = {"key1": "${FOO}", "key2": "${BAR:-b}", "key3": 42}
    expected = {"key1": "a", "key2": "b", "key3": 42}
    assert render_node(node) == expected


def test_render_node_nested(monkeypatch):
    monkeypatch.setenv("X", "1")
    monkeypatch.delenv("Y", raising=False)
    node = {
        "level1": {
            "list": ["${X}", "${Y:-y}", {"nested": "${X}"}],
            "value": "${Y:-z}"
        }
    }
    expected = {
        "level1": {
            "list": ["1", "y", {"nested": "1"}],
            "value": "z"
        }
    }
    assert render_node(node) == expected


def test_render_node_empty(monkeypatch):
    # Test empty list/dict/scalar
    monkeypatch.delenv("FOO", raising=False)
    assert render_node([]) == []
    assert render_node({}) == {}
    assert render_node("") == ""
    assert render_node(None) is None
