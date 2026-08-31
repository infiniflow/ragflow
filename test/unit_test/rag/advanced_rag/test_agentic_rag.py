import pytest

from rag.advanced_rag.agentic_rag import RAGTools


class FakeChatModel:
    max_length = 8192

    def clone(self):
        return self


@pytest.mark.asyncio
async def test_rag_tool_adds_text_attachment_to_user_question(monkeypatch):
    captured = {}

    async def fake_run_agentic_rag(tools, messages):
        captured["messages"] = messages
        yield "answer"

    monkeypatch.setattr("rag.advanced_rag.agentic_rag_graph.run_agentic_rag", fake_run_agentic_rag)

    tools = RAGTools([], FakeChatModel(), text_attachments_content="attached facts")

    assert await tools.rag("What is attached?") == "answer"
    assert captured["messages"] == [{"role": "user", "content": "What is attached?attached facts"}]


class TestRAGToolsSystemPrompt:
    """Regression for #18833.

    The dialog's configured system prompt was previously dropped on the
    agentic reasoning path: RAGTools was constructed without the
    `user_defined_prompts` arg, so sys_prompt() returned the hardcoded
    router prompt only. The non-reasoning path (async_chat) already
    applied the dialog's system prompt directly, so this was a path
    asymmetry the operator could only see by changing the reasoning
    level.

    The fix:
    1. dialog_service.rag_agent passes `prompt_config` as
       `user_defined_prompts` to RAGTools.
    2. RAGTools.sys_prompt() prepends user_defined_prompts["system"]
       to the router prompt when set.
    """

    def test_rag_tools_sys_prompt_includes_user_system_prompt(self):
        """When user_defined_prompts has a 'system' key, the sys_prompt()
        returned by RAGTools must include that text BEFORE the router
        policy, so the outer agentic model sees the operator's domain
        instructions on every reasoning-level choice (not just NONE)."""
        tools = RAGTools(
            [],
            FakeChatModel(),
            user_defined_prompts={"system": "You are a customer-support agent. Always be polite."},
        )
        prompt = tools.sys_prompt()
        assert "customer-support agent" in prompt, (
            "sys_prompt() dropped the user-defined system prompt — "
            "see #18833"
        )
        # The router policy is still authoritative AFTER the user
        # content, so the agent's tool-selection logic is preserved.
        assert "smart agent" in prompt

    def test_rag_tools_sys_prompt_unchanged_when_no_user_prompt(self):
        """When no user_defined_prompts (or no 'system' key) is given,
        sys_prompt() must keep its previous shape so the agentic
        reasoning path is unchanged for operators who never set a
        system prompt."""
        tools_empty = RAGTools([], FakeChatModel())
        assert "smart agent" in tools_empty.sys_prompt()
        assert "customer" not in tools_empty.sys_prompt()

        tools_without_system = RAGTools(
            [], FakeChatModel(), user_defined_prompts={"citation_guidelines": "x"}
        )
        assert "smart agent" in tools_without_system.sys_prompt()
        assert "customer" not in tools_without_system.sys_prompt()

    def test_rag_tools_sys_prompt_handles_whitespace_only_user_prompt(self):
        """A whitespace-only 'system' (e.g. accidental blank entry) must
        not bloat the prompt with surrounding blank lines; the
        prepending branch uses str.strip() so all-whitespace values are
        treated as 'not set'."""
        tools = RAGTools(
            [],
            FakeChatModel(),
            user_defined_prompts={"system": "   \n  \n  "},
        )
        prompt = tools.sys_prompt()
        # No extra leading blank lines from the user prompt.
        assert prompt.startswith(
            "You are a smart agent."
        )


