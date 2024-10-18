
# DRAFT! HTTP API Reference

**THE API REFERENCES BELOW ARE STILL UNDER DEVELOPMENT.**

## Create dataset

**POST** `/api/v1/dataset`

Creates a dataset.

### Request

- Method: POST
- URL: `http://{address}/api/v1/dataset`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `"id"`: `string`
  - `"name"`: `string`
  - `"avatar"`: `string`
  - `"tenant_id"`: `string`
  - `"description"`: `string`
  - `"language"`: `string`
  - `"embedding_model"`: `string`
  - `"permission"`: `string`
  - `"document_count"`: `integer`
  - `"chunk_count"`: `integer`
  - `"parse_method"`: `string`
  - `"parser_config"`: `Dataset.ParserConfig`

#### Request example

```bash
# "id": id must not be provided.
# "name": name is required and can't be duplicated.
# "tenant_id": tenant_id must not be provided.
# "embedding_model": embedding_model must not be provided.
# "naive" means general.
curl --request POST \
  --url http://{address}/api/v1/dataset \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
  "name": "test",
  "chunk_count": 0,
  "document_count": 0,
  "parse_method": "naive"
}'
```

#### Request parameters

- `"id"`: (*Body parameter*)  
    The ID of the created dataset used to uniquely identify different datasets.  
    - If creating a dataset, `id` must not be provided.

- `"name"`: (*Body parameter*)  
    The name of the dataset, which must adhere to the following requirements:  
    - Required when creating a dataset and must be unique.
    - If updating a dataset, `name` must still be unique.

- `"avatar"`: (*Body parameter*)  
    Base64 encoding of the avatar.

- `"tenant_id"`: (*Body parameter*)  
    The ID of the tenant associated with the dataset, used to link it with specific users.  
    - If creating a dataset, `tenant_id` must not be provided.
    - If updating a dataset, `tenant_id` cannot be changed.

- `"description"`: (*Body parameter*)  
    The description of the dataset.

- `"language"`: (*Body parameter*)  
    The language setting for the dataset.

- `"embedding_model"`: (*Body parameter*)  
    Embedding model used in the dataset to generate vector embeddings.  
    - If creating a dataset, `embedding_model` must not be provided.
    - If updating a dataset, `embedding_model` cannot be changed.

- `"permission"`: (*Body parameter*)  
    Specifies who can manipulate the dataset.

- `"document_count"`: (*Body parameter*)  
    Document count of the dataset.  
    - If updating a dataset, `document_count` cannot be changed.

- `"chunk_count"`: (*Body parameter*)  
    Chunk count of the dataset.  
    - If updating a dataset, `chunk_count` cannot be changed.

- `"parse_method"`: (*Body parameter*)  
    Parsing method of the dataset.  
    - If updating `parse_method`, `chunk_count` must be greater than 0.

- `"parser_config"`: (*Body parameter*)  
    The configuration settings for the dataset parser.

### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0,
    "data": {
        "avatar": null,
        "chunk_count": 0,
        "create_date": "Thu, 10 Oct 2024 05:57:37 GMT",
        "create_time": 1728539857641,
        "created_by": "69736c5e723611efb51b0242ac120007",
        "description": null,
        "document_count": 0,
        "embedding_model": "BAAI/bge-large-zh-v1.5",
        "id": "8d73076886cc11ef8c270242ac120006",
        "language": "English",
        "name": "test_1",
        "parse_method": "naive",
        "parser_config": {
            "pages": [
                [
                    1,
                    1000000
                ]
            ]
        },
        "permission": "me",
        "similarity_threshold": 0.2,
        "status": "1",
        "tenant_id": "69736c5e723611efb51b0242ac120007",
        "token_num": 0,
        "update_date": "Thu, 10 Oct 2024 05:57:37 GMT",
        "update_time": 1728539857641,
        "vector_similarity_weight": 0.3
    }
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "Duplicated knowledgebase name in creating dataset."
}
```

## Delete datasets

**DELETE** `/api/v1/dataset`

Deletes datasets by ids.

### Request

- Method: DELETE
- URL: `http://{address}/api/v1/dataset`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
  - Body:
    - `"ids"`: `List[string]`


#### Request example

```bash
# Either id or name must be provided, but not both.
curl --request DELETE \
  --url http://{address}/api/v1/dataset \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
  "ids": ["test_1", "test_2"]
  }'
```

#### Request parameters

- `"ids"`: (*Body parameter*)
    Dataset IDs to delete.


### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0 
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "You don't own the dataset."
}
```

## Update dataset

**PUT** `/api/v1/dataset/{dataset_id}`

Updates a dataset by its id.

### Request

- Method: PUT
- URL: `http://{address}/api/v1/dataset/{dataset_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
  - Body: (Refer to the "Create Dataset" for the complete structure of the request body.)


#### Request example

```bash
# "id":  id is required.
# "name": If you update name, it can't be duplicated.
# "tenant_id": If you update tenant_id, it can't be changed
# "embedding_model": If you update embedding_model, it can't be changed.
# "chunk_count": If you update chunk_count, it can't be changed.
# "document_count": If you update document_count, it can't be changed.
# "parse_method": If you update parse_method, chunk_count must be 0. 
# "naive" means general.
curl --request PUT \
  --url http://{address}/api/v1/dataset/{dataset_id} \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
  "name": "test",
  "tenant_id": "4fb0cd625f9311efba4a0242ac120006",
  "embedding_model": "BAAI/bge-zh-v1.5",
  "chunk_count": 0,
  "document_count": 0,
  "parse_method": "naive"
}'
```

