
# HTTP API Reference

## Create dataset

**POST** `/api/v1/dataset`

Creates a dataset with a name. If dataset of the same name already exists, the new dataset will be renamed by RAGFlow automatically.

### Request

- Method: POST
- URL: `/api/v1/dataset`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Body:
  - `"dataset_name"`: `string`
  - `"tenant_id"`: `string`
  - `"embedding_model"`: `string`
  - `"chunk_count"`: `integer`
  - `"document_count"`: `integer`
  - `"parse_method"`: `string`

#### Request example

```shell
curl --request POST \
     --url http://{address}/api/v1/dataset \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
     --data-binary '{
     "dataset_name": "test",
     "tenant_id": "4fb0cd625f9311efba4a0242ac120006",
     "embedding_model": "BAAI/bge--zh-v1.5",
     "chunk_count": 0,
     "document_count": 0,
     "parse_method": "general"
}'
```

#### Request parameters

- `"dataset_name"`: (*Body parameter*)
    The name of the dataset, which must adhere to the following requirements:  
    - Maximum 65,535 characters.
- `"tenant_id"`: (*Body parameter*)  
    The ID of the tenant.
- `"embedding_model"`: (*Body parameter*)  
    Embedding model used in the dataset.
- `"chunk_count"`: (*Body parameter*)  
    Chunk count of the dataset.
- `"document_count"`: (*Body parameter*)  
    Document count of the dataset.
- `"parse_mehtod"`: (*Body parameter*)  
    Parsing method of the dataset.

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0 
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't connect database"
}
```

## Delete dataset

**DELETE** `/api/v1/dataset`

Deletes a dataset by its id or name.

### Request

- Method: DELETE
- URL: `/api/v1/dataset/{dataset_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'


#### Request example

```shell
curl --request DELETE \
     --url http://{address}/api/v1/dataset/0 \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --data ' {
        "names": ["ds1", "ds2"]
     }'
```

#### Request parameters

- `"names"`: (*Body parameter*)
    Dataset names to delete.
- `"ids"`: (*Body parameter*)
    Dataset IDs to delete.

`"names"` and `"ids"` are exclusive.

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0 
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Try to delete non-existent dataset."
}
```

## Update dataset

**PUT** `/api/v1/dataset/{dataset_id}`

Updates a dataset by its id.

### Request

- Method: PUT
- URL: `/api/v1/dataset/{dataset_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'


#### Request example

```shell
curl --request PUT \
     --url http://{address}/api/v1/dataset/0 \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --data-binary '{
     "dataset_name": "test",
     "tenant_id": "4fb0cd625f9311efba4a0242ac120006",
     "embedding_model": "BAAI/bge--zh-v1.5",
     "chunk_count": 0,
     "document_count": 0,
     "parse_method": "general"
}'
```

#### Request parameters

- `"dataset_name"`: (*Body parameter*)
    The name of the dataset, which must adhere to the following requirements:  
    - Maximum 65,535 characters.
- `"tenant_id"`: (*Body parameter*)  
    The ID of the tenant.
- `"embedding_model"`: (*Body parameter*)  
    Embedding model used in the dataset.
- `"chunk_count"`: (*Body parameter*)  
    Chunk count of the dataset.
- `"document_count"`: (*Body parameter*)  
    Document count of the dataset.
- `"parse_mehtod"`: (*Body parameter*)  
    Parsing method of the dataset.

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0 
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't change embedding model since some files already use it."
}
```

## List datasets

**GET** `/api/v1/dataset?name={name}&page={page}&page_size={page_size}&orderby={orderby}&desc={desc}`

List all datasets

### Request

- Method: GET
- URL: `/api/v1/dataset?name={name}&page={page}&page_size={page_size}&orderby={orderby}&desc={desc}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'


#### Request example

