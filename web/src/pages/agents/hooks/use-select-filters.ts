import { FilterCollection } from '@/components/list-filter-bar/interface';
import { AgentCategory } from '@/constants/agent';
import {
  useFetchAgentFilters,
  useFetchAgentTags,
} from '@/hooks/use-agent-request';
import { AgentListItemType } from '@/interfaces/database/agent';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export function useSelectFilters() {
  const { t } = useTranslation();
  const { data: agentFilters } = useFetchAgentFilters();
  const { data: tagCounts } = useFetchAgentTags();

  const tagList = useMemo(
    () =>
      (tagCounts ?? []).map((t) => ({
        id: t.tag,
        label: t.tag,
        count: t.count,
      })),
    [tagCounts],
  );

  const filters: FilterCollection[] = [
    {
      field: 'owner',
      list: agentFilters?.owner,
      label: t('common.owner'),
    },
    {
      field: 'canvasCategory',
      list: [
        {
          id: AgentCategory.DataflowCanvas,
          label: t('flow.tabList.ingestionPipeline'),
          count:
            agentFilters?.canvas_category.find(
              (item) => item.id === AgentCategory.DataflowCanvas,
            )?.count ?? 0,
        },
        {
          id: AgentListItemType.CompilationTemplateGroup,
          label: t('flow.tabList.compilationOperator'),
          count:
            agentFilters?.canvas_category.find(
              (item) => item.id === AgentListItemType.CompilationTemplateGroup,
            )?.count ?? 0,
        },
        {
          id: AgentCategory.AgentCanvas,
          label: t('flow.tabList.workflow'),
          count:
            agentFilters?.canvas_category.find(
              (item) => item.id === AgentCategory.AgentCanvas,
            )?.count ?? 0,
        },
      ],
      label: t('flow.canvasCategory'),
    },
    {
      field: 'tags',
      list: tagList,
      label: t('flow.tags'),
    },
  ];

  return filters;
}
