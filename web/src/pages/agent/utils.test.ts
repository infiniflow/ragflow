import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { FileType } from '@/constants/file';
import { normalizeParserConfig } from '@/hooks/parser-config-utils';
import {
  transformApiConfigToForm,
  transformSavedParserConfigToForm,
} from '@/utils/pipeline-operator';
import { Operator } from './constant';
import { FormSchema } from './form/parser-form/schema';
import {
  generateNodeNamesWithIncreasingIndex,
  isEmptyMessageContent,
  normalizeTokenChunkerFormValues,
  receiveMessageError,
  shouldSeedDefaultDelimiter,
  transformGeneralChunkerParams,
  transformParserParams,
  transformTokenChunkerParams,
} from './utils';

describe('transformParserParams MinerU settings', () => {
  it('round-trips component-scoped choices through validation, save and reload', () => {
    const config = {
      'Parser:primary': {
        pdf: {
          parse_method: 'mineru-model-id',
          mineru_formula_enable: false,
          mineru_table_enable: true,
          mineru_parse_method: 'ocr',
          mineru_lang: 'Japanese',
        },
      },
      'Parser:secondary': {
        pdf: {
          parse_method: 'mineru-model-id',
          mineru_formula_enable: true,
          mineru_table_enable: false,
        },
      },
    };
    const form = transformSavedParserConfigToForm(config)!;
    const saved = Object.fromEntries(
      Object.entries(form).map(([id, values]) => [
        id,
        transformParserParams(FormSchema.parse(values)),
      ]),
    );
    const stored = JSON.parse(JSON.stringify(normalizeParserConfig(saved)));
    expect(stored).toMatchObject(config);
    expect(transformSavedParserConfigToForm(stored)).toMatchObject(form);
  });

  it.each([
    { modelId: 'model-b', ocrEnabled: true },
    { modelId: '', ocrEnabled: true },
    { modelId: 'model-b', ocrEnabled: false },
    { modelId: '', ocrEnabled: false },
  ])(
    'round-trips MinerU PDF controls alongside image OCR $modelId with OCR enabled=$ocrEnabled',
    ({ modelId, ocrEnabled }) => {
      const config = {
        'Parser:primary': {
          enable_vision_enhancement: !ocrEnabled,
          vlm: {
            llm_id: 'vision-model-id',
            system_prompt: 'Describe visible details.',
          },
          pdf: {
            parse_method: 'mineru-model-id',
            mineru_formula_enable: false,
            mineru_table_enable: true,
            mineru_parse_method: 'ocr',
            mineru_lang: 'Japanese',
          },
          image: {
            ocr_enabled: ocrEnabled,
            ocr_model_id: modelId,
          },
        },
      };
      // Exercise the saved-config boundary and schema, not live OCR execution.
      const form = transformSavedParserConfigToForm(config)!;
      const saved = {
        'Parser:primary': transformParserParams(
          FormSchema.parse(form['Parser:primary']),
        ),
      };
      const stored = JSON.parse(JSON.stringify(normalizeParserConfig(saved)));
      expect(stored).toMatchObject(config);
      expect(stored['Parser:primary'].pdf).not.toHaveProperty('ocr_enabled');
      expect(stored['Parser:primary'].pdf).not.toHaveProperty('ocr_model_id');
      for (const key of [
        'mineru_formula_enable',
        'mineru_table_enable',
        'mineru_parse_method',
        'mineru_lang',
      ]) {
        expect(stored['Parser:primary'].image).not.toHaveProperty(key);
      }

      const reloaded = transformSavedParserConfigToForm(stored)!;
      const savedAgain = {
        'Parser:primary': transformParserParams(
          FormSchema.parse(reloaded['Parser:primary']),
        ),
      };
      expect(
        JSON.parse(JSON.stringify(normalizeParserConfig(savedAgain))),
      ).toEqual(stored);
    },
  );

  it.each([undefined, true, false])(
    'preserves independent formula/table choices and engine settings: %s',
    (formulaEnabled) => {
      for (const tableEnabled of [undefined, true, false]) {
        const result = transformParserParams({
          setups: [
            {
              fileFormat: FileType.PDF,
              parse_method: 'mineru-model-id',
              mineru_parse_method: 'ocr',
              mineru_lang: 'Japanese',
              ...(formulaEnabled === undefined
                ? {}
                : { mineru_formula_enable: formulaEnabled }),
              ...(tableEnabled === undefined
                ? {}
                : { mineru_table_enable: tableEnabled }),
            },
          ],
        } as any);
        const saved = JSON.parse(JSON.stringify(result)).pdf;
        expect(saved).toMatchObject({
          parse_method: 'mineru-model-id',
          mineru_parse_method: 'ocr',
          mineru_lang: 'Japanese',
        });
        if (formulaEnabled === undefined) {
          expect(saved).not.toHaveProperty('mineru_formula_enable');
        } else {
          expect(saved.mineru_formula_enable).toBe(formulaEnabled);
        }
        if (tableEnabled === undefined) {
          expect(saved).not.toHaveProperty('mineru_table_enable');
        } else {
          expect(saved.mineru_table_enable).toBe(tableEnabled);
        }
      }
    },
  );
});

