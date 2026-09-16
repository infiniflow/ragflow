import {
  buildOperatorNode,
  getOperatorType,
  transformApiConfigToForm,
  transformFormConfigToApi,
} from '@/utils/pipeline-operator';

let mockIsGoBackend = true;
jest.mock('@/utils/backend-runtime', () => ({
  getBackendLanguage: () => (mockIsGoBackend ? 'go' : 'python'),
}));

const extractorNode = {
  id: 'Extractor:AutoExtractDefault',
  data: { form: {} },
} as any;

describe('GeneralChunker operator bridge', () => {
  it('recognizes and transforms the built-in general chunker config', () => {
    expect(getOperatorType('GeneralChunker:SixApplesFall')).toBe(
      'GeneralChunker',
    );

    const form = transformApiConfigToForm('GeneralChunker', {
      chunk_token_size: 512,
      delimiters: ['\n', ';'],
      overlapped_percent: 0.1,
      table_context_size: 2,
      image_context_size: 3,
    });
    expect(form.delimiters).toEqual([{ value: '\n' }, { value: ';' }]);
    expect(form.table_context_size).toBe(2);
    expect(form.image_context_size).toBe(3);
    expect(form).not.toHaveProperty('image_table_context_window');
    expect(form).not.toHaveProperty('delimiter_mode');

    const api = transformFormConfigToApi('GeneralChunker', {
      delimiter_mode: 'delimiter',
      chunk_token_size: 512,
      delimiters: [{ value: '\n' }, { value: ';' }],
      children_delimiters: [],
      enable_children: false,
      overlapped_percent: 10,
      table_context_size: 2,
      image_context_size: 3,
    });
    expect(api.delimiters).toEqual(['\n', ';']);
    expect(api.table_context_size).toBe(2);
    expect(api.image_context_size).toBe(3);
    expect(api).not.toHaveProperty('delimiter_mode');
  });
});

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

// A minimal DSL-shaped Parser node where the component's spreadsheet config
// carries column_mode:"auto" — the template default that was previously
// masking the user's upload-time "manual" selection.
const parserNodeWithAutoColumnMode = {
  id: 'Parser:HipSignsRhyme',
  data: {
    form: {
      setups: [
        {
          fileFormat: 'spreadsheet',
          column_mode: 'auto', // ← DSL template default (always "auto")
          column_names: [],
          column_roles: {},
          parse_method: 'DeepDOC',
          output_format: 'json',
        },
      ],
    },
  },
} as any;

describe('buildOperatorNode spreadsheet column_mode priority', () => {
  beforeEach(() => {
    mockIsGoBackend = true;
  });

  // Regression test: before the fix, the DSL component's column_mode:"auto" was
  // evaluated first (truthy), causing root-level table_column_mode:"manual" to
  // be silently ignored and the dialog to always display "auto".
  it('root-level table_column_mode wins over DSL component column_mode:"auto"', () => {
    const node = buildOperatorNode(parserNodeWithAutoColumnMode, {
      'Parser:HipSignsRhyme': {
        spreadsheet: {
          column_mode: 'auto',
          column_names: [],
          column_roles: {},
        },
      },
      table_column_mode: 'manual',
      table_column_names: ['col_a', 'col_b'],
      table_column_roles: { col_a: 'indexing', col_b: 'both' },
    });

    const form = (node.data as Record<string, any>).form;
    const spreadsheetSetup = form.setups?.find(
      (s: any) => s.fileFormat === 'spreadsheet',
    );
    expect(spreadsheetSetup).toBeDefined();
    // Root-level "manual" must win over component-level "auto".
    expect(spreadsheetSetup.column_mode).toBe('manual');
    expect(spreadsheetSetup.column_names).toEqual(['col_a', 'col_b']);
    expect(spreadsheetSetup.column_roles).toEqual({
      col_a: 'indexing',
      col_b: 'both',
    });
  });

  it('component-level column_mode wins when root level is absent', () => {
    const node = buildOperatorNode(parserNodeWithAutoColumnMode, {
      'Parser:HipSignsRhyme': {
        spreadsheet: {
          column_mode: 'manual',
          column_names: ['x'],
          column_roles: { x: 'both' },
        },
      },
      // no root-level table_column_mode
    });

    const form = (node.data as Record<string, any>).form;
    const spreadsheetSetup = form.setups?.find(
      (s: any) => s.fileFormat === 'spreadsheet',
    );
    expect(spreadsheetSetup.column_mode).toBe('manual');
    expect(spreadsheetSetup.column_names).toEqual(['x']);
  });

  it('falls back to "auto" when no column_mode is present anywhere', () => {
    const node = buildOperatorNode(
      {
        id: 'Parser:HipSignsRhyme',
        data: {
          form: {
            setups: [
              {
                fileFormat: 'spreadsheet',
                // no column_mode at all
                column_names: [],
                column_roles: {},
              },
            ],
          },
        },
      } as any,
      {
        'Parser:HipSignsRhyme': {
          spreadsheet: { column_names: [], column_roles: {} },
        },
        // no table_column_mode
      },
    );

    const form = (node.data as Record<string, any>).form;
    const spreadsheetSetup = form.setups?.find(
      (s: any) => s.fileFormat === 'spreadsheet',
    );
    expect(spreadsheetSetup.column_mode).toBe('auto');
  });

  it('root-level table_column_names wins over component-level column_names', () => {
    const node = buildOperatorNode(parserNodeWithAutoColumnMode, {
      'Parser:HipSignsRhyme': {
        spreadsheet: {
          column_mode: 'manual',
          column_names: ['stale_col'],
          column_roles: {},
        },
      },
      table_column_mode: 'manual',
      table_column_names: ['fresh_col_a', 'fresh_col_b'],
    });

    const form = (node.data as Record<string, any>).form;
    const spreadsheetSetup = form.setups?.find(
      (s: any) => s.fileFormat === 'spreadsheet',
    );
    expect(spreadsheetSetup.column_names).toEqual([
      'fresh_col_a',
      'fresh_col_b',
    ]);
  });
});
