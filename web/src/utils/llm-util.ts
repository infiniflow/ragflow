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
