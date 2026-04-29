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
import random
from copy import deepcopy
from typing import Any, Dict, List, Optional

from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.quality.analyzer import (
    ChunkQualityAnalyzer,
    ChunkQualityResult,
    QualityRiskLevel,
)
from rag.flow.quality.schema import (
    BatchQualitySummaryModel,
    ChunkQualityResultModel,
    QualityAnalyzerFromUpstream,
    QualityIssueModel,
)

logger = logging.getLogger(__name__)


class QualityAnalyzerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.min_chunk_length = 20
        self.max_chunk_length = 8000
        self.min_token_count = 10
        self.max_token_count = 2000
        self.garbled_threshold = 0.3
        self.enable_checks = [
            "length",
            "token_count",
            "repetition",
            "garbled",
            "missing_title",
            "table_break",
            "header_footer_pollution",
        ]
        self.attach_quality_to_chunks = True
        self.fail_on_high_risk = False
        self.header_footer_patterns = []

    def check(self):
        super().check()
        self.check_positive_integer(self.min_chunk_length, "Min chunk length")
        self.check_positive_integer(self.max_chunk_length, "Max chunk length")
        self.check_nonnegative_number(self.garbled_threshold, "Garbled threshold")

        if self.enable_checks is None:
            self.enable_checks = []
        elif isinstance(self.enable_checks, str):
            self.enable_checks = [s.strip() for s in self.enable_checks.split(",") if s.strip()]

    def get_input_form(self) -> Dict[str, Dict]:
        return {}


class QualityAnalyzer(ProcessBase):
    component_name = "QualityAnalyzer"

    def _get_chunks_from_upstream(self, upstream_data: Dict[str, Any]) -> List[Dict[str, Any]]:
        chunks = []

        if upstream_data.get("chunks"):
            chunks = upstream_data["chunks"]
        elif upstream_data.get("json") or upstream_data.get("json_result"):
            json_data = upstream_data.get("json") or upstream_data.get("json_result")
            if json_data:
                chunks = [{"text": item.get("text", "")} for item in json_data]
        elif upstream_data.get("markdown") or upstream_data.get("markdown_result"):
            md_text = upstream_data.get("markdown") or upstream_data.get("markdown_result")
            if md_text:
                chunks = [{"text": md_text}]
        elif upstream_data.get("text") or upstream_data.get("text_result"):
            text_data = upstream_data.get("text") or upstream_data.get("text_result")
            if text_data:
                chunks = [{"text": text_data}]
        elif upstream_data.get("html") or upstream_data.get("html_result"):
            html_data = upstream_data.get("html") or upstream_data.get("html_result")
            if html_data:
                chunks = [{"text": html_data}]

        return chunks if isinstance(chunks, list) else []

    def _result_to_dict(self, result: ChunkQualityResult) -> Dict[str, Any]:
        issues_dict = []
        for issue in result.issues:
            issues_dict.append({
                "issue_type": issue.issue_type,
                "risk_level": issue.risk_level.value,
                "description": issue.description,
                "details": issue.details,
                "suggestion": issue.suggestion,
            })

        return {
            "chunk_index": result.chunk_index,
            "quality_score": result.quality_score,
            "issues": issues_dict,
            "risk_level": result.risk_level.value,
            "metadata": result.metadata,
        }

    def _has_high_risk(self, results: List[ChunkQualityResult]) -> bool:
        for result in results:
            if result.has_risk_above(QualityRiskLevel.HIGH):
                return True
        return False

    async def _invoke(self, **kwargs):
        self.set_output("output_format", "chunks")
        self.callback(random.randint(1, 5) / 100.0, "Starting chunk quality analysis...")

        try:
            from_upstream = QualityAnalyzerFromUpstream.model_validate(kwargs)
        except Exception as e:
            logger.warning(f"QualityAnalyzer schema validation failed: {e}, using raw kwargs")
            from_upstream = None

        upstream_chunks = self._get_chunks_from_upstream(kwargs)

        if not upstream_chunks:
            logger.warning("QualityAnalyzer received no chunks to analyze")
            self.set_output("chunks", [])
            self.set_output("quality_results", [])
            self.set_output("quality_summary", {})
            self.callback(1, "No chunks to analyze.")
            return

        param = self._param

        header_footer_patterns = param.header_footer_patterns or None
        if isinstance(header_footer_patterns, str):
            header_footer_patterns = [p.strip() for p in header_footer_patterns.split("|") if p.strip()]

        analyzer = ChunkQualityAnalyzer(
            min_chunk_length=param.min_chunk_length,
            max_chunk_length=param.max_chunk_length,
            min_token_count=param.min_token_count,
            max_token_count=param.max_token_count,
            garbled_threshold=param.garbled_threshold,
            header_footer_patterns=header_footer_patterns,
            enable_checks=param.enable_checks,
        )

        results = analyzer.analyze_chunks(upstream_chunks)
        summary = analyzer.get_batch_summary(results)

        results_dict = [self._result_to_dict(r) for r in results]

        output_chunks = deepcopy(upstream_chunks)
        if param.attach_quality_to_chunks:
            for i, chunk in enumerate(output_chunks):
                if i < len(results_dict):
                    chunk["_quality"] = results_dict[i]

        self.set_output("chunks", output_chunks)
        self.set_output("quality_results", results_dict)
        self.set_output("quality_summary", {
            "total_chunks": summary["total_chunks"],
            "average_quality": summary["average_quality"],
            "risk_distribution": summary["risk_distribution"],
            "issue_distribution": summary["issue_distribution"],
            "high_risk_count": summary["high_risk_count"],
            "high_risk_indices": summary["high_risk_indices"],
        })

        if from_upstream:
            if from_upstream.file:
                self.set_output("file", from_upstream.file)
            if from_upstream.name:
                self.set_output("name", from_upstream.name)

        high_risk_count = summary.get("high_risk_count", 0)
        if high_risk_count > 0:
            high_risk_msg = f"Detected {high_risk_count} high-risk chunks"
            if param.fail_on_high_risk:
                self.set_output("_ERROR", high_risk_msg)
                self.callback(-1, high_risk_msg)
                return
            else:
                self.callback(0.9, high_risk_msg)

        self.callback(1, f"Quality analysis complete. Avg quality: {summary['average_quality']:.2f}, "
                        f"high risk: {high_risk_count}/{summary['total_chunks']}")
