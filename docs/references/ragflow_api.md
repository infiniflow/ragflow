---
sidebar_class_name: hidden
---

# API reference

RAGFlow offers RESTful APIs for you to integrate its capabilities into third-party applications. 

## Base URL
```
http://<host_address>/api/v1/
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