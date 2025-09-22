import { useSetModalState } from '@/hooks/common-hooks';
import { EmptyDsl, useSetAgent } from '@/hooks/use-agent-request';
import { DSL } from '@/interfaces/database/agent';
import { AgentCategory } from '@/pages/agent/constant';
import { BeginId, Operator } from '@/pages/data-flow/constant';
import { useCallback } from 'react';
import { FlowType } from '../constant';
import { FormSchemaType } from '../create-agent-form';

export const DataflowEmptyDsl = {
  graph: {
    nodes: [
      {
        id: BeginId,
        type: 'beginNode',
        position: {
          x: 50,
          y: 200,
        },
        data: {
          label: Operator.Begin,
          name: Operator.Begin,
        },
        sourcePosition: 'left',
        targetPosition: 'right',
      },
    ],
    edges: [],
  },
  components: {
    [Operator.Begin]: {
      obj: {
        component_name: Operator.Begin,
        params: {},
      },
      downstream: [], // other edge target is downstream, edge source is current node id
      upstream: [], // edge source is upstream, edge target is current node id
    },
  },
  retrieval: [], // reference
  history: [],
  path: [],
  globals: {},
};

export function useCreateAgentOrPipeline() {
  const { loading, setAgent } = useSetAgent();
  const {
    visible: creatingVisible,
    hideModal: hideCreatingModal,
    showModal: showCreatingModal,
  } = useSetModalState();

  const handleCreateAgentOrPipeline = useCallback(
    async (data: FormSchemaType) => {
      const isAgent = data.type === FlowType.Agent;
      const ret = await setAgent({
        title: data.name,
        dsl: isAgent ? (EmptyDsl as DSL) : (DataflowEmptyDsl as DSL),
        canvas_category: isAgent
          ? AgentCategory.AgentCanvas
          : AgentCategory.DataflowCanvas,
      });

      if (ret.code === 0) {
        hideCreatingModal();
      }
    },
    [hideCreatingModal, setAgent],
  );

  return {
    loading: loading,
    creatingVisible,
    hideCreatingModal,
    showCreatingModal,
    handleCreateAgentOrPipeline,
  };
}
