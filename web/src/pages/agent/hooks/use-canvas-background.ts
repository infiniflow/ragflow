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
      queryClient.setQueryData<IFlow>(AgentKeys.detail(id), (current) => {
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
      await setAgent({ id, title: data.title, dsl });
    },
    [buildDslData, data?.title, id, queryClient, setAgent],
  );

  return { setting, update };
}
