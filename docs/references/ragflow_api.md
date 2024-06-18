---
sidebar_class_name: hidden
---

# API reference

RAGFlow offers RESTful APIs for you to integrate its capabilities into third-party applications. 

## Base URL
```
https://demo.ragflow.io/api/v1/
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
You are *required* to save the `data.id` value returned in the response data, which is the session ID for all upcoming conversations.
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
    "dataset_name": "kb1"
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

```python
(200, 
{
    "code": 102,
    "data": [
        {
            "avatar": None,
            "chunk_num": 0,
            "create_date": "Mon, 17 Jun 2024 16:00:05 GMT",
            "create_time": 1718611205876,
            "created_by": "b48110a0286411ef994a3043d7ee537e",
            "description": None,
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
        },
        # ... additional datasets ...
    ],
    "message": "attempt to list datasets"
}
)
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
  "success": true, 
  "message": "Dataset deleted successfully!"
}
```  
