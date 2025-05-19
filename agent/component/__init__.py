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
from .generate import Generate, GenerateParam
from .retrieval import Retrieval, RetrievalParam
from .answer import Answer, AnswerParam
from .categorize import Categorize, CategorizeParam
from .switch import Switch, SwitchParam
from .relevant import Relevant, RelevantParam
from .message import Message, MessageParam
from .rewrite import RewriteQuestion, RewriteQuestionParam
from .keyword import KeywordExtract, KeywordExtractParam
from .concentrator import Concentrator, ConcentratorParam
from agent.tools.baidu import Baidu, BaiduParam
from agent.tools.duckduckgo import DuckDuckGo, DuckDuckGoParam
from agent.tools.wikipedia import Wikipedia, WikipediaParam
from agent.tools.pubmed import PubMed, PubMedParam
from agent.tools.arxiv import ArXiv, ArXivParam
from agent.tools.google import Google, GoogleParam
from agent.tools.bing import Bing, BingParam
from agent.tools.googlescholar import GoogleScholar, GoogleScholarParam
from agent.tools.deepl import DeepL, DeepLParam
from agent.tools.github import GitHub, GitHubParam
from agent.tools.baidufanyi import BaiduFanyi, BaiduFanyiParam
from agent.tools.qweather import QWeather, QWeatherParam
from agent.tools.exesql import ExeSQL, ExeSQLParam
from agent.tools.yahoofinance import YahooFinance, YahooFinanceParam
from agent.tools.wencai import WenCai, WenCaiParam
from agent.tools.jin10 import Jin10, Jin10Param
from agent.tools.tushare import TuShare, TuShareParam
from agent.tools.akshare import AkShare, AkShareParam
from agent.tools.crawler import Crawler, CrawlerParam
from agent.tools.invoke import Invoke, InvokeParam
from .template import Template, TemplateParam
from agent.tools.email import Email, EmailParam
from .iteration import Iteration, IterationParam
from .iterationitem import IterationItem, IterationItemParam
from .code import Code, CodeParam


def component_class(class_name):
    m = importlib.import_module("agent.component")
    c = getattr(m, class_name)
    return c


__all__ = [
    "Begin",
    "BeginParam",
    "Generate",
    "GenerateParam",
    "Retrieval",
    "RetrievalParam",
    "Answer",
    "AnswerParam",
    "Categorize",
    "CategorizeParam",
    "Switch",
    "SwitchParam",
    "Relevant",
    "RelevantParam",
    "Message",
    "MessageParam",
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
]
