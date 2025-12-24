# NLP Algorithms

## Tong Quan

RAGFlow sử dụng multiple NLP algorithms cho tokenization, term weighting, và query processing.

## 1. Trie-based Tokenization

### File Location
```
/rag/nlp/rag_tokenizer.py (lines 72-90, 120-240)
```

### Purpose
Chinese word segmentation sử dụng Trie data structure.

### Implementation

```python
import datrie

class RagTokenizer:
    def __init__(self):
        # Load dictionary into Trie
        self.trie = datrie.Trie(string.printable + "".join(
            chr(i) for i in range(0x4E00, 0x9FFF)  # CJK characters
        ))

        # Load from huqie.txt dictionary
        for line in open("rag/res/huqie.txt"):
            word, freq, pos = line.strip().split("\t")
            self.trie[word] = (int(freq), pos)

    def _max_forward(self, text, start):
        """
        Max-forward matching algorithm.
        """
        end = len(text)
        while end > start:
            substr = text[start:end]
            if substr in self.trie:
                return substr, end
            end -= 1
        return text[start], start + 1

    def _max_backward(self, text, end):
        """
        Max-backward matching algorithm.
        """
        start = 0
        while start < end:
            substr = text[start:end]
            if substr in self.trie:
                return substr, start
            start += 1
        return text[end-1], end - 1
```

### Trie Structure

```
Trie Data Structure:
         root
        /    \
       机     学
      /       \
     器        习
    / \
   学  人

Words: 机器, 机器学习, 机器人, 学习

Lookup: O(m) where m = word length
Insert: O(m)
Space: O(n × m) where n = number of words
```

### Max-Forward/Backward Algorithm

```
Max-Forward Matching:
Input: "机器学习是人工智能"

Step 1: Try "机器学习是人工智能" → Not found
Step 2: Try "机器学习是人工" → Not found
...
Step n: Try "机器学习" → Found!
Output: ["机器学习", ...]

Max-Backward Matching:
Input: "机器学习"

Step 1: Try "机器学习" from end → Found!
Output: ["机器学习"]
```

---

## 2. DFS with Memoization (Disambiguation)

### File Location
```
/rag/nlp/rag_tokenizer.py (lines 120-210)
```

### Purpose
Giải quyết ambiguity khi có nhiều cách tokenize.

### Implementation

```python
def dfs_(self, text, memo={}):
    """
    DFS with memoization for tokenization disambiguation.
    """
    if text in memo:
        return memo[text]

    if not text:
        return [[]]

    results = []
    for end in range(1, len(text) + 1):
        prefix = text[:end]
        if prefix in self.trie or len(prefix) == 1:
            suffix_results = self.dfs_(text[end:], memo)
            for suffix in suffix_results:
                results.append([prefix] + suffix)

    # Score and select best tokenization
    best = max(results, key=lambda x: self.score_(x))
    memo[text] = [best]
    return [best]

def score_(self, tokens):
    """
    Score tokenization quality.

    Formula: score = B/len(tokens) + L + F
    where:
        B = 30 (bonus for fewer tokens)
        L = sum of token lengths / total length
        F = sum of frequencies
    """
    B = 30
    L = sum(len(t) for t in tokens) / max(1, sum(len(t) for t in tokens))
    F = sum(self.trie.get(t, (1, ''))[0] for t in tokens)

    return B / len(tokens) + L + F
```

### Scoring Function

```
Tokenization Scoring:

score(tokens) = B/n + L + F

where:
- B = 30 (base bonus)
- n = number of tokens (fewer is better)
- L = coverage ratio
- F = sum of word frequencies (common words preferred)

Example:
"北京大学" →
  Option 1: ["北京", "大学"] → score = 30/2 + 1.0 + (1000+500) = 1516
  Option 2: ["北", "京", "大", "学"] → score = 30/4 + 1.0 + (10+10+10+10) = 48.5

Winner: Option 1
```

---

## 3. TF-IDF Term Weighting

### File Location
```
/rag/nlp/term_weight.py (lines 162-244)
```

### Purpose
Tính importance weight cho mỗi term trong query.

### Implementation

```python
import math
import numpy as np

class Dealer:
    def weights(self, tokens, preprocess=True):
        """
        Calculate TF-IDF based weights for tokens.
        """
        def idf(s, N):
            return math.log10(10 + ((N - s + 0.5) / (s + 0.5)))

        # IDF based on term frequency in corpus
        idf1 = np.array([idf(self.freq(t), 10000000) for t in tokens])

        # IDF based on document frequency
        idf2 = np.array([idf(self.df(t), 1000000000) for t in tokens])

        # NER and POS weights
        ner_weights = np.array([self.ner(t) for t in tokens])
        pos_weights = np.array([self.postag(t) for t in tokens])

        # Combined weight
        weights = (0.3 * idf1 + 0.7 * idf2) * ner_weights * pos_weights

        # Normalize
        total = np.sum(weights)
        return [(t, w / total) for t, w in zip(tokens, weights)]
```

