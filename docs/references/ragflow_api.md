---
sidebar_class_name: hidden
---

# API reference

RAGFlow offers RESTful APIs for you to integrate its capabilities into third-party applications. 

## Base URL
```
http://<host_address>/v1/api/
```

## Dataset URL
```
http://<host_address>/api/v1/dataset
```

## Authorization

All of RAGFlow's RESTFul APIs use API key for authorization, so keep it safe and do not expose it to the front end. 
Put your API key in the request header. 

```buildoutcfg
Authorization: Bearer {API_KEY}
```

To get your API key:

1. In RAGFlow, click **Chat** tab in the middle top of the page.
2. Hover over the corresponding dialogue **>** **Chat Bot API** to show the chatbot API configuration page.
3. Click **Api Key** **>** **Create new key** to create your API key.
4. Copy and keep your API key safe. 

## Create dataset

This method creates (news) a dataset for a specific user. 

### Request

#### Request URI

| Method | Request URI |
|--------|-------------|
| POST   | `/dataset`  |

:::note
You are *required* to save the `data.dataset_id` value returned in the response data, which is the session ID for all upcoming conversations.
:::

#### Request parameter

| Name           |  Type  | Required | Description                                                                                                                                                                                                                                                                                                         |
|----------------|--------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dataset_name` | string |   Yes    | The unique identifier assigned to each newly created dataset. `dataset_name` must be less than 2 ** 10 characters and cannot be empty. The following character sets are supported: <br />- 26 lowercase English letters (a-z)<br />- 26 uppercase English letters (A-Z)<br />- 10 digits (0-9)<br />- "_", "-", "." |

### Response 

```json
{
  "code": 0, 
  "data": {
    "dataset_name": "kb1",
    "dataset_id": "375e8ada2d3c11ef98f93043d7ee537e"
  }, 
  "message": "success"
}
```

## Get dataset list

This method lists the created datasets for a specific user. 

### Request

#### Request URI

| Method   | Request URI |
|----------|-------------|
| GET      | `/dataset`  |

### Response 

#### Response parameter

```json
{
    "code": 0,
    "data": [
        {
            "avatar": null,
            "chunk_num": 0,
            "create_date": "Mon, 17 Jun 2024 16:00:05 GMT",
            "create_time": 1718611205876,
            "created_by": "b48110a0286411ef994a3043d7ee537e",
            "description": null,
            "doc_num": 0,
            "embd_id": "BAAI/bge-large-zh-v1.5",
            "id": "9bd6424a2c7f11ef81b83043d7ee537e",
            "language": "Chinese",
            "name": "dataset3(23)",
            "parser_config": {
                "pages": [
                    [
                        1,
                        1000000
                    ]
                ]
            },
            "parser_id": "naive",
            "permission": "me",
            "similarity_threshold": 0.2,
            "status": "1",
            "tenant_id": "b48110a0286411ef994a3043d7ee537e",
            "token_num": 0,
            "update_date": "Mon, 17 Jun 2024 16:00:05 GMT",
            "update_time": 1718611205876,
            "vector_similarity_weight": 0.3
        }
    ],
    "message": "List datasets successfully!"
}
```

## Delete dataset

This method deletes a dataset for a specific user.

### Request

#### Request URI

| Method | Request URI             |
|--------|-------------------------|
| DELETE | `/dataset/{dataset_id}` |

#### Request parameter

| Name         |  Type  | Required | Description                                                                                                                                                      |
|--------------|--------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dataset_id` | string | Yes      | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.                                                                                |

### Response 

```json
{
  "code": 0,
  "message": "Remove dataset: 9cefaefc2e2611ef916b3043d7ee537e successfully"
}
```  

### Get the details of the specific dataset

This method gets the details of the specific dataset. 

### Request

#### Request URI

| Method   | Request URI             |
|----------|-------------------------|
| GET      | `/dataset/{dataset_id}` |

#### Request parameter

