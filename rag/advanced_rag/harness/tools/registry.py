"""Tool registry — all available tools with metadata and function schemas."""

from typing import Any

# Tool registry: tool_name → {metadata, function_schema, fn}
# 'fn' filled at registration time; schema used for LLM tool definitions.
TOOL_REGISTRY: dict[str, dict[str, Any]] = {}

# ── Executor interface ──
# Each tool registers a callable with signature:
#   async def fn(tools, **kwargs) -> dict  # {"chunks": [...], ...}


def register_tool(name: str, schema: dict, fn: callable, requires_compilation: bool = False, compilation_type: str | None = None, processing_time: str = "fast") -> None:
    TOOL_REGISTRY[name] = {
        "name": name,
        "function_schema": schema,
        "fn": fn,
        "requires_compilation": requires_compilation,
        "compilation_type": compilation_type,
        "processing_time": processing_time,
    }


def get_tool(tool_name: str) -> dict | None:
    return TOOL_REGISTRY.get(tool_name)


def get_function_schemas(tool_names: list[str]) -> list[dict]:
    """Return function schemas for the given tool names, if registered."""
    return [TOOL_REGISTRY[n]["function_schema"] for n in tool_names if n in TOOL_REGISTRY]


# ── Common schema builders ──


def _search_schema(name: str, desc: str) -> dict:
    return {
        "type": "function",
        "function": {
            "name": name,
            "description": desc,
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "搜索查询"},
                },
                "required": ["query"],
            },
        },
    }


def _navigate_schema(name: str, desc: str) -> dict:
    return {
        "type": "function",
        "function": {
            "name": name,
            "description": desc,
            "parameters": {
                "type": "object",
                "properties": {
                    "topic": {"type": "string", "description": "要定位的主题"},
                },
                "required": ["topic"],
            },
        },
    }


def _inspector_schema(name: str, desc: str, props: dict = None) -> dict:
    schema = {
        "type": "function",
        "function": {
            "name": name,
            "description": desc,
            "parameters": {
                "type": "object",
                "properties": props
                or {
                    "chunk_id": {"type": "string", "description": "chunk ID"},
                },
                "required": list((props or {"chunk_id": {}}).keys()),
            },
        },
    }
    return schema


def _think_schema() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "think_tool",
            "description": "内部推理。分析已收集的结果，规划下一步。不要在推理时输出物化内容，只做推理。",
            "parameters": {
                "type": "object",
                "properties": {
                    "reasoning": {
                        "type": "string",
                        "description": "推理内容——分析已找到什么、还缺什么、下一步怎么做",
                    },
                },
                "required": ["reasoning"],
            },
        },
    }


def _generate_report_schema() -> dict:
    return {
        "type": "function",
        "function": {
            "name": "generate_report",
            "description": "研究完成时调用。输出研究报告和声明级验证结果。",
            "parameters": {
                "type": "object",
                "properties": {
                    "report": {"type": "string", "description": "研究结果报告（事实性，无格式）"},
                    "is_verified": {"type": "boolean", "description": "是否找到充分证据"},
                    "confidence": {"type": "number", "description": "0-1 置信度"},
                    "evidence_ids": {
                        "type": "array",
                        "items": {"type": "integer"},
                        "description": "引用的 chunk IDs",
                    },
                    "gaps": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "未找到的信息",
                    },
                    "discovered_claims": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "研究中发现的新研究方向",
                    },
                },
                "required": ["report", "is_verified", "confidence"],
            },
        },
    }
