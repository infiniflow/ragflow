import { buildOperatorNode } from '@/utils/pipeline-operator';

let mockIsGoBackend = true;
jest.mock('@/utils/backend-runtime', () => ({
  getBackendLanguage: () => (mockIsGoBackend ? 'go' : 'python'),
}));

const extractorNode = {
  id: 'Extractor:AutoExtractDefault',
  data: { form: {} },
} as any;

describe('buildOperatorNode dataset-level metadata precedence', () => {
  beforeEach(() => {
    mockIsGoBackend = true;
  });

  it('seeds the extractor metadata toggle from the dataset-level object', () => {
    const node = buildOperatorNode(extractorNode, {
      'Extractor:AutoExtractDefault': {
        llm_id: 'llm-a',
        metadata: { enabled: false, metadata: [], built_in_metadata: [] },
      },
      metadata: {
        enabled: true,
        metadata: [],
        built_in_metadata: [{ key: 'update_time', type: 'time' }],
      },
    });

    const form = (node.data as Record<string, any>).form;
    expect(form.metadata.enabled).toBe(true);
    expect(form.metadata.built_in_metadata).toEqual([
      { key: 'update_time', type: 'time' },
    ]);
  });

  it('falls back to the per-node metadata group without a dataset-level object', () => {
    const node = buildOperatorNode(extractorNode, {
      'Extractor:AutoExtractDefault': {
        llm_id: 'llm-a',
        metadata: {
          enabled: true,
          metadata: [],
          built_in_metadata: [{ key: 'file_name', type: 'string' }],
        },
      },
    });

    const form = (node.data as Record<string, any>).form;
    expect(form.metadata.enabled).toBe(true);
    expect(form.metadata.built_in_metadata).toEqual([
      { key: 'file_name', type: 'string' },
    ]);
  });

  it('ignores a flat metadata array at the top level', () => {
    const node = buildOperatorNode(extractorNode, {
      'Extractor:AutoExtractDefault': {
        llm_id: 'llm-a',
        metadata: { enabled: false, metadata: [], built_in_metadata: [] },
      },
      metadata: [{ key: 'category', type: 'string' }],
    });

    const form = (node.data as Record<string, any>).form;
    expect(form.metadata.enabled).toBe(false);
  });

  it('ignores an object without the metadata group shape', () => {
    const node = buildOperatorNode(extractorNode, {
      'Extractor:AutoExtractDefault': {
        llm_id: 'llm-a',
        metadata: { enabled: false, metadata: [], built_in_metadata: [] },
      },
      metadata: { enabled: 'yes', metadata: [], built_in_metadata: [] },
    });

    const form = (node.data as Record<string, any>).form;
    expect(form.metadata.enabled).toBe(false);
  });

  it('does not touch non-extractor operators', () => {
    const node = buildOperatorNode(
      {
        id: 'Tokenizer:SomeNode',
        data: { form: {} },
      } as any,
      {
        'Tokenizer:SomeNode': { fields: 'text' },
        metadata: {
          enabled: true,
          metadata: [],
          built_in_metadata: [],
        },
      },
    );

    const form = (node.data as Record<string, any>).form;
    expect(form.fields).toBe('text');
    expect(form).not.toHaveProperty('metadata');
  });
});
