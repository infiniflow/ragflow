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
import os
import re
from abc import ABC

from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from agent.component.llm import LLMParam, LLM
from api.utils.api_utils import timeout
from rag.llm.chat_model import ERROR_PREFIX


class CategorizeParam(LLMParam):

    """
    Define the Categorize component parameters.
    """
    def __init__(self):
        super().__init__()
        self.category_description = {}
        self.query = "sys.query"
        self.message_history_window_size = 1
        self.update_prompt()

    def check(self):
        self.check_positive_integer(self.message_history_window_size, "[Categorize] Message window size > 0")
        self.check_empty(self.category_description, "[Categorize] Category examples")
        for k, v in self.category_description.items():
            if not k:
                raise ValueError("[Categorize] Category name can not be empty!")
            if not v.get("to"):
                raise ValueError(f"[Categorize] 'To' of category {k} can not be empty!")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "type": "line",
                "name": "Query"
            }
        }

    def update_prompt(self):
        cate_lines = []
        for c, desc in self.category_description.items():
            for line in desc.get("examples", []):
                if not line:
                    continue
                cate_lines.append("USER: \"" + re.sub(r"\n", "    ", line, flags=re.DOTALL) + "\" → "+c)

        descriptions = []
        for c, desc in self.category_description.items():
            if desc.get("description"):
                descriptions.append(
                    "\n------\nCategory: {}\nDescription: {}".format(c, desc["description"]))

        self.sys_prompt = """
You are an advanced classification system that categorizes user questions into specific types. Analyze the input question and classify it into ONE of the following categories:
{}

Here's description of each category:
 - {}

---- Instructions ----
 - Consider both explicit mentions and implied context
 - Prioritize the most specific applicable category
 - Return only the category name without explanations
 - Use "Other" only when no other category fits
 
 """.format(
            "\n - ".join(list(self.category_description.keys())),
            "\n".join(descriptions)
        )

        if cate_lines:
            self.sys_prompt += """
---- Examples ----
{}
""".format("\n".join(cate_lines))


class Categorize(LLM, ABC):
    component_name = "Categorize"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        msg = self._canvas.get_history(self._param.message_history_window_size)
        if not msg:
            msg = [{"role": "user", "content": ""}]
        if kwargs.get("sys.query"):
            msg[-1]["content"] = kwargs["sys.query"]
            self.set_input_value("sys.query", kwargs["sys.query"])
        else:
            msg[-1]["content"] = self._canvas.get_variable_value(self._param.query)
            self.set_input_value(self._param.query, msg[-1]["content"])
        self._param.update_prompt()
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)

        user_prompt = """
---- Real Data ----
{} → 
""".format(" | ".join(["{}: \"{}\"".format(c["role"].upper(), re.sub(r"\n", "", c["content"], flags=re.DOTALL)) for c in msg]))
        ans = chat_mdl.chat(self._param.sys_prompt, [{"role": "user", "content": user_prompt}], self._param.gen_conf())
        logging.info(f"input: {user_prompt}, answer: {str(ans)}")
        if ERROR_PREFIX in ans:
            raise Exception(ans)
        # Count the number of times each category appears in the answer.
        category_counts = {}
        for c in self._param.category_description.keys():
            count = ans.lower().count(c.lower())
            category_counts[c] = count

        cpn_ids = list(self._param.category_description.items())[-1][1]["to"]
        max_category = list(self._param.category_description.keys())[0]
        if any(category_counts.values()):
            max_category = max(category_counts.items(), key=lambda x: x[1])[0]
            cpn_ids = self._param.category_description[max_category]["to"]

        self.set_output("category_name", max_category)
        self.set_output("_next", cpn_ids)

    def thoughts(self) -> str:
        return "Which should it falls into {}? ...".format(",".join([f"`{c}`" for c, _ in self._param.category_description.items()]))