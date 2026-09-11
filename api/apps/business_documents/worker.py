#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from __future__ import annotations

import logging
import os
import threading
from datetime import datetime
from time import monotonic
from typing import Any

from api.apps.business_documents.ai import BusinessDocumentAI
from api.apps.business_documents.errors import BusinessDocumentError, ConflictError
from api.apps.business_documents.evidence import BusinessDocumentEvidence, related_file_search_enabled
from api.apps.business_documents.exports import BusinessDocumentExportService
from api.apps.business_documents.service import BusinessDocumentService
from api.db.db_models import BusinessDocumentJob
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp


_ACTIVE_STATUSES = ("PENDING", "RETRY")
_WAKE_EVENT = threading.Event()
_START_LOCK = threading.Lock()
_WORKER_THREAD: threading.Thread | None = None


def _error_payload(error: Exception) -> dict[str, Any]:
    if isinstance(error, BusinessDocumentError):
        return {"code": error.code, "message": error.message, "details": error.details}
    return {"code": "WORKER_FAILURE", "message": str(error)[:2000] or type(error).__name__}


class BusinessDocumentJobQueue:
    """Database-backed multi-process job lease and recovery operations."""

    @classmethod
    def claim(cls, worker_id: str, *, lease_ms: int = 60_000, now_ms: int | None = None) -> BusinessDocumentJob | None:
        now_ms = current_timestamp() if now_ms is None else now_ms
        database = BusinessDocumentJob._meta.database
        with database.atomic():
            candidates = (
                BusinessDocumentJob.select(BusinessDocumentJob.id)
                .where(
                    (BusinessDocumentJob.status.in_(_ACTIVE_STATUSES))
                    & (BusinessDocumentJob.attempt < BusinessDocumentJob.max_attempts)
                    & (BusinessDocumentJob.available_at <= now_ms)
                    & ((BusinessDocumentJob.lease_expires_at.is_null()) | (BusinessDocumentJob.lease_expires_at <= now_ms))
                )
                .order_by(BusinessDocumentJob.available_at.asc(), BusinessDocumentJob.create_time.asc())
                .limit(8)
            )
            for candidate in candidates:
                lease_token = get_uuid()
                changed = (
                    BusinessDocumentJob.update(
                        status="RUNNING",
                        progress=0.05,
                        progress_stage="STARTING",
                        progress_message="Запускаем обработку",
                        attempt=BusinessDocumentJob.attempt + 1,
                        lease_owner=worker_id,
                        lease_token=lease_token,
                        lease_expires_at=now_ms + lease_ms,
                        update_time=now_ms,
                        update_date=datetime.now(),
                    )
                    .where(
                        (BusinessDocumentJob.id == candidate.id)
                        & (BusinessDocumentJob.status.in_(_ACTIVE_STATUSES))
                        & (BusinessDocumentJob.attempt < BusinessDocumentJob.max_attempts)
                        & (BusinessDocumentJob.available_at <= now_ms)
                        & ((BusinessDocumentJob.lease_expires_at.is_null()) | (BusinessDocumentJob.lease_expires_at <= now_ms))
                    )
                    .execute()
                )
                if changed == 1:
                    return BusinessDocumentJob.get_by_id(candidate.id)
        return None

    @classmethod
    def retry(
        cls,
        job_id: str,
        worker_id: str,
        lease_token: str,
        error: dict[str, Any],
        *,
        delay_ms: int,
        now_ms: int | None = None,
    ) -> bool:
        now_ms = current_timestamp() if now_ms is None else now_ms
        changed = (
            BusinessDocumentJob.update(
                status="RETRY",
                progress=0.02,
                progress_stage="RETRY_WAIT",
                progress_message="Ожидает повторного запуска",
                available_at=now_ms + max(0, delay_ms),
                lease_owner=None,
                lease_token=None,
                lease_expires_at=None,
                error=error,
                update_time=now_ms,
                update_date=datetime.now(),
            )
            .where(
                (BusinessDocumentJob.id == job_id)
                & (BusinessDocumentJob.status == "RUNNING")
                & (BusinessDocumentJob.lease_owner == worker_id)
                & (BusinessDocumentJob.lease_token == lease_token)
                & (BusinessDocumentJob.lease_expires_at > now_ms)
            )
            .execute()
        )
        return changed == 1

    @classmethod
    def renew(cls, job_id: str, worker_id: str, lease_token: str, *, lease_ms: int) -> bool:
        now_ms = current_timestamp()
        changed = (
            BusinessDocumentJob.update(
                lease_expires_at=now_ms + lease_ms,
                update_time=now_ms,
                update_date=datetime.now(),
            )
            .where(
                (BusinessDocumentJob.id == job_id)
                & (BusinessDocumentJob.status == "RUNNING")
                & (BusinessDocumentJob.lease_owner == worker_id)
                & (BusinessDocumentJob.lease_token == lease_token)
                & (BusinessDocumentJob.lease_expires_at > now_ms)
            )
            .execute()
        )
        return changed == 1

    @classmethod
    def update_progress(
        cls,
        job_id: str,
        worker_id: str,
        lease_token: str,
        progress: float,
        stage: str,
        message: str,
    ) -> bool:
        """Persist a user-facing milestone while the worker still owns the job."""

        progress = min(max(float(progress), 0.0), 0.99)
        now_ms = current_timestamp()
        changed = (
            BusinessDocumentJob.update(
                progress=progress,
                progress_stage=stage,
                progress_message=message,
                update_time=now_ms,
                update_date=datetime.now(),
            )
            .where(
                (BusinessDocumentJob.id == job_id)
                & (BusinessDocumentJob.status == "RUNNING")
                & (BusinessDocumentJob.lease_owner == worker_id)
                & (BusinessDocumentJob.lease_token == lease_token)
                & (BusinessDocumentJob.lease_expires_at > now_ms)
            )
            .execute()
        )
        return changed == 1

    @classmethod
    def recover_stale(cls, *, now_ms: int | None = None) -> tuple[int, int]:
        """Release expired leases or permanently fail exhausted jobs."""

        now_ms = current_timestamp() if now_ms is None else now_ms
        retry_count = 0
        dead_count = 0
        stale = list(
            BusinessDocumentJob.select().where((BusinessDocumentJob.status == "RUNNING") & (BusinessDocumentJob.lease_expires_at.is_null(False)) & (BusinessDocumentJob.lease_expires_at <= now_ms))
        )
        for job in stale:
            error = {"code": "JOB_LEASE_EXPIRED", "message": "Worker lease expired before completion"}
            if job.attempt >= job.max_attempts:
                recovery_owner = get_uuid()
                recovery_token = get_uuid()
                claimed = (
                    BusinessDocumentJob.update(
                        lease_owner=recovery_owner,
                        lease_token=recovery_token,
                        lease_expires_at=now_ms + 60_000,
                        update_time=now_ms,
                        update_date=datetime.now(),
                    )
                    .where(
                        (BusinessDocumentJob.id == job.id)
                        & (BusinessDocumentJob.status == "RUNNING")
                        & (BusinessDocumentJob.lease_token == job.lease_token)
                        & (BusinessDocumentJob.lease_expires_at == job.lease_expires_at)
                    )
                    .execute()
                )
                if claimed != 1:
                    continue
                try:
                    BusinessDocumentService.fail_job(job.tenant_id, recovery_owner, job.id, error, recovery_token)
                    dead_count += 1
                except BusinessDocumentError:
                    logging.exception("Unable to dead-letter stale business document job %s", job.id)
                continue
            changed = (
                BusinessDocumentJob.update(
                    status="RETRY",
                    progress=0.02,
                    progress_stage="RETRY_WAIT",
                    progress_message="Ожидает повторного запуска",
                    available_at=now_ms,
                    lease_owner=None,
                    lease_token=None,
                    lease_expires_at=None,
                    error=error,
                    update_time=now_ms,
                    update_date=datetime.now(),
                )
                .where(
                    (BusinessDocumentJob.id == job.id)
                    & (BusinessDocumentJob.status == "RUNNING")
                    & (BusinessDocumentJob.lease_token == job.lease_token)
                    & (BusinessDocumentJob.lease_expires_at == job.lease_expires_at)
                )
                .execute()
            )
            retry_count += int(changed == 1)
        return retry_count, dead_count


