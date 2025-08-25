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
import os
import re
from dataclasses import dataclass
from typing import Any, Callable

import networkx as nx
import trio

from graphrag.general.extractor import Extractor
from rag.nlp import is_english
import editdistance
from graphrag.entity_resolution_prompt import ENTITY_RESOLUTION_PROMPT
from rag.llm.chat_model import Base as CompletionLLM
from graphrag.utils import perform_variable_replacements, chat_limiter, GraphChange

DEFAULT_RECORD_DELIMITER = "##"
DEFAULT_ENTITY_INDEX_DELIMITER = "<|>"
DEFAULT_RESOLUTION_RESULT_DELIMITER = "&&"


@dataclass
class EntityResolutionResult:
    """Entity resolution result class definition."""
    graph: nx.Graph
    change: GraphChange


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
    ):
        super().__init__(llm_invoker)
        """Init method definition."""
        self._llm = llm_invoker
        self._resolution_prompt = ENTITY_RESOLUTION_PROMPT
        self._record_delimiter_key = "record_delimiter"
        self._entity_index_dilimiter_key = "entity_index_delimiter"
        self._resolution_result_delimiter_key = "resolution_result_delimiter"
        self._input_text_key = "input_text"

    async def __call__(self, graph: nx.Graph,
                       subgraph_nodes: set[str],
                       prompt_variables: dict[str, Any] | None = None,
                       callback: Callable | None = None) -> EntityResolutionResult:
        """Call method definition."""
        if prompt_variables is None:
            prompt_variables = {}

        # Wire defaults into the prompt variables
        self.prompt_variables = {
            **prompt_variables,
            self._record_delimiter_key: prompt_variables.get(self._record_delimiter_key)
                                        or DEFAULT_RECORD_DELIMITER,
            self._entity_index_dilimiter_key: prompt_variables.get(self._entity_index_dilimiter_key)
                                              or DEFAULT_ENTITY_INDEX_DELIMITER,
            self._resolution_result_delimiter_key: prompt_variables.get(self._resolution_result_delimiter_key)
                                                   or DEFAULT_RESOLUTION_RESULT_DELIMITER,
        }

        nodes = sorted(graph.nodes())
        entity_types = sorted(set(graph.nodes[node].get('entity_type', '-') for node in nodes))
        node_clusters = {entity_type: [] for entity_type in entity_types}

        for node in nodes:
            node_clusters[graph.nodes[node].get('entity_type', '-')].append(node)

        candidate_resolution = {entity_type: [] for entity_type in entity_types}
        for k, v in node_clusters.items():
            candidate_resolution[k] = [(a, b) for a, b in itertools.combinations(v, 2) if (a in subgraph_nodes or b in subgraph_nodes) and self.is_similarity(a, b)]
        num_candidates = sum([len(candidates) for _, candidates in candidate_resolution.items()])
        callback(msg=f"Identified {num_candidates} candidate pairs")
        remain_candidates_to_resolve = num_candidates

        resolution_result = set()
        resolution_result_lock = trio.Lock()
        resolution_batch_size = 100
        max_concurrent_tasks = 5
        semaphore = trio.Semaphore(max_concurrent_tasks)

        async def limited_resolve_candidate(candidate_batch, result_set, result_lock):
            nonlocal remain_candidates_to_resolve, callback
            async with semaphore:
                try:
                    enable_timeout_assertion = os.environ.get("ENABLE_TIMEOUT_ASSERTION")
                    with trio.move_on_after(280 if enable_timeout_assertion else 1000000000) as cancel_scope:
                        await self._resolve_candidate(candidate_batch, result_set, result_lock)
                        remain_candidates_to_resolve = remain_candidates_to_resolve - len(candidate_batch[1])
                        callback(msg=f"Resolved {len(candidate_batch[1])} pairs, {remain_candidates_to_resolve} are remained to resolve. ")
                    if cancel_scope.cancelled_caught:
                        logging.warning(f"Timeout resolving {candidate_batch}, skipping...")
                        remain_candidates_to_resolve = remain_candidates_to_resolve - len(candidate_batch[1])
                        callback(msg=f"Fail to resolved {len(candidate_batch[1])} pairs due to timeout reason, skipped. {remain_candidates_to_resolve} are remained to resolve. ")
                except Exception as e:
                    logging.error(f"Error resolving candidate batch: {e}")


        async with trio.open_nursery() as nursery:
            for candidate_resolution_i in candidate_resolution.items():
                if not candidate_resolution_i[1]:
                    continue
                for i in range(0, len(candidate_resolution_i[1]), resolution_batch_size):
                    candidate_batch = candidate_resolution_i[0], candidate_resolution_i[1][i:i + resolution_batch_size]
                    nursery.start_soon(limited_resolve_candidate, candidate_batch, resolution_result, resolution_result_lock)

        callback(msg=f"Resolved {num_candidates} candidate pairs, {len(resolution_result)} of them are selected to merge.")

        change = GraphChange()
        connect_graph = nx.Graph()
        connect_graph.add_edges_from(resolution_result)

        async def limited_merge_nodes(graph, nodes, change):
            async with semaphore:
                await self._merge_graph_nodes(graph, nodes, change)

        async with trio.open_nursery() as nursery:
            for sub_connect_graph in nx.connected_components(connect_graph):
                merging_nodes = list(sub_connect_graph)
                nursery.start_soon(limited_merge_nodes, graph, merging_nodes, change)

        # Update pagerank
        pr = nx.pagerank(graph)
        for node_name, pagerank in pr.items():
            graph.nodes[node_name]["pagerank"] = pagerank

        return EntityResolutionResult(
            graph=graph,
            change=change,
        )

    async def _resolve_candidate(self, candidate_resolution_i: tuple[str, list[tuple[str, str]]], resolution_result: set[str], resolution_result_lock: trio.Lock):
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
            **self.prompt_variables,
            self._input_text_key: pair_prompt
        }
        text = perform_variable_replacements(self._resolution_prompt, variables=variables)
        logging.info(f"Created resolution prompt {len(text)} bytes for {len(candidate_resolution_i[1])} entity pairs of type {candidate_resolution_i[0]}")
        async with chat_limiter:
            try:
                enable_timeout_assertion = os.environ.get("ENABLE_TIMEOUT_ASSERTION")
                with trio.move_on_after(280 if enable_timeout_assertion else 1000000000) as cancel_scope:
                    response = await trio.to_thread.run_sync(self._chat, text, [{"role": "user", "content": "Output:"}], {})
                if cancel_scope.cancelled_caught:
                    logging.warning("_resolve_candidate._chat timeout, skipping...")
                    return
            except Exception as e:
                logging.error(f"_resolve_candidate._chat failed: {e}")
                return

        logging.debug(f"_resolve_candidate chat prompt: {text}\nchat response: {response}")
        result = self._process_results(len(candidate_resolution_i[1]), response,
                                       self.prompt_variables.get(self._record_delimiter_key,
                                                            DEFAULT_RECORD_DELIMITER),
                                       self.prompt_variables.get(self._entity_index_dilimiter_key,
                                                            DEFAULT_ENTITY_INDEX_DELIMITER),
                                       self.prompt_variables.get(self._resolution_result_delimiter_key,
                                                            DEFAULT_RESOLUTION_RESULT_DELIMITER))
        async with resolution_result_lock:
            for result_i in result:
                resolution_result.add(candidate_resolution_i[1][result_i[0] - 1])

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

    def _has_digit_in_2gram_diff(self, a, b):
        def to_2gram_set(s):
            return {s[i:i+2] for i in range(len(s) - 1)}

        set_a = to_2gram_set(a)
        set_b = to_2gram_set(b)
        diff = set_a ^ set_b

        return any(any(c.isdigit() for c in pair) for pair in diff)

    def is_similarity(self, a, b):
        if self._has_digit_in_2gram_diff(a, b):
            return False

        if is_english(a) and is_english(b):
            if editdistance.eval(a, b) <= min(len(a), len(b)) // 2:
                return True
            return False

        a, b = set(a), set(b)
        max_l = max(len(a), len(b))
        if max_l < 4:
            return len(a & b) > 1

        return len(a & b)*1./max_l >= 0.8

