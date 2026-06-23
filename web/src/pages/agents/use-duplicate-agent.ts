import message from '@/components/ui/message';
import { useSetAgent } from '@/hooks/use-agent-request';
import { IFlow } from '@/interfaces/database/agent';
import agentService from '@/services/agent-service';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

const MAX_SUFFIX = 100;

function buildCopyTitle(base: string, taken: Set<string>) {
  if (!taken.has(base)) return base;
  for (let i = 2; i <= MAX_SUFFIX; i++) {
    const candidate = `${base} (${i})`;
    if (!taken.has(candidate)) return candidate;
  }
  return `${base} (${Date.now()})`;
}

export function useDuplicateAgent() {
  const { t } = useTranslation();
  const { setAgent, loading } = useSetAgent(false);

  const duplicateAgent = useCallback(
    async (agent: Pick<IFlow, 'id' | 'title' | 'canvas_category'>) => {
      try {
        const { data: detailResp } = await agentService.getAgent(agent.id);
        const detail: IFlow | undefined = detailResp?.data;
        if (!detail?.dsl) {
          message.error(t('flow.duplicateFailed'));
          return;
        }

        const baseTitle = `${t('flow.copyOf')} ${agent.title}`;
        const listParams: Record<string, unknown> = {
          keywords: baseTitle,
          page: 1,
          page_size: 200,
        };
        if (agent.canvas_category) {
          listParams.canvas_category = agent.canvas_category;
        }
        const { data: listResp } = await agentService.listAgents(
          { params: listParams },
          true,
        );
        const existing: { title: string }[] = listResp?.data?.canvas ?? [];
        const taken = new Set(existing.map((a) => a.title));
        const newTitle = buildCopyTitle(baseTitle, taken);

        const result = await setAgent({
          title: newTitle,
          dsl: detail.dsl,
          canvas_category: detail.canvas_category ?? agent.canvas_category,
          avatar: detail.avatar,
          description: detail.description,
        });

        if (result?.code === 0) {
          message.success(t('flow.duplicated', { title: newTitle }));
        } else {
          message.error(result?.message || t('flow.duplicateFailed'));
        }
      } catch (err) {
        console.error('duplicateAgent error', err);
        message.error(t('flow.duplicateFailed'));
      }
    },
    [setAgent, t],
  );

  return { duplicateAgent, loading };
}
