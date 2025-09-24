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
    model = PipelineOperationLog

    @classmethod
    def get_file_logs_fields(cls):
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
    def get_dataset_logs_fields(cls):
        return [
            cls.model.id,
            cls.model.tenant_id,
            cls.model.kb_id,
            cls.model.progress,
            cls.model.progress_msg,
            cls.model.process_begin_at,
            cls.model.process_duration,
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
    @DB.connection_context()
    def create(cls, document_id, pipeline_id, task_type, fake_document_name=""):
        from rag.flow.pipeline import Pipeline

        tenant_id = ""
        title = ""
        avatar = ""
        dsl = ""
        operation_status = ""
        referred_document_id = document_id

        if referred_document_id == "x" and fake_document_name:
            referred_document_id = DocumentService.get_doc_id_by_doc_name(fake_document_name) or document_id
        ok, document = DocumentService.get_by_id(referred_document_id)
        if not ok:
            raise RuntimeError(f"Document for referred_document_id {referred_document_id} not found")
        DocumentService.update_progress_immediately([document.to_dict()])
        ok, document = DocumentService.get_by_id(referred_document_id)
        if not ok:
            raise RuntimeError(f"Document for referred_document_id {referred_document_id} not found")
        if document.progress not in [1, -1]:
            return
        operation_status = document.run

        if pipeline_id:
            ok, user_pipeline = UserCanvasService.get_by_id(pipeline_id)
            if not ok:
                raise RuntimeError(f"Pipeline {pipeline_id} not found")

            pipeline = Pipeline(dsl=json.dumps(user_pipeline.dsl), tenant_id=user_pipeline.user_id, doc_id=referred_document_id, task_id="", flow_id=pipeline_id)

            tenant_id = user_pipeline.user_id
            title = user_pipeline.title
            avatar = user_pipeline.avatar
            dsl = json.loads(str(pipeline))
        else:
            ok, kb_info = KnowledgebaseService.get_by_id(document.kb_id)
            if not ok:
                raise RuntimeError(f"Cannot find knowledge base {document.kb_id} for referred_document {referred_document_id}")

            tenant_id = kb_info.tenant_id
            title = document.name
            avatar = document.thumbnail

        if task_type not in VALID_PIPELINE_TASK_TYPES:
            raise ValueError(f"Invalid task type: {task_type}")

        log = dict(
            id=get_uuid(),
            document_id=document_id,  # "x" or real document_id
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
            operation_status=operation_status,
            avatar=avatar,
        )
        log["create_time"] = current_timestamp()
        log["create_date"] = datetime_format(datetime.now())
        log["update_time"] = current_timestamp()
        log["update_date"] = datetime_format(datetime.now())
        obj = cls.save(**log)
        return obj

    @classmethod
    @DB.connection_context()
    def record_pipeline_operation(cls, document_id, pipeline_id, task_type, fake_document_name=""):
        return cls.create(document_id=document_id, pipeline_id=pipeline_id, task_type=task_type, fake_document_name=fake_document_name)

    @classmethod
    @DB.connection_context()
    def get_file_logs_by_kb_id(cls, kb_id, page_number, items_per_page, orderby, desc, keywords, operation_status, types, suffix):
        fields = cls.get_file_logs_fields()
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

    @classmethod
    @DB.connection_context()
    def get_dataset_logs_by_kb_id(cls, kb_id, page_number, items_per_page, orderby, desc, operation_status):
        fields = cls.get_dataset_logs_fields()
        logs = cls.model.select(*fields).where((cls.model.kb_id == kb_id), (cls.model.document_id == "x"))

        if operation_status:
            logs = logs.where(cls.model.operation_status.in_(operation_status))

        count = logs.count()
        if desc:
            logs = logs.order_by(cls.model.getter_by(orderby).desc())
        else:
            logs = logs.order_by(cls.model.getter_by(orderby).asc())

        if page_number and items_per_page:
            logs = logs.paginate(page_number, items_per_page)

        return list(logs.dicts()), count
