from typing import Any, Dict, Optional

from api.db.db_models import RiskAiTask
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