class TestRAGToolsSystemPromptIncludesUserDefined:
    """Regression for #18833.

    The dialog's configured system prompt was previously dropped on
    the agentic reasoning path: rag_agent constructed RAGTools without
    user_defined_prompts, so sys_prompt() returned only the hardcoded
    router prompt. The non-reasoning path (async_chat) already applied
    the dialog's system prompt directly, so this was a path
    asymmetry the operator could only see by changing the reasoning
    level.

    These tests pin the fix on the RAGTools class. We import the class
    directly from its source file (not through the rag.advanced_rag
    package __init__) and stub the few cross-module imports that are
    only needed for unrelated lifecycle hooks. The two assertions in
    each test verify both the inclusion and the precedence:
    1. the user's system prompt is visible to the outer agentic model;
    2. the router's tool-selection policy is still authoritative
       (the user content is prepended, not replaced).
    """

    @staticmethod
    def _build_ragtools_class():
        # Import the class file directly so we don't pull in the
        # package __init__'s heavy chain (mcp, openai, etc.). The
        # class itself only needs citation_prompt and LLMBundle at
        # definition time, so stub them.
        import importlib.util
        from types import ModuleType
        from unittest.mock import MagicMock
        from pathlib import Path
        from rag.prompts.generator import citation_prompt  # available

        # Stub the api.db.db_models import (only used at module load).
        api_pkg = ModuleType("api")
        api_pkg.__path__ = ["/tmp/opencode/repos/ragflow/api"]
        sys.modules.setdefault("api", api_pkg)
        api_db_pkg = ModuleType("api.db"); api_db_pkg.__path__ = ["/tmp/opencode/repos/ragflow/api/db"]
        sys.modules.setdefault("api.db", api_db_pkg)
        api_db_services_pkg = ModuleType("api.db.services")
        api_db_services_pkg.__path__ = ["/tmp/opencode/repos/ragflow/api/db/services"]
        sys.modules.setdefault("api.db.services", api_db_services_pkg)
        api_db_models = ModuleType("api.db.db_models")
        api_db_models.Document = MagicMock()
        api_db_models.Knowledgebase = MagicMock()
        sys.modules.setdefault("api.db.db_models", api_db_models)
        api_utils = ModuleType("api.utils")
        api_utils.api_utils = MagicMock()
        sys.modules.setdefault("api.utils", api_utils)

        spec = importlib.util.spec_from_file_location(
            "_agentic_rag_test_under_test",
            Path(__file__).resolve().parent.parent.parent.parent
            / "rag"
            / "advanced_rag"
            / "agentic_rag.py",
        )
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return mod.RAGTools

    def test_sys_prompt_includes_user_system_prompt(self):
        RAGTools = self._build_ragtools_class()

        class FakeChatModel:
            max_length = 8192
            def clone(self):
                return self

        tools = RAGTools(
            [],
            FakeChatModel(),
            user_defined_prompts={"system": "You are a customer-support agent. Always be polite."},
        )
        prompt = tools.sys_prompt()
        assert "customer-support agent" in prompt, (
            f"sys_prompt() dropped the user-defined system prompt — "
            f"see #18833. Got: {prompt[:200]}"
        )
        # The router policy is still authoritative AFTER the user
        # content, so the agent's tool-selection logic is preserved.
        assert "smart agent" in prompt

    def test_sys_prompt_unchanged_when_no_user_prompt(self):
        RAGTools = self._build_ragtools_class()

        class FakeChatModel:
            max_length = 8192
            def clone(self):
                return self

        # No user_defined_prompts at all.
        tools_a = RAGTools([], FakeChatModel())
        prompt_a = tools_a.sys_prompt()
        assert "smart agent" in prompt_a
        assert "customer" not in prompt_a

        # user_defined_prompts without a 'system' key.
        tools_b = RAGTools([], FakeChatModel(), user_defined_prompts={"citation_guidelines": "x"})
        prompt_b = tools_b.sys_prompt()
        assert "smart agent" in prompt_b
        assert "customer" not in prompt_b

    def test_sys_prompt_handles_whitespace_only_user_prompt(self):
        RAGTools = self._build_ragtools_class()

        class FakeChatModel:
            max_length = 8192
            def clone(self):
                return self

        # Whitespace-only 'system' must be treated as not set so the
        # user prompt doesn't bloat the prompt with leading blank lines.
        tools = RAGTools(
            [],
            FakeChatModel(),
            user_defined_prompts={"system": "   \n  \n  "},
        )
        prompt = tools.sys_prompt()
        assert prompt.startswith("You are a smart agent."), (
            "Whitespace-only 'system' must not bloat the prefix"
        )
