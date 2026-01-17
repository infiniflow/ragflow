import { LLMFactory } from '@/constants/llm';
import { LocalLlmFactories } from './constants';

export const isLocalLlmFactory = (llmFactory: string) =>
  LocalLlmFactories.some((x) => x === llmFactory);

export const isDynamicProvider = (llmFactory: string) =>
  [
    LLMFactory.OpenRouter,
    LLMFactory.Xinference,
    LLMFactory.LocalAI,
    LLMFactory.Ollama,
    LLMFactory.LMStudio,
  ].some((x) => x === llmFactory);
