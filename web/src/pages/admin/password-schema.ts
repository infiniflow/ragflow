import type { TFunction } from 'i18next';
import { Base64 } from 'js-base64';
import { z } from 'zod';

// rsaPsw encrypts a Base64 string with a 2048-bit RSA key and PKCS#1 v1.5 padding.
const MaxPasswordCipherInputBytes = 256 - 11;

export function adminPasswordTransportSchema(t: TFunction) {
  return z
    .string()
    .refine(
      (value) => Base64.encode(value).length <= MaxPasswordCipherInputBytes,
      t('adminEnhancements.passwordTransportMaxLength'),
    );
}

export function adminPasswordSchema(t: TFunction) {
  return z
    .string()
    .refine(
      (value) => Array.from(value).length >= 8,
      t('admin.passwordMinLength'),
    )
    .refine(
      (value) => Array.from(value).length <= 128,
      t('adminEnhancements.passwordMaxLength'),
    )
    .pipe(adminPasswordTransportSchema(t));
}
