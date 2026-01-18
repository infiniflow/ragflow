from enum import Enum
from dataclasses import dataclass


class IndexType(Enum):
    FullText = 1
    Dense = 2
    Other = 99


@dataclass
class IndexInfo:
    field_name: str
    index_type: IndexType
    options: dict | None = None
