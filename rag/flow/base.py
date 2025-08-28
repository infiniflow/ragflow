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
import time
import os
import logging
from functools import partial
from typing import Any
import trio
from agent.component.base import ComponentParamBase, ComponentBase
from api.utils.api_utils import timeout


class ProcessParamBase(ComponentParamBase):
    def __init__(self):
        super().__init__()
        self.timeout = 100000000
        self.persist_logs = True


class ProcessBase(ComponentBase):

    def __init__(self, pipeline, id, param: ProcessParamBase):
        super().__init__(pipeline, id, param)
        self.callback = partial(self._canvas.callback, self.component_name)

    async def invoke(self, **kwargs) -> dict[str, Any]:
        self.set_output("_created_time", time.perf_counter())
        for k,v in kwargs.items():
            self.set_output(k, v)
        try:
            with trio.fail_after(self._param.timeout):
                await self._invoke(**kwargs)
                self.callback(1, "Done")
        except Exception as e:
            if self.get_exception_default_value():
                self.set_exception_default_value()
            else:
                self.set_output("_ERROR", str(e))
            logging.exception(e)
            self.callback(-1, str(e))
        self.set_output("_elapsed_time", time.perf_counter() - self.output("_created_time"))
        return self.output()

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    async def _invoke(self, **kwargs):
        raise NotImplementedError()
