#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
from collections import defaultdict, Counter
from concurrent.futures import ThreadPoolExecutor
from copy import deepcopy
from typing import Callable

from graphrag.general.graph_prompt import SUMMARIZE_DESCRIPTIONS_PROMPT
from graphrag.utils import get_llm_cache, set_llm_cache, handle_single_entity_extraction, \
    handle_single_relationship_extraction, split_string_by_multi_markers, flat_uniq_list
from rag.llm.chat_model import Base as CompletionLLM
from rag.utils import truncate

GRAPH_FIELD_SEP = "<SEP>"
DEFAULT_ENTITY_TYPES = ["organization", "person", "geo", "event", "category"]
ENTITY_EXTRACTION_MAX_GLEANINGS = 2


class Extractor:
    _llm: CompletionLLM

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        language: str | None = "English",
        entity_types: list[str] | None = None,
        get_entity: Callable | None = None,
        set_entity: Callable | None = None,
        get_relation: Callable | None = None,
        set_relation: Callable | None = None,
    ):
        self._llm = llm_invoker
        self._language = language
        self._entity_types = entity_types or DEFAULT_ENTITY_TYPES
        self._get_entity_ = get_entity
        self._set_entity_ = set_entity
        self._get_relation_ = get_relation
        self._set_relation_ = set_relation

    def _chat(self, system, history, gen_conf):
        hist = deepcopy(history)
        conf = deepcopy(gen_conf)
        response = get_llm_cache(self._llm.llm_name, system, hist, conf)
        if response:
            return response
        response = self._llm.chat(system, hist, conf)
        if response.find("**ERROR**") >= 0:
            raise Exception(response)
        set_llm_cache(self._llm.llm_name, system, response, history, gen_conf)
        return response

    def _entities_and_relations(self, chunk_key: str, records: list, tuple_delimiter: str):
        maybe_nodes = defaultdict(list)
        maybe_edges = defaultdict(list)
        ent_types = [t.lower() for t in self._entity_types]
        for record in records:
            record_attributes = split_string_by_multi_markers(
                record, [tuple_delimiter]
            )

            if_entities = handle_single_entity_extraction(
                record_attributes, chunk_key
            )
            if if_entities is not None and if_entities.get("entity_type", "unknown").lower() in ent_types:
                maybe_nodes[if_entities["entity_name"]].append(if_entities)
                continue

            if_relation = handle_single_relationship_extraction(
                record_attributes, chunk_key
            )
            if if_relation is not None:
                maybe_edges[(if_relation["src_id"], if_relation["tgt_id"])].append(
                    if_relation
                )
        return dict(maybe_nodes), dict(maybe_edges)

    def __call__(
        self, chunks: list[tuple[str, str]],
            callback: Callable | None = None
    ):

        results = []
        max_workers = int(os.environ.get('GRAPH_EXTRACTOR_MAX_WORKERS', 50))
        with ThreadPoolExecutor(max_workers=max_workers) as exe:
            threads = []
            for i, (cid, ck) in enumerate(chunks):
                ck = truncate(ck, int(self._llm.max_length*0.8))
                threads.append(
                    exe.submit(self._process_single_content, (cid, ck)))

            for i, _ in enumerate(threads):
                n, r, tc = _.result()
                if not isinstance(n, Exception):
                    results.append((n, r))
                    if callback:
                        callback(0.5 + 0.1 * i / len(threads), f"Entities extraction progress ... {i + 1}/{len(threads)} ({tc} tokens)")
                elif callback:
                    callback(msg="Knowledge graph extraction error:{}".format(str(n)))

        maybe_nodes = defaultdict(list)
        maybe_edges = defaultdict(list)
        for m_nodes, m_edges in results:
            for k, v in m_nodes.items():
                maybe_nodes[k].extend(v)
            for k, v in m_edges.items():
                maybe_edges[tuple(sorted(k))].extend(v)
        logging.info("Inserting entities into storage...")
        all_entities_data = []
        for en_nm, ents in maybe_nodes.items():
            all_entities_data.append(self._merge_nodes(en_nm, ents))

        logging.info("Inserting relationships into storage...")
        all_relationships_data = []
        for (src,tgt), rels in maybe_edges.items():
            all_relationships_data.append(self._merge_edges(src, tgt, rels))

        if not len(all_entities_data) and not len(all_relationships_data):
            logging.warning(
                "Didn't extract any entities and relationships, maybe your LLM is not working"
            )

        if not len(all_entities_data):
            logging.warning("Didn't extract any entities")
        if not len(all_relationships_data):
            logging.warning("Didn't extract any relationships")

        return all_entities_data, all_relationships_data

    def _merge_nodes(self, entity_name: str, entities: list[dict]):
        if not entities:
            return
        already_entity_types = []
        already_source_ids = []
        already_description = []

        already_node = self._get_entity_(entity_name)
        if already_node:
            already_entity_types.append(already_node["entity_type"])
            already_source_ids.extend(already_node["source_id"])
            already_description.append(already_node["description"])

        entity_type = sorted(
            Counter(
                [dp["entity_type"] for dp in entities] + already_entity_types
            ).items(),
            key=lambda x: x[1],
            reverse=True,
        )[0][0]
        description = GRAPH_FIELD_SEP.join(
            sorted(set([dp["description"] for dp in entities] + already_description))
        )
        already_source_ids = flat_uniq_list(entities, "source_id")
        description = self._handle_entity_relation_summary(
            entity_name, description
        )
        node_data = dict(
            entity_type=entity_type,
            description=description,
            source_id=already_source_ids,
        )
        node_data["entity_name"] = entity_name
        self._set_entity_(entity_name, node_data)
        return node_data

    def _merge_edges(
            self,
            src_id: str,
            tgt_id: str,
            edges_data: list[dict]
    ):
        if not edges_data:
            return
        already_weights = []
        already_source_ids = []
        already_description = []
        already_keywords = []

        relation = self._get_relation_(src_id, tgt_id)
        if relation:
            already_weights = [relation["weight"]]
            already_source_ids = relation["source_id"]
            already_description = [relation["description"]]
            already_keywords = relation["keywords"]

        weight = sum([dp["weight"] for dp in edges_data] + already_weights)
        description = GRAPH_FIELD_SEP.join(
            sorted(set([dp["description"] for dp in edges_data] + already_description))
        )
        keywords = flat_uniq_list(edges_data, "keywords") + already_keywords
        source_id = flat_uniq_list(edges_data, "source_id") + already_source_ids

        for need_insert_id in [src_id, tgt_id]:
            if self._get_entity_(need_insert_id):
                continue
            self._set_entity_(need_insert_id, {
                        "source_id": source_id,
                        "description": description,
                        "entity_type": 'UNKNOWN'
                    })
        description = self._handle_entity_relation_summary(
            f"({src_id}, {tgt_id})", description
        )
        edge_data = dict(
            src_id=src_id,
            tgt_id=tgt_id,
            description=description,
            keywords=keywords,
            weight=weight,
            source_id=source_id
        )
        self._set_relation_(src_id, tgt_id, edge_data)

        return edge_data

    def _handle_entity_relation_summary(
            self,
            entity_or_relation_name: str,
            description: str
    ) -> str:
        summary_max_tokens = 512
        use_description = truncate(description, summary_max_tokens)
        prompt_template = SUMMARIZE_DESCRIPTIONS_PROMPT
        context_base = dict(
            entity_name=entity_or_relation_name,
            description_list=use_description.split(GRAPH_FIELD_SEP),
            language=self._language,
        )
        use_prompt = prompt_template.format(**context_base)
        logging.info(f"Trigger summary: {entity_or_relation_name}")
        summary = self._chat(use_prompt, [{"role": "user", "content": "Output: "}], {"temperature": 0.8})
        return summary
