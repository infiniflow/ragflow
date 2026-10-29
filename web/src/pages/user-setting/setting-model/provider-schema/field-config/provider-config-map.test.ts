import { LLMFactory } from '@/constants/llm';
import { ProviderConfigMap } from './provider-config-map';

jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: {
    Password: 'password',
    Switch: 'switch',
    Text: 'text',
  },
}));

describe('Azure OpenAI provider configuration', () => {
  const config = ProviderConfigMap[LLMFactory.AzureOpenAI];

  it('defaults the API version to one that supports streaming usage', () => {
    expect(
      config.fields.find((field) => field.name === 'api_version'),
    ).toMatchObject({ defaultValue: '2024-10-21' });
  });

  it('verifies with the same credentials it saves', () => {
    const values = {
      instance_name: 'azure',
      api_key: 'test-key',
      api_version: '2024-02-01',
      base_url: 'https://example.openai.azure.com/openai',
    };
    const credentials = { api_key: 'test-key', api_version: '2024-02-01' };

    expect(config.verifyTransform?.(values)).toMatchObject({
      apiKey: credentials,
    });
    expect(config.submitTransform?.(values)).toMatchObject({
      api_key: credentials,
    });
  });

  it('sends the plain key when no API version is set', () => {
    const values = {
      instance_name: 'azure',
      api_key: 'test-key',
      api_version: '',
      base_url: 'https://example.openai.azure.com/openai',
    };

    expect(config.verifyTransform?.(values)).toMatchObject({
      apiKey: 'test-key',
    });
    expect(config.submitTransform?.(values)).toMatchObject({
      api_key: 'test-key',
    });
  });
});

describe('provider instance submit transforms', () => {
  it.each([
    [
      LLMFactory.GoogleCloud,
      {
        google_project_id: 'project-1',
        google_region: 'us-central1',
        google_service_account_key: '{"type":"service_account"}',
      },
    ],
    [
      LLMFactory.TencentCloud,
      {
        TencentCloud_sid: 'sid-1',
        TencentCloud_sk: 'secret-1',
      },
    ],
    [
      LLMFactory.FishAudio,
      {
        fish_audio_ak: 'key-1',
        fish_audio_refid: 'voice-1',
      },
    ],
  ])('%s places provider settings in api_key', (provider, settings) => {
    const payload = ProviderConfigMap[provider].submitTransform?.({
      instance_name: 'test-instance',
      ...settings,
    });

    expect(payload?.api_key).toEqual(settings);
    for (const field of Object.keys(settings)) {
      expect(payload).not.toHaveProperty(field);
    }
  });
});
