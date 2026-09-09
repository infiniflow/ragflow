#
#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""
Search-datasets consistency tests between Python (port 9380) and Go (port 9384) servers.
Compares /api/v1/datasets/search endpoint responses for consistency between Python and Go.

When an LLM is involved (rerank_id, keyword, or cross_languages is set), both sides
call the LLM independently and the chunk *count*, ordering, and scores can drift
across runs and between Python/Go. The test logs the per-chunk chunk_id + similarity
for human inspection but skips strict count/order/score comparisons in those cases.

When no LLM is involved, both servers run the same deterministic retrieval path
with no non-deterministic dependencies, so the responses are expected to be
byte-identical: same chunk count, same chunk order, and the same per-field
values (with the empty-value normalization done in compare_chunks).

All datasets and documents are created once at module level, then each test unit
runs against the pre-built data. Cleanup happens automatically at module teardown.
"""

import logging
import os
import sys
import pytest
import requests
import tempfile
import time
import uuid

# Logging setup
# Default is silent. Set LOG_LEVEL=INFO locally to see logger.info() messages.
_LOG_LEVEL = os.environ.get("LOG_LEVEL", "WARNING").upper()
_log_handler = logging.StreamHandler(sys.stderr)
_log_handler.setFormatter(logging.Formatter("%(levelname)s: %(message)s"))
logger = logging.getLogger("search_datasets_consistency")
logger.setLevel(getattr(logging, _LOG_LEVEL, logging.WARNING))
logger.addHandler(_log_handler)
logger.propagate = False

from test.testcases.utils import wait_for

PYTHON_HOST = "http://localhost:9380"
GO_HOST = "http://localhost:9384"

# ---------------------------------------------------------------------------
# Shared test documents
# ---------------------------------------------------------------------------
THREE_KINGDOMS_TXT = """
曹操（155 年—220 年）
作为曹魏政权的奠基者，曹操展现出卓越的政治、军事和文学才能。在政治上，
他挟天子以令诸侯，稳固自身政治地位，推行一系列政策，如推行屯田制，不仅
解决了粮食短缺问题，还使大量流民得以安置，促进了农业生产的恢复与发展 ；
在军事方面，曹操一生征战无数，官渡之战以少胜多击败袁绍，统一北方，压制
匈奴等异族势力，为北方地区带来相对稳定的局面 。然而，曹操的评价褒贬不
一，"治世能臣"体现他在治理国家、施展政治抱负方面的才能；"乱世奸雄"
则反映他在乱世中为达目的不择手段的一面 。

刘备（161 年—223 年）
蜀汉开国皇帝刘备，以"仁德"著称。他的一生充满传奇色彩，早年与关羽、张
飞桃园结义，奠定兄弟情谊，三人患难与共，为兴复汉室而努力 。刘备求贤若
渴，三顾茅庐请出诸葛亮，得到这位智谋之士的辅佐，为蜀汉政权的建立与发展
奠定根基 。刘备善于笼络人心，凭借自身的人格魅力吸引了众多人才，在乱世
中逐渐崛起，建立蜀汉，与曹魏、东吴形成三足鼎立之势 。

孙权（182 年—252 年）
东吴的建立者孙权，年少继承父兄基业，展现出非凡的领导才能 。他擅长用人
制衡，麾下人才济济，周瑜、鲁肃、陆逊等皆是东吴的栋梁之才 。孙权依托长
江天险，制定了稳健的战略，致力于开发江南经济与海外贸易，使东吴在三国中
占据重要地位 。在赤壁之战中，孙权与刘备联军击败曹操，奠定三国鼎立的基
础；之后又在夷陵之战中击败刘备，巩固了东吴在江南的统治 。

