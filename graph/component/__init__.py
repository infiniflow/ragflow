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
from .baidu import Baidu, BaiduParam
from .duckduckgosearch import DuckDuckGoSearch, DuckDuckGoSearchParam


def component_class(class_name):
    m = importlib.import_module("graph.component")
    c = getattr(m, class_name)
    return c
