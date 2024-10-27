
# DRAFT! HTTP API Reference

**THE API REFERENCES BELOW ARE STILL UNDER DEVELOPMENT.**

---

:::tip API GROUPING
Dataset Management
:::

---

## Create dataset

**POST** `/api/v1/dataset`

Creates a dataset.

### Request

- Method: POST
- URL: `/api/v1/dataset`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"name"`: `string`
  - `"avatar"`: `string`
  - `"description"`: `string`
  - `"language"`: `string`
  - `"embedding_model"`: `string`
  - `"permission"`: `string`
  - `"chunk_method"`: `string`
  - `"parser_config"`: `object`

#### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/dataset \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '{
      "name": "test_1"
      }'
```

#### Request parameters

- `"name"`: (*Body parameter*), `string`, *Required*  
  The unique name of the dataset to create. It must adhere to the following requirements:  
  - Permitted characters include:
    - English letters (a-z, A-Z)
    - Digits (0-9)
    - "_" (underscore)
  - Must begin with an English letter or underscore.
  - Maximum 65,535 characters.
  - Case-insensitive.

- `"avatar"`: (*Body parameter*), `string`  
    Base64 encoding of the avatar.

- `"description"`: (*Body parameter*), `string`  
  A brief description of the dataset to create.

- `"language"`: (*Body parameter*), `string`  
  The language setting of the dataset to create. Available options:  
  - `"English"` (Default)
  - `"Chinese"`

- `"embedding_model"`: (*Body parameter*), `string`  
  The name of the embedding model to use. For example: `"BAAI/bge-zh-v1.5"`

- `"permission"`: (*Body parameter*), `string`  
  Specifies who can access the dataset to create. You can set it only to `"me"` for now.

- `"chunk_method"`: (*Body parameter*), `enum<string>`  
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
  - `"knowledge_graph"`: Knowledge Graph
  - `"email"`: Email

- `"parser_config"`: (*Body parameter*), `object`  
  The configuration settings for the dataset parser, a JSON object containing the following attributes:
  - `"chunk_token_count"`: Defaults to `128`.
  - `"layout_recognize"`: Defaults to `true`.
  - `"delimiter"`: Defaults to `"\n!?。；！？"`.
  - `"task_page_size"`: Defaults to `12`.

### Response

Success:

```json
{
    "code": 0,
    "data": {
        "avatar": null,
        "chunk_count": 0,
        "chunk_method": "naive",
        "create_date": "Thu, 24 Oct 2024 09:14:07 GMT",
        "create_time": 1729761247434,
        "created_by": "69736c5e723611efb51b0242ac120007",
        "description": null,
        "document_count": 0,
        "embedding_model": "BAAI/bge-large-zh-v1.5",
        "id": "527fa74891e811ef9c650242ac120006",
        "language": "English",
        "name": "test_1",
        "parser_config": {
            "chunk_token_num": 128,
            "delimiter": "\\n!?;。；！？",
            "html4excel": false,
            "layout_recognize": true,
            "raptor": {
                "user_raptor": false
            }
        },
        "permission": "me",
        "similarity_threshold": 0.2,
        "status": "1",
        "tenant_id": "69736c5e723611efb51b0242ac120007",
        "token_num": 0,
        "update_date": "Thu, 24 Oct 2024 09:14:07 GMT",
        "update_time": 1729761247434,
        "vector_similarity_weight": 0.3
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Duplicated knowledgebase name in creating dataset."
}
```

---

## Delete datasets

**DELETE** `/api/v1/dataset`

Deletes datasets by ID.

### Request

- Method: DELETE
- URL: `/api/v1/dataset`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
  - Body:
    - `"ids"`: `list[string]`

#### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/dataset \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '{"ids": ["test_1", "test_2"]}'
```

#### Request parameters

- `"ids"`: (*Body parameter*), `list[string]`
  The IDs of the datasets to delete. If it is not specified, all datasets will be deleted.

### Response

Success:

```json
{
    "code": 0 
}
```

Failure:

```json
{
    "code": 102,
    "message": "You don't own the dataset."
}
```

---

## Update dataset

**PUT** `/api/v1/dataset/{dataset_id}`

Updates configurations for a specified dataset.

### Request

- Method: PUT
- URL: `/api/v1/dataset/{dataset_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"name"`: `string`
  - `"embedding_model"`: `string`
  - `"chunk_method"`: `enum<string>`

#### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/dataset/{dataset_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "name": "updated_dataset",
     }'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the dataset to update.
- `"name"`: (*Body parameter*), `string`  
  The revised name of the dataset.
- `"embedding_model"`: (*Body parameter*), `string`  
  The updated embedding model name.  
  - Ensure that `"chunk_count"` is `0` before updating `"embedding_model"`.
- `"chunk_method"`: (*Body parameter*), `enum<string>`
  The chunking method for the dataset. Available options:  
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

### Response

Success:

```json
{
    "code": 0 
}
```

Failure:

```json
{
    "code": 102,
    "message": "Can't change tenant_id."
}
```

---

## List datasets

**GET** `/api/v1/dataset?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`

Lists datasets.

### Request

- Method: GET
- URL: `/api/v1/dataset?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`
- Headers:
  - `'Authorization: Bearer {YOUR_API_KEY}'`

#### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/dataset?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id} \
     --header 'Authorization: Bearer {YOUR_API_KEY}'
```

#### Request parameters

- `page`: (*Filter parameter*)  
  Specifies the page on which the datasets will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*)  
  The number of datasets on each page. Defaults to `1024`.
- `orderby`: (*Filter parameter*)  
  The field by which datasets should be sorted. Available options:
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*)  
  Indicates whether the retrieved datasets should be sorted in descending order. Defaults to `true`.
- `name`: (*Filter parameter*)  
  The name of the dataset to retrieve.
- `id`: (*Filter parameter*)  
  The ID of the dataset to retrieve.

### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "avatar": "",
            "chunk_count": 59,
            "create_date": "Sat, 14 Sep 2024 01:12:37 GMT",
            "create_time": 1726276357324,
            "created_by": "69736c5e723611efb51b0242ac120007",
            "description": null,
            "document_count": 1,
            "embedding_model": "BAAI/bge-large-zh-v1.5",
            "id": "6e211ee0723611efa10a0242ac120007",
            "language": "English",
            "name": "mysql",
            "chunk_method": "knowledge_graph",
            "parser_config": {
                "chunk_token_num": 8192,
                "delimiter": "\\n!?;。；！？",
                "entity_types": [
                    "organization",
                    "person",
                    "location",
                    "event",
                    "time"
                ]
            },
            "permission": "me",
            "similarity_threshold": 0.2,
            "status": "1",
            "tenant_id": "69736c5e723611efb51b0242ac120007",
            "token_num": 12744,
            "update_date": "Thu, 10 Oct 2024 04:07:23 GMT",
            "update_time": 1728533243536,
            "vector_similarity_weight": 0.3
        }
    ]
}
```
  
Failure:

```json
{
    "code": 102,
    "message": "The dataset doesn't exist"
}
```

---

:::tip API GROUPING
File Management within Dataset
:::

---

## Upload documents

**POST** `/api/v1/dataset/{dataset_id}/document`

Uploads documents to a specified dataset.

### Request

- Method: POST
- URL: `/api/v1/dataset/{dataset_id}/document`
- Headers:
  - `'Content-Type: multipart/form-data'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Form:
  - `'file=@{FILE_PATH}'`

#### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/dataset/{dataset_id}/document \
     --header 'Content-Type: multipart/form-data' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \     
     --form 'file=@./test1.txt' \
     --form 'file=@./test2.pdf'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the dataset to which the documents will be uploaded.
- `'file'`: (*Body parameter*)  
  A document to upload.

### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "chunk_method": "naive",
            "created_by": "69736c5e723611efb51b0242ac120007",
            "dataset_id": "527fa74891e811ef9c650242ac120006",
            "id": "b330ec2e91ec11efbc510242ac120004",
            "location": "1.txt",
            "name": "1.txt",
            "parser_config": {
                "chunk_token_num": 128,
                "delimiter": "\\n!?;。；！？",
                "html4excel": false,
                "layout_recognize": true,
                "raptor": {
                    "user_raptor": false
                }
            },
            "run": "UNSTART",
            "size": 17966,
            "thumbnail": "",
            "type": "doc"
        }
    ]
}
```

Failure:

```json
{
    "code": 101,
    "message": "No file part!"
}
```

---

## Update document

**PUT** `/api/v1/dataset/{dataset_id}/info/{document_id}`

Updates configurations for a specified document.

### Request

- Method: PUT
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"name"`:`string`
  - `"chunk_method"`:`string`
  - `"parser_config"`:`object`

#### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/dataset/{dataset_id}/info/{document_id} \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --header 'Content-Type: application/json' \
     --data '
     {
          "name": "manual.txt", 
          "chunk_method": "manual", 
          "parser_config": {"chunk_token_count": 128}
     }'

```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The ID of the associated dataset.
- `document_id`: (*Path parameter*)  
  The ID of the document to update.
- `"name"`: (*Body parameter*), `string`
- `"chunk_method"`: (*Body parameter*), `string`  
  The parsing method to apply to the document:  
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
  - `"knowledge_graph"`: Knowledge Graph
  - `"email"`: Email
- `"parser_config"`: (*Body parameter*), `object`
  The parsing configuration for the document:  
  - `"chunk_token_count"`: Defaults to `128`.
  - `"layout_recognize"`: Defaults to `true`.
  - `"delimiter"`: Defaults to `"\n!?。；！？"`.
  - `"task_page_size"`: Defaults to `12`.

### Response

Success:

```json
{
    "code": 0
}
```
  
Failure:

```json
{
    "code": 102,
    "message": "The dataset does not have the document."
}
```

---

## Download document

**GET** `/api/v1/dataset/{dataset_id}/document/{document_id}`

Downloads a document from a specified dataset.

### Request

- Method: GET
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}`
- Headers:
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Output:
  - `'{PATH_TO_THE_FILE}'`

#### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id} \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --output ./ragflow.txt
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `documents_id`: (*Path parameter*)  
  The ID of the document to download.

### Response

Success:

```text
This is a test to verify the file download feature.
```

Failure:

```json
{
    "code": 102,
    "message": "You do not own the dataset 7898da028a0511efbf750242ac1220005."
}
```

---

## List documents

**GET** `/api/v1/dataset/{dataset_id}/info?offset={offset}&limit={limit}&orderby={orderby}&desc={desc}&keywords={keywords}&id={document_id}`

Lists documents in a specified dataset.

### Request

- Method: GET
- URL: `/api/v1/dataset/{dataset_id}/info?keywords={keyword}&page={page}&page_size={limit}&orderby={orderby}&desc={desc}&name={name}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`

#### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/dataset/{dataset_id}/info?keywords={keywords}&offset={offset}&limit={limit}&orderby={orderby}&desc={desc}&id={document_id} \
     --header 'Authorization: Bearer {YOUR_API_KEY}'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `keywords`: (*Filter parameter*), `string`  
  The keywords used to match document titles.
- `offset`: (*Filter parameter*), `integer`  
  The starting index for the documents to retrieve. Typically used in conjunction with `limit`. Defaults to `1`.
- `limit`: (*Filter parameter*), `integer`  
  The maximum number of documents to retrieve. Defaults to `1024`.
- `orderby`: (*Filter parameter*), `string`  
  The field by which documents should be sorted. Available options:
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*), `boolean`  
  Indicates whether the retrieved documents should be sorted in descending order. Defaults to `true`.
