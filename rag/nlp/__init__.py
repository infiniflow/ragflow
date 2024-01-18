from . import search
from rag.utils import ELASTICSEARCH

retrievaler = search.Dealer(ELASTICSEARCH)