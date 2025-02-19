# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

import logging
import json
import re
from typing import Callable
from dataclasses import dataclass
import networkx as nx
import pandas as pd
from graphrag.general import leiden
from graphrag.general.community_report_prompt import COMMUNITY_REPORT_PROMPT
from graphrag.general.extractor import Extractor
from graphrag.general.leiden import add_community_info2graph
from rag.llm.chat_model import Base as CompletionLLM
from graphrag.utils import perform_variable_replacements, dict_has_keys_with_types
from rag.utils import num_tokens_from_string
from timeit import default_timer as timer


@dataclass
class CommunityReportsResult:
    """Community reports result class definition."""

    output: list[str]
    structured_output: list[dict]


class CommunityReportsExtractor(Extractor):
    """Community reports extractor class definition."""

    _extraction_prompt: str
    _output_formatter_prompt: str
    _max_report_length: int

    def __init__(
            self,
            llm_invoker: CompletionLLM,
            get_entity: Callable | None = None,
            set_entity: Callable | None = None,
            get_relation: Callable | None = None,
            set_relation: Callable | None = None,
            max_report_length: int | None = None,
    ):
        super().__init__(llm_invoker, get_entity=get_entity, set_entity=set_entity, get_relation=get_relation, set_relation=set_relation)
        """Init method definition."""
        self._llm = llm_invoker
        self._extraction_prompt = COMMUNITY_REPORT_PROMPT
        self._max_report_length = max_report_length or 1500

    def __call__(self, graph: nx.Graph, callback: Callable | None = None):
        for node_degree in graph.degree:
            graph.nodes[str(node_degree[0])]["rank"] = int(node_degree[1])

        communities: dict[str, dict[str, list]] = leiden.run(graph, {})
        total = sum([len(comm.items()) for _, comm in communities.items()])
        res_str = []
        res_dict = []
        over, token_count = 0, 0
        st = timer()
        for level, comm in communities.items():
            logging.info(f"Level {level}: Community: {len(comm.keys())}")
            for cm_id, ents in comm.items():
                weight = ents["weight"]
                ents = ents["nodes"]
                ent_df = pd.DataFrame(self._get_entity_(ents)).dropna()#[{"entity": n, **graph.nodes[n]} for n in ents])
                if ent_df.empty:
                    continue
                ent_df["entity"] = ent_df["entity_name"]
                del ent_df["entity_name"]
                rela_df = pd.DataFrame(self._get_relation_(list(ent_df["entity"]), list(ent_df["entity"]), 10000))
                if rela_df.empty:
                    continue
                rela_df["source"] = rela_df["src_id"]
                rela_df["target"] = rela_df["tgt_id"]
                del rela_df["src_id"]
                del rela_df["tgt_id"]

                prompt_variables = {
                    "entity_df": ent_df.to_csv(index_label="id"),
                    "relation_df": rela_df.to_csv(index_label="id")
                }
                text = perform_variable_replacements(self._extraction_prompt, variables=prompt_variables)
                gen_conf = {"temperature": 0.3}
                try:
                    response = self._chat(text, [{"role": "user", "content": "Output:"}], gen_conf)
                    token_count += num_tokens_from_string(text + response)
                    response = re.sub(r"^[^\{]*", "", response)
                    response = re.sub(r"[^\}]*$", "", response)
                    response = re.sub(r"\{\{", "{", response)
                    response = re.sub(r"\}\}", "}", response)
                    logging.debug(response)
                    response = json.loads(response)
                    if not dict_has_keys_with_types(response, [
                                ("title", str),
                                ("summary", str),
                                ("findings", list),
                                ("rating", float),
                                ("rating_explanation", str),
                            ]):
                        continue
                    response["weight"] = weight
                    response["entities"] = ents
                except Exception:
                    logging.exception("CommunityReportsExtractor got exception")
                    continue

                add_community_info2graph(graph, ents, response["title"])
                res_str.append(self._get_text_output(response))
                res_dict.append(response)
                over += 1
                if callback:
                    callback(msg=f"Communities: {over}/{total}, elapsed: {timer() - st}s, used tokens: {token_count}")

        return CommunityReportsResult(
            structured_output=res_dict,
            output=res_str,
        )

    def _get_text_output(self, parsed_output: dict) -> str:
        title = parsed_output.get("title", "Report")
        summary = parsed_output.get("summary", "")
        findings = parsed_output.get("findings", [])

        def finding_summary(finding: dict):
            if isinstance(finding, str):
                return finding
            return finding.get("summary")

        def finding_explanation(finding: dict):
            if isinstance(finding, str):
                return ""
            return finding.get("explanation")

        report_sections = "\n\n".join(
            f"## {finding_summary(f)}\n\n{finding_explanation(f)}" for f in findings
        )
        return f"# {title}\n\n{summary}\n\n{report_sections}"
