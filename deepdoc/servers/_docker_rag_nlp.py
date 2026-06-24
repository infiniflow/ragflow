# Stub rag.nlp module for Docker deepdoc service.
# Provides minimal rag_tokenizer to satisfy table_structure_recognizer import.


class _StubTokenizer:
    """Minimal tokenizer stub — only blockType() uses tokenize in TSR."""

    def tokenize(self, text):
        return text

    def tag(self, word):
        return ""


rag_tokenizer = _StubTokenizer()
