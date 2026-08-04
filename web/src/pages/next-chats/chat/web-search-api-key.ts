import { WebSearchProvider } from '@/constants/chat';
import type { PromptConfig } from '@/interfaces/database/chat';

export function getWebSearchProvider(promptConfig?: PromptConfig) {
  const provider = promptConfig?.web_search_provider;

  if (
    provider === WebSearchProvider.Tavily ||
    provider === WebSearchProvider.Querit
  ) {
    return provider;
  }

  if (
    provider === undefined &&
    typeof promptConfig?.tavily_api_key === 'string' &&
    promptConfig.tavily_api_key.trim()
  ) {
    return WebSearchProvider.Tavily;
  }

  return undefined;
}

export function getWebSearchApiKey(promptConfig?: PromptConfig) {
  const provider = getWebSearchProvider(promptConfig);
  let apiKey: unknown;

  switch (provider) {
    case WebSearchProvider.Tavily:
      apiKey = promptConfig?.tavily_api_key;
      break;
    case WebSearchProvider.Querit:
      apiKey = promptConfig?.querit_api_key;
      break;
    default:
      return undefined;
  }

  return typeof apiKey === 'string' ? apiKey.trim() : undefined;
}
