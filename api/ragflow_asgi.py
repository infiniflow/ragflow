#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

import asyncio
import logging

from api.apps import app
from api.ragflow_init import init_ragflow, stop_event, start_update_progress_thread
from common.mcp_tool_call_conn import shutdown_all_mcp_sessions

# Initialize RAGFlow application
init_ragflow()


@app.before_serving
async def startup():
    """Startup event handler for Quart/ASGI server"""
    start_update_progress_thread()


@app.after_serving
async def shutdown():
    """Shutdown event handler for Quart/ASGI server"""
    logging.info("Shutting down background tasks...")
    shutdown_all_mcp_sessions()
    stop_event.set()
    await asyncio.sleep(1)


# Export the ASGI application
# This is what uvicorn/hypercorn will load
asgi_app = app
