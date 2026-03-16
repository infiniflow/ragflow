#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

from enum import Enum, IntEnum
from strenum import StrEnum

SERVICE_CONF = "service_conf.yaml"
RAG_FLOW_SERVICE_NAME = "ragflow"


class CustomEnum(Enum):
    @classmethod
    def valid(cls, value):
        try:
            cls(value)
            return True
        except BaseException:
            return False

    @classmethod
    def values(cls):
        return [member.value for member in cls.__members__.values()]

    @classmethod
    def names(cls):
        return [member.name for member in cls.__members__.values()]


class RetCode(IntEnum, CustomEnum):
    SUCCESS = 0
    NOT_EFFECTIVE = 10
    EXCEPTION_ERROR = 100
    ARGUMENT_ERROR = 101
    DATA_ERROR = 102
    OPERATING_ERROR = 103
    CONNECTION_ERROR = 105
    RUNNING = 106
    PERMISSION_ERROR = 108
    AUTHENTICATION_ERROR = 109
    BAD_REQUEST = 400
    UNAUTHORIZED = 401
    SERVER_ERROR = 500
    FORBIDDEN = 403
    NOT_FOUND = 404
    CONFLICT = 409


class StatusEnum(Enum):
    VALID = "1"
    INVALID = "0"


class ActiveEnum(Enum):
    ACTIVE = "1"
    INACTIVE = "0"


class LLMType(StrEnum):
    CHAT = "chat"
    EMBEDDING = "embedding"
    SPEECH2TEXT = "speech2text"
    IMAGE2TEXT = "image2text"
    RERANK = "rerank"
    TTS = "tts"
    OCR = "ocr"


class TaskStatus(StrEnum):
    UNSTART = "0"
    RUNNING = "1"
    CANCEL = "2"
    DONE = "3"
    FAIL = "4"
    SCHEDULE = "5"


VALID_TASK_STATUS = {TaskStatus.UNSTART, TaskStatus.RUNNING, TaskStatus.CANCEL, TaskStatus.DONE, TaskStatus.FAIL, TaskStatus.SCHEDULE}


class ParserType(StrEnum):
    PRESENTATION = "presentation"
    LAWS = "laws"
    MANUAL = "manual"
    PAPER = "paper"
    RESUME = "resume"
    BOOK = "book"
    QA = "qa"
    TABLE = "table"
    NAIVE = "naive"
    PICTURE = "picture"
    ONE = "one"
    AUDIO = "audio"
    EMAIL = "email"
    KG = "knowledge_graph"
    TAG = "tag"


class FileSource(StrEnum):
    LOCAL = ""
    KNOWLEDGEBASE = "knowledgebase"
    S3 = "s3"
    NOTION = "notion"
    DISCORD = "discord"
    CONFLUENCE = "confluence"
    GMAIL = "gmail"
    GOOGLE_DRIVE = "google_drive"
    JIRA = "jira"
    SHAREPOINT = "sharepoint"
    SLACK = "slack"
    TEAMS = "teams"
    WEBDAV = "webdav"
    MOODLE = "moodle"
    DROPBOX = "dropbox"
    BOX = "box"
    R2 = "r2"
    OCI_STORAGE = "oci_storage"
    GOOGLE_CLOUD_STORAGE = "google_cloud_storage"
    AIRTABLE = "airtable"
    ASANA = "asana"
    GITHUB = "github"
    GITLAB = "gitlab"
    IMAP = "imap"
    BITBUCKET = "bitbucket"
    ZENDESK = "zendesk"
    SEAFILE = "seafile"
    MYSQL = "mysql"
    POSTGRESQL = "postgresql"


class PipelineTaskType(StrEnum):
    PARSE = "Parse"
    DOWNLOAD = "Download"
    RAPTOR = "RAPTOR"
    GRAPH_RAG = "GraphRAG"
    MINDMAP = "Mindmap"
    MEMORY = "Memory"


VALID_PIPELINE_TASK_TYPES = {PipelineTaskType.PARSE, PipelineTaskType.DOWNLOAD, PipelineTaskType.RAPTOR, PipelineTaskType.GRAPH_RAG, PipelineTaskType.MINDMAP}


class MCPServerType(StrEnum):
    SSE = "sse"
    STREAMABLE_HTTP = "streamable-http"


VALID_MCP_SERVER_TYPES = {MCPServerType.SSE, MCPServerType.STREAMABLE_HTTP}


class Storage(Enum):
    MINIO = 1
    AZURE_SPN = 2
    AZURE_SAS = 3
    AWS_S3 = 4
    OSS = 5
    OPENDAL = 6
    GCS = 7


class MemoryType(Enum):
    RAW = 0b0001  # 1 << 0 = 1 (0b00000001)
    SEMANTIC = 0b0010  # 1 << 1 = 2 (0b00000010)
    EPISODIC = 0b0100  # 1 << 2 = 4 (0b00000100)
    PROCEDURAL = 0b1000  # 1 << 3 = 8 (0b00001000)


class MemoryStorageType(StrEnum):
    TABLE = "table"
    GRAPH = "graph"


class ForgettingPolicy(StrEnum):
    FIFO = "FIFO"


