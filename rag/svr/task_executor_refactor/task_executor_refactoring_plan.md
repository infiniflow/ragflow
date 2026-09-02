# Task Executor Refactoring Plan

## 1. Current State Analysis

### 1.1 Original File
- **File Location**: `rag/svr/task_executor.py`
- **Lines of Code**: Approximately 1,780 lines
- **Primary Responsibilities**: Task consumption, document chunking, vectorization, index building, RAPTOR/GraphRAG processing, heartbeat reporting

### 1.2 Identified Issues

| Issue Type | Specific Manifestation |
|------------|------------------------|
| Single Responsibility Violation | One file handles 7+ different responsibilities |
| Global State | Global variables like `DONE_TASKS`, `FAILED_TASKS`, `CURRENT_TASKS` |
| Tight Coupling | Direct dependencies on `TaskService`, `DocumentService`, `REDIS_CONN`, etc. |
| Untestable | Functions depend on global state and external services, difficult to mock |
| Hardcoded Configuration | `BATCH_SIZE`, `FACTORY`, etc. hardcoded in the file |

---

## 2. Implemented Architecture

### 2.1 Actual Module Structure

```
rag/svr/task_executor_refactor/
├── task_context.py              # Task context encapsulation (~450 lines)
├── recording_context.py         # Execution result recording context (~330 lines)
├── write_operation_interceptor.py # Write operation interceptor (~130 lines)
├── chunk_service.py             # Document chunking service (~430 lines)
├── chunk_builder.py             # Chunk building logic (~130 lines)
├── chunk_post_processor.py      # Post-chunking logic (~350 lines)
├── embedding_service.py         # Embedding service (~130 lines)
├── embedding_utils.py           # Embedding utility functions (~210 lines)
├── raptor_service.py            # RAPTOR processing service (~520 lines)
├── raptor_utils.py              # RAPTOR utility functions (~100 lines)
├── dataflow_service.py          # Dataflow pipeline service (~430 lines)
├── post_processor.py            # Post-processing service (~150 lines)
├── comparator.py                # Comparator (~550 lines)
├── report_generator.py          # Report generator (~130 lines)
├── task_handler.py              # Task handler entry point (~630 lines)
├── task_manager.py              # Task manager (~200 lines)
├── constants.py                 # Constant definitions (~25 lines)
└── insert_service.py            # Insert service (~150 lines)

test/unit_test/rag/svr/task_executor_refactor/
├── conftest.py                  # Shared test fixtures (~260 lines)
├── test_task_context.py         # TaskContext tests (~410 lines)
├── test_recording_context.py    # RecordingContext tests (~330 lines)
├── test_write_operation_interceptor.py # Interceptor tests (~450 lines)
├── test_chunk_service.py        # ChunkService tests (~560 lines)
├── test_chunk_builder.py        # ChunkBuilder tests (~290 lines)
├── test_chunk_post_processor.py # ChunkPostProcessor tests (~550 lines)
├── test_embedding_service.py    # EmbeddingService tests (~190 lines)
├── test_embedding_utils.py      # EmbeddingUtils tests (~370 lines)
├── test_raptor_service.py       # RaptorService tests (~350 lines)
├── test_dataflow_service.py     # DataflowService tests (~250 lines)
├── test_post_processor.py       # PostProcessor tests (~120 lines)
├── test_comparator.py           # Comparator tests (~570 lines)
├── test_task_handler.py         # TaskHandler unit tests (~800 lines)
├── test_task_handler_integration.py # TaskHandler integration tests (~1400 lines)
└── test_constants.py            # Constants tests (~40 lines)
```

### 2.2 Layered Architecture Design

