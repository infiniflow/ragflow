# DRAFT Python API Reference

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

The url or ???????????????????????? path to the avatar image associated with the created dataset. Defaults to `""`

#### tenant_id: `str` ?????????????????

The id of the tenant associated with the created dataset is used to identify different users. Defaults to `None`.

- If creating a dataset, tenant_id must not be provided.
- If updating a dataset, tenant_id can't be changed.

#### description: `str`

The description of the created dataset. Defaults to `""`.

#### language: `str`

The language setting of the created dataset. Defaults to `"English"`. ????????????

#### embedding_model: `str`       ????????????????

The specific model or algorithm used by the dataset to generate vector embeddings. Defaults to `""`.

- If creating a dataset, embedding_model must not be provided.
- If updating a dataset, embedding_model can't be changed.

#### permission: `str`

Specify who can operate on the dataset. Defaults to `"me"`.

#### document_count: `int`

The number of documents associated with the dataset. Defaults to `0`.

- If updating a dataset, `document_count` can't be changed.

#### chunk_count: `int`

The number of data chunks generated or processed by the created dataset. Defaults to `0`.

- If updating a dataset, chunk_count can't be changed.

#### parse_method, `str`

The method used by the dataset to parse and process data.

- If updating parse_method in a dataset, chunk_count must be greater than 0. Defaults to `"naive"`.

#### parser_config, `Dataset.ParserConfig`

The configuration settings for the parser used by the dataset.

### Returns

- Success: An `infinity.local_infinity.table.LocalTable` object in Python module mode or an `infinity.remote_thrift.table.RemoteTable` object in client-server mode.
- Failure: `InfinityException`
  - `error_code`: `int` A non-zero value indicating a specific error condition.
  - `error_msg`: `str` A message providing additional details about the error.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
ds = rag.create_dataset(name="kb_1")
```

---

## Delete knowledge base

```python
DataSet.delete() -> bool
```

Deletes a knowledge base. 

### Returns

`bool`

description:the case of updating an dateset, `True` or `False`.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
ds = rag.create_dataset(name="kb_1")
ds.delete()
```

---

## List knowledge bases

```python
RAGFlow.list_datasets(
    page: int = 1, 
    page_size: int = 1024, 
    orderby: str = "create_time", 
    desc: bool = True
) -> List[DataSet]
```

Lists all knowledge bases in the RAGFlow system. 

### Parameters

#### page: `int`

The current page number to retrieve from the paginated data. This parameter determines which set of records will be fetched. Defaults to `1`.

#### page_size: `int`

The number of records to retrieve per page. This controls how many records will be included in each page. Defaults to `1024`.

#### order_by: `str`

The field by which the records should be sorted. This specifies the attribute or column used to order the results. Defaults to `"create_time"`.

#### desc: `bool`

Whether the sorting should be in descending order. Defaults to `True`.

### Returns

```python
List[DataSet]
description:the list of datasets.
```

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
for ds in rag.list_datasets():
    print(ds)
```

---

## Retrieve knowledge base

```python
RAGFlow.get_dataset(
    id: str = None, 
    name: str = None
) -> DataSet
```

Retrieves a knowledge base by name.

### Parameters

#### name: `str`

The name of the dataset to be got. If `id` is not provided,  `name` is required.

#### id: `str`

The id of the dataset to be got. If `name` is not provided,  `id` is required.

### Returns

```python
DataSet
description: dataset object
```

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
ds = rag.get_dataset(name="ragflow")
print(ds)
```

---

## Save knowledge base configurations

```python
DataSet.save() -> bool
```

### Returns

```python
bool
description:the case of updating an dateset, True or False.
```

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
ds = rag.get_dataset(name="kb_1")
ds.parse_method = "manual"
ds.save()
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

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
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

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
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

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
doc = rag.get_document(id="wdfxb5t547d")
open("~/ragflow.txt", "w+").write(doc.download())
print(doc) 
```

---

## List documents

```python
Dataset.list_docs(keywords: str=None, offset: int=0, limit:int = -1) -> List[Document]
```

### Parameters

#### keywords: `str`

List documents whose name has the given keywords. Defaults to `None`.

#### offset: `int`

The beginning number of records for paging. Defaults to `0`.

#### limit: `int`

Records number to return, -1 means all of them. Records number to return, -1 means all of them.

### Returns

List[Document]

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
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

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
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

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
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

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
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

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
doc = rag.get_document(id="wdfxb5t547d")
chunk = doc.add_chunk(content="xxxxxxx")
chunk.content = "sdfx"
chunk.save()
```

