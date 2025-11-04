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

# from beartype import BeartypeConf
# from beartype.claw import beartype_all  # <-- you didn't sign up for this
# beartype_all(conf=BeartypeConf(violation_type=UserWarning))    # <-- emit warnings from all code


import sys
import threading
import time
import traceback

from api.db.services.connector_service import SyncLogsService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.log_utils import init_root_logger, get_project_base_directory
from api.utils.configs import show_configs
from common.data_source import BlobStorageConnector
import logging
import os
from datetime import datetime, timezone
import tracemalloc
import signal
import trio
import faulthandler
from api.db import FileSource, TaskStatus
from api import settings
from api.versions import get_ragflow_version

MAX_CONCURRENT_TASKS = int(os.environ.get('MAX_CONCURRENT_TASKS', "5"))
task_limiter = trio.Semaphore(MAX_CONCURRENT_TASKS)


class SyncBase:
    def __init__(self, conf: dict) -> None:
        self.conf = conf

    async def __call__(self, task: dict):
        SyncLogsService.start(task["id"])
        try:
            async with task_limiter:
                with trio.fail_after(task["timeout_secs"]):
                    task["poll_range_start"] = await self._run(task)
        except Exception as ex:
            msg = '\n'.join([
                ''.join(traceback.format_exception_only(None, ex)).strip(),
                ''.join(traceback.format_exception(None, ex, ex.__traceback__)).strip()
            ])
            SyncLogsService.update_by_id(task["id"], {"status": TaskStatus.FAIL, "full_exception_trace": msg})

        SyncLogsService.schedule(task["connector_id"], task["kb_id"], task["poll_range_start"])

    async def _run(self, task: dict):
        raise NotImplementedError


class S3(SyncBase):
    async def _run(self, task: dict):
        self.connector = BlobStorageConnector(
            bucket_type=self.conf.get("bucket_type", "s3"),
            bucket_name=self.conf["bucket_name"],
            prefix=self.conf.get("prefix", "")
        )
        self.connector.load_credentials(self.conf["credentials"])
        document_batch_generator = self.connector.load_from_state() if task["reindex"]=="1" or not task["poll_range_start"] \
            else  self.connector.poll_source(task["poll_range_start"].timestamp(), datetime.now(timezone.utc).timestamp())

        begin_info = "totally" if task["reindex"]=="1" or not task["poll_range_start"] else "from {}".format(task["poll_range_start"])
        logging.info("Connect to {}: {} {}".format(self.conf.get("bucket_type", "s3"),
                                                                  self.conf["bucket_name"],
                                                                  begin_info
                                                                  ))
        doc_num = 0
        next_update = datetime(1970, 1, 1, tzinfo=timezone.utc)
        if task["poll_range_start"]:
            next_update = task["poll_range_start"]
        for document_batch in document_batch_generator:
            min_update = min([doc.doc_updated_at for doc in document_batch])
            max_update = max([doc.doc_updated_at for doc in document_batch])
            next_update = max([next_update, max_update])
            docs = [{
                    "id": doc.id,
                    "connector_id": task["connector_id"],
                    "source": FileSource.S3,
                    "semantic_identifier": doc.semantic_identifier,
                    "extension": doc.extension,
                    "size_bytes": doc.size_bytes,
                    "doc_updated_at": doc.doc_updated_at,
                    "blob": doc.blob
                } for doc in document_batch]

            e, kb = KnowledgebaseService.get_by_id(task["kb_id"])
            err, dids = SyncLogsService.duplicate_and_parse(kb, docs, task["tenant_id"], f"{FileSource.S3}/{task['connector_id']}")
            SyncLogsService.increase_docs(task["id"], min_update, max_update, len(docs), "\n".join(err), len(err))
            doc_num += len(docs)

        logging.info("{} docs synchronized from {}: {} {}".format(doc_num, self.conf.get("bucket_type", "s3"),
                                                                  self.conf["bucket_name"],
                                                                  begin_info
                                                                  ))
        SyncLogsService.done(task["id"])
        return next_update


class Notion(SyncBase):

    async def __call__(self, task: dict):
        pass


class Discord(SyncBase):

    async def __call__(self, task: dict):
        pass


class Confluence(SyncBase):

    async def __call__(self, task: dict):
        pass


