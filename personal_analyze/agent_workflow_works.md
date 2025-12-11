# Cách Agent Workflow Hoạt Động trong RAGFlow

## Tổng Quan Hệ Thống

RAGFlow triển khai một hệ thống agent workflow mạnh mẽ cho phép người dùng định nghĩa và thực thi các quy trình làm việc phức tạp thông qua một **Domain Specific Language (DSL)** dạng JSON. Hệ thống này được xây dựng dựa trên kiến trúc đồ thị (graph-based architecture) với các component có thể kết nối và tương tác với nhau.

---

## 1. Kiến Trúc DSL (Domain Specific Language)

### 1.1. Cấu Trúc Tổng Thể của DSL

DSL trong RAGFlow là một cấu trúc JSON được thiết kế để mô tả workflow dưới dạng đồ thị có hướng (Directed Graph). Mỗi workflow được định nghĩa trong file JSON với cấu trúc:

```json
{
  "components": {
    "component_id": {
      "obj": {
        "component_name": "ComponentType",
        "params": { /* cấu hình component */ }
      },
      "downstream": ["next_component_id"],
      "upstream": ["previous_component_id"]
    }
  },
  "globals": {
    "sys.query": "",
    "sys.user_id": "",
    "sys.conversation_turns": 0,
    "sys.files": []
  },
  "variables": { /* biến do người dùng định nghĩa */ },
  "history": [ /* lịch sử hội thoại */ ],
  "path": ["begin"],
  "retrieval": { "chunks": [], "doc_aggs": [] },
  "memory": []
}
```

**File tham chiếu**: `ragflow/agent/canvas.py:36-73`

### 1.2. Các Thành Phần Chính của DSL

#### a) **Components** - Các Node trong Đồ Thị

Mỗi component đại diện cho một bước xử lý trong workflow. Các component được định danh bằng ID duy nhất theo format `ComponentType:UniqueIdentifier`.

**Ví dụ thực tế từ customer_service.json**:
```json
"Agent:TwelveOwlsWatch": {
  "downstream": ["VariableAggregator:FuzzyBerriesFlow"],
  "obj": {
    "component_name": "Agent",
    "params": {
      "llm_id": "deepseek-chat@DeepSeek",
      "max_rounds": 5,
      "sys_prompt": "You are a friendly and casual conversational assistant...",
      "prompts": [
        {
          "content": "The user query is {sys.query}",
          "role": "user"
        }
      ]
    }
  },
  "upstream": ["Categorize:DullFriendsThank"]
}
```

**File tham chiếu**: `ragflow/agent/templates/customer_service.json:116-166`

#### b) **Downstream/Upstream** - Định Nghĩa Luồng Thực Thi

- **downstream**: Danh sách các component tiếp theo sẽ được thực thi
- **upstream**: Danh sách các component đã thực thi trước đó

Đây là cách DSL mô tả directed graph - các node kết nối với nhau tạo thành luồng xử lý.

**Logic trong code** (`ragflow/agent/canvas.py:543`):
```python
# Sau khi một component thực thi xong, hệ thống sẽ thêm các downstream vào path
_extend_path(cpn["downstream"])
```

#### c) **Globals** - Biến Hệ Thống

Các biến toàn cục được hệ thống quản lý:

- `sys.query`: Câu hỏi của người dùng
- `sys.user_id`: ID người dùng
- `sys.conversation_turns`: Số lượt hội thoại
- `sys.files`: Danh sách file đính kèm
- `env.*`: Biến môi trường do người dùng định nghĩa

**File tham chiếu**: `ragflow/agent/canvas.py:278-283, 343-348`

#### d) **Variables** - Hệ Thống Tham Chiếu Biến

DSL hỗ trợ tham chiếu động giữa các component thông qua cú pháp `{component_id@output_name}`.

**Ví dụ**:
```json
{
  "content": "The user query is {sys.query}\n\nThe relevant document are {Retrieval:ShyPumasJoke@formalized_content}"
}
```

**Regex pattern** (`ragflow/agent/component/base.py:396`):
```python
variable_ref_patt = r"\{* *\{([a-zA-Z:0-9]+@[A-Za-z0-9_.]+|sys\.[A-Za-z0-9_.]+|env\.[A-Za-z0-9_.]+)\} *\}*"
```

**Cơ chế resolve biến** (`ragflow/agent/canvas.py:158-183`):
```python
def get_value_with_variable(self, value: str) -> Any:
    pat = re.compile(r"\{* *\{([a-zA-Z:0-9]+@[A-Za-z0-9_.]+|sys\.[A-Za-z0-9_.]+|env\.[A-Za-z0-9_.]+)\} *\}*")
    out_parts = []
    last = 0

    for m in pat.finditer(value):
        out_parts.append(value[last:m.start()])
        key = m.group(1)
        v = self.get_variable_value(key)  # Lấy giá trị thực từ component hoặc globals
        # ... xử lý giá trị
        out_parts.append(rep)
        last = m.end()

    out_parts.append(value[last:])
    return "".join(out_parts)
```

### 1.3. Các Loại Component Có Sẵn

RAGFlow cung cấp một thư viện component phong phú:

| Component | File | Chức Năng |
|-----------|------|-----------|
| **Begin** | `agent/component/begin.py` | Entry point của workflow |
| **Agent** | `agent/component/agent_with_tools.py` | LLM agent với khả năng gọi tool |
| **LLM** | `agent/component/llm.py` | Gọi LLM cơ bản không có tool |
| **Retrieval** | `agent/tools/retrieval.py` | Tìm kiếm trong knowledge base |
| **Categorize** | `agent/component/categorize.py` | Phân loại intent bằng LLM |
| **Switch** | `agent/component/switch.py` | Điều kiện rẽ nhánh |
| **Iteration** | `agent/component/iteration.py` | Vòng lặp for-each |
| **Loop** | `agent/component/loop.py` | Vòng lặp while |
| **Message** | `agent/component/message.py` | Format output trả về user |
| **VariableAggregator** | `agent/component/variable_assigner.py` | Gộp kết quả từ nhiều nhánh |

**File tham chiếu**: `ragflow/agent/component/__init__.py:51-58`

---

## 2. Execution Engine - Cơ Chế Thực Thi Workflow

### 2.1. Class Graph - Core Engine

Class `Graph` là base class chứa logic cốt lõi để load và thực thi workflow.

**File**: `ragflow/agent/canvas.py:34-273`

#### Quá Trình Load DSL