```shell
curl --request GET \
     --url http://{address}/api/v1/dataset?page=0&page_size=50&orderby=create_time&desc=false \
     --header 'Content-Type: application/json' \
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

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0,
    "data": [
        {
          "avatar": "",
          "chunk_count": 0,
          "create_date": "Thu, 29 Aug 2024 03:13:07 GMT",
          "create_time": 1724901187843,
          "created_by": "4fb0cd625f9311efba4a0242ac120006",
          "description": "",
          "document_count": 0,
          "embedding_model": "BAAI/bge-large-zh-v1.5",
          "id": "9d3d906665b411ef87d10242ac120006",
          "language": "English",
          "name": "Test",
          "parser_config": {
              "chunk_token_count": 128,
              "delimiter": "\n!?。；！？",
              "layout_recognize": true,
              "task_page_size": 12
          },
          "parse_method": "naive",
          "permission": "me",
          "similarity_threshold": 0.2,
          "status": "1",
          "tenant_id": "4fb0cd625f9311efba4a0242ac120006",
          "token_count": 0,
          "update_date": "Thu, 29 Aug 2024 03:13:07 GMT",
          "update_time": 1724901187843,
          "vector_similarity_weight": 0.3
        }
    ],
}
```

  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't access database to get the dataset list."
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

```shell
curl --request POST \
     --url http://{address}/api/v1/dataset/{dataset_id}/document \
     --header 'Content-Type: multipart/form-data' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
     --form 'file=@test.txt'
```

#### Request parameters

- `"dataset_id"`: (*Path parameter*)
    The dataset id
- `"file"`: (*Body parameter*)  
    The file to upload

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0 
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't connect database"
}
```

## Download a file from a dataset

**GET** `/api/v1/dataset/{dataset_id}/document/{document_id}`

Downloads files from a dataset. 

### Request

- Method: GET
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
- Output:
  - '{FILE_NAME}'
#### Request example

```shell
curl --request GET \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{documents_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --output '{FILE_NAME}'
```

#### Request parameters

- `"dataset_id"`: (*PATH parameter*)
    The dataset id
- `"documents_id"`: (*PATH parameter*)  
    The document id of the file.

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0 
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't connect database"
}
```


## List files of a dataset

**GET** `/api/v1/dataset/{dataset_id}/info?keywords={keyword}&page={page}&page_size={limit}&orderby={orderby}&desc={desc}&name={name}`

List files to a dataset. 

### Request

- Method: GET
- URL: `/api/v1/dataset/{dataset_id}/info?keywords={keyword}&page={page}&page_size={limit}&orderby={orderby}&desc={desc}&name={name`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request GET \
     --url http://{address}/api/v1/dataset/{dataset_id}/info?keywords=rag&page=0&page_size=10&orderby=create_time&desc=yes \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
```

#### Request parameters

- `"dataset_id"`: (*PATH parameter*)
    The dataset id
- `keywords`: (*Filter parameter*)
    The keywords matches the search key workds;
- `page`: (*Filter parameter*)
    The current page number to retrieve from the paginated data. This parameter determines which set of records will be fetched.
- `page_size`: (*Filter parameter*)
    The number of records to retrieve per page. This controls how many records will be included in each page. 
- `orderby`: (*Filter parameter*)
    The field by which the records should be sorted. This specifies the attribute or column used to order the results.
- `desc`: (*Filter parameter*)
    A boolean flag indicating whether the sorting should be in descending order.
- `name`: (*Filter parameter*)
    File name.

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0,
    "data": {
        "docs": [
            {
                "chunk_count": 0,
                "create_date": "Wed, 18 Sep 2024 08:20:49 GMT",
                "create_time": 1726647649379,
                "created_by": "134408906b6811efbcd20242ac120005",
                "id": "e970a94a759611efae5b0242ac120004",
                "knowledgebase_id": "e95f574e759611efbc850242ac120004",
                "location": "Test Document222.txt",
                "name": "Test Document222.txt",
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
                "size": 46,
                "source_type": "local",
                "status": "1",
                "thumbnail": null,
                "token_count": 0,
                "type": "doc",
                "update_date": "Wed, 18 Sep 2024 08:20:49 GMT",
                "update_time": 1726647649379
            },
            {
                "chunk_count": 0,
                "create_date": "Wed, 18 Sep 2024 08:20:49 GMT",
                "create_time": 1726647649340,
                "created_by": "134408906b6811efbcd20242ac120005",
                "id": "e96aad9c759611ef9ab60242ac120004",
                "knowledgebase_id": "e95f574e759611efbc850242ac120004",
                "location": "Test Document111.txt",
                "name": "Test Document111.txt",
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
                "size": 46,
                "source_type": "local",
                "status": "1",
                "thumbnail": null,
                "token_count": 0,
                "type": "doc",
                "update_date": "Wed, 18 Sep 2024 08:20:49 GMT",
                "update_time": 1726647649340
            }
        ],
        "total": 2
    },
}
```

- `"error_code"`: `integer`  
  `0`: The operation succeeds.

  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't connect database"
}
```

## Update a file information in dataset

**PUT** `/api/v1/dataset/{dataset_id}/info/{document_id}`

Update a file in a dataset

### Request

- Method: PUT
- URL: `/api/v1/dataset/{dataset_id}/document`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request PUT \
     --url http://{address}/api/v1/dataset/{dataset_id}/info/{document_id} \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --raw '{
         "document_id": "f6b170ac758811efa0660242ac120004", 
         "document_name": "manual.txt", 
         "thumbnail": null, 
         "knowledgebase_id": "779333c0758611ef910f0242ac120004", 
         "parser_method": "manual", 
         "parser_config": {"chunk_token_count": 128, "delimiter": "\n!?。；！？", "layout_recognize": true, "task_page_size": 12}, 
         "source_type": "local", "type": "doc", 
         "created_by": "134408906b6811efbcd20242ac120005", 
         "size": 0, "token_count": 0, "chunk_count": 0, 
         "progress": 0.0, 
         "progress_msg": "", 
         "process_begin_at": null, 
         "process_duration": 0.0
     }'
```

