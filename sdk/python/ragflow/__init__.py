import importlib.metadata

__version__ = importlib.metadata.version("ragflow")

from .ragflow import RAGFlow
from .modules.dataset import DataSet