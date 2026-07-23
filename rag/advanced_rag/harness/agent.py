"""Research Agent — inner tool-calling loop for high/ultra modes.

Native tool-calling: a chat model deep-copied from ``tools.chat_mdl`` is bound
(via ``bind_tools``) to the phase-gated tool schemas plus ``think_tool`` /
``generate_report``, and a lightweight session routes each tool call to the
harness pipeline. Binding onto a *copy* keeps the shared ``tools.chat_mdl``
(used by the other graph nodes) free of any tool schema.

Models without native tool-calling fall back to prompt-based tool selection:
the tools are described in the prompt and the model emits ``<tool_call>`` JSON
that the loop parses.
"""

import json
import logging
import re
from copy import deepcopy

from rag.advanced_rag.harness.types import ClaimTarget, ExecutionStrategy, ToolResult
from rag.advanced_rag.harness.pipeline import Pipeline
from rag.advanced_rag.harness.tools.gating import (
    get_gated_tools,
    determine_current_phase,
    SEARCH_PHASES,
)
from rag.advanced_rag.harness.tools.registry import _generate_report_schema, _think_schema
from rag.advanced_rag.harness.prompts.research_agent_prompt import (
    RESEARCH_AGENT_PROMPT,
    RESEARCH_AGENT_TEXT_PROMPT,
)

_LOG = logging.getLogger(__name__)


class ResearchToolSession:
    """ToolCallSession adapter routing native tool calls to the harness pipeline.

    - regular tools run through :func:`execute_with_fallback`;
    - ``think_tool`` is a no-op reasoning step that just lets the loop continue;
    - ``generate_report`` is *captured* (not executed) so the agent loop can
      return its structured arguments as the claim result.
    """

    def __init__(self, pipeline: Pipeline, phase: str):
        self.pipeline = pipeline
        self.phase = phase
        self.report: dict | None = None
        self.got_evidence = False
        self.evidence_ids: list[int] = []
        self._seen_evidence_ids: set[int] = set()

    async def tool_call_async(self, name: str, arguments: dict, request_timeout: float | int = 300):
        arguments = arguments or {}
        if name == "generate_report":
            self.report = self._normalize_report(arguments)
            return "Report received. Stop calling tools now."
        if name == "think_tool":
            return "Noted. Proceed with the next tool call."
        result = await execute_with_fallback(self.pipeline, name, self.phase, **arguments)
        if result.chunks:
            self.got_evidence = True
            self._record_evidence_ids(result.chunks)
        return _fmt_tool_result(result)

    def _normalize_report(self, report: dict) -> dict:
        normalized = dict(report)
        evidence_ids = []
        for eid in normalized.get("evidence_ids") or []:
            try:
                idx = int(eid)
            except (TypeError, ValueError):
                continue
            if idx not in evidence_ids:
                evidence_ids.append(idx)
        if not evidence_ids and self.evidence_ids:
            evidence_ids = list(self.evidence_ids)
        normalized["evidence_ids"] = evidence_ids
        return normalized

    def _record_evidence_ids(self, chunks: list[dict]) -> None:
        all_chunks = self.pipeline.tools.kbinfos.get("chunks", [])
        index_by_key = {}
        for idx, chunk in enumerate(all_chunks):
            index_by_key[_chunk_key(chunk)] = idx

        for chunk in chunks:
            idx = index_by_key.get(_chunk_key(chunk))
            if idx is None:
                idx = next((i for i, existing in enumerate(all_chunks) if existing is chunk), None)
            if idx is None or idx in self._seen_evidence_ids:
                continue
            self._seen_evidence_ids.add(idx)
            self.evidence_ids.append(idx)


def _chunk_key(chunk: dict) -> object:
    return chunk.get("chunk_id") or chunk.get("id") or id(chunk)


def _build_tool_schemas(gated_defs: list[dict]) -> list[dict]:
    """Phase-gated schemas (minus harness-only ``x_*`` keys) + the control tools."""
    schemas: list[dict] = []
    for d in gated_defs:
        schemas.append({k: v for k, v in d.items() if not k.startswith("x_")})
    schemas.append(_think_schema())
    schemas.append(_generate_report_schema())
    return schemas