- `id`: (*Filter parameter*), `string`  
  The ID of the document to retrieve.

### Response

Success:

```json
{
    "code": 0,
    "data": {
        "docs": [
            {
                "chunk_count": 0,
                "create_date": "Mon, 14 Oct 2024 09:11:01 GMT",
                "create_time": 1728897061948,
                "created_by": "69736c5e723611efb51b0242ac120007",
                "id": "3bcfbf8a8a0c11ef8aba0242ac120006",
                "knowledgebase_id": "7898da028a0511efbf750242ac120005",
                "location": "Test_2.txt",
                "name": "Test_2.txt",
                "parser_config": {
                    "chunk_token_count": 128,
                    "delimiter": "\n!?。；！？",
                    "layout_recognize": true,
                    "task_page_size": 12
                },
                "parser_method": "naive",
                "process_begin_at": null,
                "process_duation": 0.0,
                "progress": 0.0,
                "progress_msg": "",
                "run": "0",
                "size": 7,
                "source_type": "local",
                "status": "1",
                "thumbnail": null,
                "token_count": 0,
                "type": "doc",
                "update_date": "Mon, 14 Oct 2024 09:11:01 GMT",
                "update_time": 1728897061948
            }
        ],
        "total": 1
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "You don't own the dataset 7898da028a0511efbf750242ac1220005. "
}
```

---

## Delete documents

**DELETE** `/api/v1/dataset/{dataset_id}/document`

Deletes documents by ID.

### Request

- Method: DELETE
- URL: `/api/v1/dataset/{dataset_id}/document`
- Headers:
  - `'Content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"ids"`: `list[string]`

#### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/dataset/{dataset_id}/document \
     --header 'Content-Type: application/json' \
     --header 'Authorization: {YOUR_API_KEY}' \
     --data '
     {
          "ids": ["id_1","id_2"]
     }'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `"ids"`: (*Body parameter*), `list[string]`
  The IDs of the documents to delete. If it is not specified, all documents in the specified dataset will be deleted.

### Response

Success:

```json
{
    "code": 0
}.
```

Failure:

```json
{
    "code": 102,
    "message": "You do not own the dataset 7898da028a0511efbf750242ac1220005."
}
```

---

## Parse documents

**POST** `/api/v1/dataset/{dataset_id}/chunk`

Parses documents in a specified dataset.

### Request

- Method: POST
- URL: `/api/v1/dataset/{dataset_id}/chunk`
- Headers:
  - `'content-Type: application/json'`
  - 'Authorization: Bearer {YOUR_API_KEY}'
- Body:
  - `"document_ids"`: `list[string]`

#### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/dataset/{dataset_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "document_ids": ["97a5f1c2759811efaa500242ac120004","97ad64b6759811ef9fc30242ac120004"]
     }'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The dataset ID.
- `"document_ids"`: (*Body parameter*), `list[string]`, *Required*  
  The IDs of the documents to parse.

### Response

Success:

```json
{
    "code": 0
}
```
  
Failure:

```json
{
    "code": 102,
    "message": "`document_ids` is required"
}
```

---

## Stop parsing documents

**DELETE** `/api/v1/dataset/{dataset_id}/chunk`

Stops parsing specified documents.

### Request

- Method: DELETE
- URL: `/api/v1/dataset/{dataset_id}/chunk`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"document_ids"`: `list[string]`

#### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/dataset/{dataset_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "document_ids": ["97a5f1c2759811efaa500242ac120004","97ad64b6759811ef9fc30242ac120004"]
     }'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `"document_ids"`: (*Body parameter*), `list[string]`, *Required*  
  The IDs of the documents for which the parsing should be stopped.

### Response

Success:

```json
{
    "code": 0
}
```
  
Failure:

```json
{
    "code": 102,
    "message": "`document_ids` is required"
}
```

---

## Add chunks

**POST** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`

Adds a chunk to a specified document in a specified dataset.

### Request

- Method: POST
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"content"`: `string`
  - `"important_keywords"`: `list[string]`

#### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "content": "<SOME_CHUNK_CONTENT_HERE>"
     }'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `document_ids`: (*Path parameter*)  
  The associated document ID.
- `"content"`: (*Body parameter*), `string`, *Required*  
  The text content of the chunk.
- `"important_keywords`(*Body parameter*), `list[string]`  
  The key terms or phrases to tag with the chunk.

### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chunk": {
            "content": "ragflow content",
            "create_time": "2024-10-16 08:05:04",
            "create_timestamp": 1729065904.581025,
            "dataset_id": [
                "c7ee74067a2c11efb21c0242ac120006"
            ],
            "document_id": "5c5999ec7be811ef9cab0242ac120005",
            "id": "d78435d142bd5cf6704da62c778795c5",
            "important_keywords": []
        }
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "`content` is required"
}
```

---

## List chunks

**GET** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk?keywords={keywords}&offset={offset}&limit={limit}&id={id}`

Lists chunks in a specified document.

### Request

