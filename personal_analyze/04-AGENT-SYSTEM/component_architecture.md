# Component Architecture

## Tong Quan

Components là building blocks của Agent workflows, với shared base class và standardized lifecycle.

## File Location
```
/agent/component/base.py
/agent/component/__init__.py
```

## Class Hierarchy

```
ComponentParamBase (ABC)
    └─ All component parameters inherit from this

ComponentBase (ABC)
    ├─ LLM
    │   ├─ Categorize
    │   └─ Agent (LLM + ToolBase)
    ├─ Retrieval (ToolBase)
    ├─ Message
    ├─ Switch
    ├─ Iteration
    ├─ IterationItem
    ├─ Begin (extends UserFillUp)
    ├─ UserFillUp
    ├─ VariableAssigner
    ├─ VariableAggregator
    ├─ DataOperations
    ├─ StringTransform
    ├─ Invoke (HTTP webhook)
    └─ 15+ other components
```

## ComponentBase Class

```python
class ComponentBase(ABC):
    component_name: str
    thread_limiter = trio.CapacityLimiter(
        int(os.environ.get('MAX_CONCURRENT_CHATS', 10))
    )

    # Variable reference pattern
    variable_ref_patt = r"\{* *\{([a-zA-Z:0-9]+@[A-Za-z0-9_.]+|sys\.[A-Za-z0-9_.]+|env\.[A-Za-z0-9_.]+)\} *\}*"

    def __init__(self, canvas, id, param: ComponentParamBase):
        """
        Initialize component.

        Args:
            canvas: Parent Canvas instance
            id: Component ID
            param: Component parameters
        """
        self._canvas = canvas
        self._id = id
        self._param = param
        self._param.check()  # Validate parameters

    @property
    def id(self) -> str:
        return self._id

    @property
    def component_name(self) -> str:
        return self.__class__.component_name
```

## Component Lifecycle

```
┌─────────────────────────────────────────────────────────────────┐
│                    COMPONENT LIFECYCLE                           │
└─────────────────────────────────────────────────────────────────┘

1. INITIALIZATION
   ├─ __init__(canvas, id, param)
   ├─ Validate parameters
   └─ Initialize internal state

2. INPUT RESOLUTION (get_input)
   ├─ For each input parameter:
   │   ├─ Check if variable reference
   │   ├─ Resolve from canvas.get_variable_value()
   │   └─ Store resolved value
   └─ Return input dict

3. INVOCATION (invoke)
   ├─ Set _created_time
   ├─ Try: _invoke(**kwargs)
   ├─ Except: Set _ERROR output
   └─ Set _elapsed_time

4. OUTPUT STORAGE (set_output)
   └─ Store results in _param.outputs

5. DOWNSTREAM ROUTING
   ├─ Return outputs to canvas
   └─ Canvas determines next components
```

## invoke() Method

```python
def invoke(self, **kwargs) -> dict[str, Any]:
    """
    Main entry point for component execution.

    Returns:
        Dict of all component outputs
    """
    # Track timing
    self.set_output("_created_time", time.perf_counter())

    try:
        self._invoke(**kwargs)  # Abstract method
    except Exception as e:
        self.set_output("_ERROR", str(e))
        logging.exception(e)

    # Calculate elapsed time
    self.set_output(
        "_elapsed_time",
        time.perf_counter() - self.output("_created_time")
    )

    return self.output()

@timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
def _invoke(self, **kwargs):
    """Override in subclasses."""
    raise NotImplementedError()
```

## Input/Output Management

```python
def output(self, var_nm: str = None) -> Union[dict[str, Any], Any]:
    """
    Get component output(s).

    Args:
        var_nm: Specific output name, or None for all

    Returns:
        Single output value or dict of all outputs
    """
    if var_nm:
        return self._param.outputs.get(var_nm, {}).get("value", "")

    return {k: o.get("value") for k, o in self._param.outputs.items()}

def set_output(self, key: str, value: Any):
    """
    Set component output.
    """
    if key not in self._param.outputs:
        self._param.outputs[key] = {
            "value": None,
            "type": str(type(value))
        }
    self._param.outputs[key]["value"] = value

def get_input(self, key: str = None) -> Union[Any, dict[str, Any]]:
    """
    Get input with variable interpolation.

    Resolves {{variable}} references from canvas.
    """
    res = {}

    for var, o in self.get_input_elements().items():
        v = self.get_param(var)

        if v is None:
            continue

        # Check for variable reference
        if isinstance(v, str) and self._canvas.is_reff(v):
            actual_value = self._canvas.get_variable_value(v)
            self.set_input_value(var, actual_value)
        else:
            self.set_input_value(var, v)

        res[var] = self.get_input_value(var)

    if key:
        return res.get(key)
    return res
```

