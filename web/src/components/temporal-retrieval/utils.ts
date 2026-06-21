export function resolveEffectiveDatasetIds(
  datasetIds: string[] | undefined,
  legacyKbIds: string[] | undefined,
): string[] {
  if (Array.isArray(datasetIds) && datasetIds.length > 0) {
    return datasetIds;
  }
  return Array.isArray(legacyKbIds) ? legacyKbIds : [];
}