赵云（？—229 年）
赵云一生未败，长坂坡单骑救主，在曹操大军中七进七出，如入无人之境，成功
救出刘备之子刘禅，被誉为"常胜将军" 。赵云跟随刘备多年，忠心耿耿，多
次在危难时刻挺身而出，保护刘备及其家人的安全 。他武艺高强，为人正直，
深受刘备和蜀军将士的敬重 。
"""

WATER_MARGIN_TXT = """
宋江（？—？）
宋江是梁山泊一百零八将之首，人称"及时雨"。他原为郓城县押司，因仗义疏财、
济人贫苦，在江湖上广结英雄豪杰。宋江性格矛盾，一方面讲究忠义，一心想着
招安报国；另一方面又领导梁山好汉对抗朝廷。他带领梁山好汉两赢童贯、三败
高俅，最终接受朝廷招安，率军征讨辽国、田虎、王庆、方腊，立下赫赫战功。

武松（？—？）
武松因其排行第二，人称"武二郎"。景阳冈打虎，使他一举成名。武松武艺高强，
力大无穷，是梁山泊步军头领。他因兄长武大郎被西门庆、潘金莲毒害，怒杀二
人，被发配孟州。在孟州，他醉打蒋门神，帮助施恩夺回快活林。大闹飞云浦、
血溅鸳鸯楼后，武松走上反抗道路，最终加入梁山。

李逵（？—？）
李逵是梁山泊好汉中最具反抗精神的代表，人称"黑旋风"。他性格鲁莽直率，
嫉恶如仇，对宋江忠心耿耿。李逵使用两把板斧，在战场上所向披靡，屡立战功。
他曾在江州劫法场救宋江，在沂岭杀四虎，展现了惊人的勇气和力量。
"""

ENGLISH_DOCS = """
Artificial Intelligence
Artificial intelligence (AI) is intelligence demonstrated by machines, in contrast to
the natural intelligence displayed by humans and animals. Leading AI textbooks define
the field as the study of "intelligent agents": any system that perceives its environment
and takes actions that maximize its chance of successfully achieving its goals.
AI applications include advanced web search engines (e.g., Google Search), recommendation
systems (used by YouTube, Amazon, and Netflix), understanding human speech (such as Siri
and Alexa), self-driving cars (e.g., Waymo), generative and creative tools (ChatGPT and
AI art), and superhuman play and analysis in strategy games (such as chess and Go).

Machine Learning
Machine learning (ML) is a field of inquiry devoted to understanding and building methods
that "learn" – that is, methods that leverage data to improve performance on some set of
tasks. It is seen as a part of artificial intelligence. Machine learning algorithms build
a model based on sample data, known as training data, in order to make predictions or
decisions without being explicitly programmed to do so. Machine learning algorithms are
used in a wide variety of applications, such as in medicine, email filtering, speech
recognition, agriculture, and computer vision, where it is difficult or unfeasible to
develop conventional algorithms to perform the needed tasks.