## Parameter Validation

```python
class ComponentParamBase:
    def __init__(self):
        self.inputs = {}
        self.outputs = {}

    def check(self):
        """
        Validate component parameters.

        Override in subclasses for custom validation.
        """
        pass

class LLMParam(ComponentParamBase):
    def __init__(self):
        super().__init__()
        self.llm_id = ""
        self.sys_prompt = ""
        self.prompts = []
        self.temperature = 0.7
        self.max_tokens = 2048

    def check(self):
        if not self.llm_id:
            raise ValueError("llm_id is required")

    def gen_conf(self) -> dict:
        """Generate LLM configuration."""
        return {
            "temperature": self.temperature,
            "max_tokens": self.max_tokens
        }
```

## Exception Handling

```python
def error(self) -> str:
    """Get error message if any."""
    return self._param.outputs.get("_ERROR", {}).get("value")

def exception_handler(self) -> dict:
    """
    Return exception handling config.

    Returns:
        {
            "goto": [component_ids],
            "default_value": fallback_content
        }
    """
    if not self._param.exception_method:
        return None

    return {
        "goto": self._param.exception_goto,
        "default_value": self._param.exception_default_value
    }
```

## Component Registration

```python
# /agent/component/__init__.py

def component_class(class_name: str):
    """
    Dynamic class loader.

    Searches in multiple modules for component class.
    """
    for module_name in ["agent.component", "agent.tools", "rag.flow"]:
        try:
            module = importlib.import_module(module_name)
            return getattr(module, class_name)
        except (ImportError, AttributeError):
            pass

    raise AssertionError(f"Can't import {class_name}")

# Usage:
param_class = component_class("LLMParam")
component = component_class("LLM")(canvas, id, param)
```

## Standard Outputs

All components have these standard outputs:

| Output | Type | Description |
|--------|------|-------------|
| `_created_time` | float | Execution start time |
| `_elapsed_time` | float | Execution duration |
| `_ERROR` | str | Error message if failed |
| `content` | str | Main content output |

## Component-Specific Outputs

### LLM Component
```python
outputs = {
    "content": "Generated text...",
    "structured": {...},  # If output_structure defined
    "_think": "Reasoning..."  # If reasoning model
}
```

### Retrieval Component
```python
outputs = {
    "formalized_content": "Retrieved context...",
    "chunks": [...],
    "doc_aggs": [...]
}
```

### Categorize Component
```python
outputs = {
    "category_name": "selected_category",
    "_next": ["downstream_component_ids"]
}
```

### Switch Component
```python
outputs = {
    "_next": ["matched_route_component_ids"]
}
```

## Reset Method

```python
def reset(self):
    """
    Reset component state for re-execution.
    """
    # Clear outputs
    self._param.outputs = {}

    # Reset iteration index if applicable
    if hasattr(self, "_idx"):
        self._idx = 0

    # Clear cached data
    if hasattr(self, "_cache"):
        self._cache = {}
```

## Serialization

```python
def to_dict(self) -> dict:
    """Serialize component to dict."""
    return {
        "component_name": self.component_name,
        "params": {
            k: v for k, v in self._param.__dict__.items()
            if not k.startswith("_")
        }
    }

@classmethod
def from_dict(cls, canvas, id, data: dict):
    """Deserialize component from dict."""
    param_class = component_class(f"{data['component_name']}Param")
    param = param_class()

    for k, v in data.get("params", {}).items():
        setattr(param, k, v)

    return cls(canvas, id, param)
```

## Related Files

- `/agent/component/base.py` - ComponentBase class
- `/agent/component/__init__.py` - Component registry
- `/agent/component/llm.py` - LLM component
- `/agent/component/message.py` - Message component