### Formula

```
TF-IDF Variant:

IDF(term) = log₁₀(10 + (N - df + 0.5) / (df + 0.5))

where:
- N = total documents (10⁹ for df, 10⁷ for freq)
- df = document frequency of term

Combined Weight:
weight(term) = (0.3 × IDF_freq + 0.7 × IDF_df) × NER × POS

Normalization:
normalized_weight(term) = weight(term) / Σ weight(all_terms)
```

---

## 4. Named Entity Recognition (NER)

### File Location
```
/rag/nlp/term_weight.py (lines 84-86, 144-149)
```

### Purpose
Dictionary-based entity type detection với weight assignment.

### Implementation

```python
class Dealer:
    def __init__(self):
        # Load NER dictionary
        self.ner_dict = json.load(open("rag/res/ner.json"))

    def ner(self, token):
        """
        Get NER weight for token.
        """
        NER_WEIGHTS = {
            "toxic": 2.0,      # Toxic/sensitive words
            "func": 1.0,       # Functional words
            "corp": 3.0,       # Corporation names
            "loca": 3.0,       # Location names
            "sch": 3.0,        # School names
            "stock": 3.0,      # Stock symbols
            "firstnm": 1.0,    # First names
        }

        for entity_type, weight in NER_WEIGHTS.items():
            if token in self.ner_dict.get(entity_type, set()):
                return weight

        return 1.0  # Default
```

### Entity Types

```
NER Categories:
┌──────────┬────────┬─────────────────────────────┐
│ Type     │ Weight │ Examples                    │
├──────────┼────────┼─────────────────────────────┤
│ corp     │ 3.0    │ Microsoft, Google, Apple    │
│ loca     │ 3.0    │ Beijing, New York           │
│ sch      │ 3.0    │ MIT, Stanford               │
│ stock    │ 3.0    │ AAPL, GOOG                  │
│ toxic    │ 2.0    │ (sensitive words)           │
│ func     │ 1.0    │ the, is, are                │
│ firstnm  │ 1.0    │ John, Mary                  │
└──────────┴────────┴─────────────────────────────┘
```

---

## 5. Part-of-Speech (POS) Tagging

### File Location
```
/rag/nlp/term_weight.py (lines 179-189)
```

### Purpose
Assign weights based on grammatical category.

### Implementation

```python
def postag(self, token):
    """
    Get POS weight for token.
    """
    POS_WEIGHTS = {
        "r": 0.3,   # Pronoun
        "c": 0.3,   # Conjunction
        "d": 0.3,   # Adverb
        "ns": 3.0,  # Place noun
        "nt": 3.0,  # Organization noun
        "n": 2.0,   # Common noun
    }

    # Get POS tag from tokenizer
    pos = self.tokenizer.tag(token)

    # Check for numeric patterns
    if re.match(r"^[\d.]+$", token):
        return 2.0

    return POS_WEIGHTS.get(pos, 1.0)
```

### POS Weight Table

```
POS Weight Assignments:
┌───────┬────────┬─────────────────────┐
│ Tag   │ Weight │ Description         │
├───────┼────────┼─────────────────────┤
│ n     │ 2.0    │ Common noun         │
│ ns    │ 3.0    │ Place noun          │
│ nt    │ 3.0    │ Organization noun   │
│ v     │ 1.0    │ Verb                │
│ a     │ 1.0    │ Adjective           │
│ r     │ 0.3    │ Pronoun             │
│ c     │ 0.3    │ Conjunction         │
│ d     │ 0.3    │ Adverb              │
│ num   │ 2.0    │ Number              │
└───────┴────────┴─────────────────────┘
```

---

## 6. Synonym Detection

### File Location
```
/rag/nlp/synonym.py (lines 71-93)
```

### Purpose
Query expansion qua synonym lookup.

### Implementation

```python
from nltk.corpus import wordnet

class SynonymLookup:
    def __init__(self):
        # Load custom dictionary
        self.custom_dict = json.load(open("rag/res/synonym.json"))

    def lookup(self, token, top_n=8):
        """
        Find synonyms for token.

        Strategy:
        1. Check custom dictionary first
        2. Fall back to WordNet for English
        """
        # Custom dictionary
        if token in self.custom_dict:
            return self.custom_dict[token][:top_n]

        # WordNet for English words
        if re.match(r"^[a-z]+$", token.lower()):
            synonyms = set()
            for syn in wordnet.synsets(token):
                for lemma in syn.lemmas():
                    name = lemma.name().replace("_", " ")
                    if name.lower() != token.lower():
                        synonyms.add(name)

            return list(synonyms)[:top_n]

        return []
```