Deep Learning
Deep learning is part of a broader family of machine learning methods based on artificial
neural networks with representation learning. Learning can be supervised, semi-supervised
or unsupervised. Deep-learning architectures such as deep neural networks, deep belief
networks, deep reinforcement learning, recurrent neural networks, convolutional neural
networks and transformers have been applied to fields including computer vision, speech
recognition, natural language processing, machine translation, bioinformatics, drug design,
medical image analysis, climate science, material inspection and board game programs,
where they have produced results comparable to and in some cases surpassing human expert
performance.
"""


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
def compare_chunks(python_chunk, go_chunk):
    """Compare a single chunk between Python and Go responses."""
    all_keys = set(python_chunk.keys()) | set(go_chunk.keys())

    for field in all_keys:
        p_val = python_chunk.get(field)
        g_val = go_chunk.get(field)

        # Normalize empty values to None so the comparison treats
        # "no value" shapes interchangeably across engines:
        #   Python Infinity:    []      (keyword list, infinity_conn.py:793-794)
        #   Python Elasticsearch: ""    (keyword, es_conn.py:626-644)
        #   Go (both engines):  None   (intentional — see retrieval.go:403-424)
        if isinstance(p_val, (list, str)) and not p_val:
            p_val = None
        if isinstance(g_val, (list, str)) and not g_val:
            g_val = None

        if p_val is None or g_val is None:
            if p_val != g_val:
                raise AssertionError(f"Field '{field}': python={p_val}, go={g_val}")
            continue

        if field in ("similarity", "term_similarity", "vector_similarity"):
            if p_val != g_val:
                raise AssertionError(f"Field '{field}' differs: python={p_val}, go={g_val}, diff={abs(p_val - g_val)}")
        elif isinstance(p_val, (list, dict)):
            if p_val != g_val:
                raise AssertionError(f"Field '{field}' mismatch")
        else:
            if p_val != g_val:
                raise AssertionError(f"Field '{field}': python={p_val}, go={g_val}")


def search_and_compare(rest_client, dataset_ids, cfg):
    """Perform search on both servers and compare results.

    dataset_ids can be a single dataset ID string or a list of dataset IDs.
    cfg is a dict with keys matching the search payload:
        question (required), top_k (default 5), rerank_id, search_id, keyword,
        vector_similarity_weight, similarity_threshold, use_kg, cross_languages,
        page, size, meta_data_filter
    """
    headers = {"Authorization": f"Bearer {rest_client.token}"}
    if isinstance(dataset_ids, str):
        ids = [dataset_ids]
    else:
        ids = list(dataset_ids)

    search_payload = {
        "dataset_ids": ids,
        "question": cfg["question"],
        "top_k": cfg.get("top_k", 5),
    }
    optional_fields = [
        "rerank_id",
        "search_id",
        "keyword",
        "vector_similarity_weight",
        "similarity_threshold",
        "use_kg",
        "cross_languages",
        "page",
        "size",
        "meta_data_filter",
        "doc_ids",
    ]
    for field in optional_fields:
        value = cfg.get(field)
        if value is not None:
            search_payload[field] = value

    # Call Python server
    python_res = rest_client.post("/datasets/search", json=search_payload)
    assert python_res.status_code == 200, f"Python server error: {python_res.status_code}, body: {python_res.text}"
    python_data = python_res.json()
    assert python_data["code"] == 0, f"Python payload error: {python_data}"

    # Call Go server with same auth
    go_res = requests.post(
        f"{GO_HOST}/api/v1/datasets/search",
        json=search_payload,
        headers=headers,
        timeout=30,
    )
    assert go_res.status_code == 200, f"Go server error: {go_res.status_code}, body: {go_res.text}"
    go_data = go_res.json()
    assert go_data["code"] == 0, f"Go payload error: {go_data}"

    python_chunks = python_data["data"]["chunks"]
    go_chunks = go_data["data"]["chunks"]

    logger.info(f"python_chunks={len(python_chunks)}, go_chunks={len(go_chunks)}")
    logger.info(f"  Python chunks: {[(c.get('chunk_id', '?'), c.get('similarity', 0)) for c in python_chunks]}")
    logger.info(f"  Go chunks:     {[(c.get('chunk_id', '?'), c.get('similarity', 0)) for c in go_chunks]}")

    llm_involved = bool(cfg.get("rerank_id") or cfg.get("keyword") or cfg.get("cross_languages"))
    if not llm_involved:
        assert len(python_chunks) == len(go_chunks), f"Chunk count differs: python={len(python_chunks)}, go={len(go_chunks)}"
        for i, (p_chunk, g_chunk) in enumerate(zip(python_chunks, go_chunks)):
            try:
                compare_chunks(p_chunk, g_chunk)
            except AssertionError as e:
                raise AssertionError(f"Chunk {i} comparison failed: {e}")
        python_total = python_data["data"].get("total", 0)
        go_total = go_data["data"].get("total", 0)
        if python_total != go_total:
            raise AssertionError(f"total differs: python={python_total}, go={go_total}")

    return len(python_chunks), len(python_chunks)


def _upload_and_parse(rest_client, dataset_id, text, filename="doc.txt"):
    """Upload text as a file and wait for parsing to complete. Returns document_id."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False, encoding="utf-8") as f:
        f.write(text)
        temp_path = f.name

    with open(temp_path, "rb") as f:
        files = [("file", (filename, f))]
        upload_res = rest_client.post(f"/datasets/{dataset_id}/documents", files=files)
    assert upload_res.status_code == 200, f"Failed to upload {filename}: {upload_res.text}"
    doc_id = upload_res.json()["data"][0]["id"]

    parse_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/parse",
        json={"document_ids": [doc_id]},
    )
    assert parse_res.status_code == 200, f"Failed to start parsing {filename}: {parse_res.text}"

    @wait_for(120, 2, f"Document parsing timeout for {filename}")
    def check_parsed():
        doc_res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"id": doc_id})
        if doc_res.status_code != 200:
            return False
        docs = doc_res.json()["data"]["docs"]
        return bool(docs) and docs[0].get("run") == "DONE"

    check_parsed()
    return doc_id


