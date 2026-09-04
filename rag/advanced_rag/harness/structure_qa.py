"""Ask the chat model to answer a question from a compiled-structure outline.

Shared by the two navigation paths that read compiled rows:

* document structure navigation (``navigation._navigate_within_doc``) — the
  document's catalog / mindmap outline;
* knowledge-graph exploration (``navigation.graph_explore``) — the compiled
  knowledge graph.

Both render entities + relations into the same compact outline, ask the model
whether that outline alone answers the question, and use the returned
``relevant_entities`` to pull the underlying source chunks even when the outline
is NOT sufficient (so evidence still flows back to the caller).

Lives apart from ``navigation`` because it is pure prompt/render work with no
store access, and because both callers would otherwise import it from a module
that in turn imports them.
"""

import logging
import re

import json_repair

_LOG = logging.getLogger(__name__)

# Cap how much compiled structure we render into the prompt.
_MAX_ENTITIES = 300
_MAX_RELATIONS = 300

_NAV_SYSTEM = """You are given {noun} of one or more documents — an outline of entities and their relations — and a question.

Decide whether that outline alone already answers the question.

Rules:
1. Answer ONLY from the outline below. Do not invent facts.
2. Set "is_sufficient" to true only when the outline genuinely answers the question; otherwise false with an empty answer.
3. Always fill "relevant_entities" with the exact `name` values of the entities most related to the question (up to 10), even when the outline is not sufficient — they are used to pull the underlying source text.

Output ONLY JSON, no prose, no code fences:
{{"is_sufficient": true/false, "answer": "<answer, or empty>", "relevant_entities": ["<entity name>", ...]}}"""


def _render_structure(entities: list[dict], relations: list[dict]) -> str:
    """Render the compiled structure as a compact outline for the prompt."""
    lines: list[str] = []
    if entities:
        lines.append("Entities:")
        for e in entities[:_MAX_ENTITIES]:
            name = (e.get("name") or "").strip()
            if not name:
                continue
            typ = (e.get("type") or "other").strip()
            desc = " ".join((e.get("description") or "").split())
            lines.append(f"- {name} ({typ})" + (f": {desc}" if desc else ""))
    if relations:
        lines.append("\nRelations:")
        for r in relations[:_MAX_RELATIONS]:
            src, tgt = (r.get("from") or "").strip(), (r.get("to") or "").strip()
            if not src or not tgt:
                continue
            lines.append(f"- {src} -[{(r.get('type') or 'related').strip()}]-> {tgt}")
    return "\n".join(lines)


async def _ask_structure(tools, topic: str, entities: list[dict], relations: list[dict], noun: str, label: str) -> tuple[str, list[str]]:
    """Ask the chat model to answer ``topic`` from the rendered outline.

    Returns ``(answer, relevant_entity_names)`` — ``answer`` is empty unless the
    model judged the outline sufficient; the names are always returned so the
    caller can pull the underlying source chunks.
    """
    verdict = {}
    try:
        from rag.prompts.generator import form_message, message_fit_in

        user = f"Question:\n{topic}\n\n{noun.capitalize()}:\n{_render_structure(entities, relations)}\n\nOutput JSON:"
        _, msg = message_fit_in(form_message(_NAV_SYSTEM.format(noun=f"the {noun}"), user), tools.chat_mdl.max_length)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.2})
        if isinstance(ans, tuple):
            ans = ans[0]
        cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        verdict = json_repair.loads(cleaned) or {}
        if not isinstance(verdict, dict):
            verdict = {}
    except Exception:
        _LOG.exception(f"[{label}] Could not read the outline with the model.")

    is_sufficient = bool(verdict.get("is_sufficient"))
    raw_answer = verdict.get("answer")
    answer = str(raw_answer).strip() if is_sufficient and raw_answer is not None else ""
    relevant = [n for n in (verdict.get("relevant_entities") or []) if isinstance(n, str)]
    _LOG.info(
        "[%s] The %s %s the question; %d relevant entity(ies): %s",
        label,
        noun,
        "answers" if is_sufficient else "does not fully answer",
        len(relevant),
        ", ".join(relevant[:10]) or "none",
    )
    return answer, relevant
