"""Tool registry: all available tools with metadata and function schemas."""

from typing import Any

# Tool registry: tool_name -> {metadata, function_schema, fn}
# 'fn' filled at registration time; schema used for LLM tool definitions.
TOOL_REGISTRY: dict[str, dict[str, Any]] = {}

# Executor interface
# Each tool registers a callable with signature:
#   async def fn(tools, **kwargs) -> dict  # {"chunks": [...], ...}


def register_tool(name: str, schema: dict, fn: callable, requires_compilation: bool = False, compilation_type: str | tuple[str, ...] | None = None, processing_time: str = "fast") -> None:
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


# Common schema builders


def _search_schema(name: str, desc: str) -> dict:
    return {
        "type": "function",
        "function": {
            "name": name,
            "description": desc,
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "the original user's question."},
                    "keywords": {"type": "string", "description": "the keywords used for searching split by space or ','."},
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
                    "topic": {"type": "string", "description": "the topic to navigate to."},
                    "keywords": {"type": "string", "description": "the keywords used for searching split by space or ','."},
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
            "description": "Internal reasoning. Analyze the collected results and plan the next step. Do not output final user-facing content while reasoning.",
            "parameters": {
                "type": "object",
                "properties": {
                    "reasoning": {
                        "type": "string",
                        "description": "Reasoning content: what has been found, what is still missing, and what to do next.",
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
            "description": "Call when the research is complete. Output the research report and claim-level verification results.",
            "parameters": {
                "type": "object",
                "properties": {
                    "report": {"type": "string", "description": "Research result report, factual and unformatted."},
                    "is_verified": {"type": "boolean", "description": "Whether sufficient evidence was found."},
                    "confidence": {"type": "number", "description": "Confidence from 0 to 1."},
                    "evidence_ids": {
                        "type": "array",
                        "items": {"type": "integer"},
                        "description": "Referenced chunk IDs.",
                    },
                    "gaps": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Information that was not found.",
                    },
                    "discovered_claims": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "New research directions discovered during research.",
                    },
                },
                "required": ["report", "is_verified", "confidence"],
            },
        },
    }
