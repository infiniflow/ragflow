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


def component_class(class_name):
    m = importlib.import_module("agent.component")
    c = getattr(m, class_name)
    return c
