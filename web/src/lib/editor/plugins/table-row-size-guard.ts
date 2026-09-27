/**
 * Pure guard for table-row size. Kept in its own file so unit tests can
 * import the threshold and the check without pulling in TableTransformer's
 * markdown-import dependency chain (which transitively imports
 * `@lexical/extension` and other modules that fail to resolve under the
 * current `@lexical/*` version pinning - a pre-existing condition flagged
 * by `npx tsc --noEmit` and unrelated to this fix).
 *
 * Background (issue #321): @aawbeck reported a crash-on-load loop. The
 * TableTransformer's `$createTableCell` path recursively calls
 * `$convertFromEnhancedMarkdownString` for each cell on the main thread,
 * inside a single Lexical `editor.update()` transaction. With many large
 * cells the cumulative synchronous Lexical node allocation exhausts the
 * V8 heap and trips a TurboFan integrity-level assertion (SIGTRAP).
 * The next launch re-opens the same file (the `restoreTabs: true`
 * preference) and the loop repeats until the user manually deletes the
 * file.
 *
 * Concrete repro shape: 83 rows of 5,451 chars each (5 cells of ~1,090
 * chars each). The guard below sits above legitimate wide-table sizes
 * (a 10-column table with 400-char cells is ~4,400 chars including
 * pipes and spaces) and below the repro, so it fires on pathological
 * input without affecting normal use. Oversized rows fall through to
 * plain markdown text rendering rather than table parsing - no crash,
 * file still opens, content visible as piped text.
 *
 * Limitation: the guard is per-row only. A table built from many
 * narrow-but-numerous rows can still exhaust heap in aggregate. The
 * residual class is not addressed here; the right path for that is
 * either raising the per-row threshold (no help against narrow-row
 * pile-ups) or moving the recursive cell parse off the main thread.
 * Both are out of scope for this fix; this PR closes the reported
 * crash and unblocks the file-open path.
 */

export const MAX_TABLE_ROW_BYTES = 5_000;

/**
 * Returns true when the given table-row text content exceeds the
 * per-row size threshold and should be skipped by the table
 * transformer. The caller falls back to rendering the row as plain
 * markdown text.
 */
export function isTableRowOversized(textContent: string): boolean {
  return textContent.length > MAX_TABLE_ROW_BYTES;
}
