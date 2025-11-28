# Dialog Service Analysis - Core RAG Implementation

## Tổng Quan

`dialog_service.py` (37KB) là service quan trọng nhất, implement toàn bộ **RAG pipeline** từ retrieval đến generation.

## File Location
```
/api/db/services/dialog_service.py
```

## Core Method: `chat()`

Đây là method chính xử lý RAG chat với streaming response.

### Complete Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        RAG CHAT PIPELINE                                 │
└─────────────────────────────────────────────────────────────────────────┘

INPUT: dialog, messages[], stream=True
                │
                ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [1] MODEL INITIALIZATION                                               │
│                                                                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │
│  │ Embedding   │  │   Chat      │  │  Reranker   │  │    TTS      │  │
│  │   Model     │  │   Model     │  │   Model     │  │   Model     │  │
│  │             │  │             │  │ (optional)  │  │ (optional)  │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘  │
│                                                                        │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [2] QUESTION PROCESSING                                                │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Multi-turn Refinement (if enabled)                              │  │
│  │   "What about Python?" → "What is Python programming language?" │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                              │                                         │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Cross-language Translation (if enabled)                         │  │
│  │   "Python是什么?" → ["What is Python?", "Python是什么?"]         │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                              │                                         │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Keyword Extraction (if enabled)                                 │  │
│  │   "What is machine learning?" → ["machine learning", "ML", "AI"]│  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [3] METADATA FILTERING (Optional)                                      │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Auto-generate filters from question via LLM                     │  │
│  │   "Q3 2024 revenue" → {"quarter": "Q3", "year": "2024"}        │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                              │                                         │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Apply filters to get document IDs                               │  │
│  │   doc_ids = filter_by_metadata(conditions)                      │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [4] RETRIEVAL PHASE                                                    │
│                                                                        │
│  Option A: Deep Research Mode (reasoning=True)                         │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ DeepResearcher.thinking()                                       │  │
│  │   • Multi-step reasoning                                        │  │
│  │   • Iterative retrieval                                         │  │
│  │   • Self-reflection                                             │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
│  Option B: Standard Retrieval                                          │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ retriever.retrieval(                                            │  │
│  │     question,                                                   │  │
│  │     embd_mdl,           # Embedding model                       │  │
│  │     tenant_ids,                                                 │  │
│  │     kb_ids,             # Knowledge base IDs                    │  │
│  │     page=1,                                                     │  │
│  │     page_size=top_n,    # Default: 6                           │  │
│  │     similarity_threshold=0.2,                                   │  │
│  │     vector_similarity_weight=0.3,                              │  │
│  │     rerank_mdl=rerank_mdl                                      │  │
│  │ )                                                               │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
│  Optional Enhancements:                                                │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ • TOC Enhancement: retrieval_by_toc()                          │  │
│  │ • Web Search: Tavily API integration                           │  │
│  │ • Knowledge Graph: kg_retriever.retrieval()                    │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [5] ANSWER GENERATION                                                  │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Build Prompt:                                                   │  │
│  │   system_prompt = prompt_config["system"].format(**kwargs)      │  │
│  │   + citation_prompt (if quote=True)                            │  │
│  │   + retrieved_context                                          │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                              │                                         │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Token Fitting:                                                  │  │
│  │   used_tokens, msg = message_fit_in(msg, max_tokens * 0.95)     │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                              │                                         │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Stream Generation:                                              │  │
│  │   for token in chat_mdl.chat_streamly(prompt, msg, gen_conf):   │  │
│  │       yield {"answer": accumulated, "reference": {}}            │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│ [6] CITATION PROCESSING                                                │
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Insert Citations:                                               │  │
│  │   answer, idx = retriever.insert_citations(                     │  │
│  │       answer,                                                   │  │
│  │       chunk_contents,                                          │  │
│  │       chunk_vectors,                                           │  │
│  │       embd_mdl,                                                │  │
│  │       tkweight=0.7,    # Token similarity weight               │  │
│  │       vtweight=0.3     # Vector similarity weight              │  │
│  │   )                                                            │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                              │                                         │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ Repair Bad Citations:                                           │  │
│  │   • Fix malformed citation formats                             │  │
│  │   • Merge duplicate citations                                  │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                        │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
OUTPUT: Generator[{
    "answer": "Response text with [1] citations...",
    "reference": {
        "chunks": [...],
        "doc_aggs": [...]
    },
    "audio_binary": bytes (if TTS enabled)
}]
```

### Code Implementation

```python
@classmethod
def chat(cls, dialog, messages, stream=True, **kwargs):
    """
    Main RAG chat pipeline.

    Args:
        dialog: Dialog configuration object
        messages: List of conversation messages
        stream: Enable streaming response
        **kwargs: Additional parameters

    Yields:
        Dict with answer, reference, and optional audio
    """

    # ========================================
    # [1] MODEL INITIALIZATION
    # ========================================

    # Get embedding model from knowledge bases
    e, kbs = KnowledgebaseService.get_by_ids(dialog.kb_ids)
    embd_mdl = LLMBundle(dialog.tenant_id, LLMType.EMBEDDING, kbs[0].embd_id)

    # Get chat model
    chat_mdl = LLMBundle(dialog.tenant_id, LLMType.CHAT, dialog.llm_id)

    # Get reranker (optional)
    rerank_mdl = None
    if dialog.rerank_id:
        rerank_mdl = LLMBundle(dialog.tenant_id, LLMType.RERANK, dialog.rerank_id)

    # Get TTS model (optional)
    tts_mdl = None
    if dialog.prompt_config.get("tts"):
        tts_mdl = LLMBundle(dialog.tenant_id, LLMType.TTS, dialog.tts_id)

    # ========================================
    # [2] QUESTION PROCESSING
    # ========================================

    # Extract user question
    question = messages[-1]["content"]
    questions = [question]

    # Multi-turn refinement
    if dialog.prompt_config.get("refine_multiturn") and len(messages) > 1:
        refined = refine_question(chat_mdl, messages)
        questions = [refined]

    # Cross-language translation
    if dialog.prompt_config.get("cross_languages"):
        translated = translate_question(
            chat_mdl,
            question,
            dialog.prompt_config["cross_languages"]
        )
        questions.extend(translated)

    # Keyword extraction
    if dialog.prompt_config.get("keyword"):
        keywords = extract_keywords(chat_mdl, question)
        questions.extend(keywords)

    # ========================================
    # [3] METADATA FILTERING
    # ========================================

    doc_ids = None

    if kwargs.get("doc_ids"):
        # Manual document filtering
        doc_ids = kwargs["doc_ids"]

    elif dialog.prompt_config.get("meta_data_filter"):
        # Auto-generate filters from question
        metas = DocumentService.get_meta_by_kbs(dialog.kb_ids)

        if dialog.prompt_config["meta_data_filter"]["method"] == "auto":
            # LLM generates filter conditions
            filters = gen_meta_filter(chat_mdl, metas, question)
            doc_ids = meta_filter(metas, filters["conditions"])
        else:
            # Manual filter conditions
            doc_ids = meta_filter(
                metas,
                dialog.prompt_config["meta_data_filter"]["conditions"]
            )

    # ========================================
    # [4] RETRIEVAL PHASE
    # ========================================

    if dialog.prompt_config.get("reasoning"):
        # Deep Research Mode
        reasoner = DeepResearcher(
            chat_mdl,
            dialog.prompt_config,
            lambda q: retriever.retrieval(q, embd_mdl, ...)
        )

        for think in reasoner.thinking(kbinfos, questions):
            yield {"answer": think["thought"], "reference": {}}

        kbinfos = reasoner.get_final_context()
    else:
        # Standard Retrieval
        kbinfos = retriever.retrieval(
            question=" ".join(questions),
            embd_mdl=embd_mdl,
            tenant_ids=[kb.tenant_id for kb in kbs],
            kb_ids=dialog.kb_ids,
            page=1,
            page_size=dialog.top_n,
            similarity_threshold=dialog.similarity_threshold,
            vector_similarity_weight=dialog.vector_similarity_weight,
            doc_ids=doc_ids,
            top=dialog.top_k,
            rerank_mdl=rerank_mdl
        )

    # Optional: TOC Enhancement
    if dialog.prompt_config.get("toc_enhance"):
        kbinfos["chunks"] = retriever.retrieval_by_toc(
            question,
            kbinfos["chunks"]
        )

    # Optional: Web Search (Tavily)
    if dialog.prompt_config.get("tavily_api_key"):
        web_results = tavily_search(
            question,
            dialog.prompt_config["tavily_api_key"]
        )
        kbinfos["chunks"].extend(web_results)

    # Optional: Knowledge Graph
    if dialog.prompt_config.get("use_kg"):
        kg_result = kg_retriever.retrieval(question, dialog.kb_ids)
        if kg_result:
            kbinfos["chunks"].insert(0, kg_result)

    # ========================================
    # [5] ANSWER GENERATION
    # ========================================

    # Build prompt
    prompt_config = dialog.prompt_config
    system_prompt = prompt_config["system"].format(**kwargs)

    # Add citation prompt if quotes enabled
    if prompt_config.get("quote") and kbinfos["chunks"]:
        system_prompt += citation_prompt(question)

    # Build context from retrieved chunks
    context = kb_prompt(kbinfos)

    # Build message history
    msg = [{"role": "system", "content": system_prompt + context}]
    msg.extend(messages)

    # Token fitting (use 95% of max tokens)
    max_tokens = chat_mdl.max_length
    used_tokens, msg = message_fit_in(msg, int(max_tokens * 0.95))

    # Generation config
    gen_conf = dialog.llm_setting.copy()
    gen_conf["max_tokens"] = min(
        gen_conf.get("max_tokens", 2048),
        max_tokens - used_tokens
    )

    # Stream generation
    answer = ""
    for chunk in chat_mdl.chat_streamly(system_prompt, msg[1:], gen_conf):
        answer = chunk
        yield {
            "answer": answer,
            "reference": {},
            "audio_binary": tts_mdl.tts(answer) if tts_mdl else None
        }

    # ========================================
    # [6] CITATION PROCESSING
    # ========================================

    if prompt_config.get("quote") and kbinfos["chunks"]:
        # Insert citations
        answer, idx = retriever.insert_citations(
            answer,
            [ck["content_ltks"] for ck in kbinfos["chunks"]],
            [ck["vector"] for ck in kbinfos["chunks"]],
            embd_mdl,
            tkweight=1 - dialog.vector_similarity_weight,
            vtweight=dialog.vector_similarity_weight
        )

        # Repair malformed citations
        answer, idx = repair_bad_citation_formats(answer, kbinfos, idx)

    # Final yield with references
    yield decorate_answer(answer, kbinfos, idx)
