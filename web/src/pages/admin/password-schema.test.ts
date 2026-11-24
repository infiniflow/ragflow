import { rsaPsw } from '@/utils';
import type { TFunction } from 'i18next';
import {
  adminPasswordSchema,
  adminPasswordTransportSchema,
} from './password-schema';

it.each([
  ['a', 128, true],
  ['字', 61, true],
  ['字', 62, false],
  ['🔐', 45, true],
  ['🔐', 46, false],
] as const)(
  'matches the real RSA payload limit for %s repeated %i times',
  (character, count, allowed) => {
    const password = character.repeat(count);
    const translate = ((key: string) => key) as TFunction;
    expect(
      adminPasswordTransportSchema(translate).safeParse(password).success,
    ).toBe(allowed);
    expect(adminPasswordSchema(translate).safeParse(password).success).toBe(
      allowed,
    );
    if (allowed) {
      expect(rsaPsw(password)).toEqual(expect.any(String));
    }
  },
);
