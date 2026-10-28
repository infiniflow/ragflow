import { Operator } from '@/constants/agent';
import { cloneDeepWith, get, isPlainObject } from 'lodash';

const apiKeyOperators = [
  Operator.TavilySearch,
  Operator.TavilyExtract,
  Operator.Google,
  Operator.KeenableSearch,
  Operator.YouComSearch,
  Operator.SofyaSearch,
  Operator.BGPT,
  Operator.QueritContents,
  Operator.QueritSearch,
];

// Canvas nodes carry the operator under `data.label` and the key under
// `data.form.api_key`, so a graph export has to be sanitized separately from the
// agent-tool records above.
const nodeLabelApiKeyOperators: string[] = [
  Operator.YouComSearch,
  Operator.SofyaSearch,
];

const WebhookSecretFields = {
  token: 'token_value',
  basic_auth: 'password',
  jwt: 'secret',
  hmac: 'secret',
} as const;

function isQueritOperator(value: unknown) {
  if (typeof value !== 'string') {
    return false;
  }

  return ['querit', 'queritcontents', 'queritsearch'].includes(
    value.replace(/_/g, '').toLowerCase(),
  );
}

function isSearch1APIOperator(value: unknown) {
  if (typeof value !== 'string') {
    return false;
  }

  return ['search1api', 'search1apisearch', 'search1apicrawl'].includes(
    value.replace(/_/g, '').toLowerCase(),
  );
}

// The backend registers the tool as `perplexity` with the canvas alias
// `perplexitysearch`, and the model-facing name is `perplexity_search`, so
// every spelling has to be scrubbed, not only the canonical operator. The
// backend also trims component names before resolving them, so padded
// spellings import fine and must be scrubbed too.
function isPerplexityOperator(value: unknown) {
  if (typeof value !== 'string') {
    return false;
  }

  return ['perplexity', 'perplexitysearch'].includes(
    value.trim().replace(/_/g, '').toLowerCase(),
  );
}

// Mirrors agentToolObject on the backend: an embedded tool record is named by
// component_name, then tool_name, then name, and a blank value falls through
// to the next field. Imported DSL can use any of them.
function embeddedToolName(value: Record<string, unknown>) {
  for (const field of ['component_name', 'tool_name', 'name']) {
    const candidate = value[field];
    if (typeof candidate === 'string' && candidate.trim()) {
      return candidate.trim();
    }
  }
  return undefined;
}

function isNodeLabelApiKeyOperator(value: unknown) {
  if (typeof value !== 'string') {
    return false;
  }

  return (
    isQueritOperator(value) ||
    isSearch1APIOperator(value) ||
    isPerplexityOperator(value) ||
    nodeLabelApiKeyOperators.includes(value)
  );
}

/** Returns a copy with recognized webhook credentials blanked. */
function clearWebhookSecrets(value: Record<string, any>) {
  const security = value.security;
  if (!isPlainObject(security)) {
    return;
  }

  let changed = false;
  const sanitizedSecurity = { ...security };
  for (const [section, field] of Object.entries(WebhookSecretFields)) {
    const auth = sanitizedSecurity[section];
    if (isPlainObject(auth) && auth[field]) {
      sanitizedSecurity[section] = { ...auth, [field]: '' };
      changed = true;
    }
  }

  if (changed) {
    return { ...value, security: sanitizedSecurity };
  }
}

/** Returns a copy of the DSL with recognized credentials removed. */
export function clearSensitiveFields<T>(obj: T): T {
  return cloneDeepWith(obj, (value) => {
    if (!isPlainObject(value)) {
      return;
    }

    if (
      (apiKeyOperators.includes(value.component_name) ||
        isQueritOperator(value.component_name) ||
        isSearch1APIOperator(value.component_name) ||
        isPerplexityOperator(embeddedToolName(value))) &&
      get(value, 'params.api_key')
    ) {
      return { ...value, params: { ...value.params, api_key: '' } };
    }

    if (
      isNodeLabelApiKeyOperator(get(value, 'data.label')) &&
      get(value, 'data.form.api_key')
    ) {
      return {
        ...value,
        data: {
          ...value.data,
          form: {
            ...value.data.form,
            api_key: '',
          },
        },
      };
    }

    const sanitizedWebhookForm = clearWebhookSecrets(value);
    if (sanitizedWebhookForm) {
      return sanitizedWebhookForm;
    }
  });
}
