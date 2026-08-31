import { optionalPositiveInt } from './parser-config-schema';

describe('optionalPositiveInt', () => {
  it('leaves null and empty values unset instead of coercing to 0', () => {
    expect(optionalPositiveInt.parse(null)).toBeUndefined();
    expect(optionalPositiveInt.parse(undefined)).toBeUndefined();
    expect(optionalPositiveInt.parse('')).toBeUndefined();
  });

  it('accepts valid positive integers', () => {
    expect(optionalPositiveInt.parse(12)).toBe(12);
    expect(optionalPositiveInt.parse('24')).toBe(24);
  });
});

describe('fillDefaultParserValue null handling', () => {
  it('uses defaults when task_page_size is null in stored parser config', () => {
    const defaults = { task_page_size: 12, chunk_token_num: 512 };
    const parserConfig = { task_page_size: null, chunk_token_num: 512 };

    const filled = Object.entries(defaults).reduce<Record<string, unknown>>(
      (pre, [key, value]) => {
        const stored = parserConfig[key as keyof typeof parserConfig];
        if (key in parserConfig && stored != null) {
          pre[key] = stored;
        } else {
          pre[key] = value;
        }
        return pre;
      },
      {},
    );

    expect(filled.task_page_size).toBe(12);
  });
});
