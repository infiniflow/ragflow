#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import logging
from datetime import datetime

from anthropic import BaseModel
from peewee import SQL, fn

from api.db import InputType, TaskStatus
from api.db.db_models import Connector, SyncLogs, Connector2Kb, Knowledgebase
from api.db.services.common_service import CommonService
from api.db.services.document_service import DocumentService
from api.db.services.file_service import FileService
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp, timestamp_to_date


class ConnectorService(CommonService):
    model = Connector

    @classmethod
    def resume(cls, connector_id, status):
        for c2k in Connector2KbService.query(connector_id=connector_id):
            task = SyncLogsService.get_latest_task(connector_id, c2k.kb_id)
            if not task:
                if status == TaskStatus.SCHEDULE:
                    SyncLogsService.schedule(connector_id, c2k.kb_id)

            if task.status == TaskStatus.DONE:
                if status == TaskStatus.SCHEDULE:
                    SyncLogsService.schedule(connector_id, c2k.kb_id, task.poll_range_end, total_docs_indexed=task.total_docs_indexed)

            task = task.to_dict()
            task["status"] = status
            SyncLogsService.update_by_id(task["id"], task)
        ConnectorService.update_by_id(connector_id, {"status": status})


    @classmethod
    def list(cls, tenant_id):
        fields = [
            cls.model.id,
            cls.model.name,
            cls.model.source,
            cls.model.status
        ]
        return cls.model.select(*fields).where(
            cls.model.tenant_id == tenant_id
        ).dicts()


class SyncLogsService(CommonService):
    model = SyncLogs

    @classmethod
    def list_sync_tasks(cls, connector_id=None, page_number=None, items_per_page=15):
        fields = [
            cls.model.id,
            cls.model.connector_id,
            cls.model.kb_id,
            cls.model.poll_range_start,
            cls.model.poll_range_end,
            cls.model.new_docs_indexed,
            cls.model.error_msg,
            cls.model.error_count,
            Connector.name,
            Connector.source,
            Connector.tenant_id,
            Connector.timeout_secs,
            Knowledgebase.name.alias("kb_name"),
            cls.model.from_beginning.alias("reindex"),
            cls.model.status
        ]
        if not connector_id:
            fields.append(Connector.config)
            
        query = cls.model.select(*fields)\
            .join(Connector, on=(cls.model.connector_id==Connector.id))\
            .join(Connector2Kb, on=(cls.model.kb_id==Connector2Kb.kb_id))\
            .join(Knowledgebase, on=(cls.model.kb_id==Knowledgebase.id))

        if connector_id:
            query = query.where(cls.model.connector_id == connector_id)
        else:
            interval_expr = SQL("INTERVAL `t2`.`refresh_freq` MINUTE")
            query = query.where(
                Connector.input_type == InputType.POLL,
                Connector.status == TaskStatus.SCHEDULE,
                cls.model.status == TaskStatus.SCHEDULE,
                cls.model.update_date < (fn.NOW() - interval_expr)
            )

        query = query.distinct().order_by(cls.model.update_time.desc())
        if page_number:
            query = query.paginate(page_number, items_per_page)

        return list(query.dicts())

    @classmethod
    def start(cls, id):
        cls.update_by_id(id, {"status": TaskStatus.RUNNING, "time_started": datetime.now().strftime('%Y-%m-%d %H:%M:%S') })

    @classmethod
    def done(cls, id):
        cls.update_by_id(id, {"status": TaskStatus.DONE})

    @classmethod
    def schedule(cls, connector_id, kb_id, poll_range_start=None, reindex=False, total_docs_indexed=0):
        try:
            e = cls.query(kb_id=kb_id, connector_id=connector_id, status=TaskStatus.SCHEDULE)
            if e:
                logging.warning(f"{kb_id}--{connector_id} has already had a scheduling sync task which is abnormal.")
                return None
            reindex = "1" if reindex else "0"
            return cls.save(**{
                "id": get_uuid(),
                "kb_id": kb_id, "status": TaskStatus.SCHEDULE, "connector_id": connector_id,
                "poll_range_start": poll_range_start, "from_beginning": reindex,
                "total_docs_indexed": total_docs_indexed
            })
        except Exception as e:
            logging.exception(e)
            task = cls.get_latest_task(connector_id, kb_id)
            if task:
                cls.model.update(status=TaskStatus.SCHEDULE,
                                 poll_range_start=poll_range_start,
                                 error_msg=cls.model.error_msg + str(e),
                                 full_exception_trace=cls.model.full_exception_trace + str(e)
                                 ) \
                .where(cls.model.id == task.id).execute()

    @classmethod
    def increase_docs(cls, id, min_update, max_update, doc_num, err_msg="", error_count=0):
        cls.model.update(new_docs_indexed=cls.model.new_docs_indexed + doc_num,
                         total_docs_indexed=cls.model.total_docs_indexed + doc_num,
                         poll_range_start=fn.COALESCE(fn.LEAST(cls.model.poll_range_start,min_update), min_update),
                         poll_range_end=fn.COALESCE(fn.GREATEST(cls.model.poll_range_end, max_update), max_update),
                         error_msg=cls.model.error_msg + err_msg,
                         error_count=cls.model.error_count + error_count,
                         update_time=current_timestamp(),
                         update_date=timestamp_to_date(current_timestamp())
                         )\
            .where(cls.model.id == id).execute()

    @classmethod
    def duplicate_and_parse(cls, kb, docs, tenant_id, src):
        if not docs:
            return None

        class FileObj(BaseModel):
            filename: str
            blob: bytes

            def read(self) -> bytes:
                return self.blob

        errs = []
        files = [FileObj(filename=d["semantic_identifier"]+f".{d['extension']}", blob=d["blob"]) for d in docs]
        doc_ids = []
        err, doc_blob_pairs = FileService.upload_document(kb, files, tenant_id, src)
        errs.extend(err)
        if not err:
            kb_table_num_map = {}
            for doc, _ in doc_blob_pairs:
                DocumentService.run(tenant_id, doc, kb_table_num_map)
                doc_ids.append(doc["id"])

        return errs, doc_ids

    @classmethod
    def get_latest_task(cls, connector_id, kb_id):
        return cls.model.select().where(
            cls.model.connector_id==connector_id,
            cls.model.kb_id == kb_id
        ).order_by(cls.model.update_time.desc()).first()


class Connector2KbService(CommonService):
    model = Connector2Kb

    @classmethod
    def link_kb(cls, conn_id:str, kb_ids: list[str], tenant_id:str):
        arr = cls.query(connector_id=conn_id)
        old_kb_ids = [a.kb_id for a in arr]
        for kb_id in kb_ids:
            if kb_id in old_kb_ids:
                continue
            cls.save(**{
                "id": get_uuid(),
                "connector_id": conn_id,
                "kb_id": kb_id
            })
            SyncLogsService.schedule(conn_id, kb_id, reindex=True)

        errs = []
        e, conn = ConnectorService.get_by_id(conn_id)
        for kb_id in old_kb_ids:
            if kb_id in kb_ids:
                continue
            cls.filter_delete([cls.model.kb_id==kb_id, cls.model.connector_id==conn_id])
            SyncLogsService.filter_update([SyncLogs.connector_id==conn_id, SyncLogs.kb_id==kb_id, SyncLogs.status==TaskStatus.SCHEDULE], {"status": TaskStatus.CANCEL})
            docs = DocumentService.query(source_type=f"{conn.source}/{conn.id}")
            err = FileService.delete_docs([d.id for d in docs], tenant_id)
            if err:
                errs.append(err)
        return "\n".join(errs)

