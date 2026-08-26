# Feishu Wiki Data Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a RAGFlow-native Feishu Wiki data source with manual and scheduled acquisition, pre-download screening, incremental deduplication, and normal RAGFlow parsing/chunking.

**Architecture:** A focused Python connector owns Feishu authentication, recursive Wiki traversal, filtering, and download. The existing connector scheduler and document sink own manual rebuilds, automatic polling, stable-document upserts, metadata persistence, parsing, and sync logs. The web data-source form exposes credentials and screening rules without introducing new database tables.

**Tech Stack:** Python 3.13, requests, Pydantic document models, Quart connector API, React/TypeScript dynamic forms, Jest/pytest.

**Spec:** `workflows/feishu-wiki-ingestion.md`

**Implementation note (2026-08-26):** The repository-wide web type-check is not a clean baseline and reports many unrelated existing errors. The Feishu Wiki form is therefore verified by its focused Jest contract, focused oxlint, and oxfmt checks. Live Feishu-to-RAGFlow verification remains a deployment step because it requires the operator to enter the App Secret directly in RAGFlow.

## Global Constraints

- Do not call a Feishu or RAGFlow delete endpoint.
- Support downloadable Feishu Wiki `file` nodes only in the first release.
- Traverse non-file folder/native-document nodes so nested files remain discoverable.
- Filter nodes before downloading their file bodies.
- Use `(space_id, node_token)` as stable source identity, store the full
  SHA-256 in metadata, and use its first 32 hex characters as the RAGFlow
  `content_hash` fingerprint to fit the existing database column.
- Never log App Secret, tenant access token, or authorization headers.
- Preserve all unrelated changes in `/Users/edy/Desktop/ragflow`.

---

### Task 1: Feishu Wiki connector behavior

**Files:**
- Create: `common/data_source/feishu_wiki_connector.py`
- Create: `test/unit_test/data_source/test_feishu_wiki_connector.py`

**Interfaces:**
- Produces: `FeishuWikiConnector.build_connector(config) -> FeishuWikiConnector`
- Produces: `validate_connector_settings() -> None`
- Produces: `load_from_state() -> Iterator[list[Document]]`
- Produces: `poll_source(start: float, end: float) -> Iterator[list[Document]]`

- [ ] **Step 1: Write failing configuration and filtering tests**

```python
def test_build_connector_requires_app_credentials():
    with pytest.raises(ConnectorMissingCredentialError):
        FeishuWikiConnector.build_connector({"space_id": "s", "root_node_token": "n"})

def test_screening_rejects_before_download(fake_client):
    connector = make_connector(fake_client, include_extensions=["pdf"], exclude_keywords=["obsolete"])
    docs = list(connector.load_from_state())
    assert [doc.semantic_identifier for batch in docs for doc in batch] == ["WI-001.pdf"]
    assert fake_client.downloaded_tokens == ["approved-file-token"]
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `python -m pytest test/unit_test/data_source/test_feishu_wiki_connector.py -q`

Expected: FAIL because `common.data_source.feishu_wiki_connector` does not exist.

- [ ] **Step 3: Implement configuration, authentication, traversal, screening, and normalization**

```python
class FeishuWikiConnector(LoadConnector, PollConnector):
    @classmethod
    def build_connector(cls, config: dict[str, Any]) -> "FeishuWikiConnector":
        credentials = config.get("credentials") or {}
        connector = cls(
            space_id=str(config.get("space_id") or "").strip(),
            root_node_token=str(config.get("root_node_token") or "").strip(),
            include_extensions=config.get("include_extensions") or [],
            include_keywords=config.get("include_keywords") or [],
            exclude_keywords=config.get("exclude_keywords") or [],
            batch_size=int(config.get("batch_size") or INDEX_BATCH_SIZE),
        )
        connector.load_credentials(credentials)
        return connector

    def validate_connector_settings(self) -> None:
        self._ensure_configured()
        self._get_access_token()
        next(self._iter_child_pages(self.root_node_token), [])

    def load_from_state(self) -> GenerateDocumentsOutput:
        yield from self._yield_documents(start=None, end=None)

    def poll_source(
        self,
        start: SecondsSinceUnixEpoch,
        end: SecondsSinceUnixEpoch,
    ) -> GenerateDocumentsOutput:
        yield from self._yield_documents(start=start, end=end)

    def _matches_filters(self, title: str) -> bool:
        normalized = title.casefold()
        extension = get_file_ext(title).lstrip(".").casefold()
        if self.include_extensions and extension not in self.include_extensions:
            return False
        if self.include_keywords and not any(word in normalized for word in self.include_keywords):
            return False
        return not any(word in normalized for word in self.exclude_keywords)

    def _download_file(self, object_token: str) -> bytes:
        response = self._request(
            "GET",
            f"/open-apis/drive/v1/files/{quote(object_token, safe='')}/download",
        )
        return response.content