class _LeaseHeartbeat:
    def __init__(self, job: BusinessDocumentJob, worker_id: str, lease_ms: int):
        self._job_id = job.id
        self._worker_id = worker_id
        self._lease_token = job.lease_token
        self._lease_ms = lease_ms
        self._lost_event = threading.Event()
        self._stop_event = threading.Event()
        self._thread: threading.Thread | None = None

    def start(self) -> None:
        if not self._lease_token:
            raise RuntimeError("Claimed job has no lease token")
        interval = max(1.0, self._lease_ms / 3000)

        def run():
            while not self._stop_event.wait(interval):
                try:
                    if not BusinessDocumentJobQueue.renew(
                        self._job_id,
                        self._worker_id,
                        self._lease_token,
                        lease_ms=self._lease_ms,
                    ):
                        self._lost_event.set()
                        return
                except Exception:
                    self._lost_event.set()
                    logging.exception("Unable to renew business document job lease %s", self._job_id)
                    return

        self._thread = threading.Thread(target=run, daemon=True, name=f"business-document-lease-{self._job_id}")
        self._thread.start()

    def stop(self) -> None:
        self._stop_event.set()
        if self._thread is not None:
            self._thread.join(timeout=2)

    def ensure_current(self) -> None:
        if self._lost_event.is_set():
            raise ConflictError("JOB_LEASE_LOST", "Worker no longer owns a current lease for this job")


