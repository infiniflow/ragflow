export type ChatPromptKbIssue = {
  path: string[];
  message: string;
};

export type ChatPromptKbValue = {
  dataset_ids?: string[];
  prompt_config?: {
    system?: string;
    empty_response?: string;
  };
};

export function chatPromptKbIssues(
  value: ChatPromptKbValue,
  t: (key: string) => string,
): ChatPromptKbIssue[] {
  const system = value?.prompt_config?.system ?? '';
  const emptyResponse = value?.prompt_config?.empty_response ?? '';
  const datasetIds = value?.dataset_ids ?? [];
  const issues: ChatPromptKbIssue[] = [];

  if (
    datasetIds.length > 0 &&
    system.trim() !== '' &&
    !system.includes('{knowledge}')
  ) {
    issues.push({
      path: ['prompt_config', 'system'],
      message: t('knowledgePlaceholderMessage'),
    });
  }

  if (datasetIds.length === 0 && emptyResponse.trim() !== '') {
    issues.push({
      path: ['prompt_config', 'empty_response'],
      message: t('emptyResponseMessage'),
    });
  }

  return issues;
}
