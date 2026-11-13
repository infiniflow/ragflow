import { useMutation, useQueryClient } from '@tanstack/react-query';
import message from '@/components/ui/message';
import { AgentCategory } from '@/constants/agent';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { PipelineVersionKeys } from '@/hooks/use-pipeline-version-request';
import agentService from '@/services/agent-service';
import i18n from '@/locales/config';

// useCopyBuiltinPipeline turns a read-only built-in pipeline into a user-owned
// dataflow canvas: it fetches the built-in DSL via GET /api/v1/pipelines/:id,
// creates a new canvas with that DSL, and opens it for editing. The agents
// list refetches on return; the metadata catalog is invalidated on creation.
export function useCopyBuiltinPipeline() {
  const { navigateToAgent } = useNavigatePage();
  const queryClient = useQueryClient();
  const { mutateAsync, isPending } = useMutation({
    mutationFn: async ({ id, title }: { id: string; title: string }) => {
      try {
        // request (umi-request) resolves to the backend envelope
        // { code, message, data }, so data here is that envelope.
        const { data } = await agentService.getBuiltinPipeline(id);
        const dsl = data?.data?.dsl;
        if (!dsl) {
          message.error(i18n.t('message.requestError'));
          return null;
        }

        const { data: created } = await agentService.createAgent({
          title: i18n.t('flow.copyOfAgentName', {
            name: title,
            defaultValue: `${title} (Copy)`,
          }),
          dsl,
          canvas_category: AgentCategory.DataflowCanvas,
        });

        if (created?.code === 0) {
          queryClient.invalidateQueries({
            queryKey: PipelineVersionKeys.catalog(),
          });
          message.success(i18n.t('message.created'));
          navigateToAgent(created.data.id, AgentCategory.DataflowCanvas)();
          return created;
        }

        message.error(created?.message ?? i18n.t('message.requestError'));
        return null;
      } catch {
        message.error(i18n.t('message.requestError'));
        return null;
      }
    },
  });

  return { copy: mutateAsync, copying: isPending };
}
