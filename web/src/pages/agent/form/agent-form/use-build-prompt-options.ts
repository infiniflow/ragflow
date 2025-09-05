import { useFetchPrompt } from '@/hooks/use-agent-request';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export const PromptIdentity = 'RAGFlow-Prompt';

function wrapPromptWithTag(text: string, tag: string) {
  const capitalTag = tag.toUpperCase();
  return `<${capitalTag}>
  ${text}
</${capitalTag}>`;
}

export function useBuildPromptExtraPromptOptions() {
  const { data: prompts } = useFetchPrompt();
  const { t } = useTranslation();

  const options = useMemo(() => {
    return Object.entries(prompts || {}).map(([key, value]) => ({
      label: key,
      value: wrapPromptWithTag(value, key),
    }));
  }, [prompts]);

  const extraOptions = [
    { label: PromptIdentity, title: t('flow.frameworkPrompts'), options },
  ];

  return { extraOptions };
}
