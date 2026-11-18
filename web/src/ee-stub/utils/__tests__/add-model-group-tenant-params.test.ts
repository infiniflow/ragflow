import { addModelGroupTenantParams } from '../add-model-group-tenant-params';

it('returns the original request by reference in OSS', () => {
  const request = { llm_id: 'gp-0123456789abcdef0123456789abcdef' };
  expect(addModelGroupTenantParams(request)).toBe(request);
});
