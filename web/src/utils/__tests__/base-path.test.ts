import {
  getRouterBasename,
  normalizeViteBasePath,
  withAppBasePath,
} from '../base-path';

describe('base-path helpers', () => {
  it('normalizes vite base paths', () => {
    expect(normalizeViteBasePath()).toBe('/');
    expect(normalizeViteBasePath('/')).toBe('/');
    expect(normalizeViteBasePath('/ragflow')).toBe('/ragflow/');
    expect(normalizeViteBasePath('/ragflow/')).toBe('/ragflow/');
    expect(normalizeViteBasePath('ragflow')).toBe('/ragflow/');
  });

  it('derives router basename without trailing slash', () => {
    expect(getRouterBasename('/ragflow/')).toBe('/ragflow');
    expect(getRouterBasename('/')).toBe('/');
  });

  it('prefixes absolute paths when basename is provided', () => {
    expect(withAppBasePath('/api/v1/datasets')).toBe('/api/v1/datasets');
  });
});
