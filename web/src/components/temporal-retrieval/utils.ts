/**
 * Resolve dataset identifiers for temporal profile requests.
 *
 * Newer records use `dataset_ids`, while older chat/search records may still
 * carry `kb_ids`. An empty `dataset_ids` array should not block the legacy
 * fallback because empty arrays are truthy in JavaScript.
 */
export function resolveEffectiveDatasetIds(
  datasetIds: string[] | undefined,
  legacyKbIds: string[] | undefined,
): string[] {
  if (Array.isArray(datasetIds) && datasetIds.length > 0) {
    return datasetIds;
  }
  return Array.isArray(legacyKbIds) ? legacyKbIds : [];
}
