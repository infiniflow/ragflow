import { LocalLlmFactories } from './constants';

export const isLocalLlmFactory = (llmFactory: string) =>
  LocalLlmFactories.some((x) => x === llmFactory);