#### Request parameters
(Refer to the "Create Dataset" for the complete structure of the request parameters.)


### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0 
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "Can't change tenant_id."
}
```

## List datasets

**GET** `/api/v1/dataset?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`

List all datasets

### Request

- Method: GET
- URL: `http://{address}/api/v1/dataset?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`
- Headers:
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'


#### Request example

```bash
# If no page parameter is passed, the default is 1
# If no page_size parameter is passed, the default is 1024
# If no order_by parameter is passed, the default is "create_time"
# If no desc parameter is passed, the default is True
curl --request GET \
  --url http://{address}/api/v1/dataset?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id} \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
```

#### Request parameters

- `path`: (*Path parameter*)
    The current page number to retrieve from the paginated data. This parameter determines which set of records will be fetched.
- `path_size`: (*Path parameter*)
    The number of records to retrieve per page. This controls how many records will be included in each page. 
- `orderby`: (*Path parameter*)
    The field by which the records should be sorted. This specifies the attribute or column used to order the results.
- `desc`: (*Path parameter*)
    A boolean flag indicating whether the sorting should be in descending order.
- `name`: (*Path parameter*)
    Dataset name
- `"id"`: (*Path parameter*)  
    The ID of the dataset to be retrieved.
- `"name"`: (*Path parameter*)  
    The name of the dataset to be retrieved.

### Response

The successful response includes a JSON object like the following:

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
            "parse_method": "knowledge_graph",
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

  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "The dataset doesn't exist"
}
```

## Upload files to a dataset

**POST** `/api/v1/dataset/{dataset_id}/document`

Uploads files to a dataset. 

### Request

- Method: POST
- URL: `/api/v1/dataset/{dataset_id}/document`
- Headers:
  - 'Content-Type: multipart/form-data'
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Form:
  - 'file=@{FILE_PATH}'

#### Request example

```bash
curl --request POST \
     --url http://{address}/api/v1/dataset/{dataset_id}/document \
     --header 'Content-Type: multipart/form-data' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \     
     --form 'file=@./test.txt'
```

#### Request parameters

- `"dataset_id"`: (*Path parameter*)
    The dataset id
- `"file"`: (*Body parameter*)  
    The file to upload

### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0 
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```json
{
    "code": 101,
    "message": "No file part!"
}
```

## Delete files from a dataset

**DELETE** `/api/v1/dataset/{dataset_id}/document `

Delete files from a dataset

### Request

- Method: DELETE
- URL: `http://{address}/api/v1/dataset/{dataset_id}/document`
- Headers:
  - 'Content-Type: application/json'
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `ids`:List[str]
#### Request example

```bash
curl --request DELETE \
  --url http://{address}/api/v1/dataset/{dataset_id}/document \
  --header 'Content-Type: application/json' \
  --header 'Authorization: {YOUR ACCESS TOKEN}' \
  --data '{
  "ids": ["id_1","id_2"]
  }'
```

#### Request parameters

- `"ids"`: (*Body parameter*)
    The ids of teh documents to be deleted
### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0
}.
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "You do not own the dataset 7898da028a0511efbf750242ac1220005."
}
```

## Download a file from a dataset

**GET** `/api/v1/dataset/{dataset_id}/document/{document_id}`

Downloads a file from a dataset. 

### Request

- Method: GET
- URL: `http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}`
- Headers:
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Output:
  - '{FILE_NAME}'
#### Request example

```bash
curl --request GET \
  --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id} \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --output ./ragflow.txt
```

#### Request parameters

- `"dataset_id"`: (*PATH parameter*)
    The dataset id
- `"documents_id"`: (*PATH parameter*)  
    The document id of the file.

### Response

The successful response includes a text object like the following:

```text
test_2.
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "You do not own the dataset 7898da028a0511efbf750242ac1220005."
}
```


## List files of a dataset

**GET** `/api/v1/dataset/{dataset_id}/info?offset={offset}&limit={limit}&orderby={orderby}&desc={desc}&keywords={keywords}&id={document_id}`

List files to a dataset. 

### Request

- Method: GET
- URL: `/api/v1/dataset/{dataset_id}/info?keywords={keyword}&page={page}&page_size={limit}&orderby={orderby}&desc={desc}&name={name`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```bash
curl --request GET \
  --url http://{address}/api/v1/dataset/{dataset_id}/info?offset={offset}&limit={limit}&orderby={orderby}&desc={desc}&keywords={keywords}&id={document_id} \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
```

#### Request parameters

- `"dataset_id"`: (*PATH parameter*)
    The dataset id
- `offset`: (*Filter parameter*)
    The beginning number of records for paging.
- `keywords`: (*Filter parameter*)
    The keywords matches the search key workds;
- `limit`: (*Filter parameter*)
    Records number to return.
- `orderby`: (*Filter parameter*)
    The field by which the records should be sorted. This specifies the attribute or column used to order the results.
- `desc`: (*Filter parameter*)
    A boolean flag indicating whether the sorting should be in descending order.
- `id`: (*Filter parameter*)
    The id of the document to be got.

### Response

The successful response includes a JSON object like the following:

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

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "You don't own the dataset 7898da028a0511efbf750242ac1220005. "
}
```

## Update a file information in dataset

**PUT** `/api/v1/dataset/{dataset_id}/info/{document_id}`

Update a file in a dataset

### Request

- Method: PUT
- URL: `http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `name`:`string`
  - `parser_method`:`string`
  - `parser_config`:`dict`
#### Request example

```bash
curl --request PUT \
  --url http://{address}/api/v1/dataset/{dataset_id}/info/{document_id} \
  --header 'Authorization: Bearer {YOUR_ACCESS TOKEN}' \
  --header 'Content-Type: application/json' \
  --data '{
  "name": "manual.txt", 
  "parser_method": "manual", 
  "parser_config": {"chunk_token_count": 128, "delimiter": "\n!?。；！？", "layout_recognize": true, "task_page_size": 12}
  }'

