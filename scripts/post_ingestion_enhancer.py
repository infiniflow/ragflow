#!/usr/bin/env python3
"""
Post-ingestion enhancer for RAGFlow fund factsheet documents.

Generates domain-specific keywords for chunks and extracts document metadata.
Designed for fund factsheet PDFs with naming pattern: {FundName}-{YYYYMM}-{Lang}.pdf

Usage:
    python scripts/post_ingestion_enhancer.py --dataset-id <id> [--dry-run] [--doc-id <id>]
    python scripts/post_ingestion_enhancer.py --dataset-id <id> --metadata-only
    python scripts/post_ingestion_enhancer.py --dataset-id <id> --keywords-only
    python scripts/post_ingestion_enhancer.py --dataset-id <id> --fix-tables [--dry-run]
    python scripts/post_ingestion_enhancer.py --dataset-id <id> --export-tables
"""

import argparse
import json
import logging
import os
import re
import sys
import time
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Dict, List, Optional, Tuple

import requests
from requests.auth import HTTPBasicAuth

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Configuration (override via environment variables)
# ---------------------------------------------------------------------------
ES_HOST = os.getenv("ES_HOST", "http://127.0.0.1:19200")
ES_USER = os.getenv("ES_USER", "elastic")
ES_PASS = os.getenv("ES_PASS", "infini_rag_flow")

MYSQL_HOST = os.getenv("MYSQL_HOST", "127.0.0.1")
MYSQL_PORT = int(os.getenv("MYSQL_PORT", "13306"))
MYSQL_USER = os.getenv("MYSQL_USER", "root")
MYSQL_PASS = os.getenv("MYSQL_PASS", "infini_rag_flow")
MYSQL_DB = os.getenv("MYSQL_DB", "rag_flow")

SILICONFLOW_API_KEY = os.getenv("SILICONFLOW_API_KEY", "")
SILICONFLOW_BASE = os.getenv("SILICONFLOW_BASE", "https://api.siliconflow.cn/v1")
LLM_MODEL = os.getenv("LLM_MODEL", "Pro/MiniMaxAI/MiniMax-M2.5")

TEMPLATE_FILE = os.getenv(
    "TABLE_TEMPLATE_FILE",
    str(Path(__file__).resolve().parent / "table_templates.json"),
)

MONTH_NAMES = {
    "01": "January", "02": "February", "03": "March",
    "04": "April",   "05": "May",      "06": "June",
    "07": "July",    "08": "August",   "09": "September",
    "10": "October", "11": "November", "12": "December",
}

# ---------------------------------------------------------------------------
# Keyword extraction prompt (domain-specific for fund factsheets)
# ---------------------------------------------------------------------------
KEYWORD_PROMPT = """\
You are a keyword extractor for fund factsheet chunks.

Fund name and report month are already handled by document-level metadata.
Your job is ONLY to label the CONTENT TYPE of this chunk.

## Task
Pick 2-4 content-type labels from this list:
  top holdings, equity holdings, fixed income holdings, dividend information,
  performance, monthly returns, historical returns, portfolio characteristics,
  portfolio yield, country allocation, sector allocation, credit ratings,
  fund overview, investment objective, fund manager, disclaimer

## Rules
- Pick ONLY labels that accurately describe what this chunk contains.
- IMPORTANT: Distinguish "equity holdings" from "fixed income holdings".
  If the table lists stocks/equities, use "equity holdings".
  If the table lists bonds/notes/perpetuals, use "fixed income holdings".
  Do NOT use the generic "top holdings" alone.
- Do NOT include fund names, dates, months, company names, or person names.
- Output the labels comma-separated on a single line. Nothing else.

## Chunk Content
{content}
"""

