# Retrieval Evaluation Handbook

| | |
|---|---|
| Document ID | `retrieval_md_20_ret.md` |
| Component code | `RET` |
| Owner | Platform Reliability (ret-team) |
| Last reviewed | 2026-09-10 |
| Review cycle | every 90 days |

## 1. Purpose

This handbook covers **Retrieval Evaluation Handbook**. It exists to measure and improve retrieval quality, including recall, ranking and the effect of configuration changes.

It is written for on-call engineers who already have access to the platform
console and to the read-only observability dashboards. Everything in this
document describes steady-state behaviour; emergency procedures are owned by
the incident commander and are not repeated here.

## 2. Service endpoints

| Endpoint | Description |
|---|---|
| `POST /v1/eval/runs` | 提交一次离线评测 |
| `GET /v1/eval/runs/{id}` | 读取评测指标与逐条明细 |

All endpoints require a bearer token and are subject to the platform rate
limit. Requests without a valid token are rejected before any work is done.

## 3. Configuration defaults

Unless a site profile overrides them, the following values apply to every
deployment of Retrieval Evaluation Handbook. These are the values the operations team should quote
when sizing, tuning or comparing environments.

- **default worker concurrency**: 16 workers
- **request timeout**: 80 seconds
- **maximum retry count**: 2 retries
- **health check interval**: 35 seconds
- **maintenance batch size**: 384 items

Changing any of the values above is a configuration change and must go through
the normal review process. Values that differ from the defaults should be
recorded in the site profile so that later comparisons remain meaningful.

## 4. Other tuning parameters

The parameters below are environment-specific and are normally left at the
values shown. They are listed here for completeness.

| Parameter | Typical value |
|---|---|
| Maximum concurrent uploads | 2 |
| Log retention | 34 days |
| Slow query threshold | 400 ms |
| Circuit breaker threshold | 5 consecutive failures |
| Trace sampling ratio | 0.1 |

## 5. Normal operation

1. Confirm that the health endpoint reports the component as serving before
   starting any maintenance work.
2. Verify that no rebalance, rebuild or compaction is already in progress.
3. Apply changes to one node or one replica group at a time.
4. Watch the error rate and the latency dashboards for at least one full window
   after each step.
5. Record the change in the operations log with the exact parameter values
   used, so that the next reviewer can reconstruct the state.

## 6. Failure handling

| Symptom | First action |
|---|---|
| 评测结果不可复现 | 固定随机种子与候选集快照 |
| 标注与文档版本不匹配 | 用文档指纹校验标注时效 |

If none of the actions above resolves the symptom, escalate to the component
owner rather than retrying indefinitely: repeated retries of a saturated
component make the original symptom harder to observe.

## 7. Maintenance window procedure

- Announce the window at least one business day in advance.
- Take a restorable snapshot before touching configuration.
- Prefer draining traffic over restarting under load.
- After the window, re-run the standard smoke checks and the retrieval
  evaluation job before declaring the window closed.

## 8. Related documents

- `retrieval_md_05_chk.md`
- `retrieval_md_09_rank.md`
