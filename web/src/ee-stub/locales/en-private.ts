/**
 * Enterprise customization overlay for `en.ts`.
 *
 * Upstream ships this file as an empty object; the enterprise repository
 * fills the same-path file with real translations, so the shared locale
 * files stay merge conflict-free.
 *
 * Values here are deep-merged over `en.ts` (lodash/merge): a key present
 * here overrides the same key in `en.ts`.
 */
export default {};