```python
def load(self):
    self.components = self.dsl["components"]

    # Duyệt qua từng component trong DSL
    for k, cpn in self.components.items():
        # Tạo object ComponentParam từ tên component
        param = component_class(cpn["obj"]["component_name"] + "Param")()
        param.update(cpn["obj"]["params"])

        # Validate parameters
        param.check()

        # Tạo object Component thực tế
        cpn["obj"] = component_class(cpn["obj"]["component_name"])(self, k, param)

    self.path = self.dsl["path"]
```

**File tham chiếu**: `ragflow/agent/canvas.py:84-101`

**Giải thích logic**:
1. Parse JSON DSL thành dictionary
2. Với mỗi component, tạo object Parameter tương ứng (VD: `AgentParam`, `LLMParam`)
3. Validate parameters bằng method `check()`
4. Tạo object Component thực tế (VD: `Agent`, `LLM`) và inject vào graph
5. Component được truyền reference tới `self` (Canvas) để có thể truy cập biến global

### 2.2. Class Canvas - Agent Workflow Executor

Class `Canvas` kế thừa `Graph` và triển khai logic đặc thù cho agent workflow.

**File**: `ragflow/agent/canvas.py:275-676`

#### Phương Thức `run()` - Trái Tim của Execution Engine

Đây là async generator function thực thi workflow và yield các event real-time.

**Signature**:
```python
async def run(self, **kwargs):
    # Nhận tham số: query, files, user_id, inputs
```

**File tham chiếu**: `ragflow/agent/canvas.py:358-583`

#### Chi Tiết Từng Bước Thực Thi

##### **Bước 1: Khởi Tạo State**

```python
st = time.perf_counter()
self.message_id = get_uuid()
created_at = int(time.time())

# Lưu query vào history
self.add_user_input(kwargs.get("query"))

# Reset output của tất cả component
for k, cpn in self.components.items():
    self.components[k]["obj"].reset(True)
```

**File tham chiếu**: `ragflow/agent/canvas.py:359-364`

##### **Bước 2: Set System Variables**

```python
for k in kwargs.keys():
    if k in ["query", "user_id", "files"] and kwargs[k]:
        if k == "files":
            self.globals[f"sys.{k}"] = FileService.get_files(kwargs[k])
        else:
            self.globals[f"sys.{k}"] = kwargs[k]

# Tăng conversation turn counter
self.globals["sys.conversation_turns"] += 1
```

**File tham chiếu**: `ragflow/agent/canvas.py:372-380`

**Logic**: Các tham số từ user được map vào global variables để các component có thể reference bằng `{sys.query}`, `{sys.files}`, v.v.

##### **Bước 3: Path Initialization**

```python
if not self.path or self.path[-1].lower().find("userfillup") < 0:
    self.path.append("begin")
    self.retrieval.append({"chunks": [], "doc_aggs": []})
```

**File tham chiếu**: `ragflow/agent/canvas.py:393-395`

**Logic**: Path là một list lưu trữ thứ tự các component đã/đang/sẽ thực thi. Mọi workflow đều bắt đầu từ component `begin`.

##### **Bước 4: Yield Workflow Started Event**

```python
def decorate(event, dt):
    return {
        "event": event,
        "message_id": self.message_id,
        "created_at": created_at,
        "task_id": self.task_id,
        "data": dt
    }

yield decorate("workflow_started", {"inputs": kwargs.get("inputs")})
```

**File tham chiếu**: `ragflow/agent/canvas.py:382-402`

**Logic**: Hệ thống sử dụng Server-Sent Events (SSE) để stream các event về frontend real-time. Mỗi event có format chuẩn với `event`, `message_id`, `task_id`, `data`.

##### **Bước 5: Execute Components in Path**

```python
idx = len(self.path) - 1

while idx < len(self.path):
    to = len(self.path)

    # Yield node_started events
    for i in range(idx, to):
        yield decorate("node_started", {
            "component_id": self.path[i],
            "component_name": self.get_component_name(self.path[i]),
            "component_type": self.get_component_type(self.path[i])
        })

    # Execute batch of components
    _run_batch(idx, to)

    # ... post-processing
```

**File tham chiếu**: `ragflow/agent/canvas.py:444-548`

**Logic**:
- `path` là dynamic array có thể mở rộng trong quá trình thực thi
- Mỗi iteration thực thi một batch component từ `idx` đến `to`
- Sau khi thực thi, các component có thể thêm downstream vào `path`, làm `len(self.path)` tăng
- Vòng lặp tiếp tục cho đến khi không còn component nào trong path

##### **Bước 6: Parallel Execution với ThreadPoolExecutor**

```python
def _run_batch(f, t):
    if self.is_canceled():
        raise TaskCanceledException(...)

    with ThreadPoolExecutor(max_workers=5) as executor:
        thr = []
        i = f
        while i < t:
            cpn = self.get_component_obj(self.path[i])

            if cpn.component_name.lower() in ["begin", "userfillup"]:
                thr.append(executor.submit(cpn.invoke, inputs=kwargs.get("inputs", {})))
                i += 1
            else:
                # Kiểm tra dependencies
                for _, ele in cpn.get_input_elements().items():
                    if isinstance(ele, dict) and ele.get("_cpn_id") and ele.get("_cpn_id") not in self.path[:i]:
                        # Nếu dependency chưa execute, skip component này
                        self.path.pop(i)
                        t -= 1
                        break
                else:
                    # Execute component
                    thr.append(executor.submit(cpn.invoke, **cpn.get_input()))
                    i += 1

        # Wait for all threads to complete
        for t in thr:
            t.result()
```

**File tham chiếu**: `ragflow/agent/canvas.py:405-429`

**Logic quan trọng**:
- Hệ thống thực thi **tối đa 5 component song song** để tăng hiệu suất
- Trước khi execute, kiểm tra dependencies: nếu component A reference output của component B mà B chưa execute, thì A bị remove khỏi path
- Mỗi component được execute trong thread riêng biệt, nhưng vẫn đợi tất cả thread complete trước khi tiếp tục

##### **Bước 7: Post-Processing & Branching Logic**

```python
for i in range(idx, to):
    cpn = self.get_component(self.path[i])
    cpn_obj = self.get_component_obj(self.path[i])

    # Xử lý streaming output cho Message component
    if cpn_obj.component_name.lower() == "message":
        if isinstance(cpn_obj.output("content"), partial):
            _m = ""
            for m in cpn_obj.output("content")():
                if m == "<think>":
                    yield decorate("message", {"content": "", "start_to_think": True})
                elif m == "</think>":
                    yield decorate("message", {"content": "", "end_to_think": True})
                else:
                    yield decorate("message", {"content": m})
                    _m += m
            cpn_obj.set_output("content", _m)
        else:
            yield decorate("message", {"content": cpn_obj.output("content")})

    # Xử lý error handling
    if cpn_obj.error():
        ex = cpn_obj.exception_handler()
        if ex and ex["goto"]:
            self.path.extend(ex["goto"])  # Jump to error handler
        elif ex and ex["default_value"]:
            yield decorate("message", {"content": ex["default_value"]})
        else:
            self.error = cpn_obj.error()

    # Branching logic
    if cpn_obj.component_name.lower() in ["categorize", "switch"]:
        # Categorize/Switch component quyết định nhánh tiếp theo
        _extend_path(cpn_obj.output("_next"))
    elif cpn_obj.component_name.lower() in ("iteration", "loop"):
        # Loop component thêm start node vào path
        _append_path(cpn_obj.get_start())
    else:
        # Component thường thêm downstream vào path
        _extend_path(cpn["downstream"])

    # Yield node_finished event
    yield _node_finished(cpn_obj)
```