- Method: GET
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk?keywords={keywords}&offset={offset}&limit={limit}&id={chunk_id}`
- Headers:
  - `'Authorization: Bearer {YOUR_API_KEY}'`

#### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk?keywords={keywords}&offset={offset}&limit={limit}&id={chunk_id} \
     --header 'Authorization: Bearer {YOUR_API_KEY}' 
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `document_ids`: (*Path parameter*)  
  The associated document ID.
- `keywords`(*Filter parameter*), `string`  
  The keywords used to match chunk content.
- `offset`(*Filter parameter*), `string`  
  The starting index for the chunks to retrieve. Defaults to `1`.
- `limit`(*Filter parameter*), `integer`  
  The maximum number of chunks to retrieve.  Default: `1024`
- `id`(*Filter parameter*), `string`  
  The ID of the chunk to retrieve.

### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chunks": [
            {
                "available_int": 1,
                "content": "This is a test content.",
                "docnm_kwd": "1.txt",
                "document_id": "b330ec2e91ec11efbc510242ac120004",
                "id": "b48c170e90f70af998485c1065490726",
                "image_id": "",
                "important_keywords": "",
                "positions": [
                    ""
                ]
            }
        ],
        "doc": {
            "chunk_count": 1,
            "chunk_method": "naive",
            "create_date": "Thu, 24 Oct 2024 09:45:27 GMT",
            "create_time": 1729763127646,
            "created_by": "69736c5e723611efb51b0242ac120007",
            "dataset_id": "527fa74891e811ef9c650242ac120006",
            "id": "b330ec2e91ec11efbc510242ac120004",
            "location": "1.txt",
            "name": "1.txt",
            "parser_config": {
                "chunk_token_num": 128,
                "delimiter": "\\n!?;。；！？",
                "html4excel": false,
                "layout_recognize": true,
                "raptor": {
                    "user_raptor": false
                }
            },
            "process_begin_at": "Thu, 24 Oct 2024 09:56:44 GMT",
            "process_duation": 0.54213,
            "progress": 0.0,
            "progress_msg": "Task dispatched...",
            "run": "2",
            "size": 17966,
            "source_type": "local",
            "status": "1",
            "thumbnail": "",
            "token_count": 8,
            "type": "doc",
            "update_date": "Thu, 24 Oct 2024 11:03:15 GMT",
            "update_time": 1729767795721
        },
        "total": 1
    }
}
```
  
Failure:

```json
{
    "code": 102,
    "message": "You don't own the document 5c5999ec7be811ef9cab0242ac12000e5."
}
```

---

## Delete chunks

**DELETE** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`

Deletes chunks by ID.

### Request

- Method: DELETE
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"chunk_ids"`: `list[string]`

#### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "chunk_ids": ["test_1", "test_2"]
     }'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `document_ids`: (*Path parameter*)  
  The associated document ID.
- `"chunk_ids"`: (*Body parameter*), `list[string]`  
  The IDs of the chunks to delete. If it is not specified, all chunks of the specified document will be deleted.

### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "`chunk_ids` is required"
}
```

---

## Update chunk

**PUT** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk/{chunk_id}`

Updates content or configurations for a specified chunk.

### Request

- Method: PUT
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk/{chunk_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"content"`: `string`
  - `"important_keywords"`: `string`
  - `"available"`: `integer`

#### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk/{chunk_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: {YOUR_API_KEY}' \
     --data '
     {   
          "content": "ragflow123",  
          "important_keywords": [],   
     }'
```

#### Request parameters

- `dataset_id`: (*Path parameter*)  
  The associated dataset ID.
- `document_ids`: (*Path parameter*)  
  The associated document ID.
- `chunk_id`: (*Path parameter*)  
  The ID of the chunk to update.
- `"content"`: (*Body parameter*), `string`  
  The text content of the chunk.
- `"important_keywords"`: (*Body parameter*), `list[string]`  
  A list of key terms or phrases to tag with the chunk.
- `"available"`: (*Body parameter*) `boolean`  
  The chunk's availability status in the dataset. Value options:  
  - `true`: Available (default)
  - `false`: Unavailable

### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "Can't find this chunk 29a2d9987e16ba331fb4d7d30d99b71d2"
}
```

---

## Retrieve chunks

**GET** `/api/v1/retrieval`

Retrieves chunks from specified datasets.

### Request

- Method: POST
- URL: `/api/v1/retrieval`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"question"`: `string`  
  - `"dataset_ids"`: `list[string]`  
  - `"document_ids"`: `list[string]`
  - `"offset"`: `integer`  
  - `"limit"`: `integer`  
  - `"similarity_threshold"`: `float`  
  - `"vector_similarity_weight"`: `float`  
  - `"top_k"`: `integer`  
  - `"rerank_id"`: `string`  
  - `"keyword"`: `boolean`  
  - `"highlight"`: `boolean`

#### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/retrieval \
     --header 'Content-Type: application/json' \
     --header 'Authorization: {YOUR_API_KEY}' \
     --data '
     {
          "question": "What is advantage of ragflow?",
          "dataset_ids": ["b2a62730759d11ef987d0242ac120004"],
          "document_ids": ["77df9ef4759a11ef8bdd0242ac120004"]
     }'
```

#### Request parameter

- `"question"`: (*Body parameter*), `string`, *Required*  
  The user query or query keywords.
- `"dataset_ids"`: (*Body parameter*) `list[string]`  
  The IDs of the datasets to search. If you do not set this argument, ensure that you set `"document_ids"`.
- `"document_ids"`: (*Body parameter*), `list[string]`  
  The IDs of the documents to search. Ensure that all selected documents use the same embedding model. Otherwise, an error will occur. If you do not set this argument, ensure that you set `"dataset_ids"`.
- `"offset"`: (*Body parameter*), `integer`  
  The starting index for the documents to retrieve. Defaults to `1`.
- `"limit"`: (*Body parameter*)  
  The maximum number of chunks to retrieve. Defaults to `1024`.
- `"similarity_threshold"`: (*Body parameter*)  
  The minimum similarity score. Defaults to `0.2`.
- `"vector_similarity_weight"`: (*Body parameter*), `float`  
  The weight of vector cosine similarity. Defaults to `0.3`. If x represents the vector cosine similarity, then (1 - x) is the term similarity weight.
- `"top_k"`: (*Body parameter*), `integer`  
  The number of chunks engaged in vector cosine computaton. Defaults to `1024`.
- `"rerank_id"`: (*Body parameter*), `integer`  
  The ID of the rerank model.
- `"keyword"`: (*Body parameter*), `boolean`  
  Indicates whether to enable keyword-based matching:  
  - `true`: Enable keyword-based matching.
  - `false`: Disable keyword-based matching (default).
- `"highlight"`: (*Body parameter*), `boolean`  
  Specifies whether to enable highlighting of matched terms in the results:  
  - `true`: Enable highlighting of matched terms.
  - `false`: Disable highlighting of matched terms (default).

### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chunks": [
            {
                "content": "ragflow content",
                "content_ltks": "ragflow content",
                "document_id": "5c5999ec7be811ef9cab0242ac120005",
                "document_keyword": "1.txt",
                "highlight": "<em>ragflow</em> content",
                "id": "d78435d142bd5cf6704da62c778795c5",
                "img_id": "",
                "important_keywords": [
                    ""
                ],
                "kb_id": "c7ee74067a2c11efb21c0242ac120006",
                "positions": [
                    ""
                ],
                "similarity": 0.9669436601210759,
                "term_similarity": 1.0,
                "vector_similarity": 0.8898122004035864
            }
        ],
        "doc_aggs": [
            {
                "count": 1,
                "doc_id": "5c5999ec7be811ef9cab0242ac120005",
                "doc_name": "1.txt"
            }
        ],
        "total": 1
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "`datasets` is required."
}
```

