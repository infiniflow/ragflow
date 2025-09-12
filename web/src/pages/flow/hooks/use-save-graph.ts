import { useFetchFlow, useResetFlow, useSetFlow } from '@/hooks/flow-hooks';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { useDebounceEffect } from 'ahooks';
import dayjs from 'dayjs';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'umi';
import useGraphStore from '../store';
import { useBuildDslData } from './use-build-dsl';

export const useSaveGraph = () => {
  const { data } = useFetchFlow();
  const { setFlow, loading } = useSetFlow();
  const { id } = useParams();
  const { buildDslData } = useBuildDslData();

  const saveGraph = useCallback(
    async (currentNodes?: RAGFlowNodeType[]) => {
      return setFlow({
        id,
        title: data.title,
        dsl: buildDslData(currentNodes),
      });
    },
    [setFlow, id, data.title, buildDslData],
  );

  return { saveGraph, loading };
};

export const useSaveGraphBeforeOpeningDebugDrawer = (show: () => void) => {
  const { saveGraph, loading } = useSaveGraph();
  const { resetFlow } = useResetFlow();

  const handleRun = useCallback(
    async (nextNodes?: RAGFlowNodeType[]) => {
      const saveRet = await saveGraph(nextNodes);
      if (saveRet?.code === 0) {
        // Call the reset api before opening the run drawer each time
        const resetRet = await resetFlow();
        // After resetting, all previous messages will be cleared.
        if (resetRet?.code === 0) {
          show();
        }
      }
    },
    [saveGraph, resetFlow, show],
  );

  return { handleRun, loading };
};

export const useWatchAgentChange = (chatDrawerVisible: boolean) => {
  console.log(
    '🚀 ~ useWatchAgentChange ~ chatDrawerVisible:',
    chatDrawerVisible,
  );
  const [time, setTime] = useState<string>();
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);
  // const { saveGraph } = useSaveGraph();
  const { data: flowDetail } = useFetchFlow();

  const setSaveTime = useCallback((updateTime: number) => {
    setTime(dayjs(updateTime).format('YYYY-MM-DD HH:mm:ss'));
  }, []);

  useEffect(() => {
    setSaveTime(flowDetail?.update_time);
  }, [flowDetail, setSaveTime]);

  // const saveAgent = useCallback(async () => {
  //   if (!chatDrawerVisible) {
  //     const ret = await saveGraph();
  //     setSaveTime(ret.data.update_time);
  //   }
  // }, [chatDrawerVisible, saveGraph, setSaveTime]);

  useDebounceEffect(
    () => {
      // saveAgent();
    },
    [nodes, edges],
    {
      wait: 1000 * 20,
    },
  );

  return time;
};