# environment
# ENV_STRONG_TEST_COUNT = "STRONG_TEST_COUNT"
# ENV_RAGFLOW_SECRET_KEY = "RAGFLOW_SECRET_KEY"
# ENV_REGISTER_ENABLED = "REGISTER_ENABLED"
# ENV_DOC_ENGINE = "DOC_ENGINE"
# ENV_SANDBOX_ENABLED = "SANDBOX_ENABLED"
# ENV_SANDBOX_HOST = "SANDBOX_HOST"
# ENV_MAX_CONTENT_LENGTH = "MAX_CONTENT_LENGTH"
# ENV_COMPONENT_EXEC_TIMEOUT = "COMPONENT_EXEC_TIMEOUT"
# ENV_TRINO_USE_TLS = "TRINO_USE_TLS"
# ENV_MAX_FILE_NUM_PER_USER = "MAX_FILE_NUM_PER_USER"
# ENV_MACOS = "MACOS"
# ENV_RAGFLOW_DEBUGPY_LISTEN = "RAGFLOW_DEBUGPY_LISTEN"
# ENV_WERKZEUG_RUN_MAIN = "WERKZEUG_RUN_MAIN"
# ENV_DISABLE_SDK = "DISABLE_SDK"
# ENV_ENABLE_TIMEOUT_ASSERTION = "ENABLE_TIMEOUT_ASSERTION"
# ENV_LOG_LEVELS = "LOG_LEVELS"
# ENV_TENSORRT_DLA_SVR = "TENSORRT_DLA_SVR"
# ENV_OCR_GPU_MEM_LIMIT_MB = "OCR_GPU_MEM_LIMIT_MB"
# ENV_OCR_ARENA_EXTEND_STRATEGY = "OCR_ARENA_EXTEND_STRATEGY"
# ENV_MAX_CONCURRENT_PROCESS_AND_EXTRACT_CHUNK = "MAX_CONCURRENT_PROCESS_AND_EXTRACT_CHUNK"
# ENV_MAX_MAX_CONCURRENT_CHATS = "MAX_CONCURRENT_CHATS"
# ENV_RAGFLOW_MCP_BASE_URL = "RAGFLOW_MCP_BASE_URL"
# ENV_RAGFLOW_MCP_HOST = "RAGFLOW_MCP_HOST"
# ENV_RAGFLOW_MCP_PORT = "RAGFLOW_MCP_PORT"
# ENV_RAGFLOW_MCP_LAUNCH_MODE = "RAGFLOW_MCP_LAUNCH_MODE"
# ENV_RAGFLOW_MCP_HOST_API_KEY = "RAGFLOW_MCP_HOST_API_KEY"
# ENV_MINERU_EXECUTABLE = "MINERU_EXECUTABLE"
# ENV_MINERU_APISERVER = "MINERU_APISERVER"
# ENV_MINERU_OUTPUT_DIR = "MINERU_OUTPUT_DIR"
# ENV_MINERU_BACKEND = "MINERU_BACKEND"
# ENV_MINERU_DELETE_OUTPUT = "MINERU_DELETE_OUTPUT"
# ENV_TCADP_OUTPUT_DIR = "TCADP_OUTPUT_DIR"
# ENV_LM_TIMEOUT_SECONDS = "LM_TIMEOUT_SECONDS"
# ENV_LLM_MAX_RETRIES = "LLM_MAX_RETRIES"
# ENV_LLM_BASE_DELAY = "LLM_BASE_DELAY"
# ENV_OLLAMA_KEEP_ALIVE = "OLLAMA_KEEP_ALIVE"
# ENV_DOC_BULK_SIZE = "DOC_BULK_SIZE"
# ENV_EMBEDDING_BATCH_SIZE = "EMBEDDING_BATCH_SIZE"
# ENV_MAX_CONCURRENT_TASKS = "MAX_CONCURRENT_TASKS"
# ENV_MAX_CONCURRENT_CHUNK_BUILDERS = "MAX_CONCURRENT_CHUNK_BUILDERS"
# ENV_MAX_CONCURRENT_MINIO = "MAX_CONCURRENT_MINIO"
# ENV_WORKER_HEARTBEAT_TIMEOUT = "WORKER_HEARTBEAT_TIMEOUT"
# ENV_TRACE_MALLOC_ENABLED = "TRACE_MALLOC_ENABLED"

PAGERANK_FLD = "pagerank_fea"
SVR_QUEUE_NAME = "rag_flow_svr_queue"
SVR_CONSUMER_GROUP_NAME = "rag_flow_svr_task_broker"
TAG_FLD = "tag_feas"


MINERU_ENV_KEYS = ["MINERU_APISERVER", "MINERU_OUTPUT_DIR", "MINERU_BACKEND", "MINERU_SERVER_URL", "MINERU_DELETE_OUTPUT"]
MINERU_DEFAULT_CONFIG = {
    "MINERU_APISERVER": "",
    "MINERU_OUTPUT_DIR": "",
    "MINERU_BACKEND": "pipeline",
    "MINERU_SERVER_URL": "",
    "MINERU_DELETE_OUTPUT": 1,
}

PADDLEOCR_ENV_KEYS = ["PADDLEOCR_API_URL", "PADDLEOCR_ACCESS_TOKEN", "PADDLEOCR_ALGORITHM"]
PADDLEOCR_DEFAULT_CONFIG = {
    "PADDLEOCR_API_URL": "",
    "PADDLEOCR_ACCESS_TOKEN": None,
    "PADDLEOCR_ALGORITHM": "PaddleOCR-VL",
}
