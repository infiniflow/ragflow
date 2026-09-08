import time
from pathlib import Path
from threading import Event

from asr_service.jobs.job_models import CreateJobRequest, JobStatus
from asr_service.jobs.job_queue import JobQueue
from asr_service.jobs.job_store import JobStore
from asr_service.jobs.worker import Worker
from asr_service.models.model_manager import ModelManager
from asr_service.models.model_registry import default_registry
from asr_service.settings import Settings


def _settings() -> Settings:
    return Settings(ASR_JOB_TTL_SECONDS=5, ASR_OLLAMA_BASE_URL="http://localhost:11434")


def test_queued_to_done_transition_with_available_model(monkeypatch):
    monkeypatch.setattr("asr_service.pipeline.preprocess.preprocess_audio", lambda source_uri, settings, output_dir: (Path("sample.wav"), None))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.load", lambda self: setattr(self, "_loaded", True))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.transcribe", lambda self, audio_path, language: {"transcript": "распознанный текст", "segments": []})

    store = JobStore()
    queue = JobQueue()
    worker = Worker(store=store, queue=queue, registry=default_registry(), manager=ModelManager(), settings=_settings())
    worker.start()
    try:
        job = store.create(CreateJobRequest(model_key="whisper-large-v3", language="ru"))
        queue.put(job.id)
        for _ in range(30):
            current = store.get(job.id)
            if current and current.status == JobStatus.done:
                break
            time.sleep(0.05)
        assert current is not None
        assert current.status == JobStatus.done
        assert current.result["transcript"] == "распознанный текст"
    finally:
        worker.stop()


def test_processing_to_error_when_model_is_missing():
    store = JobStore()
    queue = JobQueue()
    worker = Worker(store=store, queue=queue, registry=default_registry(), manager=ModelManager(), settings=_settings())
    worker.start()
    try:
        job = store.create(CreateJobRequest(model_key="missing-model", language="ru"))
        queue.put(job.id)
        for _ in range(30):
            current = store.get(job.id)
            if current and current.status == JobStatus.error:
                break
            time.sleep(0.05)
        assert current is not None
        assert current.status == JobStatus.error
        assert current.error["error_code"] == "Q-ASR-MODEL-NOT-FOUND"
    finally:
        worker.stop()


def test_cancel_requested_during_inference_wins_over_done(monkeypatch):
    started = Event()
    release = Event()

    def blocking_transcribe(_self, audio_path, language):
        started.set()
        assert release.wait(timeout=5)
        return {"transcript": "result produced after cancellation", "segments": []}

    monkeypatch.setattr("asr_service.pipeline.preprocess.preprocess_audio", lambda source_uri, settings, output_dir: (Path("sample.wav"), None))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.load", lambda self: setattr(self, "_loaded", True))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.transcribe", blocking_transcribe)

    store = JobStore()
    queue = JobQueue()
    worker = Worker(store=store, queue=queue, registry=default_registry(), manager=ModelManager(), settings=_settings())
    worker.start()
    try:
        job = store.create(CreateJobRequest(model_key="whisper-large-v3", language="ru"))
        queue.put(job.id)
        assert started.wait(timeout=5)
        canceled = store.cancel(job.id)
        assert canceled is not None and canceled.cancel_requested is True
        release.set()
        for _ in range(100):
            current = store.get(job.id)
            if current and current.status == JobStatus.canceled:
                break
            time.sleep(0.02)
        assert current is not None
        assert current.status == JobStatus.canceled
        assert current.stage == "canceled"
        assert current.percent == 100
        assert current.result is None
        assert current.artifacts == {}
        assert current.error is None
    finally:
        release.set()
        worker.stop()


def test_cancel_requested_during_artifact_write_wins_over_done(monkeypatch):
    started = Event()
    release = Event()

    def blocking_write(_self, job_id, result, requested_formats, normalized_wav):
        started.set()
        assert release.wait(timeout=5)
        return {"result_json": f"{job_id}/result.json"}

    monkeypatch.setattr("asr_service.pipeline.preprocess.preprocess_audio", lambda source_uri, settings, output_dir: (Path("sample.wav"), None))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.load", lambda self: setattr(self, "_loaded", True))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.transcribe", lambda self, audio_path, language: {"transcript": "result", "segments": []})
    monkeypatch.setattr(Worker, "_write_artifacts", blocking_write)

    store = JobStore()
    queue = JobQueue()
    worker = Worker(store=store, queue=queue, registry=default_registry(), manager=ModelManager(), settings=_settings())
    worker.start()
    try:
        job = store.create(CreateJobRequest(model_key="whisper-large-v3", language="ru"))
        queue.put(job.id)
        assert started.wait(timeout=5)
        canceled = store.cancel(job.id)
        assert canceled is not None and canceled.cancel_requested is True
        release.set()
        for _ in range(100):
            current = store.get(job.id)
            if current and current.status == JobStatus.canceled:
                break
            time.sleep(0.02)
        assert current is not None
        assert current.status == JobStatus.canceled
        assert current.result is None
        assert current.artifacts == {}
    finally:
        release.set()
        worker.stop()


def test_enrich_requires_ollama_model(monkeypatch):
    monkeypatch.setattr("asr_service.pipeline.preprocess.preprocess_audio", lambda source_uri, settings, output_dir: (Path("sample.wav"), None))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.load", lambda self: setattr(self, "_loaded", True))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.transcribe", lambda self, audio_path, language: {"transcript": "text", "segments": []})

    settings = Settings(ASR_OLLAMA_MODEL="")
    store = JobStore()
    queue = JobQueue()
    worker = Worker(store=store, queue=queue, registry=default_registry(), manager=ModelManager(), settings=settings)
    worker.start()
    try:
        job = store.create(CreateJobRequest(model_key="whisper-large-v3", language="ru", options={"enrich": {"enabled": True}}))
        queue.put(job.id)
        for _ in range(30):
            current = store.get(job.id)
            if current and current.status == JobStatus.error:
                break
            time.sleep(0.05)
        assert current is not None
        assert current.error["error_code"] == "W-ASR-OLLAMA-NOT-CONFIGURED"
    finally:
        worker.stop()


def test_whisper_transcript_smoke_not_stub(monkeypatch):
    monkeypatch.setattr("asr_service.pipeline.preprocess.preprocess_audio", lambda source_uri, settings, output_dir: (Path("sample.wav"), None))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.load", lambda self: setattr(self, "_loaded", True))
    monkeypatch.setattr(
        "asr_service.models.engines.whisper_engine.WhisperEngine.transcribe",
        lambda self, audio_path, language: {"transcript": "Привет, это проверка распознавания", "segments": [{"start": 0.0, "end": 1.2, "text": "Привет"}]},
    )

    store = JobStore()
    queue = JobQueue()
    worker = Worker(store=store, queue=queue, registry=default_registry(), manager=ModelManager(), settings=_settings())
    worker.start()
    try:
        job = store.create(CreateJobRequest(model_key="whisper-large-v3", language="ru"))
        queue.put(job.id)
        for _ in range(30):
            current = store.get(job.id)
            if current and current.status in {JobStatus.done, JobStatus.error}:
                break
            time.sleep(0.05)

        assert current is not None
        assert current.status == JobStatus.done
        transcript = current.result["transcript"]
        assert transcript
        assert "stub" not in transcript.lower()
        assert "mock" not in transcript.lower()
        assert isinstance(current.result.get("segments"), list)
    finally:
        worker.stop()
