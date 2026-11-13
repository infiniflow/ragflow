import {
  buildOperatorNode,
  getOperatorType,
  transformApiConfigToForm,
  transformFormConfigToApi,
} from '@/utils/pipeline-operator';

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

  it('keeps an empty delimiter list empty (no legacy re-seed)', () => {
    // The general chunker never had the 'token_size' tab, so an empty saved
    // list is always a deliberate choice (pure token-size chunking).
    const form = transformApiConfigToForm('GeneralChunker', {
      chunk_token_size: 512,
      delimiters: [],
      overlapped_percent: 0,
    });
    expect(form.delimiters).toEqual([]);
  });
});

describe('TokenChunker delimiter seeding on load', () => {
  it('keeps an empty list saved by the current form empty', () => {
    const form = transformApiConfigToForm('TokenChunker', {
      delimiter_mode: 'delimiter',
      chunk_token_size: 512,
      delimiters: [],
      overlapped_percent: 0,
    });
    expect(form.delimiters).toEqual([]);
    expect(form.delimiter_mode).toBe('delimiter');
  });

  it('keeps an empty list in one mode empty', () => {
    const form = transformApiConfigToForm('TokenChunker', {
      delimiter_mode: 'one',
      chunk_token_size: 512,
      delimiters: [],
      overlapped_percent: 0,
    });
    expect(form.delimiters).toEqual([]);
    expect(form.delimiter_mode).toBe('one');
  });

  it('re-seeds the default row for legacy token_size nodes', () => {
    // Nodes saved under the removed 'token_size' tab persisted an empty list;
    // seed the default '\n' row so the merged 'delimiter' tab is not blank.
    const form = transformApiConfigToForm('TokenChunker', {
      delimiter_mode: 'token_size',
      chunk_token_size: 512,
      delimiters: [],
      overlapped_percent: 0,
    });
    expect(form.delimiters).toEqual([{ value: '\n' }]);
    expect(form.delimiter_mode).toBe('delimiter');
  });

  it('re-seeds the default row when the mode is absent (older DSLs)', () => {
    const form = transformApiConfigToForm('TokenChunker', {
      chunk_token_size: 512,
      delimiters: [],
      overlapped_percent: 0,
    });
    expect(form.delimiters).toEqual([{ value: '\n' }]);
    expect(form.delimiter_mode).toBe('delimiter');
  });

  it('derives enable_children from a non-empty children list', () => {
    // Configs saved before the enable_children toggle existed carry children
    // delimiters without the flag.
    const form = transformApiConfigToForm('TokenChunker', {
      delimiter_mode: 'delimiter',
      chunk_token_size: 512,
      delimiters: ['\n'],
      children_delimiters: ['|'],
      overlapped_percent: 0,
    });
    expect(form.enable_children).toBe(true);
    expect(form.children_delimiters).toEqual([{ value: '|' }]);
  });
});

describe('Parser nested setups compatibility', () => {
  it('lifts a legacy nested "setups" object into file families', () => {
    const form = transformApiConfigToForm('Parser', {
      setups: {
        pdf: { parse_method: 'vision', pages: [[1, 3]] },
      },
    });
    expect(form.setups).toHaveLength(1);
    expect(form.setups[0].fileFormat).toBe('pdf');
    expect(form.setups[0].parse_method).toBe('vision');
    expect(form.setups[0].pages).toEqual([{ from: 1, to: 3 }]);
  });

  it('keeps flat file families unchanged', () => {
    const form = transformApiConfigToForm('Parser', {
      pdf: { parse_method: 'deepdoc' },
    });
    expect(form.setups).toHaveLength(1);
    expect(form.setups[0].fileFormat).toBe('pdf');
    expect(form.setups[0].parse_method).toBe('deepdoc');
  });

  it('skips non-family keys such as outputs', () => {
    const form = transformApiConfigToForm('Parser', {
      outputs: { html: { type: 'string' } },
      pdf: { parse_method: 'deepdoc' },
    });
    const families = form.setups.map((s: any) => s.fileFormat);
    expect(families).toEqual(['pdf']);
  });

  it('does not surface "setups" as a file family in buildOperatorNode', () => {
    const node = buildOperatorNode(
      {
        id: 'Parser:HipSignsRhyme',
        data: {
          form: { setups: [{ fileFormat: 'pdf', parse_method: 'naive' }] },
        },
      } as any,
      {
        'Parser:HipSignsRhyme': {
          setups: { pdf: { parse_method: 'vision' } },
        },
      },
    );

    const form = (node.data as Record<string, any>).form;
    const families = form.setups.map((s: any) => s.fileFormat);
    expect(families).not.toContain('setups');
    const pdf = form.setups.find((s: any) => s.fileFormat === 'pdf');
    expect(pdf?.parse_method).toBe('vision');
  });
});

