import { LLMFactory } from '@/constants/llm';
import { GenericApiKeyConfig } from './generic-api-key-config';

jest.mock('@/components/dynamic-form', () => ({
  FormFieldType: {
    Password: 'password',
    Text: 'text',
  },
}));

describe('GenericApiKeyConfig base_url placeholder', () => {
  const placeholder = GenericApiKeyConfig.fields.find(
    (field) => field.name === 'base_url',
  )?.placeholder as (factory?: string) => string;

  it.each([
    [LLMFactory.Anthropic, 'anthropicBaseUrlPlaceholder'],
    [LLMFactory.MiniMax, 'minimaxBaseUrlPlaceholder'],
    [LLMFactory.OpenAI, 'openaiBaseUrlPlaceholder'],
  ])('uses the %s placeholder', (factory, expected) => {
    expect(placeholder(factory)).toBe(expected);
  });
});
