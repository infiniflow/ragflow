## 2024-05-22 - Anti-pattern: Manual Offset Pagination for Bulk Fetch
**Learning:** The codebase contained methods that manually paginated results using `offset` and `limit` inside a loop to fetch *all* records. This results in O(N/batch_size) queries and increasing database load as the offset grows (O(N^2) complexity for scanning).
**Action:** Replace manual offset loops with a single query iterator when fetching all records is intended and the result set size is manageable (e.g., fetching IDs). Peewee's `select()` returns an iterator that efficiently streams results.
