import { WebSearchProvider } from '@/constants/chat';
import type { PromptConfig } from '@/interfaces/database/chat';
import {
  getWebSearchApiKey,
  getWebSearchProvider,
  hasWebSearchProvider,
} from './web-search-api-key';

describe('getWebSearchProvider', () => {
  it('does not select a provider for a new unconfigured dialog', () => {
    expect(getWebSearchProvider({} as PromptConfig)).toBeUndefined();
  });

  it('selects Tavily for a legacy dialog with a Tavily key', () => {
    const promptConfig = {
      tavily_api_key: 'tvly-test',
    } as PromptConfig;

    expect(getWebSearchProvider(promptConfig)).toBe(WebSearchProvider.Tavily);
  });
});

describe('getWebSearchApiKey', () => {
  it('uses Tavily for dialogs saved before provider selection existed', () => {
    const promptConfig = {
      tavily_api_key: 'tvly-test',
    } as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBe('tvly-test');
  });

  it('uses only the selected Querit key', () => {
    const promptConfig = {
      web_search_provider: WebSearchProvider.Querit,
      querit_api_key: 'querit-test',
      tavily_api_key: 'tvly-test',
    } as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBe('querit-test');
  });

  it('uses only the selected Serply key', () => {
    const promptConfig = {
      web_search_provider: WebSearchProvider.Serply,
      serply_api_key: 'serply-test',
      tavily_api_key: 'tvly-test',
    } as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBe('serply-test');
  });

  it('does not fall back to Tavily when Serply is selected without a key', () => {
    const promptConfig = {
      web_search_provider: WebSearchProvider.Serply,
      tavily_api_key: 'tvly-test',
    } as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBeUndefined();
  });

  it('does not fall back to Tavily when Querit is selected without a key', () => {
    const promptConfig = {
      web_search_provider: WebSearchProvider.Querit,
      tavily_api_key: 'tvly-test',
    } as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBeUndefined();
  });

  it('treats a whitespace-only key as unconfigured', () => {
    const promptConfig = {
      web_search_provider: WebSearchProvider.Querit,
      querit_api_key: '   ',
    } as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBe('');
  });

  it('does not fall back to Tavily for an unsupported provider', () => {
    const promptConfig = {
      web_search_provider: 'unsupported',
      tavily_api_key: 'tvly-test',
    } as unknown as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBeUndefined();
  });

  it('treats a non-string key as unconfigured', () => {
    const promptConfig = {
      web_search_provider: WebSearchProvider.Querit,
      querit_api_key: 123,
    } as unknown as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBeUndefined();
  });
});

describe('hasWebSearchProvider', () => {
  it('is false for a new unconfigured dialog', () => {
    expect(hasWebSearchProvider({} as PromptConfig)).toBe(false);
  });

  it('requires a key for providers that need one', () => {
    expect(
      hasWebSearchProvider({
        web_search_provider: WebSearchProvider.Querit,
      } as PromptConfig),
    ).toBe(false);

    expect(
      hasWebSearchProvider({
        web_search_provider: WebSearchProvider.Querit,
        querit_api_key: 'querit-test',
      } as PromptConfig),
    ).toBe(true);
  });

  it('is true for keyless You.com with no key configured', () => {
    expect(
      hasWebSearchProvider({
        web_search_provider: WebSearchProvider.YouCom,
      } as PromptConfig),
    ).toBe(true);

    expect(
      hasWebSearchProvider({
        web_search_provider: WebSearchProvider.YouCom,
        youcom_api_key: '',
      } as PromptConfig),
    ).toBe(true);
  });
});

describe('You.com key selection', () => {
  it('uses only the selected You.com key', () => {
    const promptConfig = {
      web_search_provider: WebSearchProvider.YouCom,
      youcom_api_key: 'ydc-test',
      tavily_api_key: 'tvly-test',
    } as PromptConfig;

    expect(getWebSearchProvider(promptConfig)).toBe(WebSearchProvider.YouCom);
    expect(getWebSearchApiKey(promptConfig)).toBe('ydc-test');
  });
});
