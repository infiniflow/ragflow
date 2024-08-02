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
import random
from abc import ABC
from functools import partial
from agent.component.base import ComponentBase, ComponentParamBase


class MessageParam(ComponentParamBase):

    """
    Define the Message component parameters.
    """
    def __init__(self):
        super().__init__()
        self.messages = []

    def check(self):
        self.check_empty(self.messages, "[Message]")
        return True


class Message(ComponentBase, ABC):
    component_name = "Message"

    def _run(self, history, **kwargs):
        if kwargs.get("stream"):
            return partial(self.stream_output)

        return Message.be_output(random.choice(self._param.messages))

    def stream_output(self):
        res = None
        if self._param.messages:
            res = {"content": random.choice(self._param.messages)}
            yield res

        self.set_output(res)


