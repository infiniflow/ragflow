# DRAFT Python API Reference

**THE API REFERENCES BELOW ARE STILL UNDER DEVELOPMENT.**

:::tip NOTE
Knowledgebase APIs
:::

## Create knowledge base

```python
RAGFlow.create_dataset(
    name: str,
    avatar: str = "",
    description: str = "",
    language: str = "English",
    permission: str = "me", 
    document_count: int = 0,
    chunk_count: int = 0,
    parse_method: str = "naive",
    parser_config: DataSet.ParserConfig = None
) -> DataSet
```

Creates a knowledge base (dataset).

### Parameters

#### name: `str`, *Required*

The unique name of the dataset to create. It must adhere to the following requirements:

- Permitted characters include:
  - English letters (a-z, A-Z)
  - Digits (0-9)
  - "_" (underscore)
- Must begin with an English letter or underscore.
- Maximum 65,535 characters.
- Case-insensitive.

#### avatar: `str`

Base64 encoding of the avatar. Defaults to `""`

#### description

#### tenant_id: `str` 

The id of the tenant associated with the created dataset is used to identify different users. Defaults to `None`.

- If creating a dataset, tenant_id must not be provided.
- If updating a dataset, tenant_id can't be changed.

#### description: `str`

The description of the created dataset. Defaults to `""`.

#### language: `str`

The language setting of the created dataset. Defaults to `"English"`. ????????????

#### permission

Specify who can operate on the dataset. Defaults to `"me"`.

#### document_count: `int`

The number of documents associated with the dataset. Defaults to `0`.

#### chunk_count: `int`

The number of data chunks generated or processed by the created dataset. Defaults to `0`.

#### parse_method, `str`

The method used by the dataset to parse and process data. Defaults to `"naive"`.

#### parser_config

The parser configuration of the dataset. A `ParserConfig` object contains the following attributes:

- `chunk_token_count`: Defaults to `128`.
- `layout_recognize`: Defaults to `True`.
- `delimiter`: Defaults to `'\n!?。；！？'`.
- `task_page_size`: Defaults to `12`.

### Returns

- Success: A `dataset` object.
- Failure: `Exception`

### Examples

```python
from ragflow import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag_object.create_dataset(name="kb_1")
```

---

## Delete knowledge bases

```python
RAGFlow.delete_datasets(ids: list[str] = None)
```

Deletes knowledge bases by name or ID.

### Parameters

#### ids

The IDs of the knowledge bases to delete.

### Returns

- Success: No value is returned.
- Failure: `Exception`

### Examples

```python
rag.delete_datasets(ids=["id_1","id_2"])
```

---

## List knowledge bases

```python
RAGFlow.list_datasets(
    page: int = 1, 
    page_size: int = 1024, 
    orderby: str = "create_time", 
    desc: bool = True,
    id: str = None,
    name: str = None
) -> list[DataSet]
```

Retrieves a list of knowledge bases.

### Parameters

#### page: `int`

The current page number to retrieve from the paginated results. Defaults to `1`.

#### page_size: `int`

The number of records on each page. Defaults to `1024`.

#### order_by: `str`

The field by which the records should be sorted. This specifies the attribute or column used to order the results. Defaults to `"create_time"`.

#### desc: `bool`

Whether the sorting should be in descending order. Defaults to `True`.

#### id: `str`

The id of the dataset to be got. Defaults to `None`.

#### name: `str`

The name of the dataset to be got. Defaults to `None`.

### Returns

- Success: A list of `DataSet` objects representing the retrieved knowledge bases.
- Failure: `Exception`.

### Examples

#### List all knowledge bases

```python
for ds in rag_object.list_datasets():
    print(ds)
```

#### Retrieve a knowledge base by ID

```python
dataset = rag_object.list_datasets(id = "id_1")
print(dataset[0])
```

---

## Update knowledge base

```python
DataSet.update(update_message: dict)
```

Updates the current knowledge base.

### Parameters

#### update_message: `dict[str, str|int]`, *Required*

- `"name"`: `str` The name of the knowledge base to update.
- `"tenant_id"`: `str` The `"tenant_id` you get after calling `create_dataset()`.
- `"embedding_model"`: `str` The embedding model for generating vector embeddings.
  - Ensure that `"chunk_count"` is `0` before updating `"embedding_model"`.
