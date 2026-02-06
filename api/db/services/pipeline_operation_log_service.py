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
import logging
import os
from datetime import datetime, timedelta

from peewee import fn

from api.db import VALID_PIPELINE_TASK_TYPES, PipelineTaskType
from api.db.db_models import DB, Document, PipelineOperationLog
from api.db.services.canvas_service import UserCanvasService
from api.db.services.common_service import CommonService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp, datetime_format


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
    def save(cls, **kwargs):
        """
        wrap this function in a transaction
        """
        sample_obj = cls.model(**kwargs).save(force_insert=True)
        return sample_obj

    @classmethod
    @DB.connection_context()
    def create(cls, document_id, pipeline_id, task_type, fake_document_ids=[], dsl: str = "{}"):
        referred_document_id = document_id

        if referred_document_id == GRAPH_RAPTOR_FAKE_DOC_ID and fake_document_ids:
            referred_document_id = fake_document_ids[0]
        ok, document = DocumentService.get_by_id(referred_document_id)
        if not ok:
            logging.warning(f"Document for referred_document_id {referred_document_id} not found")
            return None
        DocumentService.update_progress_immediately([document.to_dict()])
        ok, document = DocumentService.get_by_id(referred_document_id)
        if not ok:
            logging.warning(f"Document for referred_document_id {referred_document_id} not found")
            return None
        if document.progress not in [1, -1]:
            return None
        operation_status = document.run

        if pipeline_id:
            ok, user_pipeline = UserCanvasService.get_by_id(pipeline_id)
            if not ok:
                raise RuntimeError(f"Pipeline {pipeline_id} not found")
            tenant_id = user_pipeline.user_id
            title = user_pipeline.title
            avatar = user_pipeline.avatar
        else:
            ok, kb_info = KnowledgebaseService.get_by_id(document.kb_id)
            if not ok:
                raise RuntimeError(f"Cannot find dataset {document.kb_id} for referred_document {referred_document_id}")

            tenant_id = kb_info.tenant_id
            title = document.parser_id
            avatar = document.thumbnail

        if task_type not in VALID_PIPELINE_TASK_TYPES:
            raise ValueError(f"Invalid task type: {task_type}")

        if task_type in [PipelineTaskType.GRAPH_RAG, PipelineTaskType.RAPTOR, PipelineTaskType.MINDMAP]:
            finish_at = document.process_begin_at + timedelta(seconds=document.process_duration)
            if task_type == PipelineTaskType.GRAPH_RAG:
                KnowledgebaseService.update_by_id(
                    document.kb_id,
                    {"graphrag_task_finish_at": finish_at},
                )
            elif task_type == PipelineTaskType.RAPTOR:
                KnowledgebaseService.update_by_id(
                    document.kb_id,
                    {"raptor_task_finish_at": finish_at},
                )
            elif task_type == PipelineTaskType.MINDMAP:
                KnowledgebaseService.update_by_id(
                    document.kb_id,
                    {"mindmap_task_finish_at": finish_at},
                )

        log = dict(
            id=get_uuid(),
            document_id=document_id,  # GRAPH_RAPTOR_FAKE_DOC_ID or real document_id
            tenant_id=tenant_id,
            kb_id=document.kb_id,
            pipeline_id=pipeline_id,
            pipeline_title=title,
            parser_id=document.parser_id,
            document_name=document.name,
            document_suffix=document.suffix,
            document_type=document.type,
            source_from=document.source_type.split("/")[0],
            progress=document.progress,
            progress_msg=document.progress_msg,
            process_begin_at=document.process_begin_at,
            process_duration=document.process_duration,
            dsl=json.loads(dsl),
            task_type=task_type,
            operation_status=operation_status,
            avatar=avatar,
        )
        timestamp = current_timestamp()
        datetime_now = datetime_format(datetime.now())
        log["create_time"] = timestamp
        log["create_date"] = datetime_now
        log["update_time"] = timestamp
        log["update_date"] = datetime_now
        with DB.atomic():
            obj = cls.save(**log)

            limit = int(os.getenv("PIPELINE_OPERATION_LOG_LIMIT", 1000))
            total = cls.model.select().where(cls.model.kb_id == document.kb_id).count()

            if total > limit:
                keep_ids = [m.id for m in cls.model.select(cls.model.id).where(cls.model.kb_id == document.kb_id).order_by(cls.model.create_time.desc()).limit(limit)]

                deleted = cls.model.delete().where(cls.model.kb_id == document.kb_id, cls.model.id.not_in(keep_ids)).execute()
                logging.info(f"[PipelineOperationLogService] Cleaned {deleted} old logs, kept latest {limit} for {document.kb_id}")

        return obj

    @classmethod
    @DB.connection_context()
    def record_pipeline_operation(cls, document_id, pipeline_id, task_type, fake_document_ids=[]):
        return cls.create(document_id=document_id, pipeline_id=pipeline_id, task_type=task_type, fake_document_ids=fake_document_ids)

    @classmethod
    @DB.connection_context()
    def get_file_logs_by_kb_id(cls, kb_id, page_number, items_per_page, orderby, desc, keywords, operation_status, types, suffix, create_date_from=None, create_date_to=None):
        fields = cls.get_file_logs_fields()
        if keywords:
            logs = cls.model.select(*fields).where((cls.model.kb_id == kb_id), (fn.LOWER(cls.model.document_name).contains(keywords.lower())))
        else:
            logs = cls.model.select(*fields).where(cls.model.kb_id == kb_id)

        logs = logs.where(cls.model.document_id != GRAPH_RAPTOR_FAKE_DOC_ID)

        if operation_status:
            logs = logs.where(cls.model.operation_status.in_(operation_status))
        if types:
            logs = logs.where(cls.model.document_type.in_(types))
        if suffix:
            logs = logs.where(cls.model.document_suffix.in_(suffix))
        if create_date_from:
            logs = logs.where(cls.model.create_date >= create_date_from)
        if create_date_to:
            logs = logs.where(cls.model.create_date <= create_date_to)

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
    def get_documents_info(cls, id):
        fields = [Document.id, Document.name, Document.progress, Document.kb_id]
        return (
            cls.model.select(*fields)
            .join(Document, on=(cls.model.document_id == Document.id))
            .where(
                cls.model.id == id
            )
            .dicts()
        )

    @classmethod
    @DB.connection_context()
    def get_dataset_logs_by_kb_id(cls, kb_id, page_number, items_per_page, orderby, desc, operation_status, create_date_from=None, create_date_to=None):
        fields = cls.get_dataset_logs_fields()
        logs = cls.model.select(*fields).where((cls.model.kb_id == kb_id), (cls.model.document_id == GRAPH_RAPTOR_FAKE_DOC_ID))

        if operation_status:
            logs = logs.where(cls.model.operation_status.in_(operation_status))
        if create_date_from:
            logs = logs.where(cls.model.create_date >= create_date_from)
        if create_date_to:
            logs = logs.where(cls.model.create_date <= create_date_to)

        count = logs.count()
        if desc:
            logs = logs.order_by(cls.model.getter_by(orderby).desc())
        else:
            logs = logs.order_by(cls.model.getter_by(orderby).asc())

        if page_number and items_per_page:
            logs = logs.paginate(page_number, items_per_page)

        return list(logs.dicts()), count