---

:::tip API GROUPING
Chat Assistant Management
:::

---

## Create chat assistant

**POST** `/api/v1/chat`

Creates a chat assistant.

### Request

- Method: POST
- URL: `/api/v1/chat`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"name"`: `string`
  - `"avatar"`: `string`
  - `"dataset_ids"`: `list[string]`
  - `"llm"`: `object`
  - `"prompt"`: `object`

#### Request example

```shell
curl --request POST \
     --url http://{address}/api/v1/chat \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}'
     --data '{
    "dataset_ids": ["0b2cbc8c877f11ef89070242ac120005"],
    "name":"new_chat_1"
}'
```

#### Request parameters

- `"name"`: (*Body parameter*), `string`, *Required*  
  The name of the chat assistant.
- `"avatar"`: (*Body parameter*), `string`  
  Base64 encoding of the avatar.
- `"dataset_ids"`: (*Body parameter*), `list[string]`  
  The IDs of the associated datasets.
- `"llm"`: (*Body parameter*), `object`  
  The LLM settings for the chat assistant to create. If it is not explicitly set, a JSON object with the following values will be generated as the default. An `llm` JSON object contains the following attributes:  
  - `"model_name"`, `string`  
    The chat model name. If not set, the user's default chat model will be used.  
  - `"temperature"`: `float`  
    Controls the randomness of the model's predictions. A lower temperature increases the model's confidence in its responses; a higher temperature increases creativity and diversity. Defaults to `0.1`.  
  - `"top_p"`: `float`  
    Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
  - `"presence_penalty"`: `float`  
    This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.2`.
  - `"frequency penalty"`: `float`  
    Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.
  - `"max_token"`: `integer`  
    The maximum length of the model’s output, measured in the number of tokens (words or pieces of words). Defaults to `512`.  
- `"prompt"`: (*Body parameter*), `object`  
  Instructions for the LLM to follow. If it is not explicitly set, a JSON object with the following values will be generated as the default. A `prompt` JSON object contains the following attributes:  
  - `"similarity_threshold"`: `float` RAGFlow uses a hybrid of weighted keyword similarity and vector cosine similarity during retrieval. This argument sets the threshold for similarities between the user query and chunks. If a similarity score falls below this threshold, the corresponding chunk will be excluded from the results. The default value is `0.2`.
  - `"keywords_similarity_weight"`: `float` This argument sets the weight of keyword similarity in the hybrid similarity score with vector cosine similarity or reranking model similarity. By adjusting this weight, you can control the influence of keyword similarity in relation to other similarity measures. The default value is `0.7`.
  - `"top_n"`: `int` This argument specifies the number of top chunks with similarity scores above the `similarity_threshold` that are fed to the LLM. The LLM will *only* access these 'top N' chunks.  The default value is `8`.
  - `"variables"`: `object[]` This argument lists the variables to use in the 'System' field of **Chat Configurations**. Note that:  
    - `"knowledge"` is a reserved variable, which represents the retrieved chunks.
    - All the variables in 'System' should be curly bracketed.
    - The default value is `[{"key": "knowledge", "optional": true}]`.
  - `"rerank_model"`: `string` If it is not specified, vector cosine similarity will be used; otherwise, reranking score will be used.
  - `"empty_response"`: `string` If nothing is retrieved in the dataset for the user's question, this will be used as the response. To allow the LLM to improvise when nothing is found, leave this blank.
  - `"opener"`: `string` The opening greeting for the user. Defaults to `"Hi! I am your assistant, can I help you?"`.
  - `"show_quote`: `boolean` Indicates whether the source of text should be displayed. Defaults to `true`.
  - `"prompt"`: `string` The prompt content. Defaults to `You are an intelligent assistant. Please summarize the content of the dataset to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.`

### Response

Success:

```json
{
    "code": 0,
    "data": {
        "avatar": "",
        "create_date": "Thu, 24 Oct 2024 11:18:29 GMT",
        "create_time": 1729768709023,
        "dataset_ids": [
            "527fa74891e811ef9c650242ac120006"
        ],
        "description": "A helpful Assistant",
        "do_refer": "1",
        "id": "b1f2f15691f911ef81180242ac120003",
        "language": "English",
        "llm": {
            "frequency_penalty": 0.7,
            "max_tokens": 512,
            "model_name": "qwen-plus@Tongyi-Qianwen",
            "presence_penalty": 0.4,
            "temperature": 0.1,
            "top_p": 0.3
        },
        "name": "12234",
        "prompt": {
            "empty_response": "Sorry! No relevant content was found in the knowledge base!",
            "keywords_similarity_weight": 0.3,
            "opener": "Hi! I'm your assistant, what can I do for you?",
            "prompt": "You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence \"The answer you are looking for is not found in the knowledge base!\" Answers need to consider chat history.\n      Here is the knowledge base:\n      {knowledge}\n      The above is the knowledge base.",
            "rerank_model": "",
            "similarity_threshold": 0.2,
            "top_n": 6,
            "variables": [
                {
                    "key": "knowledge",
                    "optional": false
                }
            ]
        },
        "prompt_type": "simple",
        "status": "1",
        "tenant_id": "69736c5e723611efb51b0242ac120007",
        "top_k": 1024,
        "update_date": "Thu, 24 Oct 2024 11:18:29 GMT",
        "update_time": 1729768709023
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Duplicated chat name in creating dataset."
}
```