# ---------------------------------------------------------------------------
# Section title → keyword mapping (deterministic, zero-cost)
# ---------------------------------------------------------------------------
SECTION_KEYWORD_MAP = [
    (r"top holdings\s*[-\u2013\u2014]\s*equit", ["top holdings", "equity holdings"]),
    (r"top holdings\s*[-\u2013\u2014]\s*fixed", ["top holdings", "fixed income holdings"]),
    (r"top holdings",                    ["top holdings"]),
    (r"fee structure",                   ["fee structure"]),
    (r"portfolio characteristics",       ["portfolio characteristics"]),
    (r"performance update",              ["performance update"]),
    (r"navs?\s*&\s*codes?",             ["navs & codes"]),
    (r"credit ratings",                  ["credit ratings"]),
    (r"asset type by sector",            ["sector exposure"]),
    (r"sector exposure",                 ["sector exposure"]),
    (r"asset type by geography",         ["geographical exposure"]),
    (r"geographical exposure",           ["geographical exposure"]),
    (r"currency breakdown",              ["currency breakdown"]),
    (r"dividend information",            ["dividend information"]),
    (r"monthly returns",                 ["monthly returns"]),
    (r"fund facts",                      ["fund facts"]),
    (r"investment objective",            ["investment objective"]),
    (r"senior investment staff",         ["fund manager"]),
    (r"fund manager",                    ["fund manager"]),
    (r"fund notices",                    ["disclaimer"]),
    (r"historical returns",              ["historical returns"]),
]


def extract_section_keywords(content: str) -> List[str]:
    """Extract keywords from known section titles in chunk content. Max 5."""
    text = re.sub(r"<[^>]+>", " ", content)
    text_lower = text.lower()
    found: List[str] = []
    for pattern, labels in SECTION_KEYWORD_MAP:
        if re.search(pattern, text_lower):
            for label in labels:
                if label not in found:
                    found.append(label)
                    if len(found) >= 5:
                        return found
    return found


# ===========================================================================
# Infrastructure helpers
# ===========================================================================
class ESClient:
    """Thin Elasticsearch helper."""

    def __init__(self, host: str, user: str, password: str):
        self.host = host.rstrip("/")
        self.auth = HTTPBasicAuth(user, password)
        self.session = requests.Session()
        self.session.auth = self.auth
        self.session.headers["Content-Type"] = "application/json"

    def search(self, index: str, body: dict, size: int = 200) -> List[dict]:
        body["size"] = size
        r = self.session.post(f"{self.host}/{index}/_search", json=body)
        r.raise_for_status()
        return r.json()["hits"]["hits"]

    def bulk(self, index: str, lines: List[str]) -> dict:
        body = "\n".join(lines) + "\n"
        r = self.session.post(
            f"{self.host}/{index}/_bulk",
            data=body,
            headers={"Content-Type": "application/x-ndjson"},
        )
        r.raise_for_status()
        return r.json()

    def get_doc(self, index: str, doc_id: str) -> Optional[dict]:
        r = self.session.get(f"{self.host}/{index}/_doc/{doc_id}")
        if r.status_code == 404:
            return None
        r.raise_for_status()
        return r.json()

    def index_doc(self, index: str, doc_id: str, body: dict):
        r = self.session.put(f"{self.host}/{index}/_doc/{doc_id}?refresh=true", json=body)
        r.raise_for_status()
        return r.json()

    def update_doc(self, index: str, doc_id: str, partial: dict):
        r = self.session.post(
            f"{self.host}/{index}/_update/{doc_id}?refresh=true",
            json={"doc": partial},
        )
        r.raise_for_status()
        return r.json()


class LLMClient:
    """OpenAI-compatible chat completions client."""

    def __init__(self, api_key: str, base_url: str, model: str):
        self.api_key = api_key
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.session = requests.Session()
        self.session.headers.update({
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        })

    def chat(self, system: str, user: str = "Output:", temperature: float = 0.2) -> str:
        r = self.session.post(
            f"{self.base_url}/chat/completions",
            json={
                "model": self.model,
                "messages": [
                    {"role": "system", "content": system},
                    {"role": "user", "content": user},
                ],
                "temperature": temperature,
                "max_tokens": 256,
            },
            timeout=30,
        )
        r.raise_for_status()
        text = r.json()["choices"][0]["message"]["content"].strip()
        text = re.sub(r"^.*?</think>", "", text, flags=re.DOTALL).strip()
        return text



def get_mysql_conn():
    import pymysql
    return pymysql.connect(
        host=MYSQL_HOST, port=MYSQL_PORT,
        user=MYSQL_USER, password=MYSQL_PASS, database=MYSQL_DB,
    )