```
┌─────────────────────────────────────────────────────────────────┐
│                        Business Layer                            │
│                        task_handler.py                           │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  TaskHandler Class                                         │  │
│  │  ├── handle_task()     # Entry point, handles cancellation and exceptions │  │
│  │  ├── handle()          # Task type routing dispatch        │  │
│  │  ├── _run_dataflow()   # Dataflow pipeline execution       │  │
│  │  ├── _run_raptor()     # RAPTOR summary generation         │  │
│  │  ├── _run_graphrag()   # GraphRAG knowledge graph          │  │
│  │  └── _run_standard_chunking() # Standard chunking flow     │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
│  Entry Functions:                                                │
│  ├── run_refactored_task()   # Refactored version entry          │
│  └── dry_run_task()          # Comparison mode entry             │
├─────────────────────────────────────────────────────────────────┤
│                        Service Layer                             │
│                                                                 │
│  ┌─────────────────┐  ┌──────────────────┐  ┌────────────────┐  │
│  │ ChunkService    │  │ EmbeddingService │  │ RaptorService  │  │
│  │                 │  │                  │  │                │  │
│  │ build_chunks()  │  │ embed_chunks()   │  │ run_raptor_    │  │
│  │ insert_chunks() │  │                  │  │   for_kb()     │  │
│  └─────────────────┘  └──────────────────┘  └────────────────┘  │
│                                                                 │
│  ┌─────────────────┐  ┌──────────────────┐  ┌────────────────┐  │
│  │DataflowService  │  │ PostProcessor    │  │ InsertService  │  │
│  │                 │  │                  │  │                │  │
│  │ run_dataflow()  │  │ process_table_   │  │ insert_chunks()│  │
│  │                 │  │   parser_        │  │                │  │
│  │                 │  │   metadata()     │  │                │  │
│  └─────────────────┘  └──────────────────┘  └────────────────┘  │
│                                                                 │
│  ┌─────────────────┐  ┌──────────────────┐                      │
│  │ ChunkBuilder    │  │ChunkPostProcessor│                      │
│  │                 │  │                  │                      │
│  │ Chunk building  │  │ Post-processing  │                      │
│  │ logic           │  │ logic            │                      │
│  └─────────────────┘  └──────────────────┘                      │
├─────────────────────────────────────────────────────────────────┤
│                     Infrastructure Layer                         │
│                                                                 │
│  ┌─────────────────┐  ┌──────────────────┐  ┌────────────────┐  │
│  │ TaskContext     │  │ RecordingContext │  │ Comparator     │  │
│  │                 │  │                  │  │                │  │
│  │ Task property   │  │ Execution result │  │ Production vs  │  │
│  │ accessors       │  │ recording        │  │ Dry-run        │  │
│  │ Rate limiter    │  │ Function return  │  │ Difference     │  │
│  │ encapsulation   │  │ value recording  │  │ report gen     │  │
│  │ Interceptor     │  │ Timing decorator │  │                │  │
│  │ references      │  │                  │  │                │  │
│  └─────────────────┘  └──────────────────┘  └────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────┐  ┌────────────────────┐   │
│  │ WriteOperationInterceptor        │  │ ReportGenerator    │   │
│  │                                  │  │                    │   │
│  │ Whitelist method interception    │  │ Difference report  │   │
│  │ Pre-recorded return value replay │  │ Formatted output   │   │
│  └──────────────────────────────────┘  └────────────────────┘   │
│                                                                 │
│  ┌──────────────────────────────────┐  ┌────────────────────┐   │
│  │ TaskManager                      │  │ Constants & Utils  │   │
│  │                                  │  │                    │   │
│  │ Task lifecycle management        │  │ CANVAS_DEBUG_      │   │
│  │ Task state tracking              │  │   DOC_ID           │   │
│  └──────────────────────────────────┘  │ GRAPH_RAPTOR_      │   │
│                                        │   FAKE_DOC_ID      │   │
│                                        └────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Core Design Patterns

### 3.1 Dependency Injection

All services receive `TaskContext` through constructors, rather than directly importing global state:

```python
class ChunkService:
    def __init__(self, ctx: TaskContext):
        self._task_context = ctx
```

### 3.2 Interceptor Pattern

`WriteOperationInterceptor` is used to replay production execution return values in comparison mode:

```python
# Comparison mode: intercept write operations
if ctx.write_interceptor:
    update_result = ctx.write_interceptor.intercept("KnowledgebaseService.update_by_id")
