import { z } from 'zod';

export const optionalPositiveInt = z.preprocess(
  (value) => {
    if (value === null || value === undefined || value === '') {
      return undefined;
    }
    const num = Number(value);
    return Number.isNaN(num) ? value : num;
  },
  z.number().int().min(1).optional(),
);