### Synonym Sources

```
Synonym Lookup Strategy:

1. Custom Dictionary (highest priority)
   - Domain-specific synonyms
   - Chinese synonyms
   - Technical terms

2. WordNet (English only)
   - General synonyms
   - Lemma extraction from synsets

Example:
"computer" → WordNet → ["machine", "calculator", "computing device"]
"机器学习" → Custom → ["ML", "machine learning", "深度学习"]
```

---

## 7. Query Expansion

### File Location
```
/rag/nlp/query.py (lines 85-218)
```

### Purpose
Build expanded query với weighted terms và synonyms.

### Implementation

```python
class FulltextQueryer:
    QUERY_FIELDS = [
        "title_tks^10",        # Title: 10x boost
        "title_sm_tks^5",      # Title sub-tokens: 5x
        "important_kwd^30",    # Keywords: 30x
        "important_tks^20",    # Keyword tokens: 20x
        "question_tks^20",     # Question tokens: 20x
        "content_ltks^2",      # Content: 2x
        "content_sm_ltks^1",   # Content sub-tokens: 1x
    ]

    def question(self, text, min_match=0.6):
        """
        Build expanded query.
        """
        # 1. Tokenize
        tokens = self.tokenizer.tokenize(text)

        # 2. Get weights
        weighted_tokens = self.term_weight.weights(tokens)

        # 3. Get synonyms
        synonyms = [self.synonym.lookup(t) for t, _ in weighted_tokens]

        # 4. Build query string
        query_parts = []
        for (token, weight), syns in zip(weighted_tokens, synonyms):
            if syns:
                # Token with synonyms
                syn_str = " ".join(syns)
                query_parts.append(f"({token}^{weight:.4f} OR ({syn_str})^0.2)")
            else:
                query_parts.append(f"{token}^{weight:.4f}")

        # 5. Add phrase queries (bigrams)
        for i in range(1, len(weighted_tokens)):
            left, _ = weighted_tokens[i-1]
            right, w = weighted_tokens[i]
            query_parts.append(f'"{left} {right}"^{w*2:.4f}')

        return MatchTextExpr(
            query=" ".join(query_parts),
            fields=self.QUERY_FIELDS,
            min_match=f"{int(min_match * 100)}%"
        )
```

### Query Expansion Example

```
Input: "machine learning tutorial"

After expansion:
(machine^0.35 OR (computer device)^0.2)
(learning^0.40 OR (study education)^0.2)
(tutorial^0.25 OR (guide lesson)^0.2)
"machine learning"^0.80
"learning tutorial"^0.50

With field boosting:
{
    "query_string": {
        "query": "(machine^0.35 learning^0.40 tutorial^0.25)",
        "fields": ["title_tks^10", "important_kwd^30", "content_ltks^2"],
        "minimum_should_match": "60%"
    }
}
```

---

## 8. Fine-Grained Tokenization

### File Location
```
/rag/nlp/rag_tokenizer.py (lines 395-420)
```

### Purpose
Secondary tokenization cho compound words.

### Implementation

```python
def fine_grained_tokenize(self, text):
    """
    Break compound words into sub-tokens.
    """
    # First pass: standard tokenization
    tokens = self.tokenize(text)

    fine_tokens = []
    for token in tokens:
        # Skip short tokens
        if len(token) < 3:
            fine_tokens.append(token)
            continue

        # Try to break into sub-tokens
        sub_tokens = self.dfs_(token)
        if len(sub_tokens[0]) > 1:
            fine_tokens.extend(sub_tokens[0])
        else:
            fine_tokens.append(token)

    return fine_tokens
```

### Example

```
Standard: "机器学习" → ["机器学习"]
Fine-grained: "机器学习" → ["机器", "学习"]

Standard: "人工智能" → ["人工智能"]
Fine-grained: "人工智能" → ["人工", "智能"]
```

---

## Summary

| Algorithm | Purpose | File |
|-----------|---------|------|
| Trie Tokenization | Word segmentation | rag_tokenizer.py |
| Max-Forward/Backward | Matching strategy | rag_tokenizer.py |
| DFS + Memo | Disambiguation | rag_tokenizer.py |
| TF-IDF | Term weighting | term_weight.py |
| NER | Entity detection | term_weight.py |
| POS Tagging | Grammatical analysis | term_weight.py |
| Synonym | Query expansion | synonym.py |
| Query Expansion | Search enhancement | query.py |
| Fine-grained | Sub-tokenization | rag_tokenizer.py |

## Related Files

- `/rag/nlp/rag_tokenizer.py` - Tokenization
- `/rag/nlp/term_weight.py` - TF-IDF, NER, POS
- `/rag/nlp/synonym.py` - Synonym lookup
- `/rag/nlp/query.py` - Query processing
