# 04-AGENT-SYSTEM - Agentic Workflows

## Tong Quan

Agent System cung cấp visual canvas để xây dựng complex agentic workflows với multi-component orchestration.

## Kien Truc Agent System

```
┌─────────────────────────────────────────────────────────────────┐
│                     AGENT SYSTEM ARCHITECTURE                    │
└─────────────────────────────────────────────────────────────────┘

                    ┌─────────────────────────────┐
                    │      Canvas API             │
                    │   (canvas_app.py)           │
                    └──────────────┬──────────────┘
                                   │
                                   ▼
                    ┌─────────────────────────────┐
                    │     Canvas Engine           │
                    │   (agent/canvas.py)         │
                    │                             │
                    │  - DSL parsing              │
                    │  - State management         │
                    │  - Path-based execution     │
                    │  - Event streaming          │
                    └──────────────┬──────────────┘
                                   │
         ┌─────────────────────────┼─────────────────────────┐
         │                         │                         │
         ▼                         ▼                         ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Components    │     │     Tools       │     │  State/Memory   │
│                 │     │                 │     │                 │
│ - Begin         │     │ - Retrieval     │     │ - Globals       │
│ - LLM           │     │ - Web Search    │     │ - Outputs       │
│ - Agent         │     │ - SQL Exec      │     │ - History       │
│ - Categorize    │     │ - Code Exec     │     │ - Memory        │
│ - Switch        │     │ - Wikipedia     │     │                 │
│ - Iteration     │     │ - ArXiv         │     │                 │
│ - Message       │     │ - PubMed        │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## Cau Truc Thu Muc

```
/agent/
├── canvas.py           # Canvas execution engine ⭐
├── component/          # Component implementations
│   ├── base.py        # ComponentBase abstract class
│   ├── llm.py         # LLM component
│   ├── agent_with_tools.py  # Multi-tool Agent
│   ├── categorize.py  # Route by classification
│   ├── switch.py      # Conditional routing
│   ├── iteration.py   # Loop components
│   ├── message.py     # Output formatting
│   ├── begin.py       # Entry point
│   └── ...            # Other components
├── tools/              # Tool implementations
│   ├── base.py        # ToolBase class
│   ├── retrieval.py   # KB search tool
│   ├── google.py      # Google search
│   ├── exesql.py      # SQL execution
│   └── ...            # Other tools
└── templates/          # Pre-built workflows
```

## Files Trong Module Nay

| File | Mo Ta |
|------|-------|
| [canvas_execution_engine.md](./canvas_execution_engine.md) | Canvas execution và workflow orchestration |
| [component_architecture.md](./component_architecture.md) | Component base class và lifecycle |
| [component_llm_analysis.md](./component_llm_analysis.md) | LLM component deep analysis |
| [tool_integration.md](./tool_integration.md) | Tool framework và implementations |
| [variable_system.md](./variable_system.md) | Variable interpolation và state |
| [react_agent_pattern.md](./react_agent_pattern.md) | ReAct agent implementation |

## Component Types

### Core Components

| Component | Purpose | Key Parameters |
|-----------|---------|----------------|
| **Begin** | Entry point | prologue, mode |
| **LLM** | Language model call | llm_id, prompt, temperature |
| **Message** | Format output | content template |
| **Categorize** | Route by LLM classification | categories, examples |
| **Switch** | Conditional routing | conditions, operators |
| **Iteration** | Loop over array | items_ref |
| **Agent** | ReAct with tools | tools, max_rounds |

### Tool Components

| Tool | Purpose |
|------|---------|
| **Retrieval** | Knowledge base search |
| **Google** | Web search |
| **ExeSQL** | Database queries |
| **CodeExec** | Python/JS execution |
| **Wikipedia** | Wikipedia search |
| **ArXiv** | Academic papers |
| **PubMed** | Biomedical literature |
| **Tavily** | Structured web search |

## DSL Structure

```json
{
    "components": {
        "begin": {
            "obj": {
                "component_name": "Begin",
                "params": {"prologue": "Hello!"}
            },
            "downstream": ["LLM:Planning"],
            "upstream": []
        },
        "LLM:Planning": {
            "obj": {
                "component_name": "LLM",
                "params": {
                    "llm_id": "gpt-4@OpenAI",
                    "prompts": [{"role": "user", "content": "{{sys.query}}"}]
                }
            },
            "downstream": ["Message:Output"],
            "upstream": ["begin"]
        }
    },
    "globals": {
        "sys.query": "",
        "sys.user_id": "tenant_123"
    },
    "path": ["begin"],
    "history": [],
    "memory": []
}
```

## Variable Reference System

```python
# System variables
{{sys.query}}          # User input
{{sys.user_id}}        # Tenant ID
{{sys.files}}          # Uploaded files

# Component output references
{{component_id@output_name}}        # Direct output
{{LLM:Planning@content}}            # LLM response
{{retrieval_0@formalized_content}}  # Retrieval results

# Nested property access
{{agent_0@results.items[0]}}        # Array indexing
{{data@response.data.name}}         # Object property
```

## Execution Flow

```
1. Load DSL → Initialize Canvas
2. Reset all components
3. Set sys.query from user input
4. Execute path: [begin]
5. For each component in path:
   a. Resolve input variables
   b. Execute component._invoke()
   c. Check for errors → Exception routes
   d. Determine downstream components
   e. Extend path with downstream
6. Stream events to client
7. Save canvas state
```

## SSE Event Types

| Event | Description |
|-------|-------------|
| `workflow_started` | Workflow execution begins |
| `node_started` | Component execution begins |
| `message` | Streaming content (LLM output) |
| `node_finished` | Component execution complete |
| `user_inputs` | User input required |
| `workflow_finished` | Workflow complete |
| `error` | Error occurred |

## Related Files

- `/agent/canvas.py` - Canvas execution engine
- `/agent/component/base.py` - Component base classes
- `/api/apps/canvas_app.py` - Canvas API endpoints
- `/api/db/services/canvas_service.py` - Canvas storage