**File tham chiếu**: `ragflow/agent/canvas.py:459-543`

**Giải thích chi tiết**:

1. **Streaming Output**: Nếu component trả về `functools.partial`, hệ thống sẽ iterate và yield từng chunk text real-time
2. **Error Handling**: Component có thể định nghĩa exception handler với `goto` (jump to error component) hoặc `default_value` (fallback response)
3. **Branching**:
   - `Categorize`/`Switch`: Quyết định nhánh dựa trên classification result
   - `Iteration`/`Loop`: Tạo vòng lặp bằng cách thêm start node của loop vào path
   - Normal component: Thêm tất cả downstream vào path

##### **Bước 8: Workflow Completion**

```python
if not self.error:
    yield decorate("workflow_finished", {
        "inputs": kwargs.get("inputs"),
        "outputs": self.get_component_obj(self.path[-1]).output(),
        "elapsed_time": time.perf_counter() - st,
        "created_at": st
    })

    # Lưu vào conversation history
    self.history.append(("assistant", self.get_component_obj(self.path[-1]).output()))
```

**File tham chiếu**: `ragflow/agent/canvas.py:566-574`

---

## 3. Component Architecture - Cách Định Nghĩa Component

### 3.1. ComponentParamBase - Base Class cho Parameters

Mọi component đều có một class Parameter tương ứng để validate và lưu trữ config.

**File**: `ragflow/agent/component/base.py:37-391`

#### Ví Dụ: AgentParam

```python
class AgentParam(LLMParam, ToolParamBase):
    def __init__(self):
        super().__init__()
        self.function_name = "agent"
        self.tools = []
        self.mcp = []
        self.max_rounds = 5
        self.description = ""
```

**File tham chiếu**: `ragflow/agent/component/agent_with_tools.py:38-79`

#### Phương Thức Quan Trọng

```python
def update(self, conf, allow_redundant=False):
    """
    Đệ quy update parameters từ JSON config
    Hỗ trợ nested parameters và validation
    """
    # ... implementation

def check(self):
    """
    Validate parameters
    Được gọi sau update() để đảm bảo config hợp lệ
    """
    raise NotImplementedError("Parameter Object should be checked.")

def as_dict(self):
    """
    Convert parameters object thành dict để serialize
    """
    # ... implementation
```

**File tham chiếu**: `ragflow/agent/component/base.py:124-184, 54-55, 96-122`

### 3.2. ComponentBase - Base Class cho Component

**File**: `ragflow/agent/component/base.py:393-583`

#### Constructor

```python
def __init__(self, canvas, id, param: ComponentParamBase):
    from agent.canvas import Graph
    assert isinstance(canvas, Graph), "canvas must be an instance of Canvas"
    self._canvas = canvas  # Reference to workflow graph
    self._id = id          # Component ID
    self._param = param    # Parameters object
    self._param.check()    # Validate ngay khi khởi tạo
```

**File tham chiếu**: `ragflow/agent/component/base.py:412-418`

**Logic**: Mỗi component giữ reference tới Canvas để có thể:
- Truy cập global variables: `self._canvas.globals`
- Lấy output từ component khác: `self._canvas.get_variable_value("other_component@output")`
- Add reference (citations): `self._canvas.add_reference(chunks, doc_infos)`

#### Phương Thức invoke() - Entry Point Execution

```python
def invoke(self, **kwargs) -> dict[str, Any]:
    self.set_output("_created_time", time.perf_counter())

    try:
        self._invoke(**kwargs)  # Template method pattern
    except Exception as e:
        if self.get_exception_default_value():
            self.set_exception_default_value()
        else:
            self.set_output("_ERROR", str(e))
        logging.exception(e)

    self.set_output("_elapsed_time", time.perf_counter() - self.output("_created_time"))
    return self.output()
```

**File tham chiếu**: `ragflow/agent/component/base.py:434-446`

**Logic**:
- `invoke()` là public method được Canvas gọi
- Bên trong gọi `_invoke()` - abstract method mà subclass phải implement
- Tự động track `_created_time` và `_elapsed_time`
- Tự động catch exception và set `_ERROR` output

#### Abstract Method: _invoke()

```python
@timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
def _invoke(self, **kwargs):
    raise NotImplementedError()
```

**File tham chiếu**: `ragflow/agent/component/base.py:448-450`

**Logic**:
- Subclass phải override method này để implement logic
- Có timeout protection (default 10 phút)
- Nhận `**kwargs` là input variables đã được resolve

### 3.3. Ví Dụ Cụ Thể: Agent Component

**File**: `ragflow/agent/component/agent_with_tools.py`

#### Constructor - Load Tools

```python
def __init__(self, canvas, id, param: AgentParam):
    LLM.__init__(self, canvas, id, param)

    # Initialize tools dictionary
    self.tools = {}

    # Load built-in tool components
    for cpn in self._param.tools:
        cpn = self._load_tool_obj(cpn)
        self.tools[cpn.get_meta()["function"]["name"]] = cpn

    # Initialize LLM with multi-round support
    self.chat_mdl = LLMBundle(
        self._canvas.get_tenant_id(),
        TenantLLMService.llm_id2llm_type(self._param.llm_id),
        self._param.llm_id,
        max_retries=self._param.max_retries,
        retry_interval=self._param.delay_after_error,
        max_rounds=self._param.max_rounds,
        verbose_tool_use=True
    )

    # Collect tool metadata for LLM
    self.tool_meta = [v.get_meta() for _, v in self.tools.items()]

    # Load MCP (Model Context Protocol) tools
    for mcp in self._param.mcp:
        _, mcp_server = MCPServerService.get_by_id(mcp["mcp_id"])
        tool_call_session = MCPToolCallSession(mcp_server, mcp_server.variables)
        for tnm, meta in mcp["tools"].items():
            self.tool_meta.append(mcp_tool_metadata_to_openai_tool(meta))
            self.tools[tnm] = tool_call_session

    # Setup callback for tool usage tracking
    self.callback = partial(self._canvas.tool_use_callback, id)
    self.toolcall_session = LLMToolPluginCallSession(self.tools, self.callback)
```

