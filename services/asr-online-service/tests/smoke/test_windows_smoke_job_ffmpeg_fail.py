import subprocess

from asr_service.jobs.job_models import CreateJobRequest, JobStatus
from asr_service.jobs.job_queue import JobQueue
from asr_service.jobs.job_store import JobStore
from asr_service.jobs.worker import Worker
from asr_service.models.model_manager import ModelManager
from asr_service.models.model_registry import default_registry
from asr_service.settings import Settings


def test_windows_smoke_job_ffmpeg_fail(monkeypatch, tmp_path):
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.load", lambda self: setattr(self, "_loaded", True))
    monkeypatch.setattr("asr_service.models.engines.whisper_engine.WhisperEngine.transcribe", lambda self, audio_path, language: {"transcript": "never", "segments": []})

    def _fail(cmd, **kwargs):
        assert cmd[0] == "ffmpeg"
        assert kwargs["check"] is True
        raise subprocess.CalledProcessError(returncode=1, cmd=cmd, stderr="invalid audio")

    monkeypatch.setattr("asr_service.pipeline.preprocess.subprocess.run", _fail)
    source = tmp_path / "invalid.wav"
    source.write_bytes(b"invalid audio")

    store = JobStore()
    queue = JobQueue()
    worker = Worker(
        store=store,
        queue=queue,
        registry=default_registry(),
        manager=ModelManager(),
        settings=Settings(ASR_FFMPEG_PATH="ffmpeg", ASR_ARTIFACTS_DIR=str(tmp_path / "artifacts")),
    )

    job = store.create(CreateJobRequest(model_key="whisper-large-v3", language="ru", source_uri=str(source)))
    queue.put(job.id)
    worker._process_job(job.id)

    updated = store.get(job.id)
    assert updated is not None
    assert updated.status == JobStatus.error
    assert updated.error["error_code"] == "W-ASR-FFMPEG-FAILED"
    assert updated.error["details"]["tool"] == "ffmpeg"
    assert updated.error["details"]["returncode"] == 1
