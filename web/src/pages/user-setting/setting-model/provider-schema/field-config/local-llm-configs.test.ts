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
