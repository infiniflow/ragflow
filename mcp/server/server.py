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

import json
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import requests
from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import JSONResponse
from starlette.routing import Mount, Route

import mcp.types as types
from mcp.server.lowlevel import Server
from mcp.server.sse import SseServerTransport

BASE_URL = "http://127.0.0.1:9380"
HOST = "127.0.0.1"
PORT = "9382"


class RAGFlowConnector:
    def __init__(self, base_url: str, version="v1"):
        self.base_url = base_url
        self.version = version
        self.api_url = f"{self.base_url}/api/{self.version}"

    def bind_api_key(self, api_key: str):
        self.api_key = api_key
        self.authorization_header = {"Authorization": "{} {}".format("Bearer", self.api_key)}

    def _post(self, path, json=None, stream=False, files=None):
        if not self.api_key:
            return None
        res = requests.post(url=self.api_url + path, json=json, headers=self.authorization_header, stream=stream, files=files)
        return res

    def _get(self, path, params=None, json=None):
        res = requests.get(url=self.api_url + path, params=params, headers=self.authorization_header, json=json)
        return res

    def list_datasets(self, page: int = 1, page_size: int = 1000, orderby: str = "create_time", desc: bool = True, id: str | None = None, name: str | None = None):
        res = self._get("/datasets", {"page": page, "page_size": page_size, "orderby": orderby, "desc": desc, "id": id, "name": name})
        if not res:
            raise Exception([types.TextContent(type="text", text=res.get("Cannot process this operation."))])

        res = res.json()
        if res.get("code") == 0:
            result_list = []
            for data in res["data"]:
                d = {"description": data["description"], "id": data["id"]}
                result_list.append(json.dumps(d, ensure_ascii=False))
            return "\n".join(result_list)
        return ""

    def retrival(
        self, dataset_ids, document_ids=None, question="", page=1, page_size=30, similarity_threshold=0.2, vector_similarity_weight=0.3, top_k=1024, rerank_id: str | None = None, keyword: bool = False
    ):
        if document_ids is None:
            document_ids = []
        data_json = {
            "page": page,
            "page_size": page_size,
            "similarity_threshold": similarity_threshold,
            "vector_similarity_weight": vector_similarity_weight,
            "top_k": top_k,
            "rerank_id": rerank_id,
            "keyword": keyword,
            "question": question,
            "dataset_ids": dataset_ids,
            "document_ids": document_ids,
        }
        # Send a POST request to the backend service (using requests library as an example, actual implementation may vary)
        res = self._post("/retrieval", json=data_json)
        if not res:
            raise Exception([types.TextContent(type="text", text=res.get("Cannot process this operation."))])

        res = res.json()
        if res.get("code") == 0:
            chunks = []
            for chunk_data in res["data"].get("chunks"):
                chunks.append(json.dumps(chunk_data, ensure_ascii=False))
            return [types.TextContent(type="text", text="\n".join(chunks))]
        raise Exception([types.TextContent(type="text", text=res.get("message"))])


class RAGFlowCtx:
    def __init__(self, connector: RAGFlowConnector):
        self.conn = connector


@asynccontextmanager
async def server_lifespan(server: Server) -> AsyncIterator[dict]:
    ctx = RAGFlowCtx(RAGFlowConnector(base_url=BASE_URL))

    try:
        yield {"ragflow_ctx": ctx}
    finally:
        pass


app = Server("ragflow-server", lifespan=server_lifespan)
sse = SseServerTransport("/messages/")


@app.list_tools()
async def list_tools() -> list[types.Tool]:
    ctx = app.request_context
    ragflow_ctx = ctx.lifespan_context["ragflow_ctx"]
    if not ragflow_ctx:
        raise ValueError("Get RAGFlow Context failed")
    connector = ragflow_ctx.conn

    api_key = ctx.session._init_options.capabilities.experimental["headers"]["api_key"]
    if not api_key:
        raise ValueError("RAGFlow API_KEY is required.")
    connector.bind_api_key(api_key)

    dataset_description = connector.list_datasets()

    return [
        types.Tool(
            name="retrival",
            description="Retrieve relevant chunks of given dataset_ids and document_ids(optional) from RAGFlow retrieve interface based on question. Here are the information of all available databases, including description and id for each dataset. If you cannot decide how many dataset is relevant for given question, just dump all datasets id to this function."
            + dataset_description,
            inputSchema={
                "type": "object",
                "properties": {"dataset_ids": {"type": "array", "items": {"type": "string"}}, "documents_ids": {"type": "array", "items": {"type": "string"}}, "question": {"type": "string"}},
                "required": ["dataset_ids", "question"],
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

    api_key = ctx.session._init_options.capabilities.experimental["headers"]["api_key"]
    if not api_key:
        raise ValueError("RAGFlow API_KEY is required.")
    connector.bind_api_key(api_key)

    if name == "ragflow_retrival":
        return connector.retrival(dataset_ids=arguments["dataset_ids"], document_ids=arguments["document_ids"], question=arguments["question"])
    raise ValueError(f"Tool not found: {name}")


async def handle_sse(request):
    async with sse.connect_sse(request.scope, request.receive, request._send) as streams:
        await app.run(streams[0], streams[1], app.create_initialization_options(experimental_capabilities={"headers": dict(request.headers)}))


class AuthMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request, call_next):
        if request.url.path.startswith("/sse") or request.url.path.startswith("/messages"):
            api_key = request.headers.get("api_key")
            if not api_key:
                return JSONResponse({"error": "Missing unauthorization header"}, status_code=401)
        return await call_next(request)


starlette_app = Starlette(
    debug=True,
    routes=[
        Route("/sse", endpoint=handle_sse),
        Mount("/messages/", app=sse.handle_post_message),
    ],
    middleware=[Middleware(AuthMiddleware)],
)


if __name__ == "__main__":
    """
    Launch example:
        uv run mcp/server/server.py --host=127.0.0.1 --port=9382 --base_url=http://127.0.0.1:9380
    """

    import argparse
    import os

    import uvicorn
    from dotenv import load_dotenv

    load_dotenv()

    parser = argparse.ArgumentParser(description="RAGFlow MCP Server, `base_url` and `api_key` are needed.")
    parser.add_argument("--base_url", type=str, default="http://127.0.0.1:9380", help="api_url: http://<host_address>")
    parser.add_argument("--host", type=str, default="127.0.0.1", help="RAGFlow MCP SERVER host")
    parser.add_argument("--port", type=str, default="9382", help="RAGFlow MCP SERVER port")
    args = parser.parse_args()

    BASE_URL = os.environ.get("RAGFLOW_MCP_BASE_URL", args.base_url)
    HOST = os.environ.get("RAGFLOW_MCP_HOST", args.host)
    PORT = os.environ.get("RAGFLOW_MCP_PORT", args.port)

    print(
        r"""
__  __  ____ ____       ____  _____ ______     _______ ____
|  \/  |/ ___|  _ \     / ___|| ____|  _ \ \   / / ____|  _ \
| |\/| | |   | |_) |    \___ \|  _| | |_) \ \ / /|  _| | |_) |
| |  | | |___|  __/      ___) | |___|  _ < \ V / | |___|  _ <
|_|  |_|\____|_|        |____/|_____|_| \_\ \_/  |_____|_| \_\
    """,
        flush=True,
    )
    print(f"MCP host: {HOST}", flush=True)
    print(f"MCP port: {PORT}", flush=True)
    print(f"MCP base_url: {BASE_URL}", flush=True)

    uvicorn.run(
        starlette_app,
        host=HOST,
        port=int(PORT),
    )
