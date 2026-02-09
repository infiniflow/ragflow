import { Parameter, PromptConfig } from '@/interfaces/database/chat';

const KNOWLEDGE_KEY = 'knowledge';
const KNOWLEDGE_PLACEHOLDER = '{knowledge}';

const stripKnowledgeLines = (system: string) => {
  const withoutKnowledge = system.split(KNOWLEDGE_PLACEHOLDER).join('');
  return withoutKnowledge
    .replace(/[ \t]+\n/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
};

const removeKnowledgeParameter = (parameters?: Parameter[]) => {
  if (!Array.isArray(parameters)) {
    return [];
  }
  return parameters.filter((parameter) => parameter?.key !== KNOWLEDGE_KEY);
};

/**
 * Sanitise the prompt config before sending it to the backend:
 * when no KB / Tavily is configured, strip the {knowledge} placeholder
 * and its parameter entry so the backend validation does not reject the
 * request.  When a KB IS configured the prompt is left untouched — the
 * user is responsible for placing {knowledge} where they want it.
 */
export const sanitizePromptConfigForKnowledge = (
  promptConfig: PromptConfig,
  kbIds?: string[],
): PromptConfig => {
  const hasKb = Array.isArray(kbIds) && kbIds.length > 0;
  const hasTavilyKey = Boolean(promptConfig.tavily_api_key?.trim());

  // Nothing to strip when a knowledge source is configured.
  if (hasKb || hasTavilyKey) {
    return promptConfig;
  }

  const nextSystem = promptConfig.system?.includes(KNOWLEDGE_PLACEHOLDER)
    ? stripKnowledgeLines(promptConfig.system)
    : promptConfig.system;

  return {
    ...promptConfig,
    system: nextSystem ?? '',
    parameters: removeKnowledgeParameter(promptConfig.parameters),
  };
};
