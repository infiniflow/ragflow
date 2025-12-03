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

"""
Agent Trace API Endpoints

Provides REST API endpoints for accessing agent execution traces,
including trace retrieval, filtering, analysis, and management.
This addresses Issue #10081: Add Trace Logging for Agent Completions API.
"""

from datetime import datetime
from quart import request, Response
import json

from api.apps import login_required, current_user
from api.db.services.trace_service import TraceService
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from common.constants import RetCode


@manager.route('/traces', methods=['GET'])  # noqa: F821
@login_required
async def list_traces():
    """
    List trace sessions for the current tenant.
    
    Query parameters:
    - agent_id: Filter by agent ID (optional)
    - user_id: Filter by user ID (optional)
    - status: Filter by status (running, completed, failed) (optional)
    - start_time: Filter by start time ISO format (optional)
    - end_time: Filter by end time ISO format (optional)
    - page: Page number (default: 1)
    - page_size: Items per page (default: 20)
    
    Returns:
        Paginated list of trace sessions
    """
    try:
        agent_id = request.args.get("agent_id")
        user_id = request.args.get("user_id")
        status = request.args.get("status")
        start_time_str = request.args.get("start_time")
        end_time_str = request.args.get("end_time")
        page = int(request.args.get("page", 1))
        page_size = int(request.args.get("page_size", 20))
        
        start_time = None
        end_time = None
        if start_time_str:
            start_time = datetime.fromisoformat(start_time_str)
        if end_time_str:
            end_time = datetime.fromisoformat(end_time_str)
        
        result = TraceService.list_traces(
            tenant_id=current_user.id,
            agent_id=agent_id,
            user_id=user_id,
            status=status,
            start_time=start_time,
            end_time=end_time,
            page=page,
            page_size=page_size,
        )
        
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@manager.route('/traces/<task_id>', methods=['GET'])  # noqa: F821
@login_required
async def get_trace(task_id):
    """
    Get a specific trace session by task ID.
    
    Path parameters:
    - task_id: The task/trace session ID
    
    Query parameters:
    - format: Output format (streaming, compact, detailed) (default: streaming)
    
    Returns:
        Trace session data
    """
    try:
        format_type = request.args.get("format", "streaming")
        
        result = TraceService.format_trace(task_id, format_type)
        
        if not result:
            return get_data_error_result(
                message="Trace session not found",
                code=RetCode.DATA_ERROR
            )
        
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@manager.route('/traces/<task_id>/events', methods=['GET'])  # noqa: F821
@login_required
async def get_trace_events(task_id):
    """
    Get trace events for a specific session.
    
    Path parameters:
    - task_id: The task/trace session ID
    
    Query parameters:
    - event_types: Comma-separated list of event types to filter (optional)
    - component_id: Filter by component ID (optional)
    - limit: Maximum number of events (default: 100)
    - offset: Number of events to skip (default: 0)
    
    Returns:
        List of trace events
    """
    try:
        event_types_str = request.args.get("event_types")
        event_types = event_types_str.split(",") if event_types_str else None
        component_id = request.args.get("component_id")
        limit = int(request.args.get("limit", 100))
        offset = int(request.args.get("offset", 0))
        
        events = TraceService.get_trace_events(
            task_id=task_id,
            event_types=event_types,
            component_id=component_id,
            limit=limit,
            offset=offset,
        )
        
        return get_json_result(data={"events": events, "count": len(events)})
    except Exception as e:
        return server_error_response(e)


@manager.route('/traces/<task_id>/summary', methods=['GET'])  # noqa: F821
@login_required
async def get_trace_summary(task_id):
    """
    Get a summary of a trace session.
    
    Path parameters:
    - task_id: The task/trace session ID
    
    Returns:
        Trace session summary
    """
    try:
        summary = TraceService.get_trace_summary(task_id)
        
        if not summary:
            return get_data_error_result(
                message="Trace session not found",
                code=RetCode.DATA_ERROR
            )
        
        return get_json_result(data=summary)
    except Exception as e:
        return server_error_response(e)


@manager.route('/traces/<task_id>/analysis', methods=['GET'])  # noqa: F821
@login_required
async def analyze_trace(task_id):
    """
    Analyze a trace session and get insights.
    
    Path parameters:
    - task_id: The task/trace session ID
    
    Returns:
        Analysis results including bottlenecks, errors, and recommendations
    """
    try:
        analysis = TraceService.analyze_trace(task_id)
        
        if not analysis:
            return get_data_error_result(
                message="Trace session not found or analysis failed",
                code=RetCode.DATA_ERROR
            )
        
        return get_json_result(data=analysis)
    except Exception as e:
        return server_error_response(e)


@manager.route('/traces/<task_id>', methods=['DELETE'])  # noqa: F821
@login_required
async def delete_trace(task_id):
    """
    Delete a trace session.
    
    Path parameters:
    - task_id: The task/trace session ID
    
    Returns:
        Success status
    """
    try:
        success, message = TraceService.delete_trace(task_id)
        
        if not success:
            return get_data_error_result(message=message)
        
        return get_json_result(data={"task_id": task_id, "message": message})
    except Exception as e:
        return server_error_response(e)


@manager.route('/traces/cleanup', methods=['POST'])  # noqa: F821
@login_required
async def cleanup_traces():
    """
    Clean up old trace sessions.
    
    Request body:
    {
        "days": 7  // Number of days to keep traces (default: 7)
    }
    
    Returns:
        Number of deleted traces
    """
    try:
        req = await get_request_json()
        days = req.get("days", 7)
        
        deleted, message = TraceService.cleanup_old_traces(days)
        
        return get_json_result(data={"deleted": deleted, "message": message})
    except Exception as e:
        return server_error_response(e)


