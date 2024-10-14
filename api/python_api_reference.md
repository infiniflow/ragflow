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

#### name: *Required*

The unique name of the dataset to create. It must adhere to the following requirements:

- Permitted characters include:
  - English letters (a-z, A-Z)
  - Digits (0-9)
  - "_" (underscore)
- Must begin with an English letter or underscore.
- Maximum 65,535 characters.
- Case-insensitive.

#### avatar

Base64 encoding of the avatar. Defaults to `""`

#### description

The description of the created dataset. Defaults to `""`.

#### language

The language setting of the created dataset. Defaults to `"English"`.

#### permission

The person who can operate on the dataset. Defaults to `"me"`.

#### document_count

The number of documents associated with the dataset. Defaults to `0`.

#### chunk_count

The number of data chunks generated or processed by the created dataset. Defaults to `0`.

#### parse_method

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

#### page

The current page number to retrieve from the paginated results. Defaults to `1`.

#### page_size

The number of records on each page. Defaults to `1024`.

#### order_by

The attribute by which the results are sorted. Defaults to `"create_time"`.

#### desc

Indicates whether to sort the results in descending order. Defaults to `True`.

#### id

The ID of the dataset to retrieve. Defaults to `None`.

#### name

The name of the dataset to retrieve. Defaults to `None`.

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
RAGFLOW.upload_document(ds:DataSet, name:str, blob:bytes)-> bool
```

### Parameters

#### ds

#### name

#### blob



### Returns


### Examples

---

## Retrieve document

```python
RAGFlow.get_document(id:str=None,name:str=None) -> Document
```

### Parameters

#### id: `str`, *Required*

ID of the document to retrieve.

#### name: `str`

Name or title of the document.

### Returns

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
doc = rag.get_document(id="wdfxb5t547d",name='testdocument.txt')
print(doc)
```

---

## Save document settings

```python
Document.save() -> bool
```

### Returns

bool

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
doc = rag.get_document(id="wdfxb5t547d")
doc.parser_method= "manual"
doc.save()
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
doc = rag.get_document(id="wdfxb5t547d")
open("~/ragflow.txt", "w+").write(doc.download())
print(doc) 
```

---

## List documents

```python
Dataset.list_docs(keywords: str=None, offset: int=0, limit:int = -1) -> list[Document]
```

### Parameters

#### keywords: `str`

List documents whose name has the given keywords. Defaults to `None`.

#### offset: `int`

The beginning number of records for paging. Defaults to `0`.

#### limit: `int`

Records number to return, -1 means all of them.

### Returns

list[Document]

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.create_dataset(name="kb_1")

filename1 = "~/ragflow.txt"
rag.create_document(ds, name=filename1 , blob=open(filename1 , "rb").read())

filename2 = "~/infinity.txt"
rag.create_document(ds, name=filename2 , blob=open(filename2 , "rb").read())

for d in ds.list_docs(keywords="rag", offset=0, limit=12):
    print(d)
```

---

## Delete documents

```python
Document.delete() -> bool
```
### Returns

bool
description: delete success or not

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.create_dataset(name="kb_1")

filename1 = "~/ragflow.txt"
rag.create_document(ds, name=filename1 , blob=open(filename1 , "rb").read())

filename2 = "~/infinity.txt"
rag.create_document(ds, name=filename2 , blob=open(filename2 , "rb").read())
for d in ds.list_docs(keywords="rag", offset=0, limit=12):
    d.delete()
```

---

## Parse document

```python
Document.async_parse() -> None
RAGFLOW.async_parse_documents() -> None
```

### Parameters

????????????????????????????????????????????????????

### Returns

????????????????????????????????????????????????????

### Examples

```python
#document parse and cancel
rag = RAGFlow(API_KEY, HOST_ADDRESS)
ds = rag.create_dataset(name="dataset_name")
name3 = 'ai.pdf'
path = 'test_data/ai.pdf'
rag.create_document(ds, name=name3, blob=open(path, "rb").read())
doc = rag.get_document(name="ai.pdf")
doc.async_parse()
print("Async parsing initiated")
```

---

## Cancel document parsing

```python
rag.async_cancel_parse_documents(ids)
RAGFLOW.async_cancel_parse_documents()-> None
```

### Parameters

#### ids, `list[]`

### Returns

?????????????????????????????????????????????????

### Examples

```python
#documents parse and cancel
rag = RAGFlow(API_KEY, HOST_ADDRESS)
ds = rag.create_dataset(name="God5")
documents = [
    {'name': 'test1.txt', 'path': 'test_data/test1.txt'},
    {'name': 'test2.txt', 'path': 'test_data/test2.txt'},
    {'name': 'test3.txt', 'path': 'test_data/test3.txt'}
]

# Create documents in bulk
for doc_info in documents:
    with open(doc_info['path'], "rb") as file:
        created_doc = rag.create_document(ds, name=doc_info['name'], blob=file.read())
