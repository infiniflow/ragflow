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
from abc import ABC
from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from common.connection_utils import timeout
import deepl


class DeepLParam(ToolParamBase):
    """
    Define the DeepL component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "deepl_translate",
            "description": "DeepL translates text into a target language using the DeepL translation service.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The text to translate.",
                    "default": "{sys.query}",
                    "required": True,
                }
            },
        }
        super().__init__()
        self.auth_key = "xxx"
        self.source_lang = "ZH"
        self.target_lang = "EN-GB"

    def check(self):
        self.check_valid_value(
            self.source_lang,
            "Source language",
            ["AR", "BG", "CS", "DA", "DE", "EL", "EN", "ES", "ET", "FI", "FR", "HU", "ID", "IT", "JA", "KO", "LT", "LV", "NB", "NL", "PL", "PT", "RO", "RU", "SK", "SL", "SV", "TR", "UK", "ZH"],
        )
        self.check_valid_value(
            self.target_lang,
            "Target language",
            [
                "AR",
                "BG",
                "CS",
                "DA",
                "DE",
                "EL",
                "EN-GB",
                "EN-US",
                "ES",
                "ET",
                "FI",
                "FR",
                "HU",
                "ID",
                "IT",
                "JA",
                "KO",
                "LT",
                "LV",
                "NB",
                "NL",
                "PL",
                "PT-BR",
                "PT-PT",
                "RO",
                "RU",
                "SK",
                "SL",
                "SV",
                "TR",
                "UK",
                "ZH",
            ],
        )

    def get_input_form(self) -> dict[str, dict]:
        return {"query": {"name": "Text", "type": "line"}}


class DeepL(ToolBase, ABC):
    component_name = "DeepL"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("DeepL processing"):
            return

        text = kwargs.get("query")
        if not text:
            self.set_output("formalized_content", "")
            return ""

        try:
            translator = deepl.Translator(self._param.auth_key)
            result = translator.translate_text(text, source_lang=self._param.source_lang, target_lang=self._param.target_lang)

            if self.check_if_canceled("DeepL processing"):
                return

            res = result.text
            self.set_output("formalized_content", res)
            return res
        except Exception as e:
            if self.check_if_canceled("DeepL processing"):
                return

            logging.exception(f"DeepL error: {e}")
            msg = f"DeepL error: {e}"
            self.set_output("_ERROR", msg)
            return msg

    def thoughts(self) -> str:
        return "Translating into {}: {}".format(self._param.target_lang, self.get_input().get("query", "-_-!"))
