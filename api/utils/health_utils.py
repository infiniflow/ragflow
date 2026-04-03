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
from datetime import datetime
import json
import os
import requests
from timeit import default_timer as timer

from api.db.db_models import DB
from rag.utils.redis_conn import REDIS_CONN
from rag.utils.es_conn import ESConnection
from rag.utils.infinity_conn import InfinityConnection
from rag.utils.ob_conn import OBConnection
from common import settings


def _ok_nok(ok: bool) -> str:
    return "ok" if ok else "nok"


def check_db() -> tuple[bool, dict]:
    st = timer()
    try:
        # lightweight probe; works for MySQL/Postgres
        DB.execute_sql("SELECT 1")
        return True, {"elapsed": f"{(timer() - st) * 1000.0:.1f}"}
    except Exception as e:
        return False, {"elapsed": f"{(timer() - st) * 1000.0:.1f}", "error": str(e)}


def check_redis() -> tuple[bool, dict]:
    st = timer()
    try:
        ok = bool(REDIS_CONN.health())
        return ok, {"elapsed": f"{(timer() - st) * 1000.0:.1f}"}
    except Exception as e:
        return False, {"elapsed": f"{(timer() - st) * 1000.0:.1f}", "error": str(e)}


def check_doc_engine() -> tuple[bool, dict]:
    st = timer()
    try:
        meta = settings.docStoreConn.health()
        # treat any successful call as ok
        return True, {"elapsed": f"{(timer() - st) * 1000.0:.1f}", **(meta or {})}
    except Exception as e:
        return False, {"elapsed": f"{(timer() - st) * 1000.0:.1f}", "error": str(e)}


def check_storage() -> tuple[bool, dict]:
    st = timer()
    try:
        settings.STORAGE_IMPL.health()
        return True, {"elapsed": f"{(timer() - st) * 1000.0:.1f}"}
    except Exception as e:
        return False, {"elapsed": f"{(timer() - st) * 1000.0:.1f}", "error": str(e)}


def get_es_cluster_stats() -> dict:
    doc_engine = os.getenv('DOC_ENGINE', 'elasticsearch')
    if doc_engine != 'elasticsearch':
        raise Exception("Elasticsearch is not in use.")
    try:
        return {
            "status": "alive",
            "message": ESConnection().get_cluster_stats()
        }
    except Exception as e:
        return {
            "status": "timeout",
            "message": f"error: {str(e)}",
        }


def get_infinity_status():
    doc_engine = os.getenv('DOC_ENGINE', 'elasticsearch')
    if doc_engine != 'infinity':
        raise Exception("Infinity is not in use.")
    try:
        return {
            "status": "alive",
            "message": InfinityConnection().health()
        }
    except Exception as e:
        return {
            "status": "timeout",
            "message": f"error: {str(e)}",
        }


def get_oceanbase_status():
    """
    Get OceanBase health status and performance metrics.
    
    Returns:
        dict: OceanBase status with health information and performance metrics
    """
    doc_engine = os.getenv('DOC_ENGINE', 'elasticsearch')
    if doc_engine != 'oceanbase':
        raise Exception("OceanBase is not in use.")
    try:
        ob_conn = OBConnection()
        health_info = ob_conn.health()
        performance_metrics = ob_conn.get_performance_metrics()
        
        # Combine health and performance metrics
        status = "alive" if health_info.get("status") == "healthy" else "timeout"
        
        return {
            "status": status,
            "message": {
                "health": health_info,
                "performance": performance_metrics
            }
        }
    except Exception as e:
        return {
            "status": "timeout",
            "message": f"error: {str(e)}",
        }


def check_oceanbase_health() -> dict:
    """
    Check OceanBase health status with comprehensive metrics.
    
    This function provides detailed health information including:
    - Connection status
    - Query latency
    - Storage usage
    - Query throughput (QPS)
    - Slow query statistics
    - Connection pool statistics
    
    Returns:
        dict: Health status with detailed metrics
    """
    doc_engine = os.getenv('DOC_ENGINE', 'elasticsearch')
    if doc_engine != 'oceanbase':
        return {
            "status": "not_configured",
            "details": {
                "connection": "not_configured",
                "message": "OceanBase is not configured as the document engine"
            }
        }
    
    try:
        ob_conn = OBConnection()
        health_info = ob_conn.health()
        performance_metrics = ob_conn.get_performance_metrics()
        
        # Determine overall health status
        connection_status = performance_metrics.get("connection", "unknown")
        
        # If connection is disconnected, return unhealthy
        if connection_status == "disconnected" or health_info.get("status") != "healthy":
            return {
                "status": "unhealthy",
                "details": {
                    "connection": connection_status,
                    "latency_ms": performance_metrics.get("latency_ms", 0),
                    "storage_used": performance_metrics.get("storage_used", "N/A"),
                    "storage_total": performance_metrics.get("storage_total", "N/A"),
                    "query_per_second": performance_metrics.get("query_per_second", 0),
                    "slow_queries": performance_metrics.get("slow_queries", 0),
                    "active_connections": performance_metrics.get("active_connections", 0),
                    "max_connections": performance_metrics.get("max_connections", 0),
                    "uri": health_info.get("uri", "unknown"),
                    "version": health_info.get("version_comment", "unknown"),
                    "error": health_info.get("error", performance_metrics.get("error"))
                }
            }
        
        # Check if healthy (connected and low latency)
        is_healthy = (
            connection_status == "connected" and
            performance_metrics.get("latency_ms", float('inf')) < 1000  # Latency under 1 second
        )
        
        return {
            "status": "healthy" if is_healthy else "degraded",
            "details": {
                "connection": performance_metrics.get("connection", "unknown"),
                "latency_ms": performance_metrics.get("latency_ms", 0),
                "storage_used": performance_metrics.get("storage_used", "N/A"),
                "storage_total": performance_metrics.get("storage_total", "N/A"),
                "query_per_second": performance_metrics.get("query_per_second", 0),
                "slow_queries": performance_metrics.get("slow_queries", 0),
                "active_connections": performance_metrics.get("active_connections", 0),
                "max_connections": performance_metrics.get("max_connections", 0),
                "uri": health_info.get("uri", "unknown"),
                "version": health_info.get("version_comment", "unknown")
            }
        }
    except Exception as e:
        return {
            "status": "unhealthy",
            "details": {
                "connection": "disconnected",
                "error": str(e)
            }
        }


