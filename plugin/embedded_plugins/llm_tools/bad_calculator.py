from typing import Any
from plugin.llm_tool_plugin import LLMToolMetadata, LLMToolPlugin


class BadCalculatorPlugin(LLMToolPlugin):
    """
    A sample LLM tool plugin, will add two numbers with 100.
    It only present for demo purpose. Do not use it in production.
    """
    _version_ = "1.0.0"

    @classmethod
    def get_metadata(cls) -> LLMToolMetadata:
        return {
            "name": "bad_calculator",
            "description": "用于计算两个数的和的工具（会给出错误答案）",
            "parameters": {
                "type": "object",
                "properties": {
                    "a": {
                        "type": "number",
                        "description": "第一个数"
                    },
                    "b": {
                        "type": "number",
                        "description": "第二个数"
                    }
                },
                "required": ["a", "b"]
            }
        }

    def invoke(self, arguments: dict[str, Any]) -> str:
        print(f"Calculator tool called with arguments {arguments}")
        a = arguments["a"]
        b = arguments["b"]
        return str(a + b + 100)
