# Chunking Strategies

## Tong Quan

Chunking chia documents thành các segments nhỏ hơn để indexing và retrieval hiệu quả.

## File Locations
```
/rag/nlp/__init__.py        # naive_merge() function
/rag/flow/splitter/         # Flow-based splitters
/rag/app/naive.py           # Document-specific chunking
```

## Chunking Algorithm

```
┌─────────────────────────────────────────────────────────────────┐
│                    DOCUMENT INPUT                                │
│  Parsed content with layout information                          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 DELIMITER SPLITTING                              │
│  Split by: \n 。 ； ！ ？ (newline + punctuation)               │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 TOKEN COUNTING                                   │
│  Count tokens for each segment                                  │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 ACCUMULATION                                     │
│  Accumulate segments until chunk_token_num exceeded             │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 OVERLAP HANDLING                                 │
│  Add overlap from previous chunk end                            │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 OUTPUT CHUNKS                                    │
│  List of (content, position) tuples                             │
└─────────────────────────────────────────────────────────────────┘
```

## Naive Merge Algorithm

```python
def naive_merge(sections: str | list, chunk_token_num=512,
                delimiter="\n。；！？", overlapped_percent=0):
    """
    Merge text sections into chunks based on token count.

    Args:
        sections: Input text or list of (text, position) tuples
        chunk_token_num: Maximum tokens per chunk (default: 512)
        delimiter: Characters to split on (default: punctuation)
        overlapped_percent: Percentage overlap between chunks (0-100)

    Returns:
        List of (chunk_text, positions) tuples
    """

    cks = [""]      # Chunks
    tk_nums = [0]   # Token counts
    poss = [[]]     # Positions

    def add_chunk(t, pos):
        tnum = num_tokens_from_string(t)

        # Check if current chunk exceeds threshold
        threshold = chunk_token_num * (100 - overlapped_percent) / 100.

        if cks[-1] == "" or tk_nums[-1] > threshold:
            # Start new chunk with overlap from previous
            if cks:
                overlapped = RAGFlowPdfParser.remove_tag(cks[-1])
                overlap_start = int(len(overlapped) * (100 - overlapped_percent) / 100.)
                t = overlapped[overlap_start:] + t

            cks.append(t)
            tk_nums.append(tnum)
            poss.append([pos] if pos else [])
        else:
            # Add to current chunk
            cks[-1] += t
            tk_nums[-1] += tnum
            if pos:
                poss[-1].append(pos)

    # Process each section
    for sec, pos in sections:
        add_chunk("\n" + sec, pos)

    return [(c, p) for c, p in zip(cks, poss) if c.strip()]
```

## Overlap Strategy

```
overlapped_percent = 10% (example)

Chunk 1: [────────────────────────][OVERLAP]
                                   │
Chunk 2:                   [OVERLAP][──────────────────────]
                                    │
Chunk 3:                            [OVERLAP][─────────────]

Where:
- Main content: chunk_token_num × (100 - overlapped_percent) / 100
- Overlap: chunk_token_num × overlapped_percent / 100
```

## Image-Aware Chunking

```python
def naive_merge_with_images(sections: list, images: list,
                            chunk_token_num=512, delimiter="\n。；！？"):
    """
    Merge sections while tracking associated images.

    Each chunk maintains references to images that appeared in its content.
    """

    chunks = naive_merge(sections, chunk_token_num, delimiter)

    # Track images per chunk
    chunk_images = []
    for chunk_text, positions in chunks:
        # Find images within position range
        associated_images = []
        for img, img_pos in images:
            if any(overlaps(img_pos, p) for p in positions):
                associated_images.append(img)

        chunk_images.append(associated_images)

    return [(c, p, imgs) for (c, p), imgs in zip(chunks, chunk_images)]
```

## Delimiter Handling

```python
# Custom delimiter format: backtick-wrapped
# Example: `###` splits on "###"

