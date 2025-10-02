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
import os
import requests
from timeit import default_timer as timer

from api import settings
from api.db.db_models import DB
from rag import settings as rag_settings
from rag.utils.redis_conn import REDIS_CONN
from rag.utils.storage_factory import STORAGE_IMPL
from rag.utils.es_conn import ESConnection
from rag.utils.infinity_conn import InfinityConnection


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
        STORAGE_IMPL.health()
        return True, {"elapsed": f"{(timer() - st) * 1000.0:.1f}"}
    except Exception as e:
        return False, {"elapsed": f"{(timer() - st) * 1000.0:.1f}", "error": str(e)}


def get_es_cluster_stats() -> dict:
    doc_engine = os.getenv('DOC_ENGINE', 'elasticsearch')
    if doc_engine != 'elasticsearch':
        raise Exception("Elasticsearch is not in use.")
    try:
        return {
            "alive": True,
            "message": ESConnection().get_cluster_stats()
        }
    except Exception as e:
        return {
            "alive": False,
            "message": f"error: {str(e)}",
        }


def get_infinity_status():
    doc_engine = os.getenv('DOC_ENGINE', 'elasticsearch')
    if doc_engine != 'infinity':
        raise Exception("Infinity is not in use.")
    try:
        return {
            "alive": True,
            "message": InfinityConnection().health()
        }
    except Exception as e:
        return {
            "alive": False,
            "message": f"error: {str(e)}",
        }


def get_mysql_status():
    try:
        cursor = DB.execute_sql("SHOW PROCESSLIST;")
        res_rows = cursor.fetchall()
        headers = ['id', 'user', 'host', 'db', 'command', 'time', 'state', 'info']
        cursor.close()
        return {
            "alive": True,
            "message": [dict(zip(headers, r)) for r in res_rows]
        }
    except Exception as e:
        return {
            "alive": False,
            "message": f"error: {str(e)}",
        }


def check_minio_alive():
    start_time = timer()
    try:
        response = requests.get(f'http://{rag_settings.MINIO["host"]}/minio/health/live')
        if response.status_code == 200:
            return {'alive': True, "message": f"Confirm elapsed: {(timer() - start_time) * 1000.0:.1f} ms."}
        else:
            return {'alive': False, "message": f"Confirm elapsed: {(timer() - start_time) * 1000.0:.1f} ms."}
    except Exception as e:
        return {
            "alive": False,
            "message": f"error: {str(e)}",
        }


def get_redis_info():
    try:
        return {
            "alive": True,
            "message": REDIS_CONN.info()
        }
    except Exception as e:
        return {
            "alive": False,
            "message": f"error: {str(e)}",
        }


def check_ragflow_server_alive():
    start_time = timer()
    try:
        response = requests.get(f'http://{settings.HOST_IP}:{settings.HOST_PORT}/v1/system/ping')
        if response.status_code == 200:
            return {'alive': True, "message": f"Confirm elapsed: {(timer() - start_time) * 1000.0:.1f} ms."}
        else:
            return {'alive': False, "message": f"Confirm elapsed: {(timer() - start_time) * 1000.0:.1f} ms."}
    except Exception as e:
        return {
            "alive": False,
            "message": f"error: {str(e)}",
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


    all_ok = (result.get("db") == "ok") and (result.get("redis") == "ok") and (result.get("doc_engine") == "ok") and (result.get("storage") == "ok")
    result["status"] = "ok" if all_ok else "nok"
    return result, all_ok


