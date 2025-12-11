# Tool Integration Framework

## Tong Quan

Tool framework cho phép Agent components gọi external services và APIs.

## File Locations
```
/agent/tools/
├── base.py           # ToolBase class
├── retrieval.py      # KB search
├── google.py         # Google search
├── tavily.py         # Tavily search
├── exesql.py         # SQL execution
├── code_executor.py  # Code execution
├── wikipedia.py      # Wikipedia
├── arxiv.py          # ArXiv papers
├── pubmed.py         # PubMed
└── ...               # Other tools
```

## Tool Base Class

```python
class ToolBase(ComponentBase):
    """Base class for all tools."""

    def invoke(self, **kwargs):
        """
        Execute tool with error handling.
        """
        self.set_output("_created_time", time.perf_counter())

        try:
            res = self._invoke(**kwargs)
        except Exception as e:
            self._param.outputs["_ERROR"] = {"value": str(e)}
            res = str(e)

        self.set_output(
            "_elapsed_time",
            time.perf_counter() - self.output("_created_time")
        )

        return res

    def _retrieve_chunks(self, res_list, get_title, get_url,
                         get_content, get_score=None):
        """
        Helper: normalize search results into RAG chunks.

        Args:
            res_list: Raw search results
            get_title: Function to extract title
            get_url: Function to extract URL
            get_content: Function to extract content
            get_score: Optional function to extract score
        """
        chunks = []
        aggs = []

        for r in res_list:
            content = get_content(r)
            id = str(hash_str2int(content))

            chunks.append({
                "chunk_id": id,
                "content": content[:10000],
                "doc_id": id,
                "docnm_kwd": get_title(r),
                "similarity": get_score(r) if get_score else 1,
                "url": get_url(r)
            })

        # Add to canvas references
        self._canvas.add_reference(chunks, aggs)
```

## Retrieval Tool

```python
class Retrieval(ToolBase, ABC):
    component_name = "Retrieval"

    def _invoke(self, **kwargs):
        """
        Search knowledge base for relevant chunks.
        """
        query = kwargs.get("query")

        if not query:
            self.set_output("formalized_content", self._param.empty_response)
            return

        # Support dynamic KB selection via variables
        kb_ids = []
        for id in self._param.kb_ids:
            if "@" in id:  # Variable reference
                kb_nm = self._canvas.get_variable_value(id)
                kb_ids.extend(kb_nm if isinstance(kb_nm, list) else [kb_nm])
            else:
                kb_ids.append(id)

        # Execute retrieval
        results = DocumentService.retrieval(
            kb_ids=kb_ids,
            query=query,
            top_k=self._param.top_k,
            top_n=self._param.top_n,
            similarity_threshold=self._param.similarity_threshold,
            use_kg=self._param.use_kg,
            rerank_id=self._param.rerank_id,
            cross_languages=self._param.cross_languages,
            meta_filter=self._param.meta_data_filter
        )

        # Format output
        self._retrieve_chunks(
            results,
            get_title=lambda r: r["doc_name"],
            get_url=lambda r: r.get("url", ""),
            get_content=lambda r: r["content"],
            get_score=lambda r: r["similarity"]
        )

        # Create formatted context
        self.set_output("formalized_content", kb_prompt(results))
```

## Google Search Tool

```python
class Google(ToolBase, ABC):
    component_name = "Google"

    def _invoke(self, **kwargs):
        """
        Execute Google Custom Search.
        """
        query = kwargs.get("query")

        # Google API call
        response = requests.get(
            "https://www.googleapis.com/customsearch/v1",
            params={
                "key": self._param.api_key,
                "cx": self._param.search_engine_id,
                "q": query,
                "num": self._param.num_results
            }
        )

        results = response.json().get("items", [])

        # Normalize to chunks
        self._retrieve_chunks(
            results,
            get_title=lambda r: r["title"],
            get_url=lambda r: r["link"],
            get_content=lambda r: r.get("snippet", ""),
            get_score=lambda r: 1.0
        )
```

## SQL Execution Tool

```python
class ExeSQL(ToolBase, ABC):
    component_name = "ExeSQL"

    def _invoke(self, **kwargs):
        """
        Execute SQL query against database.
        """
        sql = kwargs.get("sql")

        # Build connection
        conn = self._get_connection(
            db_type=self._param.db_type,
            host=self._param.host,
            port=self._param.port,
            database=self._param.database,
            username=self._param.username,
            password=self._param.password
        )

        # Execute with safety limits
        df = pd.read_sql(
            sql, conn,
            params={},
            chunksize=self._param.max_records
        )

        self.set_output("result", df.to_dict())

    def _get_connection(self, db_type, **kwargs):
        """Create database connection."""
        if db_type == "mysql":
            import pymysql
            return pymysql.connect(**kwargs)
        elif db_type == "postgresql":
            import psycopg2
            return psycopg2.connect(**kwargs)
        elif db_type == "sqlite":
            import sqlite3
            return sqlite3.connect(kwargs["database"])
```

## Code Execution Tool

