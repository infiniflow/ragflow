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
import itertools
import re
import time
from dataclasses import dataclass
from typing import Any, Callable

import networkx as nx

from graphrag.general.extractor import Extractor
from rag.nlp import is_english
import editdistance
from graphrag.entity_resolution_prompt import ENTITY_RESOLUTION_PROMPT
from rag.llm.chat_model import Base as CompletionLLM
from graphrag.utils import perform_variable_replacements

DEFAULT_RECORD_DELIMITER = "##"
DEFAULT_ENTITY_INDEX_DELIMITER = "<|>"
DEFAULT_RESOLUTION_RESULT_DELIMITER = "&&"


@dataclass
class EntityResolutionResult:
    """Entity resolution result class definition."""
    graph: nx.Graph
    removed_entities: list


class EntityResolution(Extractor):
    """Entity resolution class definition."""

    _resolution_prompt: str
    _output_formatter_prompt: str
    _record_delimiter_key: str
    _entity_index_delimiter_key: str
    _resolution_result_delimiter_key: str

    def __init__(
            self,
            llm_invoker: CompletionLLM,
            get_entity: Callable | None = None,
            set_entity: Callable | None = None,
            get_relation: Callable | None = None,
            set_relation: Callable | None = None
    ):
        super().__init__(llm_invoker, get_entity=get_entity, set_entity=set_entity, get_relation=get_relation, set_relation=set_relation)
        """Init method definition."""
        self._llm = llm_invoker
        self._resolution_prompt = ENTITY_RESOLUTION_PROMPT
        self._record_delimiter_key = "record_delimiter"
        self._entity_index_dilimiter_key = "entity_index_delimiter"
        self._resolution_result_delimiter_key = "resolution_result_delimiter"
        self._input_text_key = "input_text"

    def __call__(self, graph: nx.Graph, prompt_variables: dict[str, Any] | None = None) -> EntityResolutionResult:
        """Call method definition."""
        if prompt_variables is None:
            prompt_variables = {}

        # Wire defaults into the prompt variables
        prompt_variables = {
            **prompt_variables,
            self._record_delimiter_key: prompt_variables.get(self._record_delimiter_key)
                                        or DEFAULT_RECORD_DELIMITER,
            self._entity_index_dilimiter_key: prompt_variables.get(self._entity_index_dilimiter_key)
                                              or DEFAULT_ENTITY_INDEX_DELIMITER,
            self._resolution_result_delimiter_key: prompt_variables.get(self._resolution_result_delimiter_key)
                                                   or DEFAULT_RESOLUTION_RESULT_DELIMITER,
        }

        nodes = graph.nodes
        entity_types = list(set(graph.nodes[node].get('entity_type', '-') for node in nodes))
        node_clusters = {entity_type: [] for entity_type in entity_types}

        for node in nodes:
            node_clusters[graph.nodes[node].get('entity_type', '-')].append(node)

        candidate_resolution = {entity_type: [] for entity_type in entity_types}
        for k, v in node_clusters.items():
            candidate_resolution[k] = [(a, b) for a, b in itertools.combinations(v, 2) if self.is_similarity(a, b)]

        gen_conf = {"temperature": 0.5}
        resolution_result = set()
        for candidate_resolution_i in candidate_resolution.items():
            if candidate_resolution_i[1]:
                try:
                    pair_txt = [
                        f'When determining whether two {candidate_resolution_i[0]}s are the same, you should only focus on critical properties and overlook noisy factors.\n']
                    for index, candidate in enumerate(candidate_resolution_i[1]):
                        pair_txt.append(
                            f'Question {index + 1}: name of{candidate_resolution_i[0]} A is {candidate[0]} ,name of{candidate_resolution_i[0]} B is {candidate[1]}')
                    sent = 'question above' if len(pair_txt) == 1 else f'above {len(pair_txt)} questions'
                    pair_txt.append(
                        f'\nUse domain knowledge of {candidate_resolution_i[0]}s to help understand the text and answer the {sent} in the format: For Question i, Yes, {candidate_resolution_i[0]} A and {candidate_resolution_i[0]} B are the same {candidate_resolution_i[0]}./No, {candidate_resolution_i[0]} A and {candidate_resolution_i[0]} B are different {candidate_resolution_i[0]}s. For Question i+1, (repeat the above procedures)')
                    pair_prompt = '\n'.join(pair_txt)

                    variables = {
                        **prompt_variables,
                        self._input_text_key: pair_prompt
                    }
                    text = perform_variable_replacements(self._resolution_prompt, variables=variables)

                    response = self._chat(text, [{"role": "user", "content": "Output:"}], gen_conf)
                    result = self._process_results(len(candidate_resolution_i[1]), response,
                                                   prompt_variables.get(self._record_delimiter_key,
                                                                        DEFAULT_RECORD_DELIMITER),
                                                   prompt_variables.get(self._entity_index_dilimiter_key,
                                                                        DEFAULT_ENTITY_INDEX_DELIMITER),
                                                   prompt_variables.get(self._resolution_result_delimiter_key,
                                                                        DEFAULT_RESOLUTION_RESULT_DELIMITER))
                    for result_i in result:
                        resolution_result.add(candidate_resolution_i[1][result_i[0] - 1])
                except Exception:
                    logging.exception("error entity resolution")

        connect_graph = nx.Graph()
        removed_entities = []
        connect_graph.add_edges_from(resolution_result)
        for sub_connect_graph in nx.connected_components(connect_graph):
            sub_connect_graph = connect_graph.subgraph(sub_connect_graph)
            remove_nodes = list(sub_connect_graph.nodes)
            keep_node = remove_nodes.pop()
            self._merge_nodes(keep_node, self._get_entity_(remove_nodes))
            for remove_node in remove_nodes:
                removed_entities.append(remove_node)
                remove_node_neighbors = graph[remove_node]
                remove_node_neighbors = list(remove_node_neighbors)
                for remove_node_neighbor in remove_node_neighbors:
                    rel = self._get_relation_(remove_node, remove_node_neighbor)
                    if graph.has_edge(remove_node, remove_node_neighbor):
                        graph.remove_edge(remove_node, remove_node_neighbor)
                    if remove_node_neighbor == keep_node:
                        if graph.has_edge(keep_node, remove_node):
                            graph.remove_edge(keep_node, remove_node)
                        continue
                    if not rel:
                        continue
                    if graph.has_edge(keep_node, remove_node_neighbor):
                        self._merge_edges(keep_node, remove_node_neighbor, [rel])
                    else:
                        pair = sorted([keep_node, remove_node_neighbor])
                        graph.add_edge(pair[0], pair[1], weight=rel['weight'])
                        self._set_relation_(pair[0], pair[1],
                                           dict(
                                                src_id=pair[0],
                                                tgt_id=pair[1],
                                                weight=rel['weight'],
                                                description=rel['description'],
                                                keywords=[],
                                                source_id=rel.get("source_id", ""),
                                                metadata={"created_at": time.time()}
                                           ))
                graph.remove_node(remove_node)

        return EntityResolutionResult(
            graph=graph,
            removed_entities=removed_entities
        )

    def _process_results(
            self,
            records_length: int,
            results: str,
            record_delimiter: str,
            entity_index_delimiter: str,
            resolution_result_delimiter: str
    ) -> list:
        ans_list = []
        records = [r.strip() for r in results.split(record_delimiter)]
        for record in records:
            pattern_int = f"{re.escape(entity_index_delimiter)}(\d+){re.escape(entity_index_delimiter)}"
            match_int = re.search(pattern_int, record)
            res_int = int(str(match_int.group(1) if match_int else '0'))
            if res_int > records_length:
                continue

            pattern_bool = f"{re.escape(resolution_result_delimiter)}([a-zA-Z]+){re.escape(resolution_result_delimiter)}"
            match_bool = re.search(pattern_bool, record)
            res_bool = str(match_bool.group(1) if match_bool else '')

            if res_int and res_bool:
                if res_bool.lower() == 'yes':
                    ans_list.append((res_int, "yes"))

        return ans_list

    def is_similarity(self, a, b):
        if is_english(a) and is_english(b):
            if editdistance.eval(a, b) <= min(len(a), len(b)) // 2:
                return True

        if len(set(a) & set(b)) > 0:
            return True

        return False
