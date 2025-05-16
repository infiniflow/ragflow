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
from .baidu import Baidu, BaiduParam
from .duckduckgo import DuckDuckGo, DuckDuckGoParam
from .wikipedia import Wikipedia, WikipediaParam
from .pubmed import PubMed, PubMedParam
from .arxiv import ArXiv, ArXivParam
from .google import Google, GoogleParam
from .bing import Bing, BingParam
from .googlescholar import GoogleScholar, GoogleScholarParam
from .deepl import DeepL, DeepLParam
from .github import GitHub, GitHubParam
from .baidufanyi import BaiduFanyi, BaiduFanyiParam
from .qweather import QWeather, QWeatherParam
from .exesql import ExeSQL, ExeSQLParam
from .yahoofinance import YahooFinance, YahooFinanceParam
from .wencai import WenCai, WenCaiParam
from .jin10 import Jin10, Jin10Param
from .tushare import TuShare, TuShareParam
from .akshare import AkShare, AkShareParam
from .crawler import Crawler, CrawlerParam
from .invoke import Invoke, InvokeParam
from .template import Template, TemplateParam
from .email import Email, EmailParam
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