#### Request parameters

- `"document_id"`: (*Body parameter*)
- `"document_name"`: (*Body parameter*)

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0
}
```
  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't connect database"
}
```

## Parse files in dataset

**POST** `/api/v1/dataset/{dataset_id}/chunk`

Parse files into chunks in a dataset

### Request

- Method: POST
- URL: `/api/v1/dataset/{dataset_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request POST \
     --url http://{address}/api/v1/dataset/{dataset_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --raw '{
         "documents": ["f6b170ac758811efa0660242ac120004", "97ad64b6759811ef9fc30242ac120004"]
     }'
```

#### Request parameters

- `"dataset_id"`: (*Path parameter*)
- `"documents"`: (*Body parameter*)
  - Documents to parse

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0
}
```
  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't connect database"
}
```

## Stop file parsing

**DELETE** `/api/v1/dataset/{dataset_id}/chunk`

Stop file parsing

### Request

- Method: POST
- URL: `/api/v1/dataset/{dataset_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request DELETE \
     --url http://{address}/api/v1/dataset/{dataset_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --raw '{
         "documents": ["f6b170ac758811efa0660242ac120004", "97ad64b6759811ef9fc30242ac120004"]
     }'
```

#### Request parameters

- `"dataset_id"`: (*Path parameter*)
- `"documents"`: (*Body parameter*)
  - Documents to stop parsing

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0
}
```
  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't connect database"
}
```

## Get document chunk list

**GET** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`

Get document chunk list

### Request

- Method: GET
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request GET \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
```

#### Request parameters

- `"dataset_id"`: (*Path parameter*)
- `"document_id"`: (*Path parameter*)

### Response

The successful response includes a JSON object like the following:

