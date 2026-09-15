"""Deterministic arithmetic over retrieved evidence, ported from the
agentic_search4 v8 keyword graph.

Some questions ask for a number no single source states — the combined
population of three counties, how many of the listed films won an award, the
years between two dates. Every input is in the evidence by then and only the
arithmetic is missing, which an LLM does by writing digits one at a time and
gets wrong often enough to matter. So the LLM writes ONE Python expression and
Python evaluates it.

The expression comes from a language model, so it is NOT trusted. It is parsed
and every node checked against an AST whitelist BEFORE evaluation; anything
unlisted — an attribute access, a subscript, a lambda, an f-string, a name that
is not one of the allowed functions — is rejected outright rather than sandboxed
at run time. Evaluation then runs with no builtins at all.

Public interface
----------------
``compute(expression)``  -> ``(rendered, error)``  evaluate one expression safely.
``compute_from_facts(question, facts)`` -> optional ``{label, value, expression, uses}``
    LLM decides whether the question asks for a derivable number, writes the
    expression, evaluates it, and returns a structured result for the caller to
    attach to its evidence list.
"""

import ast
import logging

_LOG = logging.getLogger(__name__)

_COMPUTE_MAX_CHARS = 400  # the whole expression; every figure is inline, none is long


def _letters(*texts: object) -> int:
    """Count the alphabetic characters across the given names.

    "How many letters are in these names" is a real question, and every
    plain-Python way to answer it needs machinery this evaluator refuses — an
    attribute call (``"".join``, ``str.isalpha``) or a comprehension. So it is a
    function instead.

    Spaces, hyphens, apostrophes, digits and punctuation do NOT count; letters
    carrying diacritics DO ("José" is 4), because they are letters of the name.
    Takes any number of names, or a single list of them.
    """
    total = 0
    for text in texts:
        for item in text if isinstance(text, (list, tuple, set)) else [text]:
            if not isinstance(item, str):
                raise TypeError(f"letters() takes names, not {type(item).__name__}")
            total += sum(1 for ch in item if ch.isalpha())
    return total


def _date_diff(*dates: str) -> int:
    """Days between two ISO dates (inclusive of the earlier, exclusive of the
    later — a calendar span, the same as "how many days between two dates" on a
    desk calendar).

    ``date_diff("1941-07-28", "1959-07-17")`` -> the number of days from the
    first date to the second. Exactly two ISO ``YYYY-MM-DD`` dates are required;
    anything else is refused. This is the arithmetic the old ``years between two
    dates -> 1998 - 1954`` example could NOT express, so a "how many days after
    his death did X die" question (Q317: 6563 days between two death dates) fell
    back to ``needed:false`` and the final answer stopped at the two dates.
    """
    if len(dates) != 2:
        raise TypeError("date_diff() takes exactly two ISO dates")
    from datetime import date as _date

    parsed = []
    for d in dates:
        if not isinstance(d, str):
            raise TypeError(f"date_diff() takes ISO date strings, not {type(d).__name__}")
        parts = d.strip().split("-")
        if len(parts) != 3:
            raise ValueError(f"not an ISO date: {d!r}")
        try:
            parsed.append(_date(int(parts[0]), int(parts[1]), int(parts[2])))
        except ValueError:
            raise ValueError(f"not a valid ISO date: {d!r}")
    return abs((parsed[1] - parsed[0]).days)


def _digit_sum(*texts: object) -> int:
    """Add up the decimal digits inside the given values.

    "What do you get when you add up the numbers in the postcode" is a real
    question, and the plain-Python answer needs the comprehension this evaluator
    refuses — so, like ``letters``, it is a function instead of an expression.

    Every digit is added SEPARATELY: digit_sum("L7 7BN") is 14 and digit_sum("2020")
    is 4. A question that means whole numbers added together ("66 + 12") is written
    with those literals instead, because the value of a multi-digit number is not
    the sum of anything.

    Only ASCII digits count — a superscript or a fraction glyph is not a digit of
    the postcode. Takes any number of strings or whole numbers, or a list of them.
    """
    total = 0
    for text in texts:
        for item in text if isinstance(text, (list, tuple, set)) else [text]:
            if isinstance(item, bool) or not isinstance(item, (str, int)):
                raise TypeError(f"digit_sum() takes text or whole numbers, not {type(item).__name__}")
            total += sum(int(ch) for ch in str(item) if "0" <= ch <= "9")
    return total


# Pure arithmetic on literals. Nothing here can reach an object, a module or a name.
_COMPUTE_FUNCTIONS = {
    "abs": abs,
    "round": round,
    "min": min,
    "max": max,
    "sum": sum,
    "len": len,
    "int": int,
    "float": float,
    "sorted": sorted,
    "letters": _letters,
    "digit_sum": _digit_sum,
    "date_diff": _date_diff,
}