describe('transformParserParams vision settings', () => {
  it.each(['model-b', ''])(
    'saves the explicit image OCR selection %j',
    (modelId) => {
      const result: Record<string, any> = transformParserParams({
        enable_vision_enhancement: false,
        setups: [
          {
            fileFormat: FileType.Image,
            ocr_enabled: true,
            ocr_model_id: modelId,
          },
        ],
      } as any);
      expect(result.image).toMatchObject({
        ocr_enabled: true,
        ocr_model_id: modelId,
      });
      expect(result.vlm.llm_id).toBe('');
      const reloaded = transformApiConfigToForm(
        Operator.Parser,
        JSON.parse(JSON.stringify(result)),
      );
      const savedAgain: Record<string, any> = transformParserParams(
        reloaded as any,
      );
      expect(savedAgain.image).toEqual(result.image);
    },
  );
  it('saves the normalized global settings when the node form was not opened', () => {
    const result = transformParserParams({
      setups: [
        { fileFormat: FileType.PDF, vlm: { llm_id: 'model-B' } },
        { fileFormat: FileType.Audio, vlm: { llm_id: 'asr-model' } },
      ],
    } as any);

    expect(result.enable_vision_enhancement).toBe(true);
    expect(result.vlm).toEqual({
      llm_id: 'model-B',
      system_prompt: '',
    });
    expect(result).not.toHaveProperty(`${FileType.PDF}.vlm`);
    expect(result).toHaveProperty(`${FileType.Audio}.vlm`, {
      llm_id: 'asr-model',
    });
  });

  it.each([true, false])('preserves model B with enhancement %s', (enabled) => {
    const result = transformParserParams({
      enable_vision_enhancement: enabled,
      vlm: { llm_id: 'model-B' },
      setups: [{ fileFormat: FileType.Image, parse_method: 'ocr' }],
    } as any);

    expect(result.enable_vision_enhancement).toBe(enabled);
    expect(result.vlm).toEqual({
      llm_id: 'model-B',
      system_prompt: '',
    });
  });

  it('writes the image OCR switch without retired vision fields', () => {
    // The saved shape is the contract the Go parser reads, so pin it: the
    // switch out, and the retired per-family keys never written back.
    const result: Record<string, any> = transformParserParams({
      enable_vision_enhancement: false,
      vlm: { llm_id: '', system_prompt: '' },
      setups: [
        {
          fileFormat: FileType.Image,
          ocr_enabled: false,
          parse_method: 'stale-model@provider',
          lang: 'French',
          system_prompt: 'stale',
          output_format: 'json',
        },
      ],
    } as any);

    expect(result.image).toMatchObject({ ocr_enabled: false });
    expect(result.image).not.toHaveProperty('parse_method');
    expect(result.image).not.toHaveProperty('lang');
    expect(result.image).not.toHaveProperty('system_prompt');
    // output_format is retired: the Go Parser normalizes every family to
    // JSON, so a stored legacy value must never be written back.
    expect(result.image).not.toHaveProperty('output_format');
  });
});

