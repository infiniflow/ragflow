---
sidebar_position: 1
slug: /api
---

# API reference

![](https://github.com/infiniflow/ragflow/assets/12318111/df0dcc3d-789a-44f7-89f1-7a5f044ab729)

## Base URL
```
https://demo.ragflow.io/v1/
```

## Authorization

All the APIs are authorized with API-Key. Please keep it safe and private. Don't reveal it in any way from the front-end. 
The API-Key should put in the header of request:
```buildoutcfg
Authorization: Bearer {API_KEY}
```

## Start a conversation

This should be called whenever there's new user coming to chat.
### Path: /api/new_conversation
### Method: GET
### Parameter:

| name | type | optional | description|
|------|-------|----|----|
| user_id| string | No | It's for identifying user in order to search and calculate statistics.|

### Response 
```json
{
    "data": {
        "create_date": "Fri, 12 Apr 2024 17:26:21 GMT",
        "create_time": 1712913981857,
        "dialog_id": "4f0a2e4cb9af11ee9ba20aef05f5e94f",
        "duration": 0.0,
        "id": "b9b2e098f8ae11ee9f45fa163e197198",
        "message": [
            {
                "content": "Hi, I'm your assistant, can I help you?",
                "role": "assistant"
            }
        ],
        "reference": [],
        "tokens": 0,
        "update_date": "Fri, 12 Apr 2024 17:26:21 GMT",
        "update_time": 1712913981857,
        "user_id": "kevinhu"
    },
    "retcode": 0,
    "retmsg": "success"
}
```
> data['id'] in response should be stored and will be used in every round of following conversation.

## Get history of a conversation

### Path: /api/conversation/\<id\>
### Method: GET
### Response 
```json
{
    "data": {
        "create_date": "Mon, 01 Apr 2024 09:28:42 GMT",
        "create_time": 1711934922220,
        "dialog_id": "df4a4916d7bd11eeaa650242ac180006",
        "id": "2cae30fcefc711ee94140242ac180006",
        "message": [
            {
                "content": "Hi! I'm your assistant, what can I do for you?",
                "role": "assistant"
            },
            {
                "content": "What's the vit score for GPT-4?",
                "role": "user"
            },
            {
                "content": "The ViT Score for GPT-4 in the zero-shot scenario is 0.5058, and in the few-shot scenario, it is 0.6480. ##0$$",
                "role": "assistant"
            },
            {
                "content": "How is the nvlink topology like?",
                "role": "user"
            },
            {
                "content": "NVLink topology refers to the arrangement of connections between GPUs using NVIDIA's NVLink technology. Correct NVLink topology for NVIDIA A100 cards involves connecting one GPU to another through a series of NVLink bridges ##0$$. Each of the three attached bridges spans two PCIe slots, and for optimal performance and balanced bridge topology, all three NVLink bridges should be used when connecting two adjacent A100 cards.\n\nHere's a summary of the correct and incorrect topologies:\n\n- **Correct**: Both GPUs are connected via all three NVLink bridges, ensuring full bandwidth and proper communication.\n- **Incorrect**: Not using all three bridges or having an uneven connection configuration would result in suboptimal performance.\n\nIt's also important to note that for multi-CPU systems, both A100 cards in a bridged pair should be within the same CPU domain, unless each CPU has a single A100 PCIe card, in which case they can be bridged together.",
                "role": "assistant"
            }
        ],
        "user_id": "user name",
        "reference": [
            {
                "chunks": [
                    {
                        "chunk_id": "d0bc7892c3ec4aeac071544fd56730a8",
                        "content_ltks": "tabl 1:openagi task-solv perform under differ set for three closed-sourc llm . boldfac denot the highest score under each learn schema . metric gpt-3.5-turbo claude-2 gpt-4 zero few zero few zero few clip score 0.0 0.0 0.0 0.2543 0.0 0.3055 bert score 0.1914 0.3820 0.2111 0.5038 0.2076 0.6307 vit score 0.2437 0.7497 0.4082 0.5416 0.5058 0.6480 overal 0.1450 0.3772 0.2064 0.4332 0.2378 0.5281",
                        "content_with_weight": "<table><caption>Table 1: OpenAGI task-solving performances under different settings for three closed-source LLMs. Boldface denotes the highest score under each learning schema.</caption>\n<tr><th  rowspan=2 >Metrics</th><th  >GPT-3.5-turbo</th><th></th><th  >Claude-2</th><th  >GPT-4</th></tr>\n<tr><th  >Zero</th><th  >Few</th><th  >Zero Few</th><th  >Zero Few</th></tr>\n<tr><td  >CLIP Score</td><td  >0.0</td><td  >0.0</td><td  >0.0 0.2543</td><td  >0.0 0.3055</td></tr>\n<tr><td  >BERT Score</td><td  >0.1914</td><td  >0.3820</td><td  >0.2111 0.5038</td><td  >0.2076 0.6307</td></tr>\n<tr><td  >ViT Score</td><td  >0.2437</td><td  >0.7497</td><td  >0.4082 0.5416</td><td  >0.5058 0.6480</td></tr>\n<tr><td  >Overall</td><td  >0.1450</td><td  >0.3772</td><td  >0.2064 0.4332</td><td  >0.2378 0.5281</td></tr>\n</table>",
                        "doc_id": "c790da40ea8911ee928e0242ac180005",
                        "docnm_kwd": "OpenAGI When LLM Meets Domain Experts.pdf",
                        "img_id": "afab9fdad6e511eebdb20242ac180006-d0bc7892c3ec4aeac071544fd56730a8",
                        "important_kwd": [],
                        "kb_id": "afab9fdad6e511eebdb20242ac180006",
                        "positions": [
                            [
                                9.0,
                                159.9383341471354,
                                472.1773274739583,
                                223.58013916015625,
                                307.86692301432294
                            ]
                        ],
                        "similarity": 0.7310340654129031,
                        "term_similarity": 0.7671974387781668,
                        "vector_similarity": 0.40556370512552886
                    },
                    {
                        "chunk_id": "7e2345d440383b756670e1b0f43a7007",
                        "content_ltks": "5.5 experiment analysi the main experiment result are tabul in tab . 1 and 2 , showcas the result for closed-sourc and open-sourc llm , respect . the overal perform is calcul a the averag of cllp 8 bert and vit score . here , onli the task descript of the benchmark task are fed into llm(addit inform , such a the input prompt and llm\u2019output , is provid in fig . a.4 and a.5 in supplementari). broadli speak , closed-sourc llm demonstr superior perform on openagi task , with gpt-4 lead the pack under both zero-and few-shot scenario . in the open-sourc categori , llama-2-13b take the lead , consist post top result across variou learn schema--the perform possibl influenc by it larger model size . notabl , open-sourc llm significantli benefit from the tune method , particularli fine-tun and\u2019rltf . these method mark notic enhanc for flan-t5-larg , vicuna-7b , and llama-2-13b when compar with zero-shot and few-shot learn schema . in fact , each of these open-sourc model hit it pinnacl under the rltf approach . conclus , with rltf tune , the perform of llama-2-13b approach that of gpt-3.5 , illustr it potenti .",
                        "content_with_weight": "5.5 Experimental Analysis\nThe main experimental results are tabulated in Tab. 1 and 2, showcasing the results for closed-source and open-source LLMs, respectively. The overall performance is calculated as the average of CLlP\n8\nBERT and ViT scores. Here, only the task descriptions of the benchmark tasks are fed into LLMs (additional information, such as the input prompt and LLMs\u2019 outputs, is provided in Fig. A.4 and A.5 in supplementary). Broadly speaking, closed-source LLMs demonstrate superior performance on OpenAGI tasks, with GPT-4 leading the pack under both zero- and few-shot scenarios. In the open-source category, LLaMA-2-13B takes the lead, consistently posting top results across various learning schema--the performance possibly influenced by its larger model size. Notably, open-source LLMs significantly benefit from the tuning methods, particularly Fine-tuning and\u2019 RLTF. These methods mark noticeable enhancements for Flan-T5-Large, Vicuna-7B, and LLaMA-2-13B when compared with zero-shot and few-shot learning schema. In fact, each of these open-source models hits its pinnacle under the RLTF approach. Conclusively, with RLTF tuning, the performance of LLaMA-2-13B approaches that of GPT-3.5, illustrating its potential.",
                        "doc_id": "c790da40ea8911ee928e0242ac180005",
                        "docnm_kwd": "OpenAGI When LLM Meets Domain Experts.pdf",
                        "img_id": "afab9fdad6e511eebdb20242ac180006-7e2345d440383b756670e1b0f43a7007",
                        "important_kwd": [],
                        "kb_id": "afab9fdad6e511eebdb20242ac180006",
                        "positions": [
                            [
                                8.0,
                                107.3,
                                508.90000000000003,
                                686.3,
                                697.0
                            ],
                        ],
                        "similarity": 0.6691508616357027,
                        "term_similarity": 0.6999011754270821,
                        "vector_similarity": 0.39239803751328806
                    },
                ],
                "doc_aggs": [
                    {
                        "count": 8,
                        "doc_id": "c790da40ea8911ee928e0242ac180005",
                        "doc_name": "OpenAGI When LLM Meets Domain Experts.pdf"
                    }
                ],
                "total": 8
            },
            {
                "chunks": [
                    {
                        "chunk_id": "8c11a1edddb21ad2ae0c43b4a5dcfa62",
                        "content_ltks": "nvlink bridg support nvidia\u00aenvlink\u00aei a high-spe point-to-point peer transfer connect , where one gpu can transfer data to and receiv data from one other gpu . the nvidia a100 card support nvlink bridg connect with a singl adjac a100 card . each of the three attach bridg span two pcie slot . to function correctli a well a to provid peak bridg bandwidth , bridg connect with an adjac a100 card must incorpor all three nvlink bridg . wherev an adjac pair of a100 card exist in the server , for best bridg perform and balanc bridg topolog , the a100 pair should be bridg . figur 4 illustr correct and incorrect a100 nvlink connect topolog . nvlink topolog\u2013top view figur 4. correct incorrect correct incorrect for system that featur multipl cpu , both a100 card of a bridg card pair should be within the same cpu domain\u2014that is , under the same cpu\u2019s topolog . ensur thi benefit workload applic perform . the onli except is for dual cpu system wherein each cpu ha a singl a100 pcie card under it;in that case , the two a100 pcie card in the system may be bridg togeth . a100 nvlink speed and bandwidth are given in the follow tabl . tabl 5. a100 nvlink speed and bandwidth paramet valu total nvlink bridg support by nvidia a100 3 total nvlink rx and tx lane support 96 data rate per nvidia a100 nvlink lane(each direct)50 gbp total maximum nvlink bandwidth 600 gbyte per second pb-10137-001_v03|8 nvidia a100 40gb pcie gpu acceler",
                        "content_with_weight": "NVLink Bridge Support\nNVIDIA\u00aeNVLink\u00aeis a high-speed point-to-point peer transfer connection, where one GPU can transfer data to and receive data from one other GPU. The NVIDIA A100 card supports NVLink bridge connection with a single adjacent A100 card.\nEach of the three attached bridges spans two PCIe slots. To function correctly as well as to provide peak bridge bandwidth, bridge connection with an adjacent A100 card must incorporate all three NVLink bridges. Wherever an adjacent pair of A100 cards exists in the server, for best bridging performance and balanced bridge topology, the A100 pair should be bridged. Figure 4 illustrates correct and incorrect A100 NVLink connection topologies.\nNVLink Topology \u2013Top Views \nFigure 4. \nCORRECT \nINCORRECT \nCORRECT \nINCORRECT \nFor systems that feature multiple CPUs, both A100 cards of a bridged card pair should be within the same CPU domain\u2014that is, under the same CPU\u2019s topology. Ensuring this benefits workload application performance. The only exception is for dual CPU systems wherein each CPU has a single A100 PCIe card under it; in that case, the two A100 PCIe cards in the system may be bridged together.\nA100 NVLink speed and bandwidth are given in the following table.\n<table><caption>Table 5. A100 NVLink Speed and Bandwidth </caption>\n<tr><th  >Parameter </th><th  >Value </th></tr>\n<tr><td  >Total NVLink bridges supported by NVIDIA A100 </td><td  >3 </td></tr>\n<tr><td  >Total NVLink Rx and Tx lanes supported </td><td  >96 </td></tr>\n<tr><td  >Data rate per NVIDIA A100 NVLink lane (each direction)</td><td  >50 Gbps </td></tr>\n<tr><td  >Total maximum NVLink bandwidth</td><td  >600 Gbytes per second </td></tr>\n</table>\nPB-10137-001_v03 |8\nNVIDIA A100 40GB PCIe GPU Accelerator",
                        "doc_id": "806d1ed0ea9311ee860a0242ac180005",
                        "docnm_kwd": "A100-PCIE-Prduct-Brief.pdf",
                        "img_id": "afab9fdad6e511eebdb20242ac180006-8c11a1edddb21ad2ae0c43b4a5dcfa62",
                        "important_kwd": [],
                        "kb_id": "afab9fdad6e511eebdb20242ac180006",
                        "positions": [
                            [
                                12.0,
                                84.0,
                                541.3,
                                76.7,
                                96.7
                            ],
                        ],
                        "similarity": 0.3200748779905588,
                        "term_similarity": 0.3082244010114718,
                        "vector_similarity": 0.42672917080234146
                    },
                ],
                "doc_aggs": [
                    {
                        "count": 1,
                        "doc_id": "806d1ed0ea9311ee860a0242ac180005",
                        "doc_name": "A100-PCIE-Prduct-Brief.pdf"
                    }
                ],
                "total": 3
            }
        ],
        "update_date": "Tue, 02 Apr 2024 09:07:49 GMT",
        "update_time": 1712020069421
    },
    "retcode": 0,
    "retmsg": "success"
}
```

- **message**: All the chat history in it.
    - role: user or assistant
    - content: the text content of user or assistant. The citations are in format like: ##0$$. The number in the middle indicate which part in data.reference.chunks it refers to.
    
- **user_id**: This is set by the caller.
- **reference**: Every item in it refer to the corresponding message in data.message whose role is assistant. 
    - chunks
        - content_with_weight: The content of chunk.
        - docnm_kwd: the document name.
        - img_id: the image id of the chunk. It is an optional field only for PDF/pptx/picture. And accessed by 'GET' /document/get/\<id\>.
        - positions: [page_number, [upleft corner(x, y)], [right bottom(x, y)]], the chunk position, only for PDF.
        - similarity: the hybrid similarity.
        - term_similarity: keyword simimlarity
        - vector_similarity: embedding similarity
    - doc_aggs:
        - doc_id: the document can be accessed by 'GET' /document/get/\<id\>
        - doc_name: the file name
        - count: the chunk number hit in this document.
    
## Chat

This will be called to get the answer to users' questions.

### Path: /api/completion
### Method: POST
### Parameter:

| name | type | optional | description|
|------|-------|----|----|
| conversation_id| string | No | This is from calling /new_conversation.|
| messages| json | No | The latest question, such as `[{"role": "user", "content": "How are you doing!"}]`|
| quote | bool | Yes | Default: true |
| stream | bool | Yes | Default: true |
| doc_ids | string | Yes | Document IDs which is delimited by comma, like `c790da40ea8911ee928e0242ac180005,c790da40ea8911ee928e0242ac180005`. The retrieved content is limited in these documents. |

### Response 
```json
{
    "data": {
      "answer": "The ViT Score for GPT-4 in the zero-shot scenario is 0.5058, and in the few-shot scenario, it is 0.6480. ##0$$",
      "reference": {
        "chunks": [
          {
            "chunk_id": "d0bc7892c3ec4aeac071544fd56730a8",
            "content_ltks": "tabl 1:openagi task-solv perform under differ set for three closed-sourc llm . boldfac denot the highest score under each learn schema . metric gpt-3.5-turbo claude-2 gpt-4 zero few zero few zero few clip score 0.0 0.0 0.0 0.2543 0.0 0.3055 bert score 0.1914 0.3820 0.2111 0.5038 0.2076 0.6307 vit score 0.2437 0.7497 0.4082 0.5416 0.5058 0.6480 overal 0.1450 0.3772 0.2064 0.4332 0.2378 0.5281",
            "content_with_weight": "<table><caption>Table 1: OpenAGI task-solving performances under different settings for three closed-source LLMs. Boldface denotes the highest score under each learning schema.</caption>\n<tr><th  rowspan=2 >Metrics</th><th  >GPT-3.5-turbo</th><th></th><th  >Claude-2</th><th  >GPT-4</th></tr>\n<tr><th  >Zero</th><th  >Few</th><th  >Zero Few</th><th  >Zero Few</th></tr>\n<tr><td  >CLIP Score</td><td  >0.0</td><td  >0.0</td><td  >0.0 0.2543</td><td  >0.0 0.3055</td></tr>\n<tr><td  >BERT Score</td><td  >0.1914</td><td  >0.3820</td><td  >0.2111 0.5038</td><td  >0.2076 0.6307</td></tr>\n<tr><td  >ViT Score</td><td  >0.2437</td><td  >0.7497</td><td  >0.4082 0.5416</td><td  >0.5058 0.6480</td></tr>\n<tr><td  >Overall</td><td  >0.1450</td><td  >0.3772</td><td  >0.2064 0.4332</td><td  >0.2378 0.5281</td></tr>\n</table>",
            "doc_id": "c790da40ea8911ee928e0242ac180005",
            "docnm_kwd": "OpenAGI When LLM Meets Domain Experts.pdf",
            "img_id": "afab9fdad6e511eebdb20242ac180006-d0bc7892c3ec4aeac071544fd56730a8",
            "important_kwd": [],
            "kb_id": "afab9fdad6e511eebdb20242ac180006",
            "positions": [
              [
                9.0,
                159.9383341471354,
                472.1773274739583,
                223.58013916015625,
                307.86692301432294
              ]
            ],
            "similarity": 0.7310340654129031,
            "term_similarity": 0.7671974387781668,
            "vector_similarity": 0.40556370512552886
          },
          {
            "chunk_id": "7e2345d440383b756670e1b0f43a7007",
            "content_ltks": "5.5 experiment analysi the main experiment result are tabul in tab . 1 and 2 , showcas the result for closed-sourc and open-sourc llm , respect . the overal perform is calcul a the averag of cllp 8 bert and vit score . here , onli the task descript of the benchmark task are fed into llm(addit inform , such a the input prompt and llm\u2019output , is provid in fig . a.4 and a.5 in supplementari). broadli speak , closed-sourc llm demonstr superior perform on openagi task , with gpt-4 lead the pack under both zero-and few-shot scenario . in the open-sourc categori , llama-2-13b take the lead , consist post top result across variou learn schema--the perform possibl influenc by it larger model size . notabl , open-sourc llm significantli benefit from the tune method , particularli fine-tun and\u2019rltf . these method mark notic enhanc for flan-t5-larg , vicuna-7b , and llama-2-13b when compar with zero-shot and few-shot learn schema . in fact , each of these open-sourc model hit it pinnacl under the rltf approach . conclus , with rltf tune , the perform of llama-2-13b approach that of gpt-3.5 , illustr it potenti .",
            "content_with_weight": "5.5 Experimental Analysis\nThe main experimental results are tabulated in Tab. 1 and 2, showcasing the results for closed-source and open-source LLMs, respectively. The overall performance is calculated as the average of CLlP\n8\nBERT and ViT scores. Here, only the task descriptions of the benchmark tasks are fed into LLMs (additional information, such as the input prompt and LLMs\u2019 outputs, is provided in Fig. A.4 and A.5 in supplementary). Broadly speaking, closed-source LLMs demonstrate superior performance on OpenAGI tasks, with GPT-4 leading the pack under both zero- and few-shot scenarios. In the open-source category, LLaMA-2-13B takes the lead, consistently posting top results across various learning schema--the performance possibly influenced by its larger model size. Notably, open-source LLMs significantly benefit from the tuning methods, particularly Fine-tuning and\u2019 RLTF. These methods mark noticeable enhancements for Flan-T5-Large, Vicuna-7B, and LLaMA-2-13B when compared with zero-shot and few-shot learning schema. In fact, each of these open-source models hits its pinnacle under the RLTF approach. Conclusively, with RLTF tuning, the performance of LLaMA-2-13B approaches that of GPT-3.5, illustrating its potential.",
            "doc_id": "c790da40ea8911ee928e0242ac180005",
            "docnm_kwd": "OpenAGI When LLM Meets Domain Experts.pdf",
            "img_id": "afab9fdad6e511eebdb20242ac180006-7e2345d440383b756670e1b0f43a7007",
            "important_kwd": [],
            "kb_id": "afab9fdad6e511eebdb20242ac180006",
            "positions": [
              [
                8.0,
                107.3,
                508.90000000000003,
                686.3,
                697.0
              ]
            ],
            "similarity": 0.6691508616357027,
            "term_similarity": 0.6999011754270821,
            "vector_similarity": 0.39239803751328806
          }
        ],
        "doc_aggs": {
          "OpenAGI When LLM Meets Domain Experts.pdf": 4
        },
        "total": 8
      }
    },
    "retcode": 0,
    "retmsg": "success"
}
```

- **answer**: The replay of the chat bot.
- **reference**: 
    - chunks: Every item in it refer to the corresponding message in answer. 
        - content_with_weight: The content of chunk.
        - docnm_kwd: the document name.
        - img_id: the image id of the chunk. It is an optional field only for PDF/pptx/picture. And accessed by 'GET' /document/get/\<id\>.
        - positions: [page_number, [upleft corner(x, y)], [right bottom(x, y)]], the chunk position, only for PDF.
        - similarity: the hybrid similarity.
        - term_similarity: keyword simimlarity
        - vector_similarity: embedding similarity
    - doc_aggs:
        - doc_id: the document can be accessed by 'GET' /document/get/\<id\>
        - doc_name: the file name
        - count: the chunk number hit in this document.
    
## Get document content or image

This is usually used when display content of citation.
### Path: /api/document/get/\<id\>
### Method: GET

## Upload file

This is usually used when upload a file to.
### Path: /api/document/upload/
### Method: POST

### Parameter:

| name      | type   | optional | description                                             |
|-----------|--------|----------|---------------------------------------------------------|
| file      | file   | No       | Upload file.                                            |
| kb_name   | string | No       | Choose the upload knowledge base name.                  |
| parser_id | string | Yes      | Choose the parsing method.                              |
| run       | string | Yes      | Parsing will start automatically when the value is "1". |

### Response 
```json
{
    "data": {
        "chunk_num": 0,
        "create_date": "Thu, 25 Apr 2024 14:30:06 GMT",
        "create_time": 1714026606921,
        "created_by": "553ec818fd5711ee8ea63043d7ed348e",
        "id": "41e9324602cd11ef9f5f3043d7ed348e",
        "kb_id": "06802686c0a311ee85d6246e9694c130",
        "location": "readme.txt",
        "name": "readme.txt",
        "parser_config": {
            "field_map": {
            },
            "pages": [
                [
                    0,
                    1000000
                ]
            ]
        },
        "parser_id": "general",
        "process_begin_at": null,
        "process_duation": 0.0,
        "progress": 0.0,
        "progress_msg": "",
        "run": "0",
        "size": 929,
        "source_type": "local",
        "status": "1",
        "thumbnail": null,
        "token_num": 0,
        "type": "doc",
        "update_date": "Thu, 25 Apr 2024 14:30:06 GMT",
        "update_time": 1714026606921
    },
    "retcode": 0,
    "retmsg": "success"
}

```

## Get document chunks

Get the chunks of the document based on doc_name or doc_id.
### Path: /api/list_chunks/
### Method: POST

### Parameter:

| Name     | Type   | Optional | Description                     |
|----------|--------|----------|---------------------------------|
| `doc_name` | string | Yes      | The name of the document in the knowledge base. It must not be empty if `doc_id` is not set.|
| `doc_id`   | string | Yes      | The ID of the document in the knowledge base. It must not be empty if `doc_name` is not set.|


### Response 
```json
{
    "data": [
        {
            "content": "Figure 14: Per-request neural-net processingof RL-Cache.\n103\n(sn)\nCPU\n 102\nGPU\n8101\n100\n8\n16 64 256 1K\n4K",
            "doc_name": "RL-Cache.pdf",
            "img_id": "0335167613f011ef91240242ac120006-b46c3524952f82dbe061ce9b123f2211"
        },
        {
            "content": "4.3 ProcessingOverheadof RL-CacheACKNOWLEDGMENTSThis section evaluates how e￿ectively our RL-Cache implemen-tation leverages modern multi-core CPUs and GPUs to keep the per-request neural-net processing overhead low. Figure 14 depictsThis researchwas supported inpart by the Regional Government of Madrid (grant P2018/TCS-4499, EdgeData-CM)andU.S. National Science Foundation (grants CNS-1763617 andCNS-1717179).REFERENCES",
            "doc_name": "RL-Cache.pdf",
            "img_id": "0335167613f011ef91240242ac120006-d4c12c43938eb55d2d8278eea0d7e6d7"
        }
    ],
    "retcode": 0,
    "retmsg": "success"
}

```

## Get document list from knowledge base

Get document list based on the knowledge base name and corresponding parameters.
### Path: /api/list_kb_docs/
### Method: POST

### Parameter:

| Name        | Type   | Optional | Description                                                          |
|-------------|--------|----------|----------------------------------------------------------------------|
| `kb_name`   | string | No       | The name of the knowledge base, from which you get the document list. |
| `page`      | int    | Yes      | The number of pages, default:1.                                      |
| `page_size` | int    | Yes      | The number of docs for each page, default:15.                        |
| `orderby`   | string | Yes      | `chunk_num`, `create_time`, or `size`, default:`create_time`         |
| `desc`      | bool   | Yes      | Default:True.                                                        |
| `keywords`  | string | Yes      | Keyword of the document name.                                        |


### Response 
```json
{
    "data": {
        "docs": [
            {
                "doc_id": "bad89a84168c11ef9ce40242ac120006",
                "doc_name": "test.xlsx"
            },
            {
                "doc_id": "641a9b4013f111efb53f0242ac120006",
                "doc_name": "1111.pdf"
            }
        ],
        "total": 2
    },
    "retcode": 0,
    "retmsg": "success"
}

```