```

#### Request parameters

- `"parser_method"`: (*Body parameter*)  
    Method used to parse the document.  


- `"parser_config"`: (*Body parameter*)  
    Configuration object for the parser.  
    - If the value is `None`, a dictionary with default values will be generated.

- `"name"`: (*Body parameter*)  
    Name or title of the document.  




### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0
}
```
  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "The dataset not own the document."
}
```

## Parse files in dataset

**POST** `/api/v1/dataset/{dataset_id}/chunk`

Parse files into chunks in a dataset

### Request

- Method: POST
- URL: `http://{address}/api/v1/dataset/{dataset_id}/chunk `
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `document_ids`:List[str]

#### Request example

```bash
curl --request POST \
    --url http://{address}/api/v1/dataset/{dataset_id}/chunk \
    --header 'Content-Type: application/json' \
    --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
    --data '{"document_ids": ["97a5f1c2759811efaa500242ac120004","97ad64b6759811ef9fc30242ac120004"]}'
```

#### Request parameters

- `"dataset_id"`: (*Path parameter*)
- `"document_ids"`:(*Body parameter*)  
  The ids of the documents to be parsed

### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0
}
```
  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "`document_ids` is required"
}
```

## Stop file parsing

**DELETE** `/api/v1/dataset/{dataset_id}/chunk`

Stop file parsing

### Request

- Method: DELETE
- URL: `http://{address}/api/v1/dataset/{dataset_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `document_ids`:List[str]
#### Request example

```bash
curl --request DELETE \
   --url http://{address}/api/v1/dataset/{dataset_id}/chunk \
   --header 'Content-Type: application/json' \
   --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
   --data '{"document_ids": ["97a5f1c2759811efaa500242ac120004","97ad64b6759811ef9fc30242ac120004"]}'
```

#### Request parameters

- `"dataset_id"`: (*Path parameter*)
- `"document_ids"`:(*Body parameter*)  
  The ids of the documents to be parsed


### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0
}
```
  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "`document_ids` is required"
}
```

## Get document chunk list

**GET** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk?keywords={keywords}&offset={offset}&limit={limit}&id={id}`

Get document chunk list

### Request

- Method: GET
- URL: `http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk?keywords={keywords}&offset={offset}&limit={limit}&id={id}`
- Headers:
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```bash
curl --request GET \
  --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk?keywords={keywords}&offset={offset}&limit={limit}&id={id} \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' 
```

#### Request parameters

- `"dataset_id"`: (*Path parameter*)
- `"document_id"`: (*Path parameter*)
- `"offset"`(*Filter parameter*)  
  The beginning number of records for paging.
- `"keywords"`(*Filter parameter*)  
  List chunks whose name has the given keywords
- `"limit"`(*Filter parameter*)  
  Records number to return
- `"id"`(*Filter parameter*)  
  The id of chunk to be got
### Response

The successful response includes a JSON object like the following:

```json
{
    "code": 0,
    "data": {
        "chunks": [],
        "doc": {
            "chunk_num": 0,
            "create_date": "Sun, 29 Sep 2024 03:47:29 GMT",
            "create_time": 1727581649216,
            "created_by": "69736c5e723611efb51b0242ac120007",
            "id": "8cb781ec7e1511ef98ac0242ac120006",
            "kb_id": "c7ee74067a2c11efb21c0242ac120006",
            "location": "明天的天气是晴天.txt",
            "name": "明天的天气是晴天.txt",
            "parser_config": {
                "pages": [
                    [
                        1,
                        1000000
                    ]
                ]
            },
            "parser_id": "naive",
            "process_begin_at": "Tue, 15 Oct 2024 10:23:51 GMT",
            "process_duation": 1435.37,
            "progress": 0.0370833,
            "progress_msg": "\nTask has been received.",
            "run": "1",
            "size": 24,
            "source_type": "local",
            "status": "1",
            "thumbnail": null,
            "token_num": 0,
            "type": "doc",
            "update_date": "Tue, 15 Oct 2024 10:47:46 GMT",
            "update_time": 1728989266371
        },
        "total": 0
    }
}
```
  
The error response includes a JSON object like the following:

```json
{
    "code": 102,
    "message": "You don't own the document 5c5999ec7be811ef9cab0242ac12000e5."
}
```

## Delete document chunks

**DELETE** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`

Delete document chunks

### Request

- Method: DELETE
- URL: `http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `chunk_ids`:List[str]

#### Request example

```bash
curl --request DELETE \
  --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
  "chunk_ids": ["test_1", "test_2"]
  }'
```
#### Request parameters

- `"chunk_ids"`:(*Body parameter*)
  The chunks of the document to be deleted

