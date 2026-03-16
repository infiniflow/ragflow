---
sidebar_position: 5
slug: /python_api_reference
sidebar_custom_props: {
  categoryIcon: SiPython
}
---
# Python API

A complete reference for RAGFlow's Python APIs. Before proceeding, please ensure you [have your RAGFlow API key ready for authentication](https://ragflow.io/docs/dev/acquire_ragflow_api_key).

:::tip NOTE
Run the following command to download the Python SDK:

```bash
pip install ragflow-sdk
```

:::

---

## ERROR CODES

---

| Code | Message               | Description                |
|------|-----------------------|----------------------------|
| 400  | Bad Request           | Invalid request parameters |
| 401  | Unauthorized          | Unauthorized access        |
| 403  | Forbidden             | Access denied              |
| 404  | Not Found             | Resource not found         |
| 500  | Internal Server Error | Server internal error      |
| 1001 | Invalid Chunk ID      | Invalid Chunk ID           |
| 1002 | Chunk Update Failed   | Chunk update failed        |

---

## OpenAI-Compatible API

---

### Create chat completion

Creates a model response for the given historical chat conversation via OpenAI's API.

#### Parameters

##### model: `str`, *Required*

The model used to generate the response. The server will parse this automatically, so you can set it to any value for now.

##### messages: `list[object]`, *Required*

A list of historical chat messages used to generate the response. This must contain at least one message with the `user` role.

##### stream: `boolean`

Whether to receive the response as a stream. Set this to `false` explicitly if you prefer to receive the entire response in one go instead of as a stream.

#### Returns

