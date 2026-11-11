import { LLMFactory } from '@/constants/llm';
import { LIST_MODEL_PROVIDERS } from '../constants';
import { LocalLlmConfigs } from './local-llm-configs';

jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: {
    Password: 'password',
    Switch: 'switch',
    Text: 'text',
  },
}));

describe('FunASR local provider configuration', () => {
  it('uses the model picker backed by the FunASR models endpoint', () => {
    expect(LIST_MODEL_PROVIDERS.has(LLMFactory.FunASR)).toBe(true);
  });

  it('registers an optional-key local endpoint with the FunASR default URL', () => {
    const config = LocalLlmConfigs[LLMFactory.FunASR];

    expect(config).toMatchObject({
      llmFactory: LLMFactory.FunASR,
      title: 'FunASR',
      docLink: 'https://github.com/modelscope/FunASR',
    });
    expect(
      config.fields.find((field) => field.name === 'base_url'),
    ).toMatchObject({
      required: true,
      defaultValue: 'http://localhost:8000/v1',
    });
    expect(
      config.fields.find((field) => field.name === 'api_key'),
    ).toMatchObject({
      required: false,
    });
  });
});

describe('MWS provider configuration', () => {
  it('uses dynamic model discovery', () => {
    expect(LIST_MODEL_PROVIDERS.has(LLMFactory.MWS)).toBe(true);
  });

  it('requires a project API URL and Token', () => {
    const config = LocalLlmConfigs[LLMFactory.MWS];

    expect(config).toMatchObject({
      llmFactory: LLMFactory.MWS,
      title: 'MWS',
      docLink:
        'https://mws.ru/docs/cloud-platform/gpt/general/inference-text.html',
    });
    expect(
      config.fields.find((field) => field.name === 'instance_name'),
    ).toMatchObject({ label: 'instanceName', required: true });
    expect(
      config.fields.find((field) => field.name === 'base_url'),
    ).toMatchObject({ label: 'mwsApiUrl', required: true });
    expect(
      config.fields.find((field) => field.name === 'api_key'),
    ).toMatchObject({ label: 'mwsToken', required: true });
    expect(
      config.fields.some((field) => field.name === 'provider_order'),
    ).toBe(false);

    expect(
      config.submitTransform?.({
        instance_name: 'mws-project',
        base_url: 'https://gpt.mwsapis.ru/projects/demo',
        api_key: 'token',
        model_info: [],
      }),
    ).toMatchObject({
      instance_name: 'mws-project',
      llm_factory: LLMFactory.MWS,
      base_url: 'https://gpt.mwsapis.ru/projects/demo',
      api_key: 'token',
    });
  });
});
