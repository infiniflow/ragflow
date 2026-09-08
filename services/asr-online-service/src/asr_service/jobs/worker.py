import json
import logging
import subprocess
import threading
from pathlib import Path

from docx import Document

from asr_service.jobs.job_models import JobStatus
from asr_service.jobs.job_queue import JobQueue
from asr_service.jobs.job_store import JobStore
from asr_service.models.engines.base import AsrEngine
from asr_service.models.model_manager import ModelManager
from asr_service.models.model_registry import RegistryView
from asr_service.pipeline.enrich_ollama import enrich_text
from asr_service.pipeline.postprocess import finalize_transcript
from asr_service.pipeline.preprocess import ToolInvocationError, preprocess_audio
from asr_service.settings import Settings

control_logger = logging.getLogger("asr.control")


class Worker:
    def __init__(self, store: JobStore, queue: JobQueue, registry: RegistryView, manager: ModelManager, settings: Settings) -> None:
        self._store = store
        self._queue = queue
        self._registry = registry
        self._manager = manager
        self._settings = settings
        self._artifacts_dir = Path(settings.artifacts_dir)
        self._artifacts_dir.mkdir(parents=True, exist_ok=True)
        self._n_workers = max(1, settings.max_concurrent_jobs)
        self._stop_event = threading.Event()
        self._threads: list[threading.Thread] = []

    def start(self) -> None:
        if any(t.is_alive() for t in self._threads):
            return
        self._stop_event.clear()
        self._threads = [threading.Thread(target=self._run, daemon=True) for _ in range(self._n_workers)]
        for t in self._threads:
            t.start()

    def stop(self) -> None:
        self._stop_event.set()
        for t in self._threads:
            if t.is_alive():
                t.join(timeout=30)

    def _run(self) -> None:
        while not self._stop_event.is_set():
            self._store.reap_expired()
            job_id = self._queue.get(timeout=0.2)
            if not job_id:
                continue
            try:
                self._process_job(job_id)
            finally:
                self._queue.task_done()

    @staticmethod
    def _truncate_cmd(cmd: list[str]) -> list[str]:
        return [part[:200] for part in cmd[:20]]

    def _tool_error(self, error: ToolInvocationError) -> tuple[str, dict]:
        if error.tool == "ffmpeg":
            error_code = "W-ASR-FFMPEG-FAILED" if isinstance(error.cause, subprocess.CalledProcessError) else "W-ASR-FFMPEG-NOT-FOUND"
        elif error.tool == "sox":
            error_code = "W-ASR-SOX-FAILED" if isinstance(error.cause, subprocess.CalledProcessError) else "W-ASR-SOX-NOT-FOUND"
        else:
            error_code = "W-ASR-WORKER-ERROR"

        details = {
            "tool": error.tool,
            "tool_path": error.tool_path,
            "returncode": error.returncode,
            "stderr_len": len(error.stderr),
            "stderr_excerpt": error.stderr[:400],
            "cmd": self._truncate_cmd(error.cmd),
        }
        return error_code, details

    @staticmethod
    def _apply_output_contract(result: dict, include_segments: bool) -> dict:
        if include_segments:
            return result
        filtered = dict(result)
        filtered.pop("segments", None)
        return filtered

    @staticmethod
    def _finish_canceled(job) -> bool:
        """Make a cancellation observed at a stage boundary terminal."""

        if not job.cancel_requested:
            return False
        job.status = JobStatus.canceled
        job.stage = "canceled"
        job.percent = 100
        job.result = None
        job.artifacts = {}
        job.error = None
        return True

    def _process_job(self, job_id: str) -> None:
        job = self._store.get(job_id)
        if not job or job.status in {JobStatus.canceled, JobStatus.expired}:
            return

        descriptor = self._registry.by_key(job.model_key)
        if descriptor is None:
            job.status = JobStatus.error
            job.stage = "validation"
            job.error = {"error_code": "Q-ASR-MODEL-NOT-FOUND", "message": "Model not found", "details": {"model_key": job.model_key}}
            self._store.update(job)
            return

        job.status = JobStatus.processing
        job.stage = "processing"
        job.percent = 10
        self._store.update(job)

        engine: AsrEngine | None = None
        try:
            engine = self._manager.acquire(descriptor)
            if self._finish_canceled(job):
                return

            job_dir = self._artifacts_dir / job.id
            prepared_audio_path, normalized_wav = preprocess_audio(job.source_uri, self._settings, job_dir)
            if self._finish_canceled(job):
                return
            raw = engine.transcribe(audio_path=prepared_audio_path, language=job.language)
            # Engines expose a synchronous contract and cannot all be interrupted
            # safely.  A cancel received during inference must still win at the
            # next stage boundary instead of being overwritten by ``done``.
            if self._finish_canceled(job):
                return
            result = finalize_transcript(raw)
            if job.options.enrich.enabled:
                if not self._settings.ollama_model:
                    raise RuntimeError("W-ASR-OLLAMA-NOT-CONFIGURED")
                result["enriched_text"] = enrich_text(self._settings, result["transcript"])
            if self._finish_canceled(job):
                return

            result = self._apply_output_contract(result, job.options.output.include_segments)
            job.result = result
            job.artifacts = self._write_artifacts(job.id, result, job.options.output.artifact_formats, normalized_wav)
            if self._finish_canceled(job):
                return
            job.status = JobStatus.done
            job.stage = "done"
            job.percent = 100
        except Exception as exc:
            control_logger.exception("worker_job_error", extra={"event": "worker_job_error", "payload": {"job_id": job.id}})
            job.status = JobStatus.error
            job.stage = "error"
            job.percent = 100
            error_code = "W-ASR-WORKER-ERROR"
            details: dict = {"job_id": job.id}
            if isinstance(exc, ToolInvocationError):
                error_code, details = self._tool_error(exc)
            elif str(exc) == "W-ASR-OLLAMA-NOT-CONFIGURED":
                error_code = "W-ASR-OLLAMA-NOT-CONFIGURED"
            elif str(exc) == "Q-ASR-ENGINE-NOT-AVAILABLE":
                error_code = "Q-ASR-ENGINE-NOT-AVAILABLE"
            elif str(exc) == "W-ASR-SOURCE-URI-INVALID":
                error_code = "W-ASR-SOURCE-URI-INVALID"
                details = {"job_id": job.id, "source_uri": job.source_uri}
            job.error = {"error_code": error_code, "message": str(exc), "details": details}
        finally:
            if engine is not None:
                self._manager.release(descriptor, engine)
            self._store.update(job)

    def _write_artifacts(self, job_id: str, result: dict, requested_formats: list[str], normalized_wav: Path | None) -> dict[str, str]:
        formats = set(requested_formats)
        formats.add("result_json")
        job_dir = self._artifacts_dir / job_id
        job_dir.mkdir(parents=True, exist_ok=True)
        artifacts: dict[str, str] = {}

        result_json = job_dir / "result.json"
        result_json.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        artifacts["result_json"] = str(result_json)

        if "txt" in formats:
            txt_path = job_dir / "result.txt"
            txt_path.write_text(result.get("transcript", ""), encoding="utf-8")
            artifacts["txt"] = str(txt_path)

        if "docx" in formats:
            docx_path = job_dir / "result.docx"
            document = Document()
            document.add_paragraph(result.get("transcript", ""))
            document.save(docx_path)
            artifacts["docx"] = str(docx_path)

        if "normalized_wav" in formats and normalized_wav:
            artifacts["normalized_wav"] = str(normalized_wav)

        return artifacts