**File tham chiếu**: `ragflow/agent/component/agent_with_tools.py:84-107`

**Logic**:
1. Load danh sách tools từ config (Retrieval, Wikipedia, TavilySearch, etc.)
2. Khởi tạo LLMBundle với config max_rounds (số vòng ReAct tối đa)
3. Load external tools từ MCP servers
4. Tạo callback để track tool usage (lưu vào Redis)

#### _invoke() Implementation - ReAct Loop

```python
@timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 20*60)))
def _invoke(self, **kwargs):
    if self.check_if_canceled("Agent processing"):
        return

    # Xử lý nested agent calls (khi agent A gọi agent B)
    if kwargs.get("user_prompt"):
        usr_pmt = ""
        if kwargs.get("reasoning"):
            usr_pmt += "\nREASONING:\n{}\n".format(kwargs["reasoning"])
        if kwargs.get("context"):
            usr_pmt += "\nCONTEXT:\n{}\n".format(kwargs["context"])
        usr_pmt += "\nQUERY:\n{}\n".format(str(kwargs["user_prompt"]))
        self._param.prompts = [{"role": "user", "content": usr_pmt}]

    # Nếu không có tools, fallback to simple LLM
    if not self.tools:
        return LLM._invoke(self, **kwargs)

    # Prepare prompts
    prompt, msg, user_defined_prompt = self._prepare_prompt_variables()

    # Check for structured output schema
    output_schema = self._get_output_schema()
    schema_prompt = ""
    if output_schema:
        schema = json.dumps(output_schema, ensure_ascii=False, indent=2)
        schema_prompt = structured_output_prompt(schema)

    # Check if next component is Message (for streaming)
    downstreams = self._canvas.get_component(self._id)["downstream"]
    ex = self.exception_handler()

    if any([self._canvas.get_component_obj(cid).component_name.lower()=="message" for cid in downstreams]) \
       and not (ex and ex["goto"]) and not output_schema:
        # Stream output directly to Message component
        self.set_output("content", partial(self.stream_output_with_tools, prompt, msg, user_defined_prompt))
        return

    # Non-streaming mode
    _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
    use_tools = []
    ans = ""

    # Execute ReAct loop
    for delta_ans, tk in self._react_with_tools_streamly(prompt, msg, use_tools, user_defined_prompt, schema_prompt=schema_prompt):
        if self.check_if_canceled("Agent processing"):
            return
        ans += delta_ans

    # Handle errors
    if ans.find("**ERROR**") >= 0:
        logging.error(f"Agent._chat got error. response: {ans}")
        if self.get_exception_default_value():
            self.set_output("content", self.get_exception_default_value())
        else:
            self.set_output("_ERROR", ans)
        return

    # Parse structured output if schema exists
    if output_schema:
        for _ in range(self._param.max_retries + 1):
            try:
                obj = json_repair.loads(clean_formated_answer(ans))
                self.set_output("structured", obj)
                if use_tools:
                    self.set_output("use_tools", use_tools)
                return obj
            except Exception:
                # Retry with format correction
                ans = self._force_format_to_schema(ans, schema_prompt)
        self.set_output("_ERROR", "The answer cannot be parsed as JSON")
        return

    # Normal output
    self.set_output("content", ans)
    if use_tools:
        self.set_output("use_tools", use_tools)
    return ans
```

**File tham chiếu**: `ragflow/agent/component/agent_with_tools.py:164-240`

#### ReAct Loop Implementation

```python
def _react_with_tools_streamly(self, prompt, history: list[dict], use_tools, user_defined_prompt={}, schema_prompt: str = ""):
    token_count = 0
    tool_metas = self.tool_meta
    hist = deepcopy(history)

    # Optimize multi-turn conversation
    if len(hist) > 3:
        st = timer()
        user_request = full_question(messages=history, chat_mdl=self.chat_mdl)
        self.callback("Multi-turn conversation optimization", {}, user_request, elapsed_time=timer()-st)
    else:
        user_request = history[-1]["content"]

    def use_tool(name, args):
        """Call tool and track usage"""
        tool_response = self.toolcall_session.tool_call(name, args)
        use_tools.append({
            "name": name,
            "arguments": args,
            "results": tool_response
        })
        return name, tool_response

    def complete():
        """Generate final answer with optional citation"""
        need2cite = self._param.cite and self._canvas.get_reference()["chunks"] and self._id.find("-->") < 0
        if schema_prompt:
            need2cite = False

        cited = False
        if hist and hist[0]["role"] == "system":
            if schema_prompt:
                hist[0]["content"] += "\n" + schema_prompt
            if need2cite and len(hist) < 7:
                hist[0]["content"] += citation_prompt()
                cited = True

        yield "", token_count

        # Truncate history if too long
        _hist = hist
        if len(hist) > 12:
            _hist = [hist[0], hist[1], *hist[-10:]]

        # Stream answer
        entire_txt = ""
        for delta_ans in self._generate_streamly(_hist):
            if not need2cite or cited:
                yield delta_ans, 0
            entire_txt += delta_ans

        # Generate citations if needed
        if need2cite and not cited:
            st = timer()
            txt = ""
            for delta_ans in self._gen_citations(entire_txt):
                if self.check_if_canceled("Agent streaming"):
                    return
                yield delta_ans, 0
                txt += delta_ans
            self.callback("gen_citations", {}, txt, elapsed_time=timer()-st)

    # Analyze task first
    st = timer()
    task_desc = analyze_task(self.chat_mdl, prompt, user_request, tool_metas, user_defined_prompt)
    self.callback("analyze_task", {}, task_desc, elapsed_time=timer()-st)

    # ReAct loop
    for _ in range(self._param.max_rounds + 1):
        if self.check_if_canceled("Agent streaming"):
            return

        # LLM decides next step (which tools to call or complete)
        response, tk = next_step(self.chat_mdl, hist, tool_metas, task_desc, user_defined_prompt)
        token_count += tk
        hist.append({"role": "assistant", "content": response})

        try:
            # Parse function calls from LLM response
            functions = json_repair.loads(re.sub(r"```.*", "", response))
            if not isinstance(functions, list):
                raise TypeError(f"List should be returned, but `{functions}`")

            # Execute tools in parallel
            with ThreadPoolExecutor(max_workers=5) as executor:
                thr = []
                for func in functions:
                    name = func["name"]
                    args = func["arguments"]

                    if name == COMPLETE_TASK:
                        # LLM quyết định task hoàn thành
                        for txt, tkcnt in complete():
                            yield txt, tkcnt
                        return

                    thr.append(executor.submit(use_tool, name, args))

                # Reflect on tool results
                st = timer()
                reflection = reflect(self.chat_mdl, hist, [th.result() for th in thr], user_defined_prompt)
                hist.append({"role": "user", "content": reflection})
                self.callback("reflection", {}, str(reflection), elapsed_time=timer()-st)

        except Exception as e:
            logging.exception(msg=f"Wrong JSON argument format in LLM ReAct response: {e}")
            e = f"\nTool call error, please correct the input parameter of response format and call it again.\n *** Exception ***\n{e}"
            hist.append({"role": "user", "content": str(e)})

    # Exceed max rounds, force completion
    logging.warning(f"Exceed max rounds: {self._param.max_rounds}")
    final_instruction = f"""
    {user_request}
    IMPORTANT: You have reached the conversation limit. Based on ALL the information and research you have gathered so far, please provide a DIRECT and COMPREHENSIVE final answer...
    """
    hist.append({"role": "user", "content": final_instruction})

    for txt, tkcnt in complete():
        yield txt, tkcnt