# ===========================================================================
# Metadata extraction (deterministic, from filename)
# ===========================================================================
def extract_metadata_from_filename(filename: str) -> Dict[str, str]:
    """
    Extract fund_name and report_time from filename.
    Pattern: {FundName}-{YYYYMM}-{Lang}.pdf
    Example: VP_Asian Income Fund-202506-Eng.pdf
             -> fund_name="VP Asian Income Fund", report_time="2025-06"
    """
    meta = {}
    stem = filename.rsplit(".", 1)[0] if "." in filename else filename

    m = re.search(r"(\d{4})(\d{2})", stem)
    if m:
        year, month = m.group(1), m.group(2)
        meta["report_time"] = f"{year}-{month}"
        meta["year"] = year
        meta["month"] = month
        meta["month_name"] = MONTH_NAMES.get(month, month)

    parts = re.split(r"-\d{6}", stem)
    if parts:
        raw_name = parts[0].replace("_", " ").strip()
        meta["fund_name"] = raw_name

    return meta


# ===========================================================================
# Keyword generation (section-title first, LLM fallback)
# ===========================================================================
def generate_keywords(llm: LLMClient, content: str) -> List[str]:
    """Extract keywords from section titles; call LLM only when none found."""
    section_kws = extract_section_keywords(content)
    if section_kws:
        return section_kws
    prompt = KEYWORD_PROMPT.format(content=content[:4000])
    raw = llm.chat(prompt)
    keywords = [kw.strip().strip('"').strip("'").lower() for kw in raw.split(",")]
    keywords = [kw for kw in keywords if kw and len(kw) > 1]
    return keywords


# ===========================================================================
# Table structure correction (template-based)
# ===========================================================================
def _get_table_title(content: str) -> str:
    """Extract the trailing title text after </table> in a chunk."""
    m = re.search(r"</table>\s*(.*?)$", content, re.DOTALL)
    return m.group(1).strip() if m else ""


def _get_num_cols(html: str) -> int:
    """Return the max column count across all rows in a table."""
    table_m = re.search(r"<table>(.*?)</table>", html, re.DOTALL)
    if not table_m:
        return 0
    rows = re.findall(r"<tr>(.*?)</tr>", table_m.group(1), re.DOTALL)
    best = 0
    for row in rows:
        cells = re.findall(r"<td[^>]*>", row)
        best = max(best, len(cells))
    return best


def _detect_sparse_rows(html: str) -> List[str]:
    """Return first-column labels of rows that have only 1 non-empty data cell."""
    table_m = re.search(r"<table>(.*?)</table>", html, re.DOTALL)
    if not table_m:
        return []
    rows = re.findall(r"<tr>(.*?)</tr>", table_m.group(1), re.DOTALL)
    num_cols = _get_num_cols(html)
    if num_cols < 3:
        return []

    labels: List[str] = []
    for row_html in rows:
        cells = [
            re.sub(r"<[^>]+>", "", c).strip()
            for c in re.findall(r"<td[^>]*>(.*?)</td>", row_html, re.DOTALL)
        ]
        if len(cells) < num_cols:
            continue
        data = cells[1:]
        non_empty = sum(1 for c in data if c)
        if non_empty == 1 and len(data) >= 2 and cells[0]:
            labels.append(cells[0])
    return labels


def load_correction_rules(template_path: str) -> Dict[str, List[dict]]:
    """Load per-fund correction rules from JSON template file."""
    if not os.path.isfile(template_path):
        return {}
    with open(template_path, encoding="utf-8") as f:
        return json.load(f)