describe('transformTokenChunkerParams', () => {
  it('keeps overlapped_percent and delimiters when delimiter_mode is one', () => {
    // Regression: saving with delimiter_mode='one' used to zero these fields,
    // so a reload showed overlapped_percent=0 and delimiters=["\n"].
    const result = transformTokenChunkerParams({
      delimiter_mode: 'one',
      chunk_token_size: 512,
      overlapped_percent: 9,
      image_table_context_window: 81,
      delimiters: [{ value: '\n' }, { value: '!' }, { value: '。' }],
      children_delimiters: [],
      enable_children: false,
    } as any);

    expect(result.overlapped_percent).toBeCloseTo(0.09);
    expect(result.delimiters).toEqual(['\n', '!', '。']);
    expect(result.delimiter_mode).toBe('one');
  });

  it('converts form values to api format in delimiter mode', () => {
    const result = transformTokenChunkerParams({
      delimiter_mode: 'delimiter',
      chunk_token_size: 512,
      overlapped_percent: 30,
      image_table_context_window: 81,
      delimiters: [{ value: '\n' }, { value: '' }],
      children_delimiters: [{ value: '|' }],
      enable_children: true,
    } as any);

    expect(result.overlapped_percent).toBeCloseTo(0.3);
    expect(result.delimiters).toEqual(['\n']);
    expect(result.children_delimiters).toEqual(['|']);
    expect(result.table_context_size).toBe(81);
    expect(result.image_context_size).toBe(81);
  });

  it('emits an explicit empty delimiters list when the user removed all rows', () => {
    // The backend falls back to its own default (["\n"]) when the key is
    // absent, so the save path must emit [] to keep pure token-size chunking.
    const result = transformTokenChunkerParams({
      delimiter_mode: 'delimiter',
      chunk_token_size: 512,
      overlapped_percent: 0,
      image_table_context_window: 0,
      delimiters: [],
      children_delimiters: [],
      enable_children: false,
    } as any);

    expect(result.delimiters).toEqual([]);
  });

  it('migrates a legacy node on save even when the form was never opened', () => {
    // Nodes saved under the removed 'token_size' tab persisted an empty list;
    // the save path applies the same migration the form would.
    const result = transformTokenChunkerParams({
      delimiter_mode: 'token_size',
      chunk_token_size: 512,
      overlapped_percent: 0,
      image_table_context_window: 0,
      delimiters: [],
      children_delimiters: [],
    } as any);

    expect(result.delimiter_mode).toBe('delimiter');
    expect(result.delimiters).toEqual(['\n']);
  });

  it('keeps legacy children delimiters instead of wiping them', () => {
    // Nodes saved before the enable_children toggle existed carry children
    // delimiters without the flag; derive it instead of dropping the values.
    const result = transformTokenChunkerParams({
      delimiter_mode: 'delimiter',
      chunk_token_size: 512,
      overlapped_percent: 0,
      image_table_context_window: 0,
      delimiters: [{ value: '\n' }],
      children_delimiters: [{ value: '|' }],
    } as any);

    expect(result.enable_children).toBe(true);
    expect(result.children_delimiters).toEqual(['|']);
  });
});

describe('transformGeneralChunkerParams', () => {
  it('never re-seeds the general chunker delimiter list on save', () => {
    const result = transformGeneralChunkerParams({
      chunk_token_size: 512,
      overlapped_percent: 0,
      delimiters: [],
      children_delimiters: [],
      enable_children: false,
      table_context_size: 0,
      image_context_size: 0,
    } as any);

    expect(result.delimiters).toEqual([]);
    expect(result).not.toHaveProperty('delimiter_mode');
  });
});

describe('normalizeTokenChunkerFormValues', () => {
  it('normalizes a legacy token_size node and re-seeds the delimiter row', () => {
    const result = normalizeTokenChunkerFormValues({
      delimiter_mode: 'token_size',
      delimiters: [],
      children_delimiters: [],
    });

    expect(result.delimiter_mode).toBe('delimiter');
    expect(result.delimiters).toEqual([{ value: '\n' }]);
  });

  it('keeps a deliberate empty list saved by the current form', () => {
    const result = normalizeTokenChunkerFormValues({
      delimiter_mode: 'one',
      delimiters: [],
    });

    expect(result.delimiter_mode).toBe('one');
    expect(result.delimiters).toEqual([]);
  });

  it('derives enable_children from a non-empty children list', () => {
    const result = normalizeTokenChunkerFormValues({
      children_delimiters: [{ value: '\n\n' }],
    });

    expect(result.enable_children).toBe(true);
  });

  it('respects an explicit enable_children flag over derivation', () => {
    const result = normalizeTokenChunkerFormValues({
      enable_children: false,
      children_delimiters: [{ value: '\n\n' }],
    });

    expect(result.enable_children).toBe(false);
  });

  it('is idempotent', () => {
    const once = normalizeTokenChunkerFormValues({
      delimiters: [],
      children_delimiters: [{ value: '|' }],
    });

    expect(normalizeTokenChunkerFormValues(once)).toEqual(once);
  });

  it('skips the legacy re-seed when seedLegacyDelimiter is false', () => {
    const result = normalizeTokenChunkerFormValues(
      { delimiters: [] },
      { seedLegacyDelimiter: false },
    );

    expect(result.delimiters).toEqual([]);
  });
});