---

## Update chat assistant

**PUT** `/api/v1/chat/{chat_id}`

Updates configurations for a specified chat assistant.

### Request

- Method: PUT
- URL: `/api/v1/chat/{chat_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"name"`: `string`
  - `"avatar"`: `string`
  - `"dataset_ids"`: `list[string]`
  - `"llm"`: `object`
  - `"prompt"`: `object`
  
#### Request example

```bash
curl --request PUT \
     --url http://{address}/api/v1/chat/{chat_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "name":"Test"
     }'
```

#### Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the chat assistant to update.
- `"name"`: (*Body parameter*), `string`, *Required*  
  The revised name of the chat assistant.
- `"avatar"`: (*Body parameter*), `string`  
  Base64 encoding of the avatar.
- `"dataset_ids"`: (*Body parameter*), `list[string]`  
  The IDs of the associated datasets.
- `"llm"`: (*Body parameter*), `object`  
  The LLM settings for the chat assistant to create. If it is not explicitly set, a dictionary with the following values will be generated as the default. An `llm` object contains the following attributes:  
  - `"model_name"`, `string`  
    The chat model name. If not set, the user's default chat model will be used.  
  - `"temperature"`: `float`  
    Controls the randomness of the model's predictions. A lower temperature increases the model's confidence in its responses; a higher temperature increases creativity and diversity. Defaults to `0.1`.  
  - `"top_p"`: `float`  
    Also known as “nucleus sampling”, this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones. Defaults to `0.3`  
  - `"presence_penalty"`: `float`  
    This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation. Defaults to `0.2`.
  - `"frequency penalty"`: `float`  
    Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently. Defaults to `0.7`.
  - `"max_token"`: `integer`  
    The maximum length of the model’s output, measured in the number of tokens (words or pieces of words). Defaults to `512`.  
- `"prompt"`: (*Body parameter*), `object`  
  Instructions for the LLM to follow.  A `prompt` object contains the following attributes:  
  - `"similarity_threshold"`: `float` RAGFlow uses a hybrid of weighted keyword similarity and vector cosine similarity during retrieval. This argument sets the threshold for similarities between the user query and chunks. If a similarity score falls below this threshold, the corresponding chunk will be excluded from the results. The default value is `0.2`.
  - `"keywords_similarity_weight"`: `float` This argument sets the weight of keyword similarity in the hybrid similarity score with vector cosine similarity or reranking model similarity. By adjusting this weight, you can control the influence of keyword similarity in relation to other similarity measures. The default value is `0.7`.
  - `"top_n"`: `int` This argument specifies the number of top chunks with similarity scores above the `similarity_threshold` that are fed to the LLM. The LLM will *only* access these 'top N' chunks.  The default value is `8`.
  - `"variables"`: `object[]` This argument lists the variables to use in the 'System' field of **Chat Configurations**. Note that:  
    - `"knowledge"` is a reserved variable, which represents the retrieved chunks.
    - All the variables in 'System' should be curly bracketed.
    - The default value is `[{"key": "knowledge", "optional": true}]`
  - `"rerank_model"`: `string` If it is not specified, vector cosine similarity will be used; otherwise, reranking score will be used.
  - `"empty_response"`: `string` If nothing is retrieved in the dataset for the user's question, this will be used as the response. To allow the LLM to improvise when nothing is found, leave this blank.
  - `"opener"`: `string` The opening greeting for the user. Defaults to `"Hi! I am your assistant, can I help you?"`.
  - `"show_quote`: `boolean` Indicates whether the source of text should be displayed. Defaults to `true`.
  - `"prompt"`: `string` The prompt content. Defaults to `You are an intelligent assistant. Please summarize the content of the dataset to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.`

### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "Duplicated chat name in updating dataset."
}
```

---

## Delete chat assistants

**DELETE** `/api/v1/chat`

Deletes chat assistants by ID.

### Request

- Method: DELETE
- URL: `/api/v1/chat`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"ids"`: `list[string]`

#### Request example

```bash
curl --request DELETE \
     --url http://{address}/api/v1/chat \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "ids": ["test_1", "test_2"]
     }'
```

#### Request parameters

- `"ids"`: (*Body parameter*), `list[string]`  
  The IDs of the chat assistants to delete. If it is not specified, all chat assistants in the system will be deleted.

### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "ids are required"
}
```

---

## List chat assistants

**GET** `/api/v1/chat?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={chat_name}&id={chat_id}`

Lists chat assistants.

### Request

- Method: GET
- URL: `/api/v1/chat?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`
- Headers:
  - `'Authorization: Bearer {YOUR_API_KEY}'`

#### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/chat?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id} \
     --header 'Authorization: Bearer {YOUR_API_KEY}'
```

#### Request parameters

- `page`: (*Filter parameter*), `integer`  
  Specifies the page on which the chat assistants will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*), `integer`  
  The number of chat assistants on each page. Defaults to `1024`.
- `orderby`: (*Filter parameter*), `string`  
  The attribute by which the results are sorted. Available options:
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*), `boolean`  
  Indicates whether the retrieved chat assistants should be sorted in descending order. Defaults to `true`.
- `id`: (*Filter parameter*), `string`  
  The ID of the chat assistant to retrieve.
