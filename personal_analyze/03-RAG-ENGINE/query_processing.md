# Query Processing

## Tong Quan

Query processing chuyển đổi user queries thành optimized search queries với term weighting và expansion.

## File Location
```
/rag/nlp/query.py
/rag/nlp/term_weight.py
```

## Query Processing Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    USER QUERY                                    │
│  "What is machine learning?"                                    │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 QUERY NORMALIZATION                              │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  1. Lowercase                                            │   │
│  │  2. Traditional → Simplified Chinese                     │   │
│  │  3. Full-width → Half-width characters                   │   │
│  │  4. Remove question words (what, how, why)               │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 TOKENIZATION                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  RAGFlowTokenizer:                                       │   │
│  │  - Fine-grained tokenization                             │   │
│  │  - Semantic tokenization                                 │   │
│  │  - Multi-language support                                │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 TF-IDF WEIGHTING                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  weight = (0.3 × IDF1 + 0.7 × IDF2) × NER × PoS         │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 QUERY EXPANSION                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  1. Synonym expansion (0.25x weight)                     │   │
│  │  2. Phrase queries (2x boost for bigrams)                │   │
│  │  3. Field boosting                                       │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 ELASTICSEARCH QUERY                              │
│  (term^weight synonym) AND "bigram phrase"^boost                │
└─────────────────────────────────────────────────────────────────┘
```

## Query Normalization

```python
class FulltextQueryer:
    @staticmethod
    def add_space_between_eng_zh(txt):
        """Add spaces between English and Chinese characters."""
        # "hello你好" → "hello 你好"
        return re.sub(r'([a-zA-Z])([\u4e00-\u9fa5])', r'\1 \2', txt)

    @staticmethod
    def rmWWW(txt):
        """Remove question words."""
        question_words = ["what", "how", "why", "when", "where", "who",
                         "什么", "如何", "为什么", "哪里", "谁"]
        for w in question_words:
            txt = txt.replace(w, "")
        return txt

    def question(self, txt, tbl="qa", min_match=0.6):
        """Process query text."""
        # Normalize
        txt = self.add_space_between_eng_zh(txt)
        txt = re.sub(r"[ :|\r\n\t,，。？?/`!！&^%%()\[\]{}<>]+", " ",
                     rag_tokenizer.tradi2simp(
                         rag_tokenizer.strQ2B(txt.lower())
                     )).strip()

        # Remove question words
        txt = self.rmWWW(txt)

        # Tokenize
        tks = rag_tokenizer.tokenize(txt)

        return self._build_query(tks, min_match)
```

## TF-IDF Weighting

```python
class Dealer:
    def weights(self, tks, preprocess=True):
        """
        Calculate TF-IDF weights for tokens.

        Formula:
            IDF(term) = log10(10 + (N - df(term) + 0.5) / (df(term) + 0.5))
            Weight = (0.3 × IDF1 + 0.7 × IDF2) × NER × PoS

        Where:
            IDF1: based on term frequency
            IDF2: based on document frequency
            NER: Named Entity Recognition weight
            PoS: Part-of-Speech tag weight
        """

        def idf(s, N):
            return math.log10(10 + ((N - s + 0.5) / (s + 0.5)))

        tw = []

        # IDF1: based on term frequency
        idf1 = np.array([idf(freq(t), 10000000) for t in tks])

        # IDF2: based on document frequency
        idf2 = np.array([idf(df(t), 1000000000) for t in tks])

        # Composite weight
        wts = (0.3 * idf1 + 0.7 * idf2) * np.array([
            ner(t) * postag(t) for t in tks
        ])

        # Normalize
        S = np.sum([s for _, s in tw])
        return [(t, s / S) for t, s in tw]
```

## NER and PoS Weights

```python
# Named Entity Recognition weights
NER_WEIGHTS = {
    "toxic": 2,      # Toxic/sensitive words
    "func": 1,       # Functional words
    "corp": 3,       # Corporation names
    "loca": 3,       # Location names
    "sch": 3,        # School names
    "stock": 3,      # Stock symbols
    "firstnm": 1,    # First names
}

# Part-of-Speech weights
POS_WEIGHTS = {
    "r": 0.3,        # Pronoun/Adverb
    "c": 0.3,        # Conjunction
    "d": 0.3,        # Adverb
    "ns": 3,         # Location noun
    "nt": 3,         # Organization noun
    "n": 2,          # Common noun
}

