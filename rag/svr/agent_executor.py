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
import traceback
from datetime import datetime

import trio

from agent.canvas import Canvas
from api.db.services.canvas_service import UserCanvasService
from api.db.services.schedule_agent_service import ScheduleAgentService
from api.utils.log_utils import initRootLogger
from api.versions import get_ragflow_version
from api import settings
from api.db.db_models import ScheduleAgent, close_connection
from rag.utils.redis_conn import REDIS_CONN, RedisDistributedLock

CONSUMER_NO = "0" if len(sys.argv) < 2 else sys.argv[1]
CONSUMER_NAME = "agent_executor_" + CONSUMER_NO
BOOT_AT = datetime.now().astimezone().isoformat(timespec="milliseconds")
DONE_AGENTS = 0
FAILED_AGENTS = 0

MAX_CONCURRENT_AGENTS = int(os.environ.get("MAX_CONCURRENT_AGENTS", "3"))
agent_limiter = trio.CapacityLimiter(MAX_CONCURRENT_AGENTS)
WORKER_HEARTBEAT_TIMEOUT = int(os.environ.get("WORKER_HEARTBEAT_TIMEOUT", "120"))

stop_event = threading.Event()


def signal_handler(sig, frame):
    logging.info("Received interrupt signal, shutting down...")
    stop_event.set()
    time.sleep(1)
    sys.exit(0)


async def execute_scheduled_agent(schedule: ScheduleAgent):
    global DONE_AGENTS, FAILED_AGENTS
    
    start_time = time.time()
    schedule_info = f"ID:{schedule.id}, Name:{schedule.name}, Canvas:{schedule.canvas_id}, Frequency:{schedule.frequency_type}"
    
    try:
        logging.info(f"[AGENT_EXEC] Starting execution of scheduled agent - {schedule_info}")
        
        # Get canvas
        logging.debug(f"[AGENT_EXEC] Fetching canvas {schedule.canvas_id} for schedule {schedule.id}")
        success, canvas_obj = UserCanvasService.get_by_id(schedule.canvas_id)
        if not success:
            raise Exception(f"Canvas not found: {schedule.canvas_id}")
        
        logging.info(f"[AGENT_EXEC] Canvas {schedule.canvas_id} found successfully - Title: {getattr(canvas_obj, 'title', 'Unknown')}")
        
        # Create canvas instance following create_agent_session pattern
        if not isinstance(canvas_obj.dsl, str):
            canvas_obj.dsl = json.dumps(canvas_obj.dsl, ensure_ascii=False)
        
        logging.debug(f"[AGENT_EXEC] Creating canvas instance for schedule {schedule.id}")
        canvas = Canvas(canvas_obj.dsl, schedule.tenant_id)
        canvas.reset()
        
       
        # Run canvas with preset parameters (similar to create_agent_session)
        logging.info(f"[AGENT_EXEC] Running canvas initialization for schedule {schedule.id}")
        async with agent_limiter:
            for ans in canvas.run(stream=False):
                pass
        
        # Update canvas DSL after initialization
        canvas_obj.dsl = json.loads(str(canvas))
        
        # Create conversation record (similar to create_agent_session)
        from api.utils import get_uuid
        from api.db.services.api_service import API4ConversationService
        
        conv_id = get_uuid()
        
        logging.debug(f"[AGENT_EXEC] Saving conversation for schedule {schedule.id}")
        
        execution_time = time.time() - start_time
        logging.info(f"[AGENT_EXEC] Canvas execution completed for schedule {schedule.id} in {execution_time:.2f}s")
        
        # Update schedule after successful run
        logging.debug(f"[AGENT_EXEC] Updating schedule {schedule.id} after successful execution")
        ScheduleAgentService.update_after_run(schedule.id, success=True)
        DONE_AGENTS += 1
        
        total_time = time.time() - start_time
        frequency_info = f" (frequency: {schedule.frequency_type}"
        if schedule.frequency_type == 'weekly':
            frequency_info += f", days: {schedule.days_of_week}"
        elif schedule.frequency_type == 'monthly':
            frequency_info += f", day: {schedule.day_of_month}"
        frequency_info += ")"
        
        logging.info(f"[AGENT_EXEC] ✅ Successfully executed scheduled agent: {schedule_info}{frequency_info}")
        logging.info(f"[AGENT_EXEC] Execution results - Conversation ID: {conv_id}, Total time: {total_time:.2f}s")
        
    except Exception as e:
        FAILED_AGENTS += 1
        total_time = time.time() - start_time
        
        # Log detailed error information
        logging.error(f"[AGENT_EXEC] ❌ Failed to execute scheduled agent: {schedule_info}")
        logging.error(f"[AGENT_EXEC] Error type: {type(e).__name__}")
        logging.error(f"[AGENT_EXEC] Error message: {str(e)}")
        logging.error(f"[AGENT_EXEC] Execution time before failure: {total_time:.2f}s")
        
        # Log stack trace for debugging
        logging.debug(f"[AGENT_EXEC] Full stack trace for schedule {schedule.id}:")
        logging.debug(traceback.format_exc())
        
        try:
            ScheduleAgentService.update_after_run(schedule.id, success=False)
            logging.debug(f"[AGENT_EXEC] Updated schedule {schedule.id} status after failure")
        except Exception as update_error:
            logging.error(f"[AGENT_EXEC] Failed to update schedule {schedule.id} after execution failure: {update_error}")
        
        logging.exception(f"[AGENT_EXEC] Exception details for schedule {schedule.id}: {e}")
    finally:
        close_connection()
        total_time = time.time() - start_time
        logging.debug(f"[AGENT_EXEC] Completed processing schedule {schedule.id} - Total execution time: {total_time:.2f}s")