describe('shouldSeedDefaultDelimiter', () => {
  it('seeds only for legacy nodes with an empty list', () => {
    // Legacy 'token_size' tab nodes always persisted an empty list.
    expect(shouldSeedDefaultDelimiter('token_size', [])).toBe(true);
    // Older DSLs may not carry a mode at all.
    expect(shouldSeedDefaultDelimiter(undefined, [])).toBe(true);
    expect(shouldSeedDefaultDelimiter(undefined, undefined)).toBe(true);
  });

  it('never seeds when the current form saved the node', () => {
    // An explicit mode means the empty list is a deliberate user choice
    // (pure token-size chunking) and must survive a reload.
    expect(shouldSeedDefaultDelimiter('delimiter', [])).toBe(false);
    expect(shouldSeedDefaultDelimiter('one', [])).toBe(false);
  });

  it('never seeds a non-empty list', () => {
    expect(shouldSeedDefaultDelimiter('token_size', ['\n'])).toBe(false);
    expect(shouldSeedDefaultDelimiter(undefined, [{ value: '\n' }])).toBe(
      false,
    );
  });
});

describe('receiveMessageError', () => {
  it('accepts successful SSE events without an application code', () => {
    expect(
      receiveMessageError({
        response: { status: 200 },
        data: { event: 'workflow_finished' },
      }),
    ).toBe(false);
  });
  it('rejects application errors returned with HTTP 200', () => {
    expect(
      receiveMessageError({ response: { status: 200 }, data: { code: 102 } }),
    ).toBe(true);
    expect(
      receiveMessageError({ response: { status: 200 }, data: { code: 0 } }),
    ).toBe(false);
  });
});

describe('Message component content validation', () => {
  describe('isEmptyMessageContent', () => {
    it('treats missing, non-array and blank-only content as empty', () => {
      expect(isEmptyMessageContent()).toBe(true);
      expect(isEmptyMessageContent(null)).toBe(true);
      expect(isEmptyMessageContent('hello')).toBe(true);
      expect(isEmptyMessageContent([])).toBe(true);
      expect(isEmptyMessageContent(['', ' \t '])).toBe(true);
      // Non-string entries never satisfy the backend either.
      expect(isEmptyMessageContent([123])).toBe(true);
    });

    it('accepts content with at least one non-blank string entry', () => {
      expect(isEmptyMessageContent(['hi'])).toBe(false);
      expect(isEmptyMessageContent(['', '{begin@query}'])).toBe(true);
      expect(isEmptyMessageContent(['  text  '])).toBe(false);
    });
  });
});

describe('generateNodeNamesWithIncreasingIndex', () => {
  const createNamedNode = (name: string) =>
    ({
      id: `${Operator.Retrieval}:${name}`,
      type: 'ragNode',
      position: { x: 0, y: 0 },
      data: { label: Operator.Retrieval, name, form: {} },
    }) as RAGFlowNodeType;

  it('uses the bare name for the first operator of a type', () => {
    expect(generateNodeNamesWithIncreasingIndex('Retrieval', [])).toBe(
      'Retrieval',
    );
  });

  it('appends an index only from the second operator on', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval'),
      ]),
    ).toBe('Retrieval_1');
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval'),
        createNamedNode('Retrieval_1'),
      ]),
    ).toBe('Retrieval_2');
  });

  it('fills the gap between existing indexes', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval'),
        createNamedNode('Retrieval_2'),
      ]),
    ).toBe('Retrieval_1');
  });

  it('does not backfill index 0 when only suffixed names exist', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval_1'),
      ]),
    ).toBe('Retrieval_2');
  });

  it('treats a legacy _0 name as the first operator', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval_0'),
      ]),
    ).toBe('Retrieval_1');
  });

  it('ignores nodes of other types and non-indexed names', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Message'),
        createNamedNode('Retrieval_beta'),
        createNamedNode('Retrieval_1_extra'),
      ]),
    ).toBe('Retrieval');
  });
});