- `"parser_method"`: `str`
  - `"naive"`: General
  - `"manual`: Manual
  - `"qa"`: Q&A
  - `"table"`: Table
  - `"paper"`: Paper
  - `"book"`: Book
  - `"laws"`: Laws
  - `"presentation"`: Presentation
  - `"picture"`: Picture
  - `"one"`:One
  - `"knowledge_graph"`: Knowledge Graph
  - `"email"`: Email

### Returns

- Success: No value is returned.
- Failure: `Exception`

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.list_datasets(name="kb_1")
ds.update({"embedding_model":"BAAI/bge-zh-v1.5", "parse_method":"manual"})
```
---

:::tip API GROUPING
File management inside knowledge base
:::

## Upload document

```python
DataSet.upload_documents(document_list: list[dict])
```

### Parameters

#### document_list:`list[dict]`
A list composed of dicts containing `name` and `blob`.


### Returns
no return

### Examples
```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.create_dataset(name="kb_1")
ds.upload_documents([{name="1.txt", blob="123"}, ...] }
```
---

## Update document

```python
Document.update(update_message:dict)
```

### Parameters

#### update_message:`dict`  
only `name`,`parser_config`,`parser_method` can be changed

### Returns

no return

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds=rag.list_datasets(id='id')
ds=ds[0]
doc = ds.list_documents(id="wdfxb5t547d")
doc = doc[0]
doc.update([{"parser_method": "manual"...}])
```

---

## Download document

```python
Document.download() -> bytes
```

### Returns

bytes of the document.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds=rag.list_datasets(id="id")
ds=ds[0]
doc = ds.list_documents(id="wdfxb5t547d")
doc = doc[0]
open("~/ragflow.txt", "wb+").write(doc.download())
print(doc)
```

---

## List documents

```python
Dataset.list_documents(id:str =None, keywords: str=None, offset: int=0, limit:int = 1024,order_by:str = "create_time", desc: bool = True) -> list[Document]
```

### Parameters

#### id: `str`

The id of the document to be got

#### keywords: `str`

List documents whose name has the given keywords. Defaults to `None`.

#### offset: `int`

The beginning number of records for paging. Defaults to `0`.

#### limit: `int`

Records number to return, -1 means all of them. Records number to return, -1 means all of them.

#### orderby: `str`
The field by which the records should be sorted. This specifies the attribute or column used to order the results.

#### desc:`bool`
A boolean flag indicating whether the sorting should be in descending order.
### Returns

list[Document]  

A document object containing the following attributes:

#### id: `str`

Id of the retrieved document. Defaults to `""`.

#### thumbnail: `str`

Thumbnail image of the retrieved document. Defaults to `""`.

#### knowledgebase_id: `str`

Knowledge base ID related to the document. Defaults to `""`.

#### parser_method: `str`

Method used to parse the document. Defaults to `""`.

#### parser_config: `ParserConfig`

Configuration object for the parser. Defaults to `None`.

#### source_type: `str`

Source type of the document. Defaults to `""`.

#### type: `str`

Type or category of the document. Defaults to `""`.

#### created_by: `str`

Creator of the document. Defaults to `""`.

#### name: `str`
string
''
Name or title of the document. Defaults to `""`.

#### size: `int`

Size of the document in bytes or some other unit. Defaults to `0`.

#### token_count: `int`

Number of tokens in the document. Defaults to `""`.

#### chunk_count: `int`

Number of chunks the document is split into. Defaults to `0`.

#### progress: `float`

Current processing progress as a percentage. Defaults to `0.0`.

#### progress_msg: `str`

Message indicating current progress status. Defaults to `""`.

#### process_begin_at: `datetime`

Start time of the document processing. Defaults to `None`.

#### process_duation: `float`

Duration of the processing in seconds or minutes. Defaults to `0.0`.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.create_dataset(name="kb_1")

filename1 = "~/ragflow.txt"
blob=open(filename1 , "rb").read()
list_files=[{"name":filename1,"blob":blob}]
ds.upload_documents(list_files)
for d in ds.list_documents(keywords="rag", offset=0, limit=12):
    print(d)
```

---

## Delete documents

```python
DataSet.delete_documents(ids: list[str] = None)
```
### Returns