- `name`: (*Filter parameter*), `string`  
  The name of the chat assistant to retrieve.

### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "avatar": "",
            "create_date": "Fri, 18 Oct 2024 06:20:06 GMT",
            "create_time": 1729232406637,
            "description": "A helpful Assistant",
            "do_refer": "1",
            "id": "04d0d8e28d1911efa3630242ac120006",
            "dataset_ids": ["527fa74891e811ef9c650242ac120006"],
            "language": "English",
            "llm": {
                "frequency_penalty": 0.7,
                "max_tokens": 512,
                "model_name": "qwen-plus@Tongyi-Qianwen",
                "presence_penalty": 0.4,
                "temperature": 0.1,
                "top_p": 0.3
            },
            "name": "13243",
            "prompt": {
                "empty_response": "Sorry! No relevant content was found in the knowledge base!",
                "keywords_similarity_weight": 0.3,
                "opener": "Hi! I'm your assistant, what can I do for you?",
                "prompt": "You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence \"The answer you are looking for is not found in the knowledge base!\" Answers need to consider chat history.\n      Here is the knowledge base:\n      {knowledge}\n      The above is the knowledge base.",
                "rerank_model": "",
                "similarity_threshold": 0.2,
                "top_n": 6,
                "variables": [
                    {
                        "key": "knowledge",
                        "optional": false
                    }
                ]
            },
            "prompt_type": "simple",
            "status": "1",
            "tenant_id": "69736c5e723611efb51b0242ac120007",
            "top_k": 1024,
            "update_date": "Fri, 18 Oct 2024 06:20:06 GMT",
            "update_time": 1729232406638
        }
    ]
}
```

Failure:

```json
{
    "code": 102,
    "message": "The chat doesn't exist"
}
```

## Create session

**POST** `/api/v1/chat/{chat_id}/session`

Creates a chat session.

### Request

- Method: POST
- URL: `/api/v1/chat/{chat_id}/session`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"name"`: `string`

#### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/chat/{chat_id}/session \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "name": "new session"
     }'
```

#### Request parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `"name"`: (*Body parameter*), `string`  
  The name of the chat session to create.

### Response

Success:

```json
{
    "code": 0,
    "data": {
        "chat_id": "2ca4b22e878011ef88fe0242ac120005",
        "create_date": "Fri, 11 Oct 2024 08:46:14 GMT",
        "create_time": 1728636374571,
        "id": "4606b4ec87ad11efbc4f0242ac120006",
        "messages": [
            {
                "content": "Hi! I am your assistant，can I help you?",
                "role": "assistant"
            }
        ],
        "name": "new session",
        "update_date": "Fri, 11 Oct 2024 08:46:14 GMT",
        "update_time": 1728636374571
    }
}
```

Failure:

```json
{
    "code": 102,
    "message": "Name can not be empty."
}
```

---

## Update session

**PUT** `/api/v1/chat/{chat_id}/session/{session_id}`

Updates a chat session.

### Request

- Method: PUT
- URL: `/api/v1/chat/{chat_id}/session/{session_id}`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"name`: string

#### Request example
```bash
curl --request PUT \
     --url http://{address}/api/v1/chat/{chat_id}/session/{session_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data '
     {
          "name": "<REVISED_SESSION_NAME_HERE>"
     }'
```

#### Request Parameter

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `session_id`: (*Path parameter*)  
  The ID of the session to update.
- `"name"`: (*Body Parameter), `string`  
  The revised name of the session.

### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "Name cannot be empty."
}
```

---

## List sessions

**GET** `/api/v1/chat/{chat_id}/session?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={session_name}&id={session_id}`

Lists sessions associated with a specified chat assistant.

### Request

- Method: GET
- URL: `/api/v1/chat/{chat_id}/session?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={session_name}&id={session_id}`
- Headers:
  - `'Authorization: Bearer {YOUR_API_KEY}'`

#### Request example

```bash
curl --request GET \
     --url http://{address}/api/v1/chat/{chat_id}/session?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={session_name}&id={session_id} \
     --header 'Authorization: Bearer {YOUR_API_KEY}'
```

#### Request Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `page`: (*Filter parameter*), `integer`  
  Specifies the page on which the sessions will be displayed. Defaults to `1`.
- `page_size`: (*Filter parameter*), `integer`  
  The number of sessions on each page. Defaults to `1024`.
- `orderby`: (*Filter parameter*), `string`  
  The field by which sessions should be sorted. Available options:  
  - `create_time` (default)
  - `update_time`
- `desc`: (*Filter parameter*), `boolean`  
  Indicates whether the retrieved sessions should be sorted in descending order. Defaults to `true`.
- `name`: (*Filter parameter*) `string`  
  The name of the chat session to retrieve.
- `id`: (*Filter parameter*), `string`  
  The ID of the chat session to retrieve.

### Response

Success:

```json
{
    "code": 0,
    "data": [
        {
            "chat": "2ca4b22e878011ef88fe0242ac120005",
            "create_date": "Fri, 11 Oct 2024 08:46:43 GMT",
            "create_time": 1728636403974,
            "id": "578d541e87ad11ef96b90242ac120006",
            "messages": [
                {
                    "content": "Hi! I am your assistant，can I help you?",
                    "role": "assistant"
                }
            ],
            "name": "new session",
            "update_date": "Fri, 11 Oct 2024 08:46:43 GMT",
            "update_time": 1728636403974
        }
    ]
}
```

Failure:

```json
{
    "code": 102,
    "message": "The session doesn't exist"
}
```

---

## Delete sessions

**DELETE** `/api/v1/chat/{chat_id}/session`

Deletes sessions by ID.

### Request

- Method: DELETE
- URL: `/api/v1/chat/{chat_id}/session`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"ids"`: `list[string]`

#### Request example

