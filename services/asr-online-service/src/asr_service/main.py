import logging
import shutil
import subprocess
from contextlib import asynccontextmanager
from datetime import datetime, timezone
from pathlib import Path

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles

from asr_service.api import routes_asr, routes_health, routes_metrics, routes_openai
from asr_service.http.request_id import RequestIdMiddleware, request_id_ctx
from asr_service.jobs.job_queue import JobQueue
from asr_service.jobs.job_store import JobStore
from asr_service.jobs.worker import Worker
from asr_service.logging_json import configure_logging
from asr_service.models.model_manager import ModelManager
from asr_service.models.model_registry import default_registry
from asr_service.settings import settings

configure_logging(settings.log_level, settings.log_data_mode)

store = JobStore(ttl_seconds=settings.job_ttl_seconds)
queue = JobQueue()
registry = default_registry()
manager = ModelManager()
worker = Worker(store=store, queue=queue, registry=registry, manager=manager, settings=settings)


def _check_tool(tool_path: str) -> dict:
    checked_at = datetime.now(timezone.utc).isoformat()
    resolved_path = tool_path if Path(tool_path).exists() else shutil.which(tool_path)
    if not resolved_path:
        return {"ok": False, "details": {"resolved_path": None, "checked_at": checked_at, "error": "tool not found"}}

    try:
        subprocess.run([str(resolved_path), "-version"], check=False, capture_output=True, text=True, timeout=2)
        return {"ok": True, "details": {"resolved_path": str(resolved_path), "checked_at": checked_at, "error": None}}
    except subprocess.TimeoutExpired:
        return {"ok": True, "details": {"resolved_path": str(resolved_path), "checked_at": checked_at, "error": "version check timeout"}}
    except Exception as exc:
        return {"ok": False, "details": {"resolved_path": str(resolved_path), "checked_at": checked_at, "error": str(exc)}}


def _build_health_checks() -> dict:
    ffmpeg = _check_tool(settings.ffmpeg_path)
    sox = (
        _check_tool(settings.sox_path)
        if settings.enable_sox_normalize
        else {
            "ok": True,
            "details": {"resolved_path": None, "checked_at": datetime.now(timezone.utc).isoformat(), "error": "check skipped (normalization disabled)"},
        }
    )
    return {"ffmpeg": ffmpeg, "sox": sox}


@asynccontextmanager
async def _lifespan(application: FastAPI):
    application.state.health_checks = _build_health_checks()
    worker.start()
    try:
        logging.getLogger("asr.control").info(
            "service_start",
            extra={"event": "service_start", "payload": {"swagger_url": f"http://localhost:{settings.service_port}/docs"}},
        )
        yield
    finally:
        worker.stop()


app = FastAPI(title="ASR Online Service", version="0.2.0", lifespan=_lifespan)
app.add_middleware(RequestIdMiddleware)
app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])

app.state.job_store = store
app.state.job_queue = queue
app.state.registry = registry

app.include_router(routes_health.router)
app.include_router(routes_metrics.router)
app.include_router(routes_asr.router)
app.include_router(routes_openai.router)
app.mount("/", StaticFiles(directory="webapp", html=True), name="webapp")


class RequestIdFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        record.request_id = request_id_ctx.get()
        return True


for name in ["asr.control", "asr.data"]:
    logging.getLogger(name).addFilter(RequestIdFilter())
