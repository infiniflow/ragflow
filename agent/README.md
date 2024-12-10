English | [简体中文](./README_zh.md)

# *Graph*


## Introduction

*Graph* is a mathematical concept which is composed of nodes and edges. 
It is used to compose a complex work flow or agent. 
And this graph is beyond the DAG that we can use circles to describe our agent or work flow.
Under this folder, we propose a test tool ./test/client.py which can test the DSLs such as json files in folder ./test/dsl_examples.
Please use this client at the same folder you start RAGFlow. If it's run by Docker, please go into the container before running the client.
Otherwise, correct configurations in service_conf.yaml is essential.

```bash
PYTHONPATH=path/to/ragflow python graph/test/client.py -h
usage: client.py [-h] -s DSL -t TENANT_ID -m

options:
  -h, --help            show this help message and exit
  -s DSL, --dsl DSL     input dsl
  -t TENANT_ID, --tenant_id TENANT_ID
                        Tenant ID
  -m, --stream          Stream output
```
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/79179c5e-d4d6-464a-b6c4-5721cb329899" width="1000"/>
</div>


## How to gain a TENANT_ID in command line?
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/419d8588-87b1-4ab8-ac49-2d1f047a4b97" width="600"/>
</div>
💡 We plan to display it here in the near future.
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/c97915de-0091-46a5-afd9-e278946e5fe3" width="600"/>
</div>


## How to set 'kb_ids' for component 'Retrieval' in DSL?
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/0a731534-cac8-49fd-8a92-ca247eeef66d" width="600"/>
</div>

