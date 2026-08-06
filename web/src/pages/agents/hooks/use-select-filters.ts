import { FilterCollection } from '@/components/list-filter-bar/interface';
import { AgentCategory } from '@/constants/agent';
import {
  useFetchAgentList,
  useFetchAgentTags,
} from '@/hooks/use-agent-request';
import { AgentListItemType, IFlow } from '@/interfaces/database/agent';
import { buildOwnersFilter } from '@/utils/list-filter-util';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export function useSelectFilters() {
  const { t } = useTranslation();
  const { data } = useFetchAgentList({});
  const { data: tagCounts } = useFetchAgentTags();

  // The merged /agents list also contains compilation template groups, which
  // have no owner fields — drop them before building the owner filter.
  const agents = useMemo(() => {
    const canvas = (data?.canvas ?? []) as Array<
      IFlow & { type?: AgentListItemType }
    >;
    return canvas.filter(
      (x) => x.type !== AgentListItemType.CompilationTemplateGroup,
    );
  }, [data?.canvas]);

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
    buildOwnersFilter(agents, undefined, t('common.owner')),
    {
      field: 'canvasCategory',
      list: [
        {
          id: AgentCategory.DataflowCanvas,
          label: t('flow.tabList.ingestionPipeline'),
        },
        {
          id: AgentListItemType.CompilationTemplateGroup,
          label: t('flow.tabList.compilationOperator'),
        },
        {
          id: AgentCategory.AgentCanvas,
          label: t('flow.tabList.workflow'),
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