def _get_chunk_count(rest_client, dataset_id, doc_id):
    """Return the number of chunks for a parsed document."""
    doc_res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"id": doc_id})
    docs = doc_res.json()["data"]["docs"]
    return docs[0].get("chunk_count", 0)


def _set_metadata(rest_client, dataset_id, doc_id, updates):
    """Set metadata key-value pairs on a document."""
    headers = {"Authorization": f"Bearer {rest_client.token}"}
    formatted_updates = [{"key": k, "value": v} for k, v in updates.items()]
    res = requests.patch(
        f"{PYTHON_HOST}/api/v1/datasets/{dataset_id}/documents/metadatas",
        json={
            "selector": {"document_ids": [doc_id]},
            "updates": formatted_updates,
        },
        headers=headers,
        timeout=30,
    )
    assert res.status_code == 200, f"Failed to set metadata: {res.text}"
    assert res.json()["code"] == 0, f"Metadata update error: {res.json()}"
    meta_str = ", ".join(f"{k}={v}" for k, v in updates.items())
    logger.info(f"    metadata set on doc {doc_id}: {meta_str}")


# ---------------------------------------------------------------------------
# Module-level fixture: create all datasets once, delete at the end
# ---------------------------------------------------------------------------
@pytest.fixture(scope="module")
def all_datasets(rest_client):
    """Set up all datasets and documents once for the module.

    Returns a dict with:
        ds_chinese        : 1 dataset with 2 files (Three Kingdoms + Water Margin, with metadata)
        ds_chinese_doc1   : doc_id for Three Kingdoms
        ds_chinese_doc2   : doc_id for Water Margin

        ds_chinese_2 : 1 dataset with Three Kingdoms only
        ds_3k_doc         : doc_id

        ds_english        : 1 dataset with English text
        ds_english_doc    : doc_id
    """
    logger.info("\n[SETUP] Creating all datasets for search-datasets consistency tests...")

    data = {}

    # -----------------------------------------------------------------------
    # 1) 1 dataset with 2 files (Chinese)
    # -----------------------------------------------------------------------
    create_res = rest_client.post(
        "/datasets",
        json={
            "name": "consistency_chinese",
            "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
            "parser_config": {"chunk_token_num": 1, "delimiter": "`\n\n`"},
        },
    )
    assert create_res.status_code == 200, create_res.text
    assert create_res.json()["code"] == 0, create_res.json()
    ds_chinese_id = create_res.json()["data"]["id"]

    doc1 = _upload_and_parse(rest_client, ds_chinese_id, THREE_KINGDOMS_TXT, "three_kingdoms.txt")
    doc2 = _upload_and_parse(rest_client, ds_chinese_id, WATER_MARGIN_TXT, "water_margin.txt")
    logger.info(f"  ds_chinese: {ds_chinese_id} (3k={doc1}, wm={doc2})")
    logger.info(f"    3K chunks: {_get_chunk_count(rest_client, ds_chinese_id, doc1)}")
    logger.info(f"    WM chunks: {_get_chunk_count(rest_client, ds_chinese_id, doc2)}")

    # Set metadata on individual documents
    _set_metadata(rest_client, ds_chinese_id, doc1, {"era": 220, "source": "luo", "character": ["曹操", "刘备", "孙权", "赵云"]})
    _set_metadata(rest_client, ds_chinese_id, doc2, {"era": 960, "source": "shi", "character": ["宋江", "武松", "李逵"]})

    data["ds_chinese"] = ds_chinese_id
    data["ds_chinese_doc1"] = doc1
    data["ds_chinese_doc2"] = doc2

    # -----------------------------------------------------------------------
    # 2) 1 dataset with Three Kingdoms only
    # -----------------------------------------------------------------------
    create_res = rest_client.post(
        "/datasets",
        json={
            "name": "consistency_three_kingdoms",
            "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
            "parser_config": {"chunk_token_num": 1, "delimiter": "`\n\n`"},
        },
    )
    assert create_res.status_code == 200, create_res.text
    assert create_res.json()["code"] == 0, create_res.json()
    ds_3k_id = create_res.json()["data"]["id"]

    doc_3k = _upload_and_parse(rest_client, ds_3k_id, THREE_KINGDOMS_TXT, "three_kingdoms.txt")
    logger.info(f"  ds_chinese_2: {ds_3k_id} (doc={doc_3k})")
    logger.info(f"    chunks: {_get_chunk_count(rest_client, ds_3k_id, doc_3k)}")

    data["ds_chinese_2"] = ds_3k_id
    data["ds_3k_doc"] = doc_3k

    # Wait for metadata to be indexed
    time.sleep(2)

    # -----------------------------------------------------------------------
    # 3) 1 dataset with English text
    # -----------------------------------------------------------------------
    create_res = rest_client.post(
        "/datasets",
        json={
            "name": "consistency_english",
            "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
            "parser_config": {"chunk_token_num": 1, "delimiter": "`\n\n`"},
        },
    )
    assert create_res.status_code == 200, create_res.text
    assert create_res.json()["code"] == 0, create_res.json()
    ds_en_id = create_res.json()["data"]["id"]

    doc_en = _upload_and_parse(rest_client, ds_en_id, ENGLISH_DOCS, "english_docs.txt")
    logger.info(f"  ds_english: {ds_en_id} (doc={doc_en})")
    logger.info(f"    chunks: {_get_chunk_count(rest_client, ds_en_id, doc_en)}")

    data["ds_english"] = ds_en_id
    data["ds_english_doc"] = doc_en

    logger.info("[SETUP] All datasets ready.\n")

    yield data

    # Teardown: delete all datasets
    logger.info("\n[TEARDOWN] Deleting all datasets...")
    all_ids = [
        data["ds_chinese"],
        data["ds_chinese_2"],
        data["ds_english"],
    ]
    res = rest_client.delete("/datasets", json={"ids": all_ids})
    assert res.status_code == 200, f"Teardown failed: {res.text}"
    logger.info("[TEARDOWN] Done.")


