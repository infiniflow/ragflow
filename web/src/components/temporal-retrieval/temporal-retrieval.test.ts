import { resolveEffectiveDatasetIds } from './utils';

describe('resolveEffectiveDatasetIds', () => {
  it('prefers non-empty datasetIds', () => {
    expect(resolveEffectiveDatasetIds(['ds-1'], ['kb-1'])).toEqual(['ds-1']);
  });

  it('falls back to legacy kb_ids when datasetIds is empty', () => {
    expect(resolveEffectiveDatasetIds([], ['kb-1', 'kb-2'])).toEqual(['kb-1', 'kb-2']);
  });

  it('falls back to legacy kb_ids when datasetIds is undefined', () => {
    expect(resolveEffectiveDatasetIds(undefined, ['kb-1'])).toEqual(['kb-1']);
  });

  it('returns empty array when both inputs are empty', () => {
    expect(resolveEffectiveDatasetIds([], [])).toEqual([]);
  });
});