async def check_and_execute_schedules():
    """Check for pending schedules and execute them"""
    check_start_time = time.time()
    
    try:
        logging.debug("[SCHEDULE_CHECK] Starting schedule check cycle")
        pending_schedules = ScheduleAgentService.get_pending_schedules()
        
        if not pending_schedules:
            logging.debug("[SCHEDULE_CHECK] No pending schedules found")
            return
        
        logging.info(f"[SCHEDULE_CHECK] Found {len(pending_schedules)} pending schedules to process")
        
        # Filter out weekly schedules that have already run today
        current_date = datetime.now().date()
        
        filtered_schedules = []
        skipped_count = 0
        
        for schedule in pending_schedules:
            should_execute = True
            schedule_info = f"ID:{schedule.id}, Name:{schedule.name}, Type:{schedule.frequency_type}"
            
            if schedule.frequency_type == 'weekly':
                # Check if this weekly schedule has already run today
                if schedule.last_run_time:
                    last_run_date = datetime.fromtimestamp(schedule.last_run_time).date()
                    if last_run_date == current_date:
                        should_execute = False
                        skipped_count += 1
                        logging.info(f"[SCHEDULE_CHECK] ⏭️  Skipping weekly schedule {schedule_info} - already ran today ({current_date})")
            
            if should_execute:
                filtered_schedules.append(schedule)
                logging.debug(f"[SCHEDULE_CHECK] ✅ Schedule {schedule_info} added to execution queue")
        
        if skipped_count > 0:
            logging.info(f"[SCHEDULE_CHECK] Skipped {skipped_count} schedules that already ran today")
        
        if filtered_schedules:
            logging.info(f"[SCHEDULE_CHECK] 🚀 Executing {len(filtered_schedules)} schedules (filtered from {len(pending_schedules)} total)")
            
            execution_start = time.time()
            async with trio.open_nursery() as nursery:
                for schedule in filtered_schedules:
                    schedule_info = f"ID:{schedule.id}, Name:{schedule.name}"
                    logging.debug(f"[SCHEDULE_CHECK] Starting execution task for schedule {schedule_info}")
                    nursery.start_soon(execute_scheduled_agent, schedule)
            
            execution_time = time.time() - execution_start
            logging.info(f"[SCHEDULE_CHECK] ✅ Completed execution of {len(filtered_schedules)} schedules in {execution_time:.2f}s")
        else:
            logging.info(f"[SCHEDULE_CHECK] All {len(pending_schedules)} pending schedules were filtered out (already executed today)")
                    
    except Exception as e:
        check_time = time.time() - check_start_time
        logging.error(f"[SCHEDULE_CHECK] ❌ Error during schedule check cycle after {check_time:.2f}s")
        logging.error(f"[SCHEDULE_CHECK] Error type: {type(e).__name__}")
        logging.error(f"[SCHEDULE_CHECK] Error message: {str(e)}")
        logging.exception(f"[SCHEDULE_CHECK] Exception details: {e}")
    finally:
        total_check_time = time.time() - check_start_time
        logging.debug(f"[SCHEDULE_CHECK] Schedule check cycle completed in {total_check_time:.2f}s")

