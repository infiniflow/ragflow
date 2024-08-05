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
import os
import random

from api.db.db_utils import bulk_insert_into_db
from deepdoc.parser import PdfParser
from peewee import JOIN
from api.db.db_models import DB, File2Document, File
from api.db import StatusEnum, FileType, TaskStatus
from api.db.db_models import Task, Document, Knowledgebase, Tenant
from api.db.services.common_service import CommonService
from api.db.services.document_service import DocumentService
from api.utils import current_timestamp, get_uuid
from deepdoc.parser.excel_parser import RAGFlowExcelParser
from rag.settings import SVR_QUEUE_NAME
from rag.utils.minio_conn import MINIO
from rag.utils.redis_conn import REDIS_CONN


class TaskService(CommonService):
    model = Task

    @classmethod
    @DB.connection_context()
    def get_tasks(cls, task_id):
        fields = [
            cls.model.id,
            cls.model.doc_id,
            cls.model.from_page,
            cls.model.to_page,
            Document.kb_id,
            Document.parser_id,
            Document.parser_config,
            Document.name,
            Document.type,
            Document.location,
            Document.size,
            Knowledgebase.tenant_id,
            Knowledgebase.language,
            Knowledgebase.embd_id,
            Tenant.img2txt_id,
            Tenant.asr_id,
            Tenant.llm_id,
            cls.model.update_time]
        docs = cls.model.select(*fields) \
            .join(Document, on=(cls.model.doc_id == Document.id)) \
            .join(Knowledgebase, on=(Document.kb_id == Knowledgebase.id)) \
            .join(Tenant, on=(Knowledgebase.tenant_id == Tenant.id)) \
            .where(cls.model.id == task_id)
        docs = list(docs.dicts())
        if not docs: return []

        cls.model.update(progress_msg=cls.model.progress_msg + "\n" + "Task has been received.",
                         progress=random.random() / 10.).where(
            cls.model.id == docs[0]["id"]).execute()
        return docs

    @classmethod
    @DB.connection_context()
    def get_ongoing_doc_name(cls):
        with DB.lock("get_task", -1):
            docs = cls.model.select(*[Document.id, Document.kb_id, Document.location, File.parent_id]) \
                .join(Document, on=(cls.model.doc_id == Document.id)) \
                .join(File2Document, on=(File2Document.document_id == Document.id), join_type=JOIN.LEFT_OUTER) \
                .join(File, on=(File2Document.file_id == File.id), join_type=JOIN.LEFT_OUTER) \
                .where(
                    Document.status == StatusEnum.VALID.value,
                    Document.run == TaskStatus.RUNNING.value,
                    ~(Document.type == FileType.VIRTUAL.value),
                    cls.model.progress < 1,
                    cls.model.create_time >= current_timestamp() - 1000 * 600
                )
            docs = list(docs.dicts())
            if not docs: return []

            return list(set([(d["parent_id"] if d["parent_id"] else d["kb_id"], d["location"]) for d in docs]))

    @classmethod
    @DB.connection_context()
    def do_cancel(cls, id):
        try:
            task = cls.model.get_by_id(id)
            _, doc = DocumentService.get_by_id(task.doc_id)
            return doc.run == TaskStatus.CANCEL.value or doc.progress < 0
        except Exception as e:
            pass
        return False

    @classmethod
    @DB.connection_context()
    def update_progress(cls, id, info):
        if os.environ.get("MACOS"):
            if info["progress_msg"]:
                cls.model.update(progress_msg=cls.model.progress_msg + "\n" + info["progress_msg"]).where(
                    cls.model.id == id).execute()
            if "progress" in info:
                cls.model.update(progress=info["progress"]).where(
                    cls.model.id == id).execute()
            return

        with DB.lock("update_progress", -1):
            if info["progress_msg"]:
                cls.model.update(progress_msg=cls.model.progress_msg + "\n" + info["progress_msg"]).where(
                    cls.model.id == id).execute()
            if "progress" in info:
                cls.model.update(progress=info["progress"]).where(
                    cls.model.id == id).execute()


def queue_tasks(doc, bucket, name):
    def new_task():
        nonlocal doc
        return {
            "id": get_uuid(),
            "doc_id": doc["id"]
        }
    tsks = []

    if doc["type"] == FileType.PDF.value:
        file_bin = MINIO.get(bucket, name)
        do_layout = doc["parser_config"].get("layout_recognize", True)
        pages = PdfParser.total_page_number(doc["name"], file_bin)
        page_size = doc["parser_config"].get("task_page_size", 12)
        if doc["parser_id"] == "paper":
            page_size = doc["parser_config"].get("task_page_size", 22)
        if doc["parser_id"] == "one":
            page_size = 1000000000
        if doc["parser_id"] == "knowledge_graph":
            page_size = 1000000000
        if not do_layout:
            page_size = 1000000000
        page_ranges = doc["parser_config"].get("pages")
        if not page_ranges:
            page_ranges = [(1, 100000)]
        for s, e in page_ranges:
            s -= 1
            s = max(0, s)
            e = min(e - 1, pages)
            for p in range(s, e, page_size):
                task = new_task()
                task["from_page"] = p
                task["to_page"] = min(p + page_size, e)
                tsks.append(task)

    elif doc["parser_id"] == "table":
        file_bin = MINIO.get(bucket, name)
        rn = RAGFlowExcelParser.row_number(
            doc["name"], file_bin)
        for i in range(0, rn, 3000):
            task = new_task()
            task["from_page"] = i
            task["to_page"] = min(i + 3000, rn)
            tsks.append(task)
    else:
        tsks.append(new_task())

    bulk_insert_into_db(Task, tsks, True)
    DocumentService.begin2parse(doc["id"])

    for t in tsks:
        assert REDIS_CONN.queue_product(SVR_QUEUE_NAME, message=t), "Can't access Redis. Please check the Redis' status."