```bash
# Either id or name must be provided, but not both.
curl --request DELETE \
     --url http://{address}/api/v1/chat/{chat_id}/session \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bear {YOUR_API_KEY}' \
     --data '
     {
          "ids": ["test_1", "test_2"]
     }'
```

#### Request Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `"ids"`: (*Body Parameter*), `list[string]`  
  The IDs of the sessions to delete. If it is not specified, all sessions associated with the specified chat assistant will be deleted.

### Response

Success:

```json
{
    "code": 0
}
```

Failure:

```json
{
    "code": 102,
    "message": "The chat doesn't own the session"
}
```

---

## Converse

**POST** `/api/v1/chat/{chat_id}/completion`

Asks a question to start an AI-powered conversation.

### Request

- Method: POST
- URL: `/api/v1/chat/{chat_id}/completion`
- Headers:
  - `'content-Type: application/json'`
  - `'Authorization: Bearer {YOUR_API_KEY}'`
- Body:
  - `"question"`: `string`
  - `"stream"`: `boolean`
  - `"session_id"`: `string`

#### Request example

```bash
curl --request POST \
     --url http://{address} /api/v1/chat/{chat_id}/completion \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_API_KEY}' \
     --data-binary '
     {
          "question": "What is RAGFlow?",
          "stream": true
     }'
```

#### Request Parameters

- `chat_id`: (*Path parameter*)  
  The ID of the associated chat assistant.
- `"question"`: (*Body Parameter*), `string` *Required*  
  The question to start an AI-powered conversation.
- `"stream"`: (*Body Parameter*), `boolean`  
  Indicates whether to output responses in a streaming way:
  - `true`: Enable streaming.
  - `false`: Disable streaming (default).
- `"session_id"`: (*Body Parameter*)  
  The ID of session. If it is not provided, a new session will be generated.

### Response

Success:

```json
data: {
    "code": 0,
    "data": {
        "answer": "I am an intelligent assistant designed to help you with your inquiries. I can provide",
        "reference": {},
        "audio_binary": null,
        "id": "d8e5ebb6-6b52-4fd1-bd02-35b52ba3acaa",
        "session_id": "e14344d08d1a11efb6210242ac120004"
    }
}

data: {
    "code": 0,
    "data": {
        "answer": "I am an intelligent assistant designed to help you with your inquiries. I can provide information, answer questions, and assist with tasks based on the knowledge available to me",
        "reference": {},
        "audio_binary": null,
        "id": "d8e5ebb6-6b52-4fd1-bd02-35b52ba3acaa",
        "session_id": "e14344d08d1a11efb6210242ac120004"
    }
}

data: {
    "code": 0,
    "data": {
        "answer": "I am an intelligent assistant designed to help you with your inquiries. I can provide information, answer questions, and assist with tasks based on the knowledge available to me. How can I assist you today?",
        "reference": {},
        "audio_binary": null,
        "id": "d8e5ebb6-6b52-4fd1-bd02-35b52ba3acaa",
        "session_id": "e14344d08d1a11efb6210242ac120004"
    }
}

data: {
    "code": 0,
    "data": {
        "answer": "I am an intelligent assistant designed to help you with your inquiries. I can provide information, answer questions, and assist with tasks based on the knowledge available to me ##0$$. How can I assist you today?",
        "reference": {
            "total": 8,
            "chunks": [
                {
                    "chunk_id": "895d34de762e674b43e8613c6fb54c6d",
                    "content_ltks": "xxxx\r\n\r\n\"\"\"\r\nyou are an intellig assistant. pleas summar the content of the knowledg base to answer the question. pleas list thedata in the knowledg base and answer in detail. when all knowledg base content is irrelev to the question , your answer must includ the sentenc\"the answer you are lookfor isnot found in the knowledg base!\" answer needto consid chat history.\r\n here is the knowledg base:\r\n{ knowledg}\r\nthe abov is the knowledg base.\r\n\"\"\"\r\n1\r\n 2\r\n 3\r\n 4\r\n 5\r\n 6\r\nxxxx ",
                    "content_with_weight": "xxxx\r\n\r\n\"\"\"\r\nYou are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence \"The answer you are looking for is not found in the knowledge base!\" Answers need to consider chat history.\r\n      Here is the knowledge base:\r\n      {knowledge}\r\n      The above is the knowledge base.\r\n\"\"\"\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\nxxxx\r\n\r\n\"\"\"\r\nxxxx",
                    "doc_id": "5c5999ec7be811ef9cab0242ac120005",
                    "docnm_kwd": "1.txt",
                    "kb_id": "c7ee74067a2c11efb21c0242ac120006",
                    "important_kwd": [],
                    "img_id": "",
                    "similarity": 0.4442746624416507,
                    "vector_similarity": 0.3843936320913369,
                    "term_similarity": 0.4699379611632138,
                    "positions": [
                        ""
                    ]
                }
            ],
            "doc_aggs": [
                {
                    "doc_name": "1.txt",
                    "doc_id": "5c5999ec7be811ef9cab0242ac120005",
                    "count": 1
                }
            ]
        },
        "prompt": "xxxx\r\n\r\n\"\"\"\r\nYou are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence \"The answer you are looking for is not found in the knowledge base!\" Answers need to consider chat history.\r\n      Here is the knowledge base:\r\n      {knowledge}\r\n      The above is the knowledge base.\r\n\"\"\"\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\nxxxx\n\n### Query:\nwho are you,please answer me in English\n\n### Elapsed\n  - Retrieval: 332.2 ms\n  - LLM: 2972.1 ms",
        "id": "d8e5ebb6-6b52-4fd1-bd02-35b52ba3acaa",
        "session_id": "e14344d08d1a11efb6210242ac120004"
    }
}

data:{
    "code": 0,
    "data": true
}
```

Failure:

```json
{
    "code": 102,
    "message": "Please input your question."
}
```