describe('buildOperatorNode dataset-level metadata precedence', () => {
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

describe('TitleChunker per-method config round trip', () => {
  it('keeps hierarchy and group rules independent across load and save', () => {
    const form = transformApiConfigToForm('TitleChunker', {
      method: 'hierarchy',
      hierarchy: 3,
      levels: [['^#[^#]']],
      chunk_token_cap: 1024,
      hierarchyHierarchy: '3',
      hierarchyGroup: '0',
      hierarchyRules: [{ levels: [{ expression: '^#[^#]' }] }],
      groupRules: [{ levels: [{ expression: '第[0-9]+章' }] }],
    });
    expect(form.hierarchyRules).toEqual([
      { levels: [{ expression: '^#[^#]' }] },
    ]);
    expect(form.groupRules).toEqual([
      { levels: [{ expression: '第[0-9]+章' }] },
    ]);
    expect(form.hierarchyHierarchy).toBe('3');
    expect(form.hierarchyGroup).toBe('0');
    // The shared backend-facing cap seeds both per-method fields on load.
    expect(form.hierarchyChunkTokenCap).toBe(1024);
    expect(form.groupChunkTokenCap).toBe(1024);
    expect(form).not.toHaveProperty('levels');
    expect(form).not.toHaveProperty('hierarchy');
    expect(form).not.toHaveProperty('chunk_token_cap');

    // Switch to group mode, edit only the group rules and the group cap, and
    // save: the shared levels/hierarchy/chunk_token_cap keys carry the active
    // method's values while the hierarchy config survives untouched under its
    // own keys.
    const api = transformFormConfigToApi('TitleChunker', {
      ...form,
      method: 'group',
      hierarchyGroup: '2',
      groupRules: [
        { levels: [{ expression: '第[0-9]+章' }] },
        { levels: [{ expression: '第[0-9]+节' }] },
      ],
      groupChunkTokenCap: 2048,
    });
    expect(api.method).toBe('group');
    expect(api.levels).toEqual([['第[0-9]+章'], ['第[0-9]+节']]);
    expect(api.hierarchy).toBe(2);
    expect(api.chunk_token_cap).toBe(2048);
    expect(api.hierarchyRules).toEqual([
      { levels: [{ expression: '^#[^#]' }] },
    ]);
    expect(api.groupRules).toEqual([
      { levels: [{ expression: '第[0-9]+章' }] },
      { levels: [{ expression: '第[0-9]+节' }] },
    ]);
    expect(api.hierarchyHierarchy).toBe('3');
    expect(api.hierarchyGroup).toBe('2');
    expect(api.hierarchyChunkTokenCap).toBe(1024);
    expect(api.groupChunkTokenCap).toBe(2048);
    expect(api).not.toHaveProperty('rules');
  });

  it('resolves the shared chunk_token_cap from the active method on save', () => {
    const base = {
      hierarchyHierarchy: '3',
      hierarchyGroup: '0',
      hierarchyRules: [{ levels: [{ expression: '^#[^#]' }] }],
      groupRules: [{ levels: [{ expression: '第[0-9]+章' }] }],
      hierarchyChunkTokenCap: 1024,
      groupChunkTokenCap: 2048,
    };
    expect(
      transformFormConfigToApi('TitleChunker', {
        ...base,
        method: 'hierarchy',
      }).chunk_token_cap,
    ).toBe(1024);
    expect(
      transformFormConfigToApi('TitleChunker', { ...base, method: 'group' })
        .chunk_token_cap,
    ).toBe(2048);
  });

  it('falls back to a stored shared chunk_token_cap when split keys are absent', () => {
    // Canvas snapshots saved before the split carry only the shared key; the
    // save path must preserve it instead of dropping the configured cap.
    const api = transformFormConfigToApi('TitleChunker', {
      method: 'hierarchy',
      hierarchyHierarchy: '3',
      hierarchyRules: [{ levels: [{ expression: '^#[^#]' }] }],
      groupRules: [{ levels: [{ expression: '第[0-9]+章' }] }],
      chunk_token_cap: 777,
    });
    expect(api.chunk_token_cap).toBe(777);
  });

  it('migrates a legacy levels-only config into both method rule sets', () => {
    const form = transformApiConfigToForm('TitleChunker', {
      method: 'hierarchy',
      hierarchy: 2,
      levels: [['^##']],
    });
    expect(form.hierarchyRules).toEqual([{ levels: [{ expression: '^##' }] }]);
    expect(form.groupRules).toEqual([{ levels: [{ expression: '^##' }] }]);
  });
});
