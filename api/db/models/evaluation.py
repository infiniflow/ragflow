from __future__ import annotations

from peewee import BigIntegerField, CharField, FloatField, IntegerField, TextField, ForeignKeyField

from api.db.base import DataBaseModel
from api.db.fields import JSONField
from api.db.models.dialog import Dialog


class EvaluationDataset(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    tenant_id = CharField(max_length=32, null=False, index=True, help_text="tenant ID")
    name = CharField(max_length=255, null=False, index=True, help_text="dataset name")
    description = TextField(null=True, help_text="dataset description")
    kb_ids = JSONField(null=False, help_text="knowledge base IDs to evaluate against")
    created_by = CharField(max_length=32, null=False, index=True, help_text="creator user ID")
    create_time = BigIntegerField(null=False, index=True, help_text="creation timestamp")
    update_time = BigIntegerField(null=False, help_text="last update timestamp")
    status = IntegerField(null=False, default=1, help_text="1=valid, 0=invalid")

    class Meta:
        db_table = "evaluation_datasets"


class EvaluationCase(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    dataset_id = ForeignKeyField(
        EvaluationDataset,
        column_name="dataset_id",
        null=False,
        index=True,
        help_text="FK to evaluation_datasets",
        on_delete="CASCADE",
    )
    question = TextField(null=False, help_text="test question")
    reference_answer = TextField(null=True, help_text="optional ground truth answer")
    relevant_doc_ids = JSONField(null=True, help_text="expected relevant document IDs")
    relevant_chunk_ids = JSONField(null=True, help_text="expected relevant chunk IDs")
    metadata = JSONField(null=True, help_text="additional context/tags")
    create_time = BigIntegerField(null=False, help_text="creation timestamp")

    class Meta:
        db_table = "evaluation_cases"


class EvaluationRun(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    dataset_id = ForeignKeyField(
        EvaluationDataset,
        column_name="dataset_id",
        null=False,
        index=True,
        help_text="FK to evaluation_datasets",
        on_delete="CASCADE",
    )
    dialog_id = ForeignKeyField(
        Dialog,
        column_name="dialog_id",
        null=False,
        index=True,
        help_text="dialog configuration being evaluated",
        on_delete="CASCADE",
    )
    name = CharField(max_length=255, null=False, help_text="run name")
    config_snapshot = JSONField(null=False, help_text="dialog config at time of evaluation")
    metrics_summary = JSONField(null=True, help_text="aggregated metrics")
    status = CharField(max_length=32, null=False, default="PENDING", help_text="PENDING/RUNNING/COMPLETED/FAILED")
    created_by = CharField(max_length=32, null=False, index=True, help_text="user who started the run")
    create_time = BigIntegerField(null=False, index=True, help_text="creation timestamp")
    complete_time = BigIntegerField(null=True, help_text="completion timestamp")

    class Meta:
        db_table = "evaluation_runs"


class EvaluationResult(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    run_id = ForeignKeyField(
        EvaluationRun,
        column_name="run_id",
        null=False,
        index=True,
        help_text="FK to evaluation_runs",
        on_delete="CASCADE",
    )
    case_id = ForeignKeyField(
        EvaluationCase,
        column_name="case_id",
        null=False,
        index=True,
        help_text="FK to evaluation_cases",
        on_delete="CASCADE",
    )
    generated_answer = TextField(null=False, help_text="generated answer")
    retrieved_chunks = JSONField(null=False, help_text="chunks that were retrieved")
    metrics = JSONField(null=False, help_text="all computed metrics")
    execution_time = FloatField(null=False, help_text="response time in seconds")
    token_usage = JSONField(null=True, help_text="prompt/completion tokens")
    create_time = BigIntegerField(null=False, help_text="creation timestamp")

    class Meta:
        db_table = "evaluation_results"