docs = [rag.get_document(name=doc_info['name']) for doc_info in documents]
ids = [doc.id for doc in docs]

rag.async_parse_documents(ids)
print("Async bulk parsing initiated")

for doc in docs:
    for progress, msg in doc.join(interval=5, timeout=10):
        print(f"{doc.name}: Progress: {progress}, Message: {msg}")

cancel_result = rag.async_cancel_parse_documents(ids)
print("Async bulk parsing cancelled")
```

---

## Join document

??????????????????

```python
Document.join(interval=15, timeout=3600) -> iteral[Tuple[float, str]]
```

### Parameters

#### interval: `int`

Time interval in seconds for progress report. Defaults to `15`.

#### timeout: `int`

Timeout in seconds. Defaults to `3600`.

### Returns

iteral[Tuple[float, str]]

## Add chunk

```python
Document.add_chunk(content:str) -> Chunk
```

### Parameters

#### content: `str`, *Required*

### Returns

chunk

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
doc = rag.get_document(id="wdfxb5t547d")
chunk = doc.add_chunk(content="xxxxxxx")
```

---

## Delete chunk

```python
Chunk.delete() -> bool
```

### Returns

bool

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
doc = rag.get_document(id="wdfxb5t547d")
chunk = doc.add_chunk(content="xxxxxxx")
chunk.delete()
```

---

## Save chunk contents

```python
Chunk.save() -> bool
```

### Returns

bool

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
doc = rag.get_document(id="wdfxb5t547d")
chunk = doc.add_chunk(content="xxxxxxx")
chunk.content = "sdfx"
chunk.save()
```

---

## Retrieval

```python
RAGFlow.retrieval(question:str, datasets:list[Dataset], document=list[Document]=None,     offset:int=0, limit:int=6, similarity_threshold:float=0.1, vector_similarity_weight:float=0.3, top_k:int=1024) -> list[Chunk]
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

### Returns

list[Chunk]

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
ds = rag.get_dataset(name="ragflow")
name = 'ragflow_test.txt'
path = 'test_data/ragflow_test.txt'
rag.create_document(ds, name=name, blob=open(path, "rb").read())
doc = rag.get_document(name=name)
doc.async_parse()
# Wait for parsing to complete 
for progress, msg in doc.join(interval=5, timeout=30):
    print(progress, msg)
for c in rag.retrieval(question="What's ragflow?", 
             datasets=[ds], documents=[doc], 
             offset=0, limit=6, similarity_threshold=0.1, 
             vector_similarity_weight=0.3,
             top_k=1024
             ):
    print(c)
```

---

:::tip API GROUPING
Chat APIs
:::

## Create chat

Creates a chat assistant.

```python
RAGFlow.create_chat(
    name: str = "assistant", 
    avatar: str = "path", 
    knowledgebases: list[DataSet] = ["kb1"], 
    llm: Chat.LLM = None, 
    prompt: Chat.Prompt = None
) -> Chat
```

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
  This parameter controls the randomness of predictions by the model. A lower temperature makes the model more confident in its responses, while a higher temperature makes it more creative and diverse. Defaults to `0.1`.  
- **top_p**, `float`  
  Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
- **presence_penalty**, `float`  
  This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.2`.
- **frequency penalty**, `float`  
  Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.
- **max_token**, `int`  
  This sets the maximum length of the model’s output, measured in the number of tokens (words or pieces of words). Defaults to `512`.

#### Prompt: `str`

Instructions for LLM's responses, including character design, answer length, and language. Defaults to:

```
You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.
```

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
kb = rag.list_datasets(name="kb_1")
assistant = rag.create_chat("Miss R", knowledgebases=kb)
```

---

## Update chat

Updates the current chat assistant.

```python
Chat.update(update_message: dict)
```

### Parameters

#### update_message: `dict[str, Any]`, *Required*

- `"name"`: `str` The name of the chat assistant to update.
- `"avatar"`: `str` Base64 encoding of the avatar. Defaults to `""`
- `"knowledgebases"`: `list[str]` Knowledge bases to update.


### Returns