_COMPUTE_NODES = (
    ast.Expression,
    ast.Constant,
    ast.Tuple,
    ast.List,
    ast.Set,
    ast.Load,
    ast.Name,
    ast.Call,
    ast.IfExp,
    ast.UnaryOp,
    ast.UAdd,
    ast.USub,
    ast.Not,
    ast.BinOp,
    ast.Add,
    ast.Sub,
    ast.Mult,
    ast.Div,
    ast.FloorDiv,
    ast.Mod,
    ast.Pow,
    ast.BoolOp,
    ast.And,
    ast.Or,
    ast.Compare,
    ast.Eq,
    ast.NotEq,
    ast.Lt,
    ast.LtE,
    ast.Gt,
    ast.GtE,
)


# Functions whose result is a number whatever they are handed. `min`, `max` and
# `sum` are absent on purpose — min("b", "a") is a string — and `sorted` returns a
# list, so neither may stand where a number is required.
_COMPUTE_ALWAYS_NUMERIC = {"abs", "round", "int", "float", "len", "letters", "digit_sum", "date_diff"}


def _is_numeric(node: ast.AST) -> bool:
    """True when ``node`` can ONLY evaluate to a number.

    Multiplication is the one operator that turns a short expression into an
    arbitrarily large object — ``"a" * 10**9``, ``[1] * 10**9`` — so both its
    operands must be provably numeric. Everything else in the whitelist either
    cannot grow (``+`` on literals is bounded by the expression length) or is
    already constrained (``**`` takes plain numbers only).
    """
    if isinstance(node, ast.Constant):
        return isinstance(node.value, (int, float))  # bool included: it IS an int
    if isinstance(node, ast.UnaryOp):
        return _is_numeric(node.operand)
    if isinstance(node, ast.BinOp):
        return _is_numeric(node.left) and _is_numeric(node.right)
    if isinstance(node, ast.IfExp):
        return _is_numeric(node.body) and _is_numeric(node.orelse)
    if isinstance(node, ast.Compare):
        return True  # a comparison is a bool, and a bool is an int
    if isinstance(node, ast.Call) and isinstance(node.func, ast.Name):
        if node.func.id in _COMPUTE_ALWAYS_NUMERIC:
            return True
        if node.func.id in {"sum", "min", "max"}:
            return all(_is_numeric(arg) or _is_numeric_sequence(arg) for arg in node.args)
    return False


def _is_numeric_sequence(node: ast.AST) -> bool:
    """True for a literal list/tuple/set whose every element is provably numeric."""
    return isinstance(node, (ast.List, ast.Tuple, ast.Set)) and all(_is_numeric(element) for element in node.elts)


def _check_expression(tree: ast.AST) -> str:
    """Reject anything outside the arithmetic whitelist. Returns "" when clean."""
    for node in ast.walk(tree):
        if not isinstance(node, _COMPUTE_NODES):
            return f"{type(node).__name__} is not allowed"
        if isinstance(node, ast.Name) and node.id not in _COMPUTE_FUNCTIONS:
            return f"unknown name {node.id!r}"
        if isinstance(node, ast.Call):
            if not isinstance(node.func, ast.Name) or node.func.id not in _COMPUTE_FUNCTIONS:
                return "only the listed functions may be called"
            if node.keywords:
                return "keyword arguments are not allowed"
            # `len("Ada Lovelace")` is 12 and the answer is 11. The gap is silent, so
            # the expression is refused rather than counted.
            if node.func.id == "len" and node.args and isinstance(node.args[0], ast.Constant) and isinstance(node.args[0].value, str):
                return "len() on a string literal is ambiguous; use letters()"
            for arg in node.args:
                if isinstance(arg, ast.Constant) and isinstance(arg.value, str):
                    if len(arg.value) > 256:
                        return "string literal is too long"
        if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Mult):
            if not (_is_numeric(node.left) and _is_numeric(node.right)):
                return "multiplication is only allowed on numbers"
        if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Pow):
            if not (_is_numeric(node.left) and _is_numeric(node.right)):
                return "exponentiation is only allowed on numbers"
            if isinstance(node.right, ast.Constant) and isinstance(node.right.value, (int, float)) and abs(node.right.value) > 64:
                return "exponent is too large"
    return ""


def _format_number(value: float | int) -> str:
    """Render a computed number without float noise ("3.0" -> "3", 0.1+0.2 -> "0.3")."""
    if isinstance(value, int):
        return str(value)
    if value == int(value) and abs(value) < 10**15:
        return str(int(value))
    return f"{value:.6f}".rstrip("0").rstrip(".")


