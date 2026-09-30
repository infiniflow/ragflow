import {
  KEYLESS_WEB_SEARCH_PROVIDERS,
  WebSearchProvider,
} from '@/constants/chat';

/**
 * The subset of a dialog's prompt_config these helpers read — the web-search
 * provider and its per-provider key slots.
 *
 * Structural rather than PromptConfig because the save-time form schema allows
 * web_search_provider to be '' (a cleared select), which PromptConfig does not;
 * with this shape the schema can call the helpers without casting.
 */
export type WebSearchKeyConfig = {
  web_search_provider?: WebSearchProvider | '';
  tavily_api_key?: string;
};

const WEB_SEARCH_PROVIDERS: WebSearchProvider[] =
  Object.values(WebSearchProvider);

/**
 * Resolves the active web search provider from the dialog's prompt configuration,
 * including backward-compatibility fallback to Tavily for legacy dialogs.
 */
export function getWebSearchProvider(promptConfig?: WebSearchKeyConfig) {
  const provider = promptConfig?.web_search_provider;

  // Derived from the enum instead of a hand-written union: a provider added to the
  // enum — and therefore to the dropdown — used to be silently treated as
  // unconfigured here until someone remembered to extend the list too, which
  // disabled its Internet switch with no error.
  if (
    provider !== undefined &&
    provider !== '' &&
    WEB_SEARCH_PROVIDERS.includes(provider)
  ) {
    return provider;
  }

  // undefined means the dialog predates provider selection, which is where the
  // legacy Tavily key comes in. '' is an explicit "no provider" (cleared select)
  // and must NOT fall back to Tavily.
  if (
    provider === undefined &&
    typeof promptConfig?.tavily_api_key === 'string' &&
    promptConfig.tavily_api_key.trim()
  ) {
    return WebSearchProvider.Tavily;
  }

  return undefined;
}

// The prompt_config field each provider reads its key from. Kept as the single
// source of truth: the form uses it for the required marker, the schema uses it
// for validation, and the reader below uses it to fetch the value — three
// callers that used to each spell the mapping out again.
const webSearchApiKeyFields: Record<WebSearchProvider, string> = {
  [WebSearchProvider.Brave]: 'brave_api_key',
  [WebSearchProvider.Exa]: 'exa_api_key',
  [WebSearchProvider.Firecrawl]: 'firecrawl_api_key',
  [WebSearchProvider.Linkup]: 'linkup_api_key',
  [WebSearchProvider.Parallel]: 'parallel_api_key',
  [WebSearchProvider.Querit]: 'querit_api_key',
  [WebSearchProvider.SerpApi]: 'serpapi_api_key',
  [WebSearchProvider.Serply]: 'serply_api_key',
  [WebSearchProvider.Tavily]: 'tavily_api_key',
  [WebSearchProvider.YouCom]: 'youcom_api_key',
};

/**
 * Normalizes a provider value, converting cleared select values ('') to undefined.
 */
function normalizeProvider(provider?: WebSearchProvider | '') {
  return provider === '' ? undefined : provider;
}

/**
 * Reads a dynamic prompt_config field at runtime by property name.
 */
function fieldValue(
  promptConfig: WebSearchKeyConfig | undefined,
  field: string,
) {
  return (promptConfig as unknown as Record<string, unknown> | undefined)?.[
    field
  ];
}

/**
 * Returns the prompt_config field name corresponding to the specified provider's API key.
 */
export function getWebSearchApiKeyField(provider?: WebSearchProvider | '') {
  const normalized = normalizeProvider(provider);
  return normalized ? webSearchApiKeyFields[normalized] : undefined;
}

/**
 * Determines whether the specified provider requires an API key for operation.
 */
export function isWebSearchApiKeyRequired(provider?: WebSearchProvider | '') {
  const normalized = normalizeProvider(provider);
  return (
    normalized !== undefined &&
    !KEYLESS_WEB_SEARCH_PROVIDERS.includes(normalized)
  );
}

/**
 * Retrieves the configured and trimmed API key for the selected web search provider.
 */
export function getWebSearchApiKey(promptConfig?: WebSearchKeyConfig) {
  const keyField = getWebSearchApiKeyField(getWebSearchProvider(promptConfig));
  if (!keyField) {
    return undefined;
  }

  const apiKey = fieldValue(promptConfig, keyField);
  return typeof apiKey === 'string' ? apiKey.trim() : undefined;
}

/**
 * Returns the prompt_config field of the selected provider's API key when the
 * provider requires one and it is missing or blank — the reason a save has to be
 * rejected. A keyed provider without a key fails SILENTLY at runtime (the Internet
 * switch never appears), which is why the form blocks it up front. undefined means
 * the configuration is acceptable.
 *
 * Pure and exported so the save-time rule is testable without rendering the form.
 */
export function missingWebSearchApiKeyField(promptConfig?: WebSearchKeyConfig) {
  const provider = getWebSearchProvider(promptConfig);
  const keyField = getWebSearchApiKeyField(provider);
  if (!keyField || !isWebSearchApiKeyRequired(provider)) {
    return undefined;
  }

  const apiKey = fieldValue(promptConfig, keyField);
  return typeof apiKey === 'string' && apiKey.trim() ? undefined : keyField;
}

/**
 * Whether web search is usable as configured. Most providers need a key; a
 * keyless provider is usable as soon as it is selected.
 */
export function hasWebSearchProvider(promptConfig?: WebSearchKeyConfig) {
  const provider = getWebSearchProvider(promptConfig);

  if (provider === undefined) {
    return false;
  }
  if (KEYLESS_WEB_SEARCH_PROVIDERS.includes(provider)) {
    return true;
  }

  return Boolean(getWebSearchApiKey(promptConfig));
}