| Name         |  Type  | Required | Description                                                                                                                                                      |
|--------------|--------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dataset_id` | string | Yes      | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.                                                                                |

### Response 

```json
{
    "code": 0,
    "data": {
        "avatar": null,
        "chunk_num": 0, 
        "description": null,
        "doc_num": 0,
        "embd_id": "BAAI/bge-large-zh-v1.5",
        "id": "060323022e3511efa8263043d7ee537e", 
        "language": "Chinese", 
        "name": "test(1)", 
        "parser_config": 
        {
            "pages": [[1, 1000000]]
        }, 
        "parser_id": "naive", 
        "permission": "me", 
        "token_num": 0
  }, 
    "message": "success"
}
```

### Update the details of the specific dataset

This method updates the details of the specific dataset. 

### Request

#### Request URI

| Method | Request URI             |
|--------|-------------------------|
| PUT    | `/dataset/{dataset_id}` |

#### Request parameter

You are required to input at least one parameter.

| Name                 | Type   | Required | Description                                                           |
|----------------------|--------|----------|-----------------------------------------------------------------------|
| `name`               | string | No       | The name of the knowledge base, from which you get the document list. |
| `description`        | string | No       | The description of the knowledge base.                                |
| `permission`         | string | No       | The permission for the knowledge base, default:me.                    |
| `language`           | string | No       | The language of the knowledge base.                                   |
| `chunk_method`       | string | No       | The chunk method of the knowledge base.                               |
| `embedding_model_id` | string | No       | The embedding model id of the knowledge base.                         |
| `photo`              | string | No       | The photo of the knowledge base.                                      |
| `layout_recognize`   | bool   | No       | The layout recognize of the knowledge base.                           |
| `token_num`          | int    | No       | The token number of the knowledge base.                               |
| `id`                 | string | No       | The id of the knowledge base.                                         |

### Response 

### Successful response

```json
{
    "code": 0,
    "data": {
        "avatar": null,
        "chunk_num": 0,
        "create_date": "Wed, 19 Jun 2024 20:33:34 GMT",
        "create_time": 1718800414518, 
        "created_by": "b48110a0286411ef994a3043d7ee537e", 
        "description": "new_description1", 
        "doc_num": 0, 
        "embd_id": "BAAI/bge-large-zh-v1.5", 
        "id": "24f9f17a2e3811ef820e3043d7ee537e", 
        "language": "English", 
        "name": "new_name", 
        "parser_config": 
        {
            "pages": [[1, 1000000]]
        },
        "parser_id": "naive", 
        "permission": "me", 
        "similarity_threshold": 0.2, 
        "status": "1", 
        "tenant_id": "b48110a0286411ef994a3043d7ee537e", 
        "token_num": 0, 
        "update_date": "Wed, 19 Jun 2024 20:33:34 GMT", 
        "update_time": 1718800414529, 
        "vector_similarity_weight": 0.3
  }, 
    "message": "success"
}
```

### Response for the operating error

```json
{
    "code": 103, 
    "message": "Only the owner of knowledgebase is authorized for this operation!"
}
```

### Response for no parameter
```json
{ 
    "code": 102, 
    "message": "Please input at least one parameter that you want to update!"
}
```

------------------------------------------------------------------------------------------------------------------------------

## Upload documents

This method uploads documents for a specific user. 

### Request

#### Request URI

| Method | Request URI                       |
|--------|-----------------------------------|
| POST   | `/dataset/{dataset_id}/documents` |


#### Request parameter

| Name         |  Type  | Required |        Description                                         |
|--------------|--------|----------|------------------------------------------------------------|
| `dataset_id` | string |   Yes    | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID. |

### Response 

### Successful response

```json
{
      "code": 0,
      "data": [
        {
          "created_by": "b48110a0286411ef994a3043d7ee537e",
          "id": "859584a0379211efb1a23043d7ee537e",
          "kb_id": "8591349a379211ef92213043d7ee537e",
          "location": "test.txt",
          "name": "test.txt",
          "parser_config": {
            "pages": [
              [1, 1000000]
            ]
          },
          "parser_id": "naive",
          "size": 0,
          "thumbnail": null,
          "type": "doc"
        },
        {
          "created_by": "b48110a0286411ef994a3043d7ee537e",
          "id": "8596f18c379211efb1a23043d7ee537e",
          "kb_id": "8591349a379211ef92213043d7ee537e",
          "location": "test1.txt",
          "name": "test1.txt",
          "parser_config": {
            "pages": [
              [1, 1000000]
            ]
          },
          "parser_id": "naive",
          "size": 0,
          "thumbnail": null,
          "type": "doc"
        }
      ],
      "message": "success"
}
```

### Response for nonexistent files

```json
{
      "code": "RetCode.DATA_ERROR",
      "message": "The file test_data/imagination.txt does not exist"
}
```

### Response for nonexistent dataset

```json
{
      "code": 102,
      "message": "Can't find this dataset"
}
```

### Response for the number of files exceeding the limit

```json
{
      "code": 102,
      "message": "You try to upload 512 files, which exceeds the maximum number of uploading files: 256"
}
```
### Response for uploading without files.

```json
{
    "code": 101,
    "message": "None is not string."
}
```

## Delete documents

This method deletes documents for a specific user. 

### Request

#### Request URI

| Method | Request URI                       |
|--------|-----------------------------------|
| DELETE | `/dataset/{dataset_id}/documents/{document_id}` |


#### Request parameter

| Name          |  Type  | Required | Description                                                                         |
|---------------|--------|----------|-------------------------------------------------------------------------------------|
| `dataset_id`  | string |   Yes    | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.   |
| `document_id` | string |   Yes    | The ID of the document. Call ['GET' /document](#list-documents) to retrieve the ID. |

### Response 

### Successful response

```json
{
      "code": 0,
      "data": true,
      "message": "success"
}
```

### Response for deleting a document that does not exist

```json
{
      "code": 102,
      "message": "Document 111 not found!"
}
```
### Response for deleting documents from a non-existent dataset

```json
{
      "code": 101,
      "message": "The document f7aba1ec379b11ef8e853043d7ee537e is not in the dataset: 000, but in the dataset: f7a7ccf2379b11ef83223043d7ee537e."
}
```

## List documents

This method lists documents for a specific user. 

### Request

#### Request URI

| Method | Request URI                       |
|--------|-----------------------------------|
| GET    | `/dataset/{dataset_id}/documents` |


#### Request parameter

| Name         | Type   | Required | Description                                                                                                |
|--------------|--------|----------|------------------------------------------------------------------------------------------------------------|
| `dataset_id` | string | Yes      | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.                          |
| `offset`     | int    | No       | The start of the listed documents. Default: 0                                                              |
| `count`      | int    | No       | The total count of the listed documents. Default: -1, meaning all the later part of documents from the start. |
| `order_by`   | string | No       | Default: `create_time`                                                                                     |
| `descend`    | bool   | No       | The order of listing documents. Default: True                                                              |
| `keywords`   | string | No       | The searching keywords of listing documents. Default: ""                                                   |

### Response 

### Successful Response 

```json
{
      "code": 0,
      "data": {
        "docs": [
          {
            "chunk_num": 0,
            "create_date": "Mon, 01 Jul 2024 19:24:10 GMT",
            "create_time": 1719833050046,
            "created_by": "b48110a0286411ef994a3043d7ee537e",
            "id": "6fb6f588379c11ef87023043d7ee537e",
            "kb_id": "6fb1c9e6379c11efa3523043d7ee537e",
            "location": "empty.txt",
            "name": "empty.txt",
            "parser_config": {
              "pages": [
                [1, 1000000]
              ]
            },
            "parser_id": "naive",
            "process_begin_at": null,
            "process_duation": 0.0,
            "progress": 0.0,
            "progress_msg": "",
            "run": "0",
            "size": 0,
            "source_type": "local",
            "status": "1",
            "thumbnail": null,
            "token_num": 0,
            "type": "doc",
            "update_date": "Mon, 01 Jul 2024 19:24:10 GMT",
            "update_time": 1719833050046
          },
          {
            "chunk_num": 0,
            "create_date": "Mon, 01 Jul 2024 19:24:10 GMT",
            "create_time": 1719833050037,
            "created_by": "b48110a0286411ef994a3043d7ee537e",
            "id": "6fb59c60379c11ef87023043d7ee537e",
            "kb_id": "6fb1c9e6379c11efa3523043d7ee537e",
            "location": "test.txt",
            "name": "test.txt",
            "parser_config": {
              "pages": [
                [1, 1000000]
              ]
            },
            "parser_id": "naive",
            "process_begin_at": null,
            "process_duation": 0.0,
            "progress": 0.0,
            "progress_msg": "",
            "run": "0",
            "size": 0,
            "source_type": "local",
            "status": "1",
            "thumbnail": null,
            "token_num": 0,
            "type": "doc",
            "update_date": "Mon, 01 Jul 2024 19:24:10 GMT",
            "update_time": 1719833050037
          }
        ],
        "total": 2
      },
      "message": "success"
}
```

### Response for listing documents with IndexError

```json
{
      "code": 100,
      "message": "IndexError('Offset is out of the valid range.')"
}
```
## Update the details of the document

This method updates the details, including the name, enable and template type of a specific document for a specific user. 

### Request

#### Request URI

| Method | Request URI                                     |
|--------|-------------------------------------------------|
| PUT    | `/dataset/{dataset_id}/documents/{document_id}` |


#### Request parameter

| Name         | Type   | Required | Description                                                                                                |
|--------------|--------|----------|------------------------------------------------------------------------------------------------------------|
| `dataset_id`  | string |   Yes    | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.   |
| `document_id` | string |   Yes    | The ID of the document. Call ['GET' /document](#list-documents) to retrieve the ID. |

### Response 

### Successful Response 

```json
{
      "code": 0,
      "data": {
        "chunk_num": 0,
        "create_date": "Mon, 15 Jul 2024 16:55:03 GMT",
        "create_time": 1721033703914,
        "created_by": "b48110a0286411ef994a3043d7ee537e",
        "id": "ed30167a428711efab193043d7ee537e",
        "kb_id": "ed2d8770428711efaf583043d7ee537e",
        "location": "test.txt",
        "name": "new_name.txt",
        "parser_config": {
          "pages": [
            [1, 1000000]
          ]
        },
        "parser_id": "naive",
        "process_begin_at": null,
        "process_duration": 0.0,
        "progress": 0.0,
        "progress_msg": "",
        "run": "0",
        "size": 14,
        "source_type": "local",
        "status": "1",
        "thumbnail": null,
        "token_num": 0,
        "type": "doc",
        "update_date": "Mon, 15 Jul 2024 16:55:03 GMT",
        "update_time": 1721033703934
      },
      "message": "Success"
}
```

### Response for updating a document which does not exist.

```json
{
      "code": 101,
      "message": "This document weird_doc_id cannot be found!"
}
```

### Response for updating a document without giving parameters.
```json
{
      "code": 102,
      "message": "Please input at least one parameter that you want to update!"
}
```

### Response for updating a document in the nonexistent dataset.
```json
{
      "code": 102,
      "message": "This dataset fake_dataset_id cannot be found!"
}
```

### Response for updating a document with an extension name that differs from its original.
```json
{
      "code": 101,
      "data": false,
      "message": "The extension of file cannot be changed"
}
```

### Response for updating a document with a duplicate name.
```json
{
      "code": 101,
      "message": "Duplicated document name in the same dataset."
}
```

### Response for updating a document's illegal parameter.
```json
{
      "code": 101,
      "message": "illegal_parameter is an illegal parameter."
}
```

### Response for updating a document's name without its name value.
```json
{
      "code": 102,
      "message": "There is no new name."
}
```

### Response for updating a document's with giving illegal enable's value.
```json
{
      "code": 102,
      "message": "Illegal value '?' for 'enable' field."
}
```

## Download the document

This method downloads a specific document for a specific user. 

### Request

#### Request URI

| Method | Request URI                                     |
|--------|-------------------------------------------------|
| GET    | `/dataset/{dataset_id}/documents/{document_id}` |


#### Request parameter

| Name         | Type   | Required | Description                                                                                                |
|--------------|--------|----------|------------------------------------------------------------------------------------------------------------|
| `dataset_id`  | string |   Yes    | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.   |
| `document_id` | string |   Yes    | The ID of the document. Call ['GET' /document](#list-documents) to retrieve the ID. |

### Response 

### Successful Response 

```json
{
      "code": "0",
      "data": "b'test\\ntest\\ntest'"
}
```

### Response for downloading a document which does not exist.

```json
{
      "code": 101,
      "message": "This document 'imagination.txt' cannot be found!"
}
```

### Response for downloading a document in the nonexistent dataset.
```json
{
      "code": 102,
      "message": "This dataset 'imagination' cannot be found!"
}
```

### Response for downloading an empty document.
```json
{
      "code": 102,
      "message": "This file is empty."
}
```

## Start parsing a document

This method enables a specific document to start parsing for a specific user. 

### Request

#### Request URI

| Method | Request URI                                            |
|--------|--------------------------------------------------------|
| POST   | `/dataset/{dataset_id}/documents/{document_id}/status` |


#### Request parameter

| Name         | Type   | Required | Description                                                                                                |
|--------------|--------|----------|------------------------------------------------------------------------------------------------------------|
| `dataset_id`  | string |   Yes    | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.   |
| `document_id` | string |   Yes    | The ID of the document. Call ['GET' /document](#list-documents) to retrieve the ID. |

### Response 

### Successful Response 

```json
{
      "code": 0,
      "message": ""
}
```

### Response for parsing a document which does not exist.

```json
{
      "code": 101,
      "message": "This document 'imagination.txt' cannot be found!"
}
```

### Response for parsing a document in the nonexistent dataset.
```json
{
      "code": 102,
      "message": "This dataset 'imagination' cannot be found!"
}
```

### Response for parsing an empty document.
```json
{
      "code": 0,
      "message": "Empty data in the document: empty.txt;"
}
```

## Start parsing multiple documents

This method enables multiple documents, including all documents in the specific dataset or specified documents, to start parsing for a specific user. 

### Request

#### Request URI

| Method | Request URI                                           |
|--------|-------------------------------------------------------|
| POST   | `/dataset/{dataset_id}/documents/status` |


#### Request parameter

| Name         | Type   | Required | Description                                                                                                                       |
|--------------|--------|----------|-----------------------------------------------------------------------------------------------------------------------------------|
| `dataset_id`  | string |   Yes    | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.                                                 |
| `document_id` | string |   Yes    | The ID of the document. Call ['GET' /document](#list-documents) to retrieve the ID.                                               |
| `doc_ids` | list | No | The document IDs of the documents that the user would like to parse. Default: None, means all documents in the specified dataset. |
### Response 

### Successful Response 

```json
{
      "code": 0,
      "data": true,
      "message": ""
}
```

### Response for parsing documents which does not exist.

```json
{
      "code": 101,
      "message": "This document 'imagination.txt' cannot be found!"
}
```

### Response for parsing documents in the nonexistent dataset.
```json
{
      "code": 102,
      "message": "This dataset 'imagination' cannot be found!"
}
```

### Response for parsing documents, one of which is empty.
```json
{
      "code": 0,
      "data": true,
      "message": "Empty data in the document: empty.txt; "
}
```

## Show the parsing status of the document

This method shows the parsing status of the document for a specific user. 

### Request

#### Request URI

| Method | Request URI                                           |
|--------|-------------------------------------------------------|
| GET    | `/dataset/{dataset_id}/documents/status` |


#### Request parameter

| Name         | Type   | Required | Description                                                                                                                       |
|--------------|--------|----------|-----------------------------------------------------------------------------------------------------------------------------------|
| `dataset_id`  | string |   Yes    | The ID of the dataset. Call ['GET' /dataset](#create-dataset) to retrieve the ID.                                                 |
| `document_id` | string |   Yes    | The ID of the document. Call ['GET' /document](#list-documents) to retrieve the ID.                                               |

### Response 

### Successful Response 

```json
{
      "code": 0,
      "data": {
            "progress": 0.0,
            "status": "RUNNING"
      },
      "message": "success"
}
```

### Response for showing the parsing status of a document which does not exist.

```json
{
      "code": 102,
      "message": "This document: 'imagination.txt' is not a valid document."
}
```

### Response for showing the parsing status of a document in the nonexistent dataset.
```json
{
      "code": 102,
      "message": "This dataset 'imagination' cannot be found!"
}
```