def compute(expression: str) -> tuple[str, str]:
    """Evaluate an LLM-written arithmetic expression. Returns ``(rendered, error)``.

    Exactly one of the two is non-empty. Every rejection is a normal outcome — the
    caller simply carries on without the computed evidence.
    """
    expression = (expression or "").strip()
    if not expression:
        return "", "empty expression"
    if len(expression) > _COMPUTE_MAX_CHARS:
        return "", f"expression is longer than {_COMPUTE_MAX_CHARS} characters"
    try:
        tree = ast.parse(expression, mode="eval")
    except SyntaxError as exc:
        return "", f"does not parse ({exc.msg})"
    problem = _check_expression(tree)
    if problem:
        return "", problem
    try:
        value = eval(compile(tree, "<evidence-arithmetic>", "eval"), {"__builtins__": {}}, dict(_COMPUTE_FUNCTIONS))
    except Exception as exc:
        return "", f"failed to evaluate ({type(exc).__name__}: {exc})"
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return "", f"result is {type(value).__name__}, not a number"
    if isinstance(value, float) and (value != value or value in (float("inf"), float("-inf"))):
        return "", "result is not a finite number"
    return _format_number(value), ""


_COMPUTE_SYSTEM = """You are given the ORIGINAL question and every fact discovered so far. Decide
whether that question asks for a NUMBER that NO fact states outright but that FOLLOWS ARITHMETICALLY
from figures the facts DO state — a sum, a difference, a count, an average, a percentage, a unit
conversion, an elapsed span.

If it does, compute it by writing ONE Python expression with every figure substituted as a literal.
The expression is evaluated on its own: no variables, no assignments, no imports, no attributes, no
subscripts. The only functions available are abs, round, min, max, sum, len, int, float, sorted,
letters, digit_sum and date_diff.
  combined population of three  -> 12345 + 6789 + 101112
  how many of the listed items  -> len(["Alpha", "Beta", "Gamma"])
  what percentage one figure is -> 100 * 4523 / 18092
  years between two dates       -> 1998 - 1954
  days between two dates        -> date_diff("1941-07-28", "1959-07-17")
  letters in a set of names     -> letters("Ada Lovelace", "Alan Turing")
  digits of a postcode added up -> digit_sum("L7 7BN")

ADDING UP THE DIGITS of a postcode, a house number, a serial number, a year or an address: use
digit_sum(...), and never read the digits out by hand. It adds each digit separately, which is what
such a question means — digit_sum("L7 7BN") is 7+7 = 14, digit_sum("2020") is 2+0+2+0 = 4. Pass the
identifier EXACTLY as the facts write it, letters and spaces included; they are ignored. It is the
WRONG tool for whole numbers the facts state separately — two populations, two prices, two years are
added as plain literals (12345 + 6789), not fed to digit_sum.

COUNTING LETTERS: use letters(...), NEVER len(...) on a name. len counts spaces, hyphens and
apostrophes as though they were letters, so it is wrong by exactly the amount nobody notices
(len("Ada Lovelace") is 12; the name has 11 letters). letters(...) takes any number of names, or one
list of them, and counts alphabetic characters only. Spell each name EXACTLY as the facts give it,
including any middle name or accent — and if the facts do not show a name in full, that figure is
missing, so return "needed": false rather than counting a partial name.

DAYS BETWEEN TWO DATES: when the question asks "how many days after X did Y happen" / "how many days
between two dates", use date_diff("YYYY-MM-DD", "YYYY-MM-DD") with the two dates EXACTLY as the facts
write them. Do NOT subtract the years (1959 - 1941) — that is the wrong quantity for a days question
(18 is years, not days). If either date is not a full YYYY-MM-DD in the facts, the figure is missing,
so return "needed": false rather than approximating.

AGE (an age, or an age difference, at some event): the facts almost always give a birth YEAR and an
event YEAR; the age is `event_year - birth_year` (or `birth_year - event_year`, taken as the positive
difference). You do NOT need the birth month or day — the year is enough. If the facts give FULL dates
(YYYY-MM-DD), prefer date_diff(...) which handles the day correctly; otherwise subtract the years.
Example: "elected in 2010, born 1971" -> 2010 - 1971. If the event year is BEFORE the birth year, the
difference is `birth_year - event_year` (use abs(...)). Never refuse because the birthday is not a
full date — the YEAR is sufficient.

PERCENTAGE (what percent / what share / what proportion / what fraction): `100 * part / whole`, where
`part` and `whole` are the exact figures from the facts. Example:
  "2.7 million Tamazight speakers out of 556 million total" -> 100 * 2.7 / 556
Do not round to an integer unless the question asks for that; keep the source figures exact.

UNIT CONVERSION (a speed, rate, or span in mixed units): convert inside the expression. A speed in
km/h becomes m/s by dividing by 3.6. Example for a difference in m/s between a fish and a swimmer:
  fish_kmh / 3.6 - 50 / swimmer_seconds      -> e.g. 132 / 3.6 - 50 / 21.07
Use the EXACT figures the facts state (do not round 21.07 to 21); if the facts give the speed already
in m/s, use it directly without dividing.

MULTIPLICATION (a rate times a count, e.g. dollars per day times days): multiply the RATE by the
COUNT exactly as the facts state them. Read the rate's NUMBER from the facts. Example: "a suggested
donation of $25 per day, kept up for 49 days" -> 25 * 49. If the facts state the rate as 1 but the
question calls it "a suggested donation", still use the exact figure the facts give — never substitute
a made-up base amount.

Prefer computing over giving up: when the question asks for a derivable number and the facts
provide the figures (even if in different units or spread across several facts), WRITE the
expression and compute it. In particular, questions asking for an AGE DIFFERENCE, a PERCENTAGE,
a SPEED DIFFERENCE (with unit conversion), or a MULTIPLICATION (a rate times a count) are exactly
what this tool is for.

Return "needed": false, with an empty expression, ONLY when:
- the ORIGINAL question does not ask for a number;
- a fact already states that number outright — a value you would only be restating is not a
  calculation;
- a figure the calculation needs is genuinely absent from the facts, or a list the count depends on
  is not shown to be complete. NEVER invent, estimate, recall or infer a figure. When input is
  missing, say so and return "needed": false — a wrong number is worse than none — but first check
  that the figure really is absent (e.g. the age's birth YEAR is enough; you do not need the month).

"label" names what the number IS, as a short noun phrase ("combined population of the three
counties"), so a later step can use the result without re-deriving it.
"uses" lists the INDEX NUMBERS of the facts whose figures you substituted.
Output ONLY JSON, no prose, no code fences:
{"needed": true/false, "expression": "<one Python expression, or empty>", "label": "<short noun phrase>", "uses": [<index number>, ...]}"""


