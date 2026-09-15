import {
  addTenantParams,
  buildModelValue,
  getEmbeddingBaseName,
  parseModelUuid,
  parseModelValue,
} from '../llm-util';

// Composite model keys are right-anchored:
// "model_name@instance_name@provider_name" or "model_name@provider_name".
// Model names may legally contain '@' (LM Studio IDs like
// "text-embedding-nomic-embed-text-v1.5@q8_0"), so the parser must split from
// the right and only treat the last two '@'-separated fields as instance and
// provider. Mirrors api/db/joint_services/tenant_model_service.py
// split_model_name (PR #16468).

describe('parseModelValue — right-anchored split', () => {
  test('plain 3-part composite', () => {
    expect(parseModelValue('gemma@lmstudio@LM-Studio')).toEqual({
      model_name: 'gemma',
      model_instance: 'lmstudio',
      model_provider: 'LM-Studio',
    });
  });

  test('2-part composite defaults instance to "default"', () => {
    expect(parseModelValue('gemma@LM-Studio')).toEqual({
      model_name: 'gemma',
      model_instance: 'default',
      model_provider: 'LM-Studio',
    });
  });

  test('4-part composite with embedded "@" in model name (LM Studio embedding)', () => {
    expect(
      parseModelValue(
        'text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio',
      ),
    ).toEqual({
      model_name: 'text-embedding-nomic-embed-text-v1.5@q8_0',
      model_instance: 'lmstudio',
      model_provider: 'LM-Studio',
    });
  });

  test('quants with multiple "@" in model name still anchor on the last two', () => {
    expect(parseModelValue('org/model@sha@q8_0@default@Builtin')).toEqual({
      model_name: 'org/model@sha@q8_0',
      model_instance: 'default',
      model_provider: 'Builtin',
    });
  });

  test('returns null for empty input', () => {
    expect(parseModelValue('')).toBeNull();
  });

  test('returns null when no "@" is present', () => {
    expect(parseModelValue('plain-model-name')).toBeNull();
  });
});

describe('buildModelValue round-trips parseModelValue', () => {
  test('simple triplet', () => {
    const v = buildModelValue({
      model_name: 'gemma',
      model_instance: 'lmstudio',
      model_provider: 'LM-Studio',
    });
    expect(v).toBe('gemma@lmstudio@LM-Studio');
    expect(parseModelValue(v)).toEqual({
      model_name: 'gemma',
      model_instance: 'lmstudio',
      model_provider: 'LM-Studio',
    });
  });

  test('round-trip survives embedded "@" in model name', () => {
    const v = buildModelValue({
      model_name: 'text-embedding-nomic-embed-text-v1.5@q8_0',
      model_instance: 'lmstudio',
      model_provider: 'LM-Studio',
    });
    expect(v).toBe(
      'text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio',
    );
    expect(parseModelValue(v)).toEqual({
      model_name: 'text-embedding-nomic-embed-text-v1.5@q8_0',
      model_instance: 'lmstudio',
      model_provider: 'LM-Studio',
    });
  });
});

describe('parseModelUuid — right-anchored', () => {
  test('simple "model@factory" splits on the last "@"', () => {
    expect(parseModelUuid('gpt-4@ZHIPU-AI')).toEqual({
      modelName: 'gpt-4',
      factoryId: 'ZHIPU-AI',
    });
  });

  test('preserves embedded "@" in the model name (LM Studio)', () => {
    expect(
      parseModelUuid('text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio'),
    ).toEqual({
      modelName: 'text-embedding-nomic-embed-text-v1.5@q8_0',
      factoryId: 'lmstudio',
    });
  });

  test('ignores "#instance" suffix when splitting the factory portion', () => {
    expect(parseModelUuid('gpt-4@ZHIPU-AI#CI')).toEqual({
      modelName: 'gpt-4',
      factoryId: 'ZHIPU-AI',
    });
  });

  test('returns empty factoryId when no "@" is present', () => {
    expect(parseModelUuid('plain-model')).toEqual({
      modelName: 'plain-model',
      factoryId: '',
    });
  });
});

describe('getEmbeddingBaseName — dataset co-selection grouping', () => {
  test('3-part composite reduces to bare model name', () => {
    expect(getEmbeddingBaseName('BAAI/bge-m3@renew@SILICONFLOW')).toBe(
      'BAAI/bge-m3',
    );
  });

  test('same model through different instances shares one base name', () => {
    expect(
      getEmbeddingBaseName('BAAI/bge-m3@renew@SILICONFLOW') ===
        getEmbeddingBaseName('BAAI/bge-m3@COPY@SILICONFLOW'),
    ).toBe(true);
  });

  test('2-part composite strips the provider', () => {
    expect(getEmbeddingBaseName('BAAI/bge-m3@SILICONFLOW')).toBe('BAAI/bge-m3');
  });

  test('model names containing "@" keep their suffix', () => {
    expect(
      getEmbeddingBaseName(
        'text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio',
      ),
    ).toBe('text-embedding-nomic-embed-text-v1.5@q8_0');
  });

  test('opaque tenant_model id is returned unchanged', () => {
    expect(getEmbeddingBaseName('2d8ff0a97d75431c8c91526549939328')).toBe(
      '2d8ff0a97d75431c8c91526549939328',
    );
  });

  test('empty and undefined values produce an empty base name', () => {
    expect(getEmbeddingBaseName('')).toBe('');
    expect(getEmbeddingBaseName(undefined)).toBe('');
  });
});

describe('addTenantParams — clearing model selections', () => {
  test('clears a stale tenant rerank model id when rerank is explicitly cleared', () => {
    expect(
      addTenantParams(
        {
          rerank_id: '',
          tenant_rerank_id: 'stale-tenant-model-id',
        },
        '/api/v1/chats/chat-id',
      ),
    ).toEqual({
      rerank_id: '',
      tenant_rerank_id: null,
    });
  });

  test('preserves the tenant rerank model id when rerank is not part of the request', () => {
    expect(
      addTenantParams(
        { tenant_rerank_id: 'existing-tenant-model-id' },
        '/api/v1/chats/chat-id',
      ),
    ).toEqual({ tenant_rerank_id: 'existing-tenant-model-id' });
  });
});