class Gmail(SyncBase):

    async def __call__(self, task: dict):
        pass


class GoogleDriver(SyncBase):

    async def __call__(self, task: dict):
        pass


class Jira(SyncBase):

    async def __call__(self, task: dict):
        pass


class SharePoint(SyncBase):

    async def __call__(self, task: dict):
        pass


class Slack(SyncBase):

    async def __call__(self, task: dict):
        pass


class Teams(SyncBase):

    async def __call__(self, task: dict):
        pass

func_factory = {
    FileSource.S3: S3,
    FileSource.NOTION: Notion,
    FileSource.DISCORD: Discord,
    FileSource.CONFLUENCE: Confluence,
    FileSource.GMAIL: Gmail,
    FileSource.GOOGLE_DRIVER: GoogleDriver,
    FileSource.JIRA: Jira,
    FileSource.SHAREPOINT: SharePoint,
    FileSource.SLACK: Slack,
    FileSource.TEAMS: Teams
}

async def dispatch_tasks():
    async with trio.open_nursery() as nursery:
        for task in SyncLogsService.list_sync_tasks():
            if task["poll_range_start"]:
                task["poll_range_start"] = task["poll_range_start"].astimezone(timezone.utc)
            if task["poll_range_end"]:
                task["poll_range_end"] = task["poll_range_end"].astimezone(timezone.utc)
            func = func_factory[task["source"]](task["config"])
            nursery.start_soon(func, task)
    await trio.sleep(1)


stop_event = threading.Event()


# SIGUSR1 handler: start tracemalloc and take snapshot
def start_tracemalloc_and_snapshot(signum, frame):
    if not tracemalloc.is_tracing():
        logging.info("start tracemalloc")
        tracemalloc.start()
    else:
        logging.info("tracemalloc is already running")

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    snapshot_file = f"snapshot_{timestamp}.trace"
    snapshot_file = os.path.abspath(os.path.join(get_project_base_directory(), "logs", f"{os.getpid()}_snapshot_{timestamp}.trace"))

    snapshot = tracemalloc.take_snapshot()
    snapshot.dump(snapshot_file)
    current, peak = tracemalloc.get_traced_memory()
    if sys.platform == "win32":
        import  psutil
        process = psutil.Process()
        max_rss = process.memory_info().rss / 1024
    else:
        import resource
        max_rss = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
    logging.info(f"taken snapshot {snapshot_file}. max RSS={max_rss / 1000:.2f} MB, current memory usage: {current / 10**6:.2f} MB, Peak memory usage: {peak / 10**6:.2f} MB")


# SIGUSR2 handler: stop tracemalloc
def stop_tracemalloc(signum, frame):
    if tracemalloc.is_tracing():
        logging.info("stop tracemalloc")
        tracemalloc.stop()
    else:
        logging.info("tracemalloc not running")



def signal_handler(sig, frame):
    logging.info("Received interrupt signal, shutting down...")
    stop_event.set()
    time.sleep(1)
    sys.exit(0)


CONSUMER_NO = "0" if len(sys.argv) < 2 else sys.argv[1]
CONSUMER_NAME = "data_sync_" + CONSUMER_NO


async def main():
    logging.info(r"""
  _____        _           _____                  
 |  __ \      | |         / ____|                 
 | |  | | __ _| |_ __ _  | (___  _   _ _ __   ___ 
 | |  | |/ _` | __/ _` |  \___ \| | | | '_ \ / __|
 | |__| | (_| | || (_| |  ____) | |_| | | | | (__ 
 |_____/ \__,_|\__\__,_| |_____/ \__, |_| |_|\___|
                                  __/ |           
                                 |___/                              
    """)
    logging.info(f'RAGFlow version: {get_ragflow_version()}')
    show_configs()
    settings.init_settings()
    if sys.platform != "win32":
        signal.signal(signal.SIGUSR1, start_tracemalloc_and_snapshot)
        signal.signal(signal.SIGUSR2, stop_tracemalloc)
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    while not stop_event.is_set():
        await dispatch_tasks()
    logging.error("BUG!!! You should not reach here!!!")


if __name__ == "__main__":
    faulthandler.enable()
    init_root_logger(CONSUMER_NAME)
    trio.run(main)
