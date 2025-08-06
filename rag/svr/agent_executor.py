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

import os
import json
import logging
import signal
import sys
import threading
import time
from datetime import datetime

import trio

from agent.canvas import Canvas
from api.db.services.canvas_service import UserCanvasService
from api.db.services.schedule_agent_service import ScheduleAgentService
from api.utils.log_utils import init_root_logger
from api.versions import get_ragflow_version
from api import settings
from api.db.db_models import ScheduleAgent, close_connection
from rag.utils.redis_conn import REDIS_CONN, RedisDistributedLock

# Configuration
CONSUMER_NO = "0" if len(sys.argv) < 2 else sys.argv[1]
CONSUMER_NAME = f"agent_executor_{CONSUMER_NO}"
BOOT_AT = datetime.now().astimezone().isoformat(timespec="milliseconds")
MAX_CONCURRENT_AGENTS = int(os.environ.get("MAX_CONCURRENT_AGENTS", "3"))
WORKER_HEARTBEAT_TIMEOUT = int(os.environ.get("WORKER_HEARTBEAT_TIMEOUT", "120"))
CHECK_INTERVAL = 30

# Global state
DONE_AGENTS = 0
FAILED_AGENTS = 0
agent_limiter = trio.CapacityLimiter(MAX_CONCURRENT_AGENTS)
stop_event = threading.Event()


def signal_handler(sig, frame):
    logging.info("Received interrupt signal, shutting down...")
    stop_event.set()
    time.sleep(1)
    sys.exit(0)


async def execute_scheduled_agent(schedule: ScheduleAgent):
    """Execute a single scheduled agent"""
    global DONE_AGENTS, FAILED_AGENTS
    
    start_time = time.time()
    run_id = ScheduleAgentService.start_execution(schedule.id)
    conversation_id = None
    
    try:
        logging.info(f"Executing schedule {schedule.id}: {schedule.name}")
        
        # Get and validate canvas
        success, canvas_obj = UserCanvasService.get_by_id(schedule.canvas_id)
        if not success:
            raise Exception(f"Canvas not found: {schedule.canvas_id}")
        
        # Prepare canvas DSL
        if not isinstance(canvas_obj.dsl, str):
            canvas_obj.dsl = json.dumps(canvas_obj.dsl, ensure_ascii=False)
        
        # Execute canvas
        async with agent_limiter:
            canvas = Canvas(canvas_obj.dsl, schedule.tenant_id)
            canvas.reset()
            
            for ans in canvas.run(stream=False):
                pass
        
        # Save results
        canvas_obj.dsl = json.loads(str(canvas))
        conversation_id = await _create_conversation_record(schedule, canvas)
        
        # Mark as successful
        ScheduleAgentService.finish_execution(run_id, success=True, conversation_id=conversation_id)
        DONE_AGENTS += 1
        
        execution_time = time.time() - start_time
        logging.info(f"Schedule {schedule.id} completed successfully in {execution_time:.2f}s")
        
    except Exception as e:
        FAILED_AGENTS += 1
        execution_time = time.time() - start_time
        
        logging.error(f"Schedule {schedule.id} failed after {execution_time:.2f}s: {e}")
        
        try:
            ScheduleAgentService.finish_execution(run_id, success=False, error_message=str(e), conversation_id=conversation_id)
        except Exception as update_error:
            logging.error(f"Failed to update schedule {schedule.id} after failure: {update_error}")
            
    finally:
        close_connection()


async def _create_conversation_record(schedule, canvas):
    """Create conversation record for executed schedule"""
    try:
        from api.utils import get_uuid
        conversation_id = get_uuid()
        # Add conversation creation logic here if needed
        return conversation_id
    except Exception as e:
        logging.error(f"Error creating conversation record: {e}")
        return None


async def check_and_execute_schedules():
    """Check for pending schedules and execute them"""
    try:
        pending_schedules = ScheduleAgentService.get_pending_schedules()
        
        if not pending_schedules:
            logging.debug("No pending schedules found")
            return
        
        logging.info(f"Found {len(pending_schedules)} pending schedules")
        
        async with trio.open_nursery() as nursery:
            for schedule in pending_schedules:
                nursery.start_soon(execute_scheduled_agent, schedule)
                
    except Exception as e:
        logging.error(f"Error in schedule check cycle: {e}")


async def report_status():
    """Report worker status to Redis"""
    global CONSUMER_NAME, BOOT_AT, DONE_AGENTS, FAILED_AGENTS
    
    REDIS_CONN.sadd("AGENTEXE", CONSUMER_NAME)
    redis_lock = RedisDistributedLock("clean_agent_executor", lock_value=CONSUMER_NAME, timeout=60)
    
    while not stop_event.is_set():
        try:
            now = datetime.now()
            
            # Send heartbeat
            heartbeat = json.dumps({
                "name": CONSUMER_NAME,
                "now": now.astimezone().isoformat(timespec="milliseconds"),
                "boot_at": BOOT_AT,
                "done": DONE_AGENTS,
                "failed": FAILED_AGENTS,
            })
            
            REDIS_CONN.zadd(CONSUMER_NAME, heartbeat, now.timestamp())
            
            # Clean expired entries
            expired = REDIS_CONN.zcount(CONSUMER_NAME, 0, now.timestamp() - 1800)  # 30 minutes
            if expired > 0:
                REDIS_CONN.zpopmin(CONSUMER_NAME, expired)
            
            # Clean expired workers
            if redis_lock.acquire():
                _clean_expired_workers(now)
                        
        except Exception as e:
            logging.error(f"Error in status reporting: {e}")
        finally:
            redis_lock.release()
            
        await trio.sleep(30)


def _clean_expired_workers(now):
    """Clean expired agent executors from Redis"""
    try:
        agent_executors = REDIS_CONN.smembers("AGENTEXE")
        
        for consumer_name in agent_executors:
            if consumer_name == CONSUMER_NAME:
                continue
                
            expired = REDIS_CONN.zcount(
                consumer_name, 
                now.timestamp() - WORKER_HEARTBEAT_TIMEOUT, 
                now.timestamp() + 10
            )
            
            if expired == 0:
                REDIS_CONN.srem("AGENTEXE", consumer_name)
                REDIS_CONN.delete(consumer_name)
                logging.info(f"Cleaned expired worker: {consumer_name}")
                
    except Exception as e:
        logging.error(f"Error cleaning expired workers: {e}")


async def main():
    """Main execution loop"""
    logging.info(f"""
   ___                   __     ______                     __
  / _ | ___  ___ ___  __ / /_   / ____/  _____  _______  __/ /_____  _____
 / __ |/ _ \/ -_) _ \/ // __/  / __/ | |/_/ _ \/ ___/ / / / __/ __ \/ ___/
/_/ |_|\_, /\__/_//_/\_,_\__/  / /____>  </  __/ /__/ /_/ / /_/ /_/ / /
      /___/                  /_____/_/|_|\___/\___/\__,_/\__/\____/_/

AgentExecutor v{get_ragflow_version()} - {CONSUMER_NAME}
Boot time: {BOOT_AT}
Max concurrent agents: {MAX_CONCURRENT_AGENTS}
""")
    
    settings.init_settings()
    
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    async with trio.open_nursery() as nursery:
        nursery.start_soon(report_status)
        
        cycle_count = 0
        while not stop_event.is_set():
            cycle_count += 1
            logging.debug(f"Schedule check cycle #{cycle_count}")
            nursery.start_soon(check_and_execute_schedules)
            await trio.sleep(CHECK_INTERVAL)


if __name__ == "__main__":
    init_root_logger(CONSUMER_NAME)
    trio.run(main)