else:
    update_result = KnowledgebaseService.update_by_id(kb.id, {"parser_config": kb_parser_config})
```

### 3.3 Recording Context Pattern

`RecordingContext` captures intermediate results for comparison:

```python
# Record intermediate results
get_recording_context().record("chunks", chunks)
get_recording_context().record("token_count", token_count)
```

### 3.4 Factory Pattern

Parser modules are registered through factory mapping:

```python
PARSER_FACTORY = {}

def register_parser(parser_id: str, parser_module):
    PARSER_FACTORY[parser_id] = parser_module
```

---

## 4. Task Execution Flow

### 4.1 Standard Task Flow

```
run_refactored_task()
    │
    ▼
TaskContext Creation
    │
    ▼
TaskHandler.handle_task()
    │
    ├── try: handle()
    │       │
    │       ├── Task type judgment
    │       │   ├── "memory"    → handle_save_to_memory_task()
    │       │   ├── "dataflow"  → DataflowService.run_dataflow()
    │       │   ├── "raptor"    → _run_raptor()
    │       │   ├── "graphrag"  → _run_graphrag()
    │       │   ├── "mindmap"   → Placeholder
    │       │   └── Others      → _run_standard_chunking()
    │       │
    │       └── _run_standard_chunking()
    │           │
    │           ├── Bind embedding model
    │           ├── Retrieve storage binary
    │           ├── ChunkService.build_chunks()
    │           │   ├── File size validation
    │           │   ├── Parser chunking
    │           │   ├── Outline extraction
    │           │   ├── MinIO upload
    │           │   ├── Keyword extraction
    │           │   ├── Question generation
    │           │   ├── Metadata generation
    │           │   └── Content tagging
    │           ├── EmbeddingService.embed_chunks()
    │           ├── TOC generation (async)
    │           ├── ChunkService.insert_chunks()
    │           ├── PostProcessor.process_table_parser_metadata()
    │           ├── TOC insertion
    │           └── DocumentService.increment_chunk_num()
    │
    └── finally: Cancel task cleanup
```

### 4.2 Comparison Mode Flow

```
dry_run_task()
    │
    ├── Create WriteOperationInterceptor (using pre-recorded values from recording_ctx1)
    ├── Create new RecordingContext (recording_ctx2)
    ├── Set recording_context to recording_ctx2
    │
    ▼
TaskHandler.handle_task()  # Execute with interceptor replay
    │
    ▼
ContextComparator.compare(task_id, recording_ctx1, recording_ctx2)
    │
    ├── Key-by-key comparison
    ├── Generate difference report
    └── Output mismatched_keys and remaining_values
```

---

## 5. Testing Strategy

### 5.1 Test Coverage Status

| Module | Test File | Test Lines | Coverage Focus |
|--------|-----------|------------|----------------|
| `TaskContext` | `test_task_context.py` | ~410 | Property accessors, rate limiters, interceptors |
| `RecordingContext` | `test_recording_context.py` | ~330 | Record/retrieve, function return values, timing |
| `WriteOperationInterceptor` | `test_write_operation_interceptor.py` | ~450 | Whitelist validation, FIFO replay |
| `ChunkService` | `test_chunk_service.py` | ~560 | Chunking logic, post-processing, insertion |
| `ChunkBuilder` | `test_chunk_builder.py` | ~290 | Chunk building logic |
| `ChunkPostProcessor` | `test_chunk_post_processor.py` | ~550 | Post-processing logic |
| `EmbeddingService` | `test_embedding_service.py` | ~190 | Batch encoding, vector stacking |
| `EmbeddingUtils` | `test_embedding_utils.py` | ~370 | Text preparation, truncation, stacking |
| `RaptorService` | `test_raptor_service.py` | ~350 | RAPTOR execution |
| `DataflowService` | `test_dataflow_service.py` | ~250 | Dataflow execution |
| `PostProcessor` | `test_post_processor.py` | ~120 | Table metadata processing |
| `Comparator` | `test_comparator.py` | ~570 | Various type comparison logic |
| `TaskHandler` | `test_task_handler.py` | ~800 | Routing, model binding, task types |
| `TaskHandler` | `test_task_handler_integration.py` | ~1400 | Full flow integration tests |
| `constants.py` | `test_constants.py` | ~40 | Constant value validation |

**Total Test Code**: Approximately 6,700+ lines

### 5.2 Mock Strategy

```python
# conftest.py shared fixtures