def apply_colspan_fix(
    html: str,
    colspan_rows: List[str],
    colspan_split: Optional[List[int]] = None,
) -> str:
    """Apply colspan fix to specified rows in an HTML table.

    For each row whose first cell text matches an entry in colspan_rows:
    - 1 non-empty value → colspan all data columns
    - 2 non-empty values → use colspan_split (e.g. [3,1]) or default even split
    """
    table_m = re.search(r"<table>(.*?)</table>", html, re.DOTALL)
    if not table_m:
        return html

    num_cols = _get_num_cols(html)
    if num_cols < 3:
        return html

    data_span = num_cols - 1
    target_labels = {label.lower().rstrip("0123456789") for label in colspan_rows}

    def _fix_row(row_html: str) -> str:
        cells_raw = re.findall(r"<td[^>]*>(.*?)</td>", row_html, re.DOTALL)
        if len(cells_raw) < 2:
            return row_html
        label = re.sub(r"<[^>]+>", "", cells_raw[0]).strip()
        if label.lower().rstrip("0123456789") not in target_labels:
            return row_html
        non_empty = [c for c in cells_raw[1:] if re.sub(r"<[^>]+>", "", c).strip()]
        if len(non_empty) == 1:
            return f"<tr><td>{cells_raw[0]}</td><td colspan=\"{data_span}\">{non_empty[0]}</td></tr>"
        if len(non_empty) == 2 and data_span >= 2:
            if colspan_split and len(colspan_split) == 2 and sum(colspan_split) == data_span:
                s1, s2 = colspan_split
            else:
                s1 = data_span // 2
                s2 = data_span - s1
            return (f"<tr><td>{cells_raw[0]}</td>"
                    f"<td colspan=\"{s1}\">{non_empty[0]}</td>"
                    f"<td colspan=\"{s2}\">{non_empty[1]}</td></tr>")
        return row_html

    table_inner = table_m.group(1)
    fixed_inner = re.sub(r"<tr>(.*?)</tr>", lambda m: _fix_row(m.group(0)), table_inner, flags=re.DOTALL)

    return html[:table_m.start()] + "<table>" + fixed_inner + "</table>" + html[table_m.end():]


def _rule_matches_date(rule: dict, report_time: str) -> bool:
    """Check if a rule's date_from/date_to range includes this report_time."""
    date_from = rule.get("date_from")
    date_to = rule.get("date_to")
    if date_from and report_time < date_from:
        return False
    if date_to and report_time > date_to:
        return False
    return True


def fix_table_chunks(
    es: ESClient,
    rules: Dict[str, List[dict]],
    chunk_index: str,
    dataset_id: str,
    doc_id: str,
    fund_name: str,
    report_time: str,
    dry_run: bool = False,
) -> Tuple[int, int, int]:
    """Apply template-based colspan fixes to table chunks for one document.

    Returns (total_tables, matched, fixed).
    """
    fund_rules = rules.get(fund_name, [])
    if not fund_rules:
        return 0, 0, 0

    active_rules = [r for r in fund_rules if _rule_matches_date(r, report_time)]
    if not active_rules:
        return 0, 0, 0

    chunks = get_chunks(es, chunk_index, dataset_id, doc_id)
    table_chunks = [c for c in chunks if "<table>" in c["content"]]

    total = len(table_chunks)
    matched = 0
    fixed = 0

    for chunk in table_chunks:
        title = _get_table_title(chunk["content"])
        for rule in active_rules:
            if rule["match_title"].lower() not in title.lower():
                continue

            matched += 1
            colspan_rows = rule.get("colspan_rows", [])
            if not colspan_rows:
                continue

            split = rule.get("colspan_split")
            new_content = apply_colspan_fix(chunk["content"], colspan_rows, split)
            if new_content == chunk["content"]:
                log.info("    [OK] %s: no change needed", chunk["id"])
                continue

            log.info("    [FIXED] %s (%s): colspan applied to %s",
                     chunk["id"], rule["match_title"], colspan_rows)
            log.debug("    OLD: %s", chunk["content"][:200])
            log.debug("    NEW: %s", new_content[:200])

            if not dry_run:
                es.update_doc(chunk_index, chunk["id"], {"content_with_weight": new_content})
            fixed += 1
            break

    return total, matched, fixed