```

- [ ] **Step 4: Add and run pagination, incremental-window, metadata, fingerprint, and API-error tests**

Run: `python -m pytest test/unit_test/data_source/test_feishu_wiki_connector.py -q`

Expected: PASS.

- [ ] **Step 5: Commit connector behavior**

```bash
git add common/data_source/feishu_wiki_connector.py test/unit_test/data_source/test_feishu_wiki_connector.py
git commit -m "feat(data-source): add Feishu Wiki connector"
```

### Task 2: Register the source with RAGFlow synchronization

**Files:**
- Modify: `common/constants.py`
- Modify: `common/data_source/config.py`
- Modify: `common/data_source/__init__.py`
- Create: `rag/svr/feishu_wiki_sync.py`
- Modify: `rag/svr/sync_data_source.py`
- Create: `test/unit_test/rag/test_feishu_wiki_sync_adapter.py`

**Interfaces:**
- Consumes: `FeishuWikiConnector`
- Produces: `FileSource.FEISHU_WIKI == "feishu_wiki"`
- Produces: `DocumentSource.FEISHU_WIKI == "feishu_wiki"`
- Produces: `build_feishu_wiki_generator(conf, task, window_end=None)`
- Produces: `FeishuWiki` sync adapter and `func_factory` registration

- [ ] **Step 1: Write a failing source-registration test**

```python
def test_incremental_sync_uses_the_previous_window(fake_connector):
    connector, batches = build_feishu_wiki_generator(
        {"space_id": "space"},
        {"reindex": "0", "poll_range_start": datetime(2026, 1, 1, tzinfo=timezone.utc)},
        window_end=datetime(2026, 1, 2, tzinfo=timezone.utc),
    )
    assert connector is fake_connector
    assert list(batches) == [["incremental"]]
    assert fake_connector.poll_calls == [(1767225600.0, 1767312000.0)]
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `python -m pytest test/unit_test/rag/test_feishu_wiki_sync_adapter.py -q`

Expected: FAIL because `rag.svr.feishu_wiki_sync` is absent.

- [ ] **Step 3: Add enum, builder, and sync-adapter registration**

```python
class FeishuWiki(SyncBase):
    SOURCE_NAME: str = FileSource.FEISHU_WIKI

    async def _generate(self, task: dict):
        self.connector, batches = build_feishu_wiki_generator(self.conf, task)
        return batches
```

- [ ] **Step 4: Run registration and connector tests**

Run: `python -m pytest test/unit_test/data_source/test_feishu_wiki_connector.py test/unit_test/rag/test_feishu_wiki_sync_adapter.py -q`

Expected: PASS.

- [ ] **Step 5: Commit synchronization integration**

```bash
git add common/constants.py common/data_source/config.py common/data_source/__init__.py rag/svr/feishu_wiki_sync.py rag/svr/sync_data_source.py test/unit_test/rag/test_feishu_wiki_sync_adapter.py
git commit -m "feat(sync): register Feishu Wiki data source"
```

### Task 3: Expose Feishu Wiki configuration in the web UI

**Files:**
- Modify: `web/src/pages/user-setting/data-source/constant/index.tsx`
- Modify: `web/src/locales/zh.ts`
- Modify: `web/src/locales/en.ts`
- Create: `web/src/pages/user-setting/data-source/constant/__tests__/feishu-wiki.test.tsx`

**Interfaces:**
- Produces: `DataSourceKey.FEISHU_WIKI == "feishu_wiki"`
- Produces form fields `config.credentials.app_id`, `config.credentials.app_secret`, `config.space_id`, `config.root_node_token`, `config.include_extensions`, `config.include_keywords`, `config.exclude_keywords`, and `config.batch_size`

