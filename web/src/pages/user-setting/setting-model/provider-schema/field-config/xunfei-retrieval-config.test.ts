import { LLMFactory } from '@/constants/llm';
import { renderHook } from '@testing-library/react';
import { useCustomModelFields } from '../../instance-card/use-custom-model-fields';
import { LIST_MODEL_PROVIDERS } from '../constants';
import { getProviderConfig } from './get-provider-config';

jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: new Proxy({}, { get: (_target, key) => String(key) }),
}));

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));

describe('XunFei Retrieval provider configuration', () => {
  const config = getProviderConfig(LLMFactory.XunFeiRetrieval);

  it('uses the model picker and restricts setup to a retrieval endpoint', () => {
    expect(LIST_MODEL_PROVIDERS.has(LLMFactory.XunFeiRetrieval)).toBe(true);
    expect(config).toMatchObject({
      llmFactory: LLMFactory.XunFeiRetrieval,
      title: 'XunFei Retrieval',
      defaultInstanceName: 'XunFei Retrieval',
      docLink: 'https://maas.xfyun.cn/doc/',
      docLinkI18nKey: 'ollamaLink',
    });

    expect(
      config.fields.find((field) => field.name === 'api_key'),
    ).toMatchObject({
      label: 'apiKey',
      required: true,
    });
    expect(
      config.fields.find((field) => field.name === 'base_url'),
    ).toMatchObject({
      required: true,
      defaultValue: 'https://maas-api.cn-huabei-1.xf-yun.com/v2',
    });
  });

  it('offers embedding and rerank types for custom models', () => {
    const { result } = renderHook(() =>
      useCustomModelFields(LLMFactory.XunFeiRetrieval),
    );
    expect(
      result.current.find((field) => field.name === 'model_types'),
    ).toMatchObject({
      required: true,
      options: [
        { label: 'modelTypes.embedding', value: 'embedding' },
        { label: 'modelTypes.rerank', value: 'rerank' },
      ],
    });
    expect(result.current.some((field) => field.name === 'features')).toBe(
      false,
    );
  });

  it('submits the selected embedding and rerank models with the API key', () => {
    const values = {
      instance_name: 'xfyun-retrieval',
      api_key: 'test-key',
      base_url: 'https://maas-api.cn-huabei-1.xf-yun.com/v2',
      model_info: [
        { model_name: 'xop3qwen8bembedding', model_type: ['embedding'] },
        { model_name: 'xop3qwen8breranker', model_type: ['rerank'] },
      ],
    };

    expect(config.verifyTransform?.(values)).toMatchObject({
      apiKey: values.api_key,
      baseUrl: values.base_url,
      modelInfo: values.model_info,
    });
    expect(config.submitTransform?.(values)).toMatchObject({
      instance_name: values.instance_name,
      llm_factory: LLMFactory.XunFeiRetrieval,
      api_key: values.api_key,
      base_url: values.base_url,
      model_info: values.model_info,
    });
  });
});