def export_table_templates(
    es: ESClient,
    chunk_index: str,
    dataset_id: str,
    documents: List[dict],
    output_path: str,
):
    """Scan all table chunks and generate an initial template file.

    Auto-detects sparse rows and pre-fills colspan_rows suggestions.
    """
    fund_tables: Dict[str, Dict[str, List[str]]] = defaultdict(lambda: defaultdict(list))

    for doc in documents:
        meta = extract_metadata_from_filename(doc["name"])
        fund_name = meta.get("fund_name", "Unknown Fund")
        chunks = get_chunks(es, chunk_index, dataset_id, doc["id"])

        for chunk in chunks:
            if "<table>" not in chunk["content"]:
                continue
            title = _get_table_title(chunk["content"])
            if not title:
                continue
            sparse = _detect_sparse_rows(chunk["content"])
            for label in sparse:
                if label not in fund_tables[fund_name][title]:
                    fund_tables[fund_name][title].append(label)

    existing: Dict[str, List[dict]] = {}
    if os.path.isfile(output_path):
        with open(output_path, encoding="utf-8") as f:
            existing = json.load(f)
        log.info("Merging with existing template: %s", output_path)

    result: Dict[str, List[dict]] = {}
    all_funds = set(list(fund_tables.keys()) + list(existing.keys()))

    for fund in sorted(all_funds):
        existing_rules = {r["match_title"]: r for r in existing.get(fund, [])}
        new_titles = fund_tables.get(fund, {})

        merged_rules: List[dict] = []
        all_titles = set(list(existing_rules.keys()) + list(new_titles.keys()))

        for title in sorted(all_titles):
            if title in existing_rules:
                rule = existing_rules[title]
                new_sparse = new_titles.get(title, [])
                existing_labels = set(rule.get("colspan_rows", []))
                for label in new_sparse:
                    if label not in existing_labels:
                        rule.setdefault("_auto_detected", []).append(label)
                merged_rules.append(rule)
            else:
                sparse = new_titles[title]
                merged_rules.append({
                    "match_title": title[:60],
                    "colspan_rows": sparse,
                    "note": "auto-detected" if sparse else "",
                })

        if merged_rules:
            result[fund] = merged_rules

    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(result, f, indent=2, ensure_ascii=False)

    total_rules = sum(len(v) for v in result.values())
    total_fixes = sum(
        len(r.get("colspan_rows", []))
        for rules in result.values() for r in rules
    )
    log.info("Template exported to %s", output_path)
    log.info("  Funds: %d  Table types: %d  colspan_rows entries: %d",
             len(result), total_rules, total_fixes)


# ===========================================================================
# Main processing logic
# ===========================================================================
def get_tenant_and_index(dataset_id: str) -> Tuple[str, str, str]:
    """Get tenant_id and ES index name for a dataset."""
    conn = get_mysql_conn()
    cur = conn.cursor()
    cur.execute(
        "SELECT kb.tenant_id, kb.name FROM knowledgebase kb WHERE kb.id=%s",
        (dataset_id,),
    )
    row = cur.fetchone()
    conn.close()
    if not row:
        raise ValueError(f"Dataset {dataset_id} not found")
    tenant_id, ds_name = row
    chunk_index = f"ragflow_{tenant_id}"
    meta_index = f"ragflow_doc_meta_{tenant_id}"
    log.info("Dataset '%s' | tenant=%s | chunk_index=%s", ds_name, tenant_id, chunk_index)
    return tenant_id, chunk_index, meta_index


def update_metadata_schema(dataset_id: str, documents: List[dict]):
    """Auto-update the dataset metadata schema enum values from current documents."""
    fund_names = set()
    report_times = set()
    for doc in documents:
        meta = extract_metadata_from_filename(doc["name"])
        if meta.get("fund_name"):
            fund_names.add(meta["fund_name"])
        if meta.get("report_time"):
            report_times.add(meta["report_time"])

    conn = get_mysql_conn()
    cur = conn.cursor()
    cur.execute("SELECT parser_config FROM knowledgebase WHERE id=%s", (dataset_id,))
    pc = json.loads(cur.fetchone()[0])

    schema = pc.get("metadata", {})
    props = schema.get("properties", {})

    old_funds = set(props.get("fund_name", {}).get("enum", []))
    old_times = set(props.get("report_time", {}).get("enum", []))

    new_funds = sorted(fund_names | old_funds)
    new_times = sorted(report_times | old_times)

    if new_funds == sorted(old_funds) and new_times == sorted(old_times):
        log.info("Metadata schema already up to date")
        conn.close()
        return

    props["fund_name"] = {
        "type": "string",
        "description": "Fund name",
        "enum": new_funds,
    }
    props["report_time"] = {
        "type": "string",
        "description": "Report month in YYYY-MM format",
        "enum": new_times,
    }
    schema["properties"] = props
    schema["type"] = "object"
    schema["additionalProperties"] = False
    pc["metadata"] = schema
    pc["enable_metadata"] = True

    cur.execute(
        "UPDATE knowledgebase SET parser_config=%s, update_time=UNIX_TIMESTAMP()*1000, update_date=NOW() WHERE id=%s",
        (json.dumps(pc, ensure_ascii=False), dataset_id),
    )
    conn.commit()
    conn.close()

    added_times = sorted(report_times - old_times)
    added_funds = sorted(fund_names - old_funds)
    if added_times or added_funds:
        log.info("Schema updated: +funds=%s +times=%s", added_funds, added_times)