def get_mysql_status():
    try:
        cursor = DB.execute_sql("SHOW PROCESSLIST;")
        res_rows = cursor.fetchall()
        headers = ['id', 'user', 'host', 'db', 'command', 'time', 'state', 'info']
        cursor.close()
        return {
            "status": "alive",
            "message": [dict(zip(headers, r)) for r in res_rows]
        }
    except Exception as e:
        return {
            "status": "timeout",
            "message": f"error: {str(e)}",
        }


def check_minio_alive():
    start_time = timer()
    try:
        response = requests.get(f'http://{settings.MINIO["host"]}/minio/health/live')
        if response.status_code == 200:
            return {"status": "alive", "message": f"Confirm elapsed: {(timer() - start_time) * 1000.0:.1f} ms."}
        else:
            return {"status": "timeout", "message": f"Confirm elapsed: {(timer() - start_time) * 1000.0:.1f} ms."}
    except Exception as e:
        return {
            "status": "timeout",
            "message": f"error: {str(e)}",
        }


def get_redis_info():
    try:
        return {
            "status": "alive",
            "message": REDIS_CONN.info()
        }
    except Exception as e:
        return {
            "status": "timeout",
            "message": f"error: {str(e)}",
        }


def check_ragflow_server_alive():
    start_time = timer()
    try:
        url = f'http://{settings.HOST_IP}:{settings.HOST_PORT}/v1/system/ping'
        if '0.0.0.0' in url:
            url = url.replace('0.0.0.0', '127.0.0.1')
        response = requests.get(url)
        if response.status_code == 200:
            return {"status": "alive", "message": f"Confirm elapsed: {(timer() - start_time) * 1000.0:.1f} ms."}
        else:
            return {"status": "timeout", "message": f"Confirm elapsed: {(timer() - start_time) * 1000.0:.1f} ms."}
    except Exception as e:
        return {
            "status": "timeout",
            "message": f"error: {str(e)}",
        }


def check_task_executor_alive():
    task_executor_heartbeats = {}
    try:
        task_executors = REDIS_CONN.smembers("TASKEXE")
        now = datetime.now().timestamp()
        for task_executor_id in task_executors:
            heartbeats = REDIS_CONN.zrangebyscore(task_executor_id, now - 60 * 30, now)
            heartbeats = [json.loads(heartbeat) for heartbeat in heartbeats]
            task_executor_heartbeats[task_executor_id] = heartbeats
        if task_executor_heartbeats:
            status = "alive" if any(task_executor_heartbeats.values()) else "timeout"
            return {"status": status, "message": task_executor_heartbeats}
        else:
            return {"status": "timeout", "message": "Not found any task executor."}
    except Exception as e:
        return {
            "status": "timeout",
            "message": f"error: {str(e)}"
        }


def run_health_checks() -> tuple[dict, bool]:
    result: dict[str, str | dict] = {}

    db_ok, db_meta = check_db()
    result["db"] = _ok_nok(db_ok)
    if not db_ok:
        result.setdefault("_meta", {})["db"] = db_meta

    try:
        redis_ok, redis_meta = check_redis()
        result["redis"] = _ok_nok(redis_ok)
        if not redis_ok:
            result.setdefault("_meta", {})["redis"] = redis_meta
    except Exception:
        result["redis"] = "nok"

    try:
        doc_ok, doc_meta = check_doc_engine()
        result["doc_engine"] = _ok_nok(doc_ok)
        if not doc_ok:
            result.setdefault("_meta", {})["doc_engine"] = doc_meta
    except Exception:
        result["doc_engine"] = "nok"

    try:
        sto_ok, sto_meta = check_storage()
        result["storage"] = _ok_nok(sto_ok)
        if not sto_ok:
            result.setdefault("_meta", {})["storage"] = sto_meta
    except Exception:
        result["storage"] = "nok"

    all_ok = (result.get("db") == "ok") and (result.get("redis") == "ok") and (result.get("doc_engine") == "ok") and (
                result.get("storage") == "ok")
    result["status"] = "ok" if all_ok else "nok"
    return result, all_ok
