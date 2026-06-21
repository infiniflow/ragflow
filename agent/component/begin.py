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

from agent.component.fillup import UserFillUpParam, UserFillUp
from api.db.services.file_service import FileService


class BeginParam(UserFillUpParam):

    """
    Define the Begin component parameters.
    """
    def __init__(self):
        super().__init__()
        self.mode = "conversational"
        self.prologue = "Hi! I'm your smart assistant. What can I do for you?"

    def check(self):
        self.check_valid_value(self.mode, "The 'mode' should be either `conversational` or `task`", ["conversational", "task","Webhook"])

    def get_input_form(self) -> dict[str, dict]:
        return getattr(self, "inputs")


class Begin(UserFillUp):
    """Entry-point component for agent workflows.

    Receives the user's query and any form inputs, then makes them available
    to downstream components.  ``sys.query`` is propagated to the ``query``
    output field so downstream nodes can reference it as ``Begin@query``.
    """

    component_name = "Begin"

    def _invoke(self, **kwargs):
        """Process Begin inputs and populate outputs.

        Reads ``sys.query`` from canvas globals and sets it as the ``query``
        output.  Then iterates over any form inputs defined in the component
        parameters, handling file uploads and setting input/output values.

        Args:
            **kwargs: Keyword arguments passed by the canvas executor.
                ``inputs`` (dict): Form field values keyed by field name.
        """
        if self.check_if_canceled("Begin processing"):
            return

        sys_query = self._canvas.globals.get("sys.query", "")
        logging.debug("[Begin] Propagating sys.query to output: %s", sys_query)
        self.set_output("query", sys_query)

        layout_recognize = self._param.layout_recognize or None
        for k, v in kwargs.get("inputs", {}).items():
            if self.check_if_canceled("Begin processing"):
                return

            if isinstance(v, dict) and v.get("type", "").lower().find("file") >= 0:
                if v.get("optional") and v.get("value", None) is None:
                    v = None
                else:
                    file_value = v["value"]
                    # Support both single file (backward compatibility) and multiple files
                    files = file_value if isinstance(file_value, list) else [file_value]
                    v = FileService.get_files(files, layout_recognize=layout_recognize)
            else:
                v = v.get("value")
            self.set_output(k, v)
            self.set_input_value(k, v)

    def thoughts(self) -> str:
        """Return the component's thought string (empty for Begin)."""
        return ""
