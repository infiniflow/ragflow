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
#  limitations under the License
#
import json

from flask_login import login_required

from api.db.services.knowledgebase_service import KnowledgebaseService
from api.settings import DATABASE_TYPE
from api.utils.api_utils import get_json_result
from api.versions import get_rag_version
from rag.settings import SVR_QUEUE_NAME
from rag.utils.es_conn import ELASTICSEARCH
from rag.utils.storage_factory import STORAGE_IMPL, STORAGE_IMPL_TYPE
from timeit import default_timer as timer

from rag.utils.redis_conn import REDIS_CONN


@manager.route('/version', methods=['GET'])
@login_required
def version():
    return get_json_result(data=get_rag_version())


@manager.route('/status', methods=['GET'])
@login_required
def status():
    res = {}
    st = timer()
    try:
        res["es"] = ELASTICSEARCH.health()
        res["es"]["elapsed"] = "{:.1f}".format((timer() - st)*1000.)
    except Exception as e:
        res["es"] = {"status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    st = timer()
    try:
        STORAGE_IMPL.health()
        res["storage"] = {"storage": STORAGE_IMPL_TYPE.lower(), "status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.)}
    except Exception as e:
        res["storage"] = {"storage": STORAGE_IMPL_TYPE.lower(), "status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    st = timer()
    try:
        KnowledgebaseService.get_by_id("x")
        res["database"] = {"database": DATABASE_TYPE.lower(), "status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.)}
    except Exception as e:
        res["database"] = {"database": DATABASE_TYPE.lower(), "status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    st = timer()
    try:
        if not REDIS_CONN.health():
            raise Exception("Lost connection!")
        res["redis"] = {"status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.)}
    except Exception as e:
        res["redis"] = {"status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    try:
        v = REDIS_CONN.get("TASKEXE")
        if not v:
            raise Exception("No task executor running!")
        obj = json.loads(v)
        color = "green"
        for id in obj.keys():
            arr = obj[id]
            if len(arr) == 1:
                obj[id] = [0]
            else:
                obj[id] = [arr[i+1]-arr[i] for i in range(len(arr)-1)]
            elapsed = max(obj[id])
            if elapsed > 50: color = "yellow"
            if elapsed > 120: color = "red"
        res["task_executor"] = {"status": color, "elapsed": obj}
    except Exception as e:
        res["task_executor"] = {"status": "red", "error": str(e)}

    return get_json_result(data=res)