```

**File tham chiếu**: `ragflow/agent/component/agent_with_tools.py:273-406`

**Logic chi tiết của ReAct Loop**:

1. **Analyze Task**: LLM phân tích task và available tools để lập kế hoạch
2. **Loop until max_rounds**:
   - LLM quyết định next step: gọi tool nào hoặc complete task
   - Parse JSON response chứa list function calls
   - Execute tất cả tool calls song song (max 5 workers)
   - LLM reflect trên tool results để quyết định bước tiếp theo
3. **Tool Call**: Mỗi tool được execute và kết quả được append vào history
4. **Reflection**: LLM đánh giá tool results và quyết định có cần thêm information không
5. **Completion**: Khi LLM return `COMPLETE_TASK`, generate final answer
6. **Citation**: Nếu có retrieval results, tự động generate citations

---

## 4. Deep Research - Advanced Reasoning Engine

**File**: `ragflow/agentic_reasoning/deep_research.py`

### 4.1. Tổng Quan

`DeepResearcher` là một engine cao cấp implement multi-step reasoning với iterative search. Được sử dụng trong dialog service khi enable "Deep Reasoning" mode.

**File tham chiếu**: `ragflow/api/db/services/dialog_service.py:27, 441-463`

### 4.2. Architecture

```python
class DeepResearcher:
    def __init__(self,
                 chat_mdl: LLMBundle,
                 prompt_config: dict,
                 kb_retrieve: partial = None,
                 kg_retrieve: partial = None):
        self.chat_mdl = chat_mdl
        self.prompt_config = prompt_config
        self._kb_retrieve = kb_retrieve  # Knowledge base retrieval function
        self._kg_retrieve = kg_retrieve  # Knowledge graph retrieval function
```

**File tham chiếu**: `ragflow/agentic_reasoning/deep_research.py:27-37`

### 4.3. Thinking Loop

```python
def thinking(self, chunk_info, question):
    """
    Main reasoning loop với iterative search

    Args:
        chunk_info: Dictionary để lưu retrieved chunks (for citation)
        question: Câu hỏi của user

    Returns:
        Generator yield từng reasoning step
    """
    msg_history = [{"role": "user", "content": question}]
    all_reasoning_steps = []

    for step_index in range(MAX_SEARCH_LIMIT):  # Thường 3-5 steps
        # Step 1: Generate reasoning với LLM
        query_think = ""
        for ans in self._generate_reasoning(msg_history):
            query_think = ans
            yield query_think

        # Step 2: Extract search queries từ reasoning
        queries = self._extract_search_queries(query_think, question, step_index)

        if not queries:
            # Không còn query nào, reasoning complete
            break

        # Step 3: Execute searches
        for search_query in queries:
            # Retrieve from KB, Web, KG
            kbinfos = self._retrieve_information(search_query)

            # Update chunk_info for citation
            self._update_chunk_info(chunk_info, kbinfos)

            # Summarize relevant information
            summary_think = ""
            for ans in self._extract_relevant_info(
                self._truncate_previous_reasoning(all_reasoning_steps),
                search_query,
                kbinfos
            ):
                summary_think = ans
                yield summary_think

            # Append search result to reasoning
            query_think += f"\n{BEGIN_SEARCH_RESULT}\n{summary_think}\n{END_SEARCH_RESULT}"

        # Step 4: Save reasoning step
        all_reasoning_steps.append(query_think)
        msg_history.append({"role": "assistant", "content": query_think})
```

**File tham chiếu**: `ragflow/agentic_reasoning/deep_research.py` (method `thinking`)

**Logic**:

1. **Generate Reasoning**: LLM tạo chain-of-thought reasoning step
2. **Extract Queries**: Parse reasoning text để tìm `<|begin_search_query|>...<|end_search_query|>`
3. **Multi-source Retrieval**:
   - Knowledge Base (RAG)
   - Web Search (Tavily API)
   - Knowledge Graph
4. **Summarize**: LLM extract relevant info từ search results
5. **Iterate**: Append results vào history và continue reasoning
6. **Stop Condition**: Khi LLM không generate thêm search query

### 4.4. Prompt Engineering

**File**: `ragflow/agentic_reasoning/prompts.py`

```python
REASON_PROMPT = """
You are a research assistant performing deep reasoning to answer complex questions.

Instructions:
1. Break down the question into logical steps
2. For each step that requires external information, wrap search queries in tags:
   <|begin_search_query|>your search query here<|end_search_query|>
3. Use previous search results (wrapped in <|begin_search_result|>...<|end_search_result|>) to inform next steps
4. When you have enough information, provide final answer without additional searches

Current question: {question}

Previous reasoning:
{previous_steps}

Continue reasoning:
"""

RELEVANT_EXTRACTION_PROMPT = """
Given the following search results for query "{query}":

{search_results}

Extract and summarize ONLY the information directly relevant to answering:
{context}

Focus on facts, numbers, and specific details. Ignore irrelevant content.
"""
```

**File tham chiếu**: `ragflow/agentic_reasoning/prompts.py`

---

## 5. Branching & Control Flow Components

### 5.1. Categorize Component - LLM-based Intent Classification

**File**: `ragflow/agent/component/categorize.py`

#### Cách Hoạt Động

```python
class CategorizeParam(ComponentParamBase):
    def __init__(self):
        super().__init__()
        self.category_description = {
            "category_name": {
                "description": "Mô tả category",
                "examples": ["example 1", "example 2"],
                "to": ["next_component_id"]
            }
        }
        self.llm_id = ""
        self.query = "sys.query"