def get_documents(dataset_id: str, doc_id: Optional[str] = None) -> List[dict]:
    """Get documents from MySQL."""
    conn = get_mysql_conn()
    cur = conn.cursor()
    if doc_id:
        cur.execute(
            "SELECT id, name FROM document WHERE kb_id=%s AND id=%s",
            (dataset_id, doc_id),
        )
    else:
        cur.execute(
            "SELECT id, name FROM document WHERE kb_id=%s AND status='1'",
            (dataset_id,),
        )
    docs = [{"id": r[0], "name": r[1]} for r in cur.fetchall()]
    conn.close()
    return docs


def get_chunks(es: ESClient, chunk_index: str, dataset_id: str, doc_id: str) -> List[dict]:
    """Get all active chunks for a document from ES."""
    hits = es.search(chunk_index, {
        "query": {"bool": {
            "must": [
                {"term": {"kb_id": dataset_id}},
                {"term": {"doc_id": doc_id}},
            ],
            "must_not": [{"term": {"available_int": 0}}],
        }},
        "_source": ["content_with_weight", "important_kwd", "docnm_kwd"],
    }, size=500)
    return [{
        "id": h["_id"],
        "content": h["_source"].get("content_with_weight", ""),
        "keywords": h["_source"].get("important_kwd", []),
        "docnm": h["_source"].get("docnm_kwd", ""),
    } for h in hits]


