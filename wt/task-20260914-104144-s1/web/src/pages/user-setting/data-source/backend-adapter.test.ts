(globalThis as any).CSS = {
  ...(globalThis as any).CSS,
  supports: () => false,
};

const loadAvailableDataSourceKeys = (language: 'go' | 'python') => {
  jest.resetModules();
  jest.doMock('@/utils/backend-runtime', () => ({
    getBackendLanguage: () => language,
    subscribeBackendLanguage: () => () => undefined,
  }));

  const { getAvailableDataSourceKeys } = require('./backend-adapter');
  return getAvailableDataSourceKeys();
};

describe('data source backend adapter', () => {
  afterEach(() => {
    jest.dontMock('@/utils/backend-runtime');
  });

  it('includes Feishu Wiki for the Python backend', () => {
    expect(loadAvailableDataSourceKeys('python')).toContain('feishu_wiki');
  });

  it('hides only Feishu Wiki for the Go backend', () => {
    const pythonKeys = loadAvailableDataSourceKeys('python');
    const goKeys = loadAvailableDataSourceKeys('go');

    expect(goKeys).not.toContain('feishu_wiki');
    expect(goKeys).toEqual(
      pythonKeys.filter((source: string) => source !== 'feishu_wiki'),
    );
  });
});