### Response
Success
```json
{
    "code": 0
}
```
Error
```json
{
    "code": 102,
    "message": "`chunk_ids` is required"
}
```


## Update document chunk

**PUT** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk/{chunk_id}`

Update document chunk

### Request

- Method: PUT
- URL: `http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk/{chunk_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `content`:str
  - `important_keywords`:str
  - `available`:int
#### Request example

```bash
curl --request PUT \
  --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk/{chunk_id} \
  --header 'Content-Type: application/json' \
  --header 'Authorization: {YOUR_ACCESS_TOKEN}' \
  --data '{   
    "content": "ragflow123",  
    "important_keywords": [],   
}'
```
#### Request parameters
- `"content"`:(*Body parameter*)
  Contains the main text or information of the chunk.
- `"important_keywords"`:(*Body parameter*)
  list the key terms or phrases that are significant or central to the chunk's content.
- `"available"`:(*Body parameter*)
   Indicating the availability status, 0 means unavailable and 1 means available.

### Response
Success
```json
{
    "code": 0
}
```
Error
```json
{
    "code": 102,
    "message": "Can't find this chunk 29a2d9987e16ba331fb4d7d30d99b71d2"
}
```
## Insert document chunks

**POST** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`

Insert document chunks

### Request

- Method: POST
- URL: `http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `content`: str
  - `important_keywords`:List[str]
#### Request example

```bash
curl --request POST \
  --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
    "content": "ragflow content"
}'
```
#### Request parameters
- `content`:(*Body parameter*)  
  Contains the main text or information of the chunk.
- `important_keywords`(*Body parameter*)  
  list the key terms or phrases that are significant or central to the chunk's content.

### Response
Success
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

Error
```json
{
    "code": 102,
    "message": "`content` is required"
}
```
## Dataset retrieval test

**GET** `/api/v1/retrieval`

Retrieval test of a dataset

### Request

- Method: POST
- URL: `http://{address}/api/v1/retrieval`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `question`: str  
  - `datasets`: List[str]  
  - `documents`: List[str]
  - `offset`: int  
  - `limit`: int  
  - `similarity_threshold`: float  
  - `vector_similarity_weight`: float  
  - `top_k`: int  
  - `rerank_id`: string  
  - `keyword`: bool  
  - `highlight`: bool
#### Request example

```bash
curl --request POST \
  --url http://{address}/api/v1/retrieval \
  --header 'Content-Type: application/json' \
  --header 'Authorization: {YOUR_ACCESS_TOKEN}' \
  --data '{
    "question": "What is advantage of ragflow?",
    "datasets": [
        "b2a62730759d11ef987d0242ac120004"
    ],
    "documents": [
        "77df9ef4759a11ef8bdd0242ac120004"
    ]
}'
```

#### Request parameter
- `"question"`: (*Body parameter*)  
  User's question, search keywords  
  `""`
- `"datasets"`: (*Body parameter*)  
  The scope of datasets  
  `None`
- `"documents"`: (*Body parameter*)  
  The scope of document. `None` means no limitation  
  `None`
- `"offset"`: (*Body parameter*)  
  The beginning point of retrieved records  
  `1`

- `"limit"`: (*Body parameter*)  
  The maximum number of records needed to return  
  `30`

- `"similarity_threshold"`: (*Body parameter*)  
  The minimum similarity score  
  `0.2`

- `"vector_similarity_weight"`: (*Body parameter*)  
  The weight of vector cosine similarity, `1 - x` is the term similarity weight  
  `0.3`

- `"top_k"`: (*Body parameter*)  
  Number of records engaged in vector cosine computation  
  `1024`

- `"rerank_id"`: (*Body parameter*)  
  ID of the rerank model  
  `None`

- `"keyword"`: (*Body parameter*)  
  Whether keyword-based matching is enabled  
  `False`

- `"highlight"`: (*Body parameter*)  
  Whether to enable highlighting of matched terms in the results  
  `False`
### Response
Success
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
Error
```json
{
    "code": 102,
    "message": "`datasets` is required."
}
```
## Create chat

**POST** `/api/v1/chat`

Create a chat

### Request

- Method: POST
- URL: `http://{address}/api/v1/chat`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `"name"`: `string`
  - `"avatar"`: `string`
  - `"knowledgebases"`: `List[DataSet]`
  - `"id"`: `string`
  - `"llm"`: `LLM`
  - `"prompt"`: `Prompt`


#### Request example

```shell
curl --request POST \
     --url http://{address}/api/v1/chat \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --data-binary '{
   "knowledgebases": [
    {
      "avatar": null,
      "chunk_count": 0,
      "description": null,
      "document_count": 0,
      "embedding_model": "",
      "id": "0b2cbc8c877f11ef89070242ac120005",
      "language": "English",
      "name": "Test_assistant",
      "parse_method": "naive",
      "parser_config": {
        "pages": [
          [
            1,
            1000000
          ]
        ]
      },
      "permission": "me",
      "tenant_id": "4fb0cd625f9311efba4a0242ac120006"
    }
  ],
    "name":"new_chat_1"
}'
```

#### Request parameters

- `"name"`: (*Body parameter*)  
    The name of the created chat.  
    - `"assistant"`

- `"avatar"`: (*Body parameter*)  
    The icon of the created chat.  
    - `"path"`

- `"knowledgebases"`: (*Body parameter*)  
    Select knowledgebases associated.  
    - `["kb1"]`

- `"id"`: (*Body parameter*)  
    The id of the created chat.  
    - `""`

- `"llm"`: (*Body parameter*)  
    The LLM of the created chat.  
    - If the value is `None`, a dictionary with default values will be generated.

- `"prompt"`: (*Body parameter*)  
    The prompt of the created chat.  
    - If the value is `None`, a dictionary with default values will be generated.

---

##### Chat.LLM parameters:

- `"model_name"`: (*Body parameter*)  
    Large language chat model.  
    - If it is `None`, it will return the user's default model.

- `"temperature"`: (*Body parameter*)  
    Controls the randomness of predictions by the model. A lower temperature makes the model more confident, while a higher temperature makes it more creative and diverse.  
    - `0.1`

- `"top_p"`: (*Body parameter*)  
    Also known as "nucleus sampling," it focuses on the most likely words, cutting off the less probable ones.  
    - `0.3`

- `"presence_penalty"`: (*Body parameter*)  
    Discourages the model from repeating the same information by penalizing repeated content.  
    - `0.4`

- `"frequency_penalty"`: (*Body parameter*)  
    Reduces the model’s tendency to repeat words frequently.  
    - `0.7`

- `"max_tokens"`: (*Body parameter*)  
    Sets the maximum length of the model’s output, measured in tokens (words or pieces of words).  
    - `512`

---

##### Chat.Prompt parameters:

- `"similarity_threshold"`: (*Body parameter*)  
    Filters out chunks with similarity below this threshold.  
    - `0.2`

- `"keywords_similarity_weight"`: (*Body parameter*)  
    Weighted keywords similarity and vector cosine similarity; the sum of weights is 1.0.  
    - `0.7`

- `"top_n"`: (*Body parameter*)  
    Only the top N chunks above the similarity threshold will be fed to LLMs.  
    - `8`

- `"variables"`: (*Body parameter*)  
    Variables help with different chat strategies by filling in the 'System' part of the prompt.  
    - `[{"key": "knowledge", "optional": True}]`

- `"rerank_model"`: (*Body parameter*)  
    If empty, it uses vector cosine similarity; otherwise, it uses rerank score.  
    - `""`

- `"empty_response"`: (*Body parameter*)  
    If nothing is retrieved, this will be used as the response. Leave blank if LLM should provide its own opinion.  
    - `None`

- `"opener"`: (*Body parameter*)  
    The welcome message for clients.  
    - `"Hi! I'm your assistant, what can I do for you?"`

- `"show_quote"`: (*Body parameter*)  
    Indicates whether the source of the original text should be displayed.  
    - `True`

- `"prompt"`: (*Body parameter*)  
    Instructions for LLM to follow when answering questions, such as character design or answer length.  
    - `"You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence 'The answer you are looking for is not found in the knowledge base!' Answers need to consider chat history. Here is the knowledge base: {knowledge} The above is the knowledge base."`
### Response
Success:
```json
{
    "code": 0,
    "data": {
        "avatar": "",
        "create_date": "Fri, 11 Oct 2024 03:23:24 GMT",
        "create_time": 1728617004635,
        "description": "A helpful Assistant",
        "do_refer": "1",
        "id": "2ca4b22e878011ef88fe0242ac120005",
        "knowledgebases": [
            {
                "avatar": null,
                "chunk_count": 0,
                "description": null,
                "document_count": 0,
                "embedding_model": "",
                "id": "0b2cbc8c877f11ef89070242ac120005",
                "language": "English",
                "name": "Test_assistant",
                "parse_method": "naive",
                "parser_config": {
                    "pages": [
                        [
                            1,
                            1000000
                        ]
                    ]
                },
                "permission": "me",
                "tenant_id": "4fb0cd625f9311efba4a0242ac120006"
            }
        ],
        "language": "English",
        "llm": {
            "frequency_penalty": 0.7,
            "max_tokens": 512,
            "model_name": "deepseek-chat___OpenAI-API@OpenAI-API-Compatible",
            "presence_penalty": 0.4,
            "temperature": 0.1,
            "top_p": 0.3
        },
        "name": "new_chat_1",
        "prompt": {
            "empty_response": "Sorry! 知识库中未找到相关内容！",
            "keywords_similarity_weight": 0.3,
            "opener": "您好，我是您的助手小樱，长得可爱又善良，can I help you?",
            "prompt": "你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。\n            以下是知识库：\n            {knowledge}\n            以上是知识库。",
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
        "update_date": "Fri, 11 Oct 2024 03:23:24 GMT",
        "update_time": 1728617004635
    }
}
```
Error:
```json
{
    "code": 102,
    "message": "Duplicated chat name in creating dataset."
}
```

## Update chat

**PUT** `/api/v1/chat/{chat_id}`

Update a chat

### Request

- Method: PUT
- URL: `http://{address}/api/v1/chat/{chat_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body: (Refer to the "Create chat" for the complete structure of the request body.)
  
#### Request example
```bash
curl --request PUT \
  --url http://{address}/api/v1/chat/{chat_id} \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
    "name":"Test"
}'
```
#### Parameters
(Refer to the "Create chat" for the complete structure of the request parameters.)

### Response
Success
```json
{
    "code": 0
}
```
Error
```json
{
    "code": 102,
    "message": "Duplicated chat name in updating dataset."
}
```

## Delete chats

**DELETE** `/api/v1/chat`

Delete chats

### Request

- Method: DELETE
- URL: `http://{address}/api/v1/chat`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `ids`: List[string]
#### Request example
```bash
# Either id or name must be provided, but not both.
curl --request DELETE \
  --url http://{address}/api/v1/chat \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
  "ids": ["test_1", "test_2"]
  }'
}'
```
#### Request parameters:

- `"ids"`: (*Body parameter*)  
    IDs of the chats to be deleted.  
    - `None`
### Response
Success
```json
{
    "code": 0
}
```
Error
```json
{
    "code": 102,
    "message": "ids are required"
}
```

## List chats

**GET** `/api/v1/chat?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`

List chats based on filter criteria.

### Request

- Method: GET
- URL: `http://{address}/api/v1/chat?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`
- Headers:
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```bash
curl --request GET \
  --url http://{address}/api/v1/chat?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id} \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
```

#### Request parameters
- `"page"`: (*Path parameter*)  
    The current page number to retrieve from the paginated data. This parameter determines which set of records will be fetched.  
    - `1`

- `"page_size"`: (*Path parameter*)  
    The number of records to retrieve per page. This controls how many records will be included in each page.  
    - `1024`

- `"orderby"`: (*Path parameter*)  
    The field by which the records should be sorted. This specifies the attribute or column used to order the results.  
    - `"create_time"`

- `"desc"`: (*Path parameter*)  
    A boolean flag indicating whether the sorting should be in descending order.  
    - `True`

- `"id"`: (*Path parameter*)  
    The ID of the chat to be retrieved.  
    - `None`

- `"name"`: (*Path parameter*)  
    The name of the chat to be retrieved.  
    - `None`

