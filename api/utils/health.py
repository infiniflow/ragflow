from timeit import default_timer as timer

from api import settings
from api.db.db_models import DB
from rag.utils.redis_conn import REDIS_CONN
from rag.utils.storage_factory import STORAGE_IMPL


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


def check_chat() -> tuple[bool, dict]:
    st = timer()
    try:
        cfg = getattr(settings, "CHAT_CFG", None)
        ok = bool(cfg and cfg.get("factory"))
        return ok, {"elapsed": f"{(timer() - st) * 1000.0:.1f}"}
    except Exception as e:
        return False, {"elapsed": f"{(timer() - st) * 1000.0:.1f}", "error": str(e)}


def run_health_checks() -> tuple[dict, bool]:
    result: dict[str, str | dict] = {}

    db_ok, db_meta = check_db()
    chat_ok, chat_meta = check_chat()

    result["db"] = _ok_nok(db_ok)
    if not db_ok:
        result.setdefault("_meta", {})["db"] = db_meta

    result["chat"] = _ok_nok(chat_ok)
    if not chat_ok:
        result.setdefault("_meta", {})["chat"] = chat_meta

    # Optional probes (do not change minimal contract but exposed for observability)
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

    all_ok = (result.get("db") == "ok") and (result.get("chat") == "ok")
    result["status"] = "ok" if all_ok else "nok"
    return result, all_ok