@pytest.fixture
def mock_task():
    """Standard test task"""
    return {
        "id": "task-001",
        "task_type": "standard",
        "tenant_id": "tenant-001",
        "kb_id": "kb-001",
        "doc_id": "doc-001",
        "name": "test.pdf",
        ...
    }

@pytest.fixture
def mock_task_context(mock_task):
    """TaskContext fixture"""
    return TaskContext(
        task=mock_task,
        chat_limiter=asyncio.Semaphore(1),
        minio_limiter=asyncio.Semaphore(1),
        chunk_limiter=asyncio.Semaphore(1),
        embed_limiter=asyncio.Semaphore(1),
        kg_limiter=asyncio.Semaphore(1),
        progress_callback=lambda **kwargs: None,
        has_canceled_func=lambda task_id: False,
    )
```

### 5.3 Test Coverage Targets

| Module | Current Coverage | Target Coverage | Notes |
|--------|-----------------|-----------------|-------|
| `task_context.py` | ~90% | 95%+ | Good |
| `recording_context.py` | ~85% | 90%+ | Good |
| `write_operation_interceptor.py` | ~90% | 95%+ | Good |
| `chunk_service.py` | ~80% | 90%+ | Good |
| `chunk_builder.py` | ~75% | 85%+ | Needs more edge case tests |
| `chunk_post_processor.py` | ~80% | 90%+ | Good |
| `embedding_service.py` | ~85% | 90%+ | Good |
| `raptor_service.py` | ~70% | 85%+ | Improved |
| `dataflow_service.py` | ~75% | 85%+ | Good |
| `post_processor.py` | ~75% | 85%+ | Good |
| `comparator.py` | ~85% | 90%+ | Good |
| `task_handler.py` | ~75% | 85%+ | Needs more integration tests |

---

## 6. Backward Compatibility Strategy

### 6.1 Dual Code Path Coexistence

Original `task_executor.py` is preserved, importing refactored modules:

```python
# rag/svr/task_executor.py (modified)
from rag.svr.task_executor_refactor.task_handler import dry_run_task, run_refactored_task
from rag.svr.task_executor_refactor.recording_context import timed_with_recording, get_recording_context, \
    RecordingContext, set_recording_context