# Skip every test in this module from CI. Remove the next line to re-enable.
pytestmark = pytest.mark.skipif(
    os.getenv("CI") == "true",
    reason="GO server is not started in CI",
)


# ---------------------------------------------------------------------------
# Test Unit 1: Search consistency — 1 dataset with 2 files
# ---------------------------------------------------------------------------
@pytest.mark.p2
def test_search_datasets_consistency_basic(rest_client, all_datasets):
    """
    Compare /api/v1/datasets/search responses between Python and Go servers for consistency.
    Tests the single dataset (ds_chinese) which contains 2 files.
    """
    dataset_id = all_datasets["ds_chinese"]
    doc1 = all_datasets["ds_chinese_doc1"]
    doc2 = all_datasets["ds_chinese_doc2"]
    logger.info(f"Using dataset (Chinese, 2 files): {dataset_id} (doc1={doc1}, doc2={doc2})")

    search_configs = [
        {"question": "曹操"},
        {"question": "曹操", "page": 2, "size": 2},
        {"question": "曹操", "top_k": 2},
        {"question": "曹操", "similarity_threshold": 0.0},
        {"question": "曹操", "similarity_threshold": 0.5},
        {"question": "曹操", "vector_similarity_weight": 0.0},
        {"question": "曹操", "vector_similarity_weight": 0.7},
        {"question": "曹操", "keyword": True},
        {"question": "努力发展农业", "keyword": True},
        {"question": "political status", "cross_languages": ["Chinese"]},
        {"question": "诸葛亮"},
        {"question": "努力发展农业"},
        {"question": "曹操 诸葛亮 周瑜"},
        {"question": "曹操", "top_k": 3, "rerank_id": "BAAI/bge-reranker-v2-m3@CI@SILICONFLOW"},
        {"question": "曹操", "doc_ids": [doc1]},
        {"question": "曹操", "doc_ids": [doc2]},
        {"question": "曹操", "doc_ids": []},
    ]

    for cfg in search_configs:
        cfg_str = ", ".join(f"{k}={v}" for k, v in cfg.items())
        logger.info(f"\n--- Testing: {cfg_str} ---")
        total, chunk_count = search_and_compare(rest_client, dataset_id, cfg)
        logger.info(f"SUCCESS: Python and Go responses match for {chunk_count} chunks, total={total}")


