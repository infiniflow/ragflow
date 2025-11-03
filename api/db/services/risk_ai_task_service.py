from typing import Any, Dict, Optional

from api.db.db_models import RiskAiTask, RiskAiTaskRow
from api.db.services.common_service import CommonService


class RiskAITaskStatus:
    PENDING = "pending"
    RUNNING = "running"
    SUCCESS = "success"
    FAILED = "failed"


class RiskAITaskService(CommonService):
    model = RiskAiTask

    @classmethod
    def create_task(cls, **kwargs) -> RiskAiTask:
        return cls.model.create(**kwargs)

    @classmethod
    def update_task(cls, task_id: str, values: Dict[str, Any]) -> int:
        return cls.model.update(**values).where(cls.model.id == task_id).execute()

    @classmethod
    def get_task(cls, task_id: str) -> Optional[RiskAiTask]:
        return cls.model.get_or_none(cls.model.id == task_id)


class RiskAITaskRowStatus:
    PENDING = "pending"
    RUNNING = "running"
    SUCCESS = "success"
    FAILED = "failed"


class RiskAITaskRowService(CommonService):
    model = RiskAiTaskRow

    @classmethod
    def create_rows(cls, rows: list[Dict[str, Any]]):
        if not rows:
            return
        cls.model.insert_many(rows).execute()

    @classmethod
    def update_row(cls, row_id: str, values: Dict[str, Any]) -> int:
        return cls.model.update(**values).where(cls.model.id == row_id).execute()

    @classmethod
    def get_rows_by_task(cls, task_id: str):
        return list(cls.model.select().where(cls.model.task_id == task_id).dicts())

    @classmethod
    def get_next_batch(cls, task_id: str, status: str, limit: int = 10):
        rows = (
            cls.model.select()
            .where((cls.model.task_id == task_id) & (cls.model.status == status))
            .order_by(cls.model.row_index.asc())
            .limit(limit)
        )
        return list(rows)
