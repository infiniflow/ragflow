# SET / COUNT directions — the member list IS the work

Your Direction asks for a SET of named things: a batch of names is a NEW fact, not a redundant search. The convergence rule in the playbook is about re-asking what you already have; here, keep probing while the `[record]` line is still changing.

1. Guess MORE names than you expect — aliases, names seen once, names you doubt. A wrong guess costs one probe; a missing member costs the answer.
2. Probe in batches of four to six, space-separated: `name1 name2 name3`.
3. Read every returned passage for members the batch did not name, and probe those too — the corpus names the tail, not your memory.
4. Record as you go, so a timeout cannot lose what you found.
5. Stop when two consecutive batches add no member you did not have.

## What the results now carry

- **Every tool result ends with a `[record]` line. Read it, and trust it over your own impression of what you have done.** It carries the facts you cannot see in the conversation: `members` (what the slots hold now), `probed-reached` (the names you asked about that came back with a passage), `asked-nothing-back` (names that came back with NOTHING — change the wording or the angle, do not re-ask them as they are), and `FOUND BUT NOT RECORDED` (names you proved reachable and never put in a slot — resolve these before searching anything new).
- **A `[pool]` line may follow it: an excerpt from evidence ALREADY in hand that you have not been shown** (the id is its chunk). Read it for members the list is missing, and probe any name it carries that the list does not — the passage is already evidence, so naming it costs no retrieval you do not have.
- **A `[reach]` line may follow a batched or pattern query**: the terms that call reached (with how many passages each) and the ones it did NOT reach. Aim the next call at the second list, or re-word it — a term listed there was not reached by THAT query, which is not the same as absent from the corpus.
- **The direction is exhausted when the `[record]` line stops changing** — no new member, no newly reached or unanswered name — and the slots are filled. Then return the state patch immediately. While it IS still changing, the next useful call is a probe of a `FOUND BUT NOT RECORDED` name, or of an angle the line shows you have not tried.
- **What you found only counts if your patch carries it.** Neither your searches nor your reading of them reach the stage that writes the answer — only your patch does. So end the session carrying EVERY member you confirmed, not just the last batch's: measured, a round whose pool already held every member's passage recorded a fraction of them, because the session spent every turn searching and patched last, when its budget was gone.