### Response
Success
```json
{
    "code": 0,
    "data": [
        {
            "avatar": "",
            "create_date": "Fri, 11 Oct 2024 03:23:24 GMT",
            "create_time": 1728617004635,
            "description": "A helpful Assistant",
            "do_refer": "1",
            "id": "2ca4b22e878011ef88fe0242ac120005",
            "knowledgebases": [
                {
                    "avatar": "",
                    "chunk_num": 0,
                    "create_date": "Fri, 11 Oct 2024 03:15:18 GMT",
                    "create_time": 1728616518986,
                    "created_by": "69736c5e723611efb51b0242ac120007",
                    "description": "",
                    "doc_num": 0,
                    "embd_id": "BAAI/bge-large-zh-v1.5",
                    "id": "0b2cbc8c877f11ef89070242ac120005",
                    "language": "English",
                    "name": "test_delete_chat",
                    "parser_config": {
                        "chunk_token_count": 128,
                        "delimiter": "\n!?。；！？",
                        "layout_recognize": true,
                        "task_page_size": 12
                    },
                    "parser_id": "naive",
                    "permission": "me",
                    "similarity_threshold": 0.2,
                    "status": "1",
                    "tenant_id": "69736c5e723611efb51b0242ac120007",
                    "token_num": 0,
                    "update_date": "Fri, 11 Oct 2024 04:01:31 GMT",
                    "update_time": 1728619291228,
                    "vector_similarity_weight": 0.3
                }
            ],
            "language": "English",
            "llm": {
                "frequency_penalty": 0.7,
                "max_tokens": 512,
                "model_name": "deepseek-chat___OpenAI-API@OpenAI-API-Compatible",
                "presence_penalty": 0.4,
                "temperature": 0.1,
                "top_p": 0.3
            },
            "name": "Test",
            "prompt": {
                "empty_response": "Sorry! 知识库中未找到相关内容！",
                "keywords_similarity_weight": 0.3,
                "opener": "您好，我是您的助手小樱，长得可爱又善良，can I help you?",
                "prompt": "你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。\n            以下是知识库：\n            {knowledge}\n            以上是知识库。",
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
            "update_date": "Fri, 11 Oct 2024 03:47:58 GMT",
            "update_time": 1728618478392
        }
    ]
}
```
Error
```json
{
    "code": 102,
    "message": "The chat doesn't exist"
}
```

## Create a chat session

**POST** `/api/v1/chat/{chat_id}/session`

Create a chat session

### Request

- Method: POST
- URL: `http://{address}/api/v1/chat/{chat_id}/session`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' 
- Body:
  - name: `string`

#### Request example
```bash
curl --request POST \
  --url http://{address}/api/v1/chat/{chat_id}/session \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
    "name": "new session"
  }'
```
#### Request parameters
- `"id"`: (*Body parameter*)  
    The ID of the created session used to identify different sessions.  
    - `None`  
    - `id` cannot be provided when creating.

- `"name"`: (*Body parameter*)  
    The name of the created session.  
    - `"New session"`

- `"messages"`: (*Body parameter*)  
    The messages of the created session.  
    - `[{"role": "assistant", "content": "Hi! I am your assistant, can I help you?"}]`  
    - `messages` cannot be provided when creating.

- `"chat_id"`: (*Path parameter*)  
    The ID of the associated chat.  
    - `""`  
    - `chat_id` cannot be changed.

### Response
Success
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
Error
```json
{
    "code": 102,
    "message": "Name can not be empty."
}
```

## List the sessions of a chat

**GET** `/api/v1/chat/{chat_id}/session?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`

List all sessions under the chat based on the filtering criteria.

### Request

