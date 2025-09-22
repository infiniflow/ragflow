#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import json
from datetime import datetime

from peewee import fn

from api.db import VALID_PIPELINE_TASK_TYPES
from api.db.db_models import DB, PipelineOperationLog
from api.db.services.canvas_service import UserCanvasService
from api.db.services.common_service import CommonService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils import current_timestamp, datetime_format, get_uuid


class PipelineOperationLogService(CommonService):
    """Service class for managing MCP server related database operations.

    This class extends CommonService to provide specialized functionality for MCP server management,
    including MCP server creation, updates, and deletions.

    Attributes:
        model: The MCPServer model class for database operations.
    """

    model = PipelineOperationLog

    @classmethod
    def get_cls_model_fields(cls):
        return [
            cls.model.id,
            cls.model.document_id,
            cls.model.tenant_id,
            cls.model.kb_id,
            cls.model.pipeline_id,
            cls.model.pipeline_title,
            cls.model.parser_id,
            cls.model.document_name,
            cls.model.document_suffix,
            cls.model.document_type,
            cls.model.source_from,
            cls.model.progress,
            cls.model.progress_msg,
            cls.model.process_begin_at,
            cls.model.process_duration,
            cls.model.dsl,
            cls.model.task_type,
            cls.model.operation_status,
            cls.model.avatar,
            cls.model.status,
            cls.model.create_time,
            cls.model.create_date,
            cls.model.update_time,
            cls.model.update_date,
        ]

    @classmethod
    def create(cls, document_id, pipeline_id, task_type):
        from rag.flow.pipeline import Pipeline

        tenant_id = ""
        title = ""
        avatar = ""
        dsl = ""

        ok, document = DocumentService.get_by_id(document_id)
        if not ok:
            raise RuntimeError(f"Document {document_id} not found")

        if pipeline_id:
            ok, user_pipeline = UserCanvasService.get_by_id(pipeline_id)
            if not ok:
                raise RuntimeError(f"Pipeline {pipeline_id} not found")

            pipeline = Pipeline(dsl=json.dumps(user_pipeline.dsl), tenant_id=user_pipeline.user_id, doc_id=document_id, task_id="", flow_id=pipeline_id)

            tenant_id = user_pipeline.user_id
            title = user_pipeline.title
            avatar = user_pipeline.avatar
            dsl = json.loads(str(pipeline))
        else:
            ok, kb_info = KnowledgebaseService.get_by_id(document.kb_id)
            if not ok:
                raise RuntimeError(f"Cannot find knowledge base {document.kb_id} for document {document_id}")

            tenant_id = kb_info.tenant_id
            title = document.name
            avatar = document.thumbnail

        if task_type not in VALID_PIPELINE_TASK_TYPES:
            raise ValueError(f"Invalid task type: {task_type}")

        log = dict(
            id=get_uuid(),
            document_id=document_id,
            tenant_id=tenant_id,
            kb_id=document.kb_id,
            pipeline_id=pipeline_id,
            pipeline_title=title,
            parser_id=document.parser_id,
            document_name=document.name,
            document_suffix=document.suffix,
            document_type=document.type,
            source_from="",  # TODO: add in the future
            progress=document.progress,
            progress_msg=document.progress_msg,
            process_begin_at=document.process_begin_at,
            process_duration=document.process_duration,
            dsl=dsl,
            task_type=task_type,
            operation_status=document.run,
            avatar=avatar,
        )
        log["create_time"] = current_timestamp()
        log["create_date"] = datetime_format(datetime.now())
        log["update_time"] = current_timestamp()
        log["update_date"] = datetime_format(datetime.now())
        print(f"ready to save {log}", flush=True)
        obj = cls.save(**log)
        return obj

    @classmethod
    @DB.connection_context()
    def get_by_kb_id(cls, kb_id, page_number, items_per_page, orderby, desc, keywords, operation_status, types, suffix):
        fields = cls.get_cls_model_fields()
        if keywords:
            logs = cls.model.select(*fields).where((cls.model.kb_id == kb_id), (fn.LOWER(cls.model.document_name).contains(keywords.lower())))
        else:
            logs = cls.model.select(*fields).where(cls.model.kb_id == kb_id)

        if operation_status:
            logs = logs.where(cls.model.operation_status.in_(operation_status))
        if types:
            logs = logs.where(cls.model.document_type.in_(types))
        if suffix:
            logs = logs.where(cls.model.document_suffix.in_(suffix))

        count = logs.count()
        if desc:
            logs = logs.order_by(cls.model.getter_by(orderby).desc())
        else:
            logs = logs.order_by(cls.model.getter_by(orderby).asc())

        if page_number and items_per_page:
            logs = logs.paginate(page_number, items_per_page)

        return list(logs.dicts()), count
