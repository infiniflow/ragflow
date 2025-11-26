# Canvas Execution Engine

## Tong Quan

Canvas engine orchestrates workflow execution, managing component lifecycle, state, and event streaming.

## File Location
```
/agent/canvas.py
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    CANVAS EXECUTION ENGINE                       │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                         Canvas Class                             │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  DSL Parsing                                             │   │
│  │  - components: Dict[id → component_config]               │   │
│  │  - globals: System + custom variables                    │   │
│  │  - path: Execution order list                           │   │
│  │  - history: Conversation history                        │   │
│  │  - memory: Agent memory                                 │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  State Management                                        │   │
│  │  - Component outputs                                     │   │
│  │  - Variable resolution                                   │   │
│  │  - Memory accumulation                                   │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Execution Control                                       │   │
│  │  - Path-based sequential execution                      │   │
│  │  - Parallel component batching (5 workers)              │   │
│  │  - Exception routing                                    │   │
│  │  - Cancellation support                                 │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Canvas Class

```python
class Canvas(Graph):
    def __init__(self, dsl: str, tenant_id=None, task_id=None):
        """
        Initialize Canvas from DSL JSON.

        Args:
            dsl: JSON string defining workflow
            tenant_id: Tenant ID for model access
            task_id: Task ID for cancellation
        """
        self.globals = {
            "sys.query": "",
            "sys.user_id": tenant_id,
            "sys.conversation_turns": 0,
            "sys.files": []
        }

        # Parse DSL
        dsl_dict = json.loads(dsl)
        self.components = dsl_dict.get("components", {})
        self.path = dsl_dict.get("path", [])
        self.history = dsl_dict.get("history", [])
        self.memory = dsl_dict.get("memory", [])

        # Initialize component objects
        for cpn_id, config in self.components.items():
            self._initialize_component(cpn_id, config)
```

## Main Execution Loop

```python
async def run(self, **kwargs):
    """
    Main execution engine.

    Yields:
        SSE events for workflow progress
    """
    # 1. Initialize workflow
    self.reset(mem=False)
    yield decorate("workflow_started", {"inputs": kwargs.get("inputs")})

    # 2. Batch execution
    idx = len(self.path) - 1

    while idx < len(self.path):
        to = len(self.path)

        # 3. Execute batch of components in parallel
        self._run_batch(idx, to, kwargs)

        # 4. Post-processing for each component
        for i in range(idx, to):
            cpn = self.get_component(self.path[i])
            cpn_obj = cpn["obj"]

            # Yield node_started event
            yield decorate("node_started", {
                "component_id": self.path[i],
                "component_name": cpn_obj.component_name
            })

            # Handle streaming outputs
            if isinstance(cpn_obj.output("content"), partial):
                for m in cpn_obj.output("content")():
                    yield decorate("message", {"content": m})

            # Yield node_finished event
            yield decorate("node_finished", {
                "component_id": self.path[i],
                "outputs": cpn_obj.output(),
                "elapsed_time": cpn_obj.output("_elapsed_time")
            })

            # Error handling with exception routes
            if cpn_obj.error():
                ex = cpn_obj.exception_handler()
                if ex and ex["goto"]:
                    self.path.extend(ex["goto"])

            # Route to next components
            self._route_downstream(cpn, cpn_obj)

        idx = to

    # 5. Workflow complete
    yield decorate("workflow_finished", {
        "outputs": self._get_final_outputs(),
        "elapsed_time": self._total_elapsed_time()
    })
```

## Batch Execution

```python
def _run_batch(self, f, t, kwargs):
    """
    Execute components in parallel (max 5 workers).
    """
    with ThreadPoolExecutor(max_workers=5) as executor:
        threads = []

        for i in range(f, t):
            cpn = self.get_component_obj(self.path[i])

            if cpn.component_name.lower() in ["begin", "userfillup"]:
                # Begin needs inputs from kwargs
                thr = executor.submit(
                    cpn.invoke,
                    inputs=kwargs.get("inputs", {})
                )
            else:
                # Other components get resolved inputs
                thr = executor.submit(
                    cpn.invoke,
                    **cpn.get_input()
                )

            threads.append(thr)

        # Wait for all to complete
        for t in threads:
            t.result()
```

## Routing Logic

```python
def _route_downstream(self, cpn, cpn_obj):
    """
    Determine next components to execute.
    """
    component_name = cpn_obj.component_name.lower()

    # 1. Iteration exit
    if component_name == "iterationitem" and cpn_obj.end():
        parent = self.get_component(cpn["parent_id"])
        self._extend_path(parent["downstream"])

    # 2. Dynamic routing (Categorize, Switch)
    elif component_name in ["categorize", "switch"]:
        next_ids = cpn_obj.output("_next")
        self._extend_path(next_ids)

    # 3. Enter iteration
    elif component_name == "iteration":
        start_id = cpn_obj.get_start()
        self._append_path(start_id)

    # 4. Exit sub-iteration
    elif not cpn["downstream"] and cpn_obj.get_parent():
        parent_start = cpn_obj.get_parent().get_start()
        self._append_path(parent_start)

    # 5. Standard downstream
    else:
        self._extend_path(cpn["downstream"])