```

## Supporting Methods

### Question Refinement

```python
def refine_question(chat_mdl, messages):
    """
    Refine question for multi-turn context.

    Example:
        User: "What is Python?"
        Assistant: "Python is a programming language..."
        User: "What about its main features?"
        → Refined: "What are the main features of Python programming language?"
    """
    prompt = """Given the conversation history, rewrite the last question
    to be self-contained and clear.

    Conversation:
    {history}

    Last question: {question}

    Rewritten question:"""

    history = format_history(messages[:-1])
    question = messages[-1]["content"]

    return chat_mdl.chat(prompt.format(history=history, question=question))
```

### Cross-Language Translation

```python
def translate_question(chat_mdl, question, languages):
    """
    Translate question to multiple languages for broader retrieval.

    Args:
        question: Original question
        languages: List of target languages

    Returns:
        List of translated questions
    """
    translations = []
    for lang in languages:
        prompt = f"Translate to {lang}: {question}"
        translated = chat_mdl.chat(prompt)
        translations.append(translated)
    return translations
```

### Metadata Filtering

```python
def gen_meta_filter(chat_mdl, metas, question):
    """
    Generate metadata filters from question using LLM.

    Args:
        metas: Available metadata fields and values
        question: User question

    Returns:
        Filter conditions dict
    """
    prompt = f"""Given these metadata fields:
    {json.dumps(metas)}

    And this question: {question}

    Generate filter conditions as JSON:
    {{"conditions": [{{"field": "...", "operator": "==", "value": "..."}}]}}
    """

    response = chat_mdl.chat(prompt)
    return json.loads(response)
