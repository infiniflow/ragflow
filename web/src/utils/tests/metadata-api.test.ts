import api from '../api';

test('registers the metadata keys endpoint used by the knowledge service', () => {
  expect(api.getMetaKeys).toBe('/api/v1/datasets/metadata/keys');
  expect(api.getMetaKeys).not.toContain('undefined');
});
