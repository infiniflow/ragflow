from typing import Any, TypedDict
import pluginlib

from .common import PLUGIN_TYPE_LLM_TOOLS


class LLMToolParameter(TypedDict):
    type: str
    description: str
    displayDescription: str
    required: bool


class LLMToolMetadata(TypedDict):
    name: str
    displayName: str
    description: str
    displayDescription: str
    parameters: dict[str, LLMToolParameter]


@pluginlib.Parent(PLUGIN_TYPE_LLM_TOOLS)
class LLMToolPlugin:
    @classmethod
    @pluginlib.abstractmethod
    def get_metadata(cls) -> LLMToolMetadata:
        pass

    def invoke(self, **kwargs) -> str:
        raise NotImplementedError


def llm_tool_metadata_to_openai_tool(llm_tool_metadata: LLMToolMetadata) -> dict[str, Any]:
    return {
        "type": "function",
        "function": {
            "name": llm_tool_metadata["name"],
            "description": llm_tool_metadata["description"],
            "parameters": {
                "type": "object",
                "properties": {
                    k: {
                        "type": p["type"],
                        "description": p["description"]
                    }
                    for k, p in llm_tool_metadata["parameters"].items()
                },
                "required": [k for k, p in llm_tool_metadata["parameters"].items() if p["required"]]
            }
        }
    }
