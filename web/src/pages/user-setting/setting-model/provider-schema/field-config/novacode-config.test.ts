import { LLMFactory } from '@/constants/llm';
import { LIST_MODEL_PROVIDERS } from '../constants';
import { getProviderConfig } from './get-provider-config';

jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: new Proxy({}, { get: (_target, key) => String(key) }),
}));

describe('NovaCode provider configuration', () => {
  const config = getProviderConfig(LLMFactory.NovaCode);

  it('offers model discovery and defaults to Responses with medium effort', () => {
    expect(LIST_MODEL_PROVIDERS.has(LLMFactory.NovaCode)).toBe(true);
    expect(
      config.fields.find((field) => field.name === 'wire_api'),
    ).toMatchObject({
      defaultValue: 'responses',
      options: [
        { label: 'Responses', value: 'responses' },
        { label: 'Chat Completions', value: 'chat_completions' },
      ],
    });
    expect(
      config.fields.find((field) => field.name === 'reasoning_effort'),
    ).toMatchObject({
      defaultValue: 'medium',
      options: ['low', 'medium', 'high', 'xhigh', 'max'].map((value) => ({
        label: value,
        value,
      })),
    });
  });

  it('verifies with the same protocol, effort and key it saves', () => {
    const values = {
      instance_name: 'NovaCode',
      api_key: 'test-key',
      base_url: 'https://ai.novacode.top/v1',
      wire_api: 'chat_completions',
      reasoning_effort: 'high',
      model_info: [{ model_name: 'gpt-6.1-sol', model_type: ['chat'] }],
    };
    const credentials = {
      api_key: 'test-key',
      wire_api: 'chat_completions',
      reasoning_effort: 'high',
    };
    expect(config.verifyTransform?.(values)).toMatchObject({
      apiKey: credentials,
      baseUrl: values.base_url,
      modelInfo: values.model_info,
    });
    expect(config.submitTransform?.(values)).toMatchObject({
      api_key: credentials,
      llm_factory: LLMFactory.NovaCode,
      model_info: values.model_info,
    });
  });

  it('uses valid protocol and effort defaults when values are absent', () => {
    expect(config.verifyTransform?.({ api_key: 'key' }).apiKey).toEqual({
      api_key: 'key',
      wire_api: 'responses',
      reasoning_effort: 'medium',
    });
  });
});
