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

export const sanitizePromptConfigForKnowledge = (
  promptConfig: PromptConfig,
  kbIds?: string[],
): PromptConfig => {
  const hasKb = Array.isArray(kbIds) && kbIds.length > 0;
  const hasTavilyKey = Boolean(promptConfig.tavily_api_key?.trim());
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
