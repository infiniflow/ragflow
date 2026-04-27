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
from api.utils.api_utils import get_json_result
from rag.utils.redis_conn import REDIS_CONN
from api.apps import login_required


@manager.route('/cancel/<task_id>', methods=['PUT'])  # noqa: F821
@login_required
def cancel(task_id):
    try:
        REDIS_CONN.set(f"{task_id}-cancel", "x")
    except Exception as e:
        logging.exception(e)
    return get_json_result(data=True)
