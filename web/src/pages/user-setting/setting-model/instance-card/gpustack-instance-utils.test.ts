import {
  classifyConnectionError,
  diffModels,
  groupModelsByKind,
  isLoopbackUrl,
  kindFromModelTypes,
  modelTypesFromKind,
  toGPUStackModel,
} from './gpustack-instance-utils';

describe('kindFromModelTypes', () => {
  it.each([
    [['chat'], 'chat'],
    [['chat', 'vision'], 'vision'],
    [['embedding'], 'embedding'],
    [['rerank'], 'rerank'],
    [['asr'], 'asr'],
    [['tts'], 'tts'],
    [[], 'chat'],
    [undefined, 'chat'],
  ])('maps %j to %s', (types, kind) => {
    expect(kindFromModelTypes(types as string[] | undefined)).toBe(kind);
  });

  it('round-trips through modelTypesFromKind', () => {
    expect(modelTypesFromKind('vision')).toEqual(['chat', 'vision']);
    expect(kindFromModelTypes(modelTypesFromKind('vision'))).toBe('vision');
    expect(modelTypesFromKind('tts')).toEqual(['tts']);
  });
});

describe('toGPUStackModel', () => {
  it('keeps the reported context length', () => {
    expect(
      toGPUStackModel({ name: 'm', model_types: ['chat'], max_tokens: 4096 }),
    ).toEqual({ name: 'm', kind: 'chat', maxTokens: 4096 });
  });

  it('falls back when no context length is reported', () => {
    expect(
      toGPUStackModel({ name: 'm', model_types: ['asr'], max_tokens: 0 })
        .maxTokens,
    ).toBe(8192);
  });
});

describe('groupModelsByKind', () => {
  it('orders groups and omits empty ones', () => {
    const groups = groupModelsByKind([
      { name: 'r', kind: 'rerank' as const },
      { name: 'c', kind: 'chat' as const },
    ]);
    expect(groups.map((group) => group.kind)).toEqual(['chat', 'rerank']);
  });
});

describe('isLoopbackUrl', () => {
  it.each([
    ['http://localhost:8080', true],
    ['http://127.0.0.1', true],
    ['http://host.docker.internal:80', false],
    ['http://10.0.0.5', false],
    ['nonsense', false],
  ])('%s -> %s', (url, expected) => {
    expect(isLoopbackUrl(url)).toBe(expected);
  });
});

describe('classifyConnectionError', () => {
  it.each([
    ['API request failed with status 401: {"code":401}', 'auth'],
    ['failed to send request: dial tcp 10.0.0.5:80: connect', 'unreachable'],
    ['context deadline exceeded', 'unreachable'],
    ['API request failed with status 500: boom', 'other'],
    [undefined, 'other'],
  ])('%s -> %s', (message, expected) => {
    expect(classifyConnectionError(message)).toBe(expected);
  });
});

describe('diffModels', () => {
  const model = (name: string) => ({
    name,
    kind: 'chat' as const,
    maxTokens: 8192,
  });

  it('separates new models and marks stopped ones', () => {
    const { notAdded, added } = diffModels(
      [model('running'), model('new')],
      [model('running'), model('stopped')],
    );
    expect(notAdded.map((m) => m.name)).toEqual(['new']);
    expect(added).toEqual([
      { ...model('running'), running: true },
      { ...model('stopped'), running: false },
    ]);
  });
});
