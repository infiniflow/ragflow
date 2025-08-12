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

import logging
import sys
from datetime import datetime

from mcp.client.session import ClientSession
from mcp.client.sse import sse_client

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger(__name__)


async def main():
    start_time = datetime.now()
    logger.info("=" * 60)
    logger.info("🚀 Starting RAGFlow MCP Client")
    logger.info("=" * 60)
    
    # Configuration
    # Direct access: 
    # mcp_server_url = "http://localhost:9384/sse"
    # Via Nginx: 
    mcp_server_url = "http://localhost:81/mcp/sse"

    auth_token = "ragflow-...."
    
    logger.info(f"📡 MCP Server URL: {mcp_server_url}")
    logger.info(f"🔑 Auth Token: {auth_token[:20]}...")
    
    try:
        logger.info("🔌 Attempting to connect to MCP server...")
        
        # To access RAGFlow server in `host` mode, you need to attach `api_key` for each request to indicate identification.
        # async with sse_client("http://localhost:9382/sse", headers={"api_key": "ragflow-IyMGI1ZDhjMTA2ZTExZjBiYTMyMGQ4Zm"}) as streams:
        # Or follow the requirements of OAuth 2.1 Section 5 with Authorization header
        async with sse_client(
            mcp_server_url,
            headers={
                "Authorization": f"Bearer {auth_token}"
            },
        ) as streams:
            logger.info("✅ Successfully connected to MCP server via SSE")
            logger.info(f"📊 Streams established: {len(streams)} streams")

            # async with sse_client("http://localhost:9382/sse") as streams:
            logger.info("🤝 Initializing client session...")
            async with ClientSession(
                streams[0],
                streams[1],
            ) as session:
                logger.info("✅ Client session created successfully")
                
                logger.info("🔧 Initializing session...")
                await session.initialize()
                logger.info("✅ Session initialization completed")
                
                logger.info("🔍 Discovering available tools...")
                tools = await session.list_tools()
                logger.info(f"📋 Found {len(tools.tools)} available tools:")
                for i, tool in enumerate(tools.tools, 1):
                    logger.info(f"   {i}. {tool.name}: {tool.description}")
                
                # Test tool call
                test_dataset_id = "6f00e94672d811f087700242ac160002"
                test_question = "How to install neovim?"
                
                logger.info("🛠️  Calling RAGFlow retrieval tool...")
                logger.info(f"   📚 Dataset ID: {test_dataset_id}")
                logger.info(f"   ❓ Question: {test_question}")
                
                response = await session.call_tool(
                    name="ragflow_retrieval",
                    arguments={
                        "dataset_ids": [test_dataset_id],
                        "document_ids": [],
                        "question": test_question,
                    },
                )
                
                logger.info("✅ Tool call completed successfully")
                logger.info(f"📄 Response type: {type(response).__name__}")
                logger.info(f"📄 Response content: {response.model_dump()}")
                
                # Calculate execution time
                end_time = datetime.now()
                execution_time = (end_time - start_time).total_seconds()
                logger.info(f"⏱️  Total execution time: {execution_time:.2f} seconds")

    except Exception as e:
        logger.error("❌ Error occurred during MCP client execution")
        logger.error(f"🔍 Error type: {type(e).__name__}")
        logger.error(f"💬 Error message: {str(e)}")
        logger.error("📚 Full error details:", exc_info=True)
        raise


if __name__ == "__main__":
    from anyio import run
    
    logger.info("🎯 Starting MCP client application...")
    try:
        run(main)
        logger.info("🎉 MCP client application completed successfully")
    except KeyboardInterrupt:
        logger.info("⏹️  Application interrupted by user")
    except Exception as e:
        logger.error(f"💥 Application failed: {e}")
        sys.exit(1)
