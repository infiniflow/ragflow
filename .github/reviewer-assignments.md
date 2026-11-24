# Automatic PR reviewer assignments

`CODEOWNERS` assigns exactly one reviewer to each file. A PR spanning several owners requests each distinct owner. Nested matches replace parent ownership; they do not add reviewers.

## Selection

Snapshot: `64fc77e98c42ee1fccd9d25645031db85eaca459` (2026-11-17). Scanned all tracked files and the complete non-merge commit history reachable from this commit. Untracked local files were excluded. Evaluated 147 directories: depth 1 for all roots, plus depths 2 and 3 under `internal/` and `web/`. For example, `internal/service/document/` is depth 3. Deeper directories inherit that owner.

- Restrict candidates to the supplied specialty list: backend for application code/configuration/SDK/tests, frontend for `web/`, build for `.github/`, `bin/`, `deps/`, `docker/`, `scripts/`, and `tools/`, documentation for `.agents/` instruction files and `docs/`.
- Rank by distinct authored commits touching the folder, then by file touches, then by latest authored contribution. A commit counts once per folder, including descendants. A file touch is one changed path in a commit; additions, modifications, and deletions count equally. Historical paths are counted as recorded, without following renames. Merge commits and co-author trailers are excluded.
- Use the entire reachable history without a recency cutoff. File-touch counts break frequency ties without requiring historical file contents, which are incomplete in this partial clone.
- Normalize Git author aliases to the supplied GitHub handles. Notable aliases: `balibabu` → `cike8899`; `chanx`/`dcc123456` → `dcc123456`; `Jack`/`xugangqiang` → `xugangqiang`; `Jin Hai`/`jinhai` → `JinHai-CN`; `Lynn` → `Lynn-Inf`; `Sbaaoui Idriss`/`Idriss Sbaaoui` → `6ba3i`; `maoyifeng` → `myf-bee`; `YanZhang` → `Yannnnnnny`. Identities were matched using their Git author email; author name alone was not used.
- When no eligible reviewer has a recorded contribution, inherit the parent owner; at the top level, use the designated specialist. These assignments are marked below.
- Root files and new top-level folders default to `@JinHai-CN`, the most frequent backend author (684 non-merge commits repository-wide). Root Markdown/license files go to `@Yannnnnnny`; root Dockerfiles, shell scripts, Docker ignore, code coverage, and hook configuration go to `@myf-bee`.
- Combine identical ownership through parent rules. Keep explicit overrides only when a nested folder has a different winner. No ownership rule goes below depth 3.

## Activation

Merge `.github/CODEOWNERS` into each PR target branch where automatic reviewer requests are wanted. GitHub reads ownership from the PR base branch and requests reviews for opened, non-draft PRs and drafts marked ready for review. Owners must have explicit write access. GitHub does not request a review from the PR author themselves; there is no alternate owner configured. This file does not enable required approvals or change branch protection.

Repository permissions and live PR behavior were not verified because GitHub API access was unavailable in this environment. The assignments use the handles supplied by the maintainer.

Reference: [GitHub code owners documentation](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners).

## Folder evidence

Counts are authored commits / file touches. The runner-up is the next eligible reviewer with at least one contribution. Inherited folders remain listed so the full scan is reviewable.