---

## Retrieval

```python
RAGFlow.retrieval(question:str, datasets:List[Dataset], document=List[Document]=None,     offset:int=0, limit:int=6, similarity_threshold:float=0.1, vector_similarity_weight:float=0.3, top_k:int=1024) -> List[Chunk]
```

### Parameters

#### question: `str`, *Required*

The user query or query keywords. Defaults to `""`.

#### datasets: `List[Dataset]`, *Required*

The scope of datasets.

#### document: `List[Document]`

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

List[Chunk]

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
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
Chat assistant APIs
:::

## Create assistant

```python
RAGFlow.create_assistant(
    name: str = "assistant", 
    avatar: str = "path", 
    knowledgebases: List[DataSet] = ["kb1"], 
    llm: Assistant.LLM = None, 
    prompt: Assistant.Prompt = None
) -> Assistant
```

### Returns

Assistant object.

#### name: `str`

The name of the created assistant. Defaults to `"assistant"`.

#### avatar: `str`

The icon of the created assistant. Defaults to `"path"`. 

#### knowledgebases: `List[DataSet]`

Select knowledgebases associated. Defaults to `["kb1"]`.

#### id: `str`

The id of the created assistant. Defaults to `""`.

#### llm: `LLM`

The llm of the created assistant. Defaults to `None`. When the value is `None`, a dictionary with the following values will be generated as the default.

- **model_name**, `str`  
  Large language chat model. If it is `None`, it will return the user's default model.  
- **temperature**, `float`  
  This parameter controls the randomness of predictions by the model. A lower temperature makes the model more confident in its responses, while a higher temperature makes it more creative and diverse. Defaults to `0.1`.  
- **top_p**, `float`  
  Also known as “nucleus sampling,” this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
- **presence_penalty**, `float`  
  This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.2`.
- **frequency penalty**, `float`  
  Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.
- **max_token**, `int`  
  This sets the maximum length of the model’s output, measured in the number of tokens (words or pieces of words). Defaults to `512`.

#### Prompt: `str`

Instructions you need LLM to follow when LLM answers questions, like character design, answer length and answer language etc. 

Defaults:
```
You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.
```

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
kb = rag.get_dataset(name="kb_1")
assi = rag.create_assistant("Miss R", knowledgebases=[kb])
```

---

## Save updates to a chat assistant

```python
Assistant.save() -> bool
```

### Returns

```python
bool
description:the case of updating an assistant, True or False.
```

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
kb = rag.get_knowledgebase(name="kb_1")
assi = rag.create_assistant("Miss R"， knowledgebases=[kb])
assi.llm.temperature = 0.8
assi.save()
```

---

## Delete assistant

```python
Assistant.delete() -> bool
```

### Returns

```python
bool
description:the case of deleting an assistant, True or False.
```

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
kb = rag.get_knowledgebase(name="kb_1")
assi = rag.create_assistant("Miss R"， knowledgebases=[kb])
assi.delete()
```

---

## Retrieve assistant

```python
RAGFlow.get_assistant(id: str = None, name: str = None) -> Assistant
```

### Parameters

#### id: `str`

ID of the assistant to retrieve. If `name` is not provided,  `id` is required.

#### name: `str`

Name of the assistant to retrieve. If `id` is not provided,  `name` is required.

### Returns

Assistant object.

#### name: `str`

The name of the created assistant. Defaults to `"assistant"`.

#### avatar: `str`

The icon of the created assistant. Defaults to `"path"`. 

#### knowledgebases: `List[DataSet]`

Select knowledgebases associated. Defaults to `["kb1"]`.

#### id: `str`

The id of the created assistant. Defaults to `""`.

#### llm: `LLM`

The llm of the created assistant. Defaults to `None`. When the value is `None`, a dictionary with the following values will be generated as the default.

- **model_name**, `str`  
  Large language chat model. If it is `None`, it will return the user's default model.  
- **temperature**, `float`  
  This parameter controls the randomness of predictions by the model. A lower temperature makes the model more confident in its responses, while a higher temperature makes it more creative and diverse. Defaults to `0.1`.  
- **top_p**, `float`  
  Also known as “nucleus sampling,” this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
