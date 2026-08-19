"""Research Agent — inner tool-calling loop for high/ultra modes.

Native tool-calling: a chat model cloned from ``tools.chat_mdl`` is bound
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

from rag.advanced_rag.harness.pipeline import Pipeline
from rag.advanced_rag.harness.prompts.research_agent_prompt import (
    RESEARCH_AGENT_PROMPT,
    RESEARCH_AGENT_TEXT_PROMPT,
)
from rag.advanced_rag.harness.stats import in_phase
from rag.advanced_rag.harness.tools.gating import (
    SEARCH_PHASES,
    determine_current_phase,
    get_gated_tools,
)
from rag.advanced_rag.harness.tools.registry import _generate_report_schema, _think_schema
from rag.advanced_rag.harness.types import ClaimTarget, ExecutionStrategy, ToolResult

_LOG = logging.getLogger(__name__)


# Navigation tools that return passages (evidence chunks). After one of these
# runs, we judge sufficiency before the agent broadens into a full search.
_NAV_CHUNK_TOOLS = {"ontology_navigate", "mindmap_navigate"}


class ResearchToolSession:
    """ToolCallSession adapter routing native tool calls to the harness pipeline.

    - regular tools run through :func:`execute_with_fallback`;
    - ``think_tool`` is a no-op reasoning step that just lets the loop continue;
    - ``generate_report`` is *captured* (not executed) so the agent loop can
      return its structured arguments as the claim result.
    """

    def __init__(self, pipeline: Pipeline, phase: str, claim: ClaimTarget | None = None):
        self.pipeline = pipeline
        self.phase = phase
        self.claim = claim
        self.report: dict | None = None
        self.got_evidence = False
        self.evidence_ids: list[int] = []
        self._seen_evidence_ids: set[int] = set()

    async def tool_call_async(self, name: str, arguments: dict, request_timeout: float = 300):
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
        message = _fmt_tool_result(result)
        # Post-navigation sufficiency gate: once a navigation tool has returned
        # passages, decide whether they already answer the task and, if so, steer
        # the agent to finalize instead of running a broader (noisier) search.
        if name in _NAV_CHUNK_TOOLS and result.chunks and self.claim is not None:
            if await self._navigation_sufficient(result.chunks):
                ev = ", ".join(str(i) for i in self.evidence_ids) or "the passages above"
                message += f"\n\n[sufficiency check] These passages appear to answer the task. Call generate_report now with evidence_ids=[{ev}] — do not run further searches."
        return message

    async def _navigation_sufficient(self, chunks: list[dict]) -> bool:
        """One-shot LLM judge: do the given passages already answer the claim?"""
        try:
            passages = "\n\n".join((c.get("content_with_weight") or c.get("text") or "")[:500] for c in chunks[:8])
            if not passages.strip():
                return False
            system = "Judge whether the passages already contain enough information to answer the task. Reply with a single word: YES or NO."
            user = f"Task:\n{self.claim.description}\n\nPassages:\n{passages}\n\nAnswer (YES or NO):"
            ans = await self.pipeline.tools.chat_mdl.async_chat(system, [{"role": "user", "content": user}], {"temperature": 0.0})
            if isinstance(ans, tuple):
                ans = ans[0]
            ans = re.sub(r"^.*</think>", "", ans or "", flags=re.DOTALL)
            _LOG.debug("[Navigation] sufficiency check: %s", ans)
            return ans.strip().lower().startswith("yes")
        except Exception:
            _LOG.exception("[Navigation] sufficiency check failed")
            return False

    def _normalize_report(self, report: dict) -> dict:
        if not isinstance(report, dict):
            # Unstrusted text-path parser output; never crash on it.
            _LOG.warning("normalize_report: expected dict, got %s; using empty report", type(report).__name__)
            report = {}
        normalized = dict(report)
        recorded = set(self.evidence_ids)
        evidence_ids = []
        # The schema requires evidence_ids to be a list of integers. A scalar or
        # string would either raise or, worse, let the loop walk characters as
        # IDs, so coerce anything else to an empty list first.
        raw_evidence_ids = normalized.get("evidence_ids")
        if not isinstance(raw_evidence_ids, list):
            raw_evidence_ids = []
        for eid in raw_evidence_ids:
            # The schema permits integer IDs only. Reject booleans and floats
            # before conversion: int(True) == 1 and int(1.9) == 1 would both
            # silently become valid indexes, retaining evidence the model never
            # referenced. Keep only ints and numeric strings.
            if isinstance(eid, bool) or not isinstance(eid, (int, str)):
                continue
            try:
                idx = int(eid)
            except ValueError:
                continue
            # Restrict to chunks this claim actually recorded. Claims are
            # researched concurrently over a *shared* kbinfos pool, so a model
            # can otherwise cite another claim's chunk (or an out-of-range
            # index) via an arbitrary evidence_id.
            if idx in recorded:
                if idx not in evidence_ids:
                    evidence_ids.append(idx)
            elif recorded:
                _LOG.warning(
                    "evidence_id %s not in claim's recorded set (size=%d); dropping",
                    idx,
                    len(recorded),
                )
        if not evidence_ids:
            # Do NOT repurpose every recorded chunk as a citation: evidence_ids
            # means the chunks the report actually references, and the schema
            # defines it that way. When no valid ID remains, leave it empty and
            # mark the report unverified so downstream verifiers (which combine
            # evidence_ids + is_verified + confidence) cannot mistake unrelated
            # chunks for supporting evidence.
            normalized["is_verified"] = False
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


@in_phase("claim_research")
async def research_agent_loop(
    claim: ClaimTarget,
    tools,
    pipeline: Pipeline,
    context,
    mode: ExecutionStrategy,
    compilation_map: dict,
    followups: list[str] | None = None,
) -> dict:
    """Inner loop for a single claim — native tool-calling with a text fallback.

    ``followups`` (Phase-2 LLM missing-pieces feedback from the previous
    sufficiency round) is passed in explicitly by the orchestrator rather than
    read from the shared ``context`` here.  The orchestrator consumes and clears
    ``context.pending_followups`` ONCE per round so every claim researched in a
    parallel batch receives the SAME follow-up guidance (reading the shared list
    per-claim would race: the first claim to execute would clear it, starving
    the rest).
    """
    phase = determine_current_phase(context, claim=claim)
    phase_config = SEARCH_PHASES.get(phase, {})
    gated_defs = get_gated_tools(
        phase=phase,
        available_tools=mode.available_tools,
        compilation_map=compilation_map,
        context=context,
        has_routed_scope=bool(getattr(pipeline, "_routed_docs", None)),
        web_enabled=bool(getattr(tools, "has_web", lambda: False)()),
        claim=claim,
    )

    pipeline._active_phase = phase
    pipeline._round_had_evidence = False
    pipeline._round_had_routed_scope_progress = False

    # Clone so binding tools never leaks onto the shared chat model.
    agent_mdl = tools.chat_mdl.clone()
    if getattr(agent_mdl, "is_tools", False):
        result = await _research_native(claim, agent_mdl, pipeline, phase, phase_config, gated_defs, mode, followups)
    else:
        _LOG.info("research_agent: model lacks native tool support; falling back to text-based tool selection")
        result = await _research_text(claim, tools, pipeline, phase, phase_config, gated_defs, mode, followups)

    # Bookkeeping for locate-phase web fallback: a locate round that produced
    # evidence chunks or newly routed document scope counts as progress and
    # resets this claim's streak. Pre-existing request doc_scope does not
    # count: only scope produced by this round should prevent the fallback.
    # The phase stays `locate`; gating.py decides when repeated locate
    # failures should admit `web_search` into the candidate tool set.
    if pipeline._round_had_evidence or pipeline._round_had_routed_scope_progress:
        claim.locate_empty_streak = 0
    elif phase == "locate":
        claim.locate_empty_streak += 1
    return result


async def _research_native(
    claim: ClaimTarget,
    agent_mdl,
    pipeline: Pipeline,
    phase: str,
    phase_config: dict,
    gated_defs: list[dict],
    mode: ExecutionStrategy,
    followups: list[str] | None = None,
) -> dict:
    """Bind tools onto ``agent_mdl`` and let its native tool loop drive research."""
    schemas = _build_tool_schemas(gated_defs)
    session = ResearchToolSession(pipeline, phase, claim)
    agent_mdl.bind_tools(session, schemas)
    # Bound the model's internal tool loop to the mode's agent-cycle budget.
    base_rounds = max(1, mode.max_agent_cycles)
    if hasattr(agent_mdl, "mdl") and hasattr(agent_mdl.mdl, "max_rounds"):
        agent_mdl.mdl.max_rounds = base_rounds

    system = RESEARCH_AGENT_PROMPT.format(
        claim_description=claim.description,
        phase=phase,
        phase_hint=phase_config.get("tool_hint", ""),
        max_cycles=mode.max_agent_cycles,
    )
    history = [{"role": "user", "content": f"Research task: {claim.description}\nBegin."}]
    # Phase-2 missing-pieces guidance: focus this round on the specific gaps the
    # Sufficient Context AutoRater flagged, rather than re-searching broadly.
    if followups:
        history.append(
            {
                "role": "user",
                "content": "Previous evidence was incomplete. Run targeted searches specifically for the following missing pieces:\n- " + "\n- ".join(followups),
            }
        )

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
    followups: list[str] | None = None,
) -> dict:
    """Fallback: prompt-based tool selection for models without native tools."""
    # Mirror the native path's cycle budget; the locate→web_search fallback is
    # handled by gating.get_gated_tools injecting web_search into the tool set.
    text_max_cycles = mode.max_agent_cycles
    system = RESEARCH_AGENT_TEXT_PROMPT.format(
        claim_description=claim.description,
        phase=phase,
        phase_hint=phase_config.get("tool_hint", ""),
        tool_list=_fmt_tool_list(gated_defs),
        max_cycles=text_max_cycles,
    )

    history: list[dict] = []
    if followups:
        history.append(
            {
                "role": "user",
                "content": "Previous evidence was incomplete. Run targeted searches specifically for the following missing pieces:\n- " + "\n- ".join(followups),
            }
        )

    # Use a session so evidence IDs are recorded and the final report is
    # normalized against this claim's recorded chunks (same as the native path).
    session = ResearchToolSession(pipeline, phase, claim)

    for cycle in range(text_max_cycles):
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
            args = tool_call.get("arguments", {})
            if not isinstance(args, dict):
                _LOG.warning("generate_report: arguments not a dict (%s); using empty", type(args).__name__)
                args = {}
            report = session._normalize_report(args)
            return report

        if tool_call.get("name") == "think_tool":
            history.append({"role": "user", "content": "[continue]"})
            continue

        args = tool_call.get("arguments", {})
        # Text fallback bypasses ResearchToolSession.tool_call_async(), so it
        result = await execute_with_fallback(pipeline, tool_call["name"], phase, **args)
        if result.chunks:
            session._record_evidence_ids(result.chunks)
        history.append({"role": "user", "content": _fmt_tool_result(result)})

    return await _force_generate_report(history, tools, claim.claim_id, session)


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
    session: ResearchToolSession | None = None,
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

        report = json_repair.loads(text)
        if isinstance(report, dict) and session is not None:
            normalized = session._normalize_report(report)
            return normalized
        return report if isinstance(report, dict) else {"report": str(report)}
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
    # Some tools (ontology_navigate, structured_query) answer directly rather than
    # only returning passages — surface that, or the agent never sees it.
    answer = (result.metadata or {}).get("answer") if isinstance(result.metadata, dict) else ""
    if answer:
        parts.append(f"Answer: {answer}")
    # Show more chunks (6 vs the old 3) and order them by retrieval similarity,
    # so the agent actually sees the passages that best match its query. The old
    # "first 3 chunks, 300 chars each" made the agent miss the key evidence when
    # it sat in chunk 4+ (5.log/6.log: it retrieved 48m/27m but reported
    # "unrelated (Barack Obama)" and re-searched endlessly).
    chunks = list(result.chunks)
    chunks.sort(key=lambda c: float(c.get("similarity", 0.0) or 0.0), reverse=True)
    for c in chunks[:6]:
        text = c.get("content_with_weight") or c.get("text") or ""
        if text:
            parts.append(text[:300])
    if not parts:
        return "[no results found]"
    return "\n\n".join(parts)