- Method: GET
- URL: `http://{address}/api/v1/chat/{chat_id}/session?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id}`
- Headers:
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example
```bash
curl --request GET \
  --url http://{address}/api/v1/chat/{chat_id}/session?page={page}&page_size={page_size}&orderby={orderby}&desc={desc}&name={dataset_name}&id={dataset_id} \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
```

#### Request Parameters
- `"page"`: (*Path parameter*)  
    The current page number to retrieve from the paginated data. This parameter determines which set of records will be fetched.  
    - `1`

- `"page_size"`: (*Path parameter*)  
    The number of records to retrieve per page. This controls how many records will be included in each page.  
    - `1024`

- `"orderby"`: (*Path parameter*)  
    The field by which the records should be sorted. This specifies the attribute or column used to order the results.  
    - `"create_time"`

- `"desc"`: (*Path parameter*)  
    A boolean flag indicating whether the sorting should be in descending order.  
    - `True`

- `"id"`: (*Path parameter*)  
    The ID of the session to be retrieved.  
    - `None`

- `"name"`: (*Path parameter*)  
    The name of the session to be retrieved.  
    - `None`
### Response
Success
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
Error
```json
{
    "code": 102,
    "message": "The session doesn't exist"
}
```


## Delete chat sessions

**DELETE** `/api/v1/chat/{chat_id}/session`

Delete chat sessions

### Request

- Method: DELETE
- URL: `http://{address}/api/v1/chat/{chat_id}/session`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `ids`: List[string]

#### Request example
```bash
# Either id or name must be provided, but not both.
curl --request DELETE \
--url http://{address}/api/v1/chat/{chat_id}/session \
--header 'Content-Type: application/json' \
--header 'Authorization: Bear {YOUR_ACCESS_TOKEN}' \
  --data '{
  "ids": ["test_1", "test_2"]
  }'
```

#### Request Parameters
- `ids`: (*Body Parameter*)  
    IDs of the sessions to be deleted.
    - `None`
### Response
Success
```json
{
    "code": 0
}
```
Error
```json
{
    "code": 102,
    "message": "The chat doesn't own the session"
}
```
## Update a chat session

**PUT** `/api/v1/chat/{chat_id}/session/{session_id}`

Update a chat session

### Request

- Method: PUT
- URL: `http://{address}/api/v1/chat/{chat_id}/session/{session_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `name`: string

#### Request example
```bash
curl --request PUT \
  --url http://{address}/api/v1/chat/{chat_id}/session/{session_id} \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data '{
    "name": "Updated session"
  }'

```

#### Request Parameter
- `name`:(*Body Parameter)  
    The name of the created session.
    - `None`

### Response
Success
```json
{
    "code": 0
}
```
Error
```json
{
    "code": 102,
    "message": "Name can not be empty."
}
```

## Chat with a chat session

**POST** `/api/v1/chat/{chat_id}/completion`

Chat with a chat session

### Request

- Method: POST
- URL: `http://{address} /api/v1/chat/{chat_id}/completion`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `question`: string
  - `stream`: bool
  - `session_id`: str


#### Request example
```bash
curl --request POST \
  --url http://{address} /api/v1/chat/{chat_id}/completion \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data-binary '{
    "question":  "你好!",
    "stream": true
  }'
```
#### Request Parameters
- `question`:(*Body Parameter*)  
    The question you want to ask.
    - question is required.
    `None`
- `stream`: (*Body Parameter*)  
    The approach of streaming text generation.
    `False`
- `session_id`: (*Body Parameter*)  
    The id of session.If not provided, a new session will be generated.