class BusinessDocumentWorker:
    def __init__(
        self,
        *,
        worker_id: str | None = None,
        ai: BusinessDocumentAI | None = None,
        evidence: BusinessDocumentEvidence | None = None,
        export_service: type[BusinessDocumentExportService] = BusinessDocumentExportService,
        storage=None,
        lease_ms: int = 900_000,
        retry_base_ms: int = 5_000,
        maintenance_interval_seconds: float = 60.0,
    ):
        self.worker_id = worker_id or get_uuid()
        self.ai = ai or BusinessDocumentAI()
        self.evidence = evidence or BusinessDocumentEvidence()
        self.export_service = export_service
        self.storage = storage
        self.lease_ms = lease_ms
        self.retry_base_ms = retry_base_ms
        self.maintenance_interval_seconds = max(1.0, maintenance_interval_seconds)

    def recover_stale(self, *, now_ms: int | None = None) -> tuple[int, int]:
        return BusinessDocumentJobQueue.recover_stale(now_ms=now_ms)

    def reconcile_exports(self, *, now_ms: int | None = None) -> dict[str, int]:
        return self.export_service.reconcile_staging(storage=self.storage, now_ms=now_ms)

    def _set_progress(self, job: BusinessDocumentJob, progress: float, stage: str, message: str) -> None:
        if not job.lease_token:
            raise RuntimeError("Claimed job has no lease token")
        if not BusinessDocumentJobQueue.update_progress(job.id, self.worker_id, job.lease_token, progress, stage, message):
            raise ConflictError("JOB_LEASE_LOST", "Worker no longer owns a current lease for this job")

    def run_once(self, *, now_ms: int | None = None) -> bool:
        job = BusinessDocumentJobQueue.claim(self.worker_id, lease_ms=self.lease_ms, now_ms=now_ms)
        if job is None:
            return False
        claimed_lease_token = job.lease_token
        heartbeat = None
        prepared_export = None
        try:
            heartbeat = _LeaseHeartbeat(job, self.worker_id, self.lease_ms)
            heartbeat.start()
            if job.job_type == "GENERATE_EXPORT":
                self._set_progress(job, 0.35, "EXPORTING", "Формируем файл")
                prepared_export = self.export_service.generate(
                    job,
                    storage=self.storage,
                    ensure_current=heartbeat.ensure_current,
                )
                output = prepared_export
                execution_audit = None
            else:
                dataset_ids = job.payload.get("dataset_ids", []) if isinstance(job.payload, dict) else []
                if dataset_ids and related_file_search_enabled():
                    self._set_progress(job, 0.15, "RETRIEVING", "Подбираем связанные материалы")
                    evidence_snapshot = self.evidence.retrieve(job)
                    self._set_progress(job, 0.32, "RETRIEVED", "Связанные материалы подготовлены")
                    execution_audit = self.evidence.audit(evidence_snapshot, job.attempt)
                    self._set_progress(job, 0.4, "GENERATING", "Формируем результат")
                    output = self.ai.process(job, evidence_snapshot)
                else:
                    execution_audit = None
                    self._set_progress(job, 0.4, "GENERATING", "Формируем результат")
                    output = self.ai.process(job)
                self._set_progress(job, 0.82, "VALIDATING", "Проверяем результат")
            self._set_progress(job, 0.92, "PERSISTING", "Сохраняем результат")
            heartbeat.ensure_current()
            BusinessDocumentService.complete_job(
                job.tenant_id,
                self.worker_id,
                job.id,
                output,
                job.lease_token,
                execution_audit,
            )
        except Exception as error:
            if not isinstance(error, BusinessDocumentError) or error.code != "JOB_LEASE_LOST":
                payload = _error_payload(error)
                current_job = BusinessDocumentJob.get_or_none(BusinessDocumentJob.id == job.id)
                if current_job is not None and current_job.lease_owner == self.worker_id and current_job.lease_token == claimed_lease_token:
                    if current_job.attempt >= current_job.max_attempts:
                        try:
                            BusinessDocumentService.fail_job(
                                current_job.tenant_id,
                                self.worker_id,
                                current_job.id,
                                payload,
                                claimed_lease_token,
                            )
                        except BusinessDocumentError:
                            logging.exception("Unable to dead-letter business document job %s", current_job.id)
                    else:
                        delay = self.retry_base_ms * (2 ** max(0, current_job.attempt - 1))
                        BusinessDocumentJobQueue.retry(
                            current_job.id,
                            self.worker_id,
                            claimed_lease_token,
                            payload,
                            delay_ms=delay,
                        )
        finally:
            if heartbeat is not None:
                heartbeat.stop()
            if prepared_export is not None:
                self.export_service.discard(prepared_export, storage=self.storage)
        return True

    def run_forever(self, stop_event: threading.Event, *, poll_seconds: float = 2.0) -> None:
        try:
            self.recover_stale()
        except Exception:
            logging.exception("Business document stale-job recovery failed")
        try:
            self.reconcile_exports()
        except Exception:
            logging.exception("Business document export-stage reconciliation failed")
        next_maintenance_at = monotonic() + self.maintenance_interval_seconds
        while not stop_event.is_set():
            try:
                worked = self.run_once()
            except Exception:
                logging.exception("Business document worker poll failed")
                worked = False
            if worked:
                if monotonic() >= next_maintenance_at:
                    try:
                        self.recover_stale()
                    except Exception:
                        logging.exception("Business document periodic stale-job recovery failed")
                    try:
                        self.reconcile_exports()
                    except Exception:
                        logging.exception("Business document periodic export-stage reconciliation failed")
                    next_maintenance_at = monotonic() + self.maintenance_interval_seconds
                continue
            recovered = dead = 0
            try:
                recovered, dead = self.recover_stale()
            except Exception:
                logging.exception("Business document periodic stale-job recovery failed")
            reconciled = {"cleaned": 0}
            try:
                reconciled = self.reconcile_exports()
            except Exception:
                logging.exception("Business document periodic export-stage reconciliation failed")
            next_maintenance_at = monotonic() + self.maintenance_interval_seconds
            if recovered or dead or reconciled["cleaned"]:
                continue
            _WAKE_EVENT.wait(poll_seconds)
            _WAKE_EVENT.clear()


def wake_business_document_worker() -> None:
    """Wake the in-process poller; the durable row remains the source of truth."""

    _WAKE_EVENT.set()


def start_business_document_worker(stop_event: threading.Event) -> threading.Thread | None:
    """Start one poller in this process; DB leasing coordinates all processes."""

    global _WORKER_THREAD
    if os.environ.get("BUSINESS_DOCUMENT_WORKER_ENABLED", "true").lower() not in {"1", "true", "yes", "on"}:
        return None
    with _START_LOCK:
        if _WORKER_THREAD is not None and _WORKER_THREAD.is_alive():
            return _WORKER_THREAD
        poll_seconds = float(os.environ.get("BUSINESS_DOCUMENT_WORKER_POLL_SECONDS", "2"))
        worker = BusinessDocumentWorker()
        _WORKER_THREAD = threading.Thread(
            target=worker.run_forever,
            args=(stop_event,),
            kwargs={"poll_seconds": poll_seconds},
            daemon=True,
            name="business-document-worker",
        )
        _WORKER_THREAD.start()
        return _WORKER_THREAD