# ---------------------------------------------------------------------------
# Test Unit 2: Metadata filter consistency
# ---------------------------------------------------------------------------
@pytest.mark.p2
def test_search_datasets_consistency_metadata_filter(rest_client, all_datasets):
    """
    Compare Python vs Go search with metadata filtering on the Chinese dataset for consistency.
    Uses ds_chinese which has 2 documents with metadata:
      - doc1: era=220, source=luo, character=[曹操,刘备,孙权,赵云]
      - doc2: era=960, source=shi, character=[宋江,武松,李逵]
    """
    dataset_id = all_datasets["ds_chinese"]
    logger.info(f"Using dataset (Chinese, metadata): {dataset_id}")

    search_configs = [
        # Manual filters
        {"question": "打虎", "meta_data_filter": {"method": "manual", "manual": [{"key": "era", "op": "=", "value": 960}]}},
        {"question": "曹操", "meta_data_filter": {"method": "manual", "manual": [{"key": "source", "op": "=", "value": "luo"}]}},
        {"question": "打虎", "meta_data_filter": {"method": "manual", "manual": [{"key": "era", "op": "≠", "value": 960}]}},
        {"question": "打虎", "meta_data_filter": {"method": "manual", "manual": [{"key": "era", "op": ">", "value": 220}]}},
        {"question": "曹操", "meta_data_filter": {"method": "manual", "manual": [{"key": "source", "op": "contains", "value": "luo"}]}},
        {"question": "努力发展农业", "meta_data_filter": {"method": "manual", "manual": [{"key": "character", "op": "in", "value": ["曹操", "孙权"]}]}},
        {"question": "打虎", "meta_data_filter": {"method": "manual", "manual": [{"key": "character", "op": "=", "value": "武松"}]}},
    ]

    for cfg in search_configs:
        cfg_str = ", ".join(f"{k}={v}" for k, v in cfg.items())
        logger.info(f"\n--- Metadata filter: {cfg_str} ---")
        total, chunk_count = search_and_compare(rest_client, dataset_id, cfg)
        logger.info(f"SUCCESS: {chunk_count} chunks, total={total}")


