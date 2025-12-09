#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import logging
import os
import re
from collections import Counter, defaultdict
from copy import deepcopy
from typing import Callable

import networkx as nx
import trio

from api.db.services.task_service import has_canceled
from common.connection_utils import timeout
from common.token_utils import truncate
from graphrag.general.graph_prompt import SUMMARIZE_DESCRIPTIONS_PROMPT
from graphrag.utils import (
    GraphChange,
    chat_limiter,
    flat_uniq_list,
    get_from_to,
    get_llm_cache,
    handle_single_entity_extraction,
    handle_single_relationship_extraction,
    set_llm_cache,
    split_string_by_multi_markers,
)
from rag.llm.chat_model import Base as CompletionLLM
from rag.prompts.generator import message_fit_in
from common.exceptions import TaskCanceledException

GRAPH_FIELD_SEP = "<SEP>"
DEFAULT_ENTITY_TYPES = ["organization", "person", "geo", "event", "category"]
ENTITY_EXTRACTION_MAX_GLEANINGS = 2
MAX_CONCURRENT_PROCESS_AND_EXTRACT_CHUNK = int(os.environ.get("MAX_CONCURRENT_PROCESS_AND_EXTRACT_CHUNK", 10))


class Extractor:
    _llm: CompletionLLM

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        language: str | None = "English",
        entity_types: list[str] | None = None,
    ):
        self._llm = llm_invoker
        self._language = language
        self._entity_types = entity_types or DEFAULT_ENTITY_TYPES

    @timeout(60 * 20)
    def _chat(self, system, history, gen_conf={}, task_id=""):
        hist = deepcopy(history)
        conf = deepcopy(gen_conf)
        response = get_llm_cache(self._llm.llm_name, system, hist, conf)
        if response:
            return response
        _, system_msg = message_fit_in([{"role": "system", "content": system}], int(self._llm.max_length * 0.92))
        response = ""
        for attempt in range(3):

            if task_id:
                if has_canceled(task_id):
                    logging.info(f"Task {task_id} cancelled during entity resolution candidate processing.")
                    raise TaskCanceledException(f"Task {task_id} was cancelled")

            try:
                response = self._llm.chat(system_msg[0]["content"], hist, conf)
                response = re.sub(r"^.*</think>", "", response, flags=re.DOTALL)
                if response.find("**ERROR**") >= 0:
                    raise Exception(response)
                set_llm_cache(self._llm.llm_name, system, response, history, gen_conf)
            except Exception as e:
                logging.exception(e)
                if attempt == 2:
                    raise

        return response

    def _entities_and_relations(self, chunk_key: str, records: list, tuple_delimiter: str):
        maybe_nodes = defaultdict(list)
        maybe_edges = defaultdict(list)
        ent_types = [t.lower() for t in self._entity_types]
        for record in records:
            record_attributes = split_string_by_multi_markers(record, [tuple_delimiter])

            if_entities = handle_single_entity_extraction(record_attributes, chunk_key)
            if if_entities is not None and if_entities.get("entity_type", "unknown").lower() in ent_types:
                maybe_nodes[if_entities["entity_name"]].append(if_entities)
                continue

            if_relation = handle_single_relationship_extraction(record_attributes, chunk_key)
            if if_relation is not None:
                maybe_edges[(if_relation["src_id"], if_relation["tgt_id"])].append(if_relation)
        return dict(maybe_nodes), dict(maybe_edges)

    async def __call__(self, doc_id: str, chunks: list[str], callback: Callable | None = None, task_id: str = ""):
        self.callback = callback
        start_ts = trio.current_time()

        async def extract_all(doc_id, chunks, max_concurrency=MAX_CONCURRENT_PROCESS_AND_EXTRACT_CHUNK, task_id=""):
            out_results = []
            error_count = 0
            max_errors = int(os.environ.get("GRAPHRAG_MAX_ERRORS", 3))

            limiter = trio.Semaphore(max_concurrency)

            async def worker(chunk_key_dp: tuple[str, str], idx: int, total: int, task_id=""):
                nonlocal error_count
                async with limiter:

                    if task_id and has_canceled(task_id):
                        raise TaskCanceledException(f"Task {task_id} was cancelled during entity extraction")

                    try:
                        await self._process_single_content(chunk_key_dp, idx, total, out_results, task_id)
                    except Exception as e:
                        error_count += 1
                        error_msg = f"Error processing chunk {idx + 1}/{total}: {str(e)}"
                        logging.warning(error_msg)
                        if self.callback:
                            self.callback(msg=error_msg)

                        if error_count > max_errors:
                            raise Exception(f"Maximum error count ({max_errors}) reached. Last errors: {str(e)}")

            async with trio.open_nursery() as nursery:
                for i, ck in enumerate(chunks):
                    nursery.start_soon(worker, (doc_id, ck), i, len(chunks), task_id)

            if error_count > 0:
                warning_msg = f"Completed with {error_count} errors (out of {len(chunks)} chunks processed)"
                logging.warning(warning_msg)
                if self.callback:
                    self.callback(msg=warning_msg)

            return out_results

        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled before entity extraction")

        out_results = await extract_all(doc_id, chunks, max_concurrency=MAX_CONCURRENT_PROCESS_AND_EXTRACT_CHUNK, task_id=task_id)

        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled after entity extraction")

        maybe_nodes = defaultdict(list)
        maybe_edges = defaultdict(list)
        sum_token_count = 0
        for m_nodes, m_edges, token_count in out_results:
            for k, v in m_nodes.items():
                maybe_nodes[k].extend(v)
            for k, v in m_edges.items():
                maybe_edges[tuple(sorted(k))].extend(v)
            sum_token_count += token_count
        now = trio.current_time()
        if self.callback:
            self.callback(msg=f"Entities and relationships extraction done, {len(maybe_nodes)} nodes, {len(maybe_edges)} edges, {sum_token_count} tokens, {now - start_ts:.2f}s.")
        start_ts = now
        logging.info("Entities merging...")
        all_entities_data = []

        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled before nodes merging")

        async with trio.open_nursery() as nursery:
            for en_nm, ents in maybe_nodes.items():
                nursery.start_soon(self._merge_nodes, en_nm, ents, all_entities_data, task_id)

        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled after nodes merging")

        now = trio.current_time()
        if self.callback:
            self.callback(msg=f"Entities merging done, {now - start_ts:.2f}s.")

        start_ts = now
        logging.info("Relationships merging...")
        all_relationships_data = []

        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled before relationships merging")

        async with trio.open_nursery() as nursery:
            for (src, tgt), rels in maybe_edges.items():
                nursery.start_soon(self._merge_edges, src, tgt, rels, all_relationships_data, task_id)

        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled after relationships merging")

        now = trio.current_time()
        if self.callback:
            self.callback(msg=f"Relationships merging done, {now - start_ts:.2f}s.")

        if not len(all_entities_data) and not len(all_relationships_data):
            logging.warning("Didn't extract any entities and relationships, maybe your LLM is not working")

        if not len(all_entities_data):
            logging.warning("Didn't extract any entities")
        if not len(all_relationships_data):
            logging.warning("Didn't extract any relationships")

        return all_entities_data, all_relationships_data

    async def _merge_nodes(self, entity_name: str, entities: list[dict], all_relationships_data, task_id=""):
        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled during merge nodes")

        if not entities:
            return
        entity_type = sorted(
            Counter([dp["entity_type"] for dp in entities]).items(),
            key=lambda x: x[1],
            reverse=True,
        )[0][0]
        description = GRAPH_FIELD_SEP.join(sorted(set([dp["description"] for dp in entities])))
        already_source_ids = flat_uniq_list(entities, "source_id")
        description = await self._handle_entity_relation_summary(entity_name, description, task_id=task_id)
        node_data = dict(
            entity_type=entity_type,
            description=description,
            source_id=already_source_ids,
        )
        node_data["entity_name"] = entity_name
        all_relationships_data.append(node_data)

    async def _merge_edges(self, src_id: str, tgt_id: str, edges_data: list[dict], all_relationships_data=None, task_id=""):
        if not edges_data:
            return
        weight = sum([edge["weight"] for edge in edges_data])
        description = GRAPH_FIELD_SEP.join(sorted(set([edge["description"] for edge in edges_data])))
        description = await self._handle_entity_relation_summary(f"{src_id} -> {tgt_id}", description, task_id=task_id)
        keywords = flat_uniq_list(edges_data, "keywords")
        source_id = flat_uniq_list(edges_data, "source_id")
        edge_data = dict(src_id=src_id, tgt_id=tgt_id, description=description, keywords=keywords, weight=weight, source_id=source_id)
        all_relationships_data.append(edge_data)

    async def _merge_graph_nodes(self, graph: nx.Graph, nodes: list[str], change: GraphChange, task_id=""):
        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled during merge graph nodes")

        if len(nodes) <= 1:
            return
        change.added_updated_nodes.add(nodes[0])
        change.removed_nodes.update(nodes[1:])
        nodes_set = set(nodes)
        node0_attrs = graph.nodes[nodes[0]]
        node0_neighbors = set(graph.neighbors(nodes[0]))
        for node1 in nodes[1:]:
            if task_id and has_canceled(task_id):
                raise TaskCanceledException(f"Task {task_id} was cancelled during merge_graph nodes")

            # Merge two nodes, keep "entity_name", "entity_type", "page_rank" unchanged.
            node1_attrs = graph.nodes[node1]
            node0_attrs["description"] += f"{GRAPH_FIELD_SEP}{node1_attrs['description']}"
            node0_attrs["source_id"] = sorted(set(node0_attrs["source_id"] + node1_attrs["source_id"]))
            for neighbor in graph.neighbors(node1):
                change.removed_edges.add(get_from_to(node1, neighbor))
                if neighbor not in nodes_set:
                    edge1_attrs = graph.get_edge_data(node1, neighbor)
                    if neighbor in node0_neighbors:
                        # Merge two edges
                        change.added_updated_edges.add(get_from_to(nodes[0], neighbor))
                        edge0_attrs = graph.get_edge_data(nodes[0], neighbor)
                        edge0_attrs["weight"] += edge1_attrs["weight"]
                        edge0_attrs["description"] += f"{GRAPH_FIELD_SEP}{edge1_attrs['description']}"
                        for attr in ["keywords", "source_id"]:
                            edge0_attrs[attr] = sorted(set(edge0_attrs[attr] + edge1_attrs[attr]))
                        edge0_attrs["description"] = await self._handle_entity_relation_summary(f"({nodes[0]}, {neighbor})", edge0_attrs["description"], task_id=task_id)
                        graph.add_edge(nodes[0], neighbor, **edge0_attrs)
                    else:
                        graph.add_edge(nodes[0], neighbor, **edge1_attrs)
            graph.remove_node(node1)
        node0_attrs["description"] = await self._handle_entity_relation_summary(nodes[0], node0_attrs["description"], task_id=task_id)
        graph.nodes[nodes[0]].update(node0_attrs)

    async def _handle_entity_relation_summary(self, entity_or_relation_name: str, description: str, task_id="") -> str:
        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled during summary handling")

        summary_max_tokens = 512
        use_description = truncate(description, summary_max_tokens)
        description_list = use_description.split(GRAPH_FIELD_SEP)
        if len(description_list) <= 12:
            return use_description
        prompt_template = SUMMARIZE_DESCRIPTIONS_PROMPT
        context_base = dict(
            entity_name=entity_or_relation_name,
            description_list=description_list,
            language=self._language,
        )
        use_prompt = prompt_template.format(**context_base)
        logging.info(f"Trigger summary: {entity_or_relation_name}")

        if task_id and has_canceled(task_id):
            raise TaskCanceledException(f"Task {task_id} was cancelled during summary handling")

        async with chat_limiter:
            summary = await trio.to_thread.run_sync(self._chat, "", [{"role": "user", "content": use_prompt}], {}, task_id)
        return summary
