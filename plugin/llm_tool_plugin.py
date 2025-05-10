from typing import Any, Literal, TypedDict
import pluginlib

from .common import PLUGIN_TYPE_LLM_TOOLS


class LLMToolParameter(TypedDict):
    type: str
    description: str


class LLMToolParameters(TypedDict):
    type: Literal["object"]
    properties: dict[str, LLMToolParameter]
    required: list[str]


class LLMToolMetadata(TypedDict):
    name: str
    description: str
    parameters: LLMToolParameters


@pluginlib.Parent(PLUGIN_TYPE_LLM_TOOLS)
class LLMToolPlugin:
    @classmethod
    @pluginlib.abstractmethod
    def get_metadata(cls) -> LLMToolMetadata:
        pass

    @pluginlib.abstractmethod
    def invoke(self, arguments: dict[str, Any]) -> str:
        pass


def llm_tool_metadata_to_openai_tool(llm_tool_metadata: LLMToolMetadata) -> dict[str, Any]:
    return {
        "type": "function",
        "function": llm_tool_metadata
    }