def _render_facts(facts: list[str]) -> str:
    """Render the fact list for the LLM: one fact per line, prefixed with its index."""
    return "\n".join(f"[{i}] {f}" for i, f in enumerate(facts))


async def compute_from_facts(
    llm,
    question: str,
    facts: list[str],
    *,
    fit_budget: int | None = None,
) -> dict | None:
    """Ask the LLM whether ``question`` asks for a derivable number and, if so,
    write + safely evaluate the expression over ``facts``.

    ``llm`` exposes ``async_chat`` and ``max_length`` (a RAGFlow LLMBundle /
    CountingChatModel). ``facts`` is the list of discovered fact strings.

    Returns ``None`` when no derivation is needed or possible, otherwise a dict:
    ``{"needed": True, "label": str, "value": str, "expression": str, "uses": [int, ...]}``.
    The caller attaches ``value`` (with its expression and source references) to its
    evidence list.
    """
    if not question or not facts:
        return None
    from rag.prompts.generator import form_message, message_fit_in

    user = f"Facts discovered so far:\n{_render_facts(facts)}\n\nOriginal question:\n{question}\n\nOutput JSON:"
    try:
        budget = fit_budget or llm.max_length
        _, msg = message_fit_in(form_message(_COMPUTE_SYSTEM, user), budget)
        ans = await llm.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.0})
    except Exception:
        _LOG.exception("[Compute] LLM call failed")
        return None
    if isinstance(ans, tuple):
        ans = ans[0]
    if not isinstance(ans, str):
        return None

    import re

    import json_repair

    cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL).strip()
    cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
    try:
        data = json_repair.loads(cleaned)
    except Exception:
        _LOG.info("[Compute] could not parse LLM JSON: %r", ans[:200])
        return None
    if not isinstance(data, dict) or not data.get("needed"):
        return None

    expression = str(data.get("expression") or "").strip()
    if not expression:
        return None
    label = str(data.get("label") or "").strip() or "Value calculated from the facts found"
    uses = []
    for n in data.get("uses") or []:
        try:
            uses.append(int(n))
        except (TypeError, ValueError):
            continue

    value, error = compute(expression)
    if error:
        _LOG.info("[Compute] refused `%s` — %s", expression[:120], error)
        return None
    return {"needed": True, "label": label, "value": value, "expression": expression, "uses": uses}
