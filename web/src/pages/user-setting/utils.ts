import { LLMFactory } from '@/constants/llm';
import { LocalLlmFactories } from './constants';

export const isLocalLlmFactory = (llmFactory: string) =>
  LocalLlmFactories.some((x) => x === llmFactory);

// Dynamic providers fetch their model list from an API at runtime
export const DynamicProviders = [
  LLMFactory.OpenRouter,
  LLMFactory.Xinference,
  LLMFactory.LocalAI,
  LLMFactory.Ollama,
  LLMFactory.LMStudio,
];

export const isDynamicProvider = (llmFactory: string) =>
  DynamicProviders.some((x) => x === llmFactory);