no return

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.list_datasets(name="kb_1")
ds = ds[0]
ds.delete_documents(ids=["id_1","id_2"])
```

---

## Parse and stop parsing document

```python
DataSet.async_parse_documents(document_ids:list[str]) -> None
DataSet.async_cancel_parse_documents(document_ids:list[str])-> None
```

### Parameters

#### document_ids:`list[str]`
The ids of the documents to be parsed
????????????????????????????????????????????????????

### Returns
no return
????????????????????????????????????????????????????

### Examples

```python
#documents parse and cancel
rag = RAGFlow(API_KEY, HOST_ADDRESS)
ds = rag.create_dataset(name="God5")
documents = [
    {'name': 'test1.txt', 'blob': open('./test_data/test1.txt',"rb").read()},
    {'name': 'test2.txt', 'blob': open('./test_data/test2.txt',"rb").read()},
    {'name': 'test3.txt', 'blob': open('./test_data/test3.txt',"rb").read()}
]
ds.upload_documents(documents)
documents=ds.list_documents(keywords="test")
ids=[]
for document in documents:
    ids.append(document.id)
ds.async_parse_documents(ids)
print("Async bulk parsing initiated")
ds.async_cancel_parse_documents(ids)
print("Async bulk parsing cancelled")
```

## List chunks
```python
Document.list_chunks(keywords: str = None, offset: int = 0, limit: int = -1, id : str = None) -> list[Chunk]
```
### Parameters

- `keywords`: `str`  
  List chunks whose name has the given keywords  
  default: `None`

- `offset`: `int`  
  The beginning number of records for paging  
  default: `1`

- `limit`: `int`  
  Records number to return  
  default: `30`

- `id`: `str`  
  The ID of the chunk to be retrieved  
  default: `None`
### Returns
list[chunk]

### Examples
```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.list_datasets("123")
ds = ds[0]
ds.async_parse_documents(["wdfxb5t547d"])
for c in doc.list_chunks(keywords="rag", offset=0, limit=12):
    print(c)
```
## Add chunk

```python
Document.add_chunk(content:str) -> Chunk
```

### Parameters

#### content: `str`, *Required*
Contains the main text or information of the chunk.
#### important_keywords :`list[str]`
list the key terms or phrases that are significant or central to the chunk's content.

### Returns

chunk

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.list_datasets(id="123")
ds = ds[0]
doc = ds.list_documents(id="wdfxb5t547d")
doc = doc[0]
chunk = doc.add_chunk(content="xxxxxxx")
```

---

## Delete chunk

```python
Document.delete_chunks(chunk_ids: list[str])
```
### Parameters
#### chunk_ids:`list[str]`
The list of chunk_id

### Returns

no return

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.list_datasets(id="123")
ds = ds[0]
doc = ds.list_documents(id="wdfxb5t547d")
doc = doc[0]
chunk = doc.add_chunk(content="xxxxxxx")
doc.delete_chunks(["id_1","id_2"])
```

---

## Update chunk

```python
Chunk.update(update_message: dict)
```
### Parameters
- `content`: `str`  
  Contains the main text or information of the chunk

- `important_keywords`: `list[str]`  
  List the key terms or phrases that are significant or central to the chunk's content

- `available`: `int`  
  Indicating the availability status, `0` means unavailable and `1` means available

### Returns

no return

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.list_datasets(id="123")
ds = ds[0]
doc = ds.list_documents(id="wdfxb5t547d")
doc = doc[0]
chunk = doc.add_chunk(content="xxxxxxx")
chunk.update({"content":"sdfx...})
```

---

## Retrieval

```python
RAGFlow.retrieve(question:str="", datasets:list[str]=None, document=list[str]=None, offset:int=1, limit:int=30, similarity_threshold:float=0.2, vector_similarity_weight:float=0.3, top_k:int=1024,rerank_id:str=None,keyword:bool=False,higlight:bool=False) -> list[Chunk]
```

### Parameters

#### question: `str`, *Required*

The user query or query keywords. Defaults to `""`.

#### datasets: `list[Dataset]`, *Required*

The scope of datasets.

#### document: `list[Document]`

The scope of document. `None` means no limitation. Defaults to `None`.

#### offset: `int`

The beginning point of retrieved records. Defaults to `0`.

#### limit: `int`

The maximum number of records needed to return. Defaults to `6`.

#### Similarity_threshold: `float`

The minimum similarity score. Defaults to `0.2`.

#### similarity_threshold_weight: `float`

The weight of vector cosine similarity, 1 - x is the term similarity weight. Defaults to `0.3`.

#### top_k: `int`

Number of records engaged in vector cosine computaton. Defaults to `1024`.

#### rerank_id:`str`
ID of the rerank model.  Defaults to `None`.

