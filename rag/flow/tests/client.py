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
import argparse
import json
import os
import time
from concurrent.futures import ThreadPoolExecutor

import trio

from common import settings
from rag.flow.pipeline import Pipeline


def print_logs(pipeline: Pipeline):
    last_logs = "[]"
    while True:
        time.sleep(5)
        logs = pipeline.fetch_logs()
        logs_str = json.dumps(logs, ensure_ascii=False)
        if logs_str != last_logs:
            print(logs_str)
        last_logs = logs_str


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    dsl_default_path = os.path.join(
        os.path.dirname(os.path.realpath(__file__)),
        "dsl_examples",
        "general_pdf_all.json",
    )
    parser.add_argument("-s", "--dsl", default=dsl_default_path, help="input dsl", action="store", required=False)
    parser.add_argument("-d", "--doc_id", default=False, help="Document ID", action="store", required=True)
    parser.add_argument("-t", "--tenant_id", default=False, help="Tenant ID", action="store", required=True)
    args = parser.parse_args()

    settings.init_settings()
    pipeline = Pipeline(open(args.dsl, "r").read(), tenant_id=args.tenant_id, doc_id=args.doc_id, task_id="xxxx", flow_id="xxx")
    pipeline.reset()

    exe = ThreadPoolExecutor(max_workers=5)
    thr = exe.submit(print_logs, pipeline)

    # queue_dataflow(dsl=open(args.dsl, "r").read(), tenant_id=args.tenant_id, doc_id=args.doc_id, task_id="xxxx", flow_id="xxx", priority=0)

    trio.run(pipeline.run)
    thr.result()