| Folder | Reviewer | Commits / file touches | Runner-up (commits / file touches) |
| --- | --- | --- | --- |
| `.agents/` | @Yannnnnnny | 0 / 0 (specialist fallback) | — |
| `.github/` | @myf-bee | 36 / 37 | — |
| `bin/` | @myf-bee | 0 / 0 (specialist fallback) | — |
| `cmd/` | @JinHai-CN | 80 / 110 | @xugangqiang (29 / 46) |
| `conf/` | @JinHai-CN | 47 / 133 | @Lynn-Inf (29 / 62) |
| `deps/` | @myf-bee | 1 / 2 | — |
| `docker/` | @myf-bee | 9 / 21 | — |
| `docs/` | @Yannnnnnny | 49 / 849 | — |
| `example/` | @JinHai-CN | 2 / 2 | @buua436 (1 / 10) |
| `internal/` | @JinHai-CN | 330 / 5322 | @xugangqiang (305 / 3409) |
| `internal/admin/` | @JinHai-CN | 102 / 239 | @wangq8 (17 / 31) |
| `internal/agent/` | @6ba3i | 88 / 320 | @JinHai-CN (34 / 336) |
| `internal/agent/audio/` | @JinHai-CN | 6 / 8 | @buua436 (2 / 3) |
| `internal/agent/canvas/` | @6ba3i | 24 / 51 | @yuzhichang (12 / 99) |
| `internal/agent/chat/` | @qinling0210 | 2 / 5 | @yingfeng (1 / 2) |
| `internal/agent/component/` | @6ba3i | 57 / 161 | @JinHai-CN (18 / 139) |
| `internal/agent/dsl/` | @yuzhichang | 6 / 54 | @6ba3i (3 / 4) |
| `internal/agent/retrievalbridge/` | @buua436 | 5 / 7 | @6ba3i (4 / 7) |
| `internal/agent/runtime/` | @yuzhichang | 11 / 37 | @JinHai-CN (6 / 10) |
| `internal/agent/sandbox/` | @6ba3i | 6 / 41 | @yuzhichang (6 / 28) |
| `internal/agent/templates/` | @JinHai-CN | 1 / 19 | @buua436 (1 / 2) |
| `internal/agent/tool/` | @yuzhichang | 16 / 148 | @JinHai-CN (13 / 72) |
| `internal/agent/workflowx/` | @6ba3i | 5 / 9 | @yuzhichang (4 / 14) |
| `internal/agentic_rag/` | @wangq8 | 5 / 16 | @yuzhichang (3 / 46) |
| `internal/agentic_rag/decisionhead/` | @wangq8 | 0 / 0 (inherit parent) | — |
| `internal/binding/` | @buua436 | 5 / 29 | @JinHai-CN (3 / 143) |
| `internal/binding/cpp/` | @buua436 | 5 / 28 | @JinHai-CN (2 / 141) |
| `internal/channels/` | @buua436 | 5 / 31 | @6ba3i (3 / 10) |
| `internal/channels/core/` | @buua436 | 0 / 0 (inherit parent) | — |
| `internal/channels/whatsapp-gateway/` | @buua436 | 2 / 8 | @JinHai-CN (1 / 4) |
| `internal/cli/` | @JinHai-CN | 118 / 530 | @qinling0210 (16 / 83) |
| `internal/cli/filesystem/` | @JinHai-CN | 7 / 12 | @wangq8 (2 / 3) |
| `internal/cli/utils/` | @JinHai-CN | 1 / 1 | — |
| `internal/common/` | @JinHai-CN | 58 / 87 | @xugangqiang (46 / 85) |
| `internal/compliance/` | @wangq8 | 1 / 1 | — |
| `internal/dao/` | @JinHai-CN | 69 / 204 | @buua436 (31 / 110) |
| `internal/dao/transaction/` | @Lynn-Inf | 2 / 3 | @6ba3i (1 / 1) |
| `internal/deepdoc/` | @xugangqiang | 81 / 764 | @JinHai-CN (13 / 72) |
| `internal/deepdoc/native/` | @xugangqiang | 17 / 80 | @JinHai-CN (2 / 2) |
| `internal/deepdoc/parser/` | @xugangqiang | 70 / 678 | @JinHai-CN (12 / 66) |
| `internal/docguard/` | @xugangqiang | 1 / 1 | — |
| `internal/engine/` | @JinHai-CN | 37 / 111 | @qinling0210 (33 / 129) |
| `internal/engine/clickhouse/` | @JinHai-CN | 5 / 7 | @yuzhichang (1 / 2) |
| `internal/engine/elasticsearch/` | @JinHai-CN | 19 / 36 | @qinling0210 (17 / 38) |
| `internal/engine/infinity/` | @qinling0210 | 28 / 75 | @JinHai-CN (9 / 28) |
| `internal/engine/internal/` | @xugangqiang | 4 / 10 | — |
| `internal/engine/kvrocks/` | @xugangqiang | 2 / 7 | @Lynn-Inf (1 / 1) |
| `internal/engine/metafilter/` | @xugangqiang | 1 / 2 | — |
| `internal/engine/nats/` | @JinHai-CN | 6 / 6 | @xugangqiang (5 / 16) |
| `internal/engine/oceanbase/` | @JinHai-CN | 2 / 7 | @xugangqiang (2 / 4) |
| `internal/engine/opensearch/` | @xugangqiang | 6 / 14 | — |
| `internal/engine/serenedb/` | @JinHai-CN | 3 / 3 | @xugangqiang (2 / 2) |
| `internal/engine/types/` | @qinling0210 | 4 / 4 | @yuzhichang (3 / 3) |
| `internal/engine/vastbase/` | @xugangqiang | 1 / 3 | — |
| `internal/entity/` | @JinHai-CN | 76 / 1109 | @buua436 (29 / 80) |
| `internal/entity/models/` | @JinHai-CN | 59 / 1027 | @yuzhichang (17 / 163) |
| `internal/handler/` | @JinHai-CN | 129 / 379 | @buua436 (51 / 284) |
| `internal/handler/components_testpkg/` | @yuzhichang | 3 / 3 | @xugangqiang (2 / 2) |
| `internal/harness/` | @JinHai-CN | 13 / 123 | @yingfeng (8 / 460) |
| `internal/harness/core/` | @JinHai-CN | 7 / 61 | @yingfeng (5 / 233) |
| `internal/harness/events/` | @yingfeng | 1 / 8 | @yuzhichang (1 / 1) |
| `internal/harness/graph/` | @JinHai-CN | 8 / 59 | @yingfeng (7 / 194) |
| `internal/harness/metrics/` | @yingfeng | 1 / 4 | — |
| `internal/harness/prebuilt/` | @yingfeng | 2 / 6 | — |
| `internal/harness/replay/` | @yingfeng | 1 / 7 | @JinHai-CN (1 / 1) |
| `internal/ingestion/` | @xugangqiang | 127 / 1235 | @JinHai-CN (54 / 316) |
| `internal/ingestion/chunkcache/` | @xugangqiang | 4 / 9 | — |
| `internal/ingestion/compilation/` | @JinHai-CN | 3 / 5 | @yingfeng (2 / 7) |
| `internal/ingestion/component/` | @xugangqiang | 86 / 685 | @JinHai-CN (31 / 160) |
| `internal/ingestion/internal/` | @xugangqiang | 2 / 4 | — |
| `internal/ingestion/knowledge_compile/` | @buua436 | 22 / 99 | @yuzhichang (10 / 79) |
| `internal/ingestion/mediacache/` | @xugangqiang | 1 / 3 | — |
| `internal/ingestion/pipeline/` | @xugangqiang | 43 / 114 | @yuzhichang (11 / 39) |
| `internal/ingestion/registry/` | @JinHai-CN | 1 / 1 | — |
| `internal/ingestion/service/` | @xugangqiang | 26 / 84 | @buua436 (7 / 19) |
| `internal/ingestion/task/` | @xugangqiang | 51 / 258 | @JinHai-CN (26 / 47) |
| `internal/ingestion/testutil/` | @xugangqiang | 3 / 4 | @JinHai-CN (2 / 2) |
| `internal/ingestion/wire/` | @xugangqiang | 1 / 2 | @yuzhichang (1 / 1) |
| `internal/mcp/` | @6ba3i | 3 / 11 | @xugangqiang (1 / 6) |
| `internal/mcp/client/` | @xugangqiang | 1 / 6 | @wangq8 (1 / 3) |
| `internal/modelcontext/` | @buua436 | 1 / 2 | — |
| `internal/observability/` | @xugangqiang | 5 / 25 | @JinHai-CN (1 / 3) |
| `internal/parser/` | @xugangqiang | 55 / 345 | @JinHai-CN (14 / 93) |
| `internal/parser/charset/` | @xugangqiang | 0 / 0 (inherit parent) | — |
| `internal/parser/chunk/` | @JinHai-CN | 3 / 4 | @xugangqiang (2 / 4) |
| `internal/parser/media/` | @xugangqiang | 1 / 1 | — |
| `internal/parser/parser/` | @xugangqiang | 51 / 331 | @JinHai-CN (11 / 89) |
| `internal/parser/tableutil/` | @xugangqiang | 4 / 9 | — |
| `internal/permission/` | @buua436 | 13 / 60 | @yuzhichang (2 / 5) |
| `internal/permission/response/` | @buua436 | 1 / 2 | @wangq8 (1 / 1) |
| `internal/permission/testsupport/` | @buua436 | 2 / 3 | @yuzhichang (1 / 2) |
| `internal/rag/` | @qinling0210 | 8 / 123 | @yingfeng (6 / 203) |
| `internal/rag/prompts/` | @yingfeng | 5 / 13 | @qinling0210 (3 / 14) |
| `internal/rag/res/` | @JinHai-CN | 1 / 2 | — |
| `internal/router/` | @JinHai-CN | 65 / 71 | @xugangqiang (18 / 19) |
| `internal/server/` | @JinHai-CN | 57 / 110 | @xugangqiang (21 / 69) |
| `internal/server/admin/` | @wangq8 | 3 / 4 | @JinHai-CN (1 / 1) |
| `internal/server/api/` | @xugangqiang | 5 / 5 | @6ba3i (3 / 3) |
| `internal/server/bootstrap/` | @xugangqiang | 5 / 8 | @JinHai-CN (2 / 3) |
| `internal/server/config/` | @xugangqiang | 12 / 29 | @JinHai-CN (10 / 43) |
| `internal/server/deepdoc/` | @xugangqiang | 4 / 5 | — |
| `internal/server/ingestor/` | @xugangqiang | 8 / 11 | @JinHai-CN (2 / 2) |
| `internal/server/local/` | @JinHai-CN | 5 / 6 | @yuzhichang (1 / 2) |
| `internal/server/syncer/` | @JinHai-CN | 1 / 1 | @xugangqiang (1 / 1) |
| `internal/service/` | @JinHai-CN | 146 / 824 | @buua436 (100 / 926) |
| `internal/service/billing/` | @yuzhichang | 1 / 1 | — |
| `internal/service/chunk/` | @JinHai-CN | 22 / 29 | @buua436 (18 / 33) |
| `internal/service/dataset/` | @wangq8 | 25 / 47 | @buua436 (21 / 128) |
| `internal/service/document/` | @buua436 | 23 / 113 | @JinHai-CN (18 / 70) |
| `internal/service/file/` | @JinHai-CN | 14 / 44 | @buua436 (11 / 50) |
| `internal/service/graph/` | @JinHai-CN | 5 / 23 | @buua436 (2 / 3) |
| `internal/service/nav/` | @qinling0210 | 3 / 8 | @yuzhichang (3 / 3) |
| `internal/service/nlp/` | @qinling0210 | 20 / 43 | @JinHai-CN (16 / 35) |
| `internal/service/wikisearch/` | @yuzhichang | 2 / 8 | @JinHai-CN (2 / 5) |
| `internal/service/youcom/` | @JinHai-CN | 0 / 0 (inherit parent) | — |
| `internal/storage/` | @JinHai-CN | 17 / 57 | @xugangqiang (4 / 17) |
| `internal/syncer/` | @JinHai-CN | 7 / 27 | @xugangqiang (3 / 4) |
| `internal/syncer/connector/` | @JinHai-CN | 4 / 23 | @wangq8 (2 / 9) |
| `internal/tokenizer/` | @qinling0210 | 9 / 15 | @JinHai-CN (9 / 13) |
| `internal/utility/` | @JinHai-CN | 32 / 84 | @xugangqiang (12 / 32) |
| `scripts/` | @myf-bee | 1 / 1 | — |
| `sdk/` | @JinHai-CN | 20 / 61 | @yuzhichang (17 / 70) |
| `test/` | @6ba3i | 77 / 547 | @wangq8 (64 / 351) |
| `tools/` | @myf-bee | 0 / 0 (specialist fallback) | — |
| `web/` | @cike8899 | 1476 / 12562 | @dcc123456 (486 / 3752) |
| `web/.storybook/` | @cike8899 | 4 / 6 | @dcc123456 (2 / 2) |
| `web/__mocks__/` | @cike8899 | 3 / 3 | — |
| `web/public/` | @cike8899 | 10 / 10 | @dcc123456 (6 / 7) |
| `web/public/pdfjs-dist/` | @cike8899 | 1 / 1 | — |
| `web/src/` | @cike8899 | 1463 / 12141 | @dcc123456 (480 / 3673) |
| `web/src/assets/` | @cike8899 | 86 / 336 | @dcc123456 (12 / 115) |
| `web/src/components/` | @cike8899 | 579 / 1534 | @dcc123456 (231 / 588) |
| `web/src/constants/` | @cike8899 | 84 / 91 | @dcc123456 (22 / 22) |
| `web/src/ee-stub/` | @dcc123456 | 1 / 66 | — |
| `web/src/hooks/` | @cike8899 | 379 / 619 | @dcc123456 (80 / 129) |
| `web/src/icons/` | @cike8899 | 2 / 10 | — |
| `web/src/interfaces/` | @cike8899 | 162 / 202 | @dcc123456 (39 / 56) |
| `web/src/layouts/` | @cike8899 | 78 / 131 | @dcc123456 (13 / 27) |
| `web/src/less/` | @cike8899 | 14 / 15 | — |
| `web/src/lib/` | @cike8899 | 10 / 37 | @dcc123456 (1 / 1) |
| `web/src/locales/` | @cike8899 | 357 / 833 | @dcc123456 (142 / 302) |
| `web/src/pages/` | @cike8899 | 1276 / 7780 | @dcc123456 (361 / 2083) |
| `web/src/services/` | @cike8899 | 92 / 113 | @dcc123456 (32 / 41) |
| `web/src/stories/` | @cike8899 | 8 / 52 | @dcc123456 (1 / 2) |
| `web/src/theme/` | @cike8899 | 0 / 0 (inherit parent) | — |
| `web/src/utils/` | @cike8899 | 191 / 272 | @dcc123456 (51 / 69) |
| `web/src/wrappers/` | @cike8899 | 5 / 5 | @dcc123456 (2 / 2) |
