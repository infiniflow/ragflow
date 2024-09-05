import importlib.metadata

__version__ = importlib.metadata.version("ragflow")

from .ragflow import RAGFlow
from .modules.dataset import DataSet
from .modules.chat_assistant import Assistant