- Success: Response [message](https://platform.openai.com/docs/api-reference/chat/create) like OpenAI
- Failure: `Exception`

#### Examples

> **Note**
> Streaming via `client.chat.completions.create(stream=True, ...)` does not
> return `reference` currently because `reference` is only exposed in the
> non-stream response payload. The only way to return `reference` is non-stream
> mode with `with_raw_response`.
:::caution NOTE
Streaming via `client.chat.completions.create(stream=True, ...)` does not return `reference` because it is *only* included in the raw response payload in non-stream mode. To return `reference`, set `stream=False`.
:::
```python
from openai import OpenAI
import json

model = "model"
client = OpenAI(api_key="ragflow-api-key", base_url=f"http://ragflow_address/api/v1/chats_openai/<chat_id>")

stream = True
reference = True

request_kwargs = dict(
    model=model,
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Who are you?"},
        {"role": "assistant", "content": "I am an AI assistant named..."},
        {"role": "user", "content": "Can you tell me how to install neovim"},
    ],
    extra_body={
        "extra_body": {
            "reference": reference,
            "reference_metadata": {
                "include": True,
                "fields": ["author", "year", "source"],
            },
        }
    },
)

if stream:
    completion = client.chat.completions.create(stream=True, **request_kwargs)
    for chunk in completion:
        print(chunk)
else:
    resp = client.chat.completions.with_raw_response.create(
        stream=False, **request_kwargs
    )
    print("status:", resp.http_response.status_code)
    raw_text = resp.http_response.text
    print("raw:", raw_text)

    data = json.loads(raw_text)
    print("assistant:", data["choices"][0]["message"].get("content"))
    print("reference:", data["choices"][0]["message"].get("reference"))
```

When `extra_body.reference_metadata.include` is `true`, each reference chunk may include a `document_metadata` object in both streaming and non-streaming responses.

## DATASET MANAGEMENT

---

### Create dataset

```python
RAGFlow.create_dataset(
    name: str,
    avatar: Optional[str] = None,
    description: Optional[str] = None,
    embedding_model: Optional[str] = "BAAI/bge-large-zh-v1.5@BAAI",
    permission: str = "me", 
    chunk_method: str = "naive",
    parser_config: DataSet.ParserConfig = None
) -> DataSet
```

Creates a dataset.

#### Parameters

##### name: `str`, *Required*

The unique name of the dataset to create. It must adhere to the following requirements:

- Maximum 128 characters.
- Case-insensitive.

##### avatar: `str`

Base64 encoding of the avatar. Defaults to `None`

##### description: `str`

A brief description of the dataset to create. Defaults to `None`.


##### permission

Specifies who can access the dataset to create. Available options:  

- `"me"`: (Default) Only you can manage the dataset.
- `"team"`: All team members can manage the dataset.

##### chunk_method, `str`

The chunking method of the dataset to create. Available options:

- `"naive"`: General (default)
- `"manual`: Manual
- `"qa"`: Q&A
- `"table"`: Table
- `"paper"`: Paper
- `"book"`: Book
- `"laws"`: Laws
- `"presentation"`: Presentation
- `"picture"`: Picture
- `"one"`: One
- `"email"`: Email

##### parser_config

The parser configuration of the dataset. A `ParserConfig` object's attributes vary based on the selected `chunk_method`:

- `chunk_method`=`"naive"`:  
  `{"chunk_token_num":512,"delimiter":"\\n","html4excel":False,"layout_recognize":True,"raptor":{"use_raptor":False}}`.
- `chunk_method`=`"qa"`:  
  `{"raptor": {"use_raptor": False}}`
- `chunk_method`=`"manuel"`:  
  `{"raptor": {"use_raptor": False}}`
- `chunk_method`=`"table"`:  
  `None`
- `chunk_method`=`"paper"`:  
  `{"raptor": {"use_raptor": False}}`
- `chunk_method`=`"book"`:  
  `{"raptor": {"use_raptor": False}}`
- `chunk_method`=`"laws"`:  
  `{"raptor": {"use_raptor": False}}`
- `chunk_method`=`"picture"`:  
  `None`
- `chunk_method`=`"presentation"`:  
  `{"raptor": {"use_raptor": False}}`
- `chunk_method`=`"one"`:  
  `None`
- `chunk_method`=`"knowledge-graph"`:  
  `{"chunk_token_num":128,"delimiter":"\\n","entity_types":["organization","person","location","event","time"]}`
- `chunk_method`=`"email"`:  
  `None`

#### Returns

- Success: A `dataset` object.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.create_dataset(name="kb_1")
```

---

### Delete datasets

```python
RAGFlow.delete_datasets(ids: list[str] | None = None)
```

Deletes datasets by ID.

#### Parameters

##### ids: `list[str]` or `None`, *Required*

The IDs of the datasets to delete. Defaults to `None`.
  - If `None`, all datasets will be deleted.
  - If an array of IDs, only the specified datasets will be deleted.
  - If an empty array, no datasets will be deleted.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
rag_object.delete_datasets(ids=["d94a8dc02c9711f0930f7fbc369eab6d","e94a8dc02c9711f0930f7fbc369eab6e"])
```

---

### List datasets

```python
RAGFlow.list_datasets(
    page: int = 1, 
    page_size: int = 30, 
    orderby: str = "create_time", 
    desc: bool = True,
    id: str = None,
    name: str = None
) -> list[DataSet]
```

Lists datasets.

#### Parameters

##### page: `int`

Specifies the page on which the datasets will be displayed. Defaults to `1`.

##### page_size: `int`

The number of datasets on each page. Defaults to `30`.

##### orderby: `str`

The field by which datasets should be sorted. Available options:

- `"create_time"` (default)
- `"update_time"`

##### desc: `bool`

Indicates whether the retrieved datasets should be sorted in descending order. Defaults to `True`.

##### id: `str`

The ID of the dataset to retrieve. Defaults to `None`.

##### name: `str`

The name of the dataset to retrieve. Defaults to `None`.

#### Returns

- Success: A list of `DataSet` objects.
- Failure: `Exception`.

#### Examples

##### List all datasets

```python
for dataset in rag_object.list_datasets():
    print(dataset)
```

##### Retrieve a dataset by ID

```python
dataset = rag_object.list_datasets(id = "id_1")
print(dataset[0])
```

---

### Update dataset

```python
DataSet.update(update_message: dict)
```

Updates configurations for the current dataset.

#### Parameters

##### update_message: `dict[str, str|int]`, *Required*

A dictionary representing the attributes to update, with the following keys:

- `"name"`: `str` The revised name of the dataset.
  - Basic Multilingual Plane (BMP) only
  - Maximum 128 characters
  - Case-insensitive
- `"avatar"`: (*Body parameter*), `string`  
  The updated base64 encoding of the avatar.
  - Maximum 65535 characters
- `"embedding_model"`: (*Body parameter*), `string`  
  The updated embedding model name.  
  - Ensure that `"chunk_count"` is `0` before updating `"embedding_model"`.
  - Maximum 255 characters
  - Must follow `model_name@model_factory` format
- `"permission"`: (*Body parameter*), `string`  
  The updated dataset permission. Available options:  
  - `"me"`: (Default) Only you can manage the dataset.
  - `"team"`: All team members can manage the dataset.
- `"pagerank"`: (*Body parameter*), `int`  
  refer to [Set page rank](https://ragflow.io/docs/dev/set_page_rank)
  - Default: `0`
  - Minimum: `0`
  - Maximum: `100`
- `"chunk_method"`: (*Body parameter*), `enum<string>`  
  The chunking method for the dataset. Available options:  
  - `"naive"`: General (default)
  - `"book"`: Book
  - `"email"`: Email
  - `"laws"`: Laws
  - `"manual"`: Manual
  - `"one"`: One
  - `"paper"`: Paper
  - `"picture"`: Picture
  - `"presentation"`: Presentation
  - `"qa"`: Q&A
  - `"table"`: Table
  - `"tag"`: Tag

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.list_datasets(name="kb_name")
dataset = dataset[0]
dataset.update({"embedding_model":"BAAI/bge-zh-v1.5", "chunk_method":"manual"})
```

---

## FILE MANAGEMENT WITHIN DATASET

---

### Upload documents

```python
DataSet.upload_documents(document_list: list[dict])
```

Uploads documents to the current dataset.

#### Parameters

##### document_list: `list[dict]`, *Required*

A list of dictionaries representing the documents to upload, each containing the following keys:

- `"display_name"`: (Optional) The file name to display in the dataset.  
- `"blob"`: (Optional) The binary content of the file to upload.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
dataset = rag_object.create_dataset(name="kb_name")
dataset.upload_documents([{"display_name": "1.txt", "blob": "<BINARY_CONTENT_OF_THE_DOC>"}, {"display_name": "2.pdf", "blob": "<BINARY_CONTENT_OF_THE_DOC>"}])
```

---

### Update document

```python
Document.update(update_message:dict)
```

Updates configurations for the current document.

#### Parameters

##### update_message: `dict[str, str|dict[]]`, *Required*

A dictionary representing the attributes to update, with the following keys:

- `"display_name"`: `str` The name of the document to update.
- `"meta_fields"`: `dict[str, Any]` The meta fields of the document.
- `"chunk_method"`: `str` The parsing method to apply to the document.
  - `"naive"`: General
  - `"manual`: Manual
  - `"qa"`: Q&A
  - `"table"`: Table
  - `"paper"`: Paper
  - `"book"`: Book
  - `"laws"`: Laws
  - `"presentation"`: Presentation
  - `"picture"`: Picture
  - `"one"`: One
  - `"email"`: Email
- `"parser_config"`: `dict[str, Any]` The parsing configuration for the document. Its attributes vary based on the selected `"chunk_method"`:
  - `"chunk_method"`=`"naive"`:  
    `{"chunk_token_num":128,"delimiter":"\\n","html4excel":False,"layout_recognize":True,"raptor":{"use_raptor":False}}`.
  - `chunk_method`=`"qa"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"manuel"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"table"`:  
    `None`
  - `chunk_method`=`"paper"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"book"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"laws"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"presentation"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"picture"`:  
    `None`
  - `chunk_method`=`"one"`:  
    `None`
  - `chunk_method`=`"knowledge-graph"`:  
    `{"chunk_token_num":128,"delimiter":"\\n","entity_types":["organization","person","location","event","time"]}`
  - `chunk_method`=`"email"`:  
    `None`

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.list_datasets(id='id')
dataset = dataset[0]
doc = dataset.list_documents(id="wdfxb5t547d")
doc = doc[0]
doc.update([{"parser_config": {"chunk_token_num": 256}}, {"chunk_method": "manual"}])
```

---

### Download document

```python
Document.download() -> bytes
```

Downloads the current document.

#### Returns

The downloaded document in bytes.

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.list_datasets(id="id")
dataset = dataset[0]
doc = dataset.list_documents(id="wdfxb5t547d")
doc = doc[0]
open("~/ragflow.txt", "wb+").write(doc.download())
print(doc)
```

---

### List documents

```python
Dataset.list_documents(
    id: str = None,
    keywords: str = None,
    page: int = 1,
    page_size: int = 30,
    order_by: str = "create_time",
    desc: bool = True,
    create_time_from: int = 0,
    create_time_to: int = 0
) -> list[Document]
```

Lists documents in the current dataset.

#### Parameters

##### id: `str`

The ID of the document to retrieve. Defaults to `None`.

##### keywords: `str`

The keywords used to match document titles. Defaults to `None`.

##### page: `int`

Specifies the page on which the documents will be displayed. Defaults to `1`.

##### page_size: `int`

The maximum number of documents on each page. Defaults to `30`.

##### orderby: `str`

The field by which documents should be sorted. Available options:

- `"create_time"` (default)
- `"update_time"`

##### desc: `bool`

Indicates whether the retrieved documents should be sorted in descending order. Defaults to `True`.

##### create_time_from: `int`
Unix timestamp for filtering documents created after this time. 0 means no filter. Defaults to 0.

##### create_time_to: `int`
Unix timestamp for filtering documents created before this time. 0 means no filter. Defaults to 0.

#### Returns

- Success: A list of `Document` objects.
- Failure: `Exception`.

A `Document` object contains the following attributes:

- `id`: The document ID. Defaults to `""`.
- `name`: The document name. Defaults to `""`.
- `thumbnail`: The thumbnail image of the document. Defaults to `None`.
- `dataset_id`: The dataset ID associated with the document. Defaults to `None`.
- `chunk_method` The chunking method name. Defaults to `"naive"`.
- `source_type`: The source type of the document. Defaults to `"local"`.
- `type`: Type or category of the document. Defaults to `""`. Reserved for future use.
- `created_by`: `str` The creator of the document. Defaults to `""`.
- `size`: `int` The document size in bytes. Defaults to `0`.
- `token_count`: `int` The number of tokens in the document. Defaults to `0`.
- `chunk_count`: `int` The number of chunks in the document. Defaults to `0`.
- `progress`: `float` The current processing progress as a percentage. Defaults to `0.0`.
- `progress_msg`: `str` A message indicating the current progress status. Defaults to `""`.
- `process_begin_at`: `datetime` The start time of document processing. Defaults to `None`.
- `process_duration`: `float` Duration of the processing in seconds. Defaults to `0.0`.
- `run`: `str` The document's processing status:
  - `"UNSTART"`  (default)
  - `"RUNNING"`
  - `"CANCEL"`
  - `"DONE"`
  - `"FAIL"`
- `status`: `str` Reserved for future use.
- `parser_config`: `ParserConfig` Configuration object for the parser. Its attributes vary based on the selected `chunk_method`:
  - `chunk_method`=`"naive"`:  
    `{"chunk_token_num":128,"delimiter":"\\n","html4excel":False,"layout_recognize":True,"raptor":{"use_raptor":False}}`.
  - `chunk_method`=`"qa"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"manuel"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"table"`:  
    `None`
  - `chunk_method`=`"paper"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"book"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"laws"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"presentation"`:  
    `{"raptor": {"use_raptor": False}}`
  - `chunk_method`=`"picure"`:  
    `None`
  - `chunk_method`=`"one"`:  
    `None`
  - `chunk_method`=`"email"`:  
    `None`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.create_dataset(name="kb_1")

filename1 = "~/ragflow.txt"
blob = open(filename1 , "rb").read()
dataset.upload_documents([{"name":filename1,"blob":blob}])
for doc in dataset.list_documents(keywords="rag", page=0, page_size=12):
    print(doc)
```

---

### Delete documents

```python
DataSet.delete_documents(ids: list[str] = None)
```

Deletes documents by ID.

#### Parameters

##### ids: `list[list]`

The IDs of the documents to delete. Defaults to `None`. If it is not specified, all documents in the dataset will be deleted.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.list_datasets(name="kb_1")
dataset = dataset[0]
dataset.delete_documents(ids=["id_1","id_2"])
```

---

### Parse documents

```python
DataSet.async_parse_documents(document_ids:list[str]) -> None
```

Parses documents in the current dataset.

#### Parameters

##### document_ids: `list[str]`, *Required*

The IDs of the documents to parse.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.create_dataset(name="dataset_name")
documents = [
    {'display_name': 'test1.txt', 'blob': open('./test_data/test1.txt',"rb").read()},
    {'display_name': 'test2.txt', 'blob': open('./test_data/test2.txt',"rb").read()},
    {'display_name': 'test3.txt', 'blob': open('./test_data/test3.txt',"rb").read()}
]
dataset.upload_documents(documents)
documents = dataset.list_documents(keywords="test")
ids = []
for document in documents:
    ids.append(document.id)
dataset.async_parse_documents(ids)
print("Async bulk parsing initiated.")
```

---

### Parse documents (with document status)

```python
DataSet.parse_documents(document_ids: list[str]) -> list[tuple[str, str, int, int]]
```

*Asynchronously* parses documents in the current dataset.

This method encapsulates `async_parse_documents()`. It awaits the completion of all parsing tasks before returning detailed results, including the parsing status and statistics for each document. If a keyboard interruption occurs (e.g., `Ctrl+C`), all pending parsing tasks will be cancelled gracefully.

#### Parameters

##### document_ids: `list[str]`, *Required*

The IDs of the documents to parse.

#### Returns

A list of tuples with detailed parsing results:

```python
[
  (document_id: str, status: str, chunk_count: int, token_count: int),
  ...
]
```
- `status`: The final parsing state (e.g., `success`, `failed`, `cancelled`).  
- `chunk_count`: The number of content chunks created from the document.  
- `token_count`: The total number of tokens processed.  

---

#### Example

```python
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.create_dataset(name="dataset_name")
documents = dataset.list_documents(keywords="test")
ids = [doc.id for doc in documents]

try:
    finished = dataset.parse_documents(ids)
    for doc_id, status, chunk_count, token_count in finished:
        print(f"Document {doc_id} parsing finished with status: {status}, chunks: {chunk_count}, tokens: {token_count}")
except KeyboardInterrupt:
    print("\nParsing interrupted by user. All pending tasks have been cancelled.")
except Exception as e:
    print(f"Parsing failed: {e}")
```

---

### Stop parsing documents

```python
DataSet.async_cancel_parse_documents(document_ids:list[str])-> None
```

Stops parsing specified documents.

#### Parameters

##### document_ids: `list[str]`, *Required*

The IDs of the documents for which parsing should be stopped.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.create_dataset(name="dataset_name")
documents = [
    {'display_name': 'test1.txt', 'blob': open('./test_data/test1.txt',"rb").read()},
    {'display_name': 'test2.txt', 'blob': open('./test_data/test2.txt',"rb").read()},
    {'display_name': 'test3.txt', 'blob': open('./test_data/test3.txt',"rb").read()}
]
dataset.upload_documents(documents)
documents = dataset.list_documents(keywords="test")
ids = []
for document in documents:
    ids.append(document.id)
dataset.async_parse_documents(ids)
print("Async bulk parsing initiated.")
dataset.async_cancel_parse_documents(ids)
print("Async bulk parsing cancelled.")
```

---

## CHUNK MANAGEMENT WITHIN DATASET

---

### Add chunk

```python
Document.add_chunk(content:str, important_keywords:list[str] = []) -> Chunk
```

Adds a chunk to the current document.

#### Parameters

##### content: `str`, *Required*

The text content of the chunk.

##### important_keywords: `list[str]`

The key terms or phrases to tag with the chunk.

#### Returns

- Success: A `Chunk` object.
- Failure: `Exception`.

A `Chunk` object contains the following attributes:

- `id`: `str`: The chunk ID.
- `content`: `str` The text content of the chunk.
- `important_keywords`: `list[str]` A list of key terms or phrases tagged with the chunk.
- `create_time`: `str` The time when the chunk was created (added to the document).
- `create_timestamp`: `float` The timestamp representing the creation time of the chunk, expressed in seconds since January 1, 1970.
- `dataset_id`: `str` The ID of the associated dataset.
- `document_name`: `str` The name of the associated document.
- `document_id`: `str` The ID of the associated document.
- `available`: `bool` The chunk's availability status in the dataset. Value options:
  - `False`: Unavailable
  - `True`: Available (default)

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
datasets = rag_object.list_datasets(id="123")
dataset = datasets[0]
doc = dataset.list_documents(id="wdfxb5t547d")
doc = doc[0]
chunk = doc.add_chunk(content="xxxxxxx")
```

---

### List chunks

```python
Document.list_chunks(keywords: str = None, page: int = 1, page_size: int = 30, id : str = None) -> list[Chunk]
```

Lists chunks in the current document.

#### Parameters

##### keywords: `str`

The keywords used to match chunk content. Defaults to `None`

##### page: `int`

Specifies the page on which the chunks will be displayed. Defaults to `1`.

##### page_size: `int`

The maximum number of chunks on each page. Defaults to `30`.

##### id: `str`

The ID of the chunk to retrieve. Default: `None`

#### Returns

- Success: A list of `Chunk` objects.
- Failure: `Exception`.

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.list_datasets("123")
dataset = dataset[0]
docs = dataset.list_documents(keywords="test", page=1, page_size=12)
for chunk in docs[0].list_chunks(keywords="rag", page=0, page_size=12):
    print(chunk)
```

---

### Delete chunks

```python
Document.delete_chunks(chunk_ids: list[str])
```

Deletes chunks by ID.

#### Parameters

##### chunk_ids: `list[str]`

The IDs of the chunks to delete. Defaults to `None`. If it is not specified, all chunks of the current document will be deleted.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.list_datasets(id="123")
dataset = dataset[0]
doc = dataset.list_documents(id="wdfxb5t547d")
doc = doc[0]
chunk = doc.add_chunk(content="xxxxxxx")
doc.delete_chunks(["id_1","id_2"])
```

---

### Update chunk

```python
Chunk.update(update_message: dict)
```

Updates content or configurations for the current chunk.

#### Parameters

##### update_message: `dict[str, str|list[str]|int]` *Required*

A dictionary representing the attributes to update, with the following keys:

- `"content"`: `str` The text content of the chunk.
- `"important_keywords"`: `list[str]` A list of key terms or phrases to tag with the chunk.
- `"available"`: `bool` The chunk's availability status in the dataset. Value options:
  - `False`: Unavailable
  - `True`: Available (default)

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.list_datasets(id="123")
dataset = dataset[0]
doc = dataset.list_documents(id="wdfxb5t547d")
doc = doc[0]
chunk = doc.add_chunk(content="xxxxxxx")
chunk.update({"content":"sdfx..."})
```

---

### Retrieve chunks

```python
RAGFlow.retrieve(question:str="", dataset_ids:list[str]=None, document_ids=list[str]=None, page:int=1, page_size:int=30, similarity_threshold:float=0.2, vector_similarity_weight:float=0.3, top_k:int=1024,rerank_id:str=None,keyword:bool=False,cross_languages:list[str]=None,metadata_condition: dict=None) -> list[Chunk]
```

Retrieves chunks from specified datasets.

#### Parameters

##### question: `str`, *Required*

The user query or query keywords. Defaults to `""`.

##### dataset_ids: `list[str]`, *Required*

The IDs of the datasets to search. Defaults to `None`. 

##### document_ids: `list[str]`

The IDs of the documents to search. Defaults to `None`. You must ensure all selected documents use the same embedding model. Otherwise, an error will occur. 

##### page: `int`

The starting index for the documents to retrieve. Defaults to `1`.

##### page_size: `int`

The maximum number of chunks to retrieve. Defaults to `30`.

##### Similarity_threshold: `float`

The minimum similarity score. Defaults to `0.2`.

##### vector_similarity_weight: `float`

The weight of vector cosine similarity. Defaults to `0.3`. If x represents the vector cosine similarity, then (1 - x) is the term similarity weight.

##### top_k: `int`

The number of chunks engaged in vector cosine computation. Defaults to `1024`.

##### rerank_id: `str`

The ID of the rerank model. Defaults to `None`.

##### keyword: `bool`

Indicates whether to enable keyword-based matching:

- `True`: Enable keyword-based matching.
- `False`: Disable keyword-based matching (default).

##### cross_languages:  `list[string]`  

The languages that should be translated into, in order to achieve keywords retrievals in different languages.

##### metadata_condition: `dict`

filter condition for `meta_fields`.

#### Returns

- Success: A list of `Chunk` objects representing the document chunks.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
dataset = rag_object.list_datasets(name="ragflow")
dataset = dataset[0]
name = 'ragflow_test.txt'
path = './test_data/ragflow_test.txt'
documents =[{"display_name":"test_retrieve_chunks.txt","blob":open(path, "rb").read()}]
docs = dataset.upload_documents(documents)
doc = docs[0]
doc.add_chunk(content="This is a chunk addition test")
for c in rag_object.retrieve(dataset_ids=[dataset.id],document_ids=[doc.id]):
  print(c)
```

---

## CHAT ASSISTANT MANAGEMENT

---

### Create chat assistant

```python
RAGFlow.create_chat(
    name: str, 
    avatar: str = "", 
    dataset_ids: list[str] = [], 
    llm: Chat.LLM = None, 
    prompt: Chat.Prompt = None
) -> Chat
```

Creates a chat assistant.

#### Parameters

##### name: `str`, *Required*

The name of the chat assistant.

##### avatar: `str`

Base64 encoding of the avatar. Defaults to `""`.

##### dataset_ids: `list[str]`

The IDs of the associated datasets. Defaults to `[""]`.

##### llm: `Chat.LLM`

The LLM settings for the chat assistant to create. Defaults to `None`. When the value is `None`, a dictionary with the following values will be generated as the default. An `LLM` object contains the following attributes:

- `model_name`: `str`  
  The chat model name. If it is `None`, the user's default chat model will be used.  
- `temperature`: `float`  
  Controls the randomness of the model's predictions. A lower temperature results in more conservative responses, while a higher temperature yields more creative and diverse responses. Defaults to `0.1`.  
- `top_p`: `float`  
  Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
- `presence_penalty`: `float`  
  This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.2`.
- `frequency penalty`: `float`  
  Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.

##### prompt: `Chat.Prompt`

Instructions for the LLM to follow.  A `Prompt` object contains the following attributes:

- `similarity_threshold`: `float` RAGFlow employs either a combination of weighted keyword similarity and weighted vector cosine similarity, or a combination of weighted keyword similarity and weighted reranking score during retrieval. If a similarity score falls below this threshold, the corresponding chunk will be excluded from the results. The default value is `0.2`.
- `keywords_similarity_weight`: `float` This argument sets the weight of keyword similarity in the hybrid similarity score with vector cosine similarity or reranking model similarity. By adjusting this weight, you can control the influence of keyword similarity in relation to other similarity measures. The default value is `0.7`.
- `top_n`: `int` This argument specifies the number of top chunks with similarity scores above the `similarity_threshold` that are fed to the LLM. The LLM will *only* access these 'top N' chunks.  The default value is `8`.
- `variables`: `list[dict[]]` This argument lists the variables to use in the 'System' field of **Chat Configurations**. Note that:
  - `knowledge` is a reserved variable, which represents the retrieved chunks.
  - All the variables in 'System' should be curly bracketed.
  - The default value is `[{"key": "knowledge", "optional": True}]`.
- `rerank_model`: `str` If it is not specified, vector cosine similarity will be used; otherwise, reranking score will be used. Defaults to `""`.
- `top_k`: `int` Refers to the process of reordering or selecting the top-k items from a list or set based on a specific ranking criterion. Default to 1024.
- `empty_response`: `str` If nothing is retrieved in the dataset for the user's question, this will be used as the response. To allow the LLM to improvise when nothing is found, leave this blank. Defaults to `None`.
- `opener`: `str` The opening greeting for the user. Defaults to `"Hi! I am your assistant, can I help you?"`.
- `show_quote`: `bool` Indicates whether the source of text should be displayed. Defaults to `True`.
- `prompt`: `str` The prompt content.

#### Returns

- Success: A `Chat` object representing the chat assistant.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
datasets = rag_object.list_datasets(name="kb_1")
dataset_ids = []
for dataset in datasets:
    dataset_ids.append(dataset.id)
assistant = rag_object.create_chat("Miss R", dataset_ids=dataset_ids)
```

---

### Update chat assistant

```python
Chat.update(update_message: dict)
```

Updates configurations for the current chat assistant.

#### Parameters

##### update_message: `dict[str, str|list[str]|dict[]]`, *Required*

A dictionary representing the attributes to update, with the following keys:

- `"name"`: `str` The revised name of the chat assistant.
- `"avatar"`: `str` Base64 encoding of the avatar. Defaults to `""`
- `"dataset_ids"`: `list[str]` The datasets to update.
- `"llm"`: `dict` The LLM settings:
  - `"model_name"`, `str` The chat model name.
  - `"temperature"`, `float` Controls the randomness of the model's predictions. A lower temperature results in more conservative responses, while a higher temperature yields more creative and diverse responses.  
  - `"top_p"`, `float` Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from.  
  - `"presence_penalty"`, `float` This discourages the model from repeating the same information by penalizing words that have appeared in the conversation.
  - `"frequency penalty"`, `float` Similar to presence penalty, this reduces the model’s tendency to repeat the same words.
- `"prompt"` : Instructions for the LLM to follow.
  - `"similarity_threshold"`: `float` RAGFlow employs either a combination of weighted keyword similarity and weighted vector cosine similarity, or a combination of weighted keyword similarity and weighted rerank score during retrieval. This argument sets the threshold for similarities between the user query and chunks. If a similarity score falls below this threshold, the corresponding chunk will be excluded from the results. The default value is `0.2`.
  - `"keywords_similarity_weight"`: `float` This argument sets the weight of keyword similarity in the hybrid similarity score with vector cosine similarity or reranking model similarity. By adjusting this weight, you can control the influence of keyword similarity in relation to other similarity measures. The default value is `0.7`.
  - `"top_n"`: `int` This argument specifies the number of top chunks with similarity scores above the `similarity_threshold` that are fed to the LLM. The LLM will *only* access these 'top N' chunks.  The default value is `8`.
  - `"variables"`: `list[dict[]]`  This argument lists the variables to use in the 'System' field of **Chat Configurations**. Note that:
    - `knowledge` is a reserved variable, which represents the retrieved chunks.
    - All the variables in 'System' should be curly bracketed.
    - The default value is `[{"key": "knowledge", "optional": True}]`.
  - `"rerank_model"`: `str` If it is not specified, vector cosine similarity will be used; otherwise, reranking score will be used. Defaults to `""`.
  - `"empty_response"`: `str` If nothing is retrieved in the dataset for the user's question, this will be used as the response. To allow the LLM to improvise when nothing is retrieved, leave this blank. Defaults to `None`.
  - `"opener"`: `str` The opening greeting for the user. Defaults to `"Hi! I am your assistant, can I help you?"`.
  - `"show_quote`: `bool` Indicates whether the source of text should be displayed Defaults to `True`.
  - `"prompt"`: `str` The prompt content.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
datasets = rag_object.list_datasets(name="kb_1")
dataset_id = datasets[0].id
assistant = rag_object.create_chat("Miss R", dataset_ids=[dataset_id])
assistant.update({"name": "Stefan", "llm": {"temperature": 0.8}, "prompt": {"top_n": 8}})
```

---

### Delete chat assistants

```python
RAGFlow.delete_chats(ids: list[str] = None)
```

Deletes chat assistants by ID.

#### Parameters

##### ids: `list[str]`

The IDs of the chat assistants to delete. Defaults to `None`. If it is empty or not specified, all chat assistants in the system will be deleted.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag_object.delete_chats(ids=["id_1","id_2"])
```

---

### List chat assistants

```python
RAGFlow.list_chats(
    page: int = 1, 
    page_size: int = 30, 
    orderby: str = "create_time", 
    desc: bool = True,
    id: str = None,
    name: str = None
) -> list[Chat]
```

Lists chat assistants.

#### Parameters

##### page: `int`

Specifies the page on which the chat assistants will be displayed. Defaults to `1`.

##### page_size: `int`

The number of chat assistants on each page. Defaults to `30`.

##### orderby: `str`

The attribute by which the results are sorted. Available options:

- `"create_time"` (default)
- `"update_time"`

##### desc: `bool`

Indicates whether the retrieved chat assistants should be sorted in descending order. Defaults to `True`.

##### id: `str`  

The ID of the chat assistant to retrieve. Defaults to `None`.

##### name: `str`  

The name of the chat assistant to retrieve. Defaults to `None`.

#### Returns

- Success: A list of `Chat` objects.
- Failure: `Exception`.

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
for assistant in rag_object.list_chats():
    print(assistant)
```

---

## SESSION MANAGEMENT

---

### Create session with chat assistant

```python
Chat.create_session(name: str = "New session") -> Session
```

Creates a session with the current chat assistant.

#### Parameters

##### name: `str`

The name of the chat session to create.

#### Returns

- Success: A `Session` object containing the following attributes:
  - `id`: `str` The auto-generated unique identifier of the created session.
  - `name`: `str` The name of the created session.
  - `message`: `list[Message]` The opening message of the created session. Default: `[{"role": "assistant", "content": "Hi! I am your assistant, can I help you?"}]`
  - `chat_id`: `str` The ID of the associated chat assistant.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag_object.list_chats(name="Miss R")
assistant = assistant[0]
session = assistant.create_session()
```

---

### Update chat assistant's session

```python
Session.update(update_message: dict)
```

Updates the current session of the current chat assistant.

#### Parameters

##### update_message: `dict[str, Any]`, *Required*

A dictionary representing the attributes to update, with only one key:

- `"name"`: `str` The revised name of the session.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag_object.list_chats(name="Miss R")
assistant = assistant[0]
session = assistant.create_session("session_name")
session.update({"name": "updated_name"})
```

---

### List chat assistant's sessions

```python
Chat.list_sessions(
    page: int = 1, 
    page_size: int = 30, 
    orderby: str = "create_time", 
    desc: bool = True,
    id: str = None,
    name: str = None
) -> list[Session]
```

Lists sessions associated with the current chat assistant.

#### Parameters

##### page: `int`

Specifies the page on which the sessions will be displayed. Defaults to `1`.

##### page_size: `int`

The number of sessions on each page. Defaults to `30`.

##### orderby: `str`

The field by which sessions should be sorted. Available options:

- `"create_time"` (default)
- `"update_time"`

##### desc: `bool`

Indicates whether the retrieved sessions should be sorted in descending order. Defaults to `True`.

##### id: `str`

The ID of the chat session to retrieve. Defaults to `None`.

##### name: `str`

The name of the chat session to retrieve. Defaults to `None`.

#### Returns

- Success: A list of `Session` objects associated with the current chat assistant.
- Failure: `Exception`.

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag_object.list_chats(name="Miss R")
assistant = assistant[0]
for session in assistant.list_sessions():
    print(session)
```

---

### Delete chat assistant's sessions

```python
Chat.delete_sessions(ids:list[str] = None)
```

Deletes sessions of the current chat assistant by ID.

#### Parameters

##### ids: `list[str]`

The IDs of the sessions to delete. Defaults to `None`. If it is not specified, all sessions associated with the current chat assistant will be deleted.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag_object.list_chats(name="Miss R")
assistant = assistant[0]
assistant.delete_sessions(ids=["id_1","id_2"])
```

---

### Converse with chat assistant

```python
Session.ask(question: str = "", stream: bool = False, **kwargs) -> Optional[Message, iter[Message]]
```

Asks a specified chat assistant a question to start an AI-powered conversation.

:::tip NOTE
In streaming mode, not all responses include a reference, as this depends on the system's judgement.
:::

#### Parameters

##### question: `str`, *Required*

The question to start an AI-powered conversation. Default to `""`

##### stream: `bool`

Indicates whether to output responses in a streaming way:

- `True`: Enable streaming (default).
- `False`: Disable streaming.

##### **kwargs

The parameters in prompt(system).

#### Returns

- A `Message` object containing the response to the question if `stream` is set to `False`.
- An iterator containing multiple `message` objects (`iter[Message]`) if `stream` is set to `True`

The following shows the attributes of a `Message` object:

##### id: `str`

The auto-generated message ID.

##### content: `str`

The content of the message. Defaults to `"Hi! I am your assistant, can I help you?"`.

##### reference: `list[Chunk]`

A list of `Chunk` objects representing references to the message, each containing the following attributes:

- `id` `str`  
  The chunk ID.
- `content` `str`  
  The content of the chunk.
- `img_id` `str`  
  The ID of the snapshot of the chunk. Applicable only when the source of the chunk is an image, PPT, PPTX, or PDF file.
- `document_id` `str`  
  The ID of the referenced document.
- `document_name` `str`  
  The name of the referenced document.
- `document_metadata` `dict`  
  Optional document metadata, returned only when `extra_body.reference_metadata.include` is `true`.
- `position` `list[str]`  
  The location information of the chunk within the referenced document.
- `dataset_id` `str`  
  The ID of the dataset to which the referenced document belongs.
- `similarity` `float`  
  A composite similarity score of the chunk ranging from `0` to `1`, with a higher value indicating greater similarity. It is the weighted sum of `vector_similarity` and `term_similarity`.
- `vector_similarity` `float`  
  A vector similarity score of the chunk ranging from `0` to `1`, with a higher value indicating greater similarity between vector embeddings.
- `term_similarity` `float`  
  A keyword similarity score of the chunk ranging from `0` to `1`, with a higher value indicating greater similarity between keywords.

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
assistant = rag_object.list_chats(name="Miss R")
assistant = assistant[0]
session = assistant.create_session()    

print("\n==================== Miss R =====================\n")
print("Hello. What can I do for you?")

while True:
    question = input("\n==================== User =====================\n> ")
    print("\n==================== Miss R =====================\n")
    
    cont = ""
    for ans in session.ask(question, stream=True):
        print(ans.content[len(cont):], end='', flush=True)
        cont = ans.content
```

---

### Create session with agent

```python
Agent.create_session(**kwargs) -> Session
```

Creates a session with the current agent.

#### Parameters

##### **kwargs

The parameters in `begin` component.

#### Returns

- Success: A `Session` object containing the following attributes:
  - `id`: `str` The auto-generated unique identifier of the created session.
  - `message`: `list[Message]` The messages of the created session assistant. Default: `[{"role": "assistant", "content": "Hi! I am your assistant, can I help you?"}]`
  - `agent_id`: `str` The ID of the associated agent.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow, Agent

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
agent_id = "AGENT_ID"
agent = rag_object.list_agents(id = agent_id)[0]
session = agent.create_session()
```

---

### Converse with agent

```python
Session.ask(question: str="", stream: bool = False) -> Optional[Message, iter[Message]]
```

Asks a specified agent a question to start an AI-powered conversation.

:::tip NOTE
In streaming mode, not all responses include a reference, as this depends on the system's judgement.
:::

#### Parameters

##### question: `str`

The question to start an AI-powered conversation. If the **Begin** component takes parameters, a question is not required.

##### stream: `bool`

Indicates whether to output responses in a streaming way:

- `True`: Enable streaming (default).
- `False`: Disable streaming.

#### Returns

- A `Message` object containing the response to the question if `stream` is set to `False`
- An iterator containing multiple `message` objects (`iter[Message]`) if `stream` is set to `True`

The following shows the attributes of a `Message` object:

##### id: `str`

The auto-generated message ID.

##### content: `str`

The content of the message. Defaults to `"Hi! I am your assistant, can I help you?"`.

##### reference: `list[Chunk]`

A list of `Chunk` objects representing references to the message, each containing the following attributes:

- `id` `str`  
  The chunk ID.
- `content` `str`  
  The content of the chunk.
- `image_id` `str`  
  The ID of the snapshot of the chunk. Applicable only when the source of the chunk is an image, PPT, PPTX, or PDF file.
- `document_id` `str`  
  The ID of the referenced document.
- `document_name` `str`  
  The name of the referenced document.
- `document_metadata` `dict`  
  Optional document metadata, returned only when `extra_body.reference_metadata.include` is `true`.
- `position` `list[str]`  
  The location information of the chunk within the referenced document.
- `dataset_id` `str`  
  The ID of the dataset to which the referenced document belongs.
- `similarity` `float`  
  A composite similarity score of the chunk ranging from `0` to `1`, with a higher value indicating greater similarity. It is the weighted sum of `vector_similarity` and `term_similarity`.
- `vector_similarity` `float`  
  A vector similarity score of the chunk ranging from `0` to `1`, with a higher value indicating greater similarity between vector embeddings.
- `term_similarity` `float`  
  A keyword similarity score of the chunk ranging from `0` to `1`, with a higher value indicating greater similarity between keywords.

#### Examples

```python
from ragflow_sdk import RAGFlow, Agent

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
AGENT_id = "AGENT_ID"
agent = rag_object.list_agents(id = AGENT_id)[0]
session = agent.create_session()    

print("\n===== Miss R ====\n")
print("Hello. What can I do for you?")

while True:
    question = input("\n===== User ====\n> ")
    print("\n==== Miss R ====\n")
    
    cont = ""
    for ans in session.ask(question, stream=True):
        print(ans.content[len(cont):], end='', flush=True)
        cont = ans.content
```

---

### List agent sessions

```python
Agent.list_sessions(
    page: int = 1, 
    page_size: int = 30, 
    orderby: str = "update_time", 
    desc: bool = True,
    id: str = None
) -> List[Session]
```

Lists sessions associated with the current agent.

#### Parameters

##### page: `int`

Specifies the page on which the sessions will be displayed. Defaults to `1`.

##### page_size: `int`

The number of sessions on each page. Defaults to `30`.

##### orderby: `str`

The field by which sessions should be sorted. Available options:

- `"create_time"`
- `"update_time"`(default)

##### desc: `bool`

Indicates whether the retrieved sessions should be sorted in descending order. Defaults to `True`.

##### id: `str`

The ID of the agent session to retrieve. Defaults to `None`.

#### Returns

- Success: A list of `Session` objects associated with the current agent.
- Failure: `Exception`.

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
AGENT_id = "AGENT_ID"
agent = rag_object.list_agents(id = AGENT_id)[0]
sessons = agent.list_sessions()
for session in sessions:
    print(session)
```
---
### Delete agent's sessions

```python
Agent.delete_sessions(ids: list[str] = None)
```

Deletes sessions of an agent by ID.

#### Parameters

##### ids: `list[str]`

The IDs of the sessions to delete. Defaults to `None`. If it is not specified, all sessions associated with the agent will be deleted.

#### Returns

- Success: No value is returned.
- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow

rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
AGENT_id = "AGENT_ID"
agent = rag_object.list_agents(id = AGENT_id)[0]
agent.delete_sessions(ids=["id_1","id_2"])
```

---

## AGENT MANAGEMENT

---

### List agents

```python
RAGFlow.list_agents(
    page: int = 1, 
    page_size: int = 30, 
    orderby: str = "create_time", 
    desc: bool = True,
    id: str = None,
    title: str = None
) -> List[Agent]
```

Lists agents.

#### Parameters

##### page: `int`

Specifies the page on which the agents will be displayed. Defaults to `1`.

##### page_size: `int`

The number of agents on each page. Defaults to `30`.

##### orderby: `str`

The attribute by which the results are sorted. Available options:

- `"create_time"` (default)
- `"update_time"`

##### desc: `bool`

Indicates whether the retrieved agents should be sorted in descending order. Defaults to `True`.

##### id: `str`  

The ID of the agent to retrieve. Defaults to `None`.

##### name: `str`  

The name of the agent to retrieve. Defaults to `None`.

#### Returns

- Success: A list of `Agent` objects.
- Failure: `Exception`.

#### Examples

```python
from ragflow_sdk import RAGFlow
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
for agent in rag_object.list_agents():
    print(agent)
```

---

### Create agent

```python
RAGFlow.create_agent(
    title: str,
    dsl: dict,
    description: str | None = None
) -> None
```

Create an agent.

#### Parameters

##### title: `str`

Specifies the title of the agent.

##### dsl: `dict`

Specifies the canvas DSL of the agent.

##### description: `str`

The description of the agent. Defaults to `None`.

#### Returns

- Success: Nothing.
- Failure: `Exception`.

#### Examples

```python
from ragflow_sdk import RAGFlow
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag_object.create_agent(
  title="Test Agent",
  description="A test agent",
  dsl={
    # ... canvas DSL here ...
  }
)
```

---

### Update agent

```python
RAGFlow.update_agent(
    agent_id: str,
    title: str | None = None,
    description: str | None = None,
    dsl: dict | None = None
) -> None
```

Update an agent.

#### Parameters

##### agent_id: `str`

Specifies the id of the agent to be updated.

##### title: `str`

Specifies the new title of the agent. `None` if you do not want to update this.

##### dsl: `dict`

Specifies the new canvas DSL of the agent. `None` if you do not want to update this.

##### description: `str`

The new description of the agent. `None` if you do not want to update this.

#### Returns

- Success: Nothing.
- Failure: `Exception`.

#### Examples

```python
from ragflow_sdk import RAGFlow
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag_object.update_agent(
  agent_id="58af890a2a8911f0a71a11b922ed82d6",
  title="Test Agent",
  description="A test agent",
  dsl={
    # ... canvas DSL here ...
  }
)
```

---

### Delete agent

```python
RAGFlow.delete_agent(
    agent_id: str
) -> None
```

Delete an agent.

#### Parameters

##### agent_id: `str`

Specifies the id of the agent to be deleted.

#### Returns

- Success: Nothing.
- Failure: `Exception`.

#### Examples

```python
from ragflow_sdk import RAGFlow
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag_object.delete_agent("58af890a2a8911f0a71a11b922ed82d6")
```

---



## Memory Management

### Create Memory

```python
Ragflow.create_memory(
    name: str, 
    memory_type: list[str], 
    embd_id: str, 
    llm_id: str
) -> Memory
```

Create a new memory.

#### Parameters

##### name: `str`, *Required*

The unique name of the memory to create. It must adhere to the following requirements:

- Basic Multilingual Plane (BMP) only
- Maximum 128 characters

##### memory_type: `list[str]`, *Required* 

Specifies the types of memory to extract. Available options:

- `raw`: The raw dialogue content between the user and the agent . *Required by default*.
- `semantic`: General knowledge and facts about the user and world.
- `episodic`: Time-stamped records of specific events and experiences.
- `procedural`: Learned skills, habits, and automated procedures.

##### embd_id: `str`, *Required*

The name of the embedding model to use. For example: `"BAAI/bge-large-zh-v1.5@BAAI"`

- Maximum 255 characters
- Must follow `model_name@model_factory` format

##### llm_id: `str`, *Required*

The name of the chat model to use. For example: `"glm-4-flash@ZHIPU-AI"`

- Maximum 255 characters
- Must follow `model_name@model_factory` format

#### Returns

- Success: A `memory` object.

- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import RAGFlow
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
memory = rag_obj.create_memory("name", ["raw"], "BAAI/bge-large-zh-v1.5@SILICONFLOW", "glm-4-flash@ZHIPU-AI")
```

---



### Update Memory

```python
Memory.update(
	update_dict: dict
) -> Memory
```

Updates configurations for a specified memory.

#### Parameters

##### update_dict: `dict`, *Required*

Configurations to update. Available configurations:

- `name`: `string`, *Optional*

  The revised name of the memory.

  - Basic Multilingual Plane (BMP) only
  - Maximum 128 characters, *Optional*

- `avatar`: `string`, *Optional* 

  The updated base64 encoding of the avatar.

  - Maximum 65535 characters

- `permission`:  `enum<string>`, *Optional*

  The updated memory permission. Available options:

  - `"me"`: (Default) Only you can manage the memory.
  - `"team"`: All team members can manage the memory.

- `llm_id`: `string`, *Optional*

  The name of the chat model to use. For example: `"glm-4-flash@ZHIPU-AI"`

  - Maximum 255 characters
  - Must follow `model_name@model_factory` format

- `description`: `string`, *Optional*

  The description of the memory. Defaults to `None`.

- `memory_size`: `int`, *Optional*

  Defaults to `5*1024*1024` Bytes. Accounts for each message's content + its embedding vector (≈ Content + Dimensions × 8 Bytes). Example: A 1 KB message with 1024-dim embedding uses ~9 KB. The 5 MB default limit holds ~500 such messages.

  - Maximum 10 * 1024 * 1024 Bytes

- `forgetting_policy`: `enum<string>`, *Optional*

  Evicts existing data based on the chosen policy when the size limit is reached, freeing up space for new messages. Available options:

  - `"FIFO"`: (Default) Prioritize messages with the earliest `forget_at` time for removal. When the pool of messages that have `forget_at` set is insufficient, it falls back to selecting messages in ascending order of their `valid_at` (oldest first).

- `temperature`: (*Body parameter*), `float`, *Optional*

  Adjusts output randomness. Lower = more deterministic; higher = more creative.

  - Range [0, 1]

- `system_prompt`: (*Body parameter*), `string`, *Optional*

  Defines the system-level instructions and role for the AI assistant. It is automatically assembled based on the selected `memory_type` by `PromptAssembler` in `memory/utils/prompt_util.py`. This prompt sets the foundational behavior and context for the entire conversation.

  - Keep the `OUTPUT REQUIREMENTS` and `OUTPUT FORMAT` parts unchanged.

- `user_prompt`: (*Body parameter*), `string`, *Optional*

  Represents the user's custom setting, which is the specific question or instruction the AI needs to respond to directly. Defaults to `None`.

#### Returns

- Success: A `memory` object.

- Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow, Memory
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
memory_obejct = Memory(rag_object, {"id": "your memory_id"})
memory_object.update({"name": "New_name"})
```

---



### List Memory

```python
Ragflow.list_memory(
    page: int = 1, 
    page_size: int = 50, 
    tenant_id: str | list[str] = None, 
    memory_type: str | list[str] = None, 
    storage_type: str = None, 
    keywords: str = None) -> dict
```

List memories.

#### Parameters

##### page: `int`, *Optional*

Specifies the page on which the datasets will be displayed. Defaults to `1`

##### page_size: `int`, *Optional*

The number of memories on each page. Defaults to `50`.

##### tenant_id: `str` or `list[str]`, *Optional*

The owner's ID, supports search multiple IDs.

##### memory_type: `str` or `list[str]`, *Optional*

The type of memory (as set during creation). A memory matches if its type is **included in** the provided value(s). Available options:

- `raw`
- `semantic`
- `episodic`
- `procedural`

##### storage_type: `str`, *Optional*

The storage format of messages. Available options:

- `table`: (Default)

##### keywords: `str`, *Optional*

The name of memory to retrieve, supports fuzzy search.

#### Returns

Success: A dict of `Memory` object list and total count. 

```json
{"memory_list": list[Memory], "total_count": int}
```

Failure: `Exception`

#### Examples

```
from ragflow_sdk import Ragflow, Memory
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag_obejct.list_memory()
```

---



### Get Memory Config

```python
Memory.get_config()
```

Get the configuration of a specified memory.

#### Parameters

None

#### Returns

Success: A `Memory` object.

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow, Memory
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
memory_obejct = Memory(rag_object, {"id": "your memory_id"})
memory_obejct.get_config()
```

---



### Delete Memory

```python
Ragflow.delete_memory(
    memory_id: str
) -> None
```

Delete a specified memory.

#### Parameters

##### memory_id: `str`, *Required*

The ID of the memory.

#### Returns

Success: Nothing

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow, Memory
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag_object.delete_memory("your memory_id")
```

---



### List messages of a memory

```python
Memory.list_memory_messages(
    agent_id: str | list[str]=None, 
    keywords: str=None, 
    page: int=1, 
    page_size: int=50
) -> dict
```

List the messages of a specified memory.

#### Parameters

##### agent_id: `str` or `list[str]`, *Optional*

Filters messages by the ID of their source agent. Supports multiple values.

##### keywords: `str`, *Optional*

Filters messages by their session ID. This field supports fuzzy search.

##### page: `int`, *Optional*

Specifies the page on which the messages will be displayed. Defaults to `1`.

##### page_size: `int`, *Optional*

The number of messages on each page. Defaults to `50`.

#### Returns

Success: a dict of messages and meta info. 

```json
{"messages": {"message_list": [{message dict}], "total_count": int}, "storage_type": "table"}
```

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow, Memory
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
memory_obejct = Memory(rag_object, {"id": "your memory_id"})
memory_obejct.list_memory_messages()
```

---



### Add Message

```python
Ragflow.add_message(
    memory_id: list[str], 
    agent_id: str, 
    session_id: str, 
    user_input: str, 
    agent_response: str, 
    user_id: str = ""
) -> str
```

Add a message to specified memories.

#### Parameters

##### memory_id: `list[str]`, *Required*

The IDs of the memories to save messages.

##### agent_id: `str`, *Required*

The ID of the message's source agent.

##### session_id: `str`, *Required*

The ID of the message's session.

##### user_input: `str`, *Required*

The text input provided by the user.

##### agent_response: `str`, *Required*

The text response generated by the AI agent.

##### user_id: `str`, *Optional*

The user participating in the conversation with the agent. Defaults to `""`.

#### Returns

Success:  A text `"All add to task."`

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow, Memory
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
message_payload = {
    "memory_id": memory_ids,
    "agent_id": agent_id,
    "session_id": session_id,
    "user_id": "",
    "user_input": "Your question here",
    "agent_response": """
Your agent response here
"""
}
client.add_message(**message_payload)
```

---



### Forget Message

```python
Memory.forget_message(message_id: int) -> bool
```

Forget a specified message. After forgetting, this message will not be retrieved by agents, and it will also be prioritized for cleanup by the forgetting policy.

#### Parameters

##### message_id: `int`, *Required*

The ID of the message to forget.

#### Returns

Success: True

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow, Memory
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
memory_object = Memory(rag_object, {"id": "your memory_id"})
memory_object.forget_message(message_id)
```

---



### Update message status

```python
Memory.update_message_status(message_id: int, status: bool) -> bool
```

Update message status, enable or disable a message. Once a message is disabled, it will not be retrieved by agents.

#### Parameters

##### message_id: `int`, *Required*

The ID of the message to enable or disable.

##### status: `bool`, *Required*

The status of message. `True` = `enabled`, `False` = `disabled`.

#### Returns

Success: `True`

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow, Memory
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
memory_object = Memory(rag_object, {"id": "your memory_id"})
memory_object.update_message_status(message_id, True)
```

---



### Search message

```python
Ragflow.search_message(
    query: str, 
    memory_id: list[str], 
    agent_id: str=None, 
    session_id: str=None, 
    similarity_threshold: float=0.2, 
    keywords_similarity_weight: float=0.7, 
    top_n: int=10
) -> list[dict]
```

Searches and retrieves messages from memory based on the provided `query` and other configuration parameters.

#### Parameters

##### query: `str`, *Required*

The search term or natural language question used to find relevant messages.

##### memory_id: `list[str]`, *Required*

The IDs of the memories to search. Supports multiple values.

##### agent_id: `str`, *Optional*

The ID of the message's source agent. Defaults to `None`.

##### session_id: `str`, *Optional*

The ID of the message's session. Defaults to `None`.

##### similarity_threshold: `float`, *Optional*

The minimum cosine similarity score required for a message to be considered a match. A higher value yields more precise but fewer results. Defaults to `0.2`.

- Range [0.0, 1.0]

##### keywords_similarity_weight: `float`, *Optional*

Controls the influence of keyword matching versus semantic (embedding-based) matching in the final relevance score. A value of 0.5 gives them equal weight. Defaults to `0.7`.

- Range [0.0, 1.0]

##### top_n: `int`, *Optional*

The maximum number of most relevant messages to return. This limits the result set size for efficiency. Defaults to `10`.

#### Returns

Success: A list of `message` dict.

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag_object.search_message("your question", ["your memory_id"])
```

---



### Get Recent Messages

```python
Ragflow.get_recent_messages(
    memory_id: list[str], 
    agent_id: str=None, 
    session_id: str=None, 
    limit: int=10
) -> list[dict]
```

Retrieves the most recent messages from specified memories. Typically accepts a `limit` parameter to control the number of messages returned.

#### Parameters

##### memory_id: `list[str]`, *Required*

The IDs of the memories to search. Supports multiple values.

##### agent_id: `str`, *Optional*

The ID of the message's source agent. Defaults to `None`.

##### session_id: `str`, *Optional*

The ID of the message's session. Defaults to `None`.

##### limit: `int`, *Optional*

Control the number of messages returned. Defaults to `10`.

#### Returns

Success: A list of `message` dict.

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
rag_object.get_recent_messages(["your memory_id"])
```

---



### Get Message Content

```python
Memory.get_message_content(message_id: int)
```

Retrieves the full content and embed vector of a specific message using its unique message ID.

#### Parameters

##### message_id: `int`, *Required*

#### Returns

Success: A `message` dict.

Failure: `Exception`

#### Examples

```python
from ragflow_sdk import Ragflow
rag_object = RAGFlow(api_key="<YOUR_API_KEY>", base_url="http://<YOUR_BASE_URL>:9380")
memory_object = Memory(rag_object, {"id": "your memory_id"})
memory_object.get_message_content(message_id)
```

---