async def report_status():
    global CONSUMER_NAME, BOOT_AT, DONE_AGENTS, FAILED_AGENTS
    
    REDIS_CONN.sadd("AGENTEXE", CONSUMER_NAME)
    redis_lock = RedisDistributedLock("clean_agent_executor", lock_value=CONSUMER_NAME, timeout=60)
    
    logging.info(f"[STATUS_REPORT] Started status reporting for {CONSUMER_NAME}")
    
    while not stop_event.is_set():
        try:
            now = datetime.now()
            
            heartbeat = json.dumps({
                "name": CONSUMER_NAME,
                "now": now.astimezone().isoformat(timespec="milliseconds"),
                "boot_at": BOOT_AT,
                "done": DONE_AGENTS,
                "failed": FAILED_AGENTS,
            })
            
            REDIS_CONN.zadd(CONSUMER_NAME, heartbeat, now.timestamp())
            logging.debug(f"[STATUS_REPORT] {CONSUMER_NAME} sent heartbeat - Done: {DONE_AGENTS}, Failed: {FAILED_AGENTS}")
            
            # Clean expired entries
            expired = REDIS_CONN.zcount(CONSUMER_NAME, 0, now.timestamp() - 60 * 30)
            if expired > 0:
                REDIS_CONN.zpopmin(CONSUMER_NAME, expired)
                logging.debug(f"[STATUS_REPORT] Cleaned {expired} expired entries for {CONSUMER_NAME}")
            
            # Clean expired agent executors
            if redis_lock.acquire():
                agent_executors = REDIS_CONN.smembers("AGENTEXE")
                cleaned_count = 0
                
                for consumer_name in agent_executors:
                    if consumer_name == CONSUMER_NAME:
                        continue
                    expired = REDIS_CONN.zcount(
                        consumer_name, now.timestamp() - WORKER_HEARTBEAT_TIMEOUT, now.timestamp() + 10
                    )
                    if expired == 0:
                        REDIS_CONN.srem("AGENTEXE", consumer_name)
                        REDIS_CONN.delete(consumer_name)
                        cleaned_count += 1
                        logging.info(f"[STATUS_REPORT] 🧹 Cleaned expired agent executor: {consumer_name}")
                
                if cleaned_count > 0:
                    logging.info(f"[STATUS_REPORT] Cleaned {cleaned_count} expired agent executors")
                        
        except Exception as e:
            logging.error(f"[STATUS_REPORT] Error in status reporting for {CONSUMER_NAME}: {e}")
            logging.exception(f"[STATUS_REPORT] Status report exception details: {e}")
        finally:
            redis_lock.release()
            
        await trio.sleep(30)

async def main():
    logging.info(r"""
   ___                   __     ______                     __
  / _ | ___  ___ ___  __ / /_   / ____/  _____  _______  __/ /_____  _____
 / __ |/ _ \/ -_) _ \/ // __/  / __/ | |/_/ _ \/ ___/ / / / __/ __ \/ ___/
/_/ |_|\_, /\__/_//_/\_,_\__/  / /____>  </  __/ /__/ /_/ / /_/ /_/ / /
      /___/                  /_____/_/|_|\___/\___/\__,_/\__/\____/_/
    """)
    logging.info(f'[AGENT_EXEC] 🚀 AgentExecutor starting - RAGFlow version: {get_ragflow_version()}')
    logging.info(f'[AGENT_EXEC] Consumer name: {CONSUMER_NAME}')
    logging.info(f'[AGENT_EXEC] Boot time: {BOOT_AT}')
    logging.info(f'[AGENT_EXEC] Max concurrent agents: {MAX_CONCURRENT_AGENTS}')
    
    settings.init_settings()
    
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    logging.info(f'[AGENT_EXEC] Starting main execution loop...')
    
    async with trio.open_nursery() as nursery:
        logging.debug(f'[AGENT_EXEC] Starting status reporter task')
        nursery.start_soon(report_status)
        
        cycle_count = 0
        while not stop_event.is_set():
            cycle_count += 1
            logging.debug(f'[AGENT_EXEC] Starting schedule check cycle #{cycle_count}')
            nursery.start_soon(check_and_execute_schedules)
            await trio.sleep(30)  # Check


if __name__ == "__main__":
    initRootLogger(CONSUMER_NAME)
    trio.run(main)
