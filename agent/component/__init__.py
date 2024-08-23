import importlib

from .answer import Answer, AnswerParam
from .arxiv import ArXiv, ArXivParam
from .baidu import Baidu, BaiduParam
from .baidufanyi import BaiduFanyi, BaiduFanyiParam
from .begin import Begin, BeginParam
from .bing import Bing, BingParam
from .categorize import Categorize, CategorizeParam
from .deepl import DeepL, DeepLParam
from .duckduckgo import DuckDuckGo, DuckDuckGoParam
from .exesql import ExeSQL, ExeSQLParam
from .generate import Generate, GenerateParam
from .github import GitHub, GitHubParam
from .google import Google, GoogleParam
from .googlescholar import GoogleScholar, GoogleScholarParam
from .keyword import KeywordExtract, KeywordExtractParam
from .message import Message, MessageParam
from .pubmed import PubMed, PubMedParam
from .qweather import QWeather, QWeatherParam
from .relevant import Relevant, RelevantParam
from .retrieval import Retrieval, RetrievalParam
from .rewrite import RewriteQuestion, RewriteQuestionParam
from .switch import Switch, SwitchParam
from .wikipedia import Wikipedia, WikipediaParam


def component_class(class_name):
    m = importlib.import_module("agent.component")
    c = getattr(m, class_name)
    return c