def process_dataset(
    dataset_id: str,
    doc_id: Optional[str] = None,
    dry_run: bool = False,
    keywords_only: bool = False,
    metadata_only: bool = False,
    fix_tables: bool = False,
    export_tables: bool = False,
    concurrency: int = 5,
):
    tenant_id, chunk_index, meta_index = get_tenant_and_index(dataset_id)
    es = ESClient(ES_HOST, ES_USER, ES_PASS)
    llm = LLMClient(SILICONFLOW_API_KEY, SILICONFLOW_BASE, LLM_MODEL)

    documents = get_documents(dataset_id, doc_id)
    log.info("Found %d documents to process", len(documents))

    if export_tables:
        all_docs = get_documents(dataset_id) if doc_id else documents
        export_table_templates(es, chunk_index, dataset_id, all_docs, TEMPLATE_FILE)
        return

    if fix_tables:
        rules = load_correction_rules(TEMPLATE_FILE)
        if not rules:
            log.error("No template file found at %s. Run --export-tables first.", TEMPLATE_FILE)
            sys.exit(1)
        grand_total = grand_matched = grand_fixed = 0
        for doc in documents:
            meta = extract_metadata_from_filename(doc["name"])
            fund_name = meta.get("fund_name", "Unknown Fund")
            report_time = meta.get("report_time", "")
            log.info("─── %s [%s %s] ───", doc["name"], fund_name, report_time)
            t, m, x = fix_table_chunks(
                es, rules, chunk_index, dataset_id, doc["id"],
                fund_name, report_time, dry_run,
            )
            grand_total += t
            grand_matched += m
            grand_fixed += x
            log.info("  tables=%d  matched=%d  fixed=%d", t, m, x)
        log.info("═══ Table Fix Summary ═══")
        log.info("  Documents:  %d", len(documents))
        log.info("  Tables:     %d", grand_total)
        log.info("  Matched:    %d", grand_matched)
        log.info("  Fixed:      %d", grand_fixed)
        return

    all_docs = get_documents(dataset_id) if doc_id else documents
    update_metadata_schema(dataset_id, all_docs)

    total_chunks = 0
    total_kw_updated = 0
    total_meta_updated = 0

    for doc in documents:
        doc_name = doc["name"]
        meta = extract_metadata_from_filename(doc_name)
        fund_name = meta.get("fund_name", "Unknown Fund")
        month_name = meta.get("month_name", "Unknown")
        year = meta.get("year", "0000")
        report_time = meta.get("report_time", "")

        log.info("─── %s [%s %s] ───", doc_name, month_name, year)

        # ----- Metadata update -----
        if not keywords_only:
            meta_payload = {"fund_name": fund_name, "report_time": report_time}
            if dry_run:
                log.info("  [DRY-RUN] Would write metadata: %s", meta_payload)
            else:
                existing = es.get_doc(meta_index, doc["id"])
                doc_meta = {
                    "id": doc["id"],
                    "kb_id": dataset_id,
                    "meta_fields": meta_payload,
                }
                if existing and existing.get("found"):
                    es.update_doc(meta_index, doc["id"], {"meta_fields": meta_payload})
                else:
                    es.index_doc(meta_index, doc["id"], doc_meta)
                total_meta_updated += 1
                log.info("  Metadata updated: %s", meta_payload)

        # ----- Keyword update -----
        if not metadata_only:
            chunks = get_chunks(es, chunk_index, dataset_id, doc["id"])
            log.info("  %d chunks to process", len(chunks))
            total_chunks += len(chunks)

            bulk_lines = []

            def _gen_kw(chunk):
                try:
                    kws = generate_keywords(llm, chunk["content"])
                    return chunk["id"], kws, None
                except Exception as e:
                    return chunk["id"], None, str(e)

            with ThreadPoolExecutor(max_workers=concurrency) as pool:
                futures = {pool.submit(_gen_kw, c): c for c in chunks}
                for fut in as_completed(futures):
                    chunk_id, kws, err = fut.result()
                    if err:
                        log.warning("  FAILED %s: %s", chunk_id, err)
                        continue
                    if dry_run:
                        log.info("  [DRY-RUN] %s -> %s", chunk_id, kws)
                    else:
                        bulk_lines.append(json.dumps({"update": {"_id": chunk_id}}))
                        bulk_lines.append(json.dumps({"doc": {
                            "important_kwd": kws,
                            "available_int": 1,
                        }}))
                        total_kw_updated += 1

            if bulk_lines and not dry_run:
                result = es.bulk(chunk_index, bulk_lines)
                ok = sum(1 for i in result["items"] if i.get("update", {}).get("status") == 200)
                fail = len(result["items"]) - ok
                log.info("  ES bulk: %d ok, %d failed", ok, fail)

    log.info("═══ Summary ═══")
    log.info("  Documents processed: %d", len(documents))
    log.info("  Chunks processed:    %d", total_chunks)
    log.info("  Keywords updated:    %d", total_kw_updated)
    log.info("  Metadata updated:    %d", total_meta_updated)


# ===========================================================================
# CLI
# ===========================================================================
def main():
    parser = argparse.ArgumentParser(description="Post-ingestion enhancer for RAGFlow")
    parser.add_argument("--dataset-id", required=True, help="RAGFlow dataset (knowledgebase) ID")
    parser.add_argument("--doc-id", help="Process a single document only")
    parser.add_argument("--dry-run", action="store_true", help="Preview without writing")
    parser.add_argument("--keywords-only", action="store_true", help="Only update keywords")
    parser.add_argument("--metadata-only", action="store_true", help="Only update metadata")
    parser.add_argument("--fix-tables", action="store_true",
                        help="Apply table corrections from template file")
    parser.add_argument("--export-tables", action="store_true",
                        help="Export initial table template file for manual review")
    parser.add_argument("--concurrency", type=int, default=10, help="Parallel LLM calls (default: 10)")
    parser.add_argument("-v", "--verbose", action="store_true")
    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    process_dataset(
        dataset_id=args.dataset_id,
        doc_id=args.doc_id,
        dry_run=args.dry_run,
        keywords_only=args.keywords_only,
        metadata_only=args.metadata_only,
        fix_tables=args.fix_tables,
        export_tables=args.export_tables,
        concurrency=args.concurrency,
    )


if __name__ == "__main__":
    main()
