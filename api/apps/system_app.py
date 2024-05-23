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
from flask_login import login_required

from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import get_json_result
from api.versions import get_rag_version
from rag.settings import SVR_QUEUE_NAME
from rag.utils.es_conn import ELASTICSEARCH
from rag.utils.minio_conn import MINIO
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
        MINIO.health()
        res["minio"] = {"status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.)}
    except Exception as e:
        res["minio"] = {"status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    st = timer()
    try:
        KnowledgebaseService.get_by_id("x")
        res["mysql"] = {"status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.)}
    except Exception as e:
        res["mysql"] = {"status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    st = timer()
    try:
        qinfo = REDIS_CONN.health(SVR_QUEUE_NAME)
        res["redis"] = {"status": "green", "elapsed": "{:.1f}".format((timer() - st)*1000.),
                        "pending": qinfo.get("pending", 0)}
    except Exception as e:
        res["redis"] = {"status": "red", "elapsed": "{:.1f}".format((timer() - st)*1000.), "error": str(e)}

    return get_json_result(data=res)