def _extend_path(self, component_ids):
    """Add multiple components to path."""
    if component_ids:
        self.path.extend(component_ids)

def _append_path(self, component_id):
    """Add single component to path."""
    if component_id:
        self.path.append(component_id)
```

## Variable Resolution

```python
def get_variable_value(self, exp: str) -> Any:
    """
    Resolve variable expression.

    Examples:
        - sys.query → self.globals["sys.query"]
        - LLM:0@content → component output
        - data@items[0].name → nested property
    """
    exp = exp.strip("{").strip("}").strip(" ")

    # Case 1: System/environment variable
    if "@" not in exp:
        return self.globals.get(exp)

    # Case 2: Component output reference
    cpn_id, var_nm = exp.split("@")
    cpn = self.get_component(cpn_id)

    # Get root value
    parts = var_nm.split(".", 1)
    root_key = parts[0]
    root_val = cpn["obj"].output(root_key)

    # Case 3: Nested property access
    if len(parts) > 1:
        return self._get_nested_value(root_val, parts[1])

    return root_val

def _get_nested_value(self, obj: Any, path: str) -> Any:
    """
    Deep property access with list/dict/object support.
    """
    cur = obj

    for key in path.split('.'):
        if cur is None:
            return None

        # Handle JSON strings
        if isinstance(cur, str):
            try:
                cur = json.loads(cur)
            except:
                return None

        # Dict access
        if isinstance(cur, dict):
            cur = cur.get(key)
        # Array indexing
        elif isinstance(cur, (list, tuple)):
            cur = cur[int(key)]
        # Object attribute
        else:
            cur = getattr(cur, key, None)

    return cur
```

## State Management

```python
class Canvas:
    def reset(self, mem=True):
        """Reset canvas state for new execution."""
        # Reset path to initial component
        self.path = ["begin"]

        # Reset all component outputs
        for cpn_id in self.components:
            self.get_component_obj(cpn_id).reset()

        # Optionally reset memory
        if mem:
            self.memory = []

    def set_global_variable(self, key: str, value: Any):
        """Set global variable."""
        self.globals[key] = value

    def add_memory(self, user: str, assist: str, summ: str):
        """Store conversation memory."""
        self.memory.append((user, assist, summ))

    def get_history(self, window_size: int = 5) -> list:
        """Get recent conversation history."""
        return self.history[-window_size:]

    def to_json(self) -> str:
        """Serialize canvas state to JSON."""
        return json.dumps({
            "components": {
                cpn_id: {
                    "obj": cpn["obj"].to_dict(),
                    "downstream": cpn["downstream"],
                    "upstream": cpn["upstream"]
                }
                for cpn_id, cpn in self.components.items()
            },
            "globals": self.globals,
            "path": self.path,
            "history": self.history,
            "memory": self.memory
        })
```

## Cancellation Support

```python
def has_canceled(self) -> bool:
    """Check if task has been canceled."""
    if not self.task_id:
        return False

    # Check Redis flag
    return REDIS_CONN.get(f"cancel:{self.task_id}") == "1"

# Usage in run():
async def run(self, **kwargs):
    # ...
    while idx < len(self.path):
        # Check cancellation before each batch
        if self.has_canceled():
            yield decorate("workflow_finished", {
                "canceled": True
            })
            return
        # ...
```

## Event Decoration

```python
def decorate(event_type: str, data: dict) -> dict:
    """
    Format SSE event.

    Returns:
        {
            "event": event_type,
            "message_id": uuid,
            "created_at": timestamp,
            "data": data
        }
    """
    return {
        "event": event_type,
        "message_id": str(uuid.uuid4()),
        "created_at": int(time.time()),
        "data": data
    }
```

## Error Handling

```python
async def run(self, **kwargs):
    try:
        # ... execution logic ...
    except ComponentExecutionError as e:
        yield decorate("error", {
            "component_id": e.component_id,
            "message": str(e)
        })
    except TimeoutError:
        yield decorate("error", {
            "message": "Workflow execution timed out"
        })
    except Exception as e:
        logging.exception(e)
        yield decorate("error", {
            "message": str(e)
        })
    finally:
        # Always save state
        self._save_state()
```

## Performance Optimizations

```python
# Thread pool for parallel execution
ThreadPoolExecutor(max_workers=5)

# Capacity limiting for LLM calls
thread_limiter = trio.CapacityLimiter(
    int(os.environ.get('MAX_CONCURRENT_CHATS', 10))
)

# Component timeout
@timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
def _invoke(self, **kwargs):
    # ...
```

## Related Files

- `/agent/canvas.py` - Canvas implementation
- `/agent/component/base.py` - Component base class
- `/api/apps/canvas_app.py` - Canvas API
