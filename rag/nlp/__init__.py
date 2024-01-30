from . import search
from rag.utils import ELASTICSEARCH

retrievaler = search.Dealer(ELASTICSEARCH)

from nltk.stem import PorterStemmer
stemmer = PorterStemmer()