```shell
{
    "code": 0
    "data": {
        "chunks": [
            {
                "available_int": 1,
                "content": "<em>advantag</em>of ragflow increas accuraci and relev:by incorpor retriev inform , ragflow can gener respons that are more accur",
                "document_keyword": "ragflow_test.txt",
                "document_id": "77df9ef4759a11ef8bdd0242ac120004",
                "id": "4ab8c77cfac1a829c8d5ed022a0808c0",
                "image_id": "",
                "important_keywords": [],
                "positions": [
                    ""
                ]
            }
        ],
        "doc": {
            "chunk_count": 5,
            "create_date": "Wed, 18 Sep 2024 08:46:16 GMT",
            "create_time": 1726649176833,
            "created_by": "134408906b6811efbcd20242ac120005",
            "id": "77df9ef4759a11ef8bdd0242ac120004",
            "knowledgebase_id": "77d9d24e759a11ef880c0242ac120004",
            "location": "ragflow_test.txt",
            "name": "ragflow_test.txt",
            "parser_config": {
                "chunk_token_count": 128,
                "delimiter": "\n!?。；！？",
                "layout_recognize": true,
                "task_page_size": 12
            },
            "parser_method": "naive",
            "process_begin_at": "Wed, 18 Sep 2024 08:46:16 GMT",
            "process_duation": 7.3213,
            "progress": 1.0,
            "progress_msg": "\nTask has been received.\nStart to parse.\nFinish parsing.\nFinished slicing files(5). Start to embedding the content.\nFinished embedding(6.16)! Start to build index!\nDone!",
            "run": "3",
            "size": 4209,
            "source_type": "local",
            "status": "1",
            "thumbnail": null,
            "token_count": 746,
            "type": "doc",
            "update_date": "Wed, 18 Sep 2024 08:46:23 GMT",
            "update_time": 1726649183321
        },
        "total": 1
    },
}
```
  
The error response includes a JSON object like the following:

```shell
{
    "code": 3016,
    "message": "Can't connect database"
}
```

## Delete document chunks

**DELETE** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`

Delete document chunks

### Request

- Method: DELETE
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request DELETE \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --raw '{
         "chunks": ["f6b170ac758811efa0660242ac120004", "97ad64b6759811ef9fc30242ac120004"]
     }'
```

## Update document chunk

**PUT** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`

Update document chunk

### Request

- Method: PUT
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request PUT \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --raw '{
        "chunk_id": "d87fb0b7212c15c18d0831677552d7de",  
        "knowledgebase_id": null,  
        "name": "",  
        "content": "ragflow123",  
        "important_keywords": [],   
        "document_id": "e6bbba92759511efaa900242ac120004",  
        "status": "1" 
     }'
```

## Insert document chunks

**POST** `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`

Insert document chunks

### Request

- Method: POST
- URL: `/api/v1/dataset/{dataset_id}/document/{document_id}/chunk`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request POST \
     --url http://{address}/api/v1/dataset/{dataset_id}/document/{document_id}/chunk \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --raw '{
         "document_id": "97ad64b6759811ef9fc30242ac120004",
         "content": ["ragflow content", "ragflow content"]
     }'
```

## Dataset retrieval test

**GET** `/api/v1/dataset/{dataset_id}/retrieval`

Retrieval test of a dataset

### Request

- Method: GET
- URL: `/api/v1/dataset/{dataset_id}/retrieval`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request GET \
     --url http://{address}/api/v1/dataset/{dataset_id}/retrieval \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --raw '{
         "query_text": "This is a cat."
     }'
```

## Create chat

**POST** `/api/v1/chat`

Create a chat

### Request

- Method: POST
- URL: `/api/v1/chat`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

