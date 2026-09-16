import ast
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import Mock
from urllib.parse import urlparse

from common.llm_request_context import openai_user_kwargs, reset_llm_request_context, set_llm_request_context


SOURCE = Path(__file__).resolve().parents[4] / "rag/llm/embedding_model.py"
TREE = ast.parse(SOURCE.read_text())
CLASSES = [node for node in TREE.body if isinstance(node, ast.ClassDef) and node.name == "OpenAIEmbed"]
NAMESPACE = {
    "Base": object,
    "openai_user_kwargs": openai_user_kwargs,
    "urlparse": urlparse,
    "_sorted_by_index": lambda items: sorted(items, key=lambda item: item.index),
    "total_token_count_from_response": lambda response: response.usage.total_tokens,
}
exec(compile(ast.Module(body=CLASSES, type_ignores=[]), str(SOURCE), "exec"), NAMESPACE)
OpenAIEmbed = NAMESPACE["OpenAIEmbed"]


class OvhEmbeddingUserTests(unittest.TestCase):
    def setUp(self):
        self.context = set_llm_request_context(session_id="test-session")
        self.embed = object.__new__(OpenAIEmbed)
        self.embed.model_name = "BGE-M3"
        response = SimpleNamespace(
            data=[SimpleNamespace(index=1, embedding=[2.0]), SimpleNamespace(index=0, embedding=[1.0])],
            usage=SimpleNamespace(total_tokens=7),
        )
        self.create = Mock(return_value=response)
        self.embed.client = SimpleNamespace(
            base_url="https://oai.endpoints.kepler.ai.cloud.ovh.net/v1",
            embeddings=SimpleNamespace(create=self.create),
        )

    def tearDown(self):
        reset_llm_request_context(self.context)

    def test_ovh_omits_user_and_preserves_payload_and_result(self):
        vectors, tokens = self.embed._call(["first", "second"])
        self.assertEqual(self.create.call_args.kwargs, {
            "input": ["first", "second"], "model": "BGE-M3", "encoding_format": "float",
        })
        self.assertEqual(vectors, [[1.0], [2.0]])
        self.assertEqual(tokens, 7)

    def test_ovh_hostname_case_port_and_trailing_slash(self):
        self.embed.client.base_url = "https://OAI.ENDPOINTS.KEPLER.AI.CLOUD.OVH.NET:443/v1/"
        self.embed._call(["query"])
        self.assertNotIn("user", self.create.call_args.kwargs)

    def test_other_hosts_keep_context_user(self):
        for url in (
            "https://api.openai.com/v1",
            "https://example.test/v1?host=oai.endpoints.kepler.ai.cloud.ovh.net",
            "https://oai.endpoints.kepler.ai.cloud.ovh.net.example.test/v1",
        ):
            with self.subTest(url=url):
                self.embed.client.base_url = url
                self.embed._call(["query"])
                self.assertEqual(self.create.call_args.kwargs["user"], "test-session")

    def test_extra_body_is_preserved(self):
        self.embed._extra_body = lambda: {"custom": True}
        self.embed._call(["query"])
        self.assertEqual(self.create.call_args.kwargs["extra_body"], {"custom": True})
        self.assertNotIn("user", self.create.call_args.kwargs)

    def test_provider_errors_propagate_without_retry(self):
        self.create.side_effect = RuntimeError("provider error")
        with self.assertRaisesRegex(RuntimeError, "provider error"):
            self.embed._call(["query"])
        self.assertEqual(self.create.call_count, 1)

    def test_no_context_does_not_add_user(self):
        token = set_llm_request_context()
        try:
            self.embed.client.base_url = "https://api.openai.com/v1"
            self.embed._call(["query"])
            self.assertNotIn("user", self.create.call_args.kwargs)
        finally:
            reset_llm_request_context(token)


if __name__ == "__main__":
    unittest.main()
