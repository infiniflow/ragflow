"""Research Agent — inner tool-calling loop for high/ultra modes.

Uses prompt-based tool selection: LLM outputs tool decisions as JSON in its
response text, which the harness parses and executes. This avoids dependency
on the LLMBundle's tool-calling protocol.
"""

import json
import logging
import re

from rag.advanced_rag.harness.types import ClaimTarget, ExecutionStrategy, ToolResult
from rag.advanced_rag.harness.pipeline import Pipeline
from rag.advanced_rag.harness.tools.gating import (
    get_gated_tools,
    determine_current_phase,
    SEARCH_PHASES,
)
from rag.advanced_rag.harness.prompts.research_agent_prompt import RESEARCH_AGENT_PROMPT

_LOG = logging.getLogger(__name__)


async def research_agent_loop(
    claim: ClaimTarget,
    tools,
    pipeline: Pipeline,
    context,
    mode: ExecutionStrategy,
    compilation_map: dict,
) -> dict:
    """Inner loop: prompt-based tool selection for a single claim."""
    phase = determine_current_phase(context)
    phase_config = SEARCH_PHASES.get(phase, {})

    gated_defs = get_gated_tools(
        phase=phase,
        available_tools=mode.available_tools,
        compilation_map=compilation_map,
        context=context,
    )

    tool_list_text = _fmt_tool_list(gated_defs)
    system = RESEARCH_AGENT_PROMPT.format(
        claim_description=claim.description,
        phase=phase,
        phase_hint=phase_config.get("tool_hint", ""),
        tool_list=tool_list_text,
        max_cycles=mode.max_agent_cycles,
    )

    history: list[dict] = []

    for cycle in range(mode.max_agent_cycles):
        try:
            ans = await tools.chat_mdl.async_chat(system, history, {"temperature": 0.3})
            if isinstance(ans, tuple):
                ans = ans[0]
        except Exception:
            _LOG.exception("research_agent: LLM call failed cycle %d", cycle)
            continue

        history.append({"role": "assistant", "content": ans})

        tool_call = _parse_tool_call(ans)
        if not tool_call:
            history.append({"role": "user", "content": "Please call a tool. Do not output plain text."})
            continue

        if tool_call.get("name") == "generate_report":
            return tool_call.get("arguments", {})

        elif tool_call.get("name") == "think_tool":
            history.append({"role": "user", "content": "[continue]"})
            continue

        else:
            args = tool_call.get("arguments", {})
            result = await execute_with_fallback(
                pipeline,
                tool_call["name"],
                phase,
                **args,
            )
            result_text = _fmt_tool_result(result)
            history.append({"role": "user", "content": result_text})

    # Max cycles — force report
    return await _force_generate_report(history, tools, claim.claim_id)


def _parse_tool_call(text: str) -> dict | None:
    """Parse tool call from LLM response text."""
    # Try JSON in <tool_call> tags
    m = re.search(r"<tool_call>(.*?)</tool_call>", text, re.DOTALL)
    if m:
        try:
            return json.loads(m.group(1).strip())
        except Exception:
            pass

    # Try standalone JSON block
    m = re.search(r"```(?:json)?\s*({.*?})\s*```", text, re.DOTALL)
    if m:
        try:
            return json.loads(m.group(1).strip())
        except Exception:
            pass

    # Try raw JSON object
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
    """Force generate report when max cycles reached."""
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
    chunks = result.chunks[:3]
    texts = [c.get("content_with_weight", c.get("text", ""))[:300] for c in chunks]
    if not texts:
        return "[no results found]"
    return "\n\n".join(texts)