- Success: No value is returned.
- Failure: `Exception`

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
kb = rag.list_datasets(name="kb_1")
assistant = rag.create_chat("Miss R"， knowledgebases=[kb])
assistant.update({llm: {"temperature":0.8}})
```

---

## Delete chats

Deletes specified chat assistants.

```python
RAGFlow.delete_chats(ids: list[str] = None)
```

### Parameters

#### ids

IDs of the chat assistants to delete.

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

The current page number to retrieve from the paginated results. Defaults to `1`.

#### page_size

The number of records on each page. Defaults to `1024`.

#### order_by

The attribute by which the results are sorted. Defaults to `"create_time"`.

#### desc

Indicates whether to sort the results in descending order. Defaults to `True`.

#### id: `string`  

The ID of the chat to be retrieved. Defaults to `None`.

#### name: `string`  

The name of the chat to be retrieved. Defaults to `None`.

### Returns

- Success: A list of `Chat` objects representing the retrieved knowledge bases.
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

### Returns

A `session` object.

#### id: `str`

The id of the created session is used to identify different sessions.
- id can not be provided in creating

#### name: `str`

The name of the created session. Defaults to `"New session"`.

#### messages: `list[Message]`

The messages of the created session.
- messages cannot be provided.

Defaults:

??????????????????????????????????????????????????????????????????????????????????????????????

```
[{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
```

#### chat_id: `str`

The id of associated chat
- `chat_id` can't be changed

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assi = rag.list_chats(name="Miss R")
assi = assi[0]
sess = assi.create_session()
```


## Update session

```python
Session.update(update_message:dict)
```

### Returns

no return

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assi = rag.list_chats(name="Miss R")
assi = assi[0]
sess = assi.create_session("new_session")
sess.update({"name": "Updated session"...})
```

---

## Chat

```python
Session.ask(question: str, stream: bool = False) -> Optional[Message, iter[Message]]
```

### Parameters

#### question: `str`, *Required*

The question to start an AI chat. Defaults to `None`. ???????????????????

#### stream: `bool`

The approach of streaming text generation. When stream is True, it outputs results in a streaming fashion; otherwise, it outputs the complete result after the model has finished generating.


### Returns

[Message, iter[Message]]

#### id: `str`

The id of the message. `id` is automatically generated. Defaults to `None`. ???????????????????

#### content: `str`

The content of the message. Defaults to `"Hi! I am your assistant, can I help you?"`.

#### reference: `list[Chunk]`

The auto-generated reference of the message. Each `chunk` object includes the following attributes:

- **id**: `str`  
  The id of the chunk. ?????????????????  
- **content**: `str`  
  The content of the chunk. Defaults to `None`. ?????????????????????  
- **document_id**: `str`  
  The ID of the document being referenced. Defaults to `""`.  
- **document_name**: `str`  
  The name of the referenced document being referenced. Defaults to `""`.  
- **knowledgebase_id**: `str`  
  The id of the knowledge base to which the relevant document belongs. Defaults to `""`.  
- **image_id**: `str`  
  The id of the image related to the chunk. Defaults to `""`.  
- **similarity**: `float`
  A general similarity score, usually a composite score derived from various similarity measures . This score represents the degree of similarity between two objects. The value ranges between 0 and 1, where a value closer to 1 indicates higher similarity. Defaults to `None`. ????????????????????????????????????   
- **vector_similarity**: `float`  
  A similarity score based on vector representations. This score is obtained by converting texts, words, or objects into vectors and then calculating the cosine similarity or other distance measures between these vectors to determine the similarity in vector space. A higher value indicates greater similarity in the vector space. Defaults to `None`. ?????????????????????????????????
- **term_similarity**: `float`  
  The similarity score based on terms or keywords. This score is calculated by comparing the similarity of key terms between texts or datasets, typically measuring how similar two words or phrases are in meaning or context. A higher value indicates a stronger similarity between terms. Defaults to `None`. ???????????????????  
- **position**: `list[string]`  
  Indicates the position or index of keywords or specific terms within the text. An array is typically used to mark the location of keywords or specific elements, facilitating precise operations or analysis of the text. Defaults to `None`. ??????????????

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assi = rag.list_chats(name="Miss R")
assi = assi[0]
sess = assi.create_session()    

print("\n==================== Miss R =====================\n")
print(assi.get_prologue())

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

### Returns

list[Session]
description: the List contains information about multiple assistant object, with each dictionary containing information about one assistant.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assi = rag.list_chats(name="Miss R")
assi = assi[0]
for sess in assi.list_sessions():
    print(sess)
```

### Parameters

#### page: `int`  

The current page number to retrieve from the paginated data. This parameter determines which set of records will be fetched.  
- `1`

#### page_size: `int`  

The number of records to retrieve per page. This controls how many records will be included in each page.  
- `1024`

#### orderby: `string`  

The field by which the records should be sorted. This specifies the attribute or column used to order the results.  
- `"create_time"`

#### desc: `bool`  

A boolean flag indicating whether the sorting should be in descending order.  
- `True`

#### id: `string`  

The ID of the chat to be retrieved.  
- `None`

#### name: `string`  

The name of the chat to be retrieved.  
- `None`
---

## Delete session

```python
Chat.delete_sessions(ids:list[str] = None)
```

### Returns

no return

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assi = rag.list_chats(name="Miss R")
assi = assi[0]
assi.delete_sessions(ids=["id_1","id_2"])
```
### Parameters
#### ids: `list[string]`
IDs of the sessions to be deleted.
- `None`

