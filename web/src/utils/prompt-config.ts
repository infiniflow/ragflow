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

const ensureKnowledgeParameter = (parameters?: Parameter[]): Parameter[] => {
  const list = Array.isArray(parameters) ? parameters : [];
  if (list.some((p) => p?.key === KNOWLEDGE_KEY)) {
    return list;
  }
  return [...list, { key: KNOWLEDGE_KEY, optional: false }];
};

const ensureKnowledgePlaceholder = (system?: string): string => {
  if (!system || system.includes(KNOWLEDGE_PLACEHOLDER)) {
    return system ?? KNOWLEDGE_PLACEHOLDER;
  }
  return `${system.trim()}\n\n${KNOWLEDGE_PLACEHOLDER}`;
};

/**
 * Synchronises the prompt config with the current knowledge-base state:
 * - KB present  -> ensure {knowledge} placeholder + parameter exist
 * - KB absent   -> strip {knowledge} placeholder + parameter
 */
export const sanitizePromptConfigForKnowledge = (
  promptConfig: PromptConfig,
  kbIds?: string[],
): PromptConfig => {
  const hasKb = Array.isArray(kbIds) && kbIds.length > 0;
  const hasTavilyKey = Boolean(promptConfig.tavily_api_key?.trim());

  if (hasKb || hasTavilyKey) {
    return {
      ...promptConfig,
      system: ensureKnowledgePlaceholder(promptConfig.system),
      parameters: ensureKnowledgeParameter(promptConfig.parameters),
    };
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