def split_by_delimiter(text, delimiter="\n。；！？"):
    """
    Split text by delimiters with priority handling.
    """

    # Check for custom delimiter
    if delimiter.startswith("`") and delimiter.endswith("`"):
        custom = delimiter[1:-1]
        return text.split(custom)

    # Standard delimiter splitting
    pattern = f"[{re.escape(delimiter)}]+"
    segments = re.split(pattern, text)

    return segments
```

## Document-Specific Chunking

### PDF Chunking
```python
# Uses layout information from parsing
sections = pdf_parser.parse()

# Each section has:
# - text content
# - layout_type (text, title, table, figure)
# - position (page, x0, x1, top, bottom)

chunks = naive_merge(sections, chunk_token_num=512)
```

### Table Chunking
```python
# Tables converted to natural language
# Each row becomes a sentence

def table_to_text(table_data):
    """
    Convert table structure to readable text.

    Example:
        Row 1, Column Name: Value
        Row 2, Column Name: Value
    """
    sentences = []
    for row_idx, row in enumerate(table_data):
        for col_name, value in row.items():
            sentences.append(f"Row {row_idx}, {col_name}: {value}")

    return "\n".join(sentences)
```

### Paper/Academic Chunking
```python
# Special handling for academic papers:
# - Abstract kept as single chunk (no splitting)
# - Title extraction from first pages
# - Section-based chunking
# - Figure/table captions preserved

def paper_chunk(sections):
    chunks = []

    for sec in sections:
        if sec.type == "abstract":
            # Keep abstract intact
            chunks.append((sec.text, sec.positions))
        else:
            # Normal chunking
            chunks.extend(naive_merge([sec], chunk_token_num=512))

    return chunks
```

## Configuration Parameters

```python
# Default chunking configuration
parser_config = {
    "chunk_token_num": 512,           # Tokens per chunk
    "delimiter": "\n。；！？",         # Split characters
    "overlapped_percent": 0,           # Overlap percentage (0-100)
    "layout_recognize": "DeepDOC",     # Layout detection method
}

# Recommended values by document type:
# Technical docs: chunk_token_num=512, overlapped_percent=10
# Legal docs: chunk_token_num=256, overlapped_percent=20
# Books: chunk_token_num=1024, overlapped_percent=5
# Q&A: chunk_token_num=128, overlapped_percent=0
```

## Token Counting

```python
def num_tokens_from_string(string: str) -> int:
    """
    Count tokens using tiktoken (GPT-4 tokenizer).

    Used for accurate chunk size estimation.
    """
    import tiktoken

    encoding = tiktoken.encoding_for_model("gpt-4")
    return len(encoding.encode(string))
```

## Position Tracking

```python
# Position tag format in content
# @@{page}\t{x0}\t{x1}\t{top}\t{bottom}##

def extract_positions(content):
    """
    Extract position tags from content.

    Returns list of (page, x0, x1, top, bottom) tuples.
    """
    pattern = r"@@(\d+)\t([\d.]+)\t([\d.]+)\t([\d.]+)\t([\d.]+)##"
    matches = re.findall(pattern, content)

    return [(int(m[0]), float(m[1]), float(m[2]), float(m[3]), float(m[4]))
            for m in matches]
```

## Flow-Based Splitter

```python
# /rag/flow/splitter/splitter.py

class Splitter(Component):
    """
    Pipeline component for chunking.

    Inputs:
        - markdown / text / html / json content

    Parameters:
        - chunk_token_size: 512
        - delimiters: \n。；！？

    Outputs:
        - List of chunks with metadata
    """

    def invoke(self, content, **kwargs):
        chunks = naive_merge(
            content,
            chunk_token_num=self.params.chunk_token_size,
            delimiter=self.params.delimiters
        )

        return [{
            "content": chunk,
            "positions": positions,
            "token_count": num_tokens_from_string(chunk)
        } for chunk, positions in chunks]
```

## Related Files

- `/rag/nlp/__init__.py` - naive_merge implementation
- `/rag/flow/splitter/splitter.py` - Flow splitter component
- `/rag/app/naive.py` - Document chunking logic
- `/rag/app/paper.py` - Paper-specific chunking
