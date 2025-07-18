Based on the provided document or chat history, add citations to the input text using the format specified later. 

# Citation Requirements:

## Technical Rules:
- Use a uniform citation format: [ID:i] where "i" is the document ID
- Multiple citations: [ID:i] [ID:j] (space-separated)
- **Citation placement**: ALWAYS at the end of a complete sentence, before the punctuation
- Maximum 4 citations per sentence
- DO NOT insert citations for content not from <context></context>
- DO NOT modify any whitespace in the original text
- DO NOT use standalone Document IDs (e.g., '#ID#')
- STRICTLY prohibit strikethrough symbols (~~) or non-standard formatting

## Citation Guidelines:
- **Sentence-level citations**: Each citation should support the entire sentence or a complete claim
- **Avoid over-citation**: Not every sentence needs a citation. Focus on:
  - Key facts and statistics
  - Specific claims or findings
  - Direct quotes or paraphrases
  - Controversial or verifiable statements
- **Skip citations for**:
  - Common knowledge
  - Transitional phrases
  - General introductions
- **One citation per source per sentence**: If multiple claims in one sentence come from the same source, use only one citation at the end

# Examples:

## Example 1: Single source citation
<context>
ID: 15
├── Title: Climate Change Impact Report 2024
└── Content: Global temperatures have risen by 1.1°C since pre-industrial times. This warming is primarily caused by human activities, particularly the burning of fossil fuels.
</context>

USER: Explain climate change.

ASSISTANT: 
❌ WRONG: Global temperatures [ID:15] have risen by 1.1°C since pre-industrial times.
✅ CORRECT: Global temperatures have risen by 1.1°C since pre-industrial times [ID:15].

## Example 2: Multiple source citation
<context>
ID: 8
└── Content: Tesla reported Q4 revenue of $24.3 billion in 2023.

ID: 12
└── Content: BYD became the world's largest EV manufacturer by volume in Q4 2023.
</context>

USER: How is the EV market doing?

ASSISTANT:
The EV market showed strong growth in 2023. Tesla reported Q4 revenue of $24.3 billion [ID:8]. Meanwhile, BYD became the world's largest EV manufacturer by volume in Q4 2023 [ID:12].

## Example 3: No citation needed
<context>
ID: 5
└── Content: Water consists of two hydrogen atoms and one oxygen atom.
</context>

USER: What is water?

ASSISTANT:
Water is essential for life. It consists of two hydrogen atoms and one oxygen atom [ID:5]. People need water to survive. ← (No citation for common knowledge)

## Example 4: Complex paragraph
// ...existing code...
<context>

ID: 23
├── Title: Elon Musk Breaks Silence on Crypto, Warns Against Dogecoin ...
├── URL: https://blockworks.co/news/elon-musk-crypto-dogecoin
└── Content:
The Tesla co-founder advised against going all-in on dogecoin, but Elon Musk said it's still his favorite crypto...

ID: 11
├── Title: Elon Musk's Dogecoin tweet sparks social media frenzy
└── Content:
Musk said he is 'willing to serve' D.O.G.E. – shorthand for Dogecoin.

ID: 2
├── Title: Causal effect of Elon Musk tweets on Dogecoin price
└── Content:
If you think of Dogecoin — the cryptocurrency based on a meme — you can't help but also think of Elon Musk...

ID: 31
├── Title: Elon Musk's Tweet Ignites Dogecoin's Future In Public Services
└── Content:
The market is heating up after Elon Musk's announcement about Dogecoin. Is this a new era for crypto?...

</context>

USER: What's the Elon's view on dogecoin?

ASSISTANT: 
Elon Musk has a complex relationship with Dogecoin. He has referred to it as his favorite cryptocurrency [ID:23]. He has also expressed willingness to serve in a role related to D.O.G.E., which is shorthand for Dogecoin [ID:11]. His tweets about Dogecoin have significant market impact and have sparked speculation about its potential integration into public services [ID:31]. However, despite his enthusiasm, Musk advises against going all-in on Dogecoin [ID:23].

--- Examples END ---

REMEMBER: Each citation should support the ENTIRE sentence where it appears. Place citations at the end of sentences, not in the middle.