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
import datetime
import json
import logging
import random
import time
from timeit import default_timer as timer
import trio

from agent.canvas import Graph
from api.db import PipelineTaskType
from api.db.services.document_service import DocumentService
from api.db.services.task_service import has_canceled
from api.db.services.pipeline_operation_log_service import PipelineOperationLogService
from rag.utils.redis_conn import REDIS_CONN


class Pipeline(Graph):
    def __init__(self, dsl: str|dict, tenant_id=None, doc_id=None, task_id=None, flow_id=None):
        if isinstance(dsl, dict):
            dsl = json.dumps(dsl, ensure_ascii=False)
        super().__init__(dsl, tenant_id, task_id)
        self._doc_id = doc_id
        if self._doc_id == "x":
            self._doc_id = None
        self._flow_id = flow_id
        self._kb_id = None
        if self._doc_id:
            self._kb_id = DocumentService.get_knowledgebase_id(doc_id)
            if not self._kb_id:
                self._doc_id = None

    def callback(self, component_name: str, progress: float | int | None = None, message: str = "") -> None:
        from rag.svr.task_executor import TaskCanceledException
        log_key = f"{self._flow_id}-{self.task_id}-logs"
        timestamp = timer()
        if has_canceled(self.task_id):
            progress = -1
            message += "[CANCEL]"
        try:
            bin = REDIS_CONN.get(log_key)
            obj = json.loads(bin.encode("utf-8"))
            if obj:
                if obj[-1]["component_id"] == component_name:
                    obj[-1]["trace"].append(
                        {
                            "progress": progress,
                            "message": message,
                            "datetime": datetime.datetime.now().strftime("%H:%M:%S"),
                            "timestamp": timestamp,
                            "elapsed_time": timestamp - obj[-1]["trace"][-1]["timestamp"],
                        }
                    )
                else:
                    obj.append(
                        {
                            "component_id": component_name,
                            "trace": [{"progress": progress, "message": message, "datetime": datetime.datetime.now().strftime("%H:%M:%S"), "timestamp": timestamp, "elapsed_time": 0}],
                        }
                    )
            else:
                obj = [
                    {
                        "component_id": component_name,
                        "trace": [{"progress": progress, "message": message, "datetime": datetime.datetime.now().strftime("%H:%M:%S"), "timestamp": timestamp, "elapsed_time": 0}],
                    }
                ]
            REDIS_CONN.set_obj(log_key, obj, 60 * 30)
            if self._doc_id:
                percentage = 1.0 / len(self.components.items())
                msg = ""
                finished = 0.0
                for o in obj:
                    if o["component_id"] == "END":
                        continue
                    msg += f"\n[{o['component_id']}]:\n"
                    for t in o["trace"]:
                        msg += "%s: %s\n" % (t["datetime"], t["message"])
                        if t["progress"] < 0:
                            finished = -1
                            break
                    if finished < 0:
                        break
                    finished += o["trace"][-1]["progress"] * percentage
                DocumentService.update_by_id(self._doc_id, {"progress": finished, "progress_msg": msg})
        except Exception as e:
            logging.exception(e)

        if has_canceled(self.task_id):
            raise TaskCanceledException(message)

    def fetch_logs(self):
        log_key = f"{self._flow_id}-{self.task_id}-logs"
        try:
            bin = REDIS_CONN.get(log_key)
            if bin:
                return json.loads(bin.encode("utf-8"))
        except Exception as e:
            logging.exception(e)
        return []

    def reset(self):
        super().reset()
        log_key = f"{self._flow_id}-{self.task_id}-logs"
        try:
            REDIS_CONN.set_obj(log_key, [], 60 * 10)
        except Exception as e:
            logging.exception(e)

    async def run(self, **kwargs):
        st = time.perf_counter()
        if not self.path:
            self.path.append("File")

        if self._doc_id:
            DocumentService.update_by_id(
                self._doc_id, {"progress": random.randint(0, 5) / 100.0, "progress_msg": "Start the pipeline...", "process_begin_at": datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")}
            )

        self.error = ""
        idx = len(self.path) - 1
        if idx == 0:
            cpn_obj = self.get_component_obj(self.path[0])
            await cpn_obj.invoke(**kwargs)
            if cpn_obj.error():
                self.error = "[ERROR]" + cpn_obj.error()
            else:
                idx += 1
                self.path.extend(cpn_obj.get_downstream())

        while idx < len(self.path) and not self.error:
            last_cpn = self.get_component_obj(self.path[idx - 1])
            cpn_obj = self.get_component_obj(self.path[idx])

            async def invoke():
                nonlocal last_cpn, cpn_obj
                await cpn_obj.invoke(**last_cpn.output())

            async with trio.open_nursery() as nursery:
                nursery.start_soon(invoke)
            if cpn_obj.error():
                self.error = "[ERROR]" + cpn_obj.error()
                self.callback(cpn_obj.component_name, -1, self.error)
                break
            idx += 1
            self.path.extend(cpn_obj.get_downstream())

        self.callback("END", 1, json.dumps(self.get_component_obj(self.path[-1]).output(), ensure_ascii=False))

        if self._doc_id:
            DocumentService.update_by_id(
                self._doc_id,
                {
                    "progress": 1 if not self.error else -1,
                    "progress_msg": "Pipeline finished...\n" + self.error,
                    "process_duration": time.perf_counter() - st,
                },
            )

            PipelineOperationLogService.create(document_id=self._doc_id, pipeline_id=self._flow_id, task_type=PipelineTaskType.PARSE)
