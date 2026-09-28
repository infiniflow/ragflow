"""Four-aspect keyword extraction with entity weighting, ported from the
agentic_search4 v8 keyword graph.

A keyword search matches only the surface forms you give it, so a single flat
bag of terms is a poor retrieval driver: the model does not know how the corpus
phrases a fact. This extraction asks the LLM for FOUR aspects of the question —
``entity`` (what the fact is about), ``aliases`` (its surface variants),
``fact_type`` (words the corpus might use for this kind of fact, including table
column abbreviations) and ``qualifiers`` (year / edition / jurisdiction /
revision). ``entity`` is what discriminates, so it is repeated in the search
query to weight BM25 toward it; the plain deduped union of all four aspects is
used to narrow retrieved chunks to their keyword-bearing sentences.

Public interface
----------------
``extract_weighted_keywords(llm, question)`` -> ``(query, union)``
    ``query``  — entity terms repeated (x3) + qualifiers repeated (x3) + the other
                aspects once, comma-joined for the actual retrieval string (BM25
                weights the repeated entity/qualifier up; a year or jurisdiction is
                as discriminating as the entity and must dominate the ranking).
    ``union``  — every term once, comma-joined, for narrowing retrieved chunks.
"""

import logging

_LOG = logging.getLogger(__name__)

# Keywords are extracted into FOUR weighted aspects. ``entity`` terms are what
# discriminate and are repeated in the search query so BM25 weights them up;
# ``aliases`` find the document, ``fact_type`` and ``qualifiers`` only boost
# ranking. The plain deduped union (``keywords``) narrows retrieved chunks to
# their keyword-bearing sentences; the entity-weighted string (``query``) drives
# retrieval.
_KEYWORD_ASPECTS = ("entity", "aliases", "fact_type", "qualifiers")
_KEYWORD_ENTITY_REPEAT = 3  # copies of each entity term in the query, to weight it up
_KEYWORD_QUALIFIER_REPEAT = 3  # copies of each qualifier (year / version / jurisdiction), weighted up like entity
_KEYWORD_MAX_CHARS = 400  # hard cap on the weighted query so it never pollutes

_KEYWORDS_SYSTEM = """You turn ONE question into search terms for a keyword/BM25 search engine.

Emit the terms that would appear VERBATIM in a document that answers the question, sorted into FOUR
categories. Every term must come from the question itself or be a surface form of something in it.

A. "entity" — the specific thing the fact is ABOUT: proper nouns, titles, identifiers. Keep a
   multi-word entity whole, as ONE term ("Brown County", "Treaty of Versailles"); split across
   several terms its tokens match independently and drag in noise. A bare identifier — a serial,
   patent, catalogue or case number — is a complete entity on its own; never glue it to the words
   around it.
B. "aliases" — the engine matches ONLY the surface forms you supply, so emit the plausible variants
   of A: full vs. short name, native-language and transliterated forms, official vs. common name,
   acronym and its expansion, and the qualified form ("Brown County" -> "Brown County, Kansas").
C. "fact_type" — 3 to 6 words the corpus might use for this KIND of fact, since you cannot know how
   it is phrased. Spread them across registers:
     quantity of people -> population, inhabitants, residents, census, demographics, headcount
     time of an event   -> founded, established, opened, dated, began
     role of a person   -> served, appointed, elected, held, director
   SOURCES TABULATE WHAT QUESTIONS SPELL OUT: a statistic named in prose is usually written in a
   table as a column abbreviation, and the prose wording may not appear in the document at all. So
   include the abbreviation a table would use — "points per game" -> "PPG", "PTS"; "earnings per
   share" -> "EPS"; "games played" -> "GP" — and reach a superlative through its plain column too:
   "leading scorer" is found by looking for "PTS" and "PPG", not for the phrase itself.
D. "qualifiers" — year, edition, jurisdiction, revision. Worth emitting even when it looks
   redundant: the qualifier often sits in a table header or a document title that chunking has
   severed from the value. Include EVERY alternative expression of a DATE or NUMBER in the
   question — ordinals and their words ("21st" -> "twenty-first"), digits and their words
   ("2000000" -> "two million", "2 million"), and each common date format ("Aug 2nd" -> "August 2",
   "2 August", "08-02").

A and B are what FINDS the document; C and D only boost the ranking. So never withhold an entity
because you are unsure of it, and never pad C or D to reach a count.

DROP entirely: question words ("which", "who", "when", "how many"), relational scaffolding, and
generic high-frequency nouns ("year", "number", "city", "total", "list", "information"). They cost
ranking quality and retrieve nothing.

Output ONLY JSON, no prose, no code fences:
{"entity": ["<term>", ...], "aliases": ["<term>", ...], "fact_type": ["<term>", ...], "qualifiers": ["<term>", ...]}
Any category may be empty."""


