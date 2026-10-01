import {
  CanvasBackgroundGlobalKey,
  CanvasBackgroundSetting,
  canvasBackgroundFromGlobals,
  readStoredCanvasBackground,
  writeStoredCanvasBackground,
} from '@/components/canvas/canvas-background';
import {
  AgentKeys,
  useFetchAgent,
  useSetAgent,
} from '@/hooks/use-agent-request';
import { IFlow } from '@/interfaces/database/agent';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect } from 'react';
import { useParams } from 'react-router';
import { useBuildDslData } from './use-build-dsl';

export function useCanvasBackground() {
  const { id } = useParams();
  const { data } = useFetchAgent();
  const { buildDslData } = useBuildDslData();
  const { setAgent } = useSetAgent(false, true);
  const queryClient = useQueryClient();
  const loaded = Boolean(data?.id);
  const fetched = canvasBackgroundFromGlobals(data?.dsl?.globals);
  const setting = loaded
    ? fetched
    : (readStoredCanvasBackground(id) ?? null);

  useEffect(() => {
    if (!loaded || !id) return;
    writeStoredCanvasBackground(id, fetched);
  }, [fetched.color, fetched.image, fetched.mode, id, loaded]);

  const update = useCallback(
    async (next: CanvasBackgroundSetting) => {
      if (!id || !data?.title) return;
      const dsl = buildDslData();
      dsl.globals = {
        ...(dsl.globals ?? {}),
        [CanvasBackgroundGlobalKey]: next,
      };
      writeStoredCanvasBackground(id, next);
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
          if (previous?.dsl) {
            writeStoredCanvasBackground(
              id,
              canvasBackgroundFromGlobals(previous.dsl.globals),
            );
          }
        }
      } catch (error) {
        queryClient.setQueryData(detailKey, previous);
        if (previous?.dsl) {
          writeStoredCanvasBackground(
            id,
            canvasBackgroundFromGlobals(previous.dsl.globals),
          );
        }
        throw error;
      }
    },
    [buildDslData, data?.title, id, queryClient, setAgent],
  );

  return { setting, update };
}