# ---------------------------------------------------------------------------
# Test Unit 3: Search with search_id
# ---------------------------------------------------------------------------
@pytest.mark.p2
def test_search_datasets_consistency_with_search_id(rest_client, all_datasets):
    """
    Compare Python vs Go with search_id parameter for consistency.
    Creates a search config with multiple parameters set, then tests that
    both servers honor the stored config overrides.
    """
    dataset_id = all_datasets["ds_chinese"]
    logger.info(f"Using dataset (Chinese): {dataset_id}")

    # Create a search config first
    search_name = f"consistency_search_{uuid.uuid4().hex[:8]}"
    search_res = rest_client.post("/searches", json={"name": search_name, "description": "consistency test search"})
    assert search_res.status_code == 200, f"Failed to create search: {search_res.text}"
    search_payload = search_res.json()
    assert search_payload["code"] == 0, f"Search creation error: {search_payload}"
    search_id = search_payload["data"]["search_id"]
    logger.info(f"Created search_id: {search_id}")

    # Update the search config with multiple parameters
    search_config = {
        "similarity_threshold": 0.2,
        "vector_similarity_weight": 0.7,
        "top_k": 3,
        "use_kg": False,
        "keyword": False,
    }
    update_res = rest_client.put(
        f"/searches/{search_id}",
        json={"name": search_name, "search_config": search_config},
    )
    assert update_res.status_code == 200, f"Failed to update search config: {update_res.text}"
    assert update_res.json()["code"] == 0, f"Search config update error: {update_res.json()}"
    logger.info(f"Updated search config: {search_config}")

    # Search with search_id — config overrides should apply
    search_configs = [
        {"question": "曹操", "search_id": search_id},
        {"question": "曹操 诸葛亮", "search_id": search_id},
    ]

    for cfg in search_configs:
        cfg_str = ", ".join(f"{k}={v}" for k, v in cfg.items())
        logger.info(f"\n--- Testing with search_id: {cfg_str} ---")
        total, chunk_count = search_and_compare(rest_client, dataset_id, cfg)
        logger.info(f"SUCCESS: Python and Go responses match for {chunk_count} chunks, total={total}")


# ---------------------------------------------------------------------------
# Test Unit 4: Multi-dataset search
# ---------------------------------------------------------------------------
@pytest.mark.p2
def test_search_datasets_consistency_multi_dataset(rest_client, all_datasets):
    """
    Compare Python vs Go when searching across 2 datasets simultaneously for consistency.
    Uses ds_chinese and ds_chinese_2.
    """
    both_ids = [all_datasets["ds_chinese"], all_datasets["ds_chinese_2"]]
    logger.info(f"Using datasets (multi): ds_chinese={all_datasets['ds_chinese']}, ds_chinese_2={all_datasets['ds_chinese_2']}")

    search_configs = [
        {"question": "武松打虎"},
        {"question": "努力发展农业"},
        {"question": "曹操 宋江"},
    ]

    for cfg in search_configs:
        cfg_str = ", ".join(f"{k}={v}" for k, v in cfg.items())
        logger.info(f"\n--- Multi-dataset Testing: {cfg_str} ---")
        total, chunk_count = search_and_compare(rest_client, both_ids, cfg)
        logger.info(f"SUCCESS: Python and Go responses match for {chunk_count} chunks, total={total}")


# ---------------------------------------------------------------------------
# Test Unit 5: English search consistency
# ---------------------------------------------------------------------------
@pytest.mark.p2
def test_search_datasets_consistency_english(rest_client, all_datasets):
    """
    Compare Python vs Go with English text documents for consistency.
    Uses ds_english.
    """
    dataset_id = all_datasets["ds_english"]
    logger.info(f"Using English dataset: {dataset_id}")

    search_configs = [
        {"question": "artificial intelligence"},
        {"question": "neural networks and deep learning"},
        {"question": "neural networks", "keyword": True},
        {"question": "convolutional neural networks", "keyword": True},
        {"question": "人工智能", "cross_languages": ["English"]},
        {"question": "机器学习", "cross_languages": ["English"]},
    ]

    for cfg in search_configs:
        cfg_str = ", ".join(f"{k}={v}" for k, v in cfg.items())
        logger.info(f"\n--- Testing (English): {cfg_str} ---")
        total, chunk_count = search_and_compare(rest_client, dataset_id, cfg)
        logger.info(f"SUCCESS: Python and Go responses match for {chunk_count} chunks, total={total}")
