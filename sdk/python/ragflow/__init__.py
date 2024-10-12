import importlib.metadata

__version__ = importlib.metadata.version("ragflow")

from .ragflow import RAGFlow
from .modules.dataset import DataSet
from .modules.chat import Chat
from .modules.session import Session
from .modules.document import Document
from .modules.chunk import Chunk