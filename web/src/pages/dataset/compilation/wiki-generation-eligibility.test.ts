import { IDataset } from '@/interfaces/database/dataset';

import { canGenerateWiki } from './wiki-generation-eligibility';

function dataset(overrides: Partial<IDataset> = {}): IDataset {
  return {
    chunk_count: 1,
    pipeline_id: '',
    parser_config: {},
    ...overrides,
  } as IDataset;
}

describe('canGenerateWiki', () => {
  it('requires parsed chunks', () => {
    expect(
      canGenerateWiki(dataset({ chunk_count: 0, pipeline_id: 'pipeline' })),
    ).toBe(false);
  });

  it('requires an ingestion pipeline', () => {
    expect(canGenerateWiki(dataset())).toBe(false);
  });

  it('allows pipeline-backed datasets', () => {
    expect(canGenerateWiki(dataset({ pipeline_id: 'pipeline-id' }))).toBe(true);
  });

  it('ignores a blank pipeline id', () => {
    expect(canGenerateWiki(dataset({ pipeline_id: '   ' }))).toBe(false);
  });
});
