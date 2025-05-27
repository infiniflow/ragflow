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

import importlib
from .begin import Begin, BeginParam
from .llm import LLM, LLMParam
from .llm_with_tools import Agent, AgentParam
from .categorize import Categorize, CategorizeParam
from .switch import Switch, SwitchParam
from .message import Message, MessageParam
from .iteration import Iteration, IterationParam
from .iterationitem import IterationItem, IterationItemParam
from .code import Code, CodeParam
from .fillup import UserFillUp, UserFillUpParam


def component_class(class_name):
    m = importlib.import_module("agent.component")
    try:
        return getattr(m, class_name)
    except:
        return getattr(importlib.import_module("agent.tools"), class_name)


__all__ = [
    "Begin",
    "BeginParam",
    "UserFillUp",
    "UserFillUpParam",
    "LLMParam",
    "LLM",
    "Categorize",
    "CategorizeParam",
    "Switch",
    "SwitchParam",
    "Message",
    "MessageParam",
    "Agent",
    "AgentParam"
]
"""
    "RewriteQuestion",
    "RewriteQuestionParam",
    "KeywordExtract",
    "KeywordExtractParam",
    "Concentrator",
    "ConcentratorParam",
    "Baidu",
    "BaiduParam",
    "DuckDuckGo",
    "DuckDuckGoParam",
    "Wikipedia",
    "WikipediaParam",
    "PubMed",
    "PubMedParam",
    "ArXiv",
    "ArXivParam",
    "Google",
    "GoogleParam",
    "Bing",
    "BingParam",
    "GoogleScholar",
    "GoogleScholarParam",
    "DeepL",
    "DeepLParam",
    "GitHub",
    "GitHubParam",
    "BaiduFanyi",
    "BaiduFanyiParam",
    "QWeather",
    "QWeatherParam",
    "ExeSQL",
    "ExeSQLParam",
    "YahooFinance",
    "YahooFinanceParam",
    "WenCai",
    "WenCaiParam",
    "Jin10",
    "Jin10Param",
    "TuShare",
    "TuShareParam",
    "AkShare",
    "AkShareParam",
    "Crawler",
    "CrawlerParam",
    "Invoke",
    "InvokeParam",
    "Iteration",
    "IterationParam",
    "IterationItem",
    "IterationItemParam",
    "Template",
    "TemplateParam",
    "Email",
    "EmailParam",
    "Code",
    "CodeParam",
    "component_class"
"""
