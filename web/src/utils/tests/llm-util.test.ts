import { buildModelValue, parseModelUuid, parseModelValue } from '../llm-util';

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
