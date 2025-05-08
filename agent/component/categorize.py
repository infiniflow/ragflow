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
from agent.component import GenerateParam, Generate


class CategorizeParam(GenerateParam):

    """
    Define the Categorize component parameters.
    """
    def __init__(self):
        super().__init__()
        self.category_description = {}
        self.prompt = ""

    def check(self):
        super().check()
        self.check_empty(self.category_description, "[Categorize] Category examples")
        for k, v in self.category_description.items():
            if not k:
                raise ValueError("[Categorize] Category name can not be empty!")
            if not v.get("to"):
                raise ValueError(f"[Categorize] 'To' of category {k} can not be empty!")

    def get_prompt(self, chat_hist):
        cate_lines = []
        for c, desc in self.category_description.items():
            for line in desc.get("examples", "").split("\n"):
                if not line:
                    continue
                cate_lines.append("USER: {}\nCategory: {}".format(line, c))
        descriptions = []
        for c, desc in self.category_description.items():
            if desc.get("description"):
                descriptions.append(
                    "\nCategory: {}\nDescription: {}".format(c, desc["description"]))

        self.prompt = """
Role: You're a text classifier. 
Task: You need to categorize the user’s questions into {} categories, namely: {}

Here's description of each category:
{}

You could learn from the following examples:
{}
You could learn from the above examples.

Requirements:
- Just mention the category names, no need for any additional words.

---- Real Data ----
USER: {}\n
        """.format(
            len(self.category_description.keys()),
            "/".join(list(self.category_description.keys())),
            "\n".join(descriptions),
            "\n\n- ".join(cate_lines),
            chat_hist
        )
        return self.prompt


class Categorize(Generate, ABC):
    component_name = "Categorize"

    def _run(self, history, **kwargs):
        input = self.get_input()
        input = " - ".join(input["content"]) if "content" in input else ""
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        ans = chat_mdl.chat(self._param.get_prompt(input), [{"role": "user", "content": "\nCategory: "}],
                            self._param.gen_conf())
        logging.debug(f"input: {input}, answer: {str(ans)}")    
        # Count the number of times each category appears in the answer.
        category_counts = {}
        for c in self._param.category_description.keys():
            count = ans.lower().count(c.lower())
            category_counts[c] = count
            
        # If a category is found, return the category with the highest count.
        if any(category_counts.values()):
            max_category = max(category_counts.items(), key=lambda x: x[1])
            return Categorize.be_output(self._param.category_description[max_category[0]]["to"])

        return Categorize.be_output(list(self._param.category_description.items())[-1][1]["to"])

    def debug(self, **kwargs):
        df = self._run([], **kwargs)
        cpn_id = df.iloc[0, 0]
        return Categorize.be_output(self._canvas.get_component_name(cpn_id))

