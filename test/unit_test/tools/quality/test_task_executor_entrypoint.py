#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from __future__ import annotations

import asyncio
import builtins
import faulthandler
import runpy
import sys
import types
from enum import StrEnum
from pathlib import Path

import pytest


_ROOT = Path(__file__).resolve().parents[4]
_EXPECTED_PARSER_FACTORY = {
    "general": "rag.app.naive",
    "naive": "rag.app.naive",
    "paper": "rag.app.paper",
    "book": "rag.app.book",
    "presentation": "rag.app.presentation",
    "manual": "rag.app.manual",
    "laws": "rag.app.laws",
    "qa": "rag.app.qa",
    "table": "rag.app.table",
    "resume": "rag.app.resume",
    "picture": "rag.app.picture",
    "one": "rag.app.one",
    "audio": "rag.app.audio",
    "email": "rag.app.email",
    "knowledge_graph": "rag.app.naive",
    "tag": "rag.app.tag",
}
_EXPECTED_TASK_TYPE_MAP = {
    "dataflow": "Parse",
    "raptor": "RAPTOR",
    "graphrag": "GraphRAG",
    "mindmap": "Mindmap",
    "memory": "Memory",
    "artifact": "Artifact",
    "skill": "Skill",
}


def _install_package(monkeypatch, name: str):
    package = types.ModuleType(name)
    package.__path__ = []
    monkeypatch.setitem(sys.modules, name, package)
    parent_name, _, child_name = name.rpartition(".")
    if parent_name:
        setattr(sys.modules[parent_name], child_name, package)
    return package


def _install_module(monkeypatch, name: str, **attributes):
    module = types.ModuleType(name)
    module.__dict__.update(attributes)
    monkeypatch.setitem(sys.modules, name, module)
    parent_name, _, child_name = name.rpartition(".")
    if parent_name:
        setattr(sys.modules[parent_name], child_name, module)
    return module


def _identity_decorator(function):
    return function


def _timeout_decorator(*_args, **_kwargs):
    return _identity_decorator


def _unused(*_args, **_kwargs):
    return None


