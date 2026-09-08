import json
import logging
import time
import types
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from asr_service import main as main_module
from asr_service.jobs.worker import Worker
from asr_service.logging_json import JsonFormatter
from asr_service.main import app


EXPECTED_RUNTIME_ROUTES = {
    ("", (), "webapp", "Mount"),
    ("/docs", ("GET", "HEAD"), "swagger_ui_html", "Route"),
    ("/docs/oauth2-redirect", ("GET", "HEAD"), "swagger_ui_redirect", "Route"),
    ("/health/live", ("GET",), "live", "APIRoute"),
    ("/health/ready", ("GET",), "ready", "APIRoute"),
    ("/metrics", ("GET",), "metrics", "APIRoute"),
    ("/openapi.json", ("GET", "HEAD"), "openapi", "Route"),
    ("/redoc", ("GET", "HEAD"), "redoc_html", "Route"),
    ("/v1/asr/jobs", ("POST",), "create_job", "APIRoute"),
    ("/v1/asr/jobs/{job_id}", ("DELETE",), "cancel_job", "APIRoute"),
    ("/v1/asr/jobs/{job_id}", ("GET",), "get_job", "APIRoute"),
    ("/v1/asr/jobs/{job_id}/artifacts/{kind}", ("GET",), "download_artifact", "APIRoute"),
    ("/v1/asr/jobs/{job_id}/result", ("GET",), "get_result", "APIRoute"),
    ("/v1/asr/languages", ("GET",), "get_languages", "APIRoute"),
    ("/v1/asr/models", ("GET",), "get_models", "APIRoute"),
    ("/v1/asr/uploads", ("POST",), "upload_audio", "APIRoute"),
    ("/v1/audio/transcriptions", ("POST",), "create_transcription", "APIRoute"),
    ("/v1/models", ("GET",), "list_models", "APIRoute"),
}


def _extract_enum(yaml_text: str, key: str) -> list[str]:
    marker = f"    {key}:"
    start = yaml_text.find(marker)
    block = yaml_text[start:]
    enum_marker = "      enum: ["
    enum_start = block.find(enum_marker)
    enum_end = block.find("]", enum_start)
    values = block[enum_start + len(enum_marker) : enum_end]
    return [v.strip() for v in values.split(",")]


def test_runtime_route_inventory_is_exact():
    actual = {(route.path, tuple(sorted(getattr(route, "methods", None) or ())), route.name, type(route).__name__) for route in app.routes}

    assert actual == EXPECTED_RUNTIME_ROUTES


@pytest.mark.asyncio
async def test_application_lifespan_stops_worker_on_failure(monkeypatch):
    events = []
    application = types.SimpleNamespace(state=types.SimpleNamespace())
    lifecycle_worker = types.SimpleNamespace(
        start=lambda: events.append(("worker_start",)),
        stop=lambda: events.append(("worker_stop",)),
    )
    health_checks = {"ffmpeg": {"ok": True}, "sox": {"ok": True}}
    monkeypatch.setattr(main_module, "worker", lifecycle_worker)
    monkeypatch.setattr(main_module, "_build_health_checks", lambda: events.append(("health_checks",)) or health_checks)

    with pytest.raises(RuntimeError, match="serve failed"):
        async with main_module._lifespan(application):
            events.append(("serve",))
            assert application.state.health_checks is health_checks
            raise RuntimeError("serve failed")

    assert events == [
        ("health_checks",),
        ("worker_start",),
        ("serve",),
        ("worker_stop",),
    ]


def test_worker_thread_lifecycle_is_exact(monkeypatch, tmp_path):
    events = []

    class Thread:
        def __init__(self, *, target, daemon):
            self.target = target
            self.daemon = daemon
            self.alive = False
            events.append(("thread_init", target.__name__, daemon))

        def is_alive(self):
            return self.alive

        def start(self):
            self.alive = True
            events.append(("thread_start", self.target.__name__))

        def join(self, *, timeout):
            events.append(("thread_join", self.target.__name__, timeout))
            self.alive = False

    monkeypatch.setattr("asr_service.jobs.worker.threading.Thread", Thread)
    settings = types.SimpleNamespace(artifacts_dir=str(tmp_path / "artifacts"), max_concurrent_jobs=2)
    worker = Worker(store=object(), queue=object(), registry=object(), manager=object(), settings=settings)

    worker.start()
    worker.start()
    worker.stop()

    assert worker._stop_event.is_set()
    assert len(worker._threads) == 2
    assert events == [
        ("thread_init", "_run", True),
        ("thread_init", "_run", True),
        ("thread_start", "_run"),
        ("thread_start", "_run"),
        ("thread_join", "_run", 30),
        ("thread_join", "_run", 30),
    ]


def test_enum_contract_matches_runtime():
    yaml_text = Path("openapi/asr.yaml").read_text(encoding="utf-8")
    declared_statuses = _extract_enum(yaml_text, "JobStatus")
    declared_engine_types = _extract_enum(yaml_text, "EngineType")

    with TestClient(app) as client:
        runtime = client.get("/openapi.json").json()

    runtime_statuses = runtime["components"]["schemas"]["JobStatus"]["enum"]
    runtime_engine_types = runtime["components"]["schemas"]["EngineType"]["enum"]

    assert declared_statuses == runtime_statuses
    assert declared_engine_types == runtime_engine_types