#### keyword:`bool`
Indicating whether keyword-based matching is enabled (True) or disabled (False).

#### highlight:`bool`

Specifying whether to enable highlighting of matched terms in the results (True) or not (False).
### Returns

list[Chunk]

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.list_datasets(name="ragflow")
ds = ds[0]
name = 'ragflow_test.txt'
path = './test_data/ragflow_test.txt'
rag.create_document(ds, name=name, blob=open(path, "rb").read())
doc = ds.list_documents(name=name)
doc = doc[0]
ds.async_parse_documents([doc.id])
for c in rag.retrieve(question="What's ragflow?", 
             datasets=[ds.id], documents=[doc.id], 
             offset=1, limit=30, similarity_threshold=0.2, 
             vector_similarity_weight=0.3,
             top_k=1024
             ):
    print(c)
```

---

:::tip API GROUPING
Chat APIs
:::

## Create chat assistant

```python
RAGFlow.create_chat(
    name: str = "assistant", 
    avatar: str = "path", 
    knowledgebases: list[DataSet] = [], 
    llm: Chat.LLM = None, 
    prompt: Chat.Prompt = None
) -> Chat
```

Creates a chat assistant.

### Returns

- Success: A `Chat` object representing the chat assistant.
- Failure: `Exception`

#### name: `str`

The name of the chat assistant. Defaults to `"assistant"`.

#### avatar: `str`

Base64 encoding of the avatar. Defaults to `""`.

#### knowledgebases: `list[str]`

The associated knowledge bases. Defaults to `["kb1"]`.

#### llm: `LLM`

The llm of the created chat. Defaults to `None`. When the value is `None`, a dictionary with the following values will be generated as the default.

- **model_name**, `str`  
  The chat model name. If it is `None`, the user's default chat model will be returned.  
- **temperature**, `float`  
  Controls the randomness of the model's predictions. A lower temperature increases the model's conficence in its responses; a higher temperature increases creativity and diversity. Defaults to `0.1`.  
- **top_p**, `float`  
  Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
- **presence_penalty**, `float`  
  This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.2`.
- **frequency penalty**, `float`  
  Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.
- **max_token**, `int`  
  This sets the maximum length of the model’s output, measured in the number of tokens (words or pieces of words). Defaults to `512`.

#### Prompt: `str`

Instructions for the LLM to follow.