def _norm_keyword(s: str) -> str:
    """Normalise a keyword term for cross-category dedup."""
    return " ".join((s or "").lower().split())


def _parse_aspects(raw: str) -> dict[str, list[str]]:
    """Parse the LLM's JSON into one deduped list per aspect.

    ONE dedup set spans all four categories: a term the model emits as both an
    entity and an alias must not collect a second share of the query's mass on
    the strength of having been named twice.
    """
    import re

    import json_repair

    data: dict = {}
    cleaned = re.sub(r"^.*</think>", "", raw or "", flags=re.DOTALL).strip()
    cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
    try:
        parsed = json_repair.loads(cleaned)
        if isinstance(parsed, dict):
            data = parsed
    except Exception:
        pass

    aspects: dict[str, list[str]] = {}
    seen: set[str] = set()
    for aspect in _KEYWORD_ASPECTS:
        terms: list[str] = []
        for k in data.get(aspect) or []:
            term = str(k).strip()
            key = _norm_keyword(term)
            if term and key and key not in seen:
                seen.add(key)
                terms.append(term)
        aspects[aspect] = terms
    return aspects


async def extract_weighted_keywords(llm, question: str) -> tuple[str, str]:
    """Extract FOUR keyword aspects for ``question`` and return ``(query, keywords)``.

    ``query`` is the weighted search string: every entity term is repeated
    ``_KEYWORD_ENTITY_REPEAT`` times and every qualifier (year / version /
    jurisdiction) ``_KEYWORD_QUALIFIER_REPEAT`` times, so BM25 weights both up
    inside the same query; aliases and fact-type vocabulary appear once each.
    ``keywords`` is the plain deduped union of all four aspects (one copy each)
    used to narrow retrieved chunks to their keyword-bearing sentences.

    ``llm`` exposes ``async_chat`` and ``max_length``. Falls back to
    ``(question, question)`` when extraction fails.
    """
    if not question:
        return "", ""
    from rag.prompts.generator import form_message, message_fit_in

    aspects: dict[str, list[str]] = {}
    try:
        _, msg = message_fit_in(form_message(_KEYWORDS_SYSTEM, question), llm.max_length)
        ans = await llm.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.1})
        if isinstance(ans, tuple):
            ans = ans[0]
        aspects = _parse_aspects(ans if isinstance(ans, str) else "")
    except Exception:
        _LOG.exception("extract_weighted_keywords failed")

    keywords = ", ".join(t for aspect in _KEYWORD_ASPECTS for t in aspects.get(aspect) or []) or question
    # Entity and qualifier terms are repeated so BM25 weights them up: the entity
    # is what the fact is about, and a year / edition / jurisdiction is just as
    # discriminating (often the only thing that separates two otherwise identical
    # chunks). Aliases and fact-type vocabulary appear once each — they find or
    # boost, but must not dominate the ranking.
    weighted = [t for t in (aspects.get("entity") or []) for _ in range(_KEYWORD_ENTITY_REPEAT)]
    weighted += [t for t in (aspects.get("qualifiers") or []) for _ in range(_KEYWORD_QUALIFIER_REPEAT)]
    weighted += [t for aspect in ("aliases", "fact_type") for t in (aspects.get(aspect) or [])]
    query = ", ".join(weighted) or keywords
    query = query[:_KEYWORD_MAX_CHARS]
    keywords = keywords[:_KEYWORD_MAX_CHARS]
    _LOG.info(
        "[Keywords] entity x%d: %s | aliases: %s | fact-type: %s | qualifiers x%d: %s",
        _KEYWORD_ENTITY_REPEAT,
        "; ".join(aspects.get("entity") or []) or "-",
        "; ".join(aspects.get("aliases") or []) or "-",
        "; ".join(aspects.get("fact_type") or []) or "-",
        _KEYWORD_QUALIFIER_REPEAT,
        "; ".join(aspects.get("qualifiers") or []) or "-",
    )
    return query, keywords
