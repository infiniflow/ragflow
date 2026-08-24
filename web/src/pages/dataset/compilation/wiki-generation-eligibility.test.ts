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

function parserConfig(
  overrides: Record<string, unknown>,
): IDataset['parser_config'] {
  return overrides as unknown as IDataset['parser_config'];
}

describe('canGenerateWiki', () => {
  it('requires parsed chunks', () => {
    expect(canGenerateWiki(dataset({ chunk_count: 0, pipeline_id: 'pipeline' }))).toBe(
      false,
    );
  });

  it('blocks generation when no compilation configuration is present', () => {
    expect(canGenerateWiki(dataset())).toBe(false);
  });

  it('allows pipeline-backed datasets', () => {
    expect(canGenerateWiki(dataset({ pipeline_id: 'pipeline-id' }))).toBe(true);
  });

  it('allows the singular parser-config template group field', () => {
    expect(
      canGenerateWiki(
        dataset({
          parser_config: parserConfig({
            compilation_template_group_id: ['group-id'],
          }),
        }),
      ),
    ).toBe(true);
  });

  it('allows the plural parser-config template group field', () => {
    expect(
      canGenerateWiki(
        dataset({
          parser_config: parserConfig({
            compilation_template_group_ids: ['group-id'],
          }),
        }),
      ),
    ).toBe(true);
  });

  it('checks plural and singular template group fields independently', () => {
    expect(
      canGenerateWiki(
        dataset({
          parser_config: parserConfig({
            compilation_template_group_ids: [],
            compilation_template_group_id: ['group-id'],
          }),
        }),
      ),
    ).toBe(true);
  });

  it('ignores blank pipeline and template group values', () => {
    expect(
      canGenerateWiki(
        dataset({
          pipeline_id: '   ',
          parser_config: parserConfig({
            compilation_template_group_id: [' ', ''],
          }),
        }),
      ),
    ).toBe(false);
  });
});
