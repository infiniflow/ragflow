# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""
import json
import logging
import numbers
import re
import time
import traceback
from collections import defaultdict, Counter
from typing import Any, Callable, Mapping
from dataclasses import dataclass
import tiktoken

from graphrag.extractor import Extractor
from graphrag.graph_prompt import GRAPH_EXTRACTION_PROMPT, CONTINUE_PROMPT, LOOP_PROMPT
from graphrag.utils import ErrorHandlerFn, perform_variable_replacements, clean_str
from lightrag.prompt import PROMPTS, GRAPH_FIELD_SEP
from lightrag.utils import compute_args_hash, handle_single_entity_extraction, pack_user_ass_to_openai_messages, \
    split_string_by_multi_markers, is_float_regex
from rag.llm.chat_model import Base as CompletionLLM
import networkx as nx
from rag.utils import num_tokens_from_string
from timeit import default_timer as timer

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
    """Unipartite graph extractor class definition."""

    _join_descriptions: bool
    _tuple_delimiter_key: str
    _record_delimiter_key: str
    _entity_types: str
    _encoding_model: str
    _input_text_key: str
    _completion_delimiter_key: str
    _entity_name_key: str
    _input_descriptions_key: str
    _extraction_prompt: str
    _summarization_prompt: str
    _loop_args: dict[str, Any]
    _max_gleanings: int
    _on_error: ErrorHandlerFn

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        prompt: str | None = None,
        language: str | None = "English",
        entity_types: list[str] | None = None,
        example_number: int = 3,
        tuple_delimiter_key: str | None = None,
        record_delimiter_key: str | None = None,
        input_text_key: str | None = None,
        completion_delimiter_key: str | None = None,
        join_descriptions=True,
        encoding_model: str | None = None,
        max_gleanings: int | None = None,
        on_error: ErrorHandlerFn | None = None,
    ):
        """Init method definition."""
        # TODO: streamline construction
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

        self._join_descriptions = join_descriptions
        self._input_text_key = input_text_key or "input_text"
        self._tuple_delimiter_key = tuple_delimiter_key or "tuple_delimiter"
        self._record_delimiter_key = record_delimiter_key or "record_delimiter"
        self._completion_delimiter_key = (
            completion_delimiter_key or "completion_delimiter"
        )
        self._extraction_prompt = prompt or GRAPH_EXTRACTION_PROMPT
        self._on_error = on_error or (lambda _e, _s, _d: None)
        self.prompt_token_count = num_tokens_from_string(self._extraction_prompt)

        # Construct the looping arguments
        encoding = tiktoken.get_encoding(encoding_model or "cl100k_base")
        yes = encoding.encode("YES")
        no = encoding.encode("NO")
        self._loop_args = {"logit_bias": {yes[0]: 100, no[0]: 100}, "max_tokens": 1}

    def __call__(
        self, chunks: list,
            knowledge_graph_inst: nx.Graph | None = None,
            callback: Callable | None = None
    ) -> GraphExtractionResult:
        ordered_chunks = list(chunks.items())
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

        entity_extract_prompt = PROMPTS["entity_extraction"]
        context_base = dict(
            tuple_delimiter=PROMPTS["DEFAULT_TUPLE_DELIMITER"],
            record_delimiter=PROMPTS["DEFAULT_RECORD_DELIMITER"],
            completion_delimiter=PROMPTS["DEFAULT_COMPLETION_DELIMITER"],
            entity_types=",".join(self._entity_types),
            examples=examples,
            language=self._language,
        )

        continue_prompt = PROMPTS["entiti_continue_extraction"]
        if_loop_prompt = PROMPTS["entiti_if_loop_extraction"]

        already_processed = 0
        already_entities = 0
        already_relations = 0

        def _process_single_content(chunk_key_dp: tuple[str, TextChunkSchema]):
            nonlocal already_processed, already_entities, already_relations, entity_extract_prompt
            chunk_key = chunk_key_dp[0]
            chunk_dp = chunk_key_dp[1]
            content = chunk_dp["content"]
            # hint_prompt = entity_extract_prompt.format(**context_base, input_text=content)
            hint_prompt = entity_extract_prompt.format(
                **context_base, input_text="{input_text}"
            ).format(**context_base, input_text=content)

            gen_conf = {"temperature": 0.3}
            final_result = self._chat(hint_prompt, [{"role": "user", "content": "Output:"}], gen_conf)
            history = pack_user_ass_to_openai_messages(hint_prompt, final_result)
            for now_glean_index in range(self._max_gleanings):
                glean_result = self._chat(continue_prompt, history, gen_conf)
                history += pack_user_ass_to_openai_messages(continue_prompt, glean_result)
                final_result += glean_result
                if now_glean_index == self._max_gleanings - 1:
                    break

                if_loop_result = self._chat(if_loop_prompt, history, gen_conf)
                if_loop_result = if_loop_result.strip().strip('"').strip("'").lower()
                if if_loop_result != "yes":
                    break

            records = split_string_by_multi_markers(
                final_result,
                [context_base["record_delimiter"], context_base["completion_delimiter"]],
            )

            maybe_nodes = defaultdict(list)
            maybe_edges = defaultdict(list)
            for record in records:
                record = re.search(r"\((.*)\)", record)
                if record is None:
                    continue
                record = record.group(1)
                record_attributes = split_string_by_multi_markers(
                    record, [context_base["tuple_delimiter"]]
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

        results = []
        for c in ordered_chunks:
            try:
                results.append(_process_single_content(c))
            except Exception as e:
                if callback:
                    callback(msg="Knowledge graph extraction error:{}".format(str(e)))
                logging.exception("error extracting graph")

        maybe_nodes = defaultdict(list)
        maybe_edges = defaultdict(list)
        for m_nodes, m_edges in results:
            for k, v in m_nodes.items():
                maybe_nodes[k].extend(v)
            for k, v in m_edges.items():
                maybe_edges[tuple(sorted(k))].extend(v)
        logging.info("Inserting entities into storage...")
        all_entities_data = []
        for k, v in maybe_nodes.items():
            all_entities_data.append(self._merge_nodes(k, v, knowledge_graph_inst))

        logging.info("Inserting relationships into storage...")
        all_relationships_data = []
        for k, v in maybe_edges.items():
            all_relationships_data.append(self._merge_nodes(k, v, knowledge_graph_inst))

        if not len(all_entities_data) and not len(all_relationships_data):
            logging.warning(
                "Didn't extract any entities and relationships, maybe your LLM is not working"
            )
            return None

        if not len(all_entities_data):
            logging.warning("Didn't extract any entities")
        if not len(all_relationships_data):
            logging.warning("Didn't extract any relationships")

        if entity_vdb is not None:
            data_for_vdb = {
                compute_mdhash_id(dp["entity_name"], prefix="ent-"): {
                    "content": dp["entity_name"] + dp["description"],
                    "entity_name": dp["entity_name"],
                }
                for dp in all_entities_data
            }
            await entity_vdb.upsert(data_for_vdb)

        if relationships_vdb is not None:
            data_for_vdb = {
                compute_mdhash_id(dp["src_id"] + dp["tgt_id"], prefix="rel-"): {
                    "src_id": dp["src_id"],
                    "tgt_id": dp["tgt_id"],
                    "content": dp["keywords"]
                    + dp["src_id"]
                    + dp["tgt_id"]
                    + dp["description"],
                    "metadata": {
                        "created_at": dp.get("metadata", {}).get("created_at", time.time())
                    },
                }
                for dp in all_relationships_data
            }
            await relationships_vdb.upsert(data_for_vdb)

        return knowledge_graph_inst

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

    def _merge_nodes(self, knowledge_graph_inst: nx.Graph, entity_name: str, nodes_data: list[dict]):
        already_entity_types = []
        already_source_ids = []
        already_description = []

        already_node = await knowledge_graph_inst.nodes.get(entity_name)
        if already_node is not None:
            already_entity_types.append(already_node["entity_type"])
            already_source_ids.extend(
                split_string_by_multi_markers(already_node["source_id"], [GRAPH_FIELD_SEP])
            )
            already_description.append(already_node["description"])

        entity_type = sorted(
            Counter(
                [dp["entity_type"] for dp in nodes_data] + already_entity_types
            ).items(),
            key=lambda x: x[1],
            reverse=True,
        )[0][0]
        description = GRAPH_FIELD_SEP.join(
            sorted(set([dp["description"] for dp in nodes_data] + already_description))
        )
        source_id = GRAPH_FIELD_SEP.join(
            set([dp["source_id"] for dp in nodes_data] + already_source_ids)
        )
        description = self._handle_entity_relation_summary(
            entity_name, description
        )
        knowledge_graph_inst.add_node(
            entity_name,
            entity_type=entity_type,
            description=description,
            source_id=source_id,
            weight=1
        )
        node_data = dict(
            entity_type=entity_type,
            description=description,
            source_id=source_id,
        )
        node_data["entity_name"] = entity_name
        return node_data

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

    def _process_document(
        self, text: str, prompt_variables: dict[str, str]
    ) -> str:
        variables = {
            **prompt_variables,
            self._input_text_key: text,
        }
        token_count = 0
        text = perform_variable_replacements(self._extraction_prompt, variables=variables)
        gen_conf = {"temperature": 0.3}
        response = self._chat(text, [{"role": "user", "content": "Output:"}], gen_conf)
        token_count = num_tokens_from_string(text + response)

        results = response or ""
        history = [{"role": "system", "content": text}, {"role": "assistant", "content": response}]

        # Repeat to ensure we maximize entity count
        for i in range(self._max_gleanings):
            text = perform_variable_replacements(CONTINUE_PROMPT, history=history, variables=variables)
            history.append({"role": "user", "content": text})
            response = self._chat("", history, gen_conf)
            results += response or ""

            # if this is the final glean, don't bother updating the continuation flag
            if i >= self._max_gleanings - 1:
                break
            history.append({"role": "assistant", "content": response})
            history.append({"role": "user", "content": LOOP_PROMPT})
            continuation = self._chat("", history, self._loop_args)
            if continuation != "YES":
                break

        return results, token_count

    def _process_results(
        self,
        results: dict[int, str],
        tuple_delimiter: str,
        record_delimiter: str,
    ) -> nx.Graph:
        """Parse the result string to create an undirected unipartite graph.

        Args:
            - results - dict of results from the extraction chain
            - tuple_delimiter - delimiter between tuples in an output record, default is '<|>'
            - record_delimiter - delimiter between records, default is '##'
        Returns:
            - output - unipartite graph in graphML format
        """
        graph = nx.Graph()
        for source_doc_id, extracted_data in results.items():
            records = [r.strip() for r in extracted_data.split(record_delimiter)]

            for record in records:
                record = re.sub(r"^\(|\)$", "", record.strip())
                record_attributes = record.split(tuple_delimiter)

                if record_attributes[0] == '"entity"' and len(record_attributes) >= 4:
                    # add this record as a node in the G
                    entity_name = clean_str(record_attributes[1].upper())
                    entity_type = clean_str(record_attributes[2].upper())
                    entity_description = clean_str(record_attributes[3])

                    if entity_name in graph.nodes():
                        node = graph.nodes[entity_name]
                        if self._join_descriptions:
                            node["description"] = "\n".join(
                                list({
                                    *_unpack_descriptions(node),
                                    entity_description,
                                })
                            )
                        else:
                            if len(entity_description) > len(node["description"]):
                                node["description"] = entity_description
                        node["source_id"] = ", ".join(
                            list({
                                *_unpack_source_ids(node),
                                str(source_doc_id),
                            })
                        )
                        node["entity_type"] = (
                            entity_type if entity_type != "" else node["entity_type"]
                        )
                    else:
                        graph.add_node(
                            entity_name,
                            entity_type=entity_type,
                            description=entity_description,
                            source_id=str(source_doc_id),
                            weight=1
                        )

                if (
                    record_attributes[0] == '"relationship"'
                    and len(record_attributes) >= 5
                ):
                    # add this record as edge
                    source = clean_str(record_attributes[1].upper())
                    target = clean_str(record_attributes[2].upper())
                    edge_description = clean_str(record_attributes[3])
                    edge_source_id = clean_str(str(source_doc_id))
                    weight = (
                        float(record_attributes[-1])
                        if isinstance(record_attributes[-1], numbers.Number)
                        else 1.0
                    )
                    if source not in graph.nodes():
                        graph.add_node(
                            source,
                            entity_type="",
                            description="",
                            source_id=edge_source_id,
                            weight=1
                        )
                    if target not in graph.nodes():
                        graph.add_node(
                            target,
                            entity_type="",
                            description="",
                            source_id=edge_source_id,
                            weight=1
                        )
                    if graph.has_edge(source, target):
                        edge_data = graph.get_edge_data(source, target)
                        if edge_data is not None:
                            weight += edge_data["weight"]
                            if self._join_descriptions:
                                edge_description = "\n".join(
                                    list({
                                        *_unpack_descriptions(edge_data),
                                        edge_description,
                                    })
                                )
                            edge_source_id = ", ".join(
                                list({
                                    *_unpack_source_ids(edge_data),
                                    str(source_doc_id),
                                })
                            )
                    graph.add_edge(
                        source,
                        target,
                        weight=weight,
                        description=edge_description,
                        source_id=edge_source_id,
                    )

        for node_degree in graph.degree:
            graph.nodes[str(node_degree[0])]["rank"] = int(node_degree[1])
        return graph


def _unpack_descriptions(data: Mapping) -> list[str]:
    value = data.get("description", None)
    return [] if value is None else value.split("\n")


def _unpack_source_ids(data: Mapping) -> list[str]:
    value = data.get("source_id", None)
    return [] if value is None else value.split(", ")



