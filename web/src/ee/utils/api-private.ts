/**
 * Enterprise customization overlay for `api.ts`.
 *
 * Upstream ships this file as an empty object; the enterprise repository
 * fills the same-path file with real endpoints, so the shared endpoint
 * table stays merge conflict-free.
 *
 * Values here are deep-merged over the endpoint table in `api.ts`
 * (lodash/merge): a key present here overrides the same key there.
 */
export default {};