def ner(token):
    """Get NER weight for token."""
    for entity_type, weight in NER_WEIGHTS.items():
        if token in NER_DICT.get(entity_type, set()):
            return weight
    return 1

def postag(token):
    """Get PoS weight for token."""
    pos = get_pos_tag(token)
    return POS_WEIGHTS.get(pos, 1)
```

## Query Expansion

```python
def _build_query(self, tks_w, min_match=0.6):
    """
    Build expanded Elasticsearch query.

    Expansion strategies:
    1. Synonym expansion with 0.25x weight
    2. Bigram phrase queries with 2x boost
    3. Field boosting
    """

    # Sort by weight
    tks_w = sorted(tks_w, key=lambda x: x[1] * -1)

    q = []
    for (tk, w), syn in zip(tks_w[:256], syns):
        # Add term with synonym
        if syn:
            q.append(f"({tk}^{w:.4f} {syn})")
        else:
            q.append(f"{tk}^{w:.4f}")

    # Add phrase queries (bigrams) with 2x boost
    for i in range(1, len(tks_w)):
        left, right = tks_w[i - 1][0], tks_w[i][0]
        weight = max(tks_w[i - 1][1], tks_w[i][1]) * 2
        q.append(f'"{left} {right}"^{weight:.4f}')

    query = " ".join(q)

    # Build match expression with minimum_should_match
    return MatchTextExpr(
        query,
        fields=self.query_fields,
        min_match=f"{int(min_match * 100)}%"
    )
```

## Field Boosting

```python
# Query fields with boost factors
query_fields = [
    "title_tks^10",           # Title tokens: 10x boost
    "title_sm_tks^5",         # Small title tokens: 5x boost
    "important_kwd^30",       # Important keywords: 30x boost
    "important_tks^20",       # Important tokens: 20x boost
    "question_tks^20",        # Question tokens: 20x boost
    "content_ltks^2",         # Content tokens: 2x boost
    "content_sm_ltks",        # Small content tokens: 1x boost
]
```

## Synonym Expansion

```python
class SynonymLookup:
    def lookup(self, token):
        """
        Find synonyms for token.

        Returns:
            Space-separated synonym string with reduced weight
        """
        synonyms = self.synonym_dict.get(token, [])

        if not synonyms:
            return ""

        # Synonyms get 0.25x weight
        return " ".join(synonyms)

# Example:
# "machine learning" → "ML AI 机器学习"
```

## Final Query Example

```
Input: "What is machine learning?"

After processing:
(machine^0.8542 ML AI) (learning^0.7231 教育) "machine learning"^1.7084

With field boosting:
{
    "query_string": {
        "query": "(machine^0.8542 ML AI) (learning^0.7231) \"machine learning\"^1.7084",
        "fields": ["title_tks^10", "important_kwd^30", "content_ltks^2"],
        "minimum_should_match": "60%"
    }
}
```

## Tokenization

```python
# /rag/nlp/rag_tokenizer.py

class RAGFlowTokenizer:
    def tokenize(self, text, fine_grained=True):
        """
        Tokenize text with multi-granularity.

        Args:
            text: Input text
            fine_grained: Use fine-grained tokenization

        Returns:
            List of tokens
        """
        if fine_grained:
            # Fine-grained: "机器学习" → ["机器", "学习", "机", "器", "学", "习"]
            return self.fine_grained_tokenize(text)
        else:
            # Semantic: "机器学习" → ["机器学习"]
            return self.semantic_tokenize(text)

    def fine_grained_tokenize(self, text):
        """Break into smallest meaningful units."""
        tokens = []
        # ... tokenization logic
        return tokens

    def semantic_tokenize(self, text):
        """Keep semantic units intact."""
        tokens = []
        # ... tokenization logic
        return tokens
```

## Configuration

```python
# Search configuration
{
    "min_match": 0.3,              # Minimum term match percentage
    "query_fields": [...],         # Fields with boost factors
    "synonym_expansion": True,     # Enable synonym expansion
}

# Tokenizer configuration
{
    "fine_grained": True,          # Fine-grained tokenization
    "semantic": True,              # Also use semantic tokenization
}
```

## Related Files

- `/rag/nlp/query.py` - FulltextQueryer class
- `/rag/nlp/term_weight.py` - TF-IDF weighting
- `/rag/nlp/rag_tokenizer.py` - RAGFlow tokenizer
- `/rag/nlp/search.py` - Query integration in search