def _install_task_executor_boundaries(monkeypatch, events):
    for package_name in (
        "api",
        "api.db",
        "api.db.services",
        "api.db.joint_services",
        "common",
        "rag",
        "rag.svr",
        "rag.svr.task_executor_refactor",
        "rag.utils",
        "rag.graphrag",
        "rag.prompts",
        "rag.app",
        "rag.nlp",
        "rag.advanced_rag",
        "rag.advanced_rag.knowlege_compile",
    ):
        _install_package(monkeypatch, package_name)

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

    class PipelineTaskType(StrEnum):
        PARSE = "Parse"
        DOWNLOAD = "Download"
        RAPTOR = "RAPTOR"
        GRAPH_RAG = "GraphRAG"
        MINDMAP = "Mindmap"
        MEMORY = "Memory"
        ARTIFACT = "Artifact"
        SKILL = "Skill"

    class LLMType:
        CHAT = "chat"
        EMBEDDING = "embedding"

    class TaskCanceledException(Exception):
        pass

    class DoesNotExist(Exception):
        pass

    class TaskManager:
        pass

    class RecordingContext:
        pass

    class NullRecordingContext:
        pass

    parser_modules = {}
    for parser_name in (
        "laws",
        "paper",
        "presentation",
        "manual",
        "qa",
        "table",
        "book",
        "resume",
        "picture",
        "naive",
        "one",
        "audio",
        "email",
        "tag",
    ):
        parser_module = types.ModuleType(f"rag.app.{parser_name}")
        parser_modules[parser_name] = parser_module
        setattr(sys.modules["rag.app"], parser_name, parser_module)

    _install_module(monkeypatch, "rag.svr.task_executor_refactor.task_manager", TaskManager=TaskManager)
    _install_module(
        monkeypatch,
        "rag.svr.task_executor_refactor.recording_context",
        timed_with_recording=_identity_decorator,
        get_recording_context=_unused,
        RecordingContext=RecordingContext,
        set_recording_context=_unused,
        NullRecordingContext=NullRecordingContext,
    )
    _install_module(monkeypatch, "common.misc_utils", thread_pool_exec=_unused)
    sys.modules["api.db"].PIPELINE_SPECIAL_PROGRESS_FREEZE_TASK_TYPES = set()
    _install_module(monkeypatch, "api.db.services.knowledgebase_service", KnowledgebaseService=type("KnowledgebaseService", (), {}))
    _install_module(
        monkeypatch,
        "api.db.services.pipeline_operation_log_service",
        PipelineOperationLogService=type("PipelineOperationLogService", (), {}),
    )
    _install_module(monkeypatch, "api.db.joint_services.memory_message_service", handle_save_to_memory_task=_unused)
    _install_module(monkeypatch, "common.connection_utils", timeout=_timeout_decorator)
    _install_module(monkeypatch, "common.metadata_utils", turn2jsonschema=_unused, update_metadata_to=_unused)
    _install_module(monkeypatch, "rag.utils.base64_image", image2id=_unused)
    _install_module(
        monkeypatch,
        "rag.utils.raptor_utils",
        collect_raptor_chunk_ids=_unused,
        collect_raptor_methods=_unused,
        get_raptor_clustering_method=_unused,
        get_raptor_tree_builder=_unused,
        get_skip_reason=_unused,
        make_raptor_summary_chunk_id=_unused,
        should_skip_raptor=_unused,
    )
    _install_module(monkeypatch, "common.log_utils", init_root_logger=lambda name: events.append(("root_logger", name)))
    _install_module(
        monkeypatch,
        "common.observability",
        configure_otel=lambda service, version: events.append(("otel", service, version)),
        consume_queue_context=_unused,
    )
    _install_module(monkeypatch, "common.config_utils", show_configs=_unused)
    _install_module(
        monkeypatch,
        "rag.graphrag.utils",
        get_llm_cache=_unused,
        set_llm_cache=_unused,
        get_tags_from_cache=_unused,
        set_tags_to_cache=_unused,
        chat_limiter=object(),
    )
    _install_module(
        monkeypatch,
        "rag.prompts.generator",
        keyword_extraction=_unused,
        question_proposal=_unused,
        content_tagging=_unused,
        run_toc_from_text=_unused,
        gen_metadata=_unused,
    )
    _install_module(monkeypatch, "xxhash")
    _install_module(monkeypatch, "exceptiongroup", ExceptionGroup=builtins.ExceptionGroup)
    _install_module(monkeypatch, "numpy")
    _install_module(monkeypatch, "peewee", DoesNotExist=DoesNotExist)
    _install_module(
        monkeypatch,
        "common.constants",
        LLMType=LLMType,
        ParserType=ParserType,
        PipelineTaskType=PipelineTaskType,
        PAGERANK_FLD="pagerank_fea",
        TAG_FLD="tag_fea",
        SVR_CONSUMER_GROUP_NAME="rag_flow_svr_task_broker",
    )
    for module_name, symbol_name in (
        ("api.db.services.document_service", "DocumentService"),
        ("api.db.services.doc_metadata_service", "DocMetadataService"),
        ("api.db.services.llm_service", "LLMBundle"),
        ("api.db.services.file2document_service", "File2DocumentService"),
    ):
        _install_module(monkeypatch, module_name, **{symbol_name: type(symbol_name, (), {})})
    _install_module(
        monkeypatch,
        "api.db.services.task_service",
        TaskService=type("TaskService", (), {}),
        has_canceled=_unused,
        CANVAS_DEBUG_DOC_ID="canvas-debug",
        GRAPH_RAPTOR_FAKE_DOC_ID="graph-raptor",
    )
    _install_module(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=_unused,
        get_model_config_from_provider_instance=_unused,
    )
    _install_module(monkeypatch, "common.versions", get_ragflow_version=lambda: "test-version")
    _install_module(monkeypatch, "api.db.db_models", close_connection=_unused)
    sys.modules["rag.nlp"].search = types.ModuleType("rag.nlp.search")
    sys.modules["rag.nlp"].rag_tokenizer = types.ModuleType("rag.nlp.rag_tokenizer")
    sys.modules["rag.nlp"].add_positions = _unused
    _install_module(
        monkeypatch,
        "rag.advanced_rag.knowlege_compile.raptor",
        RAPTOR_TREE_BUILDER="balanced",
    )
    _install_module(monkeypatch, "common.token_utils", num_tokens_from_string=_unused, truncate=_unused)
    _install_module(monkeypatch, "rag.utils.redis_conn", REDIS_CONN=object(), RedisDistributedLock=type("RedisDistributedLock", (), {}))
    _install_module(monkeypatch, "common.signal_utils", start_tracemalloc_and_snapshot=_unused, stop_tracemalloc=_unused)
    _install_module(monkeypatch, "common.exceptions", TaskCanceledException=TaskCanceledException)
    _install_module(
        monkeypatch,
        "rag.utils.table_es_metadata",
        aggregate_table_doc_metadata=_unused,
        merge_table_parser_config_from_kb=_unused,
        table_parser_strip_doc_metadata_keys=_unused,
    )
    _install_module(
        monkeypatch,
        "rag.svr.task_executor_limiter",
        task_limiter=object(),
        chunk_limiter=object(),
        embed_limiter=object(),
        minio_limiter=object(),
        kg_limiter=object(),
    )
    _install_module(
        monkeypatch,
        "common.settings",
        init_settings=_unused,
        check_and_install_torch=_unused,
        print_rag_settings=_unused,
        EMBEDDING_CFG={},
    )


