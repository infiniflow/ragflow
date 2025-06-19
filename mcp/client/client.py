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


from mcp.client.session import ClientSession
from mcp.client.sse import sse_client


async def main():
    try:
        # To access RAGFlow server in `host` mode, you need to attach `api_key` for each request to indicate identification.
        # async with sse_client("http://localhost:9382/sse", headers={"api_key": "ragflow-IyMGI1ZDhjMTA2ZTExZjBiYTMyMGQ4Zm"}) as streams:
        # Or follow the requirements of OAuth 2.1 Section 5 with Authorization header
        # async with sse_client("http://localhost:9382/sse", headers={"Authorization": "Bearer ragflow-IyMGI1ZDhjMTA2ZTExZjBiYTMyMGQ4Zm"}) as streams:

        async with sse_client("http://localhost:9382/sse") as streams:
            async with ClientSession(
                streams[0],
                streams[1],
            ) as session:
                await session.initialize()
                tools = await session.list_tools()
                print(f"{tools.tools=}")
                response = await session.call_tool(name="ragflow_retrieval", arguments={"dataset_ids": ["ce3bb17cf27a11efa69751e139332ced"], "document_ids": [], "question": "How to install neovim?"})
                print(f"Tool response: {response.model_dump()}")

    except Exception as e:
        print(e)


if __name__ == "__main__":
    from anyio import run

    run(main)