async def research_agent_loop(
    claim: ClaimTarget,
    tools,
    pipeline: Pipeline,
    context,
    mode: ExecutionStrategy,
    compilation_map: dict,
) -> dict:
    """Inner loop for a single claim — native tool-calling with a text fallback."""
    phase = determine_current_phase(context)
    phase_config = SEARCH_PHASES.get(phase, {})
    gated_defs = get_gated_tools(
        phase=phase,
        available_tools=mode.available_tools,
        compilation_map=compilation_map,
        context=context,
    )

    # Deep-copy so binding tools never leaks onto the shared chat model.
    agent_mdl = deepcopy(tools.chat_mdl)
    if getattr(agent_mdl, "is_tools", False):
        return await _research_native(claim, agent_mdl, pipeline, phase, phase_config, gated_defs, mode)

    _LOG.info("research_agent: model lacks native tool support; falling back to text-based tool selection")
    return await _research_text(claim, tools, pipeline, phase, phase_config, gated_defs, mode)


async def _research_native(
    claim: ClaimTarget,
    agent_mdl,
    pipeline: Pipeline,
    phase: str,
    phase_config: dict,
    gated_defs: list[dict],
    mode: ExecutionStrategy,
) -> dict:
    """Bind tools onto ``agent_mdl`` and let its native tool loop drive research."""
    schemas = _build_tool_schemas(gated_defs)
    session = ResearchToolSession(pipeline, phase)
    agent_mdl.bind_tools(session, schemas)
    # Bound the model's internal tool loop to the mode's agent-cycle budget.
    if hasattr(agent_mdl, "mdl") and hasattr(agent_mdl.mdl, "max_rounds"):
        agent_mdl.mdl.max_rounds = max(1, mode.max_agent_cycles)

    system = RESEARCH_AGENT_PROMPT.format(
        claim_description=claim.description,
        phase=phase,
        phase_hint=phase_config.get("tool_hint", ""),
        max_cycles=mode.max_agent_cycles,
    )
    history = [{"role": "user", "content": f"Research task: {claim.description}\nBegin."}]

    final_text = ""
    try:
        final_text = await agent_mdl.async_chat(system, history, {"temperature": 0.3})
        if isinstance(final_text, tuple):
            final_text = final_text[0]
    except Exception:
        _LOG.exception("research_agent(native): tool loop failed")

    if session.report is not None:
        return session.report

    # The model finished without calling generate_report — synthesize a report
    # from its final free-text turn so the claim still yields something usable.
    _LOG.info("research_agent(native): no generate_report call; using final text as report")
    return {
        "report": (final_text or "").strip(),
        "is_verified": session.got_evidence,
        "confidence": 0.5 if session.got_evidence else 0.0,
        "evidence_ids": list(session.evidence_ids),
        "gaps": [] if session.got_evidence else ["no generate_report emitted"],
        "discovered_claims": [],
    }


async def _research_text(
    claim: ClaimTarget,
    tools,
    pipeline: Pipeline,
    phase: str,
    phase_config: dict,
    gated_defs: list[dict],
    mode: ExecutionStrategy,
) -> dict:
    """Fallback: prompt-based tool selection for models without native tools."""
    system = RESEARCH_AGENT_TEXT_PROMPT.format(
        claim_description=claim.description,
        phase=phase,
        phase_hint=phase_config.get("tool_hint", ""),
        tool_list=_fmt_tool_list(gated_defs),
        max_cycles=mode.max_agent_cycles,
    )

    history: list[dict] = []

    for cycle in range(mode.max_agent_cycles):
        try:
            ans = await tools.chat_mdl.async_chat(system, history, {"temperature": 0.3})
            if isinstance(ans, tuple):
                ans = ans[0]
        except Exception:
            _LOG.exception("research_agent(text): LLM call failed cycle %d", cycle)
            continue

        history.append({"role": "assistant", "content": ans})

        tool_call = _parse_tool_call(ans)
        if not tool_call:
            history.append({"role": "user", "content": "Please call a tool. Do not output plain text."})
            continue

        if tool_call.get("name") == "generate_report":
            return tool_call.get("arguments", {})

        if tool_call.get("name") == "think_tool":
            history.append({"role": "user", "content": "[continue]"})
            continue

        args = tool_call.get("arguments", {})
        result = await execute_with_fallback(pipeline, tool_call["name"], phase, **args)
        history.append({"role": "user", "content": _fmt_tool_result(result)})

    return await _force_generate_report(history, tools, claim.claim_id)


