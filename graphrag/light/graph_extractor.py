# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""
import logging
import os
import re
import time
from collections import defaultdict, Counter
from concurrent.futures import ThreadPoolExecutor
from typing import Any, Callable
from dataclasses import dataclass
import tiktoken

from graphrag.general.extractor import Extractor
from graphrag.light.prompt import PROMPTS, GRAPH_FIELD_SEP
from graphrag.utils import pack_user_ass_to_openai_messages, split_string_by_multi_markers, \
    handle_single_entity_extraction, clean_str, is_float_regex
from rag.llm.chat_model import Base as CompletionLLM
import networkx as nx
from rag.utils import num_tokens_from_string

DEFAULT_TUPLE_DELIMITER = "<|>"
DEFAULT_RECORD_DELIMITER = "##"
DEFAULT_COMPLETION_DELIMITER = "<|COMPLETE|>"
DEFAULT_ENTITY_TYPES = ["organization", "person", "location", "event", "time"]
ENTITY_EXTRACTION_MAX_GLEANINGS = 1


@dataclass
class GraphExtractionResult:
    """Unipartite graph extraction result class definition."""

    output: nx.Graph
    source_docs: dict[Any, Any]


class GraphExtractor(Extractor):

    _encoding_model: str
    _max_gleanings: int

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        language: str | None = "English",
        entity_types: list[str] | None = None,
        example_number: int = 3,
        get_entity: Callable | None = None,
        set_entity: Callable | None = None,
        get_relation: Callable | None = None,
        set_relation: Callable | None = None,
        encoding_model: str | None = None,
        max_gleanings: int | None = None,
    ):
        """Init method definition."""
        self._llm = llm_invoker
        self._language = language
        self._entity_types = entity_types or PROMPTS["DEFAULT_ENTITY_TYPES"]
        self._max_gleanings = (
            max_gleanings
            if max_gleanings is not None
            else ENTITY_EXTRACTION_MAX_GLEANINGS
        )
        self._example_number = example_number
        self._encoding_model = encoding_model
        self._get_entity_ = get_entity
        self._set_entity_ = set_entity
        self._get_relation_ = get_relation
        self._set_relation_ = set_relation
        examples = "\n".join(
                PROMPTS["entity_extraction_examples"][: int(self._example_number)]
            )

        example_context_base = dict(
            tuple_delimiter=PROMPTS["DEFAULT_TUPLE_DELIMITER"],
            record_delimiter=PROMPTS["DEFAULT_RECORD_DELIMITER"],
            completion_delimiter=PROMPTS["DEFAULT_COMPLETION_DELIMITER"],
            entity_types=",".join(self._entity_types),
            language=self._language,
        )
        # add example's format
        examples = examples.format(**example_context_base)

        self._entity_extract_prompt = PROMPTS["entity_extraction"]
        self._context_base = dict(
            tuple_delimiter=PROMPTS["DEFAULT_TUPLE_DELIMITER"],
            record_delimiter=PROMPTS["DEFAULT_RECORD_DELIMITER"],
            completion_delimiter=PROMPTS["DEFAULT_COMPLETION_DELIMITER"],
            entity_types=",".join(self._entity_types),
            examples=examples,
            language=self._language,
        )

        self._continue_prompt = PROMPTS["entiti_continue_extraction"]
        self._if_loop_prompt = PROMPTS["entiti_if_loop_extraction"]

        self._left_token_count = llm_invoker.max_length - num_tokens_from_string(
            self._entity_extract_prompt.format(
                **self._context_base, input_text="{input_text}"
            ).format(**self._context_base, input_text="")
        )
        self._left_token_count = max(llm_invoker.max_length * 0.6, self._left_token_count)

    def __call__(
        self, chunks: list[tuple[str, str]],
            callback: Callable | None = None
    ) -> GraphExtractionResult:

        already_processed = 0
        already_entities = 0
        already_relations = 0

        def _process_single_content(chunk_key_dp: tuple[str, str]):
            nonlocal already_processed, already_entities, already_relations
            try:
                chunk_key = chunk_key_dp[0]
                content = chunk_key_dp[1]
                hint_prompt = self._entity_extract_prompt.format(
                    **self._context_base, input_text="{input_text}"
                ).format(**self._context_base, input_text=content)

                gen_conf = {"temperature": 0.3}
                final_result = self._chat(hint_prompt, [{"role": "user", "content": "Output:"}], gen_conf)
                history = pack_user_ass_to_openai_messages(hint_prompt, final_result)
                for now_glean_index in range(self._max_gleanings):
                    glean_result = self._chat(self._continue_prompt, history, gen_conf)
                    history += pack_user_ass_to_openai_messages(self._continue_prompt, glean_result)
                    final_result += glean_result
                    if now_glean_index == self._max_gleanings - 1:
                        break

                    if_loop_result = self._chat(self._if_loop_prompt, history, gen_conf)
                    if_loop_result = if_loop_result.strip().strip('"').strip("'").lower()
                    if if_loop_result != "yes":
                        break

                records = split_string_by_multi_markers(
                    final_result,
                    [self._context_base["record_delimiter"], self._context_base["completion_delimiter"]],
                )

                maybe_nodes = defaultdict(list)
                maybe_edges = defaultdict(list)
                for record in records:
                    record = re.search(r"\((.*)\)", record)
                    if record is None:
                        continue
                    record = record.group(1)
                    record_attributes = split_string_by_multi_markers(
                        record, [self._context_base["tuple_delimiter"]]
                    )
                    if_entities = handle_single_entity_extraction(
                        record_attributes, chunk_key
                    )
                    if if_entities is not None:
                        maybe_nodes[if_entities["entity_name"]].append(if_entities)
                        continue

                    if_relation = self._handle_single_relationship_extraction(
                        record_attributes, chunk_key
                    )
                    if if_relation is not None:
                        maybe_edges[(if_relation["src_id"], if_relation["tgt_id"])].append(
                            if_relation
                        )
                already_processed += 1
                already_entities += len(maybe_nodes)
                already_relations += len(maybe_edges)
                now_ticks = PROMPTS["process_tickers"][
                    already_processed % len(PROMPTS["process_tickers"])
                ]
                if callback:
                    callback(msg=f"{now_ticks} Processed {already_processed} chunks, {already_entities} entities(duplicated), {already_relations} relations(duplicated)\n")
                return dict(maybe_nodes), dict(maybe_edges)
            except Exception as e:
                if callback:
                    callback(msg="Knowledge graph extraction error:{}".format(str(e)))
                logging.exception("error extracting graph")
                return e

        results = []
        max_workers = int(os.environ.get('GRAPH_EXTRACTOR_MAX_WORKERS', 50))
        with ThreadPoolExecutor(max_workers=max_workers) as exe:
            threads = []
            for i, (cid, ck) in enumerate(chunks):
                threads.append(
                    exe.submit(_process_single_content, (cid, ck)))

            for i, _ in enumerate(threads):
                o = _.result()
                if not isinstance(o, Exception):
                    results.append(o)
                    if callback:
                        callback(0.5 + 0.1 * i / len(threads), f"Entities extraction progress ... {i + 1}/{len(threads)}")

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

    def _handle_single_relationship_extraction(self, record_attributes: list[str], chunk_key: str):
        if len(record_attributes) < 5 or record_attributes[0] != '"relationship"':
            return None
        # add this record as edge
        source = clean_str(record_attributes[1].upper())
        target = clean_str(record_attributes[2].upper())
        edge_description = clean_str(record_attributes[3])

        edge_keywords = clean_str(record_attributes[4])
        edge_source_id = chunk_key
        weight = (
            float(record_attributes[-1]) if is_float_regex(record_attributes[-1]) else 1.0
        )
        return dict(
            src_id=source,
            tgt_id=target,
            weight=weight,
            description=edge_description,
            keywords=edge_keywords,
            source_id=edge_source_id,
            metadata={"created_at": time.time()},
        )

    def _merge_nodes(self, entity_name: str, entities: list[dict]):
        already_entity_types = []
        already_source_ids = []
        already_description = []

        already_node = self._get_entity_(entity_name)
        if already_node is not None:
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
        already_source_ids.extend(set([dp["source_id"] for dp in entities]))
        already_source_ids = list(set(already_source_ids))
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
        already_weights = []
        already_source_ids = []
        already_description = []
        already_keywords = []

        relation = self._get_relation_(src_id, tgt_id)
        if relation is not None:
            already_weights = [relation["weight"]]
            already_source_ids = relation["source_id"]
            already_description = [relation["description"]]
            already_keywords = relation["keywords"]

        weight = sum([dp["weight"] for dp in edges_data] + already_weights)
        description = GRAPH_FIELD_SEP.join(
            sorted(set([dp["description"] for dp in edges_data] + already_description))
        )
        keywords = list(set([dp["keywords"] for dp in edges_data] + already_keywords))

        source_id = list(set([dp["source_id"] for dp in edges_data] + already_source_ids))

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
        llm_max_tokens = 512
        summary_max_tokens = 512

        encoder = tiktoken.get_encoding(self._encoding_model or "cl100k_base")
        tokens = encoder.encode(description)
        if len(tokens) < summary_max_tokens:  # No need for summary
            return description
        prompt_template = PROMPTS["summarize_entity_descriptions"]
        use_description = encoder.decode(tokens[:llm_max_tokens])
        context_base = dict(
            entity_name=entity_or_relation_name,
            description_list=use_description.split(GRAPH_FIELD_SEP),
            language=self._language,
        )
        use_prompt = prompt_template.format(**context_base)
        logging.info(f"Trigger summary: {entity_or_relation_name}")
        summary = self._chat(use_prompt, [{"role": "assistanb", "content": "Output: "}], {"temperature": 0.8})
        return summary
