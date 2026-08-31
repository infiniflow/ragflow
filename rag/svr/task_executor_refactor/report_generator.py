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

"""
Report Generator Module.

Provides data classes for comparison result reporting:
- [`ComparisonResult`](rag/svr/task_executor_refactor/report_generator.py:40): Single key comparison result
- [`ComparisonReport`](rag/svr/task_executor_refactor/report_generator.py:66): Full comparison report with serialization
"""

from dataclasses import dataclass, field
from typing import Any, List, Optional


@dataclass
class ComparisonResult:
    """Result of comparing a single key between two contexts.

    Attributes:
        key: The key being compared.
        match: Whether the values match.
        production_value: Value from production context.
        dry_run_value: Value from dry-run context.
        diff_details: Optional description of the difference.
    """

    key: str
    match: bool
    production_value: Any = None
    dry_run_value: Any = None
    diff_details: Optional[str] = None

    def to_dict(self) -> dict:
        """Convert to dictionary for serialization."""
        return {
            "key": self.key,
            "match": self.match,
            "diff_details": self.diff_details,
        }


@dataclass
class ComparisonReport:
    """Report of comparing two RecordingContext instances.

    Attributes:
        task_id: The task identifier.
        total_keys: Total number of keys compared.
        matched_keys: Number of keys that matched.
        mismatched_keys: Number of keys that mismatched.
        missing_in_production: Keys missing in production context.
        missing_in_dry_run: Keys missing in dry-run context.
        details: List of individual comparison results.
    """

    task_id: str
    total_keys: int = 0
    matched_keys: int = 0
    mismatched_keys: int = 0
    missing_in_production: List[str] = field(default_factory=list)
    missing_in_dry_run: List[str] = field(default_factory=list)
    details: List["ComparisonResult"] = field(default_factory=list)

    def summary(self) -> str:
        """Generate a summary string.

        Returns:
            A human-readable summary of the comparison.
        """
        if self.total_keys == 0:
            return f"Task {self.task_id}: No keys to compare"
        match_rate = (self.matched_keys / self.total_keys) * 100
        return f"Task {self.task_id}: {self.matched_keys}/{self.total_keys} keys matched ({match_rate:.1f}%)"

    def to_dict(self) -> dict:
        """Convert to dictionary for serialization.

        Returns:
            A dictionary representation of the report.
        """
        return {
            "task_id": self.task_id,
            "total_keys": self.total_keys,
            "matched_keys": self.matched_keys,
            "mismatched_keys": self.mismatched_keys,
            "missing_in_production": self.missing_in_production,
            "missing_in_dry_run": self.missing_in_dry_run,
            "details": [d.to_dict() for d in self.details],
            "summary": self.summary(),
        }

    def to_markdown(self) -> str:
        """Generate a mark-down report.

        Returns:
            A markdown-formatted report string.
        """
        lines = [
            f"# Comparison Report: {self.task_id}",
            "",
            "## Summary",
            "",
            f"- **Total keys**: {self.total_keys}",
            f"- **Matched**: {self.matched_keys}",
            f"- **Mismatched**: {self.mismatched_keys}",
            f"- **Missing in production**: {', '.join(self.missing_in_production) or 'None'}",
            f"- **Missing in dry-run**: {', '.join(self.missing_in_dry_run) or 'None'}",
            "",
            "## Details",
            "",
        ]

        if self.details:
            lines.append("| Key | Match | Details |")
            lines.append("|-----|-------|---------|")
            for d in self.details:
                match_str = "✅" if d.match else "❌"
                details_str = d.diff_details or "-"
                lines.append(f"| {d.key} | {match_str} | {details_str} |")
        else:
            lines.append("No comparison details available.")

        lines.append("")
        return "\n".join(lines)
