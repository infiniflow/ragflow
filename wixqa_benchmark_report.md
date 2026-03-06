# WixQA A/B Benchmark Report

## Dataset
- Source: [Wix/WixQA](https://huggingface.co/datasets/Wix/WixQA)
- Questions: 100 total (50 expert-written, 50 simulated)
- Knowledge Bases: 2
  - `wix-expert-articles`: Wix support articles referenced by expert-written Q&A
  - `wix-sim-articles`: Wix support articles referenced by simulated Q&A

## Routing Distribution
{
  "wix-expert-articles": 48,
  "wix-sim-articles": 52
}

## Summary

| Metric            | Baseline (Both KBs) | With Routing | Delta       |
|-------------------|--------------------|--------------|-------------|
| Context Precision | 0.543         | 0.554   | +2.0% |
| Context Recall    | 0.287         | 0.300   | +4.5% |
| Avg Latency (ms)  | 403.1 | 366.2 | -9.2% |
| Errors            | 0         | 0   | —           |

## Latency Details

| Stat  | Baseline | Routing |
|-------|----------|---------|
| avg   | 403.1 ms | 366.2 ms |
| p50   | 357.0 ms | 338.4 ms |
| p90   | 516.4 ms | 479.1 ms |
| p95   | 632.9 ms | 531.2 ms |

## Dataset IDs
{
  "wix-expert-articles": "04c16e78190611f1912ae375f48003ca",
  "wix-sim-articles": "422ee506190611f1912ae375f48003ca"
}

## Interpretation
- **Context Precision**: fraction of retrieved chunks that are relevant.
- **Context Recall**: fraction of relevant information that was retrieved.
- Higher precision with routing means MCP Skill eliminates irrelevant KB noise.
- Lower latency reflects fewer documents being searched.
