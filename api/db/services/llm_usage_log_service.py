import logging
import time
from uuid import uuid4

from api.db.db_models import DB, LLMUsageLog
from api.db.services.common_service import CommonService


class LLMUsageLogService(CommonService):
    model = LLMUsageLog

    @classmethod
    @DB.connection_context()
    def create(
        cls,
        tenant_id: str,
        tenant_llm_id: int,
        model_type: str,
        total_tokens: int,
        user_id: str = None,
        biz_type: str = "other",
        biz_id: str = None,
        prompt_tokens: int = 0,
        completion_tokens: int = 0,
        cost: float = 0.0,
    ):
        """写入一条 LLM 调用明细记录。

        Args:
            tenant_id:        租户 ID
            tenant_llm_id:    TenantLLM.id（关联具体模型配置）
            model_type:       模型类型，如 "chat"/"embedding"/"rerank"
            total_tokens:     本次调用消耗的总 token 数
            user_id:          发起调用的用户 ID（可选）
            biz_type:         业务类型，如 "dialog"/"agent"/"document_parse"
            biz_id:           业务对象 ID，如 session_id/canvas_id/document_id
            prompt_tokens:    输入 token 数（Chat 模式有值，其他模式为 0）
            completion_tokens:输出 token 数（Chat 模式有值，其他模式为 0）
            cost:             本次调用费用（USD），LiteLLM 模式有值，其他暂为 0
        """
        try:
            cls.model.create(
                id=uuid4().hex,
                tenant_id=tenant_id,
                user_id=user_id,
                biz_type=biz_type,
                biz_id=biz_id,
                tenant_llm_id=tenant_llm_id,
                model_type=model_type,
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
                total_tokens=total_tokens,
                cost=cost,
                created_at=int(time.time() * 1000),
            )
        except Exception:
            logging.exception(
                "LLMUsageLogService.create failed for tenant_id=%s, biz_type=%s, biz_id=%s",
                tenant_id, biz_type, biz_id,
            )