```

**Ví dụ từ customer_service.json**:
```json
{
  "category_description": {
    "1. contact": {
      "description": "User provides contact information",
      "examples": ["My phone is 123456", "john@email.com"],
      "to": ["Message:BreezyDonutsHeal"]
    },
    "2. casual": {
      "description": "Casual chat, not product related",
      "examples": ["How are you?", "What's your name?"],
      "to": ["Agent:TwelveOwlsWatch"]
    },
    "4. product related": {
      "description": "Questions about product usage",
      "examples": ["How to install?", "Why it doesn't work?"],
      "to": ["Retrieval:ShyPumasJoke"]
    }
  }
}
```

**File tham chiếu**: `ragflow/agent/templates/customer_service.json:177-213`

#### _invoke() Implementation

```python
def _invoke(self, **kwargs):
    # Get query from variable reference
    query = self._canvas.get_value_with_variable(self._param.query)

    # Build prompt with categories and examples
    prompt = "Classify the following query into one of these categories:\n\n"
    for cat_name, cat_info in self._param.category_description.items():
        prompt += f"{cat_name}: {cat_info['description']}\n"
        prompt += f"Examples: {', '.join(cat_info['examples'])}\n\n"

    prompt += f"Query: {query}\n\nCategory:"

    # Call LLM for classification
    response = self.chat_mdl.chat(prompt, [], {"temperature": 0.1})

    # Find matching category
    for cat_name, cat_info in self._param.category_description.items():
        if cat_name in response:
            self.set_output("category_name", cat_name)
            self.set_output("_next", cat_info["to"])
            return

    # Default to first category
    first_cat = list(self._param.category_description.values())[0]
    self.set_output("_next", first_cat["to"])
```

**Logic**:
1. LLM classify user query vào một trong các category
2. Set `_next` output = downstream của category đó
3. Canvas engine sẽ đọc `_next` và append vào path

### 5.2. Switch Component - Conditional Branching

**File**: `ragflow/agent/component/switch.py`

#### Example Configuration

```json
{
  "component_name": "Switch",
  "params": {
    "cases": [
      {
        "condition": "{sys.conversation_turns} > 5",
        "to": ["Agent:SuggestEnd"]
      },
      {
        "condition": "{User:Profile@premium} == true",
        "to": ["Agent:PremiumSupport"]
      }
    ],
    "default": ["Agent:StandardSupport"]
  }
}
```

#### Logic

```python
def _invoke(self, **kwargs):
    for case in self._param.cases:
        # Resolve variables in condition
        condition = self._canvas.get_value_with_variable(case["condition"])

        # Evaluate condition
        if eval(condition):
            self.set_output("_next", case["to"])
            return

    # Default branch
    self.set_output("_next", self._param.default)
```

### 5.3. Iteration & Loop Components

**Files**:
- `ragflow/agent/component/iteration.py`
- `ragflow/agent/component/loop.py`

#### Iteration - For-Each Loop

```json
{
  "component_name": "Iteration",
  "params": {
    "items": "{DataProcessor@results}",
    "item_var": "current_item"
  }
}
```

**Logic**:
```python
def _invoke(self, **kwargs):
    items = self._canvas.get_variable_value(self._param.items)

    if not isinstance(items, list):
        items = [items]

    for idx, item in enumerate(items):
        # Set item variable
        self._canvas.set_variable_value(self._param.item_var, item)

        # Add loop body to path
        self._canvas.path.append(self.get_start())  # Start of loop body
```

#### Loop - While Loop

```json
{
  "component_name": "Loop",
  "params": {
    "condition": "{attempt_count} < 3",
    "max_iterations": 10
  }
}
```

---

## 6. API Integration - Cách User Tương Tác với Workflow

### 6.1. REST API Endpoint

**File**: `ragflow/api/apps/canvas_app.py:124-178`

#### Endpoint: POST `/completion`

```python
@manager.route('/completion', methods=['POST'])
@validate_request("id")
@login_required
async def run():
    req = await request_json()
    query = req.get("query", "")
    files = req.get("files", [])
    inputs = req.get("inputs", {})
    user_id = req.get("user_id", current_user.id)

    # Permission check
    if not UserCanvasService.accessible(req["id"], current_user.id):
        return get_json_result(
            data=False,
            message='Only owner of canvas authorized for this operation.',
            code=RetCode.OPERATING_ERROR
        )

    # Load canvas DSL from database
    e, cvs = UserCanvasService.get_by_id(req["id"])
    if not e:
        return get_data_error_result(message="canvas not found.")

    if not isinstance(cvs.dsl, str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)

    # Create Canvas instance
    try:
        canvas = Canvas(cvs.dsl, current_user.id)
    except Exception as e:
        return server_error_response(e)

    # Server-Sent Events (SSE) stream
    async def sse():
        nonlocal canvas, user_id
        try:
            # Execute workflow và stream events
            async for ans in canvas.run(query=query, files=files, user_id=user_id, inputs=inputs):
                yield "data:" + json.dumps(ans, ensure_ascii=False) + "\n\n"

            # Save updated DSL (với updated history, variables, etc.)
            cvs.dsl = json.loads(str(canvas))
            UserCanvasService.update_by_id(req["id"], cvs.to_dict())

        except Exception as e:
            logging.exception(e)
            canvas.cancel_task()
            yield "data:" + json.dumps({
                "code": 500,
                "message": str(e),
                "data": False
            }, ensure_ascii=False) + "\n\n"

    # Return SSE response
    resp = Response(sse(), mimetype="text/event-stream")
    resp.headers.add_header("Cache-control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")
    resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
    return resp
```

**File tham chiếu**: `ragflow/api/apps/canvas_app.py:124-178`

### 6.2. Event Stream Format

Frontend nhận stream các event theo format:

```
data:{"event":"workflow_started","message_id":"uuid","task_id":"uuid","data":{"inputs":{}}}

data:{"event":"node_started","message_id":"uuid","data":{"component_id":"begin","component_name":"Begin"}}

data:{"event":"node_finished","message_id":"uuid","data":{"component_id":"begin","outputs":{},"elapsed_time":0.001}}

data:{"event":"node_started","message_id":"uuid","data":{"component_id":"Agent:xxx","component_name":"Agent"}}

data:{"event":"message","message_id":"uuid","data":{"content":"Hello"}}
data:{"event":"message","message_id":"uuid","data":{"content":" there"}}
data:{"event":"message","message_id":"uuid","data":{"content":"!"}}

data:{"event":"message_end","message_id":"uuid","data":{"reference":{"chunks":[],"doc_aggs":[]}}}

data:{"event":"node_finished","message_id":"uuid","data":{"component_id":"Agent:xxx"}}