- **presence_penalty**, `float`  
  This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.2`.
- **frequency penalty**, `float`  
  Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.
- **max_token**, `int`  
  This sets the maximum length of the model’s output, measured in the number of tokens (words or pieces of words). Defaults to `512`.

#### Prompt: `str`

Instructions you need LLM to follow when LLM answers questions, like character design, answer length and answer language etc. 

Defaults:
```
You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.
```

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
assi = rag.get_assistant(name="Miss R")
```

---

## List assistants

```python
RAGFlow.list_assistants() -> List[Assistant]
```

### Returns

A list of assistant objects.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
for assi in rag.list_assistants():
    print(assi)
```

---

:::tip API GROUPING
Chat-session APIs
:::

## Create session

```python
assistant_1.create_session(name: str = "New session") -> Session
```

### Returns

A `session` object.

#### id: `str`

The id of the created session is used to identify different sessions.
- `id` cannot be provided in creating
- `id` is required in updating

#### name: `str`

The name of the created session. Defaults to `"New session"`.

#### messages: `List[Message]`

The messages of the created session.
- messages cannot be provided.

Defaults:

??????????????????????????????????????????????????????????????????????????????????????????????

```
[{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
```

#### assistant_id: `str`

The id of associated assistant. Defaults to `""`.
- `assistant_id` is required in creating if you use HTTP API.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
assi = rag.get_assistant(name="Miss R")
sess = assi.create_session()
```

## Retrieve session

```python
Assistant.get_session(id: str) -> Session
```

### Parameters

#### id: `str`, *Required*

???????????????????????????????

### Returns

### Returns

A `session` object.

#### id: `str`

The id of the created session is used to identify different sessions.
- `id` cannot be provided in creating
- `id` is required in updating

#### name: `str`

The name of the created session. Defaults to `"New session"`.

#### messages: `List[Message]`

The messages of the created session.
- messages cannot be provided.

Defaults:

??????????????????????????????????????????????????????????????????????????????????????????????

```
[{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
```

#### assistant_id: `str`


???????????????????????????????????????How to get

The id of associated assistant. Defaults to `""`.
- `assistant_id` is required in creating if you use HTTP API.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
assi = rag.get_assistant(name="Miss R")
sess = assi.get_session(id="d5c55d2270dd11ef9bd90242ac120007")
```

---

## Save session settings

```python
Session.save() -> bool
```

### Returns

bool
description:the case of updating a session, True or False.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
assi = rag.get_assistant(name="Miss R")
sess = assi.get_session(id="d5c55d2270dd11ef9bd90242ac120007")
sess.name = "Updated session"
sess.save()
```

---

## Chat

```python
Session.chat(question: str, stream: bool = False) -> Optional[Message, iter[Message]]
```

### Parameters

#### question: `str`, *Required*

The question to start an AI chat. Defaults to `None`. ???????????????????

#### stream: `bool`

The approach of streaming text generation. When stream is True, it outputs results in a streaming fashion; otherwise, it outputs the complete result after the model has finished generating.

#### session_id: `str` ??????????????????

### Returns

[Message, iter[Message]]

#### id: `str`

The id of the message. `id` is automatically generated. Defaults to `None`. ???????????????????

#### content: `str`

The content of the message. Defaults to `"Hi! I am your assistant, can I help you?"`.

#### reference: `List[Chunk]`

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
- **position**: `List[string]`  
  Indicates the position or index of keywords or specific terms within the text. An array is typically used to mark the location of keywords or specific elements, facilitating precise operations or analysis of the text. Defaults to `None`. ??????????????

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
assi = rag.get_assistant(name="Miss R")
sess = assi.create_session()    

print("\n==================== Miss R =====================\n")
print(assi.get_prologue())

while True:
    question = input("\n==================== User =====================\n> ")
    print("\n==================== Miss R =====================\n")
    
    cont = ""
    for ans in sess.chat(question, stream=True):
        print(ans.content[len(cont):], end='', flush=True)
        cont = ans.content
```

---

## List sessions

```python
Assistant.list_session() -> List[Session]
```

### Returns

List[Session]
description: the List contains information about multiple assistant object, with each dictionary containing information about one assistant.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
assi = rag.get_assistant(name="Miss R")

for sess in assi.list_session():
    print(sess)
```

---

## Delete session

```python
Session.delete() -> bool
```

### Returns

bool
description:the case of deleting a session, True or False.

### Examples

```python
from ragflow import RAGFlow

rag = RAGFlow(api_key="xxxxxx", base_url="http://xxx.xx.xx.xxx:9380")
assi = rag.get_assistant(name="Miss R")
sess = assi.create_session()
sess.delete()
```