def _load_task_executor(monkeypatch, *, run_as_main=False):
    events = []
    _install_task_executor_boundaries(monkeypatch, events)
    monkeypatch.setenv("LITELLM_LOCAL_MODEL_COST_MAP", "True")
    monkeypatch.setenv("WORKER_HEARTBEAT_TIMEOUT", "120")
    monkeypatch.setattr(faulthandler, "enable", lambda: events.append(("faulthandler",)))

    def fake_asyncio_run(coroutine):
        events.append(("asyncio.run", coroutine.cr_code.co_name))
        coroutine.close()

    monkeypatch.setattr(asyncio, "run", fake_asyncio_run)
    monkeypatch.setattr(sys, "argv", ["task_executor.py", "--type", "graphrag", "--index", "7"])
    run_name = "__main__" if run_as_main else "task_executor_runtime_contract"
    namespace = runpy.run_path(str(_ROOT / "rag" / "svr" / "task_executor.py"), run_name=run_name)
    return namespace, events


def _assert_runtime_registries(namespace):
    parser_factory = {key: module.__name__ for key, module in namespace["FACTORY"].items()}
    task_type_map = {key: value.value for key, value in namespace["TASK_TYPE_TO_PIPELINE_TASK_TYPE"].items()}
    assert parser_factory == _EXPECTED_PARSER_FACTORY
    assert task_type_map == _EXPECTED_TASK_TYPE_MAP


class _QueueMessage:
    def __init__(self, payload, events=None, name="message"):
        self.payload = payload
        self.events = events if events is not None else []
        self.name = name
        self.ack_count = 0

    def get_message(self):
        self.events.append(("get_message", self.name))
        return self.payload

    def get_msg_id(self):
        return self.name

    def ack(self):
        self.ack_count += 1
        self.events.append(("ack", self.name))


def test_task_executor_runtime_registries_are_exact(monkeypatch):
    namespace, events = _load_task_executor(monkeypatch)

    _assert_runtime_registries(namespace)
    assert events == []


def test_task_executor_main_bootstrap_is_exact(monkeypatch):
    namespace, events = _load_task_executor(monkeypatch, run_as_main=True)

    assert namespace["TASK_TYPE"] == "graphrag"
    assert namespace["TE_IDX"] == "7"
    assert namespace["CONSUMER_NAME"] == "task_executor_graphrag_7"
    assert events == [
        ("faulthandler",),
        ("root_logger", "task_executor_graphrag_7"),
        ("otel", "ragflow-ingestion", "test-version"),
        ("asyncio.run", "main"),
    ]


def test_task_executor_registry_contract_rejects_missing_parser(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    namespace["FACTORY"].pop("knowledge_graph")

    with pytest.raises(AssertionError):
        _assert_runtime_registries(namespace)


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("run_mode", "expected_execution"),
    [
        ("0", [("recording_context", "NullRecordingContext"), ("refactored", "task-1")]),
        (
            "1",
            [
                ("recording_context", "RecordingContext"),
                ("original", "task-1"),
                ("dry_run", "task-1", "RecordingContext"),
            ],
        ),
        ("legacy", [("recording_context", "NullRecordingContext"), ("original", "task-1")]),
    ],
)
async def test_task_executor_run_modes_preserve_cleanup_and_ack(monkeypatch, run_mode, expected_execution):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["handle_task"].__globals__
    events = []
    recording_context = None
    task = {"id": "task-1", "task_type": "dataflow", "dataflow_id": "pipeline-1"}

    class QueueMessage:
        def ack(self):
            events.append(("ack",))

    class TelemetryContext:
        def __enter__(self):
            events.append(("telemetry_enter",))

        def __exit__(self, *_args):
            events.append(("telemetry_exit",))

    class TaskManager:
        @staticmethod
        async def run_refactored_task(current_task, *_args):
            events.append(("refactored", current_task["id"]))

        @staticmethod
        async def dry_run_task(current_task, current_context, *_args):
            events.append(("dry_run", current_task["id"], type(current_context).__name__))

    async def collect():
        events.append(("collect",))
        return QueueMessage(), task

    async def do_handle_task(current_task):
        events.append(("original", current_task["id"]))

    def consume_queue_context(current_task, description):
        events.append(("telemetry", current_task["id"], description))
        return TelemetryContext()

    def set_recording_context(context):
        nonlocal recording_context
        recording_context = context
        events.append(("recording_context", type(context).__name__))

    monkeypatch.setenv("TE_RUN_MODE", run_mode)
    runtime_globals.update(
        {
            "collect": collect,
            "consume_queue_context": consume_queue_context,
            "set_recording_context": set_recording_context,
            "get_recording_context": lambda: recording_context,
            "TaskManager": TaskManager,
            "do_handle_task": do_handle_task,
            "CURRENT_TASKS": {},
            "DONE_TASKS": 0,
            "FAILED_TASKS": 0,
        }
    )

    await namespace["handle_task"]()

    assert events == [
        ("collect",),
        ("telemetry", "task-1", "task dataflow"),
        ("telemetry_enter",),
        *expected_execution,
        ("telemetry_exit",),
        ("ack",),
    ]
    assert runtime_globals["DONE_TASKS"] == 1
    assert runtime_globals["FAILED_TASKS"] == 0
    assert runtime_globals["CURRENT_TASKS"] == {}


