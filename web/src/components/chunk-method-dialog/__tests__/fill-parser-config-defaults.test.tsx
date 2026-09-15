import { fillParserConfigDefaults } from '../fill-parser-config-defaults';
import type { IParserConfig } from '@/interfaces/database/document';

const DEFAULTS = {
  task_page_size: 12,
  layout_recognize: 'DeepDOC' as const,
  chunk_token_num: 512,
  delimiter: '\n',
  enable_children: false,
  auto_keywords: 0,
} as unknown as IParserConfig;

describe('fillParserConfigDefaults', () => {
  it('uses the per-field default when the key is absent (Issue #19039)', () => {
    expect(fillParserConfigDefaults(undefined, DEFAULTS)).toEqual(
      expect.objectContaining({
        task_page_size: 12,
        chunk_token_num: 512,
        delimiter: '\n',
      }),
    );
  });

  it('uses the per-field default when the stored value is null', () => {
    const result = fillParserConfigDefaults(
      { task_page_size: null } as Partial<IParserConfig>,
      DEFAULTS,
    );
    expect(result.task_page_size).toBe(12);
  });

  it('uses the per-field default when the stored value is undefined', () => {
    const result = fillParserConfigDefaults(
      { task_page_size: undefined } as Partial<IParserConfig>,
      DEFAULTS,
    );
    expect(result.task_page_size).toBe(12);
  });

  it('preserves a user-supplied non-null value', () => {
    const result = fillParserConfigDefaults(
      { task_page_size: 22, delimiter: '###' } as Partial<IParserConfig>,
      DEFAULTS,
    );
    expect(result.task_page_size).toBe(22);
    expect(result.delimiter).toBe('###');
  });

  it('drops the null entry while preserving non-null entries in the same payload', () => {
    const result = fillParserConfigDefaults(
      {
        task_page_size: null,
        chunk_token_num: 1024,
      } as Partial<IParserConfig>,
      DEFAULTS,
    );
    expect(result.task_page_size).toBe(12);
    expect(result.chunk_token_num).toBe(1024);
  });
});