def test_upload_filename_sanitization():
    with TestClient(app) as client:
        uploaded = client.post("/v1/asr/uploads", files={"file": ("../unsafe.wav", b"RIFF", "audio/wav")})
        assert uploaded.status_code == 201
        assert uploaded.json()["source_uri"].endswith("unsafe.wav")
        assert ".." not in uploaded.json()["source_uri"]


def test_job_api_artifacts_docx_txt(monkeypatch):
    monkeypatch.setattr("asr_service.jobs.worker.preprocess_audio", lambda source_uri, settings, output_dir: (Path("sample.wav"), Path("sample.wav")))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.load", lambda self: setattr(self, "_loaded", True))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.transcribe", lambda self, audio_path, language: {"transcript": "integration transcript", "segments": []})

    descriptor = next(item for item in app.state.registry.items if item.key == "whisper-large-v3")
    previous = descriptor.available
    descriptor.available = True
    try:
        with TestClient(app) as client:
            created = client.post(
                "/v1/asr/jobs",
                json={
                    "model_key": "whisper-large-v3",
                    "language": "ru",
                    "source_uri": "memory://sample.wav",
                    "options": {"output": {"artifact_formats": ["result_json", "txt", "docx", "normalized_wav"]}},
                },
            )
            assert created.status_code == 201
            job_id = created.json()["job_id"]

            for _ in range(40):
                state = client.get(f"/v1/asr/jobs/{job_id}").json()
                if state["status"] in {"done", "error"}:
                    break
                time.sleep(0.05)

            result = client.get(f"/v1/asr/jobs/{job_id}/result")
            assert result.status_code == 200
            artifacts = result.json()["artifacts"]
            assert {"result_json", "txt", "docx", "normalized_wav"}.issubset(artifacts.keys())

            assert client.get(f"/v1/asr/jobs/{job_id}/artifacts/txt").status_code == 200
            assert client.get(f"/v1/asr/jobs/{job_id}/artifacts/docx").status_code == 200
    finally:
        descriptor.available = previous


def test_log_masked_mode_hides_strings():
    formatter = JsonFormatter(data_mode="masked")
    record = logging.LogRecord("asr.data", logging.INFO, __file__, 0, "evt", (), None)
    record.event = "ollama_call_done"
    record.request_id = "rq-1"
    record.payload = {"prompt": "secret", "response": "hidden", "duration_ms": 1}
    parsed = json.loads(formatter.format(record))
    assert parsed["prompt"] == {"masked": True, "length": 6}
    assert parsed["response"] == {"masked": True, "length": 6}
    assert parsed["request_id"] == "rq-1"


def test_openapi_declares_upload_multipart_and_result_shape():
    yaml_text = Path("openapi/asr.yaml").read_text(encoding="utf-8")

    assert "multipart/form-data" in yaml_text
    assert "required: [file]" in yaml_text
    assert "ResultPayload" in yaml_text
    assert "transcript" in yaml_text
    assert "segments" in yaml_text


def test_create_job_unavailable_engine_returns_q_error():
    descriptor = next(item for item in app.state.registry.items if item.key == "t-one")
    previous = descriptor.available
    descriptor.available = False
    try:
        with TestClient(app) as client:
            created = client.post(
                "/v1/asr/jobs",
                json={
                    "model_key": "t-one",
                    "language": "ru",
                    "source_uri": "memory://sample.wav",
                },
            )

        assert created.status_code == 409
        assert created.json()["detail"]["error_code"] == "Q-ASR-ENGINE-NOT-AVAILABLE"
    finally:
        descriptor.available = previous


def test_log_masked_mode_masks_message_and_keeps_shape():
    formatter = JsonFormatter(data_mode="masked")
    record = logging.LogRecord("asr.data", logging.INFO, __file__, 0, "raw transcript", (), None)
    record.event = "worker_job_done"
    record.request_id = "rq-2"
    parsed = json.loads(formatter.format(record))

    assert parsed["message"] == {"masked": True, "length": 14}
    assert parsed["event"] == "worker_job_done"
    assert parsed["request_id"] == "rq-2"


def test_openapi_and_runtime_include_segments_contract():
    yaml_text = Path("openapi/asr.yaml").read_text(encoding="utf-8")

    assert "include_segments" in yaml_text
    assert "include_segments: { type: boolean, default: true }" in yaml_text

    with TestClient(app) as client:
        runtime = client.get("/openapi.json").json()

    output_options = runtime["components"]["schemas"]["OutputOptions"]
    include_segments = output_options["properties"]["include_segments"]
    assert include_segments["type"] == "boolean"
    assert include_segments["default"] is True

    create_job = runtime["components"]["schemas"]["CreateJobRequest"]
    options_ref = create_job["properties"]["options"]["$ref"]
    assert options_ref.endswith("/JobOptions")
