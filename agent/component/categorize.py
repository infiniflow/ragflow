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
from abc import ABC
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from agent.component import LLMParam, LLM


class CategorizeParam(LLMParam):

    """
    Define the Categorize component parameters.
    """
    def __init__(self):
        super().__init__()
        self.category_description = {}
        self.prompts = []

    def check(self):
        super().check()
        self.check_empty(self.category_description, "[Categorize] Category examples")
        for k, v in self.category_description.items():
            if not k:
                raise ValueError("[Categorize] Category name can not be empty!")
            if not v.get("to"):
                raise ValueError(f"[Categorize] 'To' of category {k} can not be empty!")

    def update_prompt(self, chat_hist):
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

Instructions:
 - Consider both explicit mentions and implied context
 - Prioritize the most specific applicable category
 - Return only the category name without explanations
 - Use "Other" only when no other category fits

---- Real Data ----
{}\n
        """.format(
            "\n - ".join(list(self.category_description.keys())),
            "\n".join(descriptions),
            "\n".join(["{}: {}".format(c["role"].upper(), c["content"]) for c in chat_hist])
        )


class Categorize(LLM, ABC):
    component_name = "Categorize"

    async def _invoke(self, **kwargs):
        msg = self._canvas.get_history(self._param.message_history_window_size)
        self._param.update_prompt(msg)
        args = self.get_input_elements()
        prompt = self.string_format(self._param.sys_prompt, args)
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)

        ans = chat_mdl.chat(prompt, [{"role": "user", "content": "\nCategory: "}],
                            self._param.gen_conf())
        logging.debug(f"input: {input}, answer: {str(ans)}")    
        # Count the number of times each category appears in the answer.
        category_counts = {}
        for c in self._param.category_description.keys():
            count = ans.lower().count(c.lower())
            category_counts[c] = count

        cpn_id = list(self._param.category_description.items())[-1][1]["to"]
        if any(category_counts.values()):
            max_category = max(category_counts.items(), key=lambda x: x[1])
            cpn_id = self._param.category_description[max_category[0]]["to"]

        self.set_output("next", self._canvas.get_component_name(cpn_id))
        self.set_output("_next", cpn_id)

    def debug(self, **kwargs):
        df = self._run([], **kwargs)
        cpn_id = df.iloc[0, 0]
        return Categorize.be_output(self._canvas.get_component_name(cpn_id))

