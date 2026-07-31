import { useFetchPrompt } from '@/hooks/use-agent-request';
import { Edge } from '@xyflow/react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { hasSubAgentOrTool } from '../../utils';

export const PromptIdentity = 'RAGFlow-Prompt';

function wrapPromptWithTag(text: string, tag: string) {
  const capitalTag = tag.toUpperCase();
  return `<${capitalTag}>
  ${text}
</${capitalTag}>`;
}

export function useBuildPromptExtraPromptOptions(
  edges: Edge[],
  nodeId?: string,
) {
  const { data: prompts } = useFetchPrompt();
  const { t } = useTranslation();
  const has = hasSubAgentOrTool(edges, nodeId);

  const options = useMemo(() => {
    return Object.entries(prompts || {})
      .map(([key, value]) => ({
        label: key,
        value: wrapPromptWithTag(value, key),
        icon: null,
      }))
      .filter((x) => {
        if (!has) {
          return x.label === 'citation_guidelines';
        }
        return true;
      });
  }, [has, prompts]);

  const extraOptions = [
    { label: PromptIdentity, title: t('flow.frameworkPrompts'), options },
  ];

  return { extraOptions };
}