@manager.route('/traces/<task_id>/stream', methods=['GET'])  # noqa: F821
@login_required
async def stream_trace(task_id):
    """
    Stream trace events in real-time using Server-Sent Events.
    
    Path parameters:
    - task_id: The task/trace session ID
    
    Query parameters:
    - format: Output format (streaming, compact, detailed) (default: streaming)
    
    Returns:
        SSE stream of trace events
    """
    try:
        format_type = request.args.get("format", "streaming")
        
        from agent.trace.trace_collector import get_trace_collector
        from agent.trace.trace_formatter import TraceFormatterFactory
        
        collector = get_trace_collector(task_id)
        
        if not collector:
            return get_data_error_result(
                message="Active trace session not found",
                code=RetCode.DATA_ERROR
            )
        
        formatter = TraceFormatterFactory.create(format_type)
        
        async def generate():
            import asyncio
            
            for event in collector.get_events():
                yield formatter.format_for_stream(event)
            
            event_queue = []
            
            def on_event(event):
                event_queue.append(event)
            
            collector.subscribe(on_event)
            
            try:
                while collector._is_active:
                    while event_queue:
                        event = event_queue.pop(0)
                        yield formatter.format_for_stream(event)
                    await asyncio.sleep(0.1)
            finally:
                collector.unsubscribe(on_event)
            
            yield "data:[DONE]\n\n"
        
        resp = Response(generate(), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp
    except Exception as e:
        return server_error_response(e)


@manager.route('/agents/<agent_id>/traces', methods=['GET'])  # noqa: F821
@login_required
async def list_agent_traces(agent_id):
    """
    List trace sessions for a specific agent.
    
    Path parameters:
    - agent_id: The agent ID
    
    Query parameters:
    - status: Filter by status (optional)
    - page: Page number (default: 1)
    - page_size: Items per page (default: 20)
    
    Returns:
        Paginated list of trace sessions for the agent
    """
    try:
        status = request.args.get("status")
        page = int(request.args.get("page", 1))
        page_size = int(request.args.get("page_size", 20))
        
        result = TraceService.list_traces(
            tenant_id=current_user.id,
            agent_id=agent_id,
            status=status,
            page=page,
            page_size=page_size,
        )
        
        return get_json_result(data=result)
    except Exception as e:
        return server_error_response(e)


@manager.route('/agents/<agent_id>/completions/trace', methods=['POST'])  # noqa: F821
@login_required
@validate_request("question")
async def agent_completion_with_trace(agent_id):
    """
    Execute agent completion with trace logging enabled.
    
    This endpoint is similar to /agents/<agent_id>/completions but includes
    trace information in the response, addressing Issue #10081.
    
    Path parameters:
    - agent_id: The agent ID
    
    Request body:
    {
        "question": "User question",
        "session_id": "Optional session ID",
        "stream": true,
        "trace_level": "standard",  // minimal, standard, detailed, debug
        "include_trace": true
    }
    
    Returns:
        Agent response with trace information
    """
    try:
        from api.db.services.canvas_service import completion as agent_completion
        
        req = await get_request_json()
        stream = req.get("stream", True)
        trace_level = req.get("trace_level", "standard")
        include_trace = req.get("include_trace", True)
        
        from common.misc_utils import get_uuid
        task_id = get_uuid()
        
        if include_trace:
            success, trace_id = TraceService.create_trace_session(
                task_id=task_id,
                agent_id=agent_id,
                session_id=req.get("session_id", ""),
                user_id=current_user.id,
                tenant_id=current_user.id,
                trace_level=trace_level,
            )
        
        if stream:
            async def generate():
                full_content = ""
                reference = {}
                
                async for answer in agent_completion(
                    tenant_id=current_user.id,
                    agent_id=agent_id,
                    **req
                ):
                    try:
                        ans = json.loads(answer[5:])
                        
                        if ans["event"] == "message":
                            full_content += ans["data"]["content"]
                        
                        if ans.get("data", {}).get("reference"):
                            reference.update(ans["data"]["reference"])
                        
                        yield answer
                    except Exception:
                        continue
                
                if include_trace:
                    TraceService.save_trace_session(task_id)
                    trace_data = TraceService.format_trace(task_id, "compact")
                    yield f"data:{json.dumps({'event': 'trace', 'data': trace_data}, ensure_ascii=False)}\n\n"
                
                yield "data:[DONE]\n\n"
            
            resp = Response(generate(), mimetype="text/event-stream")
            resp.headers.add_header("Cache-control", "no-cache")
            resp.headers.add_header("Connection", "keep-alive")
            resp.headers.add_header("X-Accel-Buffering", "no")
            resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
            return resp
        
        full_content = ""
        reference = {}
        final_ans = None
        
        async for answer in agent_completion(
            tenant_id=current_user.id,
            agent_id=agent_id,
            **req
        ):
            try:
                ans = json.loads(answer[5:])
                
                if ans["event"] == "message":
                    full_content += ans["data"]["content"]
                
                if ans.get("data", {}).get("reference"):
                    reference.update(ans["data"]["reference"])
                
                final_ans = ans
            except Exception:
                continue
        
        if final_ans:
            final_ans["data"]["content"] = full_content
            final_ans["data"]["reference"] = reference
        
        if include_trace:
            TraceService.save_trace_session(task_id)
            trace_data = TraceService.format_trace(task_id, "compact")
            if final_ans:
                final_ans["trace"] = trace_data
        
        return get_json_result(data=final_ans)
    except Exception as e:
        return server_error_response(e)
