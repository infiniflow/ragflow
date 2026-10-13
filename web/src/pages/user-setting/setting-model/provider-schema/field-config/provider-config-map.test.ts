import { LLMFactory } from '@/constants/llm';
import { ProviderConfigMap } from './provider-config-map';

jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: {
    Password: 'password',
    Select: 'select',
    Switch: 'switch',
    Text: 'text',
  },
}));

describe('MinerU provider submit payload', () => {
  it('serializes api_key as JSON so instance create/verify bind a string', () => {
    const payload = ProviderConfigMap[LLMFactory.MinerU].submitTransform?.({
      instance_name: 'ddd',
      mineru_apiserver: 'http://127.0.0.1:8000',
      mineru_backend: 'standard',
      mineru_delete_output: true,
    });

    expect(typeof payload?.api_key).toBe('string');
    expect(JSON.parse(payload?.api_key as string)).toEqual(
      expect.objectContaining({
        mineru_apiserver: 'http://127.0.0.1:8000',
        mineru_backend: 'standard',
      }),
    );
    expect(payload?.base_url).toBe('http://127.0.0.1:8000');
  });
});
