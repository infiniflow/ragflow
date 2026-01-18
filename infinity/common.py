from enum import Enum


class SortType(Enum):
    Asc = 0
    Desc = 1


class ConflictType(Enum):
    # Minimal subset used by tests
    Abort = 0
    Ignore = 1


class InfinityException(Exception):
    def __init__(self, error_code: int | None = None, *args, **kwargs):
        super().__init__(*args)
        self.error_code = error_code or 0


class NetworkAddress:
    def __init__(self, host: str, port: int):
        self.host = host
        self.port = port

    def __str__(self):
        return f"{self.host}:{self.port}"
