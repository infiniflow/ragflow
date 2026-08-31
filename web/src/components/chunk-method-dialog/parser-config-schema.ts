import { z } from 'zod';

export function fillDefaultParserValue<T extends Record<string, unknown>>(
  defaults: T,
  parserConfig: Record<string, unknown>,
) {
  return Object.entries(defaults).reduce<Record<string, unknown>>(
    (pre, [key, value]) => {
      const stored = parserConfig[key];
      if (key in parserConfig && stored != null) {
        pre[key] = stored;
      } else {
        pre[key] = value;
      }
      return pre;
    },
    {},
  );
}

export const optionalPositiveInt = z.preprocess(
  (value) => {
    if (value === null || value === undefined || value === '') {
      return undefined;
    }
    if (typeof value !== 'string' && typeof value !== 'number') {
      return value;
    }
    const num = Number(value);
    return Number.isNaN(num) ? value : num;
  },
  z.number().int().min(1).optional(),
);
