"""Pipeline — unified tool execution dispatcher."""

import time
import logging
from typing import Any

from rag.advanced_rag.harness.types import ToolResult
from rag.advanced_rag.harness.tools.registry import TOOL_REGISTRY

_LOG = logging.getLogger(__name__)


class Pipeline:
    """Unified tool execution layer.

    - execute(tool_name, **kwargs): dispatch to registered tool, normalize result
    - available_tools(mode_tools): return LLM-visible tool definitions (compilation-filtered)
    - get_chunks(evidence_ids): retrieve raw chunks for sufficiency cross-check
    - trace: execution history for auditing
    """

    def __init__(self, rag_tools, compilation_map: dict[str, set[str]] | None = None):
        self.tools = rag_tools
        self.compilation_map = compilation_map or {}
        self.trace: list[dict] = []

    async def execute(self, tool_name: str, **kwargs) -> ToolResult:
        """Execute a registered tool by name."""
        tool = TOOL_REGISTRY.get(tool_name)
        if not tool:
            return ToolResult(chunks=[], metadata={}, error=f"Unknown tool: {tool_name}")

        fn = tool.get("fn")
        if not fn:
            return ToolResult(chunks=[], metadata={}, error=f"Tool {tool_name} has no executor")

        start = time.time()
        try:
            raw = await fn(self.tools, **kwargs)
            elapsed = time.time() - start
            self.trace.append({"tool": tool_name, "args": kwargs, "elapsed": elapsed, "success": True})
            result = self._normalize(raw)
            # Feed the shared citation pool: agent searches go through the
            # pipeline, so without this their evidence never reaches kbinfos and
            # the final answer has nothing to cite.
            self._merge_into_kbinfos(result)
            return result
        except Exception as e:
            elapsed = time.time() - start
            _LOG.exception("Pipeline.execute(%s) failed", tool_name)
            self.trace.append({"tool": tool_name, "args": kwargs, "elapsed": elapsed, "success": False, "error": str(e)})
            return ToolResult(chunks=[], metadata={}, error=str(e))

    def available_tools(self, mode_tools: list[str]) -> list[dict]:
        """Return LLM-visible tool definitions, filtered by compilation availability."""
        names = filter_available_tools(mode_tools, self.compilation_map)
        defs = []
        for name in names:
            tool = TOOL_REGISTRY.get(name)
            if tool and tool.get("function_schema"):
                defs.append(tool["function_schema"])
        return defs

    def get_chunks(self, evidence_ids: list[int]) -> dict[int, dict]:
        """Retrieve raw chunks by ID from current kbinfos."""
        result = {}
        chunks = self.tools.kbinfos.get("chunks", [])
        for eid in evidence_ids:
            if 0 <= eid < len(chunks):
                result[eid] = chunks[eid]
        return result

    def get_trace(self) -> list[dict]:
        return list(self.trace)

    # ── Private ──

    def _merge_into_kbinfos(self, result: ToolResult) -> None:
        """Merge a tool result's chunks/doc_aggs into ``tools.kbinfos``, deduped."""
        if not result or not result.chunks:
            return
        kb = self.tools.kbinfos
        seen = {c.get("chunk_id") or c.get("id") or id(c) for c in kb.get("chunks", [])}
        for c in result.chunks:
            k = c.get("chunk_id") or c.get("id") or id(c)
            if k in seen:
                continue
            seen.add(k)
            kb.setdefault("chunks", []).append(c)
        aggs = result.metadata.get("aggs") if isinstance(result.metadata, dict) else None
        if aggs:
            dseen = {d.get("doc_id") for d in kb.get("doc_aggs", [])}
            for d in aggs:
                if d.get("doc_id") in dseen:
                    continue
                dseen.add(d.get("doc_id"))
                kb.setdefault("doc_aggs", []).append(d)

    @staticmethod
    def _normalize(raw: Any) -> ToolResult:
        if isinstance(raw, ToolResult):
            return raw
        if isinstance(raw, dict):
            return ToolResult(
                chunks=raw.get("chunks", []),
                metadata={"aggs": raw.get("doc_aggs", []), "answer": raw.get("answer", "")},
            )
        if isinstance(raw, list):
            return ToolResult(chunks=raw, metadata={})
        return ToolResult(chunks=[], metadata={"raw": str(raw)})


def filter_available_tools(tool_names: list[str], compilation_map: dict[str, set[str]]) -> list[str]:
    """Filter tool list by compilation artifact availability."""
    available = []
    for name in tool_names:
        tool = TOOL_REGISTRY.get(name)
        if not tool:
            continue
        if tool.get("requires_compilation"):
            comp_type = tool.get("compilation_type")
            # ``compilation_type`` may name one artifact or several (a tool that
            # reads either one is available when ANY of them is compiled).
            wanted = {comp_type} if isinstance(comp_type, str) else set(comp_type or ())
            if wanted and not any(wanted & comps for comps in compilation_map.values()):
                continue
        available.append(name)
    return available
