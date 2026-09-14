from types import SimpleNamespace

from rag.advanced_rag.harness.action_session import _TOOL_MAP, _active_tool_specs
from rag.advanced_rag.harness.config import _ALL_TOOLS
from rag.prompts.template import load_prompt

PLAYBOOK_ANCHORS = ["WHEN TO CALL", "DO NOT CALL", "ARGUMENTS", "OUTPUT", "IF IT FAILS"]

# Params the executor (execute_tool) actually consumes for each tool. The schema
# MUST NOT declare any param outside this set — otherwise the model is told to
# fill an argument the runtime silently ignores (the list_chunks(chunk_ids) ghost
# bug). Note: doc_scope / keywords are supported by the executor but intentionally
# NOT declared in the schema (decided in plan: keep description honest with impl).
_EXECUTOR_SUPPORTED = {
    "retrieve": {"query", "doc_scope"},
    "search_chunks": {"query"},
    "list_chunks": {"doc_id"},
    "navigate_tree": {"query", "keywords"},
    "navigate_structure": {"doc_id", "query", "kind"},
    "calculate": {"question", "facts"},
    "web_search": {"query"},
}


def _mk_tools(mode: str, web: bool = True) -> SimpleNamespace:
    # ``decision`` is injected at runtime and is NOT part of _TOOL_MAP, so the
    # web_search presence is what flips 7<->6; thinking_mode selects the surface.
    return SimpleNamespace(
        thinking_mode=mode,
        web_search=("provider" if web else None),
        _disabled_tools=set(),
    )


def test_tool_specs_have_playbook_sections():
    """Each of the 7 tools documents the 5-section contract, within the token budget."""
    for name in _ALL_TOOLS:
        desc = _TOOL_MAP[name]["function"]["description"]
        for anchor in PLAYBOOK_ANCHORS:
            assert anchor in desc, f"{name} description missing anchor {anchor!r}"
        assert len(desc) <= 1200, f"{name} description too long: {len(desc)} chars (cap 1200)"


def test_active_tool_specs_tool_surface():
    """Mode → exposed tool count: low=0, medium/high=7, ultra=8, web-hidden=6."""
    assert len(_active_tool_specs(_mk_tools("low"))) == 0
    assert len(_active_tool_specs(_mk_tools("medium"))) == 7
    assert len(_active_tool_specs(_mk_tools("high"))) == 7
    assert len(_active_tool_specs(_mk_tools("ultra"))) == 8
    assert len(_active_tool_specs(_mk_tools("medium", web=False))) == 6


def test_schema_params_match_executor():
    """No schema declares a param the executor cannot consume (no ghost args)."""
    for name in _ALL_TOOLS:
        props = _TOOL_MAP[name]["function"]["parameters"].get("properties", {})
        declared = set(props) - {"decision"}
        unsupported = declared - _EXECUTOR_SUPPORTED[name]
        assert not unsupported, f"{name} declares unsupported params: {unsupported}"


def test_action_run_prompt_has_playbook():
    """action_run.md exposes the TOOL PLAYBOOK and only references real tools."""
    prompt = load_prompt("action_run")
    assert "TOOL PLAYBOOK" in prompt
    for tool in ("navigate_structure", "calculate", "graph_explore"):
        assert tool in prompt
    for tool in (
        "retrieve",
        "search_chunks",
        "list_chunks",
        "navigate_tree",
        "navigate_structure",
        "calculate",
        "web_search",
        "graph_explore",
    ):
        assert tool in _TOOL_MAP
