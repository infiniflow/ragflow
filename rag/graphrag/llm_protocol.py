from typing import Any, Protocol, TypeAlias, runtime_checkable

GraphRAGChatResponse: TypeAlias = str | tuple[str, int]


def unwrap_graphrag_chat_response(response: GraphRAGChatResponse) -> str:
    if isinstance(response, tuple):
        return response[0]
    return response


@runtime_checkable
class GraphRAGCompletionLLM(Protocol):
    """Minimal async chat contract used across GraphRAG components."""

    llm_name: str
    max_length: int

    async def async_chat(
        self,
        system: str,
        history: list[dict[str, Any]],
        gen_conf: dict[str, Any] | None = None,
        **kwargs,
    ) -> GraphRAGChatResponse: ...
