#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from ragflow_sdk import RAGFlow
from starlette.applications import Starlette
from starlette.routing import Mount, Route

import mcp.types as types
from mcp.server.lowlevel import Server
from mcp.server.sse import SseServerTransport

BASE_URL = "http://127.0.0.1:9380"
API_KEY = ""
HOST = "127.0.0.1"
PORT = "9382"


class RAGFlowConnector:
    def __init__(self, base_url: str, api_key: str):
        self.base_url = base_url
        self.api_key = api_key
        self.client = RAGFlow(base_url=base_url, api_key=api_key)

    def ragflow_retrival(self, kb_ids: list[str], document_ids: list[str], question: str) -> list[types.TextContent]:
        try:
            chunks = []
            for c in self.client.retrieve(dataset_ids=kb_ids, document_ids=document_ids, question=question):
                chunks.append(str(c.to_json()))
            return [types.TextContent(type="text", text="\n".join(chunks))]
        except Exception:
            return [types.TextContent(type="text", text="")]


class RAGFlowCtx:
    def __init__(self, connector: RAGFlowConnector):
        self.conn = connector


@asynccontextmanager
async def server_lifespan(server: Server) -> AsyncIterator[dict]:
    ctx = RAGFlowCtx(RAGFlowConnector(base_url=BASE_URL, api_key=API_KEY))

    try:
        yield {"ragflow_ctx": ctx}
    finally:
        pass


app = Server("ragflow-server", lifespan=server_lifespan)
sse = SseServerTransport("/messages/")


@app.list_tools()
async def list_tools() -> list[types.Tool]:
    return [
        types.Tool(
            name="calculate_sum", description="Add two numbers together", inputSchema={"type": "object", "properties": {"a": {"type": "number"}, "b": {"type": "number"}}, "required": ["a", "b"]}
        ),
        types.Tool(
            name="ragflow_retrival",
            description="Retrive relavant chunks of given kb_ids and document_ids(optional) from RAGFlow retrave interface based on question.",
            inputSchema={
                "type": "object",
                "properties": {"kb_ids": {"type": "array", "items": {"type": "string"}}, "documents_ids": {"type": "array", "items": {"type": "string"}}, "question": {"type": "string"}},
                "required": ["kb_ids", "question"],
            },
        ),
    ]


@app.call_tool()
async def call_tool(name: str, arguments: dict) -> list[types.TextContent | types.ImageContent | types.EmbeddedResource]:
    ctx = app.request_context
    ragflow_ctx = ctx.lifespan_context["ragflow_ctx"]
    if not ragflow_ctx:
        raise ValueError("Get RAGFlow Context failed")
    connector = ragflow_ctx.conn
    if name == "ragflow_retrival":
        return connector.ragflow_retrival(kb_ids=arguments["kb_ids"], document_ids=arguments["document_ids"], question=arguments["question"])
    raise ValueError(f"Tool not found: {name}")


async def handle_sse(request):
    async with sse.connect_sse(request.scope, request.receive, request._send) as streams:
        await app.run(streams[0], streams[1], app.create_initialization_options())


starlette_app = Starlette(
    routes=[
        Route("/sse", endpoint=handle_sse),
        Mount("/messages/", app=sse.handle_post_message),
    ]
)


if __name__ == "__main__":
    """
    Launch example:
        uv run mcp/server/server.py --host=127.0.0.1 --port=9382 --base_url=http://127.0.0.1:9380 --api_key=ragflow-xxxxxxxxxxxx
    """

    import argparse
    import os

    import uvicorn
    from dotenv import load_dotenv

    load_dotenv()

    parser = argparse.ArgumentParser(description="RAGFlow MCP Server, `base_url` and `api_key` are needed.")
    parser.add_argument("--base_url", type=str, default="http://127.0.0.1:9380", help="api_url: http://<host_address>")
    parser.add_argument("--api_key", type=str, help="RAGFlow api_key")
    parser.add_argument("--host", type=str, default="127.0.0.1", help="RAGFlow MCP SERVER host")
    parser.add_argument("--port", type=str, default="9382", help="RAGFlow MCP SERVER port")
    args = parser.parse_args()

    BASE_URL = os.environ.get("RAGFLOW_MCP_BASE_URL", args.base_url)
    API_KEY = os.environ.get("RAGFLOW_API_KEY_FOR_MCP", args.api_key)
    HOST = os.environ.get("RAGFLOW_MCP_HOST", args.host)
    PORT = os.environ.get("RAGFLOW_MCP_PORT", args.port)

    uvicorn.run(starlette_app, host=HOST, port=int(PORT))
