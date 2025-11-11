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
import time
from abc import ABC
import wikipedia
from agent.tools.base import ToolMeta, ToolParamBase, ToolBase
from common.connection_utils import timeout


class WikipediaParam(ToolParamBase):
    """
    Define the Wikipedia component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "wikipedia_search",
            "description": """A wide range of how-to and information pages are made available in wikipedia. Since 2001, it has grown rapidly to become the world's largest reference website. From Wikipedia, the free encyclopedia.""",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The search keyword to execute with wikipedia. The keyword MUST be a specific subject that can match the title.",
                    "default": "{sys.query}",
                    "required": True
                }
            }
        }
        super().__init__()
        self.top_n = 10
        self.language = "en"

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")
        self.check_valid_value(self.language, "Wikipedia languages",
                               ['af', 'pl', 'ar', 'ast', 'az', 'bg', 'nan', 'bn', 'be', 'ca', 'cs', 'cy', 'da', 'de',
                                'et', 'el', 'en', 'es', 'eo', 'eu', 'fa', 'fr', 'gl', 'ko', 'hy', 'hi', 'hr', 'id',
                                'it', 'he', 'ka', 'lld', 'la', 'lv', 'lt', 'hu', 'mk', 'arz', 'ms', 'min', 'my', 'nl',
                                'ja', 'nb', 'nn', 'ce', 'uz', 'pt', 'kk', 'ro', 'ru', 'ceb', 'sk', 'sl', 'sr', 'sh',
                                'fi', 'sv', 'ta', 'tt', 'th', 'tg', 'azb', 'tr', 'uk', 'ur', 'vi', 'war', 'zh', 'yue'])

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line"
            }
        }

class Wikipedia(ToolBase, ABC):
    component_name = "Wikipedia"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 60)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("Wikipedia processing"):
            return

        if not kwargs.get("query"):
            self.set_output("formalized_content", "")
            return ""

        last_e = ""
        for _ in range(self._param.max_retries+1):
            if self.check_if_canceled("Wikipedia processing"):
                return

            try:
                wikipedia.set_lang(self._param.language)
                wiki_engine = wikipedia
                pages = []
                for p in wiki_engine.search(kwargs["query"], results=self._param.top_n):
                    if self.check_if_canceled("Wikipedia processing"):
                        return

                    try:
                        pages.append(wikipedia.page(p))
                    except Exception:
                        pass
                self._retrieve_chunks(pages,
                                      get_title=lambda r: r.title,
                                      get_url=lambda r: r.url,
                                      get_content=lambda r: r.summary)
                return self.output("formalized_content")
            except Exception as e:
                if self.check_if_canceled("Wikipedia processing"):
                    return

                last_e = e
                logging.exception(f"Wikipedia error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"Wikipedia error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return """
Keywords: {}
Looking for the most relevant articles.
        """.format(self.get_input().get("query", "-_-!"))