def _parse_tool_call(text: str) -> dict | None:
    """Parse tool call from LLM response text (text-fallback path)."""
    m = re.search(r"<tool_call>(.*?)</tool_call>", text, re.DOTALL)
    if m:
        try:
            return json.loads(m.group(1).strip())
        except Exception:
            pass

    m = re.search(r"```(?:json)?\s*({.*?})\s*```", text, re.DOTALL)
    if m:
        try:
            return json.loads(m.group(1).strip())
        except Exception:
            pass

    m = re.search(r'\{\s*"name"\s*:', text)
    if m:
        try:
            import json_repair

            return json_repair.loads(text)
        except Exception:
            pass

    return None


async def execute_with_fallback(
    pipeline: Pipeline,
    tool_name: str,
    phase: str,
    **kwargs,
) -> ToolResult:
    """Execute tool; if empty, fall back along phase priority."""
    result = await pipeline.execute(tool_name, **kwargs)

    if result.chunks or result.error:
        return result

    phase_config = SEARCH_PHASES.get(phase, {})
    priority = phase_config.get("tools_priority", [])
    current_idx = next(
        (i for i, t in enumerate(priority) if t == tool_name),
        -1,
    )
    for fallback_name in priority[current_idx + 1 :]:
        fallback_result = await pipeline.execute(fallback_name, **kwargs)
        if fallback_result.chunks:
            _LOG.info("fallback: %s empty → %s found %d chunks", tool_name, fallback_name, len(fallback_result.chunks))
            fallback_result.metadata["was_fallback"] = True
            fallback_result.metadata["fallback_from"] = tool_name
            return fallback_result
        if fallback_result.error:
            break
    return result


async def _force_generate_report(
    history: list,
    tools,
    claim_id: str,
) -> dict:
    """Force generate report when max cycles reached (text-fallback path)."""
    try:
        ans = await tools.chat_mdl.async_chat(
            "",
            history + [{"role": "user", "content": "We've reached the research limit. Please output a final report as JSON."}],
            {"temperature": 0.3},
        )
        if isinstance(ans, tuple):
            ans = ans[0]
        text = re.sub(r"```(?:json)?\s*|\s*```", "", ans).strip()
        import json_repair

        return json_repair.loads(text)
    except Exception:
        _LOG.exception("force_generate_report failed")
        return {
            "report": "",
            "is_verified": False,
            "confidence": 0.0,
            "evidence_ids": [],
            "gaps": ["forced report — data may be incomplete"],
            "discovered_claims": [],
        }


def _fmt_tool_list(defs: list[dict]) -> str:
    lines = []
    for d in defs:
        func = d.get("function", d)
        name = func.get("name", "?")
        desc = func.get("description", "")
        params = func.get("parameters", {}).get("properties", {})
        params_text = ", ".join(f"{k}: {v.get('description', '')}" for k, v in params.items())
        lines.append(f"- {name}: {desc}")
        if params_text:
            lines.append(f"  Parameters: {params_text}")
    return "\n".join(lines)


def _fmt_tool_result(result: ToolResult) -> str:
    if result.error:
        return f"[tool error] {result.error}"
    parts: list[str] = []
    # Some tools (catalog_navigate, structured_query) answer directly rather than
    # only returning passages — surface that, or the agent never sees it.
    answer = (result.metadata or {}).get("answer") if isinstance(result.metadata, dict) else ""
    if answer:
        parts.append(f"Answer: {answer}")
    parts.extend(c.get("content_with_weight", c.get("text", ""))[:300] for c in result.chunks[:3])
    if not parts:
        return "[no results found]"
    return "\n\n".join(parts)