```

### 6.2 Migration Plan

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1 | ✅ Completed | Dual code paths parallel, `run_refactored_task()` and `dry_run_task()` available |
| Phase 2 | ⏳ Pending | Switch default execution to refactored code, keep old code as fallback |
| Phase 3 | ⏳ Pending | Remove old code after validation period |

---

## 7. Equivalence Guarantee Strategy

### 7.1 Comparison Mode

The refactoring introduces a unique comparison mode to verify equivalence:

1. **Production Execution**: Run original code path, record all intermediate results to `RecordingContext`
2. **Dry Run**: Use `WriteOperationInterceptor` to replay production results, record new intermediate results
3. **Comparison**: `ContextComparator` compares differences between two contexts

### 7.2 Comparison Strategy

| Data Type | Comparison Strategy |
|-----------|---------------------|
| Primitives (int, str, bool) | Direct equality |
| Floating point | Tolerance range |
| Lists | Length + ID set + sampled content |
| Dictionaries | Key set + recursive value comparison |
| None | Equal |

---

## 8. Risks and Mitigations

| Risk | Mitigation | Status |
|------|------------|--------|
| Refactoring introduces bugs | Comparison mode verifies equivalence | ✅ Implemented |
| Performance regression | Benchmark comparison | ⏳ Pending |
| Memory increase | RecordingContext stores intermediate results | ⚠️ Needs monitoring |
| Insufficient test coverage | Supplement RaptorService tests | ✅ Improved |
| Large modules | Split chunk_service.py | ✅ Split |

---

## 9. Future Improvement Suggestions

### 9.1 High Priority

1. **Performance Benchmarking**: Compare performance before and after refactoring
2. **Improve Integration Tests**: Add more end-to-end test scenarios
3. **Fix Type Annotations**: Add `Any` type for `default_value` and similar parameters

### 9.2 Medium Priority

4. **Improve Exception Handling**: Preserve more context information when wrapping exceptions
5. **Documentation Improvement**: Add usage examples to docstrings

### 9.3 Low Priority

6. **Memory Optimization**: Consider streaming recording for large tasks
7. **Code Cleanup**: Remove unused imports and functions

---

## 10. Code Statistics

### 10.1 Source Code

| Module | Lines | Type |
|--------|-------|------|
| `task_context.py` | ~450 | Infrastructure |
| `recording_context.py` | ~330 | Infrastructure |
| `write_operation_interceptor.py` | ~130 | Infrastructure |
| `comparator.py` | ~550 | Infrastructure |
| `report_generator.py` | ~130 | Infrastructure |
| `constants.py` | ~25 | Infrastructure |
| `task_manager.py` | ~200 | Infrastructure |
| `chunk_service.py` | ~430 | Service |
| `chunk_builder.py` | ~130 | Service |
| `chunk_post_processor.py` | ~350 | Service |
| `embedding_service.py` | ~130 | Service |
| `embedding_utils.py` | ~210 | Utility |
| `raptor_service.py` | ~520 | Service |
| `raptor_utils.py` | ~100 | Utility |
| `dataflow_service.py` | ~430 | Service |
| `post_processor.py` | ~150 | Service |
| `insert_service.py` | ~150 | Service |
| `task_handler.py` | ~630 | Business |
| **Source Code Total** | **~4,900** | |

### 10.2 Test Code

| Test File | Lines |
|-----------|-------|
| `conftest.py` | ~260 |
| `test_task_context.py` | ~410 |
| `test_recording_context.py` | ~330 |
| `test_write_operation_interceptor.py` | ~450 |
| `test_chunk_service.py` | ~560 |
| `test_chunk_builder.py` | ~290 |
| `test_chunk_post_processor.py` | ~550 |
| `test_embedding_service.py` | ~190 |
| `test_embedding_utils.py` | ~370 |
| `test_raptor_service.py` | ~350 |
| `test_dataflow_service.py` | ~250 |
| `test_post_processor.py` | ~120 |
| `test_comparator.py` | ~570 |
| `test_task_handler.py` | ~800 |
| `test_task_handler_integration.py` | ~1400 |
| `test_constants.py` | ~40 |
| **Test Code Total** | **~6,700+** |

### 10.3 Documentation

| Document | Lines |
|----------|-------|
| `task_executor_refactoring_plan.md` | This document |

---

## 11. Time Estimation

| Phase | Completed | Estimated Time |
|-------|-----------|----------------|
| Infrastructure Preparation | ✅ Completed | - |
| Core Logic Decoupling | ✅ Completed | - |
| Advanced Feature Decoupling | ✅ Completed | - |
| Test Writing | ✅ Mostly Completed | - |
| Performance Benchmarking | ⏳ Pending | 1-2 days |
| Migration to Production | ⏳ Pending | 1-2 days |
| **Remaining Total** | | **2-4 days** |

---

## 12. Summary

This refactoring has successfully decomposed the monolithic `task_executor.py` into a layered, testable module architecture:

- ✅ **Layered Architecture**: Infrastructure Layer → Service Layer → Business Layer
- ✅ **Dependency Injection**: Execution resources injected via `TaskContext`
- ✅ **Comparison Mode**: Innovative Production vs Dry-run comparison framework
- ✅ **Test Coverage**: Approximately 6,700+ lines of test code
- ✅ **Module Decomposition**: Large modules split into smaller responsibility units
- ⚠️ **Pending Improvements**: Performance benchmarking, production migration validation

**Overall Status**: Core refactoring completed, test coverage is good, ready for validation and migration phases.
