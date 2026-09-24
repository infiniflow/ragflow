import type { IParserConfig } from '@/interfaces/database/document';

/**
 * Merge a stored parser-config payload with the per-field defaults, treating
 * `null` and `undefined` as "not set" rather than as authoritative values.
 *
 * The backend stores every parser_config field with a `null` placeholder until
 * the user sets it. The chunk-method dialog's zod schema uses
 * `z.coerce.number().optional()`, which coerces `null` to `0` on submit. The
 * backend then rejects `task_page_size: 0` because the Pydantic field is
 * `ge=1` — every save fails with "Input should be greater than or equal to 1"
 * even when the user only changed an unrelated field. (See #19039.)
 *
 * Returning `null` to the default keeps every field at its schema default until
 * the user sets one, matching the pre-`null`-placeholder behaviour.
 */
export function fillParserConfigDefaults(
  parserConfig: Partial<IParserConfig> | undefined,
  defaultParserValues: IParserConfig,
): IParserConfig {
  return Object.entries(defaultParserValues).reduce<Record<string, any>>(
    (pre, [key, value]) => {
      const stored = parserConfig?.[key as keyof IParserConfig];
      if (stored !== null && stored !== undefined) {
        pre[key] = stored;
      } else {
        pre[key] = value;
      }
      return pre;
    },
    {},
  ) as IParserConfig;
}
