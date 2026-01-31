import { AgentCategory, Operator } from '@/constants/agent';
import { useSetModalState } from '@/hooks/common-hooks';
import { EmptyDsl, useSetAgent } from '@/hooks/use-agent-request';
import { DSL } from '@/interfaces/database/agent';

import { FileId, initialParserValues } from '@/pages/agent/constant';
import { useCallback } from 'react';
import { FlowType } from '../constant';
import { FormSchemaType } from '../create-agent-form';

export const DataflowEmptyDsl = {
  graph: {
    nodes: [
      {
        id: FileId,
        type: 'beginNode',
        position: {
          x: 50,
          y: 200,
        },
        data: {
          label: Operator.File,
          name: Operator.File,
        },
        sourcePosition: 'left',
        targetPosition: 'right',
      },
      {
        data: {
          form: initialParserValues,
          label: 'Parser',
          name: 'Parser_0',
        },
        dragging: false,
        id: 'Parser:HipSignsRhyme',
        measured: {
          height: 57,
          width: 200,
        },
        position: {
          x: 316.99524094206413,
          y: 195.39629819663406,
        },
        selected: true,
        sourcePosition: 'right',
        targetPosition: 'left',
        type: 'parserNode',
      },
    ],
    edges: [
      {
        id: 'xy-edge__Filestart-Parser:HipSignsRhymeend',
        source: FileId,
        sourceHandle: 'start',
        target: 'Parser:HipSignsRhyme',
        targetHandle: 'end',
      },
    ],
  },
  components: {
    [Operator.File]: {
      obj: {
        component_name: Operator.File,
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
