"""Planner decompose prompt — a single structure-aware template.

The decomposition is grounded in a preliminary hybrid search: ``{retrieved}``
carries the chunks retrieved for the user's question, so the decomposition
reflects what the corpus actually contains rather than guessing.
"""

# ═══════════════════════════════════════════════════════════════
# Unified structure-aware decomposition.
#
# Instead of routing by a brittle regex to a per-type template, ONE prompt asks
# the LLM to judge the question's reasoning STRUCTURE and emit typed claims.
# This covers the four shapes that matter for the failure cases observed in the
# FRAMES benchmark (aggregate, temporal, chain, flat), none of which a keyword
# regex could reliably detect:
#   • chain      — answer needs an intermediate entity/relationship (Glyn Harper
#                  → the book he wrote → the book's award).
#   • aggregate  — answer is a combination (average/count/sum/max/min) over ALL
#                  members of a set (e.g. average left-field distance across
#                  every retractable-roof MLB stadium), requiring an ENUMERATION
#                  claim + a combine claim, not independent per-member claims.
#   • temporal   — answer depends on a specific year/period or a cross-time link
#                  (e.g. "in 1978 ... Best Picture the same year").
#   • flat       — independent single-hop facts.
# ═══════════════════════════════════════════════════════════════
DECOMPOSE_UNIFIED = """Decompose the question into research claims, choosing each claim's reasoning structure yourself. Base the decomposition on BOTH the question and the preliminary retrieved context below.

Judge which structure(s) apply to the question and use them:

1. flat — independent single-hop facts. Default for most questions.
2. chain — the answer depends on resolving one or more intermediate entities/relationships, in dependency order, before reaching the final fact (e.g. "Which law did the company that employs Brian Bergstein violate?" needs his employer first, then the law). Set "prerequisites" to an ORDERED list of OPEN queries, ONE per hop, in the order they must be resolved. Each hop is a PLAIN atomic open query (no bridge placeholders — the intermediate value is resolved at runtime by the search loop and the next hop anchors to it dynamically). Example: "In what city did the 6th deadliest single-structure attack in US history take place, and who was the King of Siam at that time?" → prerequisites ["What was the 6th deadliest single-structure attack in US history?", "In what year did that attack occur?", "Who was the King of Siam in that year?"]. Every hop must be a SINGLE atomic fact (one entity/relation/time); do NOT merge hops. If there is only one hop, set a single-element prerequisites list.
3. aggregate — the answer is a combination (average / count / sum / maximum / minimum) over ALL members of a set. Emit ONE enumeration claim that asks for the values of EVERY member (e.g. "What is the left-field distance of every MLB stadium with a retractable roof?"), and ONE combine claim that asks for the aggregate (e.g. "What is the average of those left-field distances?"). Do NOT split into one claim per member — that can never be aggregated. Mark the enumeration claim's "claim_type" as "aggregate" and set "target" to the full set.
4. temporal — the answer depends on a specific year/period or a cross-time link between events (e.g. "the film that won Best Picture the same year Argentina won the 1978 World Cup"). Make the year/period explicit in the claim description.

Relevance rules (strict):
- Every claim MUST be directly needed to answer the question. Do NOT add background or tangential facts.
- Prefer fewer, well-scoped claims over many overlapping ones.
- OPEN QUERY (CRITICAL): each claim's description must be an OPEN research target (what/which/when/how many/what is X), NOT a pre-answered assertion. Never bake a guessed value into the description.
- ATOMICITY (CRITICAL): each claim and each prerequisite hop must be a SINGLE atomic fact (one entity, one relation, one time). Do NOT join multiple facts with "and"/"as well as"/"/" in one description or one hop. A multi-hop question MUST be split into ordered prerequisites, never crammed into one description.

Question: {question}
Maximum number of claims: {max_claims}
Detail level: {detail_level}

Preliminary retrieved context (use it to ground claims in what the corpus actually contains; do not invent facts it cannot support):
{retrieved}

Output format (JSON):
{{
    "claims": [
        {{
            "claim_id": "c1",
            "description": "What is the left-field distance of every MLB stadium with a retractable roof?",
            "claim_type": "aggregate",
            "prerequisite": "",
            "prerequisites": [],
            "target": "all MLB stadiums with a retractable roof",
            "priority": 1
        }},
        {{
            "claim_id": "c2",
            "description": "What is the average of those left-field distances?",
            "claim_type": "flat",
            "prerequisite": "",
            "prerequisites": [],
            "priority": 2
        }},
        {{
            "claim_id": "c3",
            "description": "Who was the King of Siam in the year of the 6th deadliest single-structure attack in US history?",
            "claim_type": "chain",
            "prerequisites": [
                "What was the 6th deadliest single-structure attack in US history?",
                "In what year did that attack occur?"
            ],
            "priority": 3
        }}
    ]
}}
"""