- [ ] **Step 1: Write a failing form-contract test**

```typescript
it('builds the Feishu Wiki credential and screening fields', () => {
  const fields = getDataSourceFieldsWithExtras(t, DataSourceKey.FEISHU_WIKI);
  expect(fields.map((field) => field.name)).toEqual(
    expect.arrayContaining([
      'config.credentials.app_id',
      'config.credentials.app_secret',
      'config.space_id',
      'config.root_node_token',
      'config.include_extensions',
      'config.include_keywords',
      'config.exclude_keywords',
    ]),
  );
});
```

- [ ] **Step 2: Run the focused Jest test and verify RED**

Run: `pnpm --dir web --lockfile=false test -- --runInBand src/pages/user-setting/data-source/constant/__tests__/feishu-wiki.test.tsx`

Expected: FAIL because `DataSourceKey.FEISHU_WIKI` is absent.

- [ ] **Step 3: Add the source card, dynamic form, defaults, and translations**

```typescript
[DataSourceKey.FEISHU_WIKI]: {
  name: 'Feishu Wiki',
  description: t('setting.feishu_wikiDescription'),
  icon: <BookOpen size={22} />,
}
```

The password field uses `FormFieldType.Password`; keyword and extension fields
use `FormFieldType.Tag`; defaults are empty lists and `batch_size: 2`.

- [ ] **Step 4: Run focused Jest, type-check, and lint**

Run: `pnpm --dir web --lockfile=false test -- --runInBand src/pages/user-setting/data-source/constant/__tests__/feishu-wiki.test.tsx`

Run: `pnpm --dir web --lockfile=false type-check`

Run: `pnpm --dir web --lockfile=false lint`

Expected: all commands succeed.

- [ ] **Step 5: Commit the web configuration**

```bash
git add web/src/pages/user-setting/data-source/constant/index.tsx web/src/pages/user-setting/data-source/constant/__tests__/feishu-wiki.test.tsx web/src/locales/zh.ts web/src/locales/en.ts
git commit -m "feat(web): configure Feishu Wiki data source"
```

### Task 4: Document, verify, and prepare local deployment

**Files:**
- Modify: `docs/guides/data_source/data_source_categories_and_selection.md`
- Modify: `docs/guides/data_source/data_source_configuration.md`
- Modify: `workflows/feishu-wiki-ingestion.md`

**Interfaces:**
- Consumes: the registered source and form contract from Tasks 1–3
- Produces: operator instructions for test, link, manual rebuild, scheduled sync, and chunk verification

- [ ] **Step 1: Add operator documentation with exact UI path and permission requirements**

```markdown
1. Open User settings > Data source > Feishu Wiki.
2. Enter App ID, App Secret, Wiki space ID, and root node token.
3. Configure screening and test the connection.
4. Link the source from the target dataset and keep Auto parse enabled.
5. Use Rebuild for a manual scan or Resume for scheduled scans.
```

- [ ] **Step 2: Run backend regression tests**

Run: `python -m pytest test/unit_test/data_source/test_feishu_wiki_connector.py test/unit_test/rag/test_sync_data_source.py test/testcases/restful_api/test_connector_routes_unit.py -q`

Expected: PASS.

- [ ] **Step 3: Run web verification**

Run: `pnpm --dir web --lockfile=false test -- --runInBand src/pages/user-setting/data-source/constant/__tests__/feishu-wiki.test.tsx`

Run: `pnpm --dir web --lockfile=false type-check && pnpm --dir web --lockfile=false lint`

Expected: PASS.

- [ ] **Step 4: Inspect the diff for secrets and deletion behavior**

Run: `git diff --check && ! rg -n "app_secret\s*[:=]\s*['\"][^'\"]+|tenant_access_token\s*[:=]\s*['\"][^'\"]+" common web workflows docs`

Expected: no whitespace errors, embedded secrets, Feishu delete calls, or RAGFlow delete calls.

- [ ] **Step 5: Commit documentation and verification updates**

```bash
git add docs/guides/data_source/data_source_categories_and_selection.md docs/guides/data_source/data_source_configuration.md workflows/feishu-wiki-ingestion.md
git commit -m "docs: add Feishu Wiki ingestion workflow"
```
