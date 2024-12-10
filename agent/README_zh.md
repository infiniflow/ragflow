[English](./README.md) | 简体中文

# *Graph*


## 简介

"Graph"是一个由节点和边组成的数学概念。
它被用来构建复杂的工作流或代理。
这个图超越了有向无环图（DAG），我们可以使用循环来描述我们的代理或工作流。
在这个文件夹下，我们提出了一个测试工具 ./test/client.py，
它可以测试像文件夹./test/dsl_examples下一样的DSL文件。
请在启动 RAGFlow 的同一文件夹中使用此客户端。如果它是通过 Docker 运行的，请在运行客户端之前进入容器。
否则，正确配置 service_conf.yaml 文件是必不可少的。

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
<img src="https://github.com/infiniflow/ragflow/assets/12318111/05924730-c427-495b-8ee4-90b8b2250681" width="1000"/>
</div>


## 命令行中的TENANT_ID如何获得?
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/419d8588-87b1-4ab8-ac49-2d1f047a4b97" width="600"/>
</div>
💡 后面会展示在这里：
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/c97915de-0091-46a5-afd9-e278946e5fe3" width="600"/>
</div>


## DSL里面的Retrieval组件的kb_ids怎么填?
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/0a731534-cac8-49fd-8a92-ca247eeef66d" width="600"/>
</div>

