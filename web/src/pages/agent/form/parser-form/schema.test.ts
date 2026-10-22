import { FileType } from '@/constants/file';
import en from '@/locales/en';
import {
  HyperlinkDefaultSetup,
  HyperlinkLimits,
  ParserFields,
} from '../../constant/pipeline';
import { FormSchema } from './schema';

describe('parser FormSchema', () => {
  it.each([undefined, true, false])(
    'retains MinerU extraction switches set to %s',
    (enabled) => {
      const result = FormSchema.parse({
        setups: [
          {
            fileFormat: FileType.PDF,
            parse_method: 'mineru-model-id',
            mineru_parse_method: 'ocr',
            mineru_lang: 'Japanese',
            ...(enabled === undefined
              ? {}
              : {
                  mineru_formula_enable: enabled,
                  mineru_table_enable: enabled,
                }),
          },
        ],
      });
      expect(result.setups[0]).toMatchObject({
        mineru_parse_method: 'ocr',
        mineru_lang: 'Japanese',
      });
      if (enabled !== undefined) {
        expect(result.setups[0]).toMatchObject({
          mineru_formula_enable: enabled,
          mineru_table_enable: enabled,
        });
      }
    },
  );

  it.each(['mineru_formula_enable', 'mineru_table_enable'])(
    'rejects non-boolean %s',
    (key) => {
      for (const value of [null, 'false', 0, [], {}]) {
        const result = FormSchema.safeParse({
          setups: [{ fileFormat: FileType.PDF, [key]: value }],
        });
        expect(result.success).toBe(false);
      }
    },
  );

  it('keeps the explicit image OCR model through schema save/readback', () => {
    const values = {
      setups: [
        {
          fileFormat: FileType.Image,
          ocr_enabled: true,
          ocr_model_id: 'model-b',
        },
      ],
    };
    expect(
      FormSchema.parse(JSON.parse(JSON.stringify(FormSchema.parse(values)))),
    ).toEqual(values);
  });

  it('rejects non-string image OCR model IDs', () => {
    expect(
      FormSchema.safeParse({
        setups: [
          { fileFormat: FileType.Image, ocr_enabled: true, ocr_model_id: 123 },
        ],
      }).success,
    ).toBe(false);
  });

  it('accepts video and audio setups without a model', () => {
    const result = FormSchema.safeParse({
      setups: [
        { fileFormat: FileType.Video, output_format: 'text' },
        { fileFormat: FileType.Audio, output_format: 'text' },
      ],
    });
    expect(result.success).toBe(true);
  });

  it('requires at least one file type', () => {
    const result = FormSchema.safeParse({ setups: [] });
    expect(result.success).toBe(false);
  });

  it('still requires fields for email', () => {
    const result = FormSchema.safeParse({
      setups: [{ fileFormat: FileType.Email, output_format: 'text' }],
    });
    expect(result.success).toBe(false);

    const withFields = FormSchema.safeParse({
      setups: [
        {
          fileFormat: FileType.Email,
          output_format: 'text',
          fields: Object.values(ParserFields),
        },
      ],
    });
    expect(withFields.success).toBe(true);
  });

  it('rejects image OCR off without vision enhancement', () => {
    const result = FormSchema.safeParse({
      setups: [{ fileFormat: FileType.Image, ocr_enabled: false }],
      enable_vision_enhancement: false,
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.path).toEqual([
        'setups',
        0,
        'ocr_enabled',
      ]);
    }
  });

  it('accepts image OCR off with vision enhancement on', () => {
    const result = FormSchema.safeParse({
      setups: [{ fileFormat: FileType.Image, ocr_enabled: false }],
      enable_vision_enhancement: true,
    });
    expect(result.success).toBe(true);
  });

  it('accepts image OCR on without vision enhancement', () => {
    const result = FormSchema.safeParse({
      setups: [{ fileFormat: FileType.Image, ocr_enabled: true }],
    });
    expect(result.success).toBe(true);
  });
});

describe('parser FormSchema linked-content budgets', () => {
  it.each([
    ['max_urls', HyperlinkLimits.maxUrls],
    ['max_bytes_per_url', HyperlinkLimits.maxBytesPerUrl],
    ['max_total_bytes', HyperlinkLimits.maxTotalBytes],
    ['timeout_s', HyperlinkLimits.timeoutSeconds],
  ])('accepts the defaults and rejects %s outside its range', (key, max) => {
    const parse = (setup: Record<string, unknown>) =>
      FormSchema.safeParse({
        setups: [{ fileFormat: FileType.PDF, ...setup }],
      });

    expect(
      parse({
        [key]: HyperlinkDefaultSetup[key as keyof typeof HyperlinkDefaultSetup],
      }).success,
    ).toBe(true);
    // z.coerce accepts a numeric string: the input is a number field and the
    // API stores a JSON number, so the form normalizes rather than rejects.
    for (const value of [0, -1, max + 1, 1.5]) {
      expect(parse({ [key]: value }).success).toBe(false);
    }
  });

  it('reports the localized message rather than zod defaults', () => {
    const result = FormSchema.safeParse({
      setups: [
        { fileFormat: FileType.PDF, max_urls: HyperlinkLimits.maxUrls + 1 },
      ],
    });
    expect(result.success).toBe(false);
    expect(result.error!.issues[0].message).toBe(
      en.translation.flow.hyperlinkBudgetInvalid,
    );
  });
});
