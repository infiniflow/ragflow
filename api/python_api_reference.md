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

Creates a dataset.

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

The url or path to the avatar image associated with the created dataset. Defaults to `""`

#### tenant_id: `str`

The id of the tenant associated with the created dataset is used to identify different users. Defaults to `None`.

- If creating a dataset, tenant_id must not be provided.
- If updating a dataset, tenant_id can't be changed.

#### description: `str`

The description of the created dataset. Defaults to `""`.

#### language: `str`

The language setting for the created dataset. Defaults to `"English"`.

#### embedding_model: `str`

The specific model or algorithm used by the dataset to generate vector embeddings. Defaults to `""`.

- If creating a dataset, embedding_model must not be provided.
- If updating a dataset, embedding_model can't be changed.

#### permission: `str`

Specify who can operate on the dataset. Defaults to `"me"`.

#### document_count: `int`

The number of documents associated with the created dataset. Defaults to `0`.

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

## Update knowledge base information

```python

```