```

### Citation Processing

```python
def decorate_answer(answer, kbinfos, citation_indices):
    """
    Decorate final answer with references.

    Returns:
        {
            "answer": "Answer with [1] citations...",
            "reference": {
                "chunks": [
                    {
                        "chunk_id": "...",
                        "content": "...",
                        "doc_id": "...",
                        "docnm_kwd": "Document Name",
                        "positions": [[x0, x1, top, bottom]],
                        "similarity": 0.85
                    }
                ],
                "doc_aggs": [
                    {"doc_id": "...", "doc_name": "...", "count": 3}
                ]
            }
        }
    """
    # Filter chunks that are actually cited
    cited_chunks = [
        kbinfos["chunks"][i]
        for i in citation_indices
        if i < len(kbinfos["chunks"])
    ]

    return {
        "answer": answer,
        "reference": {
            "chunks": cited_chunks,
            "doc_aggs": kbinfos.get("doc_aggs", [])
        }
    }
```

## Configuration Options

### Dialog.prompt_config

```python
{
    # Basic settings
    "system": "You are a helpful assistant...",
    "prologue": "Hello! How can I help you?",
    "empty_response": "I couldn't find relevant information.",

    # Citation settings
    "quote": True,           # Enable citations [1], [2]

    # Retrieval enhancements
    "toc_enhance": False,    # Use table of contents
    "reasoning": False,      # Deep research mode
    "use_kg": False,         # Knowledge graph

    # Question processing
    "refine_multiturn": False,  # Multi-turn refinement
    "cross_languages": [],      # ["English", "Chinese"]
    "keyword": False,           # Extract keywords

    # External search
    "tavily_api_key": "",    # Tavily web search

    # Audio
    "tts": False,            # Text-to-speech

    # Metadata filtering
    "meta_data_filter": {
        "method": "auto",    # or "manual"
        "conditions": []
    }
}
```

### Dialog.llm_setting

```python
{
    "temperature": 0.7,
    "max_tokens": 2048,
    "top_p": 1.0,
    "frequency_penalty": 0.0,
    "presence_penalty": 0.0
}
```

## Performance Metrics

The `decorate_answer()` function tracks:

```python
{
    "total_time": 5.2,           # Total execution time
    "llm_init_time": 0.1,        # Model initialization
    "retrieval_time": 1.5,       # Search time
    "generation_time": 3.5,      # LLM generation
    "tokens_per_second": 45.2,   # Generation speed
    "input_tokens": 1500,        # Prompt tokens
    "output_tokens": 250         # Response tokens
}
```

## Related Methods

| Method | Purpose |
|--------|---------|
| `chat_solo()` | Chat without RAG (no retrieval) |
| `ask()` | Search-focused with summary |
| `gen_mindmap()` | Generate mind map from content |
| `use_sql()` | SQL-based structured retrieval |

## Related Files

- `/rag/nlp/search.py` - retriever implementation
- `/rag/llm/chat_model.py` - LLM interface
- `/rag/prompts/*.md` - Prompt templates