```shell
curl --request POST \
     --url http://{address}/api/v1/chat \
     --header 'Content-Type: application/json' \
     --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
     --data-binary '{
     "avatar": "path",
     "create_date": "Wed, 04 Sep 2024 10:08:01 GMT",
     "create_time": 1725444481128,
     "description": "A helpful Assistant",
     "do_refer": "",
     "knowledgebases": [
       {
         "avatar": null,
         "chunk_count": 0,
         "description": null,
         "document_count": 0,
         "embedding_model": "",
         "id": "d6d0e8e868cd11ef92250242ac120006",
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
       "model_name": "deepseek-chat",
       "presence_penalty": 0.4,
       "temperature": 0.1,
       "top_p": 0.3
     },
     "name": "Miss R",
     "prompt": {
       "empty_response": "Sorry! Can't find the context!",
       "keywords_similarity_weight": 0.7,
       "opener": "Hi! I am your assistant, what can I do for you?",
       "prompt": "You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence 'The answer you are looking for is not found in the knowledge base!' Answers need to consider chat history.\nHere is the knowledge base:\n{knowledge}\nThe above is the knowledge base.",
       "rerank_model": "",
       "show_quote": true,
       "similarity_threshold": 0.2,
       "top_n": 8,
       "variables": [
         {
           "key": "knowledge",
           "optional": true
         }
       ]
     },
     "prompt_type": "simple",
     "status": "1",
     "top_k": 1024,
     "update_date": "Wed, 04 Sep 2024 10:08:01 GMT",
     "update_time": 1725444481128
}'
```

## Update chat

**PUT** `/api/v1/chat`

Update a chat

### Request

- Method: PUT
- URL: `/api/v1/chat`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

curl --request PUT \
  --url http://{address}/api/v1/chat \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data-binary '{
    "id":"554e96746aaa11efb06b0242ac120005",
    "name":"Test"
}'

## Delete chat

**DELETE** `/api/v1/chat/{chat_id}`

Delete a chat

### Request

- Method: PUT
- URL: `/api/v1/chat/{chat_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

curl --request PUT \
  --url http://{address}/api/v1/chat/554e96746aaa11efb06b0242ac120005 \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
}'

## List chat

**GET** `/api/v1/chat`

List all chat assistants

### Request

- Method: GET
- URL: `/api/v1/chat`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example

curl --request GET \
  --url http://{address}/api/v1/chat \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

## Create a chat session

**POST** `/api/v1/chat/{chat_id}/session`

Create a chat session

### Request

- Method: POST
- URL: `/api/v1/chat/{chat_id}/session`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example
curl --request POST \
  --url http://{address}/api/v1/chat/{chat_id}/session \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}' \
  --data-binary '{
    "name": "new session"
  }'

## List the sessions of a chat

**GET** `/api/v1/chat/{chat_id}/session`

List all the session of a chat

### Request

- Method: GET
- URL: `/api/v1/chat/{chat_id}/session`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example
curl --request GET \
  --url http://{address}/api/v1/chat/554e96746aaa11efb06b0242ac120005/session \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

## Delete a chat session

**DELETE** `/api/v1/chat/{chat_id}/session/{session_id}`

Delete a chat session

### Request

- Method: DELETE
- URL: `/api/v1/chat/{chat_id}/session/{session_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example
curl --request DELETE \
  --url http://{address}/api/v1/chat/554e96746aaa11efb06b0242ac120005/session/791aed9670ea11efbb7e0242ac120007 \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

## Update a chat session

**PUT** `/api/v1/chat/{chat_id}/session/{session_id}`

Update a chat session

### Request

- Method: PUT
- URL: `/api/v1/chat/{chat_id}/session/{session_id}`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example
curl --request PUT \
  --url http://{address}/api/v1/chat/554e96746aaa11efb06b0242ac120005/session/791aed9670ea11efbb7e0242ac120007 \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
  --data-binary '{
    "name": "Updated session"
  }'

## Chat with a chat session

**POST** `/api/v1/chat/{chat_id}/session/{session_id}/completion`

Chat with a chat session

### Request

- Method: POST
- URL: `/api/v1/chat/{chat_id}/session/{session_id}/completion`
- Headers:
  - `content-Type: application/json`
  - 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'

#### Request example
curl --request POST \
  --url http://{address}/api/v1/chat/554e96746aaa11efb06b0242ac120005/session/791aed9670ea11efbb7e0242ac120007/completion \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer {YOUR_ACCESS_TOKEN}'
  --data-binary '{
    "question":  "Hello!",
    "stream": true,
  }'
