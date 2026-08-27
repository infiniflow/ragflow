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
from fastapi import APIRouter, Depends
from services.auth import require_api_token
from services.limiter import RUN_RATE_LIMIT, limiter
from services.preauth import preauth_rate_limit

from api.handlers import healthz_handler, run_code_handler

router = APIRouter()

router.get("/")(healthz_handler)
router.get("/healthz")(healthz_handler)
# Execution endpoints throttle ALL traffic per client address before
# authentication (so invalid-token floods eventually receive 429), then
# require the shared-secret token, then apply the larger authenticated
# execution quota. Health probes stay unauthenticated.
router.post("/run", dependencies=[Depends(preauth_rate_limit), Depends(require_api_token)])(limiter.limit(RUN_RATE_LIMIT)(run_code_handler))
