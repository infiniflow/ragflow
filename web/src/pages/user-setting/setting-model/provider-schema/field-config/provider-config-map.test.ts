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

describe('MinerU provider configuration', () => {
  const config = ProviderConfigMap[LLMFactory.MinerU];

  it('stores the mineru.net token inside api_key', () => {
    const values = {
      instance_name: 'mineru',
      mineru_apiserver: 'https://mineru.net',
      mineru_api_token: 'sk-ui-token',
      mineru_backend: 'pipeline',
      mineru_delete_output: true,
    };

    expect(
      config.fields.find((field) => field.name === 'mineru_api_token'),
    ).toMatchObject({ type: 'password', required: false });

    const saved = config.submitTransform?.(values);
    expect(saved?.api_key).toMatchObject({
      mineru_apiserver: 'https://mineru.net',
      mineru_api_token: 'sk-ui-token',
      mineru_backend: 'pipeline',
      mineru_delete_output: '1',
    });
    expect(saved).not.toHaveProperty('mineru_api_token');
    expect(
      config.echoTransform?.({
        api_key_config: saved?.api_key,
        base_url: 'https://mineru.net',
      }),
    ).toMatchObject({ mineru_api_token: 'sk-ui-token' });
  });

  it('does not leak edit-mode instance metadata into api_key', () => {
    const saved = config.submitTransform?.({
      instance_name: 'mineru',
      mineru_apiserver: 'http://172.16.0.119:8000',
      mineru_api_token: '',
      mineru_output_dir: '',
      mineru_backend: 'standard',
      mineru_delete_output: true,
      api_key: '',
      api_key_configured: true,
      api_key_config: { mineru_apiserver: 'http://172.16.0.119:8000' },
      credential_status: { mineru_api_token: true },
      credential_masks: { mineru_api_token: '********' },
      base_url: 'http://172.16.0.119:8000',
      region: 'default',
      model_info: [
        {
          model_name: 'MinerU-model',
          model_type: ['ocr'],
          max_tokens: 8192,
        },
      ],
    });

    expect(saved?.api_key).toEqual({
      mineru_apiserver: 'http://172.16.0.119:8000',
      mineru_api_token: '',
      mineru_output_dir: '',
      mineru_backend: 'standard',
      mineru_delete_output: '1',
    });
    expect(JSON.stringify(saved?.api_key).length).toBeLessThan(512);
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
