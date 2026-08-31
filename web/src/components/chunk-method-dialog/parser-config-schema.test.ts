import {
  fillDefaultParserValue,
  optionalPositiveInt,
} from './parser-config-schema';

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

  it('rejects nonnumeric types instead of coercing them', () => {
    expect(() => optionalPositiveInt.parse(true)).toThrow();
    expect(() => optionalPositiveInt.parse({})).toThrow();
  });
});

describe('fillDefaultParserValue null handling', () => {
  it('uses defaults when task_page_size is null in stored parser config', () => {
    const defaults = { task_page_size: 12, chunk_token_num: 512 };
    const parserConfig = { task_page_size: null, chunk_token_num: 512 };

    const filled = fillDefaultParserValue(defaults, parserConfig);

    expect(filled.task_page_size).toBe(12);
  });
});