### Response
Success
```json
data: {
    "code": 0,
    "data": {
        "answer": "您好！有什么具体的问题或者需要的帮助",
        "reference": {},
        "audio_binary": null,
        "id": "31153052-7bac-4741-a513-ed07d853f29e"
    }
}

data: {
    "code": 0,
    "data": {
        "answer": "您好！有什么具体的问题或者需要的帮助可以告诉我吗？我在这里是为了帮助",
        "reference": {},
        "audio_binary": null,
        "id": "31153052-7bac-4741-a513-ed07d853f29e"
    }
}

data: {
    "code": 0,
    "data": {
        "answer": "您好！有什么具体的问题或者需要的帮助可以告诉我吗？我在这里是为了帮助您的。如果您有任何疑问或是需要获取",
        "reference": {},
        "audio_binary": null,
        "id": "31153052-7bac-4741-a513-ed07d853f29e"
    }
}

data: {
    "code": 0,
    "data": {
        "answer": "您好！有什么具体的问题或者需要的帮助可以告诉我吗？我在这里是为了帮助您的。如果您有任何疑问或是需要获取某些信息，请随时提出。",
        "reference": {},
        "audio_binary": null,
        "id": "31153052-7bac-4741-a513-ed07d853f29e"
    }
}

data: {
    "code": 0,
    "data": {
        "answer": "您好！有什么具体的问题或者需要的帮助可以告诉我吗 ##0$$？我在这里是为了帮助您的。如果您有任何疑问或是需要获取某些信息，请随时提出。",
        "reference": {
            "total": 19,
            "chunks": [
                {
                    "chunk_id": "9d87f9d70a0d8a7565694a81fd4c5d5f",
                    "content_ltks": "当所有知识库内容都与问题无关时 ,你的回答必须包括“知识库中未找到您要的答案!”这句话。回答需要考虑聊天历史。\r\n以下是知识库:\r\n{knowledg}\r\n以上是知识库\r\n\"\"\"\r\n 1\r\n 2\r\n 3\r\n 4\r\n 5\r\n 6\r\n总结\r\n通过上面的介绍,可以对开源的 ragflow有了一个大致的了解,与前面的有道qanyth整体流程还是比较类似的。 ",
                    "content_with_weight": "当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。\r\n    以下是知识库：\r\n    {knowledge}\r\n    以上是知识库\r\n\"\"\"\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n总结\r\n通过上面的介绍，可以对开源的 RagFlow 有了一个大致的了解，与前面的 有道 QAnything 整体流程还是比较类似的。",
                    "doc_id": "5c5999ec7be811ef9cab0242ac120005",
                    "docnm_kwd": "1.txt",
                    "kb_id": "c7ee74067a2c11efb21c0242ac120006",
                    "important_kwd": [],
                    "img_id": "",
                    "similarity": 0.38337178633282265,
                    "vector_similarity": 0.3321336754679629,
                    "term_similarity": 0.4053309767034769,
                    "positions": [
                        ""
                    ]
                },
                {
                    "chunk_id": "895d34de762e674b43e8613c6fb54c6d",
                    "content_ltks": "\r\n\r\n实际内容可能会超过大模型的输入token数量,因此在调用大模型前会调用api/db/servic/dialog_service.py文件中 messag_fit_in ()根据大模型可用的 token数量进行过滤。这部分与有道的 qanyth的实现大同小异,就不额外展开了。\r\n\r\n将检索的内容,历史聊天记录以及问题构造为 prompt ,即可作为大模型的输入了 ,默认的英文prompt如下所示:\r\n\r\n\"\"\"\r\nyou are an intellig assistant. pleas summar the content of the knowledg base to answer the question. pleas list thedata in the knowledg base and answer in detail. when all knowledg base content is irrelev to the question , your answer must includ the sentenc\"the answer you are lookfor isnot found in the knowledg base!\" answer needto consid chat history.\r\n here is the knowledg base:\r\n{ knowledg}\r\nthe abov is the knowledg base.\r\n\"\"\"\r\n1\r\n 2\r\n 3\r\n 4\r\n 5\r\n 6\r\n对应的中文prompt如下所示:\r\n\r\n\"\"\"\r\n你是一个智能助手,请总结知识库的内容来回答问题,请列举知识库中的数据详细回答。 ",
                    "content_with_weight": "\r\n\r\n实际内容可能会超过大模型的输入 token 数量，因此在调用大模型前会调用 api/db/services/dialog_service.py 文件中 message_fit_in() 根据大模型可用的 token 数量进行过滤。这部分与有道的 QAnything 的实现大同小异，就不额外展开了。\r\n\r\n将检索的内容，历史聊天记录以及问题构造为 prompt，即可作为大模型的输入了，默认的英文 prompt 如下所示：\r\n\r\n\"\"\"\r\nYou are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence \"The answer you are looking for is not found in the knowledge base!\" Answers need to consider chat history.\r\n      Here is the knowledge base:\r\n      {knowledge}\r\n      The above is the knowledge base.\r\n\"\"\"\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n对应的中文 prompt 如下所示：\r\n\r\n\"\"\"\r\n你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。",
                    "doc_id": "5c5999ec7be811ef9cab0242ac120005",
                    "docnm_kwd": "1.txt",
                    "kb_id": "c7ee74067a2c11efb21c0242ac120006",
                    "important_kwd": [],
                    "img_id": "",
                    "similarity": 0.2788204323926715,
                    "vector_similarity": 0.35489427679953667,
                    "term_similarity": 0.2462173562183008,
                    "positions": [
                        ""
                    ]
                }
            ],
            "doc_aggs": [
                {
                    "doc_name": "1.txt",
                    "doc_id": "5c5999ec7be811ef9cab0242ac120005",
                    "count": 2
                }
            ]
        },
        "prompt": "你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。\n            以下是知识库：\n            当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。\r\n    以下是知识库：\r\n    {knowledge}\r\n    以上是知识库\r\n\"\"\"\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n总结\r\n通过上面的介绍，可以对开源的 RagFlow 有了一个大致的了解，与前面的 有道 QAnything 整体流程还是比较类似的。\n\n------\n\n\r\n\r\n实际内容可能会超过大模型的输入 token 数量，因此在调用大模型前会调用 api/db/services/dialog_service.py 文件中 message_fit_in() 根据大模型可用的 token 数量进行过滤。这部分与有道的 QAnything 的实现大同小异，就不额外展开了。\r\n\r\n将检索的内容，历史聊天记录以及问题构造为 prompt，即可作为大模型的输入了，默认的英文 prompt 如下所示：\r\n\r\n\"\"\"\r\nYou are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence \"The answer you are looking for is not found in the knowledge base!\" Answers need to consider chat history.\r\n      Here is the knowledge base:\r\n      {knowledge}\r\n      The above is the knowledge base.\r\n\"\"\"\r\n1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n对应的中文 prompt 如下所示：\r\n\r\n\"\"\"\r\n你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。\n            以上是知识库。\n\n### Query:\n你好，请问有什么问题需要我帮忙解答吗？\n\n### Elapsed\n  - Retrieval: 9131.1 ms\n  - LLM: 12802.6 ms",
        "id": "31153052-7bac-4741-a513-ed07d853f29e"
    }
}

data:{
    "code": 0,
    "data": true
}
```
Error
```json
{
    "code": 102,
    "message": "Please input your question."
}
```