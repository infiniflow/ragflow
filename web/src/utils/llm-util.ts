import { IThirdOAIModel } from '@/interfaces/database/llm';

export const getLLMIconName = (fid: string, llm_name: string) => {
  if (fid === 'FastEmbed') {
    return llm_name.split('/').at(0) ?? '';
  }

  return fid;
};

export const getLlmNameAndFIdByLlmId = (llmId?: string) => {
  const [llmName, fId] = llmId?.split('@') || [];

  return { fId, llmName };
};

// The names of the large models returned by the interface are similar to "deepseek-r1___OpenAI-API"
export function getRealModelName(llmName: string) {
  return llmName.split('__').at(0) ?? '';
}

export function buildLlmUuid(llm: IThirdOAIModel) {
  return `${llm.llm_name}@${llm.fid}`;
}