@pytest.mark.asyncio
async def test_task_executor_main_cancels_report_and_workers_on_stop(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["main"].__globals__
    events = []

    class FakeTask:
        def __init__(self, coroutine):
            self.coroutine = coroutine
            self.name = coroutine.cr_code.co_name

        def cancel(self):
            events.append(("cancel", self.name))
            self.coroutine.close()

    class FakeAsyncio:
        @staticmethod
        def create_task(coroutine):
            task = FakeTask(coroutine)
            events.append(("create_task", task.name))
            return task

        @staticmethod
        async def gather(*tasks, return_exceptions):
            events.append(("gather", tuple(task.name for task in tasks), return_exceptions))

    class TaskLimiter:
        @staticmethod
        async def acquire():
            events.append(("acquire",))

    class StopEvent:
        checks = 0

        def is_set(self):
            self.checks += 1
            return self.checks > 1

    async def report_status():
        return None

    async def task_manager():
        return None

    settings = types.SimpleNamespace(
        EMBEDDING_CFG={},
        init_settings=lambda: events.append(("settings_init",)),
        check_and_install_torch=lambda: events.append(("torch_check",)),
        print_rag_settings=lambda: events.append(("settings_print",)),
    )
    fake_signal = types.SimpleNamespace(
        SIGINT="SIGINT",
        SIGTERM="SIGTERM",
        signal=lambda signal_name, handler: events.append(("signal", signal_name, handler.__name__)),
    )
    monkeypatch.delenv("TRACE_MALLOC_ENABLED", raising=False)
    runtime_globals.update(
        {
            "asyncio": FakeAsyncio,
            "random": types.SimpleNamespace(uniform=lambda *_args: 0),
            "sys": types.SimpleNamespace(platform="win32"),
            "settings": settings,
            "signal": fake_signal,
            "show_configs": lambda: events.append(("show_configs",)),
            "get_ragflow_version": lambda: events.append(("version",)) or "test-version",
            "report_status": report_status,
            "task_manager": task_manager,
            "task_limiter": TaskLimiter(),
            "stop_event": StopEvent(),
            "CONSUMER_NAME": "task_executor_common_0",
        }
    )

    await namespace["main"]()

    assert events == [
        ("version",),
        ("show_configs",),
        ("settings_init",),
        ("torch_check",),
        ("settings_print",),
        ("signal", "SIGINT", "signal_handler"),
        ("signal", "SIGTERM", "signal_handler"),
        ("create_task", "report_status"),
        ("acquire",),
        ("create_task", "task_manager"),
        ("cancel", "task_manager"),
        ("gather", ("task_manager",), True),
        ("cancel", "report_status"),
        ("gather", ("report_status",), True),
    ]


@pytest.mark.asyncio
async def test_task_executor_collect_replays_unacked_before_live_queue(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["collect"].__globals__
    events = []
    unacked_message = _QueueMessage({"id": "unacked", "doc_id": "doc-1", "task_type": "graphrag"}, events, "unacked")
    live_message = _QueueMessage({"id": "live", "doc_id": "doc-2", "task_type": "raptor"}, events, "live")

    class RedisConnection:
        @staticmethod
        def get_unacked_iterator(queue_names, group_name, consumer_name):
            events.append(("get_unacked", tuple(queue_names), group_name, consumer_name))
            return iter([unacked_message])

        @staticmethod
        def queue_consumer(queue_name, group_name, consumer_name):
            events.append(("queue_consumer", queue_name, group_name, consumer_name))
            return live_message if queue_name == "queue-b" else None

    class TaskService:
        @staticmethod
        def get_task(task_id, *_args):
            events.append(("get_task", task_id))
            return {"id": task_id}

    settings = types.SimpleNamespace(get_svr_queue_names=lambda task_type: events.append(("queues", task_type)) or ["queue-a", "queue-b"])
    runtime_globals.update(
        {
            "settings": settings,
            "REDIS_CONN": RedisConnection(),
            "TaskService": TaskService,
            "has_canceled": lambda task_id: events.append(("has_canceled", task_id)) or False,
            "UNACKED_ITERATOR": None,
            "TASK_TYPE": "common",
            "CONSUMER_NAME": "task_executor_common_2",
        }
    )

    first_message, first_task = await namespace["collect"]()
    second_message, second_task = await namespace["collect"]()

    assert first_message is unacked_message
    assert first_task == {"id": "unacked", "task_type": "graphrag"}
    assert second_message is live_message
    assert second_task == {"id": "live", "task_type": "raptor"}
    assert unacked_message.ack_count == live_message.ack_count == 0
    assert events == [
        ("queues", "common"),
        ("get_unacked", ("queue-a", "queue-b"), "rag_flow_svr_task_broker", "task_executor_common_2"),
        ("get_message", "unacked"),
        ("get_task", "unacked"),
        ("has_canceled", "unacked"),
        ("queues", "common"),
        ("queue_consumer", "queue-a", "rag_flow_svr_task_broker", "task_executor_common_2"),
        ("queue_consumer", "queue-b", "rag_flow_svr_task_broker", "task_executor_common_2"),
        ("get_message", "live"),
        ("get_task", "live"),
        ("has_canceled", "live"),
    ]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("scenario", "payload", "expected_failed"),
    [
        ("empty", {}, 0),
        ("unknown", {"id": "task-1", "doc_id": "doc-1", "task_type": "graphrag"}, 1),
        ("canceled", {"id": "task-1", "doc_id": "doc-1", "task_type": "graphrag"}, 1),
    ],
)
async def test_task_executor_collect_acks_non_runnable_messages(monkeypatch, scenario, payload, expected_failed):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["collect"].__globals__
    message = _QueueMessage(payload)

    class RedisConnection:
        @staticmethod
        def get_unacked_iterator(*_args):
            return iter([message])

    class TaskService:
        @staticmethod
        def get_task(task_id, *_args):
            return None if scenario == "unknown" else {"id": task_id}

    runtime_globals.update(
        {
            "settings": types.SimpleNamespace(get_svr_queue_names=lambda _task_type: ["queue-a"]),
            "REDIS_CONN": RedisConnection(),
            "TaskService": TaskService,
            "has_canceled": lambda _task_id: scenario == "canceled",
            "UNACKED_ITERATOR": None,
            "TASK_TYPE": "common",
            "CONSUMER_NAME": "task_executor_common_2",
            "FAILED_TASKS": 0,
        }
    )

    redis_message, task = await namespace["collect"]()

    assert redis_message is None
    assert task is None
    assert message.ack_count == 1
    assert runtime_globals["FAILED_TASKS"] == expected_failed


@pytest.mark.asyncio
async def test_task_executor_collect_failure_returns_no_task(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["collect"].__globals__

    class RedisConnection:
        @staticmethod
        def get_unacked_iterator(*_args):
            raise RuntimeError("redis unavailable")

    runtime_globals.update(
        {
            "settings": types.SimpleNamespace(get_svr_queue_names=lambda _task_type: ["queue-a"]),
            "REDIS_CONN": RedisConnection(),
            "UNACKED_ITERATOR": None,
            "TASK_TYPE": "common",
            "CONSUMER_NAME": "task_executor_common_2",
            "DONE_TASKS": 0,
            "FAILED_TASKS": 0,
        }
    )

    assert await namespace["collect"]() == (None, None)
    assert runtime_globals["DONE_TASKS"] == 0
    assert runtime_globals["FAILED_TASKS"] == 0


@pytest.mark.asyncio
async def test_task_executor_empty_poll_backs_off_without_counting(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["handle_task"].__globals__
    events = []

    async def collect():
        events.append(("collect",))
        return None, None

    async def sleep(seconds):
        events.append(("sleep", seconds))

    runtime_globals.update(
        {
            "collect": collect,
            "asyncio": types.SimpleNamespace(sleep=sleep),
            "DONE_TASKS": 0,
            "FAILED_TASKS": 0,
        }
    )

    await namespace["handle_task"]()

    assert events == [("collect",), ("sleep", 5)]
    assert runtime_globals["DONE_TASKS"] == 0
    assert runtime_globals["FAILED_TASKS"] == 0


def _configure_failing_handle_task(runtime_globals, events, failure):
    task = {"id": "task-1", "task_type": "dataflow", "dataflow_id": "pipeline-1"}
    message = _QueueMessage(task, events)

    class TelemetryContext:
        def __enter__(self):
            events.append(("telemetry_enter",))

        def __exit__(self, *_args):
            events.append(("telemetry_exit",))

    async def collect():
        return message, task

    async def do_handle_task(current_task):
        events.append(("original", current_task["id"]))
        raise failure

    runtime_globals.update(
        {
            "collect": collect,
            "consume_queue_context": lambda *_args: TelemetryContext(),
            "set_recording_context": lambda context: events.append(("recording_context", type(context).__name__)),
            "do_handle_task": do_handle_task,
            "CURRENT_TASKS": {},
            "DONE_TASKS": 0,
            "FAILED_TASKS": 0,
        }
    )
    return message


@pytest.mark.asyncio
async def test_task_executor_cancellation_completes_and_acks(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["handle_task"].__globals__
    events = []
    message = _configure_failing_handle_task(runtime_globals, events, runtime_globals["TaskCanceledException"]("canceled"))
    runtime_globals["set_progress"] = lambda *_args, **_kwargs: events.append(("set_progress",))
    monkeypatch.setenv("TE_RUN_MODE", "legacy")

    await namespace["handle_task"]()

    assert events == [
        ("telemetry_enter",),
        ("recording_context", "NullRecordingContext"),
        ("original", "task-1"),
        ("telemetry_exit",),
        ("ack", "message"),
    ]
    assert runtime_globals["DONE_TASKS"] == 1
    assert runtime_globals["FAILED_TASKS"] == 0
    assert runtime_globals["CURRENT_TASKS"] == {}
    assert message.ack_count == 1


@pytest.mark.asyncio
async def test_task_executor_failure_marks_trace_progress_and_acks(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["handle_task"].__globals__
    events = []
    failure = RuntimeError("boom")
    message = _configure_failing_handle_task(runtime_globals, events, failure)

    class Span:
        @staticmethod
        def record_exception(error):
            events.append(("trace_exception", error))

        @staticmethod
        def set_status(status):
            events.append(("trace_status", status.code))

    class Status:
        def __init__(self, code):
            self.code = code

    class StatusCode:
        ERROR = "ERROR"

    _install_package(monkeypatch, "opentelemetry")
    _install_module(monkeypatch, "opentelemetry.trace", get_current_span=lambda: Span(), Status=Status, StatusCode=StatusCode)
    runtime_globals["set_progress"] = lambda task_id, **kwargs: events.append(("set_progress", task_id, kwargs))
    monkeypatch.setenv("TE_RUN_MODE", "legacy")

    await namespace["handle_task"]()

    assert events == [
        ("telemetry_enter",),
        ("recording_context", "NullRecordingContext"),
        ("original", "task-1"),
        ("trace_exception", failure),
        ("trace_status", "ERROR"),
        ("set_progress", "task-1", {"prog": -1, "msg": "[Exception]: boom"}),
        ("telemetry_exit",),
        ("ack", "message"),
    ]
    assert runtime_globals["DONE_TASKS"] == 0
    assert runtime_globals["FAILED_TASKS"] == 1
    assert runtime_globals["CURRENT_TASKS"] == {}
    assert message.ack_count == 1


@pytest.mark.asyncio
async def test_task_executor_task_manager_releases_limiter_when_cancelled(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["task_manager"].__globals__
    events = []

    async def handle_task():
        events.append(("handle_task",))
        raise asyncio.CancelledError

    runtime_globals.update(
        {
            "handle_task": handle_task,
            "task_limiter": types.SimpleNamespace(release=lambda: events.append(("release",))),
        }
    )

    with pytest.raises(asyncio.CancelledError):
        await namespace["task_manager"]()

    assert events == [("handle_task",), ("release",)]


@pytest.mark.asyncio
async def test_task_executor_collect_hydrates_special_fanout_task(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["collect"].__globals__
    events = []
    payload = {
        "id": "task-special",
        "doc_id": "graph-raptor",
        "doc_ids": ["doc-1", "doc-2"],
        "task_type": "graphrag",
        "_telemetry": {"traceparent": "trace-1"},
    }
    message = _QueueMessage(payload, events, "special-message")

    class RedisConnection:
        @staticmethod
        def get_unacked_iterator(queue_names, group_name, consumer_name):
            events.append(("get_unacked", tuple(queue_names), group_name, consumer_name))
            return iter([message])

    class TaskService:
        @staticmethod
        def get_task(task_id, doc_ids):
            events.append(("get_task", task_id, tuple(doc_ids)))
            return {"id": task_id, "tenant_id": "tenant-1", "from_db": True}

    runtime_globals.update(
        {
            "settings": types.SimpleNamespace(get_svr_queue_names=lambda task_type: events.append(("queues", task_type)) or ["queue-special"]),
            "REDIS_CONN": RedisConnection(),
            "TaskService": TaskService,
            "PIPELINE_SPECIAL_PROGRESS_FREEZE_TASK_TYPES": {"graphrag"},
            "has_canceled": lambda task_id: events.append(("has_canceled", task_id)) or False,
            "UNACKED_ITERATOR": None,
            "TASK_TYPE": "graphrag",
            "CONSUMER_NAME": "task_executor_graphrag_4",
        }
    )

    redis_message, task = await namespace["collect"]()

    assert redis_message is message
    assert task == {
        "id": "task-special",
        "tenant_id": "tenant-1",
        "from_db": True,
        "doc_id": "graph-raptor",
        "doc_ids": ["doc-1", "doc-2"],
        "task_type": "graphrag",
        "_telemetry": {"traceparent": "trace-1"},
    }
    assert message.ack_count == 0
    assert events == [
        ("queues", "graphrag"),
        ("get_unacked", ("queue-special",), "rag_flow_svr_task_broker", "task_executor_graphrag_4"),
        ("get_message", "special-message"),
        ("get_task", "task-special", ("doc-1", "doc-2")),
        ("has_canceled", "task-special"),
    ]


@pytest.mark.asyncio
async def test_task_executor_records_special_pipeline_operation_before_ack(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["handle_task"].__globals__
    events = []
    task = {
        "id": "task-raptor",
        "doc_id": "graph-raptor",
        "doc_ids": ["doc-1", "doc-2"],
        "task_type": "raptor",
    }
    message = _QueueMessage(task, events, "raptor-message")

    class TelemetryContext:
        def __enter__(self):
            events.append(("telemetry_enter",))

        def __exit__(self, *_args):
            events.append(("telemetry_exit",))

    class RecordingContext:
        @staticmethod
        def save_func_return_value(name, value):
            events.append(("record_return", name, value))

    class PipelineOperationLogService:
        @staticmethod
        def record_pipeline_operation(**kwargs):
            events.append(
                (
                    "operation_log",
                    kwargs["document_id"],
                    kwargs["pipeline_id"],
                    kwargs["task_type"].value,
                    kwargs["task_id"],
                    kwargs["referred_document_id"],
                )
            )
            return "operation-log-1"

    async def collect():
        events.append(("collect",))
        return message, task

    async def do_handle_task(current_task):
        events.append(("execute", current_task["id"]))

    runtime_globals.update(
        {
            "collect": collect,
            "consume_queue_context": lambda *_args: TelemetryContext(),
            "set_recording_context": lambda context: events.append(("recording_context", type(context).__name__)),
            "get_recording_context": lambda: RecordingContext(),
            "do_handle_task": do_handle_task,
            "PipelineOperationLogService": PipelineOperationLogService,
            "CURRENT_TASKS": {},
            "DONE_TASKS": 0,
            "FAILED_TASKS": 0,
        }
    )
    monkeypatch.setenv("TE_RUN_MODE", "legacy")

    await namespace["handle_task"]()

    assert events == [
        ("collect",),
        ("telemetry_enter",),
        ("recording_context", "NullRecordingContext"),
        ("execute", "task-raptor"),
        ("operation_log", "graph-raptor", "", "RAPTOR", "task-raptor", "doc-1"),
        ("record_return", "PipelineOperationLogService.record_pipeline_operation", "operation-log-1"),
        ("telemetry_exit",),
        ("ack", "raptor-message"),
    ]
    assert runtime_globals["DONE_TASKS"] == 1
    assert runtime_globals["FAILED_TASKS"] == 0
    assert runtime_globals["CURRENT_TASKS"] == {}
    assert message.ack_count == 1


@pytest.mark.asyncio
async def test_task_executor_heartbeat_reports_and_cleans_expired_workers(monkeypatch):
    namespace, _events = _load_task_executor(monkeypatch)
    runtime_globals = namespace["report_status"].__globals__
    events = []
    fixed_now = runtime_globals["datetime"].fromisoformat("2026-09-08T12:00:00+03:00")
    now_timestamp = fixed_now.timestamp()

    class Clock:
        @staticmethod
        def now():
            return fixed_now

    class RedisConnection:
        REDIS = None

        def __init__(self):
            self.REDIS = self

        @staticmethod
        def sadd(key, value):
            events.append(("sadd", key, value))

        @staticmethod
        def queue_info(queue_name, group_name):
            events.append(("queue_info", queue_name, group_name))
            return {"pending": "3", "lag": "4"}

        @staticmethod
        def zadd(key, heartbeat, score):
            events.append(("zadd", key, runtime_globals["json"].loads(heartbeat), score))

        @staticmethod
        def zremrangebyscore(key, minimum, maximum):
            events.append(("zremrangebyscore", key, minimum, maximum))

        @staticmethod
        def smembers(key):
            events.append(("smembers", key))
            return ["task_executor_common_0", "healthy-worker", "stale-worker", "missing-worker", "unreadable-worker"]

        @staticmethod
        def zrevrange(worker_name, start, end, withscores):
            events.append(("zrevrange", worker_name, start, end, withscores))
            if worker_name == "healthy-worker":
                return [("heartbeat", now_timestamp - 10)]
            if worker_name == "stale-worker":
                return [("heartbeat", now_timestamp - 121)]
            if worker_name == "missing-worker":
                return []
            raise RuntimeError("heartbeat unavailable")

        @staticmethod
        def srem(key, value):
            events.append(("srem", key, value))

        @staticmethod
        def delete(key):
            events.append(("delete", key))

    class RedisLock:
        def __init__(self, name, *, lock_value, timeout):
            events.append(("lock_init", name, lock_value, timeout))

        @staticmethod
        def acquire():
            events.append(("lock_acquire",))
            return True

        @staticmethod
        def release():
            events.append(("lock_release",))

    async def get_server_ip():
        events.append(("server_ip",))
        return "10.0.0.5"

    async def sleep(seconds):
        events.append(("sleep", seconds))
        raise asyncio.CancelledError

    settings = types.SimpleNamespace(get_svr_queue_name=lambda index: events.append(("queue_name", index)) or "queue-common")
    runtime_globals.update(
        {
            "datetime": Clock,
            "os": types.SimpleNamespace(getpid=lambda: 4321),
            "get_server_ip": get_server_ip,
            "asyncio": types.SimpleNamespace(sleep=sleep),
            "settings": settings,
            "REDIS_CONN": RedisConnection(),
            "RedisDistributedLock": RedisLock,
            "CONSUMER_NAME": "task_executor_common_0",
            "BOOT_AT": "2026-09-08T11:00:00.000+03:00",
            "CURRENT_TASKS": {"task-1": {"task_type": "dataflow"}},
            "PENDING_TASKS": 0,
            "LAG_TASKS": 0,
            "DONE_TASKS": 7,
            "FAILED_TASKS": 2,
            "WORKER_HEARTBEAT_TIMEOUT": 120,
        }
    )

    with pytest.raises(asyncio.CancelledError):
        await namespace["report_status"]()

    expected_heartbeat = {
        "ip_address": "10.0.0.5",
        "pid": 4321,
        "name": "task_executor_common_0",
        "now": fixed_now.astimezone().isoformat(timespec="milliseconds"),
        "boot_at": "2026-09-08T11:00:00.000+03:00",
        "pending": 3,
        "lag": 4,
        "done": 7,
        "failed": 2,
        "current": {"task-1": {"task_type": "dataflow"}},
    }
    assert events == [
        ("server_ip",),
        ("sadd", "TASKEXE", "task_executor_common_0"),
        ("lock_init", "clean_task_executor", "task_executor_common_0", 60),
        ("queue_name", 0),
        ("queue_info", "queue-common", "rag_flow_svr_task_broker"),
        ("zadd", "task_executor_common_0", expected_heartbeat, now_timestamp),
        ("zremrangebyscore", "task_executor_common_0", 0, now_timestamp - 60 * 30),
        ("lock_acquire",),
        ("smembers", "TASKEXE"),
        ("zrevrange", "healthy-worker", 0, 0, True),
        ("zrevrange", "stale-worker", 0, 0, True),
        ("srem", "TASKEXE", "stale-worker"),
        ("delete", "stale-worker"),
        ("zrevrange", "missing-worker", 0, 0, True),
        ("srem", "TASKEXE", "missing-worker"),
        ("delete", "missing-worker"),
        ("zrevrange", "unreadable-worker", 0, 0, True),
        ("lock_release",),
        ("sleep", 30),
    ]
    assert runtime_globals["PENDING_TASKS"] == 3
    assert runtime_globals["LAG_TASKS"] == 4
