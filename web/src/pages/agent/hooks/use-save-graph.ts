import message from '@/components/ui/message';
import { ModelTypeMap } from '@/components/model-tree-select';
import { useModelValidIds } from '@/hooks/use-llm-request';
import { Operator } from '../constant';
import {
  useFetchAgent,
  useResetAgent,
  useSaveAgent,
} from '@/hooks/use-agent-request';
import {
  GlobalVariableType,
  RAGFlowNodeType,
} from '@/interfaces/database/agent';
import { formatDate } from '@/utils/date';
import i18n from '@/locales/config';
import { useDebounceEffect } from 'ahooks';
import { useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router';
import useGraphStore from '../store';
import { collectCanvasIssues } from '../utils/canvas-checklist';
import { useBuildDslData } from './use-build-dsl';

export const useSaveGraph = (
  showMessage: boolean = true,
  skipDetailRefresh: boolean = false,
) => {
  const { data } = useFetchAgent();
  const { saveAgent, loading } = useSaveAgent(showMessage, skipDetailRefresh);
  const { id } = useParams();
  const { buildDslData } = useBuildDslData();

  // Draft saving is independent of run-readiness checks.
  const saveGraph = useCallback(
    async (
      currentNodes?: RAGFlowNodeType[],
      otherParam?: {
        globalVariables: Record<string, GlobalVariableType>;
      },
      release?: boolean,
      autosave?: boolean,
      alias?: string,
    ) => {
      if (!id) {
        return;
      }

      const params: Record<string, any> = {
        id,
        title: data.title,
        dsl: buildDslData(currentNodes, otherParam),
      };

      if (release) {
        params.release = 'true';
      }
      // Both the debounced auto-save and the explicit Save button pass
      // autosave:true so the backend refreshes only the live draft and never
      // mints a new version. Only publishing (release) writes a version row,
      // optionally carrying an alias into user_canvas_version.
      if (autosave) {
        params.autosave = true;
      }
      if (alias) {
        params.alias = alias;
      }

      return saveAgent(params);
    },
    [id, data.title, buildDslData, saveAgent],
  );

  return { saveGraph, loading };
};

export const useSaveGraphBeforeOpeningDebugDrawer = (show: () => void) => {
  const { saveGraph, loading } = useSaveGraph();
  const { resetAgent } = useResetAgent();
  const { data: agent } = useFetchAgent();
  const { resolveValidIds } = useModelValidIds(
    ModelTypeMap.llm_id,
    agent?.tenant_id,
  );

  const handleRun = useCallback(
    async (nextNodes?: RAGFlowNodeType[]) => {
      const { nodes, edges, editedNodeFormIds } = useGraphStore.getState();
      const inputs = { nodes: nextNodes ?? nodes, edges, editedNodeFormIds };
      let issue = collectCanvasIssues({
        nodes: nextNodes ?? nodes,
        edges,
        editedNodeFormIds,
      }).find((issue) => issue.blocksRun);
      if (
        !issue &&
        inputs.nodes.some((node) => node.data?.label === Operator.Agent)
      ) {
        try {
          const modelValidIds = await resolveValidIds();
          issue = collectCanvasIssues({ ...inputs, modelValidIds }).find(
            (issue) => issue.blocksRun,
          );
        } catch {
          message.error(i18n.t('common.modelUnavailable'));
          return;
        }
      }
      if (issue) {
        message.error(
          `${issue.nodeName}: ${i18n.t(issue.messageKey, issue.messageParams)}`,
        );
        return;
      }
      const saveRet = await saveGraph(nextNodes, undefined, undefined, true);
      if (saveRet?.code === 0) {
        // Call the reset api before opening the run drawer each time
        const resetRet = await resetAgent();
        // After resetting, all previous messages will be cleared.
        if (resetRet?.code === 0) {
          show();
        }
      }
    },
    [saveGraph, resetAgent, show, resolveValidIds],
  );

  return { handleRun, loading };
};

export function shouldAutosaveCanvas({
  chatDrawerVisible,
  agentId,
  agentLoaded,
  nodeCount,
  edgeCount,
}: {
  chatDrawerVisible: boolean;
  agentId?: string;
  agentLoaded: boolean;
  nodeCount: number;
  edgeCount: number;
}): boolean {
  if (chatDrawerVisible) {
    return false;
  }
  if (!agentId || !agentLoaded) {
    return false;
  }
  // An empty store is the Zustand default and also the fallback when DSL has
  // not been applied yet. Autosaving it would PUT components:{} over a real
  // pipeline (#18771). Explicit Save still persists an empty canvas.
  if (nodeCount === 0 && edgeCount === 0) {
    return false;
  }
  return true;
}

export const useWatchAgentChange = (chatDrawerVisible: boolean) => {
  const [time, setTime] = useState<string>();
  const { id } = useParams();
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);
  const { saveGraph } = useSaveGraph(false, true);
  const { data: flowDetail } = useFetchAgent();

  const setSaveTime = useCallback((updateTime: number) => {
    setTime(formatDate(updateTime));
  }, []);

  useEffect(() => {
    setSaveTime(flowDetail?.update_time);
  }, [flowDetail, setSaveTime]);

  const saveAgent = useCallback(async () => {
    if (
      !shouldAutosaveCanvas({
        chatDrawerVisible,
        agentId: id,
        agentLoaded: Boolean(flowDetail?.id),
        nodeCount: nodes.length,
        edgeCount: edges.length,
      })
    ) {
      return;
    }
    const ret = await saveGraph(undefined, undefined, undefined, true);
    if (ret?.data?.update_time) {
      setSaveTime(ret.data.update_time);
    }
  }, [
    chatDrawerVisible,
    edges.length,
    flowDetail?.id,
    id,
    nodes.length,
    saveGraph,
    setSaveTime,
  ]);

  useDebounceEffect(
    () => {
      saveAgent();
    },
    [nodes, edges],
    {
      wait: 1000 * 20,
    },
  );

  return time;
};