- `"similarity_threshold"`: `float` A similarity score to evaluate distance between two lines of text. It's weighted keywords similarity and vector cosine similarity. If the similarity between query and chunk is less than this threshold, the chunk will be filtered out. Defaults to `0.2`.
- `"keywords_similarity_weight"`: `float` It's weighted keywords similarity and vector cosine similarity or rerank score (0~1). Defaults to `0.7`.
- `"top_n"`: `int` Not all the chunks whose similarity score is above the 'similarity threshold' will be feed to LLMs. LLM can only see these 'Top N' chunks. Defaults to `8`.
- `"variables"`: `list[dict[]]` If you use dialog APIs, the variables might help you chat with your clients with different strategies. The variables are used to fill in the 'System' part in prompt in order to give LLM a hint. The 'knowledge' is a very special variable which will be filled-in with the retrieved chunks. All the variables in 'System' should be curly bracketed. Defaults to `[{"key": "knowledge", "optional": True}]`
- `"rerank_model"`: `str` If it is not specified, vector cosine similarity will be used; otherwise, reranking score will be used. Defaults to `""`.
- `"empty_response"`: `str` If nothing is retrieved in the knowledge base for the user's question, this will be used as the response. To allow the LLM to improvise when nothing is retrieved, leave this blank. Defaults to `None`.
- `"opener"`: `str` The opening greeting for the user. Defaults to `"Hi! I am your assistant, can I help you?"`.
- `"show_quote`: `bool` Indicates whether the source of text should be displayed Defaults to `True`.
- `"prompt"`: `str` The prompt content. Defaults to `You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.`.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
knowledge_base = rag.list_datasets(name="kb_1")
assistant = rag.create_chat("Miss R", knowledgebases=knowledge_base)
```

---

## Update chat

```python
Chat.update(update_message: dict)
```

Updates the current chat assistant.

### Parameters

#### update_message: `dict[str, Any]`, *Required*

- `"name"`: `str` The name of the chat assistant to update.
- `"avatar"`: `str` Base64 encoding of the avatar. Defaults to `""`
- `"knowledgebases"`: `list[str]` Knowledge bases to update.
- `"llm"`: `dict` The LLM settings:
  - `"model_name"`, `str` The chat model name.
  - `"temperature"`, `float` Controls the randomness of the model's predictions.  
  - `"top_p"`, `float` Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from.  
  - `"presence_penalty"`, `float` This discourages the model from repeating the same information by penalizing words that have appeared in the conversation.
  - `"frequency penalty"`, `float` Similar to presence penalty, this reduces the model’s tendency to repeat the same words.
  - `"max_token"`, `int` This sets the maximum length of the model’s output, measured in the number of tokens (words or pieces of words).
- `"prompt"` : Instructions for the LLM to follow.
  - `"similarity_threshold"`: `float` A score to evaluate distance between two lines of text. It's weighted keywords similarity and vector cosine similarity. If the similarity between query and chunk is less than this threshold, the chunk will be filtered out. Defaults to `0.2`.
  - `"keywords_similarity_weight"`: `float` It's weighted keywords similarity and vector cosine similarity or rerank score (0~1). Defaults to `0.7`.
  - `"top_n"`: `int` Not all the chunks whose similarity score is above the 'similarity threshold' will be feed to LLMs. LLM can only see these 'Top N' chunks. Defaults to `8`.
  - `"variables"`: `list[dict[]]` If you use dialog APIs, the variables might help you chat with your clients with different strategies. The variables are used to fill in the 'System' part in prompt in order to give LLM a hint. The 'knowledge' is a very special variable which will be filled-in with the retrieved chunks. All the variables in 'System' should be curly bracketed. Defaults to `[{"key": "knowledge", "optional": True}]`
  - `"rerank_model"`: `str` If it is not specified, vector cosine similarity will be used; otherwise, reranking score will be used. Defaults to `""`.
  - `"empty_response"`: `str` If nothing is retrieved in the knowledge base for the user's question, this will be used as the response. To allow the LLM to improvise when nothing is retrieved, leave this blank. Defaults to `None`.
  - `"opener"`: `str` The opening greeting for the user. Defaults to `"Hi! I am your assistant, can I help you?"`.
  - `"show_quote`: `bool` Indicates whether the source of text should be displayed Defaults to `True`.
  - `"prompt"`: `str` The prompt content. Defaults to `You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.`.

### Returns

- Success: No value is returned.
- Failure: `Exception`

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
knowledge_base = rag.list_datasets(name="kb_1")
assistant = rag.create_chat("Miss R", knowledgebases=knowledge_base)
assistant.update({"name": "Stefan", "llm": {"temperature": 0.8}, "prompt": {"top_n": 8}})
```

---

## Delete chats

Deletes specified chat assistants.

```python
RAGFlow.delete_chats(ids: list[str] = None)
```

### Parameters

#### ids

IDs of the chat assistants to delete. If not specified, all chat assistants will be deleted.

### Returns

- Success: No value is returned.
- Failure: `Exception`

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag.delete_chats(ids=["id_1","id_2"])
```

---

## List chats

```python
RAGFlow.list_chats(
    page: int = 1, 
    page_size: int = 1024, 
    orderby: str = "create_time", 
    desc: bool = True,
    id: str = None,
    name: str = None
) -> list[Chat]
```

### Parameters

#### page

Specifies the page on which the records will be displayed. Defaults to `1`.

#### page_size

The number of records on each page. Defaults to `1024`.

#### order_by

The attribute by which the results are sorted. Defaults to `"create_time"`.

#### desc

Indicates whether to sort the results in descending order. Defaults to `True`.

#### id: `string`  

The ID of the chat to retrieve. Defaults to `None`.

#### name: `string`  

The name of the chat to retrieve. Defaults to `None`.

### Returns

- Success: A list of `Chat` objects.
- Failure: `Exception`.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
for assistant in rag.list_chats():
    print(assistant)