data:{"event":"workflow_finished","message_id":"uuid","data":{"outputs":{"content":"Hello there!"},"elapsed_time":2.5}}
```

**Frontend có thể**:
- Track progress real-time
- Display streaming responses
- Show which component đang execute
- Handle errors gracefully

---

## 7. Cách Định Nghĩa Custom DSL - Hướng Dẫn Thực Hành

### 7.1. Bước 1: Thiết Kế Workflow Graph

Vẽ sơ đồ workflow với các node và edge:

```
[Begin]
   ↓
[Categorize Intent]
   ├→ "Order Status" → [Retrieval:OrderDB] → [Agent:OrderSupport] → [Message]
   ├→ "Product Info" → [Retrieval:ProductKB] → [Agent:ProductExpert] → [Message]
   └→ "General Chat" → [Agent:CasualChat] → [Message]
```

### 7.2. Bước 2: Viết JSON DSL

#### Template Cơ Bản

```json
{
  "components": {
    "begin": {
      "obj": {
        "component_name": "Begin",
        "params": {
          "prologue": "Welcome! How can I help you?",
          "mode": "conversational"
        }
      },
      "downstream": ["Categorize:IntentClassifier"],
      "upstream": []
    },

    "Categorize:IntentClassifier": {
      "obj": {
        "component_name": "Categorize",
        "params": {
          "llm_id": "deepseek-chat@DeepSeek",
          "query": "sys.query",
          "category_description": {
            "order_status": {
              "description": "User asking about order tracking or delivery status",
              "examples": [
                "Where is my order?",
                "When will my package arrive?"
              ],
              "to": ["Retrieval:OrderDB"]
            },
            "product_info": {
              "description": "Questions about product features, specs, or usage",
              "examples": [
                "What are the features?",
                "How to use this product?"
              ],
              "to": ["Retrieval:ProductKB"]
            },
            "general_chat": {
              "description": "Casual conversation not related to orders or products",
              "examples": [
                "Hello",
                "How are you?"
              ],
              "to": ["Agent:CasualChat"]
            }
          }
        }
      },
      "downstream": [
        "Retrieval:OrderDB",
        "Retrieval:ProductKB",
        "Agent:CasualChat"
      ],
      "upstream": ["begin"]
    },

    "Retrieval:OrderDB": {
      "obj": {
        "component_name": "Retrieval",
        "params": {
          "kb_ids": ["order_database_kb_id"],
          "query": "sys.query",
          "top_n": 5,
          "similarity_threshold": 0.3
        }
      },
      "downstream": ["Agent:OrderSupport"],
      "upstream": ["Categorize:IntentClassifier"]
    },

    "Agent:OrderSupport": {
      "obj": {
        "component_name": "Agent",
        "params": {
          "llm_id": "deepseek-chat@DeepSeek",
          "max_rounds": 3,
          "sys_prompt": "You are an order support specialist. Help users track their orders based on the database information provided.",
          "prompts": [
            {
              "role": "user",
              "content": "User question: {sys.query}\n\nOrder database results: {Retrieval:OrderDB@formalized_content}"
            }
          ],
          "tools": []
        }
      },
      "downstream": ["Message:FinalResponse"],
      "upstream": ["Retrieval:OrderDB"]
    },

    "Retrieval:ProductKB": {
      "obj": {
        "component_name": "Retrieval",
        "params": {
          "kb_ids": ["product_kb_id"],
          "query": "sys.query",
          "top_n": 8
        }
      },
      "downstream": ["Agent:ProductExpert"],
      "upstream": ["Categorize:IntentClassifier"]
    },

    "Agent:ProductExpert": {
      "obj": {
        "component_name": "Agent",
        "params": {
          "llm_id": "deepseek-chat@DeepSeek",
          "sys_prompt": "You are a product expert. Answer questions based on official product documentation.",
          "prompts": [
            {
              "role": "user",
              "content": "{sys.query}\n\nProduct docs: {Retrieval:ProductKB@formalized_content}"
            }
          ]
        }
      },
      "downstream": ["Message:FinalResponse"],
      "upstream": ["Retrieval:ProductKB"]
    },

    "Agent:CasualChat": {
      "obj": {
        "component_name": "Agent",
        "params": {
          "llm_id": "deepseek-chat@DeepSeek",
          "sys_prompt": "You are a friendly assistant for casual conversation.",
          "prompts": [
            {
              "role": "user",
              "content": "{sys.query}"
            }
          ]
        }
      },
      "downstream": ["Message:FinalResponse"],
      "upstream": ["Categorize:IntentClassifier"]
    },

    "Message:FinalResponse": {
      "obj": {
        "component_name": "Message",
        "params": {
          "content": [
            "{Agent:OrderSupport@content}",
            "{Agent:ProductExpert@content}",
            "{Agent:CasualChat@content}"
          ]
        }
      },
      "downstream": [],
      "upstream": [
        "Agent:OrderSupport",
        "Agent:ProductExpert",
        "Agent:CasualChat"
      ]
    }
  },

  "globals": {
    "sys.query": "",
    "sys.user_id": "",
    "sys.conversation_turns": 0,
    "sys.files": []
  },

  "variables": {},
  "history": [],
  "path": [],
  "retrieval": [],
  "memory": []
}
```

### 7.3. Bước 3: Variable References

#### Các Pattern Tham Chiếu

1. **System Variables**:
   ```json
   "{sys.query}"              // User's question
   "{sys.user_id}"            // User ID
   "{sys.conversation_turns}" // Conversation count
   "{sys.files}"              // Uploaded files
   ```

2. **Component Outputs**:
   ```json
   "{ComponentID@output_name}"

   // Examples:
   "{Retrieval:OrderDB@formalized_content}"
   "{Agent:ProductExpert@content}"
   "{Categorize:IntentClassifier@category_name}"
   ```

3. **Nested Access**:
   ```json
   "{Agent:Analysis@structured.summary}"
   "{DataProcessor@results.0.score}"
   ```

### 7.4. Bước 4: Upload và Test

```bash
# Upload canvas via API
curl -X POST http://localhost:9380/api/canvas/set \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "My Custom Workflow",
    "dsl": { ... your JSON DSL ... }
  }'

# Execute workflow
curl -X POST http://localhost:9380/api/canvas/completion \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "canvas_id_from_previous_response",
    "query": "Where is my order #12345?"
  }'
