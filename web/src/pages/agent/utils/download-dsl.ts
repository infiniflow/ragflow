import { DSL } from '@/interfaces/database/agent';
import { downloadJsonFile } from '@/utils/file-util';
import { clearSensitiveFields } from './clear-sensitive-fields';

/**
 * Shared agent-DSL download path: strips sensitive fields (api_key)
 * and writes the canonical wire shape to `<title>.json`. Used by the
 * canvas export button and the version-history dialog so both emit
 * the same structure.
 */
export const downloadDsl = (dsl: DSL | Record<string, any>, title: string) => {
  const sanitizedDsl = clearSensitiveFields(dsl);
  downloadJsonFile(
    { ...sanitizedDsl, globals: { ...(sanitizedDsl.globals ?? {}) } },
    `${title}.json`,
  );
};