```

---

:::tip API GROUPING
Chat-session APIs
:::

## Create session

```python
Chat.create_session(name: str = "New session") -> Session
```

Creates a chat session.

### Parameters

#### name

The name of the chat session to create.

### Returns

- Success: A `Session` object containing the following attributes:
  - `id`: `str` The auto-generated unique identifier of the created session.
  - `name`: `str` The name of the created session.
  - `message`: `list[Message]` The messages of the created session assistant. Default: `[{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]`
  - `chat_id`: `str` The ID of the associated chat assistant.
- Failure: `Exception`

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag.list_chats(name="Miss R")
assistant = assistant[0]
session = assistant.create_session()
```

## Update session

```python
Session.update(update_message: dict)
```

Updates the current session.

### Parameters

#### update_message: `dict[str, Any]`, *Required*

- `"name"`: `str` The name of the session to update.

### Returns

- Success: No value is returned.
- Failure: `Exception`

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag.list_chats(name="Miss R")
assistant = assistant[0]
session = assistant.create_session("session_name")
session.update({"name": "updated_name"})
```

---

## Chat

```python
Session.ask(question: str, stream: bool = False) -> Optional[Message, iter[Message]]
```

### Parameters

#### question *Required*

The question to start an AI chat. Defaults to `None`.

#### stream

Indicates whether to output responses in a streaming way. Defaults to `False`.

### Returns

Optional[Message, iter[Message]]

- Message object, if `stream` is set to `False`
- iter[Message] object, if `stream` is set to `True`

#### id: `str`

The ID of the message. `id` is automatically generated. 

#### content: `str`

The content of the message. Defaults to `"Hi! I am your assistant, can I help you?"`.

#### reference: `list[Chunk]`

The auto-generated reference of the message. Each `chunk` object includes the following attributes:

- **id**: `str`  
  The id of the chunk. 
- **content**: `str`  
  The content of the chunk. 
- **document_id**: `str`  
  The ID of the document being referenced. 
- **document_name**: `str`  
  The name of the referenced document being referenced.
- **knowledgebase_id**: `str`  
  The id of the knowledge base to which the relevant document belongs.
- **image_id**: `str`  
  The id of the image related to the chunk.
- **similarity**: `float`
  A general similarity score, usually a composite score derived from various similarity measures . This score represents the degree of similarity between two objects. The value ranges between 0 and 1, where a value closer to 1 indicates higher similarity.
- **vector_similarity**: `float`  
  A similarity score based on vector representations. This score is obtained by converting texts, words, or objects into vectors and then calculating the cosine similarity or other distance measures between these vectors to determine the similarity in vector space. A higher value indicates greater similarity in the vector space. 
- **term_similarity**: `float`  
  The similarity score based on terms or keywords. This score is calculated by comparing the similarity of key terms between texts or datasets, typically measuring how similar two words or phrases are in meaning or context. A higher value indicates a stronger similarity between terms.  
- **position**: `list[string]`  
  Indicates the position or index of keywords or specific terms within the text. An array is typically used to mark the location of keywords or specific elements, facilitating precise operations or analysis of the text.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag.list_chats(name="Miss R")
assistant = assistant[0]
sess = assistant.create_session()    

print("\n==================== Miss R =====================\n")
print(assistant.get_prologue())

while True:
    question = input("\n==================== User =====================\n> ")
    print("\n==================== Miss R =====================\n")
    
    cont = ""
    for ans in sess.ask(question, stream=True):
        print(ans.content[len(cont):], end='', flush=True)
        cont = ans.content

```

---

## List sessions

```python
Chat.list_sessions(
    page: int = 1, 
    page_size: int = 1024, 
    orderby: str = "create_time", 
    desc: bool = True,
    id: str = None,
    name: str = None
) -> list[Session]
```

Lists sessions associated with the current chat assistant.

### Parameters

#### page

Specifies the page on which records will be displayed. Defaults to `1`.

#### page_size

The number of records on each page. Defaults to `1024`.

#### orderby

The field by which the records should be sorted. This specifies the attribute or column used to sort the results. Defaults to `"create_time"`.

#### desc

Whether the sorting should be in descending order. Defaults to `True`.

#### id

The ID of the chat session to retrieve. Defaults to `None`.

#### name

The name of the chat to retrieve. Defaults to `None`.

### Returns

- Success: A list of `Session` objects associated with the current chat assistant.
- Failure: `Exception`.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag.list_chats(name="Miss R")
assistant = assistant[0]
for session in assistant.list_sessions():
    print(session)
```

---

## Delete sessions

```python
Chat.delete_sessions(ids:list[str] = None)
```

Deletes specified sessions or all sessions associated with the current chat assistant.

### Parameters

#### ids

IDs of the sessions to delete. If not specified, all sessions associated with the current chat assistant will be deleted.

### Returns

- Success: No value is returned.
- Failure: `Exception`

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag.list_chats(name="Miss R")
assistant = assistant[0]
assistant.delete_sessions(ids=["id_1","id_2"])
```