```

---

## 8. Advanced Features

### 8.1. Nested Agent Composition

Agent có thể gọi agent khác như một tool:

```json
{
  "component_name": "Agent",
  "params": {
    "llm_id": "deepseek-chat@DeepSeek",
    "tools": [
      {
        "component_name": "Agent",
        "name": "product_specialist",
        "params": {
          "llm_id": "deepseek-chat@DeepSeek",
          "sys_prompt": "You are a product specialist...",
          "user_prompt": "Answer this product question: {user_input}"
        }
      },
      {
        "component_name": "Agent",
        "name": "order_specialist",
        "params": {
          "llm_id": "deepseek-chat@DeepSeek",
          "sys_prompt": "You are an order tracking specialist..."
        }
      }
    ]
  }
}
```

**Cách hoạt động**:
- Supervisor agent phân tích query
- Quyết định gọi sub-agent nào
- Sub-agent execute và return result
- Supervisor synthesize final answer

**File tham chiếu**: `ragflow/agent/component/agent_with_tools.py:109-119`

### 8.2. Model Context Protocol (MCP) Integration

RAGFlow hỗ trợ external tools qua MCP:

```json
{
  "component_name": "Agent",
  "params": {
    "mcp": [
      {
        "mcp_id": "github_mcp_server_id",
        "tools": {
          "search_code": {
            "name": "search_code",
            "description": "Search code in GitHub repositories",
            "parameters": {
              "query": { "type": "string" },
              "repo": { "type": "string" }
            }
          }
        }
      }
    ]
  }
}
```

**File tham chiếu**: `ragflow/agent/component/agent_with_tools.py:99-104`

### 8.3. Structured Output Schema

Force agent return JSON theo schema:

```json
{
  "component_name": "Agent",
  "params": {
    "outputs": {
      "structured": {
        "type": "object",
        "properties": {
          "product_name": { "type": "string" },
          "price": { "type": "number" },
          "features": {
            "type": "array",
            "items": { "type": "string" }
          },
          "recommendation": { "type": "boolean" }
        },
        "required": ["product_name", "price"]
      }
    }
  }
}
```

**Logic** (`ragflow/agent/component/agent_with_tools.py:141-154, 215-235`):
1. Inject schema vào system prompt
2. LLM generate JSON
3. Validate và parse bằng `json_repair`
4. Retry nếu invalid JSON (max retries configurable)

### 8.4. Exception Handling

```json
{
  "component_name": "Retrieval",
  "params": {
    "kb_ids": ["kb_123"],
    "exception_method": "goto",
    "exception_goto": ["Agent:Fallback"],
    "exception_default_value": "I couldn't find information in our database."
  }
}
```

**Modes**:
- `goto`: Jump to specific component khi error
- `comment`: Return default value và continue
- `null`: Raise error và stop workflow

**File tham chiếu**: `ragflow/agent/component/base.py:565-579`

---

## 9. Tổng Kết Flow Hoàn Chỉnh

### User Request → Response Journey

```
1. User sends request
   POST /api/canvas/completion
   { "id": "canvas_123", "query": "Where is my order?" }

2. API loads DSL from database
   UserCanvasService.get_by_id(canvas_123)
   → DSL JSON

3. Canvas initialization
   canvas = Canvas(dsl_json, tenant_id)
   → Parse JSON
   → Create component objects
   → Validate parameters

4. Execute workflow
   async for event in canvas.run(query="Where is my order?"):

   4.1. Initialize state
       - Set sys.query = "Where is my order?"
       - Reset component outputs
       - path = ["begin"]

   4.2. Execute "begin"
       - Yield workflow_started event
       - Yield node_started event
       - invoke() → set prologue output
       - Yield node_finished event
       - Append downstream ["Categorize:IntentClassifier"] to path

   4.3. Execute "Categorize:IntentClassifier"
       - Yield node_started event
       - invoke():
         * Call LLM with query and categories
         * LLM returns "order_status"
         * Set output._next = ["Retrieval:OrderDB"]
       - Yield node_finished event
       - Append output._next to path

   4.4. Execute "Retrieval:OrderDB"
       - Yield node_started event
       - invoke():
         * Resolve query = sys.query
         * Search in knowledge base
         * Return chunks
         * Set output.formalized_content = formatted chunks
       - Yield node_finished event
       - Append downstream ["Agent:OrderSupport"] to path

   4.5. Execute "Agent:OrderSupport"
       - Yield node_started event
       - invoke():
         * Resolve prompts:
           - sys.query → "Where is my order?"
           - Retrieval:OrderDB@formalized_content → retrieved chunks
         * Start ReAct loop:
           Step 1: LLM analyze task
           Step 2: LLM decides no tools needed
           Step 3: LLM returns COMPLETE_TASK
         * Stream answer: "Your order #12345 is..."
       - Yield message events (streaming)
       - Yield node_finished event
       - Append downstream ["Message:FinalResponse"] to path

   4.6. Execute "Message:FinalResponse"
       - Yield node_started event
       - invoke():
         * Resolve content variables:
           - Agent:OrderSupport@content → "Your order #12345 is..."
           - Other agents → empty (not executed)
         * Return first non-empty content
       - Yield message event
       - Yield message_end event
       - Yield node_finished event
       - No downstream → workflow complete

   4.7. Workflow completion
       - Yield workflow_finished event
       - Save updated DSL to database (with history, path, etc.)

5. SSE stream to frontend
   Frontend displays:
   - Progress indicators
   - Streaming response
   - Citations (if any)

6. User sees response
   "Your order #12345 is currently in transit. Expected delivery: tomorrow."
```

---

## Phụ Lục: File References Chính

| Component | File Path | Line Numbers | Mục Đích |
|-----------|-----------|--------------|----------|
| **DSL Schema** | `ragflow/agent/canvas.py` | 36-73 | Định nghĩa cấu trúc DSL |
| **Graph Engine** | `ragflow/agent/canvas.py` | 34-273 | Core workflow execution |
| **Canvas Executor** | `ragflow/agent/canvas.py` | 275-676 | Agent workflow runner |
| **Component Base** | `ragflow/agent/component/base.py` | 37-583 | Base class cho component |
| **Agent Component** | `ragflow/agent/component/agent_with_tools.py` | 81-437 | LLM agent với ReAct |
| **Deep Researcher** | `ragflow/agentic_reasoning/deep_research.py` | 27-150 | Multi-step reasoning |
| **API Endpoint** | `ragflow/api/apps/canvas_app.py` | 124-178 | REST API |
| **Component Discovery** | `ragflow/agent/component/__init__.py` | 51-58 | Plugin system |
| **Templates** | `ragflow/agent/templates/` | - | Pre-built workflows |

---

**Tài liệu này mô tả chi tiết cách RAGFlow implement agent workflow system. Hệ thống cho phép người dùng định nghĩa complex multi-agent workflows thông qua JSON DSL, với execution engine mạnh mẽ hỗ trợ branching, looping, tool calling, và streaming responses.**
