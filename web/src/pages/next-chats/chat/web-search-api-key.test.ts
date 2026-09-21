import { WebSearchProvider } from '@/constants/chat';
import type { PromptConfig } from '@/interfaces/database/chat';
import {
  getWebSearchApiKey,
  getWebSearchApiKeyField,
  getWebSearchProvider,
  hasWebSearchProvider,
  isWebSearchApiKeyRequired,
  missingWebSearchApiKeyField,
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

// Brave, Exa, Firecrawl, Linkup and Parallel all authenticate with a key and
// have no keyless path, so none of them may show up as "usable" without one.
// Exa has a free tier of 1,000 requests/month, but it is not keyless.
describe('keyed providers added in 2026-09', () => {
  const cases = [
    {
      provider: WebSearchProvider.Brave,
      keyName: 'brave_api_key',
      key: 'brave-test',
    },
    {
      provider: WebSearchProvider.Exa,
      keyName: 'exa_api_key',
      key: 'exa-test',
    },
    {
      provider: WebSearchProvider.Firecrawl,
      keyName: 'firecrawl_api_key',
      key: 'firecrawl-test',
    },
    {
      provider: WebSearchProvider.Linkup,
      keyName: 'linkup_api_key',
      key: 'linkup-test',
    },
    {
      provider: WebSearchProvider.Parallel,
      keyName: 'parallel_api_key',
      key: 'parallel-test',
    },
  ] as const;

  it.each(cases)(
    'reads $provider from $keyName',
    ({ provider, keyName, key }) => {
      const promptConfig = {
        web_search_provider: provider,
        [keyName]: `  ${key}  `,
        tavily_api_key: 'tvly-test',
      } as unknown as PromptConfig;

      expect(getWebSearchProvider(promptConfig)).toBe(provider);
      expect(getWebSearchApiKey(promptConfig)).toBe(key);
      expect(hasWebSearchProvider(promptConfig)).toBe(true);
    },
  );

  it.each(cases)('is unusable when $provider has no key', ({ provider }) => {
    const promptConfig = {
      web_search_provider: provider,
      tavily_api_key: 'tvly-test',
    } as PromptConfig;

    expect(getWebSearchApiKey(promptConfig)).toBeUndefined();
    expect(hasWebSearchProvider(promptConfig)).toBe(false);
  });
});

// The required marker and the save-time validation both read the same two
// helpers, so the field name and the required flag have to agree for every
// provider — a mismatch would mark a field required and then validate a
// different one.
describe('provider key field mapping', () => {
  it('maps every provider to its own prompt_config key', () => {
    expect(getWebSearchApiKeyField(WebSearchProvider.Brave)).toBe(
      'brave_api_key',
    );
    expect(getWebSearchApiKeyField(WebSearchProvider.Exa)).toBe('exa_api_key');
    expect(getWebSearchApiKeyField(WebSearchProvider.Firecrawl)).toBe(
      'firecrawl_api_key',
    );
    expect(getWebSearchApiKeyField(WebSearchProvider.Linkup)).toBe(
      'linkup_api_key',
    );
    expect(getWebSearchApiKeyField(WebSearchProvider.Parallel)).toBe(
      'parallel_api_key',
    );
    expect(getWebSearchApiKeyField(WebSearchProvider.Querit)).toBe(
      'querit_api_key',
    );
    expect(getWebSearchApiKeyField(WebSearchProvider.Serply)).toBe(
      'serply_api_key',
    );
    expect(getWebSearchApiKeyField(WebSearchProvider.Tavily)).toBe(
      'tavily_api_key',
    );
    expect(getWebSearchApiKeyField(WebSearchProvider.YouCom)).toBe(
      'youcom_api_key',
    );
  });

  it('has no key field when no provider is selected', () => {
    expect(getWebSearchApiKeyField(undefined)).toBeUndefined();
    expect(
      getWebSearchApiKeyField('' as unknown as WebSearchProvider),
    ).toBeUndefined();
  });

  it('requires a key for every provider except the keyless ones', () => {
    expect(isWebSearchApiKeyRequired(WebSearchProvider.Brave)).toBe(true);
    expect(isWebSearchApiKeyRequired(WebSearchProvider.Exa)).toBe(true);
    expect(isWebSearchApiKeyRequired(WebSearchProvider.Tavily)).toBe(true);
    expect(isWebSearchApiKeyRequired(WebSearchProvider.YouCom)).toBe(false);
    expect(isWebSearchApiKeyRequired(undefined)).toBe(false);
  });
});

// Exa is NOT keyless despite its free tier: 1,000 requests/month comes with no
// credit card, but every request still carries a key, so a blank field must read
// as "not configured" exactly like the other keyed providers.
describe('Exa key requirement', () => {
  it('is unusable with no key configured', () => {
    expect(
      hasWebSearchProvider({
        web_search_provider: WebSearchProvider.Exa,
      } as PromptConfig),
    ).toBe(false);

    expect(
      hasWebSearchProvider({
        web_search_provider: WebSearchProvider.Exa,
        exa_api_key: '  ',
      } as PromptConfig),
    ).toBe(false);
  });
});

// Clearing the provider emits '' from SelectWithSearch (the schema accepts it), and
// undefined means the same thing. The field lookup and the required marker have to
// agree on that, or the form marks a field required while validating nothing.
describe('cleared provider value', () => {
  const cleared = '' as unknown as WebSearchProvider;

  it('is not a provider for either helper', () => {
    expect(getWebSearchApiKeyField(cleared)).toBeUndefined();
    expect(isWebSearchApiKeyRequired(cleared)).toBe(false);
  });
});

// The save-time rule. A keyed provider without a key fails SILENTLY at runtime —
// the Internet switch never appears — so the schema rejects the save; the rule
// lives in this pure helper so it can be tested without rendering the form.
describe('missingWebSearchApiKeyField', () => {
  it('names the field when a keyed provider has no key', () => {
    expect(
      missingWebSearchApiKeyField({
        web_search_provider: WebSearchProvider.Firecrawl,
      } as PromptConfig),
    ).toBe('firecrawl_api_key');
  });

  it('names the field when the key is only whitespace', () => {
    expect(
      missingWebSearchApiKeyField({
        web_search_provider: WebSearchProvider.Linkup,
        linkup_api_key: '   ',
      } as PromptConfig),
    ).toBe('linkup_api_key');
  });

  it('accepts a keyed provider once its key is set', () => {
    expect(
      missingWebSearchApiKeyField({
        web_search_provider: WebSearchProvider.Parallel,
        parallel_api_key: 'parallel-test',
      } as PromptConfig),
    ).toBeUndefined();
  });

  it('exempts the keyless provider', () => {
    expect(
      missingWebSearchApiKeyField({
        web_search_provider: WebSearchProvider.YouCom,
      } as PromptConfig),
    ).toBeUndefined();
  });

  it('accepts an unconfigured new dialog', () => {
    expect(missingWebSearchApiKeyField({} as PromptConfig)).toBeUndefined();
    expect(missingWebSearchApiKeyField(undefined)).toBeUndefined();
  });

  it('accepts a legacy dialog that only carries a Tavily key', () => {
    expect(
      missingWebSearchApiKeyField({
        tavily_api_key: 'tvly-test',
      } as PromptConfig),
    ).toBeUndefined();
  });

  it('accepts an unknown provider instead of guessing a field', () => {
    expect(
      missingWebSearchApiKeyField({
        web_search_provider: 'unsupported',
        tavily_api_key: 'tvly-test',
      } as unknown as PromptConfig),
    ).toBeUndefined();
  });
});
