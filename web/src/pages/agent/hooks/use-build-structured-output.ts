import { get } from 'lodash';
import { ReactNode, useCallback } from 'react';
import { AgentStructuredOutputField, Operator } from '../constant';
import useGraphStore from '../store';
import { filterAgentStructuredOutput } from '../utils/filter-agent-structured-output';

function getNodeId(value: string) {
  return value.split('@').at(0);
}

export function useShowSecondaryMenu() {
  const { getOperatorTypeFromId } = useGraphStore((state) => state);

  const showSecondaryMenu = useCallback(
    (value: string, outputLabel: string) => {
      const nodeId = getNodeId(value);
      return (
        getOperatorTypeFromId(nodeId) === Operator.Agent &&
        outputLabel === AgentStructuredOutputField
      );
    },
    [getOperatorTypeFromId],
  );

  return showSecondaryMenu;
}

export function useFilterStructuredOutputByValue() {
  const { getOperatorTypeFromId, getNode, clickedNodeId } = useGraphStore(
    (state) => state,
  );

  const filterStructuredOutput = useCallback(
    (value: string) => {
      const node = getNode(getNodeId(value));
      const structuredOutput = get(
        node,
        `data.form.outputs.${AgentStructuredOutputField}`,
      );

      const filteredStructuredOutput = filterAgentStructuredOutput(
        structuredOutput,
        getOperatorTypeFromId(clickedNodeId),
      );

      return filteredStructuredOutput;
    },
    [clickedNodeId, getNode, getOperatorTypeFromId],
  );

  return filterStructuredOutput;
}

export function useFindAgentStructuredOutputLabel() {
  const getOperatorTypeFromId = useGraphStore(
    (state) => state.getOperatorTypeFromId,
  );

  const findAgentStructuredOutputLabel = useCallback(
    (
      value: string,
      options: Array<{
        label: string;
        value: string;
        parentLabel?: string | ReactNode;
        icon?: ReactNode;
      }>,
    ) => {
      // agent structured output
      const fields = value.split('@');
      if (
        getOperatorTypeFromId(fields.at(0)) === Operator.Agent &&
        fields.at(1)?.startsWith(AgentStructuredOutputField)
      ) {
        // is agent structured output
        const agentOption = options.find((x) => value.includes(x.value));
        const jsonSchemaFields = fields
          .at(1)
          ?.slice(AgentStructuredOutputField.length);

        return {
          ...agentOption,
          label: (agentOption?.label ?? '') + jsonSchemaFields,
          value: value,
        };
      }
    },
    [getOperatorTypeFromId],
  );

  return findAgentStructuredOutputLabel;
}
