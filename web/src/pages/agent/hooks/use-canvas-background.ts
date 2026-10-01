import {
  CanvasBackgroundGlobalKey,
  CanvasBackgroundSetting,
  canvasBackgroundFromGlobals,
} from '@/components/canvas/canvas-background';
import {
  AgentKeys,
  useFetchAgent,
  useSetAgent,
} from '@/hooks/use-agent-request';
import { IFlow } from '@/interfaces/database/agent';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';
import { useParams } from 'react-router';
import { useBuildDslData } from './use-build-dsl';

export function useCanvasBackground() {
  const { id } = useParams();
  const { data } = useFetchAgent();
  const { buildDslData } = useBuildDslData();
  const { setAgent } = useSetAgent(false, true);
  const queryClient = useQueryClient();
  const setting = canvasBackgroundFromGlobals(data?.dsl?.globals);

  const update = useCallback(
    async (next: CanvasBackgroundSetting) => {
      if (!id || !data?.title) return;
      const dsl = buildDslData();
      dsl.globals = {
        ...(dsl.globals ?? {}),
        [CanvasBackgroundGlobalKey]: next,
      };
      const detailKey = AgentKeys.detail(id);
      const previous = queryClient.getQueryData<IFlow>(detailKey);
      queryClient.setQueryData<IFlow>(detailKey, (current) => {
        if (!current?.dsl) return current;
        return {
          ...current,
          dsl: {
            ...current.dsl,
            globals: {
              ...(current.dsl.globals ?? {}),
              [CanvasBackgroundGlobalKey]: next,
            },
          },
        };
      });
      try {
        const response = await setAgent({ id, title: data.title, dsl });
        if (response?.code !== 0) {
          queryClient.setQueryData(detailKey, previous);
        }
      } catch (error) {
        queryClient.setQueryData(detailKey, previous);
        throw error;
      }
    },
    [buildDslData, data?.title, id, queryClient, setAgent],
  );

  return { setting, update };
}
