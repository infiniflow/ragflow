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
import os
import time
import random
from datetime import datetime
from api.db.db_models import Task
from api.db.db_utils import bulk_insert_into_db
from api.db.services.task_service import TaskService
from deepdoc.parser import PdfParser
from deepdoc.parser.excel_parser import HuExcelParser
from rag.settings import cron_logger
from rag.utils import MINIO
from rag.utils import findMaxTm
import pandas as pd
from api.db import FileType, TaskStatus
from api.db.services.document_service import DocumentService
from api.settings import database_logger
from api.utils import get_format_time, get_uuid
from api.utils.file_utils import get_project_base_directory


def collect(tm):
    docs = DocumentService.get_newly_uploaded(tm)
    if len(docs) == 0:
        return pd.DataFrame()
    docs = pd.DataFrame(docs)
    mtm = docs["update_time"].max()
    cron_logger.info("TOTAL:{}, To:{}".format(len(docs), mtm))
    return docs


def set_dispatching(docid):
    try:
        DocumentService.update_by_id(
            docid, {"progress": random.random() * 1 / 100.,
                    "progress_msg": "Task dispatched...",
                    "process_begin_at": get_format_time()
                    })
    except Exception as e:
        cron_logger.error("set_dispatching:({}), {}".format(docid, str(e)))


def dispatch():
    tm_fnm = os.path.join(
        get_project_base_directory(),
        "rag/res",
        f"broker.tm")
    tm = findMaxTm(tm_fnm)
    rows = collect(tm)
    if len(rows) == 0:
        return

    tmf = open(tm_fnm, "a+")
    for _, r in rows.iterrows():
        try:
            tsks = TaskService.query(doc_id=r["id"])
            if tsks:
                for t in tsks:
                    TaskService.delete_by_id(t.id)
        except Exception as e:
            cron_logger.exception(e)

        def new_task():
            nonlocal r
            return {
                "id": get_uuid(),
                "doc_id": r["id"]
            }

        tsks = []
        try:
            if r["type"] == FileType.PDF.value:
                do_layout = r["parser_config"].get("layout_recognize", True)
                pages = PdfParser.total_page_number(
                        r["name"], MINIO.get(r["kb_id"], r["location"]))
                page_size = r["parser_config"].get("task_page_size", 12)
                if r["parser_id"] == "paper":
                    page_size = r["parser_config"].get("task_page_size", 22)
                if r["parser_id"] == "one":
                    page_size = 1000000000
                if not do_layout:
                    page_size = 1000000000
                page_ranges = r["parser_config"].get("pages")
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

            elif r["parser_id"] == "table":
                rn = HuExcelParser.row_number(
                    r["name"], MINIO.get(
                        r["kb_id"], r["location"]))
                for i in range(0, rn, 3000):
                    task = new_task()
                    task["from_page"] = i
                    task["to_page"] = min(i + 3000, rn)
                    tsks.append(task)
            else:
                tsks.append(new_task())

            bulk_insert_into_db(Task, tsks, True)
            set_dispatching(r["id"])
        except Exception as e:
            cron_logger.exception(e)

        tmf.write(str(r["update_time"]) + "\n")
    tmf.close()


def update_progress():
    docs = DocumentService.get_unfinished_docs()
    for d in docs:
        try:
            tsks = TaskService.query(doc_id=d["id"], order_by=Task.create_time)
            if not tsks:
                continue
            msg = []
            prg = 0
            finished = True
            bad = 0
            status = TaskStatus.RUNNING.value
            for t in tsks:
                if 0 <= t.progress < 1:
                    finished = False
                prg += t.progress if t.progress >= 0 else 0
                msg.append(t.progress_msg)
                if t.progress == -1:
                    bad += 1
            prg /= len(tsks)
            if finished and bad:
                prg = -1
                status = TaskStatus.FAIL.value
            elif finished:
                status = TaskStatus.DONE.value

            msg = "\n".join(msg)
            info = {
                "process_duation": datetime.timestamp(
                    datetime.now()) -
                d["process_begin_at"].timestamp(),
                "run": status}
            if prg != 0:
                info["progress"] = prg
            if msg:
                info["progress_msg"] = msg
            DocumentService.update_by_id(d["id"], info)
        except Exception as e:
            cron_logger.error("fetch task exception:" + str(e))


if __name__ == "__main__":
    peewee_logger = logging.getLogger('peewee')
    peewee_logger.propagate = False
    peewee_logger.addHandler(database_logger.handlers[0])
    peewee_logger.setLevel(database_logger.level)

    while True:
        dispatch()
        time.sleep(1)
        update_progress()
