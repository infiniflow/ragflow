import message from '@/components/ui/message';
import {
  useFetchAgent,
  useResetAgent,
  useSetAgent,
} from '@/hooks/use-agent-request';
import {
  GlobalVariableType,
  RAGFlowNodeType,
} from '@/interfaces/database/agent';
import { formatDate } from '@/utils/date';
import { useDebounceEffect } from 'ahooks';
import { t } from 'i18next';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { Operator } from '../constant';
import { FormSchema as ParserFormSchema } from '../form/parser-form';
import { FormSchema as RetrievalFormSchema } from '../form/retrieval-form/next';
import useGraphStore from '../store';
import { getEmptyMessageNodeNames } from '../utils';
import { findAgentNodeWithoutModel } from '../utils/agent-node-model';
import { useBuildDslData } from './use-build-dsl';

/**
 * A node's form only validates while its panel is mounted, so bad values can
 * sit in the store long after the user has moved on. Re-check them against the
 * same schema before persisting, but only for nodes the user actually edited —
 * a node still carrying stale DSL from an older version must not wedge saving.
 */
function findInvalidNode(
  nodes: RAGFlowNodeType[],
  editedNodeIds: string[],
): { node: RAGFlowNodeType; messageKey: string } | undefined {
  const invalidParserNode = nodes.find(
    (node) =>
      editedNodeIds.includes(node.id) &&
      node.data?.label === Operator.Parser &&
      !ParserFormSchema.safeParse(node.data?.form).success,
  );
  if (invalidParserNode) {
    return { node: invalidParserNode, messageKey: 'flow.nodeFormInvalid' };
  }
  // A Retrieval node sourcing from datasets must name at least one dataset,
  // and one sourcing from memories must name at least one memory; the backend
  // otherwise rejects the run with a `dataset_ids`/`memory_ids is required`
  // error that only surfaces at runtime. The same applies to Retrieval tools
  // embedded in an Agent node, which is how template-built agents usually
  // bind retrieval. Only forms/tools carrying the canonical `dataset_ids`/
  // `memory_ids` field are checked, so legacy DSLs still keyed on `kb_ids`
  // cannot wedge saving. The warning follows the field that actually failed,
  // so a memory-mode retrieval never reports a missing dataset.
  const toInvalidRetrieval = (
    params: Record<string, any>,
  ): { memoryMissing: boolean } | undefined => {
    if (
      !Array.isArray(params?.dataset_ids) &&
      !Array.isArray(params?.memory_ids)
    ) {
      return undefined;
    }
    const parsed = RetrievalFormSchema.safeParse(params);
    if (parsed.success) {
      return undefined;
    }
    return {
      memoryMissing: parsed.error.issues.some(
        (issue) => issue.path[0] === 'memory_ids',
      ),
    };
  };

  for (const node of nodes) {
    if (node.data?.label === Operator.Retrieval) {
      const invalid = toInvalidRetrieval(node.data?.form);
      if (invalid) {
        return {
          node,
          messageKey: invalid.memoryMissing
            ? 'flow.retrievalMemoryMissing'
            : 'flow.retrievalDatasetMissing',
        };
      }
    }
    if (node.data?.label === Operator.Agent) {
      const tools = node.data?.form?.tools;
      if (Array.isArray(tools)) {
        for (const tool of tools) {
          if (tool?.component_name !== 'Retrieval') {
            continue;
          }
          const invalid = toInvalidRetrieval(tool?.params);
          if (invalid) {
            return {
              node,
              messageKey: invalid.memoryMissing
                ? 'flow.retrievalMemoryMissing'
                : 'flow.retrievalDatasetMissing',
            };
          }
        }
      }
    }
  }
  // Unlike the schema re-check above, the missing-model check is not gated on
  // editedNodeIds: a canvas loaded from storage with an empty model must warn
  // on the very next save, not only after the node is edited again.
  const agentNode = findAgentNodeWithoutModel(nodes);
  if (agentNode) {
    return { node: agentNode, messageKey: 'flow.agentModelMissing' };
  }
  return undefined;
}

export const useValidateNodeForms = () => {
  const { t } = useTranslation();
  const nodes = useGraphStore((state) => state.nodes);
  const editedNodeFormIds = useGraphStore((state) => state.editedNodeFormIds);

  const getInvalidNode = useCallback(
    (currentNodes?: RAGFlowNodeType[]) =>
      findInvalidNode(currentNodes ?? nodes, editedNodeFormIds),
    [editedNodeFormIds, nodes],
  );

  // Only the entry points the user drives explicitly should warn; autosave and
  // publish stay silent and just skip the write.
  const notifyIfInvalid = useCallback(
    (currentNodes?: RAGFlowNodeType[]) => {
      const invalid = getInvalidNode(currentNodes);
      if (invalid) {
        message.warning(
          t(invalid.messageKey, { name: invalid.node.data?.name }),
        );
        return false;
      }
      return true;
    },
    [getInvalidNode, t],
  );

  return { getInvalidNode, notifyIfInvalid };
};

export const useSaveGraph = (
  showMessage: boolean = true,
  skipInvalidation: boolean = false,
) => {
  const { data } = useFetchAgent();
  const { setAgent, loading } = useSetAgent(showMessage, skipInvalidation);
  const { id } = useParams();
  const { buildDslData } = useBuildDslData();
  const { getInvalidNode } = useValidateNodeForms();

  const saveGraph = useCallback(
    async (
      currentNodes?: RAGFlowNodeType[],
      otherParam?: {
        globalVariables: Record<string, GlobalVariableType>;
      },
      release?: boolean,
    ) => {
      if (!id) {
        return;
      }

      if (getInvalidNode(currentNodes)) {
        return;
      }

      // Warn about Message components with empty content at save time; the
      // canvas only fails on them when the agent runs otherwise (backend
      // MessageParam.check() rejects empty content). Advisory only — the save
      // itself proceeds. Autosave/publish (showMessage=false) stay silent to
      // avoid nagging toasts.
      if (showMessage) {
        const emptyMessageNodeNames = getEmptyMessageNodeNames(
          currentNodes ?? useGraphStore.getState().nodes,
        );
        if (emptyMessageNodeNames.length > 0) {
          message.warning(
            `${emptyMessageNodeNames.join(', ')}: ${t('flow.messageMsg')}`,
          );
          return;
        }
      }

      const params: Record<string, any> = {
        id,
        title: data.title,
        dsl: buildDslData(currentNodes, otherParam),
      };

      if (release) {
        params.release = 'true';
      }

      return setAgent(params);
    },
    [id, getInvalidNode, showMessage, data.title, buildDslData, setAgent],
  );

  return { saveGraph, loading };
};

export const useSaveGraphBeforeOpeningDebugDrawer = (show: () => void) => {
  const { saveGraph, loading } = useSaveGraph();
  const { resetAgent } = useResetAgent();
  const { notifyIfInvalid } = useValidateNodeForms();

  const handleRun = useCallback(
    async (nextNodes?: RAGFlowNodeType[]) => {
      if (!notifyIfInvalid(nextNodes)) {
        return;
      }

      const saveRet = await saveGraph(nextNodes);
      if (saveRet?.code === 0) {
        // Call the reset api before opening the run drawer each time
        const resetRet = await resetAgent();
        // After resetting, all previous messages will be cleared.
        if (resetRet?.code === 0) {
          show();
        }
      }
    },
    [notifyIfInvalid, saveGraph, resetAgent, show],
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
    const ret = await saveGraph();
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