```python
class CodeExec(ToolBase, ABC):
    component_name = "CodeExec"

    def _invoke(self, **kwargs):
        """
        Execute code in sandboxed environment.
        """
        code_b64 = kwargs.get("code_b64")
        code = base64.b64decode(code_b64).decode('utf-8')
        language = kwargs.get("language", "python")
        arguments = kwargs.get("arguments", {})

        if language == "python":
            result = self._execute_python(code, arguments)
        elif language == "nodejs":
            result = self._execute_nodejs(code, arguments)
        else:
            raise ValueError(f"Unsupported language: {language}")

        self.set_output("result", json.dumps(result))

    def _execute_python(self, code, arguments):
        """Execute Python code with restricted builtins."""
        restricted_builtins = {
            'print': print,
            'len': len,
            'range': range,
            'str': str,
            'int': int,
            'float': float,
            'list': list,
            'dict': dict,
            'json': json,
            # ... limited set
        }

        exec_globals = {"__builtins__": restricted_builtins}
        exec(code, exec_globals)

        return exec_globals["main"](arguments)
```

## Tavily Search Tool

```python
class Tavily(ToolBase, ABC):
    component_name = "Tavily"

    def _invoke(self, **kwargs):
        """
        Tavily structured web search.
        """
        query = kwargs.get("query")

        response = requests.post(
            "https://api.tavily.com/search",
            json={
                "api_key": self._param.api_key,
                "query": query,
                "search_depth": self._param.search_depth,
                "include_answer": True,
                "max_results": self._param.max_results
            }
        )

        data = response.json()

        # Get direct answer if available
        if data.get("answer"):
            self.set_output("answer", data["answer"])

        # Process results
        self._retrieve_chunks(
            data.get("results", []),
            get_title=lambda r: r["title"],
            get_url=lambda r: r["url"],
            get_content=lambda r: r["content"],
            get_score=lambda r: r.get("score", 1.0)
        )
```

## Academic Search Tools

### ArXiv
```python
class ArXiv(ToolBase, ABC):
    component_name = "ArXiv"

    def _invoke(self, **kwargs):
        """Search ArXiv for academic papers."""
        import arxiv

        query = kwargs.get("query")
        search = arxiv.Search(
            query=query,
            max_results=self._param.max_results,
            sort_by=arxiv.SortCriterion.Relevance
        )

        results = list(search.results())

        self._retrieve_chunks(
            results,
            get_title=lambda r: r.title,
            get_url=lambda r: r.pdf_url,
            get_content=lambda r: r.summary,
            get_score=lambda r: 1.0
        )
```

### PubMed
```python
class PubMed(ToolBase, ABC):
    component_name = "PubMed"

    def _invoke(self, **kwargs):
        """Search PubMed for biomedical literature."""
        from Bio import Entrez

        Entrez.email = self._param.email

        query = kwargs.get("query")

        # Search
        handle = Entrez.esearch(
            db="pubmed",
            term=query,
            retmax=self._param.max_results
        )
        record = Entrez.read(handle)
        ids = record["IdList"]

        # Fetch details
        handle = Entrez.efetch(
            db="pubmed",
            id=ids,
            rettype="abstract"
        )
        results = Entrez.read(handle)

        # Process results
        # ...
```

## Tool Meta for Agent

```python
def get_meta(self) -> dict:
    """
    Return tool metadata for function calling.
    """
    return {
        "function": {
            "name": self.component_name.lower(),
            "description": self._param.description,
            "parameters": {
                "type": "object",
                "properties": self._param.get_properties(),
                "required": self._param.get_required()
            }
        }
    }

# Example output:
{
    "function": {
        "name": "retrieval",
        "description": "Search knowledge base for relevant information",
        "parameters": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Search query"
                }
            },
            "required": ["query"]
        }
    }
}
```

## MCP Tool Integration

```python
# Model Context Protocol tools
class MCPToolCallSession:
    def __init__(self, mcp_server, variables):
        self.server = mcp_server
        self.variables = variables

    def tool_call(self, name: str, args: dict) -> str:
        """Execute MCP tool."""
        # Connect to MCP server
        response = self.server.call_tool(name, args)
        return response

# Usage in Agent:
for mcp_config in self._param.mcp:
    _, mcp_server = MCPServerService.get_by_id(mcp_config["mcp_id"])
    session = MCPToolCallSession(mcp_server, mcp_server.variables)

    for tool_name, meta in mcp_config["tools"].items():
        self.tools[tool_name] = session
```

## Tool Configuration

```python
# Retrieval parameters
{
    "kb_ids": ["kb_123", "{{sys.selected_kb}}"],  # Static + dynamic
    "top_k": 1024,
    "top_n": 6,
    "similarity_threshold": 0.2,
    "use_kg": False,
    "rerank_id": "jina-reranker-v2",
}

# Google parameters
{
    "api_key": "...",
    "search_engine_id": "...",
    "num_results": 10
}

# SQL parameters
{
    "db_type": "mysql",
    "host": "localhost",
    "port": 3306,
    "database": "mydb",
    "username": "user",
    "password": "***",
    "max_records": 1000
}
```

## Related Files

- `/agent/tools/base.py` - ToolBase class
- `/agent/tools/retrieval.py` - KB retrieval
- `/agent/tools/*.py` - Individual tool implementations
- `/agent/component/agent_with_tools.py` - Tool-enabled agent
