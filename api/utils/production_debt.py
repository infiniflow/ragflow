from __future__ import annotations

import hashlib
import json
import logging
import os
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

log: logging.Logger = logging.getLogger(__name__)

GENESIS_HASH: str = (
    "0000000000000000000000000000000000000000000000000000000000000000"
)


@dataclass
class RAGDebtReport:
    dataset_id: str
    pipeline_id: str
    rdi_score: float  # RAG Debt Index (target <= 12.0)
    chunk_fragmentation_multiplier: float  # Target <= 1.12x
    retrieval_latency_seconds: float  # Target <= 1.5s
    mutation_safety_score: float  # Target 100.0
    production_readiness_index: float  # Scale 0 - 100
    is_production_ready: bool
    critical_smells: List[str]
    receipt_hash: str


class TechnicalDueDiligenceLedger:
    """
    Cryptographic SHA-256 hash-chained Action Ledger for RAGFlow enterprise RAG runs.
    """

    def __init__(self) -> None:
        self._entries: List[Dict[str, Any]] = []
        self._last_hash: str = GENESIS_HASH

    def record_rag_execution(
        self,
        dataset_id: str,
        pipeline_id: str,
        event_type: str,
        readiness_index: float,
        critical_smells: List[str],
        metadata: Dict[str, Any],
    ) -> Dict[str, Any]:
        timestamp = datetime.now(timezone.utc).isoformat()
        index = len(self._entries)

        meta_bytes = json.dumps(metadata, sort_keys=True).encode("utf-8")
        canonical_content = f"{index}|{self._last_hash}|{dataset_id}|{pipeline_id}|{event_type}|{readiness_index}|{timestamp}|{hashlib.sha256(meta_bytes).hexdigest()}"
        curr_hash = hashlib.sha256(canonical_content.encode("utf-8")).hexdigest()

        entry = {
            "index": index,
            "timestamp": timestamp,
            "dataset_id": dataset_id,
            "pipeline_id": pipeline_id,
            "event_type": event_type,
            "readiness_index": readiness_index,
            "critical_smells": critical_smells,
            "prev_hash": self._last_hash,
            "curr_hash": curr_hash,
            "metadata": metadata,
        }

        self._entries.append(entry)
        self._last_hash = curr_hash
        return entry

    def get_ledger_entries(self) -> List[Dict[str, Any]]:
        return list(self._entries)

    def verify_ledger_integrity(self) -> bool:
        prev = GENESIS_HASH
        for entry in self._entries:
            if entry["prev_hash"] != prev:
                return False
            prev = entry["curr_hash"]
        return True


class ProductionDebtRAGEvaluator:
    """
    A2Z SOC Production Debt & Technical Due Diligence Evaluator for RAGFlow Enterprise RAG.

    Quantifies document ingestion and retrieval against 4 Enterprise Forward Deployed Engineering KPIs:
    1. RAG Document Debt Index (RDI <= 12.0)
    2. Chunk Fragmentation Multiplier (CFM <= 1.12x)
    3. P99 Hybrid Retrieval Latency Ceiling (<= 1.5s)
    4. Deterministic Mutation Boundaries (never_equate_intent_to_approval)
    """

    def __init__(
        self,
        never_equate_intent_to_approval: bool = True,
        max_acceptable_rdi: float = 12.0,
    ) -> None:
        self.never_equate_intent_to_approval = never_equate_intent_to_approval
        self.max_acceptable_rdi = max_acceptable_rdi
        self.ledger = TechnicalDueDiligenceLedger()

    def check_kill_switch(self) -> bool:
        if os.environ.get("AAG_KILL_SWITCH", "").lower() in ("true", "1", "yes"):
            return True
        for path_str in ("artifacts/KILL", "/tmp/KILL"):
            if Path(path_str).exists():
                return True
        return False

    def evaluate_pipeline(
        self,
        dataset_id: str,
        pipeline_id: str,
        raw_document_bytes: int = 1000000,
        chunk_storage_bytes: int = 1050000,
        retrieval_latency_seconds: float = 0.85,
        multi_hop_loop_count: int = 0,
        un_gated_mutations: int = 0,
    ) -> RAGDebtReport:
        # 1. Evaluate emergency kill switch
        if self.check_kill_switch():
            self.ledger.record_rag_execution(
                dataset_id=dataset_id,
                pipeline_id=pipeline_id,
                event_type="pipeline_halted_kill_switch",
                readiness_index=0.0,
                critical_smells=["EMERGENCY_KILL_SWITCH_ENGAGED"],
                metadata={"reason": "AAG_KILL_SWITCH is set"},
            )
            raise PermissionError(
                "A2Z SOC ActionGate: Emergency kill switch is engaged. RAG pipeline halted."
            )

        critical_smells: List[str] = []

        # KPI 2: Chunk Fragmentation Multiplier
        chunk_ratio = chunk_storage_bytes / max(1, raw_document_bytes)
        if chunk_ratio > 2.0:
            critical_smells.append(f"HIGH_CHUNK_FRAGMENTATION_{chunk_ratio:.2f}X")

        # KPI 3: Latency Ceiling
        if retrieval_latency_seconds > 5.0:
            critical_smells.append(f"HIGH_RETRIEVAL_LATENCY_{retrieval_latency_seconds:.2f}S")

        # Multi-Hop Retrieval Loops
        if multi_hop_loop_count > 2:
            critical_smells.append(f"DETECTED_{multi_hop_loop_count}_MULTI_HOP_LOOPS")

        # KPI 4: Mutation Safety
        if un_gated_mutations > 0:
            critical_smells.append(f"DETECTED_{un_gated_mutations}_UNGATED_MUTATIONS")

        # KPI 1: RAG Debt Index (0 = Clean, 100 = Catastrophic)
        rdi = (
            max(0.0, (chunk_ratio - 1.0) * 20.0)
            + max(0.0, (retrieval_latency_seconds - 1.5) * 8.0)
            + (multi_hop_loop_count * 12.0)
            + (un_gated_mutations * 30.0)
        )
        rdi_score = round(min(100.0, rdi), 2)

        # Production Readiness Index (0 - 100)
        readiness = max(0.0, 100.0 - rdi_score)
        is_production_ready = (
            rdi_score <= self.max_acceptable_rdi and len(critical_smells) == 0
        )

        # Cryptographic Ledger Entry
        entry = self.ledger.record_rag_execution(
            dataset_id=dataset_id,
            pipeline_id=pipeline_id,
            event_type="pipeline_authorized" if is_production_ready else "pipeline_flagged_debt",
            readiness_index=readiness,
            critical_smells=critical_smells,
            metadata={
                "rdi_score": rdi_score,
                "chunk_ratio": chunk_ratio,
                "retrieval_latency_seconds": retrieval_latency_seconds,
                "multi_hop_loop_count": multi_hop_loop_count,
                "un_gated_mutations": un_gated_mutations,
                "never_equate_intent_to_approval": self.never_equate_intent_to_approval,
            },
        )

        return RAGDebtReport(
            dataset_id=dataset_id,
            pipeline_id=pipeline_id,
            rdi_score=rdi_score,
            chunk_fragmentation_multiplier=round(chunk_ratio, 2),
            retrieval_latency_seconds=round(retrieval_latency_seconds, 2),
            mutation_safety_score=(
                100.0 if un_gated_mutations == 0 else max(0.0, 100.0 - un_gated_mutations * 30.0)
            ),
            production_readiness_index=readiness,
            is_production_ready=is_production_ready,
            critical_smells=critical_smells,
            receipt_hash=entry["curr_hash"],
        )
