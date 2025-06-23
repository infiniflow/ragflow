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
from functools import wraps

import click
import requests
from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.responses import JSONResponse, Response
from starlette.routing import Mount, Route
from strenum import StrEnum

import mcp.types as types
from mcp.server.lowlevel import Server
from mcp.server.sse import SseServerTransport


class LaunchMode(StrEnum):
    SELF_HOST = "self-host"
    HOST = "host"


BASE_URL = "http://127.0.0.1:9380"
HOST = "127.0.0.1"
PORT = "9382"
HOST_API_KEY = ""
MODE = ""


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

    def retrieval(
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


def with_api_key(required=True):
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            ctx = app.request_context
            ragflow_ctx = ctx.lifespan_context.get("ragflow_ctx")
            if not ragflow_ctx:
                raise ValueError("Get RAGFlow Context failed")

            connector = ragflow_ctx.conn

            if MODE == LaunchMode.HOST:
                headers = ctx.session._init_options.capabilities.experimental.get("headers", {})
                token = None

                # lower case here, because of Starlette conversion
                auth = headers.get("authorization", "")
                if auth.startswith("Bearer "):
                    token = auth.removeprefix("Bearer ").strip()
                elif "api_key" in headers:
                    token = headers["api_key"]

                if required and not token:
                    raise ValueError("RAGFlow API key or Bearer token is required.")

                connector.bind_api_key(token)
            else:
                connector.bind_api_key(HOST_API_KEY)

            return await func(*args, connector=connector, **kwargs)

        return wrapper

    return decorator


@app.list_tools()
@with_api_key(required=True)
async def list_tools(*, connector) -> list[types.Tool]:
    dataset_description = connector.list_datasets()

    return [
        types.Tool(
            name="ragflow_retrieval",
            description="Retrieve relevant chunks from the RAGFlow retrieve interface based on the question, using the specified dataset_ids and optionally document_ids. Below is the list of all available datasets, including their descriptions and IDs. If you're unsure which datasets are relevant to the question, simply pass all dataset IDs to the function."
            + dataset_description,
            inputSchema={
                "type": "object",
                "properties": {
                    "dataset_ids": {
                        "type": "array",
                        "items": {"type": "string"},
                    },
                    "document_ids": {
                        "type": "array",
                        "items": {"type": "string"},
                    },
                    "question": {"type": "string"},
                },
                "required": ["dataset_ids", "question"],
            },
        ),
    ]


@app.call_tool()
@with_api_key(required=True)
async def call_tool(name: str, arguments: dict, *, connector) -> list[types.TextContent | types.ImageContent | types.EmbeddedResource]:
    if name == "ragflow_retrieval":
        document_ids = arguments.get("document_ids", [])
        return connector.retrieval(
            dataset_ids=arguments["dataset_ids"],
            document_ids=document_ids,
            question=arguments["question"],
        )
    raise ValueError(f"Tool not found: {name}")


async def handle_sse(request):
    async with sse.connect_sse(request.scope, request.receive, request._send) as streams:
        await app.run(streams[0], streams[1], app.create_initialization_options(experimental_capabilities={"headers": dict(request.headers)}))
    return Response()


def create_starlette_app():
    middleware = None
    if MODE == LaunchMode.HOST:
        from starlette.types import ASGIApp, Receive, Scope, Send

        class AuthMiddleware:
            def __init__(self, app: ASGIApp):
                self.app = app

            async def __call__(self, scope: Scope, receive: Receive, send: Send):
                if scope["type"] != "http":
                    await self.app(scope, receive, send)
                    return

                path = scope["path"]
                if path.startswith("/messages/") or path.startswith("/sse"):
                    headers = dict(scope["headers"])
                    token = None
                    auth_header = headers.get(b"authorization")
                    if auth_header and auth_header.startswith(b"Bearer "):
                        token = auth_header.removeprefix(b"Bearer ").strip()
                    elif b"api_key" in headers:
                        token = headers[b"api_key"]

                    if not token:
                        response = JSONResponse({"error": "Missing or invalid authorization header"}, status_code=401)
                        await response(scope, receive, send)
                        return

                await self.app(scope, receive, send)

        middleware = [Middleware(AuthMiddleware)]

    return Starlette(
        debug=True,
        routes=[
            Route("/sse", endpoint=handle_sse, methods=["GET"]),
            Mount("/messages/", app=sse.handle_post_message),
        ],
        middleware=middleware,
    )


@click.command()
@click.option("--base-url", type=str, default="http://127.0.0.1:9380", help="API base URL for RAGFlow backend")
@click.option("--host", type=str, default="127.0.0.1", help="Host to bind the RAGFlow MCP server")
@click.option("--port", type=int, default=9382, help="Port to bind the RAGFlow MCP server")
@click.option(
    "--mode",
    type=click.Choice(["self-host", "host"]),
    default="self-host",
    help=("Launch mode:\n  self-host: run MCP for a single tenant (requires --api-key)\n  host: multi-tenant mode, users must provide Authorization headers"),
)
@click.option("--api-key", type=str, default="", help="API key to use when in self-host mode")
def main(base_url, host, port, mode, api_key):
    import os

    import uvicorn
    from dotenv import load_dotenv

    load_dotenv()

    global BASE_URL, HOST, PORT, MODE, HOST_API_KEY
    BASE_URL = os.environ.get("RAGFLOW_MCP_BASE_URL", base_url)
    HOST = os.environ.get("RAGFLOW_MCP_HOST", host)
    PORT = os.environ.get("RAGFLOW_MCP_PORT", str(port))
    MODE = os.environ.get("RAGFLOW_MCP_LAUNCH_MODE", mode)
    HOST_API_KEY = os.environ.get("RAGFLOW_MCP_HOST_API_KEY", api_key)

    if MODE == "self-host" and not HOST_API_KEY:
        raise click.UsageError("--api-key is required when --mode is 'self-host'")

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
    print(f"MCP launch mode: {MODE}", flush=True)
    print(f"MCP host: {HOST}", flush=True)
    print(f"MCP port: {PORT}", flush=True)
    print(f"MCP base_url: {BASE_URL}", flush=True)

    uvicorn.run(
        create_starlette_app(),
        host=HOST,
        port=int(PORT),
    )


if __name__ == "__main__":
    """
    Launch example:
        self-host:
            uv run mcp/server/server.py --host=127.0.0.1 --port=9382 --base-url=http://127.0.0.1:9380 --mode=self-host --api-key=ragflow-xxxxx
        host:
            uv run mcp/server/server.py --host=127.0.0.1 --port=9382 --base-url=http://127.0.0.1:9380 --mode=host
    """
    main()
