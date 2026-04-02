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
        session_id: str = None,
        prompt_tokens: int = 0,
        completion_tokens: int = 0,
        cost: float = 0.0,
    ):
        """Create one detailed LLM usage record.

        Args:
            tenant_id:         Tenant ID.
            tenant_llm_id:     TenantLLM.id linked to the concrete model configuration.
            model_type:        Model type, such as "chat", "embedding", or "rerank".
            total_tokens:      Total tokens consumed by this call.
            user_id:           User ID that initiated the call, if any.
            biz_type:          Business type, such as "dialog", "agent", or "document_parse".
            biz_id:            Business object ID, such as dialog.id, canvas.id, or document_id.
            session_id:        Session ID, such as Conversation.id or API4Conversation.id.
            prompt_tokens:     Input token count. Non-chat modes usually use 0.
            completion_tokens: Output token count. Non-chat modes usually use 0.
            cost:              Call cost in USD. Populated in LiteLLM mode; otherwise currently 0.
        """
        try:
            cls.model.create(
                id=uuid4().hex,
                tenant_id=tenant_id,
                user_id=user_id,
                biz_type=biz_type,
                biz_id=biz_id,
                session_id=session_id,
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
                "LLMUsageLogService.create failed for tenant_id=%s, biz_type=%s, biz_id=%s, session_id=%s",
                tenant_id, biz_type, biz_id, session_id,
            )
